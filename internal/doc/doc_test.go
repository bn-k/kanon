package doc

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bn-k/kanon/internal/diag"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		want  string
		diags []string
	}{
		{
			name: "empty",
			src:  "",
			want: "root\n",
		},
		{
			name: "front matter",
			src:  "---\ntype: Facture\n\nnote:\n---\n",
			want: "front-matter\n" +
				"  key 2:1 type=\"Facture\" 2:7\n" +
				"  key 4:1 note=\"\" 4:6\n" +
				"root\n",
		},
		{
			name: "front matter with lines that are not keys",
			src:  "---\ntype Facture\n  x\nType: X\n---\n",
			want: "front-matter\nroot\n",
			diags: []string{
				`2:1 expected "key: value" in front matter, got "type Facture"`,
				`3:3 expected "key: value" in front matter, got "x"`,
				`4:1 expected "key: value" in front matter, got "Type: X"`,
			},
		},
		{
			name:  "unterminated front matter",
			src:   "---\ntype: X\n",
			want:  "root\n  key 2:1 type=\"X\" 2:7\n",
			diags: []string{`1:1 unterminated front matter: no closing "---"`},
		},
		{
			name: "front matter only on the first line",
			src:  "\n---\n",
			want: "root\n  paragraph\n    2:1 \"---\"\n",
		},
		{
			name: "carriage returns",
			src:  "---  \r\ntype: X\r\n---\r\n# T\r\n",
			want: "front-matter\n" +
				"  key 2:1 type=\"X\" 2:7\n" +
				"root\n" +
				"  h1 4:1 \"T\" 4:3\n",
		},
		{
			name: "headings nest by level",
			src:  "# A\n## B {#b}\n### C\n## D\n#### E\n# F\n",
			want: "root\n" +
				"  h1 1:1 \"A\" 1:3\n" +
				"    h2 2:1 \"B\" 2:4 #b 2:8\n" +
				"      h3 3:1 \"C\" 3:5\n" +
				"    h2 4:1 \"D\" 4:4\n" +
				"      h4 5:1 \"E\" 5:6\n" +
				"  h1 6:1 \"F\" 6:3\n",
		},
		{
			name: "accented heading with an id",
			src:  "## Émetteur {#atelier-lune}",
			want: "root\n  h2 1:1 \"Émetteur\" 1:4 #atelier-lune 1:15\n",
		},
		{
			name: "headings without text",
			src:  "#\n##\t\n### {#only}\n",
			want: "root\n" +
				"  h1 1:1 \"\" 1:2\n" +
				"    h2 2:1 \"\" 2:4\n" +
				"      h3 3:1 \"\" 3:5 #only 3:7\n",
		},
		{
			name: "heading text is trimmed and keeps inner braces",
			src:  "##\tTab {#a} title  \n### x {#a} {#b}\n",
			want: "root\n" +
				"  h2 1:1 \"Tab {#a} title\" 1:4\n" +
				"    h3 2:1 \"x {#a}\" 2:5 #b 2:14\n",
		},
		{
			name: "not headings",
			src:  "#hashtag\n####### seven\n  # indented\n",
			want: "root\n  paragraph\n" +
				"    1:1 \"#hashtag\"\n" +
				"    2:1 \"####### seven\"\n" +
				"    3:3 \"# indented\"\n",
		},
		{
			name:  "empty id",
			src:   "## T {#}",
			want:  "root\n  h2 1:1 \"T\" 1:4\n",
			diags: []string{`1:6 empty id "{#}"`},
		},
		{
			name: "keys at the start of a section",
			src:  "# T\n\nnom: A\nadresse:  8 quai  \n\nville: Lyon\nText after keys\nkey: not a key\n",
			want: "root\n" +
				"  h1 1:1 \"T\" 1:3\n" +
				"    key 3:1 nom=\"A\" 3:6\n" +
				"    key 4:1 adresse=\"8 quai\" 4:11\n" +
				"    key 6:1 ville=\"Lyon\" 6:8\n" +
				"    paragraph\n" +
				"      7:1 \"Text after keys\"\n" +
				"      8:1 \"key: not a key\"\n",
		},
		{
			name: "keys in root and in each section",
			src:  "a: 1\n# T\nb: 2\nprix_unitaire: 3\nétat: usé\nvide:   \n",
			want: "root\n" +
				"  key 1:1 a=\"1\" 1:4\n" +
				"  h1 2:1 \"T\" 2:3\n" +
				"    key 3:1 b=\"2\" 3:4\n" +
				"    key 4:1 prix_unitaire=\"3\" 4:16\n" +
				"    key 5:1 état=\"usé\" 5:7\n" +
				"    key 6:1 vide=\"\" 6:6\n",
		},
		{
			name: "lines that are not keys",
			src:  "https://x\nNom: x\na:b\n",
			want: "root\n  paragraph\n" +
				"    1:1 \"https://x\"\n" +
				"    2:1 \"Nom: x\"\n" +
				"    3:1 \"a:b\"\n",
		},
		{
			name: "a key after a block is text",
			src:  "# T\n- a\n\nk: v\n",
			want: "root\n" +
				"  h1 1:1 \"T\" 1:3\n" +
				"    bullets 2:1\n" +
				"      item 2:1\n" +
				"        2:3 \"a\"\n" +
				"    paragraph\n" +
				"      4:1 \"k: v\"\n",
		},
		{
			name: "paragraphs",
			src:  "one\n  two  \n\nthree\n# H\n",
			want: "root\n" +
				"  paragraph\n" +
				"    1:1 \"one\"\n" +
				"    2:3 \"two\"\n" +
				"  paragraph\n" +
				"    4:1 \"three\"\n" +
				"  h1 5:1 \"H\" 5:3\n",
		},
		{
			name: "paragraph ends at a list, a fence or a table",
			src:  "a\n- b\nc\n```\nx\n```\nd\n| e |\n|---|\n",
			want: "root\n" +
				"  paragraph\n" +
				"    1:1 \"a\"\n" +
				"  bullets 2:1\n" +
				"    item 2:1\n" +
				"      2:3 \"b\"\n" +
				"  paragraph\n" +
				"    3:1 \"c\"\n" +
				"  code 4:1 \"\"\n" +
				"    5:1 \"x\"\n" +
				"  paragraph\n" +
				"    7:1 \"d\"\n" +
				"  table 8:1\n" +
				"    header \"e\"@8:3\n",
		},
		{
			name: "non-breaking spaces stay",
			src:  "k: 1 250 EUR\n",
			want: "root\n  key 1:1 k=\"1\\u00a0250 EUR\" 1:4\n",
		},
		{
			name: "fenced code is raw",
			src:  "# T\n```go  \n# not a heading\nkey: v\n\n  | x |\n```\nafter\n",
			want: "root\n" +
				"  h1 1:1 \"T\" 1:3\n" +
				"    code 2:1 \"go\"\n" +
				"      3:1 \"# not a heading\"\n" +
				"      4:1 \"key: v\"\n" +
				"      5:1 \"\"\n" +
				"      6:1 \"  | x |\"\n" +
				"    paragraph\n" +
				"      8:1 \"after\"\n",
		},
		{
			name: "closing fence",
			src:  "~~~\n```\n~~\n~~~~  \n````\n```js\n````` \n",
			want: "root\n" +
				"  code 1:1 \"\"\n" +
				"    2:1 \"```\"\n" +
				"    3:1 \"~~\"\n" +
				"  code 5:1 \"\"\n" +
				"    6:1 \"```js\"\n",
		},
		{
			name:  "unterminated code",
			src:   "text\n```sh\nx\n",
			want:  "root\n  paragraph\n    1:1 \"text\"\n  code 2:1 \"sh\"\n    3:1 \"x\"\n",
			diags: []string{"2:1 unterminated code block: no closing \"```\""},
		},
		{
			name: "lists",
			src:  "- 250 g de farine {#farine}\n* 4 œufs\n  frais {#oeufs}\n\n+ sel\n\n1. one\n2) two\n\tmore\n- back\n10. ten\n",
			want: "root\n" +
				"  bullets 1:1\n" +
				"    item 1:1 #farine 1:21\n" +
				"      1:3 \"250 g de farine\"\n" +
				"    item 2:1 #oeufs 3:11\n" +
				"      2:3 \"4 œufs\"\n" +
				"      3:3 \"frais\"\n" +
				"    item 5:1\n" +
				"      5:3 \"sel\"\n" +
				"  ordered 7:1\n" +
				"    item 7:1\n" +
				"      7:4 \"one\"\n" +
				"    item 8:1\n" +
				"      8:4 \"two\"\n" +
				"      9:2 \"more\"\n" +
				"  bullets 10:1\n" +
				"    item 10:1\n" +
				"      10:3 \"back\"\n" +
				"  ordered 11:1\n" +
				"    item 11:1\n" +
				"      11:5 \"ten\"\n",
		},
		{
			name: "item edge cases",
			src:  "- a\n  {#x}\n-\ty   \n- \n-x\n",
			want: "root\n" +
				"  bullets 1:1\n" +
				"    item 1:1 #x 2:5\n" +
				"      1:3 \"a\"\n" +
				"    item 3:1\n" +
				"      3:3 \"y\"\n" +
				"    item 4:1\n" +
				"      4:3 \"\"\n" +
				"  paragraph\n" +
				"    5:1 \"-x\"\n",
		},
		{
			name: "single-line item with only an id",
			src:  "- {#x}\n",
			want: "root\n  bullets 1:1\n    item 1:1 #x 1:5\n      1:3 \"\"\n",
		},
		{
			name: "list ends at an indented line after a blank",
			src:  "- a\n\n  b\n",
			want: "root\n" +
				"  bullets 1:1\n" +
				"    item 1:1\n" +
				"      1:3 \"a\"\n" +
				"  paragraph\n" +
				"    3:3 \"b\"\n",
		},
		{
			name:  "empty item id",
			src:   "- a {#}\n",
			want:  "root\n  bullets 1:1\n    item 1:1\n      1:3 \"a\"\n",
			diags: []string{`1:5 empty id "{#}"`},
		},
		{
			name: "table",
			src:  "| id | é |\n|---|:-:|\n| a\\|b |  |\n  | é | x\n",
			want: "root\n" +
				"  table 1:1\n" +
				"    header \"id\"@1:3 \"é\"@1:8\n" +
				"    row 3:1 \"a|b\"@3:3 \"\"@3:9\n" +
				"    row 4:3 \"é\"@4:5 \"x\"@4:9\n",
		},
		{
			name: "table ends at a line without a pipe",
			src:  "| a |\n| --- |\n| 1 \\ 2 |\ntext\n",
			want: "root\n" +
				"  table 1:1\n" +
				"    header \"a\"@1:3\n" +
				"    row 3:1 \"1 \\\\ 2\"@3:3\n" +
				"  paragraph\n" +
				"    4:1 \"text\"\n",
		},
		{
			name:  "table without separator",
			src:   "| a |\n",
			want:  "root\n  table 1:1\n    header \"a\"@1:3\n",
			diags: []string{`1:1 expected a separator line such as "|---|" under the table header`},
		},
		{
			name:  "table with a bad separator",
			src:   "| a |\n| -x- |\n",
			want:  "root\n  table 1:1\n    header \"a\"@1:3\n    row 2:1 \"-x-\"@2:3\n",
			diags: []string{`2:1 expected a separator line such as "|---|" under the table header`},
		},
		{
			name:  "table with an empty separator",
			src:   "|\n|\n",
			want:  "root\n  table 1:1\n    header\n    row 2:1\n",
			diags: []string{`2:1 expected a separator line such as "|---|" under the table header`},
		},
		{
			name: "table rows of the wrong width",
			src:  "| a | b |\n|---|\n| 1 |\n| 1 | 2 | 3 |\n| 1 | 2 |\n",
			want: "root\n" +
				"  table 1:1\n" +
				"    header \"a\"@1:3 \"b\"@1:7\n" +
				"    row 3:1 \"1\"@3:3\n" +
				"    row 4:1 \"1\"@4:3 \"2\"@4:7 \"3\"@4:11\n" +
				"    row 5:1 \"1\"@5:3 \"2\"@5:7\n",
			diags: []string{
				"2:1 table separator has 1 cell, header has 2",
				"3:1 table row has 1 cell, header has 2",
				"4:1 table row has 3 cells, header has 2",
			},
		},
		{
			name: "all diagnostics are collected",
			src:  "---\nbad\n---\n| a |\n```\n",
			want: "front-matter\nroot\n  table 4:1\n    header \"a\"@4:3\n  code 5:1 \"\"\n",
			diags: []string{
				`2:1 expected "key: value" in front matter, got "bad"`,
				`4:1 expected a separator line such as "|---|" under the table header`,
				"5:1 unterminated code block: no closing \"```\"",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, diags := Parse("t.kan", tt.src)
			if got := dump(d); got != tt.want {
				t.Errorf("Parse(%q) =\n%s\nwant\n%s", tt.src, got, tt.want)
			}
			if got, want := dumpDiags(diags), strings.Join(tt.diags, "\n"); got != want {
				t.Errorf("Parse(%q) diagnostics =\n%s\nwant\n%s", tt.src, got, want)
			}
			for _, dg := range diags {
				if dg.Pos.File != "t.kan" || dg.Severity != diag.Error {
					t.Errorf("diagnostic %v: want an error in t.kan", dg)
				}
			}
		})
	}
}

func TestHelpers(t *testing.T) {
	src := "# T\n## U\npara one\ntwo\n```\n```\n- a\n  b\n| x |\n|---|\n"
	d, diags := Parse("t.kan", src)
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if d.Path != "t.kan" || d.Root.Level() != 0 {
		t.Errorf("Path = %q, Root.Level() = %d", d.Path, d.Root.Level())
	}
	u := d.Root.Children[0].Children[0]
	if u.Level() != 2 {
		t.Errorf("Level() = %d, want 2", u.Level())
	}
	at := []string{"3:1", "5:1", "7:1", "9:1"}
	if len(u.Blocks) != len(at) {
		t.Fatalf("got %d blocks, want %d", len(u.Blocks), len(at))
	}
	for i, b := range u.Blocks {
		if got := pos(b.At()); got != at[i] || b.At().File != "t.kan" {
			t.Errorf("block %d At() = %v, want t.kan:%s", i, b.At(), at[i])
		}
	}
	if got := u.Blocks[0].(*Paragraph).Text(); got != "para one\ntwo" {
		t.Errorf("Paragraph.Text() = %q", got)
	}
	if got := u.Blocks[2].(*List).Items[0].Text(); got != "a b" {
		t.Errorf("Item.Text() = %q", got)
	}
	d, _ = Parse("t.kan", "```sh\n  ls  \n\necho\n```\n")
	if got := d.Root.Blocks[0].(*Code).Text(); got != "  ls  \n\necho" {
		t.Errorf("Code.Text() = %q", got)
	}
}

func TestText(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"nothing", "## P\n", ""},
		{"paragraphs", "## P\n\n  one  \ntwo\n\n\n\nthree\n", "  one\ntwo\n\nthree"},
		{"key lines and a paragraph", "## P\nnote: a\n\nb: c\nd\n", "note: a\n\nb: c\nd"},
		{"lists", "## P\n- a {#x}\n  b\n1. c\n  {#y}\n\n- d\n", "- a {#x}\n  b\n1. c\n  {#y}\n\n- d"},
		{"tables", "## P\n| x |\n|---|\n| 1 |\n\n| y |\n|---|\ntext\n", "| x |\n|---|\n| 1 |\n\n| y |\n|---|\ntext"},
		{"code", "## P\n```sh\nls   \n\n\necho\n```\n\n```\nopen", "```sh\nls\n\n\necho\n```\n\n```\nopen"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, _ := Parse("t.kan", tt.src)
			p := d.Root.Children[0]
			if got := d.Text(p.Keys, p.Blocks); got != tt.want {
				t.Errorf("Text() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCases(t *testing.T) {
	docs := map[string]*Doc{}
	err := filepath.WalkDir("../../cases", func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || filepath.Ext(path) != ".kan" {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		d, diags := Parse(path, string(src))
		for _, dg := range diags {
			t.Errorf("%v", dg)
		}
		rel, _ := filepath.Rel("../../cases", path)
		docs[filepath.ToSlash(rel)] = d
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) < 12 {
		t.Fatalf("parsed %d documents, want at least 12", len(docs))
	}

	inv := docs["invoice/f-2026-031.kan"]
	if !inv.HasFrontMatter || len(inv.FrontMatter) != 1 || inv.FrontMatter[0].Key != "type" || inv.FrontMatter[0].Value != "Facture" {
		t.Errorf("invoice front matter = %v", inv.FrontMatter)
	}
	h1 := inv.Root.Children[0]
	if h1.Heading.Level != 1 || h1.Heading.Text != "Facture F-2026-031" || len(h1.Keys) != 6 {
		t.Errorf("invoice H1 = %+v with %d keys", h1.Heading, len(h1.Keys))
	}
	if k := h1.Keys[3]; k.Key != "client" || k.Value != "[[clients/dupont-batiment]]" {
		t.Errorf("invoice key 3 = %s: %s", k.Key, k.Value)
	}
	if got := headings(h1.Children); got != "Émetteur#atelier-lune Lignes Conditions" {
		t.Errorf("invoice sections = %s", got)
	}
	tbl := h1.Children[1].Blocks[0].(*Table)
	if got := texts(tbl.Header); got != "id designation quantite prix-unitaire" || len(tbl.Rows) != 2 {
		t.Errorf("invoice table header = %s, %d rows", got, len(tbl.Rows))
	}
	if c := tbl.Rows[1][3]; c.Text != "40.00 EUR" || pos(c.Pos) != "24:62" {
		t.Errorf("invoice cell = %q at %s", c.Text, pos(c.Pos))
	}
	if b := h1.Children[2].Blocks; len(b) != 1 || len(b[0].(*Paragraph).Lines) != 3 {
		t.Errorf("invoice conditions = %v", b)
	}

	rec := docs["recipe/crepes.kan"].Root.Children[0]
	ing := rec.Children[0].Blocks[0].(*List)
	var ids []string
	for _, it := range ing.Items {
		ids = append(ids, it.ID)
	}
	if ing.Ordered || strings.Join(ids, " ") != "farine oeufs lait beurre sel" {
		t.Errorf("recipe ingredients: ordered %v, ids %v", ing.Ordered, ids)
	}
	if steps := rec.Children[1].Blocks[0].(*List); !steps.Ordered || len(steps.Items) != 4 {
		t.Errorf("recipe steps: ordered %v, %d items", steps.Ordered, len(steps.Items))
	}

	atlas := docs["inventory/maison.kan"].Root.Children[0].Children[0]
	if atlas.Heading.ID != "atlas" || len(atlas.Keys) != 7 || headings(atlas.Children) != "Disques Notes" {
		t.Errorf("inventory atlas = %+v, %d keys, children %s", atlas.Heading, len(atlas.Keys), headings(atlas.Children))
	}
	disks := atlas.Children[0].Blocks[0].(*Table)
	if c := disks.Rows[1][2]; c.Text != "" || pos(c.Pos) != "22:40" {
		t.Errorf("inventory empty cell = %q at %s", c.Text, pos(c.Pos))
	}

	if e := docs["car/clio/entretiens/2025-06-10-vidange.kan"]; e.HasFrontMatter || len(e.Root.Children[0].Keys) != 5 {
		t.Errorf("car entry: front matter %v, keys %v", e.HasFrontMatter, e.Root.Children[0].Keys)
	}
}

func headings(ss []*Section) string {
	var parts []string
	for _, s := range ss {
		h := s.Heading.Text
		if s.Heading.ID != "" {
			h += "#" + s.Heading.ID
		}
		parts = append(parts, h)
	}
	return strings.Join(parts, " ")
}

func texts(cells []Cell) string {
	var parts []string
	for _, c := range cells {
		parts = append(parts, c.Text)
	}
	return strings.Join(parts, " ")
}

func pos(p diag.Pos) string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

func dumpDiags(l diag.List) string {
	var parts []string
	for _, d := range l {
		parts = append(parts, pos(d.Pos)+" "+d.Msg)
	}
	return strings.Join(parts, "\n")
}

// dump writes the shape of d, one construct per line, indented by depth, with every position as line:col.
func dump(d *Doc) string {
	var b strings.Builder
	if d.HasFrontMatter {
		b.WriteString("front-matter\n")
		for _, k := range d.FrontMatter {
			dumpKey(&b, 1, k)
		}
	}
	dumpSection(&b, 0, d.Root)
	return b.String()
}

func line(b *strings.Builder, depth int, format string, args ...any) {
	b.WriteString(strings.Repeat("  ", depth))
	fmt.Fprintf(b, format, args...)
	b.WriteByte('\n')
}

func dumpKey(b *strings.Builder, depth int, k *KeyLine) {
	line(b, depth, "key %s %s=%q %s", pos(k.Pos), k.Key, k.Value, pos(k.ValuePos))
}

func dumpLines(b *strings.Builder, depth int, lines []Line) {
	for _, l := range lines {
		line(b, depth, "%s %q", pos(l.Pos), l.Text)
	}
}

func dumpCells(cells []Cell) string {
	var s string
	for _, c := range cells {
		s += fmt.Sprintf(" %q@%s", c.Text, pos(c.Pos))
	}
	return s
}

func dumpSection(b *strings.Builder, depth int, s *Section) {
	if h := s.Heading; h == nil {
		line(b, depth, "root")
	} else {
		id := ""
		if h.ID != "" {
			id = fmt.Sprintf(" #%s %s", h.ID, pos(h.IDPos))
		}
		line(b, depth, "h%d %s %q %s%s", h.Level, pos(h.Pos), h.Text, pos(h.TextPos), id)
	}
	depth++
	for _, k := range s.Keys {
		dumpKey(b, depth, k)
	}
	for _, blk := range s.Blocks {
		switch blk := blk.(type) {
		case *Paragraph:
			line(b, depth, "paragraph")
			dumpLines(b, depth+1, blk.Lines)
		case *Code:
			line(b, depth, "code %s %q", pos(blk.Pos), blk.Info)
			dumpLines(b, depth+1, blk.Lines)
		case *List:
			kind := "bullets"
			if blk.Ordered {
				kind = "ordered"
			}
			line(b, depth, "%s %s", kind, pos(blk.Pos))
			for _, it := range blk.Items {
				id := ""
				if it.ID != "" {
					id = fmt.Sprintf(" #%s %s", it.ID, pos(it.IDPos))
				}
				line(b, depth+1, "item %s%s", pos(it.Pos), id)
				dumpLines(b, depth+2, it.Lines)
			}
		case *Table:
			line(b, depth, "table %s", pos(blk.Pos))
			line(b, depth+1, "header%s", dumpCells(blk.Header))
			for i, r := range blk.Rows {
				line(b, depth+1, "row %s%s", pos(blk.RowPos[i]), dumpCells(r))
			}
		}
	}
	for _, c := range s.Children {
		dumpSection(b, depth, c)
	}
}
