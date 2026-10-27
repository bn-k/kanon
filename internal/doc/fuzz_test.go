package doc

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

// FuzzParse parses a document: it doesn't panic, every diagnostic is a message at a place of the text, and every
// section gives its text back.
func FuzzParse(f *testing.F) {
	addFiles(f, "../../cases", ".kan")
	addFiles(f, "../sqlexport/testdata", ".kan")
	addFiles(f, "../sqlimport/testdata", ".kan")
	f.Fuzz(func(t *testing.T, src string) {
		d, diags := Parse("fuzz.kan", src)
		lines := strings.Split(src, "\n")
		for _, dg := range diags {
			if !within(dg.Pos, lines) {
				t.Errorf("%s: outside the text", dg)
			}
			if r, _ := utf8.DecodeRuneInString(dg.Msg); dg.Msg == "" || unicode.IsUpper(r) || strings.HasSuffix(dg.Msg, ".") {
				t.Errorf("%s: not a message", dg)
			}
		}
		var walk func(s *Section)
		walk = func(s *Section) {
			if h := s.Heading; h != nil && (!within(h.Pos, lines) || !within(h.TextPos, lines)) {
				t.Errorf("heading %q at %s, its text at %s: outside the text", h.Text, h.Pos, h.TextPos)
			}
			for _, k := range s.Keys {
				if !within(k.Pos, lines) || !within(k.ValuePos, lines) {
					t.Errorf("key line %q at %s, its value at %s: outside the text", k.Text, k.Pos, k.ValuePos)
				}
			}
			for _, b := range s.Blocks {
				if !within(b.At(), lines) || b.End() < b.At().Line || b.End() > len(lines) {
					t.Errorf("block at %s to line %d: outside the text", b.At(), b.End())
				}
				d.LineEnd(b.End())
			}
			d.Text(s.Keys, s.Blocks)
			for _, c := range s.Children {
				walk(c)
			}
		}
		walk(d.Root)
	})
}

// within reports whether pos is a line and a column of the text whose lines are lines.
func within(pos diag.Pos, lines []string) bool {
	return pos.File == "fuzz.kan" && pos.Line >= 1 && pos.Line <= len(lines) && pos.Col >= 1 &&
		pos.Col <= utf8.RuneCountInString(lines[pos.Line-1])+1
}

// addFiles adds the text of each file under root whose name ends with ext to the seed corpus. The Northwind sample is
// left out: its thousand documents are of the same few shapes.
func addFiles(f *testing.F, root, ext string) {
	err := testcases.Walk(root, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && d.Name() == "northwind":
			return filepath.SkipDir
		case d.IsDir() || filepath.Ext(path) != ext:
			return nil
		}
		src, err := os.ReadFile(path)
		f.Add(string(src))
		return err
	})
	if err != nil {
		f.Fatal(err)
	}
}
