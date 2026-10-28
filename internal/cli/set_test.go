package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const shelfSchema = `// A shelf of books, for the tests of kan set.

enum Mood { calm, busy }

schema Shelf {
  title:   Text @heading
  owner:   Text?
  mood:    Mood?
  sizes:   [Int]
  tags:    [Text]
  photos:  [Image]
  snippet: Code?
  place:   Place?
  books:   [Book]
  loans:   [Loan]
}

schema Place {
  city:   Text
  street: Text?
}

schema Book {
  title:  Text? @heading
  author: Text
  year:   Int?
  about:  Prose? @body
}

schema Loan reads "{book} to {friend}[ until {until}]" {
  book:   Text
  friend: Text
  until:  Date?
}
`

// fm is the front matter of a shelf.
const fm = "---\ntype: Shelf\n---\n\n"

// shelfText is a shelf with no error.
const shelfText = fm + `# Reading corner

owner: Kim
mood: calm

## Sizes

- 12
- 30

## Tags

- old
- new

## Place {#home}

city: Riverside

## Books

| id        | title     | author      | year |
|-----------|-----------|-------------|------|
| tidewater | Tidewater | Ruth Okafor | 1998 |
| orchard   | Orchard   | Ines Varga  |      |

## Loans

- Tidewater to Kim {#first}
- Orchard to Lou until 2026-11-01 {#second}
`

// shelf writes the schema of a shelf, the folder of its photos and files, by path, into a new folder, and makes it
// the working directory.
func shelf(t *testing.T, files map[string]string) {
	t.Helper()
	t.Chdir(t.TempDir())
	files["schema.kanonarch"] = shelfSchema
	files["shelf/photos/door.png"] = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"
	for path, text := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// setText returns the text kan set makes of the document at path with sets, and what it refuses, a line each.
func setText(t *testing.T, path string, sets ...string) (string, string) {
	t.Helper()
	o, err := setArgs(append([]string{"-today", "2026-10-27", "schema.kanonarch", path}, sets...))
	if err != nil {
		t.Fatal(err)
	}
	text, diags := apply(o)
	var refused []string
	for _, d := range diags {
		refused = append(refused, d.String())
	}
	return text, strings.Join(refused, "\n")
}

// Each value is set by the least edit of the text: a key line is replaced, added where a person would write it, or
// removed with the blank line it would leave; a title or a heading changes its text alone.
func TestSetText(t *testing.T) {
	tests := []struct {
		name, text string
		sets       []string
		want       string
	}{
		{"an empty value", fm + "# Corner\n\nowner:\nmood: calm\n", []string{"owner=Ann"},
			fm + "# Corner\n\nowner: Ann\nmood: calm\n"},
		{"before the first key line", fm + "# Corner\n\nmood: calm\n", []string{"owner=Ann"},
			fm + "# Corner\n\nowner: Ann\nmood: calm\n"},
		{"under the title", fm + "# Corner\n\n## Tags\n\n- old\n", []string{"mood=busy"},
			fm + "# Corner\n\nmood: busy\n\n## Tags\n\n- old\n"},
		{"under a heading", fm + "# Corner\n\n## Place {#home}\n## Books\n", []string{"#home.city=Riverside"},
			fm + "# Corner\n\n## Place {#home}\n\ncity: Riverside\n\n## Books\n"},
		{"after the front matter", fm + "## Tags\n\n- old\n", []string{"owner=Ann"},
			fm + "owner: Ann\n\n## Tags\n\n- old\n"},
		{"a title", fm + "owner: Ann\n", []string{"title=Corner"}, fm + "# Corner\n\nowner: Ann\n"},
		{"a new title", fm + "# Corner\n", []string{"title=Den"}, fm + "# Den\n"},
		{"the title of a section", fm + "# Corner\n\n## Books\n\n### Tidewater {#tidewater}\n\nauthor: Ruth Okafor\n",
			[]string{"#tidewater.title="}, fm + "# Corner\n\n## Books\n\n### {#tidewater}\n\nauthor: Ruth Okafor\n"},
		{"a section with no title", fm + "# Corner\n\n## Books\n\n### {#tidewater}\n\nauthor: Ruth Okafor\n",
			[]string{"#tidewater.title=Tidewater"}, fm + "# Corner\n\n## Books\n\n### Tidewater {#tidewater}\n\nauthor: Ruth Okafor\n"},
		{"a key line and the blank line before", fm + "# Corner\n\nmood: calm\n\n## Tags\n\n- old\n", []string{"mood="},
			fm + "# Corner\n\n## Tags\n\n- old\n"},
		{"the last line", fm + "# Corner\n\nowner: Ann\nmood: calm", []string{"mood="}, fm + "# Corner\n\nowner: Ann"},
		{"the last line after a blank line", fm + "# Corner\n\nmood: calm\n", []string{"mood="}, fm + "# Corner\n"},
		{"no line ending at the end", fm + "# Corner\n\nmood: calm", []string{"sizes=12, 30"},
			fm + "# Corner\n\nmood: calm\nsizes: 12, 30"},
		{"CRLF", strings.ReplaceAll(fm+"# Corner\n\nmood: calm\n", "\n", "\r\n"), []string{"owner=Ann", "mood=busy"},
			strings.ReplaceAll(fm+"# Corner\n\nowner: Ann\nmood: busy\n", "\n", "\r\n")},
		{"nothing to remove", fm + "# Corner\n\n## Books\n\n| id | author |\n|---|---|\n| tidewater | Ruth Okafor |\n",
			[]string{"owner=", "#tidewater.year="}, fm + "# Corner\n\n## Books\n\n| id | author |\n|---|---|\n| tidewater | Ruth Okafor |\n"},
		{"a list item", shelfText, []string{"#first.friend=Ann", "#second.until=2026-12-01"},
			strings.Replace(strings.Replace(shelfText, "to Kim {", "to Ann {", 1), "2026-11-01", "2026-12-01", 1)},

		// A table not aligned by hand stays as it is, a cell at a time.
		{"a table not aligned", fm + "# Corner\n\n## Books\n\n| id | title | author | year |\n|---|---|---|---|\n" +
			"| tidewater | Tidewater | Ruth Okafor | |\n| orchard | Orchard | Ines Varga | 2004 |\n",
			[]string{"#tidewater.year=1998", "#orchard.year=", "#orchard.author=I. Varga"},
			fm + "# Corner\n\n## Books\n\n| id | title | author | year |\n|---|---|---|---|\n" +
				"| tidewater | Tidewater | Ruth Okafor | 1998 |\n| orchard | Orchard | I. Varga | |\n"},
		{"a table without closing pipes", fm + "# Corner\n\n## Books\n\n| id | title | author | year\n|---|---|---|---\n" +
			"| tidewater | Tidewater | Ruth Okafor | 1998\n| orchard | Orchard | Ines Varga | 2004\n",
			[]string{"#tidewater.year=1999", "#orchard.year="},
			fm + "# Corner\n\n## Books\n\n| id | title | author | year\n|---|---|---|---\n" +
				"| tidewater | Tidewater | Ruth Okafor | 1999\n| orchard | Orchard | Ines Varga | |\n"},

		// A table aligned by hand stays aligned: a pipe in a value is escaped, and the column widens on every line,
		// colons kept.
		{"an aligned table", fm + "# Corner\n\n## Books\n\n| id        | title     | author      |\n" +
			"|-----------|:---------:|-------------|\n| tidewater | Tidewater | Ruth Okafor |\n",
			[]string{"#tidewater.title=Tide | Water", "#tidewater.author=R. Okafor"},
			fm + "# Corner\n\n## Books\n\n| id        | title         | author      |\n" +
				"|-----------|:-------------:|-------------|\n| tidewater | Tide \\| Water | R. Okafor   |\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shelf(t, map[string]string{"shelf.kan": tt.text})
			got, refused := setText(t, "shelf.kan", tt.sets...)
			if refused != "" || got != tt.want {
				t.Errorf("set %q made\n%q\nwant\n%q\nrefused\n%s", tt.sets, got, tt.want, refused)
			}
		})
	}
}

// An entry of a folder has no front matter: its title goes first.
func TestSetEntry(t *testing.T) {
	const path = "shelf/books/tidewater.kan"
	tests := []struct {
		text string
		sets []string
		want string
	}{
		{"author: Ruth Okafor\n", []string{"title=Tidewater", "year=1998"}, "# Tidewater\n\nauthor: Ruth Okafor\nyear: 1998\n"},
		{"# Tidewater\n\nauthor: Ruth Okafor\n", []string{"title="}, "author: Ruth Okafor\n"},
		{"", []string{"author=Ruth Okafor", "title="}, "author: Ruth Okafor\n"},
	}
	for _, tt := range tests {
		shelf(t, map[string]string{"shelf.kan": fm + "# Corner\n", path: tt.text})
		got, refused := setText(t, path, tt.sets...)
		if refused != "" || got != tt.want {
			t.Errorf("set %q made\n%q\nwant\n%q\nrefused\n%s", tt.sets, got, tt.want, refused)
		}
	}
}

// What kan set can't change in place, it refuses, and it changes nothing.
func TestSetRefused(t *testing.T) {
	tests := []struct {
		name, text string
		sets       []string
		want       string
	}{
		{"fields", shelfText, []string{"photos=door.png", "snippet=x", "tags=new", "place=Riverside", "title="},
			"shelf.kan: error: `snippet` is code, written under its heading; change it with kan write\n" +
				"shelf.kan:5:3: error: `Shelf` needs `title`, so it can't be removed\n" +
				"shelf.kan:15:4: error: `tags` is a list, written under its heading; change it with kan write\n" +
				"shelf.kan:20:4: error: `place` holds a `Place`, not a value; set its fields one at a time, or change it with kan write\n" +
				"shelf/photos: error: `photos` is the folder shelf/photos; change its files"},
		{"unwritten", fm + "## Place {#home}\n", []string{"title=", "tags=new", "#home.city=", "#homes.city=Riverside", "#nowhere.city=Riverside"},
			"shelf.kan: error: `Shelf` needs `title`, so it can't be removed\n" +
				"shelf.kan: error: `tags` is a list, written under its heading; change it with kan write\n" +
				"shelf.kan: error: unknown id `homes`; did you mean `home`?\n" +
				"shelf.kan: error: unknown id `nowhere`\n" +
				"shelf.kan:5:1: error: `Place` needs `city`, so it can't be removed"},
		{"cells", fm + "# Corner\n\n## Books\n\n| id | title | author |\n|---|---|---|\n| tidewater | Tidewater | Ruth Okafor |\n" +
			"| orchard | Orchard |\n| salt-road |\n",
			[]string{"#tidewater.author=", "#tidewater.year=1998", "#orchard.author=Ines Varga", "#salt-road.author="},
			"shelf.kan:11:1: error: the table has no column `year`; add it with kan write\n" +
				"shelf.kan:11:27: error: `Book` needs `author`, so it can't be removed\n" +
				"shelf.kan:12:1: error: this row has no cell for `author`; fix it with kan write\n" +
				"shelf.kan:13:1: error: `Book` needs `author`, so it can't be removed"},
		{"body", fm + "# Corner\n\n## Books\n\n### Tidewater {#tidewater}\n\nauthor: Ruth Okafor\n\nA slow novel about a harbour.\n",
			[]string{"#tidewater.about=A short one."},
			"shelf.kan:13:1: error: `about` is prose, written as text under the heading; change it with kan write"},
		{"items", shelfText, []string{"#first.until=2026-12-01", "#first.friend=", "#second.book=Orchard of glass to me"},
			"shelf.kan:33:1: error: `until` is read from the item; change the item with kan write\n" +
				"shelf.kan:33:1: error: `friend` is read from the item; change the item with kan write\n" +
				"shelf.kan:34:1: error: the item would read differently; change it with kan write"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shelf(t, map[string]string{"shelf.kan": tt.text})
			got, refused := setText(t, "shelf.kan", tt.sets...)
			if refused != tt.want || got != tt.text {
				t.Errorf("set %q refused\n%s\nwant\n%s\nand made\n%q", tt.sets, refused, tt.want, got)
			}
		})
	}
}

// An item that no longer reads is not refused: lint says why.
func TestSetItemUnread(t *testing.T) {
	t.Chdir("../../cases/recipe")
	text, refused := setText(t, "crepes.kan", "#flour.quantity=lots")
	if refused != "" || !strings.Contains(text, "\n- lots of flour {#flour}\n") {
		t.Errorf("set made\n%s\nrefused\n%s", text, refused)
	}
}

// A row of a spreadsheet is changed in the spreadsheet.
func TestSetSheet(t *testing.T) {
	shelf(t, map[string]string{
		"shelf.kan": fm + "# Corner\n\nbooks: [[books.csv]]\n",
		"books.csv": "id,title,author,year\nsalt-road,Salt Road,Ines Varga,2011\n",
	})
	if _, refused := setText(t, "shelf.kan", "#salt-road.year=2012"); refused != "shelf.kan: error: `#salt-road` is a row of books.csv; change it there" {
		t.Errorf("set refused %q", refused)
	}
}

// kan set writes a document only when its text changes, and checks it as kan write does.
func TestSet(t *testing.T) {
	shelf(t, map[string]string{"shelf.kan": shelfText})
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"set", "schema.kanonarch", "shelf.kan", "mood=busy"}, &stdout, &stderr); code != 0 ||
		stdout.String() != "wrote shelf.kan\n" || stderr.Len() > 0 {
		t.Fatalf("set = %d\n%s%s", code, stdout.String(), stderr.String())
	}
	if want := strings.Replace(shelfText, "mood: calm", "mood: busy", 1); read(t, "shelf.kan") != want {
		t.Errorf("shelf.kan holds\n%s", read(t, "shelf.kan"))
	}

	// The same value writes nothing: the folder is locked, and that is no error.
	os.Chmod(".", 0o555)
	t.Cleanup(func() { os.Chmod(".", 0o755) })
	stdout.Reset()
	if code := Run([]string{"set", "schema.kanonarch", "shelf.kan", "mood=busy"}, &stdout, &stderr); code != 0 ||
		stdout.String() != "ok  shelf.kan\n" || stderr.Len() > 0 {
		t.Errorf("set of the same value = %d\n%s%s", code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	if code := Run([]string{"set", "schema.kanonarch", "shelf.kan", "mood=calm"}, &stdout, &stderr); code != 1 ||
		!strings.HasPrefix(stderr.String(), "kan: cannot write shelf.kan: ") {
		t.Errorf("set in a locked folder = %d\n%s%s", code, stdout.String(), stderr.String())
	}

	stderr.Reset()
	if code := Run([]string{"set", "-n", "-format", "json", "schema.kanonarch", "shelf.kan", "mood=calm"}, failingWriter{}, &stderr); code != 1 ||
		stderr.String() != "kan: broken pipe\n" {
		t.Errorf("set to a broken pipe = %d\n%s", code, stderr.String())
	}
}

// What lint can't read, kan set doesn't change: a schema with an error, a document with no type, or one in a hidden
// folder, which no list holds.
func TestSetUnread(t *testing.T) {
	tests := []struct {
		files map[string]string
		path  string
		want  string
	}{
		{map[string]string{"shelf.kan": shelfText, "notes.kan": "owner: Ann\n"}, "notes.kan",
			"notes.kan:1:1: error: this document names no schema"},
		{map[string]string{"shelf.kan": shelfText, "shelf/.drafts/notes.kan": "owner: Ann\n"}, "shelf/.drafts/notes.kan",
			"shelf/.drafts/notes.kan: error: this document has no type and is in no folder of a list: give it its type in its front matter\n"},
	}
	for _, tt := range tests {
		shelf(t, tt.files)
		var out bytes.Buffer
		if code := Run([]string{"set", "schema.kanonarch", tt.path, "owner=Kim"}, &out, &out); code != 1 ||
			!strings.HasPrefix(out.String(), tt.want) {
			t.Errorf("set %s = %d\n%s", tt.path, code, out.String())
		}
	}

	shelf(t, map[string]string{"shelf.kan": shelfText})
	os.WriteFile("schema.kanonarch", []byte("schema Shelf {\n  title: Txet\n}\n"), 0o644)
	var out bytes.Buffer
	if code := Run([]string{"set", "schema.kanonarch", "shelf.kan", "title=Den"}, &out, &out); code != 1 ||
		!strings.Contains(out.String(), "schema.kanonarch:2:10: error: ") {
		t.Errorf("set with a wrong schema = %d\n%s", code, out.String())
	}
}

func TestSetUsage(t *testing.T) {
	shelf(t, map[string]string{"shelf.kan": shelfText})
	os.Mkdir("folder.kan", 0o755)
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"schema.kanonarch", "shelf.kan"}, "set takes the fields to set after the document, as in status=done"},
		{[]string{"schema.kanonarch", "mood=busy"}, "set takes a schema and a document, got 1 argument"},
		{[]string{"schema.kanonarch", "nowhere.kan", "mood=busy"}, "nowhere.kan does not exist"},
		{[]string{"schema.kanonarch", "folder.kan", "mood=busy"}, "cannot read folder.kan: is a directory"},
		{[]string{"schema.kanonarch", "shelf.kan", "mood=busy", "mood=calm"}, "mood is set twice"},
		{[]string{"schema.kanonarch", "shelf.kan", "#home.city=a", "#home.city=b"}, "#home.city is set twice"},
		{[]string{"schema.kanonarch", "shelf.kan", "=busy"}, `write the field to set as status or #id.status, not ""`},
		{[]string{"schema.kanonarch", "shelf.kan", "#home=busy"}, `write the field to set as status or #id.status, not "#home"`},
		{[]string{"schema.kanonarch", "shelf.kan", "#.city=x"}, `write the field to set as status or #id.status, not "#.city"`},
		{[]string{"schema.kanonarch", "shelf.kan", "home.city=x"}, `write the field to set as status or #id.status, not "home.city"`},
		{[]string{"schema.kanonarch", "shelf.kan", "owner=Ann\nKim"}, "the value of owner holds a line break: change text over lines with kan write"},
		{[]string{"-format", "csv", "schema.kanonarch", "shelf.kan", "mood=busy"}, "-format is text or json, not csv"},
		{[]string{"-today", "x", "schema.kanonarch", "shelf.kan", "mood=busy"}, `write -today 2026-10-22, not "x"`},
	}
	for _, tt := range tests {
		var stdout, stderr bytes.Buffer
		if code := Run(append([]string{"set"}, tt.args...), &stdout, &stderr); code != 2 {
			t.Errorf("set %q = %d, want 2", tt.args, code)
		}
		if want := "kan: " + tt.want + "\n" + usage + "\n"; stderr.String() != want || stdout.Len() > 0 {
			t.Errorf("set %q printed %q, want %q", tt.args, stderr.String(), want)
		}
	}
}
