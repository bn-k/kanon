package schema

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/testcases"
)

// FuzzParse parses a schema then, when it parses, checks it, as Load does: neither panics, and every diagnostic is
// a message at a place of the text.
func FuzzParse(f *testing.F) {
	addFiles(f, "../../cases", ".kanonarch")
	addFiles(f, "../sqlexport/testdata", ".kanonarch")
	addFiles(f, "../sqlimport/testdata", ".kanonarch")
	corpus, _ := filepath.Glob("../../editors/tree-sitter-kanonarch/test/corpus/*.txt")
	for _, path := range corpus {
		src, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		for _, example := range corpusExamples(string(src)) {
			f.Add(example)
		}
	}
	f.Fuzz(func(t *testing.T, src string) {
		file, diags := Parse("fuzz.kanonarch", src)
		checkDiags(t, src, diags)
		if diags.Count(diag.Error) == 0 {
			checkDiags(t, src, Check(file))
		}
	})
}

// addFiles adds the text of each file under root whose name ends with ext to the seed corpus.
func addFiles(f *testing.F, root, ext string) {
	err := testcases.Walk(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ext {
			return err
		}
		src, err := os.ReadFile(path)
		f.Add(string(src))
		return err
	})
	if err != nil {
		f.Fatal(err)
	}
}

// corpusExamples returns the source of each example of a tree-sitter corpus file: the lines between its name, set
// between two ==== lines, and its ---- line.
func corpusExamples(src string) []string {
	var examples, lines []string
	rules, in := 0, false
	for line := range strings.Lines(src) {
		switch {
		case strings.HasPrefix(line, "===="):
			rules++
			in = rules%2 == 0
			lines = nil
		case in && strings.HasPrefix(line, "----"):
			examples = append(examples, strings.Join(lines, ""))
			in = false
		case in:
			lines = append(lines, line)
		}
	}
	return examples
}

// checkDiags checks that each diagnostic of src has a message, written as CLAUDE.md says, at a line and a column
// of src.
func checkDiags(t *testing.T, src string, diags diag.List) {
	t.Helper()
	lines := strings.Split(src, "\n")
	for _, d := range diags {
		p := d.Pos
		if p.File != "fuzz.kanonarch" || p.Line < 1 || p.Line > len(lines) || p.Col < 1 ||
			p.Col > utf8.RuneCountInString(lines[p.Line-1])+1 {
			t.Errorf("%s: outside the text", d)
		}
		if r, _ := utf8.DecodeRuneInString(d.Msg); d.Msg == "" || unicode.IsUpper(r) || strings.HasSuffix(d.Msg, ".") {
			t.Errorf("%s: not a message", d)
		}
	}
}
