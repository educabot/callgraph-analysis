# Functions-Tool

A static analysis tool for Go codebases that identifies which entrypoints (cloud functions, APIs, etc.) are affected by code changes. This helps developers and QA teams determine what needs to be deployed and tested in a PR.

## Purpose

This tool analyzes call graphs in your codebase to trace paths from source functions (entrypoints) to sink functions (where changes were made). By understanding these connections, you can:

- Identify which cloud functions or API endpoints are impacted by code changes
- Focus testing efforts on affected functionality
- Make more informed deployment decisions

## Installation

1. Clone this repository
2. Ensure you have Go installed (version 1.18+ recommended)
3. Initialize the Go module and install dependencies:

```bash
go mod init functions-tool
go mod tidy
```

## Usage

Run the tool with the following command:

```bash
go run main.go -repo=REPO_NAME -sources=SOURCE_FILES -sinks=SINK_FILES [-test=BOOL]
```

### Required Flags

- `-repo`: Name of the repository being analyzed (used to construct the module name)
  - Example: `-repo=ted`

- `-sources`: Comma-separated list of filepath(s) where entrypoints or cloud functions are defined
  - Example: `-sources="functions.go,src/app/web/mapping.go"`

- `-sinks`: Comma-separated list of filepath(s) that contain code changes
  - Example: `-sinks="src/core/usecases/videos/save_v2.go"`

### Optional Flags

- `-test`: Test mode flag (default: "false")
  - When set to "true", the tool looks for the repository in the parent directory, this means that the repository to analyze is cloned in the parent directory
  - When "false", it uses the current directory
  - Example: `-test=true`

## Examples

```bash
# Regular mode (analyzing code in current directory)
go run main.go -repo=ted -sources="functions.go,src/app/web/mapping.go" -sinks="src/core/usecases/videos/save_v2.go"

# Test mode (analyzing code in parent directory)
go run main.go -repo=ted -sources="functions.go,src/app/web/mapping.go" -sinks="src/core/usecases/videos/save_v2.go" -test=true
```

## Output

The tool will output a list of source functions (entrypoints) and the paths through which they reach any sink functions. This helps identify which entrypoints are affected by changes in the sink files.

## Requirements

- Go 1.18 or higher
- golang.org/x/tools package

## Troubleshooting

- **Module not found**: Ensure you've run `go mod init` and `go mod tidy`
- **Directory not found**: Check the path construction with the `-test` flag
- **No sources/sinks found**: Verify file paths are correct relative to the directory being analyzed