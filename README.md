# dupl

**dupl** is a tool written in Go for finding code clones. It can find clones only
in the Go source files. The method uses a suffix tree for serialized ASTs. It ignores
values of AST nodes. It just operates with their types (e.g. `if a == 13 {}` and
`if x == 100 {}` are considered the same provided it exceeds the minimal token
sequence size).

Due to the used method dupl can report so called "false positives" on the output.
These are the ones we do not consider clones (whether they are too small, or the
values of the matched tokens are completely different).

## Installation

```bash
go install github.com/mibk/dupl@latest
```

## Usage

```
Usage: dupl [flags] [paths]

Paths:
  If the given path is a file, dupl will use it regardless of
  the file extension. If it is a directory, it will recursively
  search for *.go files in that directory.

  If no path is given, dupl will recursively search for *.go
  files in the current directory.

Flags:
  -files
        read file names from stdin one at each line
  -html
        output the results as HTML, including duplicate code fragments
  -plumbing
        plumbing (easy-to-parse) output for consumption by scripts or tools
  -t, -threshold size
        minimum token sequence size as a clone (default 100)
  -vendor
        check files in vendor directory
  -v, -verbose
        explain what is being done

Examples:
  dupl -t 200
        Search clones in the current directory of size at least
        200 tokens.
  dupl $(find app/ -name '*_test.go')
        Search for clones in tests in the app directory.
  find app/ -name '*_test.go' |dupl -files
        The same as above.
```

## Package API

This fork also exposes a package API for integrations that need structured results
and golangci-lint-compatible JSON output:

```go
package main

import (
	"encoding/json"
	"fmt"

	dupl "github.com/mibk/dupl/pkg"
)

func main() {
	opts := dupl.DefaultOptions()
	opts.Threshold = 100
	opts.ExcludePathSubstrings = []string{"vendor/generated"}

	report, err := dupl.CheckGolangCILintJSON([]string{"."}, opts)
	if err != nil {
		panic(err)
	}

	data, err := json.Marshal(report)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(data))
}
```

Use `CheckPaths` when you need Heuris-native diagnostics with clone group
metadata such as `Hash`, `GroupID`, and `DuplicateOf`. Set
`ExcludePathSubstrings` to skip any file path that contains one of the configured
substrings.

## Example

The reduced output of this command with the following parameters for the
[Docker](https://www.docker.com) source code looks like
[this](http://htmlpreview.github.io/?https://github.com/mibk/dupl/blob/master/_output_example/docker.html).

```bash
$ dupl -t 200 -html >docker.html
```
