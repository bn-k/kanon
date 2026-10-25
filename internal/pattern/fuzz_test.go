package pattern

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/bn-k/kanon/internal/names"
	"github.com/bn-k/kanon/internal/testcases"
)

// FuzzPattern parses a pattern and matches a line with it. An error is a message at a column of the pattern; a
// pattern has distinct fields; a line matches whether its fields are lazy or greedy, and each value is a part of the
// line, neither empty nor framed by spaces.
func FuzzPattern(f *testing.F) {
	for _, seed := range [][2]string{
		{"{quantity} [of] {name}", "200 g of flour"},
		{"{a} x {b}", "1 x 2 x 3"},
		{"Total: {amount} EUR", "Total: 1 250.50 EUR"},
		{"just text", "just  text"},
		{"{n} item[s] left", "1 item left"},
		{"{name} [({note})]", "eggs (free range)"},
		{"{a} [x {b} y] z", "1 x 2 y z"},
		{"{n} [[very] {adj}] {name}", "2 very ripe pears"},
		{"[[x] y] {b}", "x y 1"},
		{`\{{a}\} \[x\] \\`, `{1} [x] \`},
		{"{a}{b}", "ab"},
		{"[{a}] [{b}]", "a"},
		{"{a} [b", "a b"},
	} {
		f.Add(seed[0], seed[1])
	}
	addReads(f, "../../cases")
	f.Fuzz(func(t *testing.T, text, line string) {
		p, err := Parse(text)
		if err != nil {
			pe := (*Error)(nil)
			if !errors.As(err, &pe) || pe.Col < 1 || pe.Col > utf8.RuneCountInString(text)+1 {
				t.Fatalf("Parse(%q): %#v is not an error at a column of the pattern", text, err)
			}
			if r, _ := utf8.DecodeRuneInString(pe.Msg); pe.Msg == "" || unicode.IsUpper(r) || strings.HasSuffix(pe.Msg, ".") {
				t.Fatalf("Parse(%q): %q is not a message", text, pe.Msg)
			}
			return
		}
		fields := p.Fields()
		for i, name := range fields {
			if !names.IsName(name) || slices.Contains(fields[:i], name) {
				t.Fatalf("Parse(%q): fields %q", text, fields)
			}
		}
		lazy, lazyOK := p.Match(line, nil, nil)
		greedy, greedyOK := p.Match(line, nil, func(string) bool { return true })
		if lazyOK != greedyOK {
			t.Fatalf("%q matches %q when lazy: %v, when greedy: %v", text, line, lazyOK, greedyOK)
		}
		for _, values := range []map[string]string{lazy, greedy} {
			for name, v := range values {
				r, _ := utf8.DecodeRuneInString(v)
				last, _ := utf8.DecodeLastRuneInString(v)
				if !slices.Contains(fields, name) || v == "" || unicode.IsSpace(r) || unicode.IsSpace(last) ||
					!strings.Contains(line, v) || utf8.ValidString(line) && !utf8.ValidString(v) {
					t.Fatalf("%q matches %q with %s = %q", text, line, name, v)
				}
			}
		}
	})
}

// readsRe finds the text of a reads clause of a schema.
var readsRe = regexp.MustCompile(`reads(?: [a-z]+)? "((?:[^"\\\n]|\\.)*)"`)

// addReads adds the reads patterns of the schemas under root to the seed corpus, each with the list items of the
// documents of its folder.
func addReads(f *testing.F, root string) {
	err := testcases.Walk(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".kanonarch" {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		items := []string{""}
		docs, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.kan"))
		for _, doc := range docs {
			src, err := os.ReadFile(doc)
			if err != nil {
				return err
			}
			for line := range strings.Lines(string(src)) {
				if item, ok := strings.CutPrefix(line, "- "); ok {
					items = append(items, strings.TrimSpace(item))
				}
			}
		}
		unquote := strings.NewReplacer(`\"`, `"`, `\\`, `\`)
		for _, m := range readsRe.FindAllStringSubmatch(string(src), -1) {
			for _, item := range items {
				f.Add(unquote.Replace(m[1]), item)
			}
		}
		return nil
	})
	if err != nil {
		f.Fatal(err)
	}
}
