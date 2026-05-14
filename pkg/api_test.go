package pkg

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"testing"
)

func TestCheckFilesGolangCILintJSON(t *testing.T) {
	dir := t.TempDir()
	src := `package sample

func first(xs []int) int {
	total := 0
	for _, x := range xs {
		if x > 0 {
			total += x
		}
	}
	return total
}

func second(xs []int) int {
	total := 0
	for _, x := range xs {
		if x > 0 {
			total += x
		}
	}
	return total
}
`
	filename := filepath.Join(dir, "sample.go")
	if err := ioutil.WriteFile(filename, []byte(src), 0600); err != nil {
		t.Fatal(err)
	}

	diagnostics, err := CheckFiles([]string{filename}, Options{Threshold: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) == 0 {
		t.Fatal("expected duplicate diagnostics")
	}
	if diagnostics[0].Path != filename {
		t.Fatalf("unexpected path: %q", diagnostics[0].Path)
	}
	if diagnostics[0].DuplicateOf == nil {
		t.Fatal("expected DuplicateOf metadata")
	}

	report := DiagnosticsToGolangCILintJSON(diagnostics)
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}

	var decoded GolangCILintJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	roundTrip := decoded.Diagnostics()
	if len(roundTrip) != len(diagnostics) {
		t.Fatalf("expected %d round-trip diagnostics, got %d", len(diagnostics), len(roundTrip))
	}
	if roundTrip[0].Path != diagnostics[0].Path || roundTrip[0].Line != diagnostics[0].Line {
		t.Fatalf("round-trip mismatch: %#v != %#v", roundTrip[0], diagnostics[0])
	}
}
