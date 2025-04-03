package main

import (
	"flag"
	"fmt"
	"go/token"
	"log"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

var (
	repo         string
	module       string
	dir          string
	sourcesFlag  string
	sinksFlag    string
	testModeFlag string
	testMode     bool
)

var srcs []string
var sinks []string

func main() {
	// Define command-line flags
	flag.StringVar(&repo, "repo", "", "Name of the repository using the tool")
	flag.StringVar(&sourcesFlag, "sources", "", "Comma-separated filepaths where the entrypoints/cloudfns are called")
	flag.StringVar(&sinksFlag, "sinks", "", "Comma-separated filepaths that have changes made")
	flag.StringVar(&testModeFlag, "test", "false", "Test mode, true or false")
	flag.Parse()

	// Validate required flags
	if repo == "" || sourcesFlag == "" || sinksFlag == "" {
		log.Fatal("Error: repo, sources, and sinks flags are required")
	}

	testMode = testModeFlag == "true"

	// Set module and dir based on repo
	module = "educabot.com/" + repo
	if testMode {
		dir = "../" + repo
	} else {
		dir = "./"
	}

	// Split comma-separated paths into slices
	srcs = strings.Split(sourcesFlag, ",")
	sinks = strings.Split(sinksFlag, ",")

	// Trim whitespace from each path
	for i := range srcs {
		src := strings.TrimSpace(srcs[i])
		srcs[i] = filepath.Join(dir, src)
	}
	for i := range sinks {
		sink := strings.TrimSpace(sinks[i])
		sinks[i] = filepath.Join(dir, sink)
	}

	cfg := &packages.Config{
		Mode: packages.LoadAllSyntax,
		Dir:  dir,
	}
	initial, err := packages.Load(cfg, dir)
	if err != nil {
		log.Fatal("Error loading packages:", err)
	}
	if packages.PrintErrors(initial) > 0 {
		log.Fatal("Error loading packages:", packages.PrintErrors(initial))
	}

	// Create and build SSA-form program representation.
	mode := ssa.InstantiateGenerics // instantiate generics by default for soundness
	prog, _ := ssautil.AllPackages(initial, mode)
	prog.Build()
	// Generate the call graph
	cg := cha.CallGraph(prog)
	cg.DeleteSyntheticNodes()

	toRemove := make([]*callgraph.Node, 0)
	for _, node := range cg.Nodes {
		if node.Func != nil {
			pos := prog.Fset.Position(node.Func.Pos())
			filename := pos.Filename
			if strings.Contains(filename, "wire_gen") {
				toRemove = append(toRemove, node)
			}
			if !strings.Contains(node.Func.String(), module) {
				toRemove = append(toRemove, node)
			}
		}
	}
	for _, node := range toRemove {
		cg.DeleteNode(node)
	}

	// Create maps for source and sink functions
	sourceFuncs := make(map[*ssa.Function]bool)
	sinkFuncs := make(map[*ssa.Function]bool)
	fset := prog.Fset
	for _, node := range cg.Nodes {
		if node.Func != nil {
			pos := fset.Position(node.Func.Pos())
			filename := pos.Filename

			// Check if function is in a source file
			for _, src := range srcs {
				s, _ := filepath.Abs(src)
				if s == filename {
					sourceFuncs[node.Func] = true
					break
				}
			}

			// Check if function is in a sink file
			for _, sink := range sinks {
				s, _ := filepath.Abs(sink)
				if s == filename {
					sinkFuncs[node.Func] = true
					break
				}
			}
		}
	}

	// Build reachability graph (adjacency list)
	g := make(map[*ssa.Function]map[*ssa.Function]bool)
	err = callgraph.GraphVisitEdges(cg, func(edge *callgraph.Edge) error {
		caller := edge.Caller.Func
		callee := edge.Callee.Func

		// check that both caller and callee are in module
		if caller == nil || callee == nil {
			return nil
		}
		if !strings.Contains(caller.String(), module) || !strings.Contains(callee.String(), module) {
			return nil
		}
		if g[caller] == nil {
			g[caller] = make(map[*ssa.Function]bool)
		}
		g[caller][callee] = true
		return nil
	})
	if err != nil {
		log.Fatal("Error visiting edges:", err)
	}

	// Add edges between functions and their anonymous versions
	for _, node := range cg.Nodes {
		if node.Func != nil {
			funcName := node.Func.String()
			if !strings.Contains(funcName, module) {
				continue
			}
			// Check if this is a named function that might have anonymous functions
			if !strings.Contains(funcName, "$") {
				// Look for anonymous functions derived from this one
				baseFuncName := funcName
				for _, otherNode := range cg.Nodes {
					if otherNode.Func != nil {
						otherFuncName := otherNode.Func.String()
						if !strings.Contains(otherFuncName, module) {
							continue
						}
						// Check if the other function is an anonymous function of this one
						if strings.HasPrefix(otherFuncName, baseFuncName+"$") {
							// Add edge from the named function to its anonymous function
							if g[node.Func] == nil {
								g[node.Func] = make(map[*ssa.Function]bool)
							}
							g[node.Func][otherNode.Func] = true

							// For debugging
							//fmt.Printf("Added edge: %s -> %s\n",
							//	node.Func.String(), otherNode.Func.String())
						}
					}
				}
			}
		}
	}

	// Find paths from sources to sinks
	fmt.Println("Analyzing paths from sources to sinks:")

	// For each source function
	for sourceFunc := range sourceFuncs {
		sourcePos := fset.Position(sourceFunc.Pos())
		fmt.Printf("\nSource: %s (%s:%d)\n", sourceFunc.Name(), sourcePos.Filename, sourcePos.Line)

		// Find sink reachability
		reachedSinks := make(map[*ssa.Function]bool)
		visited := make(map[*ssa.Function]bool)

		// Use DFS to find one path to each reachable sink
		for sinkFunc := range sinkFuncs {
			if visited[sinkFunc] {
				continue
			}

			path := findPath(fset, sourceFunc, sinkFunc, g, make(map[*ssa.Function]bool))
			if path != nil {
				reachedSinks[sinkFunc] = true

				// Print the path
				sinkPos := fset.Position(sinkFunc.Pos())
				fmt.Printf("  Sink reached: %s (%s:%d)\n", sinkFunc.Name(), sinkPos.Filename, sinkPos.Line)
				fmt.Println("  Path:")
				for i, func_ := range path {
					pos := fset.Position(func_.Pos())
					fmt.Printf("    %d. %s (%s:%d)\n", i+1, func_.Name(), pos.Filename, pos.Line)
				}
			}
		}

		if len(reachedSinks) == 0 {
			fmt.Println("  No sinks reached from this source.")
		}
	}
}

// findPath uses DFS to find a path from src to dest
func findPath(fset *token.FileSet, src, dest *ssa.Function, graph map[*ssa.Function]map[*ssa.Function]bool, visited map[*ssa.Function]bool) []*ssa.Function {
	posSRC := fset.Position(src.Pos())
	posDEST := fset.Position(dest.Pos())
	if posSRC.Filename == posDEST.Filename {
		return []*ssa.Function{src}
	}
	visited[src] = true

	neighbourhood := graph[src]
	for neighbor := range neighbourhood {
		if !visited[neighbor] {
			if path := findPath(fset, neighbor, dest, graph, visited); path != nil {
				return append([]*ssa.Function{src}, path...)
			}
		}
	}

	return nil
}
