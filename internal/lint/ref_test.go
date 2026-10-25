package lint

import (
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/bn-k/kanon/internal/diag"
)

func TestRefs(t *testing.T) {
	write(t, files{
		"schema.kanonarch": `enum State { new, worn }
schema Line reads "{name} {state}" { name: Text  state: State }
schema Client { name: Text }
schema Part { qty: Int }
schema Doc { title: Text @heading  client: -> Client  state: State  lines: [Line]  parts: [Part]  notes: Prose? }
rule brand-new(d: Doc) -> Bool = d.state == new`,
		"doc.kan":          "not read: the editor gives its text",
		"clients/acme.kan": "---\ntype: Client\n---\n# Acme\nname: Acme\n",
		"photo.png":        "\x89PNG\r\n\x1a\n",
	})
	open := map[string]string{"doc.kan": `---
type: Doc
---
# Quote
client: [[clients/acme]]
state: new

## Lines

- screw worn {#l1}

## Parts

| id | qty |
|----|-----|
| p1 | 2   |

## Notes

See [[#l1]], {state} and [photo](photo.png) and [[schema#brand-new]].
`}
	read := func(path string) ([]byte, error) {
		if src, ok := open[path]; ok {
			return []byte(src), nil
		}
		return os.ReadFile(path)
	}
	_, r := LoadWith("schema.kanonarch", "doc.kan", Options{Today: time.Now(), ReadFile: read})
	check(t, r, nil)

	// text returns the n runes at pos, or the name declared there when n is 0.
	text := func(pos diag.Pos, n int) string {
		src, err := read(pos.File)
		if err != nil {
			t.Fatal(err)
		}
		line := []rune(strings.Split(string(src), "\n")[pos.Line-1])[pos.Col-1:]
		if n == 0 {
			n = slices.IndexFunc(line, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' })
		}
		return string(line[:n])
	}
	var got []string
	schemaRefs := 0
	for _, ref := range r.Refs {
		if ref.Pos.File == "schema.kanonarch" {
			schemaRefs++
			continue
		}
		decl := ref.Decl.File
		if ref.Decl.Line > 0 {
			decl = fmt.Sprintf("%s:%d %q", decl, ref.Decl.Line, text(ref.Decl, 0))
		}
		got = append(got, fmt.Sprintf("%s:%d:%d %q -> %s", ref.Pos.File, ref.Pos.Line, ref.Pos.Col, text(ref.Pos, ref.Len), decl))
	}
	slices.Sort(got)
	want := []string{
		`clients/acme.kan:2:7 "Client" -> schema.kanonarch:3 "Client"`,
		`clients/acme.kan:5:1 "name" -> schema.kanonarch:3 "name"`,
		`doc.kan:10:16 "l1" -> doc.kan:10 "l1"`,
		`doc.kan:10:9 "worn" -> schema.kanonarch:1 "worn"`,
		`doc.kan:12:4 "Parts" -> schema.kanonarch:5 "parts"`,
		`doc.kan:14:8 "qty" -> schema.kanonarch:4 "qty"`,
		`doc.kan:16:3 "p1" -> doc.kan:16 "p1"`,
		`doc.kan:18:4 "Notes" -> schema.kanonarch:5 "notes"`,
		`doc.kan:20:15 "state" -> schema.kanonarch:5 "state"`,
		`doc.kan:20:26 "[photo](photo.png)" -> photo.png`,
		`doc.kan:20:49 "[[schema#brand-new]]" -> schema.kanonarch:6 "brand-new"`,
		`doc.kan:20:5 "[[#l1]]" -> doc.kan:10 "l1"`,
		`doc.kan:2:7 "Doc" -> schema.kanonarch:5 "Doc"`,
		`doc.kan:5:1 "client" -> schema.kanonarch:5 "client"`,
		`doc.kan:5:9 "[[clients/acme]]" -> clients/acme.kan`,
		`doc.kan:6:1 "state" -> schema.kanonarch:5 "state"`,
		`doc.kan:6:8 "new" -> schema.kanonarch:1 "new"`,
		`doc.kan:8:4 "Lines" -> schema.kanonarch:5 "lines"`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("refs differ\n--- got\n%s\n--- want\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if schemaRefs == 0 {
		t.Errorf("no refs in the schema")
	}
}

func TestRefsOfItemsOnSeveralLines(t *testing.T) {
	write(t, files{
		"schema.kanonarch": `enum State { new, worn }
schema Line reads "{name} {state}" { name: Text  state: State }
schema Doc { lines: [Line] }`,
		"doc.kan": "---\ntype: Doc\n---\n## Lines\n\n- screw\n  worn {#a}\n- big\n  screw new {#b}\n",
	})
	r := Run("schema.kanonarch", "doc.kan")
	check(t, r, nil)
	var got []string
	for _, ref := range r.Refs {
		if ref.Pos.File == "doc.kan" && ref.Decl.Line == 1 {
			got = append(got, fmt.Sprintf("%v %d", ref.Pos, ref.Len))
		}
	}
	// new is on the second line of its item; "big screw" is on none, so it is placed where the item starts.
	want := []string{"doc.kan:7:3 4", "doc.kan:9:9 3"}
	if !slices.Equal(got, want) {
		t.Errorf("refs to the enum = %q, want %q", got, want)
	}
}

func TestRefsOfValuesOnAKeyLine(t *testing.T) {
	write(t, files{
		"schema.kanonarch": "enum Norm { iso-9001, iso-14001 }\nschema Supplier { name: Text @heading  norms: [Norm] }",
		"doc.kan":          "---\ntype: Supplier\n---\n# Harbour Cables\n\nnorms: iso-9001,  iso-14001\n",
	})
	r := Run("schema.kanonarch", "doc.kan")
	check(t, r, nil)
	var got []string
	for _, ref := range r.Refs {
		if ref.Pos.File == "doc.kan" && ref.Decl.Line == 1 {
			got = append(got, fmt.Sprintf("%v %d %v", ref.Pos, ref.Len, ref.Decl))
		}
	}
	// Each value points to its own declaration, from its own column.
	want := []string{"doc.kan:6:8 8 schema.kanonarch:1:13", "doc.kan:6:19 9 schema.kanonarch:1:23"}
	if !slices.Equal(got, want) {
		t.Errorf("refs to the enum = %q, want %q", got, want)
	}
}

func TestRefsOfAnUnreadableSchemaFile(t *testing.T) {
	write(t, files{
		"schema.kanonarch": "schema Doc { notes: Prose? }",
		"other.kanonarch":  "rule r() = yes",
		"doc.kan":          "---\ntype: Doc\n---\n## Notes\n\n[[other#r]]\n",
	})
	read := func(path string) ([]byte, error) {
		if path == "other.kanonarch" {
			return nil, fs.ErrPermission
		}
		return os.ReadFile(path)
	}
	_, r := LoadWith("schema.kanonarch", "doc.kan", Options{ReadFile: read})
	check(t, r, []string{"other.kanonarch: error: cannot read the file: permission denied"})
	for _, ref := range r.Refs {
		if ref.Decl.File == "other.kanonarch" {
			t.Errorf("ref %v to a schema file that can't be read", ref)
		}
	}
}

func TestRefsOfAnotherSchemaFile(t *testing.T) {
	write(t, files{
		"schema.kanonarch": "schema Doc { notes: Prose? }",
		"other.kanonarch":  "rule r() = yes",
		"doc.kan":          "---\ntype: Doc\n---\n## Notes\n\n[[other]] and [[other#r]]\n",
	})
	r := Run("schema.kanonarch", "doc.kan")
	check(t, r, nil)
	var got []string
	for _, ref := range r.Refs {
		got = append(got, fmt.Sprintf("%v %d -> %v", ref.Pos, ref.Len, ref.Decl))
	}
	for _, want := range []string{"doc.kan:6:1 9 -> other.kanonarch", "doc.kan:6:15 11 -> other.kanonarch:1:6", "other.kanonarch:1:6 1 -> other.kanonarch:1:6"} {
		if !slices.Contains(got, want) {
			t.Errorf("no ref %s in %q", want, got)
		}
	}
}
