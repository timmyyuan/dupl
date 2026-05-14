package pkg

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mibk/dupl/suffixtree"
	"github.com/mibk/dupl/syntax"
	"github.com/mibk/dupl/syntax/golang"
)

const (
	LinterName       = "dupl"
	DefaultThreshold = 100
)

type Options struct {
	Threshold     int
	IncludeVendor bool
	SkipTests     bool
}

type Diagnostic struct {
	Path        string    `json:"Path"`
	Line        int       `json:"Line"`
	Column      int       `json:"Column"`
	EndLine     int       `json:"EndLine,omitempty"`
	Message     string    `json:"Message"`
	Hash        string    `json:"Hash,omitempty"`
	GroupID     int       `json:"GroupID,omitempty"`
	DuplicateOf *Fragment `json:"DuplicateOf,omitempty"`
}

type Fragment struct {
	Path        string `json:"Path"`
	LineStart   int    `json:"LineStart"`
	ColumnStart int    `json:"ColumnStart,omitempty"`
	LineEnd     int    `json:"LineEnd"`
}

type GolangCILintJSON struct {
	Issues []GolangCILintIssue    `json:"Issues"`
	Report GolangCILintJSONReport `json:"Report"`
}

type GolangCILintJSONReport struct {
	Linters []GolangCILintLinter `json:"Linters"`
}

type GolangCILintLinter struct {
	Name    string `json:"Name"`
	Enabled bool   `json:"Enabled,omitempty"`
}

type GolangCILintIssue struct {
	FromLinter           string               `json:"FromLinter"`
	Text                 string               `json:"Text"`
	Severity             string               `json:"Severity"`
	SourceLines          []string             `json:"SourceLines"`
	Pos                  GolangCILintPosition `json:"Pos"`
	ExpectNoLint         bool                 `json:"ExpectNoLint"`
	ExpectedNoLintLinter string               `json:"ExpectedNoLintLinter"`
}

type GolangCILintPosition struct {
	Filename string `json:"Filename"`
	Offset   int    `json:"Offset"`
	Line     int    `json:"Line"`
	Column   int    `json:"Column"`
}

func DefaultOptions() Options {
	return Options{Threshold: DefaultThreshold}
}

func CheckPaths(paths []string, opts Options) ([]Diagnostic, error) {
	opts = normalizeOptions(opts)
	if len(paths) == 0 {
		paths = []string{"."}
	}

	var files []string
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			if !opts.SkipTests || !strings.HasSuffix(path, "_test.go") {
				files = append(files, path)
			}
			continue
		}

		err = filepath.Walk(path, func(filename string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if shouldSkipDir(filename, opts.IncludeVendor) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(info.Name(), ".go") {
				return nil
			}
			if opts.SkipTests && strings.HasSuffix(info.Name(), "_test.go") {
				return nil
			}
			files = append(files, filename)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	sort.Strings(files)
	return CheckFiles(files, opts)
}

func CheckFiles(files []string, opts Options) ([]Diagnostic, error) {
	opts = normalizeOptions(opts)

	tree := suffixtree.New()
	data := make([]*syntax.Node, 0, 100)
	for _, filename := range files {
		if opts.SkipTests && strings.HasSuffix(filename, "_test.go") {
			continue
		}
		root, err := golang.Parse(filename)
		if err != nil {
			return nil, err
		}
		seq := syntax.Serialize(root)
		data = append(data, seq...)
		for _, node := range seq {
			tree.Update(node)
		}
	}
	tree.Update(&syntax.Node{Type: -1})

	groups := make(map[string][][]*syntax.Node)
	for match := range tree.FindDuplOver(opts.Threshold) {
		syntaxMatch := syntax.FindSyntaxUnits(data, match, opts.Threshold)
		if len(syntaxMatch.Frags) > 0 {
			groups[syntaxMatch.Hash] = append(groups[syntaxMatch.Hash], syntaxMatch.Frags...)
		}
	}

	diagnostics, err := diagnosticsFromGroups(groups)
	if err != nil {
		return nil, err
	}
	SortDiagnostics(diagnostics)
	return diagnostics, nil
}

func CheckGolangCILintJSON(paths []string, opts Options) (GolangCILintJSON, error) {
	diagnostics, err := CheckPaths(paths, opts)
	if err != nil {
		return GolangCILintJSON{}, err
	}
	return DiagnosticsToGolangCILintJSON(diagnostics), nil
}

func DiagnosticsToGolangCILintJSON(diagnostics []Diagnostic) GolangCILintJSON {
	SortDiagnostics(diagnostics)
	issues := make([]GolangCILintIssue, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		issues = append(issues, diagnostic.GolangCILintIssue())
	}
	return GolangCILintJSON{
		Issues: issues,
		Report: GolangCILintJSONReport{
			Linters: []GolangCILintLinter{
				{Name: LinterName, Enabled: true},
			},
		},
	}
}

func (diagnostic Diagnostic) GolangCILintIssue() GolangCILintIssue {
	return GolangCILintIssue{
		FromLinter:   LinterName,
		Text:         diagnostic.Message,
		Severity:     "",
		SourceLines:  []string{},
		Pos:          GolangCILintPosition{Filename: diagnostic.Path, Line: diagnostic.Line, Column: diagnostic.Column},
		ExpectNoLint: false,
	}
}

func (output GolangCILintJSON) Diagnostics() []Diagnostic {
	diagnostics := make([]Diagnostic, 0, len(output.Issues))
	for _, issue := range output.Issues {
		diagnostics = append(diagnostics, issue.Diagnostic())
	}
	return diagnostics
}

func (issue GolangCILintIssue) Diagnostic() Diagnostic {
	return Diagnostic{
		Path:    issue.Pos.Filename,
		Line:    issue.Pos.Line,
		Column:  issue.Pos.Column,
		Message: issue.Text,
	}
}

func SortDiagnostics(diagnostics []Diagnostic) {
	sort.Slice(diagnostics, func(i, j int) bool {
		if diagnostics[i].GroupID != diagnostics[j].GroupID {
			return diagnostics[i].GroupID < diagnostics[j].GroupID
		}
		if diagnostics[i].Path != diagnostics[j].Path {
			return diagnostics[i].Path < diagnostics[j].Path
		}
		if diagnostics[i].Line != diagnostics[j].Line {
			return diagnostics[i].Line < diagnostics[j].Line
		}
		if diagnostics[i].Column != diagnostics[j].Column {
			return diagnostics[i].Column < diagnostics[j].Column
		}
		return diagnostics[i].Message < diagnostics[j].Message
	})
}

func normalizeOptions(opts Options) Options {
	if opts.Threshold <= 0 {
		opts.Threshold = DefaultThreshold
	}
	return opts
}

func shouldSkipDir(path string, includeVendor bool) bool {
	if includeVendor {
		return false
	}
	return filepath.Base(path) == "vendor"
}

func diagnosticsFromGroups(groups map[string][][]*syntax.Node) ([]Diagnostic, error) {
	hashes := make([]string, 0, len(groups))
	for hash := range groups {
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)

	var diagnostics []Diagnostic
	groupID := 0
	cache := map[string][]byte{}
	for _, hash := range hashes {
		frags := unique(groups[hash])
		if len(frags) < 2 {
			continue
		}
		fragments, err := fragmentsFromNodes(cache, frags)
		if err != nil {
			return nil, err
		}
		sortFragments(fragments)
		if len(fragments) < 2 {
			continue
		}

		groupID++
		for i, fragment := range fragments {
			duplicateOf := fragments[(i+1)%len(fragments)]
			diagnostic := Diagnostic{
				Path:        fragment.Path,
				Line:        fragment.LineStart,
				Column:      fragment.ColumnStart,
				EndLine:     fragment.LineEnd,
				Hash:        hash,
				GroupID:     groupID,
				DuplicateOf: &duplicateOf,
			}
			diagnostic.Message = duplicateMessage(diagnostic)
			diagnostics = append(diagnostics, diagnostic)
		}
	}
	return diagnostics, nil
}

func unique(group [][]*syntax.Node) [][]*syntax.Node {
	fileMap := make(map[string]map[int]struct{})

	var out [][]*syntax.Node
	for _, seq := range group {
		if len(seq) == 0 {
			continue
		}
		node := seq[0]
		file, ok := fileMap[node.Filename]
		if !ok {
			file = make(map[int]struct{})
			fileMap[node.Filename] = file
		}
		if _, ok := file[node.Pos]; ok {
			continue
		}
		file[node.Pos] = struct{}{}
		out = append(out, seq)
	}
	return out
}

func fragmentsFromNodes(cache map[string][]byte, dups [][]*syntax.Node) ([]Fragment, error) {
	fragments := make([]Fragment, 0, len(dups))
	for _, dup := range dups {
		if len(dup) == 0 {
			continue
		}
		start := dup[0]
		end := dup[len(dup)-1]
		file, err := readFile(cache, start.Filename)
		if err != nil {
			return nil, err
		}
		lineStart, columnStart, lineEnd := blockLocation(file, start.Pos, end.End)
		fragments = append(fragments, Fragment{
			Path:        start.Filename,
			LineStart:   lineStart,
			ColumnStart: columnStart,
			LineEnd:     lineEnd,
		})
	}
	return fragments, nil
}

func readFile(cache map[string][]byte, filename string) ([]byte, error) {
	if file, ok := cache[filename]; ok {
		return file, nil
	}
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	cache[filename] = file
	return file, nil
}

func blockLocation(file []byte, from, to int) (lineStart, columnStart, lineEnd int) {
	line, column := 1, 1
	for offset, b := range file {
		if offset == from {
			lineStart, columnStart = line, column
		}
		if offset == to-1 {
			lineEnd = line
			break
		}
		if b == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	if lineStart == 0 {
		lineStart, columnStart = line, column
	}
	if lineEnd == 0 {
		lineEnd = lineStart
	}
	return lineStart, columnStart, lineEnd
}

func duplicateMessage(diagnostic Diagnostic) string {
	if diagnostic.DuplicateOf == nil {
		return "duplicate code fragment"
	}
	return fmt.Sprintf(
		"duplicate code fragment; duplicate of %s:%d-%d",
		diagnostic.DuplicateOf.Path,
		diagnostic.DuplicateOf.LineStart,
		diagnostic.DuplicateOf.LineEnd,
	)
}

func sortFragments(fragments []Fragment) {
	sort.Slice(fragments, func(i, j int) bool {
		if fragments[i].Path != fragments[j].Path {
			return fragments[i].Path < fragments[j].Path
		}
		if fragments[i].LineStart != fragments[j].LineStart {
			return fragments[i].LineStart < fragments[j].LineStart
		}
		if fragments[i].ColumnStart != fragments[j].ColumnStart {
			return fragments[i].ColumnStart < fragments[j].ColumnStart
		}
		return fragments[i].LineEnd < fragments[j].LineEnd
	})
}
