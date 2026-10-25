package lint

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bn-k/kanon/internal/testcases"
)

// overlay is a folder in memory whose file name holds text in place of what the folder holds.
type overlay struct {
	fstest.MapFS
	name, text string
}

func (o overlay) ReadFile(name string) ([]byte, error) {
	if name == o.name {
		return []byte(o.text), nil
	}
	return o.MapFS.ReadFile(name)
}

// fuzzCase is a schema and the document a case of cases/ lints or evaluates with it, as paths of fuzzCases.
type fuzzCase struct{ schema, doc string }

// fuzzCases are the files of cases/ but the Northwind sample, whose thousand documents would slow every run down,
// with the schema and document of each case that lints or evaluates one.
func fuzzCases(f *testing.F) (fstest.MapFS, []fuzzCase) {
	mem := fstest.MapFS{}
	var cases []fuzzCase
	command := regexp.MustCompile(`^\$ kan (?:lint|eval) (?:-today \S+ )?(\S+) (\S+)\n`)
	err := testcases.Walk("../../cases", func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && strings.HasPrefix(d.Name(), "northwind"):
			return filepath.SkipDir
		case d.IsDir():
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name, _ := filepath.Rel("../../cases", path)
		mem[filepath.ToSlash(name)] = &fstest.MapFile{Data: data}
		if m := command.FindSubmatch(data); m != nil && d.Name() == "expected.txt" {
			dir := filepath.Dir(name)
			cases = append(cases, fuzzCase{filepath.Join(dir, string(m[1])), filepath.Join(dir, string(m[2]))})
		}
		return nil
	})
	if err != nil || len(cases) == 0 {
		f.Fatalf("reading the cases: %v, %d cases", err, len(cases))
	}
	return mem, cases
}

// FuzzLint lints a document against the schema of a case: it doesn't panic, and every diagnostic is a message at a
// place of the files read.
func FuzzLint(f *testing.F) {
	mem, cases := fuzzCases(f)
	for i, c := range cases {
		f.Add(uint8(i), string(mem[c.doc].Data))
	}
	f.Fuzz(func(t *testing.T, i uint8, text string) {
		c := cases[int(i)%len(cases)]
		fsys := overlay{mem, c.doc, text}
		_, r := LoadWith(c.schema, c.doc, Options{Today: time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC), FS: fsys})
		checkDiags(t, r, fsys)
	})
}

// FuzzLintSchema lints the board of cases/board against a schema: it doesn't panic, and every diagnostic is a
// message at a place of the files read.
func FuzzLintSchema(f *testing.F) {
	mem, _ := fuzzCases(f)
	for _, name := range slices.Sorted(maps.Keys(mem)) {
		if strings.HasSuffix(name, ".kanonarch") {
			f.Add(string(mem[name].Data))
		}
	}
	f.Fuzz(func(t *testing.T, text string) {
		fsys := overlay{mem, "board/schema.kanonarch", text}
		_, r := LoadWith("board/schema.kanonarch", "board/board.kan", Options{Today: time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC), FS: fsys})
		checkDiags(t, r, fsys)
	})
}

// checkDiags checks that each diagnostic of r has a message, written as CLAUDE.md says, at a line and a column of a
// document or a schema of fsys, or for a whole file. A message may start or end as the text of fsys.text does, since
// it can quote it: this comes before the title; move it under # Results.
func checkDiags(t *testing.T, r Result, fsys overlay) {
	t.Helper()
	for _, d := range r.Diags {
		p := d.Pos
		words := strings.Fields(d.Msg)
		if len(words) == 0 {
			t.Errorf("%s: no message", d)
			continue
		}
		first, last := words[0], words[len(words)-1]
		if r, _ := utf8.DecodeRuneInString(first); unicode.IsUpper(r) && !strings.Contains(fsys.text, first) ||
			strings.HasSuffix(last, ".") && !strings.Contains(fsys.text, last) {
			t.Errorf("%s: not a message", d)
		}
		src, err := fsys.ReadFile(p.File)
		switch ext := filepath.Ext(p.File); {
		case p.File == "":
			t.Errorf("%s: in no file", d)
		case p.Line == 0 && p.Col == 0:
		case err != nil:
			t.Errorf("%s: at a line of a file that can't be read", d)
		case ext == ".kan" || ext == ".kanonarch":
			lines := strings.Split(string(src), "\n")
			if p.Line < 1 || p.Line > len(lines) || p.Col < 1 || p.Col > utf8.RuneCountInString(lines[p.Line-1])+1 {
				t.Errorf("%s: outside the text", d)
			}
		}
	}
}
