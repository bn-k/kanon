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
		"schema.kanonarch": `enum Etat { neuf, use }
schema Ligne reads "{nom} {etat}" { nom: Text  etat: Etat }
schema Client { nom: Text }
schema Part { qte: Int }
schema Doc { titre: Text @heading  client: -> Client  etat: Etat  lignes: [Ligne]  parts: [Part]  notes: Prose? }
rule neuve(d: Doc) -> Bool = d.etat == neuf`,
		"doc.kan":            "not read: the editor gives its text",
		"clients/dupont.kan": "---\ntype: Client\n---\n# Dupont\nnom: Dupont\n",
		"photo.png":          "\x89PNG\r\n\x1a\n",
	})
	open := map[string]string{"doc.kan": `---
type: Doc
---
# Devis
client: [[clients/dupont]]
etat: neuf

## Lignes

- vis use {#l1}

## Parts

| id | qte |
|----|-----|
| p1 | 2   |

## Notes

Voir [[#l1]], {etat} et [photo](photo.png) et [[schema#neuve]].
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
		`clients/dupont.kan:2:7 "Client" -> schema.kanonarch:3 "Client"`,
		`clients/dupont.kan:5:1 "nom" -> schema.kanonarch:3 "nom"`,
		`doc.kan:10:13 "l1" -> doc.kan:10 "l1"`,
		`doc.kan:10:7 "use" -> schema.kanonarch:1 "use"`,
		`doc.kan:12:4 "Parts" -> schema.kanonarch:5 "parts"`,
		`doc.kan:14:8 "qte" -> schema.kanonarch:4 "qte"`,
		`doc.kan:16:3 "p1" -> doc.kan:16 "p1"`,
		`doc.kan:18:4 "Notes" -> schema.kanonarch:5 "notes"`,
		`doc.kan:20:16 "etat" -> schema.kanonarch:5 "etat"`,
		`doc.kan:20:25 "[photo](photo.png)" -> photo.png`,
		`doc.kan:20:47 "[[schema#neuve]]" -> schema.kanonarch:6 "neuve"`,
		`doc.kan:20:6 "[[#l1]]" -> doc.kan:10 "l1"`,
		`doc.kan:2:7 "Doc" -> schema.kanonarch:5 "Doc"`,
		`doc.kan:5:1 "client" -> schema.kanonarch:5 "client"`,
		`doc.kan:5:9 "[[clients/dupont]]" -> clients/dupont.kan`,
		`doc.kan:6:1 "etat" -> schema.kanonarch:5 "etat"`,
		`doc.kan:6:7 "neuf" -> schema.kanonarch:1 "neuf"`,
		`doc.kan:8:4 "Lignes" -> schema.kanonarch:5 "lignes"`,
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
		"schema.kanonarch": `enum Etat { neuf, use }
schema Ligne reads "{nom} {etat}" { nom: Text  etat: Etat }
schema Doc { lignes: [Ligne] }`,
		"doc.kan": "---\ntype: Doc\n---\n## Lignes\n\n- vis\n  use {#a}\n- grosse\n  vis neuf {#b}\n",
	})
	r := Run("schema.kanonarch", "doc.kan")
	check(t, r, nil)
	var got []string
	for _, ref := range r.Refs {
		if ref.Pos.File == "doc.kan" && ref.Decl.Line == 1 {
			got = append(got, fmt.Sprintf("%v %d", ref.Pos, ref.Len))
		}
	}
	// neuf is on the second line of its item; "grosse vis" is on none, so it is placed where the item starts.
	want := []string{"doc.kan:7:3 3", "doc.kan:9:7 4"}
	if !slices.Equal(got, want) {
		t.Errorf("refs to the enum = %q, want %q", got, want)
	}
}

func TestRefsOfAnUnreadableSchemaFile(t *testing.T) {
	write(t, files{
		"schema.kanonarch": "schema Doc { notes: Prose? }",
		"autre.kanonarch":  "rule r() = yes",
		"doc.kan":          "---\ntype: Doc\n---\n## Notes\n\n[[autre#r]]\n",
	})
	read := func(path string) ([]byte, error) {
		if path == "autre.kanonarch" {
			return nil, fs.ErrPermission
		}
		return os.ReadFile(path)
	}
	_, r := LoadWith("schema.kanonarch", "doc.kan", Options{ReadFile: read})
	check(t, r, []string{"autre.kanonarch: error: cannot read the file: permission denied"})
	for _, ref := range r.Refs {
		if ref.Decl.File == "autre.kanonarch" {
			t.Errorf("ref %v to a schema file that can't be read", ref)
		}
	}
}

func TestRefsOfAnotherSchemaFile(t *testing.T) {
	write(t, files{
		"schema.kanonarch": "schema Doc { notes: Prose? }",
		"autre.kanonarch":  "rule r() = yes",
		"doc.kan":          "---\ntype: Doc\n---\n## Notes\n\n[[autre]] et [[autre#r]]\n",
	})
	r := Run("schema.kanonarch", "doc.kan")
	check(t, r, nil)
	var got []string
	for _, ref := range r.Refs {
		got = append(got, fmt.Sprintf("%v %d -> %v", ref.Pos, ref.Len, ref.Decl))
	}
	for _, want := range []string{"doc.kan:6:1 9 -> autre.kanonarch", "doc.kan:6:14 11 -> autre.kanonarch:1:6", "autre.kanonarch:1:6 1 -> autre.kanonarch:1:6"} {
		if !slices.Contains(got, want) {
			t.Errorf("no ref %s in %q", want, got)
		}
	}
}
