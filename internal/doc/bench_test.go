package doc

import (
	"fmt"
	"strings"
	"testing"
)

// BenchmarkParse parses a document of 1 000 sections, about 30 000 lines: key lines, prose with links, a list,
// a table and a code block in each.
func BenchmarkParse(b *testing.B) {
	var w strings.Builder
	w.WriteString("---\ntype: Book\n---\n\n# Book\n\n")
	for i := range 1000 {
		fmt.Fprintf(&w, "## Chapter %d {#chapter-%d}\n\nauthor: [[/people#p-%d]]\nwritten: 2026-10-22\npages: %d\n\n", i, i, i%40, i%300)
		w.WriteString("A paragraph of prose that links to [[/places#harbour]] and inserts {author},\n" +
			"written over two lines.\n\n" +
			"- the first item\n- the second item {#second}\n  goes on here\n- the third item\n\n" +
			"| id | name | quantity |\n|---|---|---|\n| a | Alpha | 12 |\n| b | Beta \\| Gamma | 7 |\n| c | Delta | 30 |\n\n" +
			"```sh\necho one\necho two\n```\n\n")
	}
	src := w.String()
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		if _, ds := Parse("book.kan", src); len(ds) > 0 {
			b.Fatal(ds)
		}
	}
}
