package sqlimport

import (
	"database/sql"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bn-k/kanon/internal/doc"
	"github.com/bn-k/kanon/internal/lint"
)

// TestLibrary imports the fixture testdata/library.sql and compares the result with testdata/library/, written by
// hand.
func TestLibrary(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "library")
	res, err := Import("testdata/library.sql", dir)
	if err != nil {
		t.Fatal(err)
	}
	compareTrees(t, "testdata/library", dir)
	want := []string{
		"`Books`: 2 keys change to make ids, such as \"DUNE\" → \"dune\"",
		"`Books.Summary`: 1 empty text is written as no value",
		"`Members.Name`: the spaces around 1 value are dropped",
		"the view `Overdue` is not imported",
	}
	if !slices.Equal(res.Notes, want) {
		t.Errorf("notes:\n%s\nwant:\n%s", strings.Join(res.Notes, "\n"), strings.Join(want, "\n"))
	}
	if res.Name != "library" || res.Schemas != 6 || res.Documents != 12 || res.Files != 3 {
		t.Errorf("result %+v, want library, 6 schemas, 12 documents, 3 files", res)
	}
	lintClean(t, dir)
}

// TestNorthwind imports the source of cases/northwind and compares the result with the case, so that the case
// stays what kan import writes.
func TestNorthwind(t *testing.T) {
	src := "../../cases/northwind/source/create.sql"
	if _, err := os.Stat(src); err != nil {
		t.Skip("no cases/northwind yet")
	}
	dir := filepath.Join(t.TempDir(), "northwind")
	if _, err := Import(src, dir); err != nil {
		t.Fatal(err)
	}
	compareTrees(t, "../../cases/northwind", dir, "source", "expected.txt")
	lintClean(t, dir)
}

// lintClean lints the result of an import in dir, which must hold no mistake.
func lintClean(t *testing.T, dir string) {
	t.Helper()
	r := lint.Run(filepath.Join(dir, "schema.kanonarch"), filepath.Join(dir, filepath.Base(dir)+".kan"))
	for _, d := range r.Diags {
		t.Error(d)
	}
}

// TestSQLite reads a database file, opened read-only.
func TestSQLite(t *testing.T) {
	tmp := t.TempDir()
	db := filepath.Join(tmp, "shop data.sqlite")
	run(t, db, `CREATE TABLE Items (ItemID INTEGER PRIMARY KEY, Name TEXT NOT NULL);
		INSERT INTO Items VALUES (1, 'Pen'), (2, 'Ink');`)
	before, _ := os.ReadFile(db)
	dir := filepath.Join(tmp, "shop")
	res, err := Import(db, dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Documents != 3 || res.Files != 0 || len(res.Notes) != 0 {
		t.Errorf("result %+v", res)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "shop/items/2.kan"))
	if string(got) != "# Item 2\n\nname: Ink\n" {
		t.Errorf("items/2.kan:\n%s", got)
	}
	after, _ := os.ReadFile(db)
	if string(before) != string(after) {
		t.Error("the database changed")
	}
}

// TestSchemas imports small databases and checks the schema written, for what the fixture doesn't show.
func TestSchemas(t *testing.T) {
	tests := []struct {
		name, sql, schema string
		notes             []string
		docs              map[string]string
	}{
		{
			name: "types",
			sql: `CREATE TABLE T (ID INTEGER PRIMARY KEY, Count INT, Ratio REAL, Big DECIMAL(10,2), Flag BOOL,
				Untyped, Mixed INTEGER, NoValue DATE, NoTime DATETIME, NoBlob BLOB, NoBool BIT, NoText VARCHAR(4),
				NoReal DOUBLE, NoInt BIGINT, NoAny, Stamp TIMESTAMP, Zoned DATETIME, Fraction DATETIME,
				NotDate DATE, Two BOOLEAN, Blob BLOB, Kind TEXT);
				INSERT INTO T (ID, Count, Ratio, Big, Flag, Untyped, Mixed, Stamp, Zoned, Fraction, NotDate, Two,
					Blob, Kind)
				VALUES (1, 1200, 0.5, 10, 1, 'a', 1, '2026-10-22T14:30:05', '2026-10-22 14:30Z',
					'2026-10-22 14:30:00.500', 'soon', 2, x'00ff', 'x'),
				(2, NULL, 2, 12.25, 0, 3, 'two', '2026-10-22 00:00:00+02:00', '2026-10-22T09:00:00-05:30',
					NULL, '2026-10-22', 0, x'0001', 'y');`,
			schema: `schema T {
  count:    Int?
  ratio:    Number?
  big:      Number?
  flag:     Bool?
  untyped:  Text?
  mixed:    Text?
  no-value: Date?
  no-time:  DateTime?
  no-blob:  File?
  no-bool:  Bool?
  no-text:  Text?
  no-real:  Number?
  no-int:   Int?
  no-any:   Text?
  stamp:    DateTime?
  zoned:    DateTime?
  fraction: Text?
  not-date: Text?
  two:      Int?
  blob:     File?
  kind:     Text?
}`,
			notes: []string{
				"`T.Mixed` is declared `INTEGER` but holds other values, so it is imported as `Text`",
				"`T.Fraction` holds fractions of a second, so it is imported as `Text`",
				"`T.NotDate` is declared `DATE` but holds other values, so it is imported as `Text`",
				"`T.Two` is declared `BOOLEAN` but holds other values, so it is imported as `Int`",
			},
			docs: map[string]string{
				"test/ts/1.kan": `# T 1

count: 1 200
ratio: 0.5
big: 10
flag: yes
untyped: a
mixed: 1
stamp: 2026-10-22 14:30:05
zoned: 2026-10-22 14:30 UTC
fraction: 2026-10-22 14:30:00.500
not-date: soon
two: 2
blob: [[1/blob.bin]]
kind: x
`,
				"test/ts/2.kan": `# T 2

ratio: 2
big: 12.25
flag: no
untyped: 3
mixed: two
stamp: 2026-10-22 00:00 +02:00
zoned: 2026-10-22 09:00 -05:30
not-date: 2026-10-22
two: 0
blob: [[2/blob.bin]]
kind: y
`,
			},
		},
		{
			name: "prose layout",
			sql: `CREATE TABLE T (ID INTEGER PRIMARY KEY, Body TEXT, Clean TEXT);
				INSERT INTO T VALUES (1, 'one  ' || char(13, 10) || 'two', 'a' || char(10) || 'b'),
					(2, char(10, 10) || 'para' || char(10, 10, 10, 10) || 'next ', 'c' || char(10) || 'd'),
					(3, 'x' || char(13, 10) || 'y', 'e' || char(10, 10) || 'f');`,
			schema: `schema T {
  body:  Prose?
  clean: Prose?
}`,
			notes: []string{
				"`T.Body`: spaces at the ends of lines and extra blank lines are dropped in 2 values",
				"`T.Body`: 2 values have Windows line breaks (\\r\\n), written as \\n",
			},
			docs: map[string]string{
				"test/ts/2.kan": `# T 2

## Body

para

next

## Clean

c
d
`,
			},
		},
		{
			name: "numbers and values",
			sql: `CREATE TABLE T (ID INTEGER PRIMARY KEY, Value INTEGER NOT NULL, Price NUMERIC NOT NULL,
					Data NOT NULL);
				INSERT INTO T VALUES (1, 2, 3.5, 4), (2, 3, 4, 5.5);`,
			schema: `schema T {
  value: Int
  price: Number
  data:  Number
}`,
		},
		{
			name: "one to one and inverse names",
			sql: `CREATE TABLE Persons (PersonID INTEGER PRIMARY KEY);
				CREATE TABLE Passports (PersonID INTEGER PRIMARY KEY REFERENCES Persons);
				CREATE TABLE Cards (CardID INTEGER PRIMARY KEY, Owner INTEGER UNIQUE REFERENCES Persons (PersonID),
					Holder INTEGER REFERENCES Persons, Cards TEXT);
				CREATE TABLE Empty (ID INTEGER PRIMARY KEY);`,
			schema: `schema Card {
  owner:  -> Person?
  holder: -> Person?
  cards:  Text?
}

schema Empty {
}

schema Passport {
  person: -> Person
}

schema Person {
  card:     Card? <- owner
  cards:    [Card] <- holder
  passport: Passport? <- person
}`,
		},
		{
			name: "join tables and weak tables",
			sql: `CREATE TABLE Tags (Tag TEXT PRIMARY KEY);
				CREATE TABLE Posts (PostID INTEGER PRIMARY KEY, Body TEXT);
				CREATE TABLE PostTags (PostID INTEGER REFERENCES Posts, Tag TEXT REFERENCES Tags,
					PRIMARY KEY (Tag, PostID)) WITHOUT ROWID;
				CREATE TABLE Friends (A INTEGER REFERENCES Posts, B INTEGER REFERENCES Posts, PRIMARY KEY (A, B));
				CREATE TABLE Revisions (PostID INTEGER REFERENCES Posts, No INTEGER, Diff BLOB,
					PRIMARY KEY (PostID, No));
				CREATE TABLE Lines (PostID INTEGER REFERENCES Posts, No INTEGER, Text TEXT,
					PRIMARY KEY (PostID, No));
				CREATE TABLE Likes (PostID INTEGER REFERENCES Posts, Tag TEXT REFERENCES Tags, Day DATE,
					PRIMARY KEY (PostID, Tag));
				CREATE TABLE Edits (PostID INTEGER, No INTEGER, Note TEXT, PRIMARY KEY (PostID, No),
					FOREIGN KEY (PostID, No) REFERENCES Revisions);
				INSERT INTO Tags VALUES ('go');
				INSERT INTO Posts VALUES (1, 'x'), (2, NULL);
				INSERT INTO PostTags VALUES (1, 'go');
				INSERT INTO Friends VALUES (1, 2), (2, 1);
				INSERT INTO Revisions VALUES (1, 1, x'00');
				INSERT INTO Lines VALUES (1, 1, 'a' || char(10) || 'b');
				INSERT INTO Likes VALUES (1, 'go', '2026-10-22');
				INSERT INTO Edits VALUES (1, 1, 'fix');`,
			schema: `schema Edit {
  revision: -> Revision
  note:     Text?
}

schema Like {
  tag: -> Tag
  day: Date?
}

schema Line {
  post: -> Post
  text: Prose?
}

schema Post {
  body:  Text?
  likes: [Like]?
  posts: [-> Post]

  friends-by-posts: [Post] <- posts
  lines:            [Line] <- post
  tags:             [Tag] <- posts
  revisions:        [Revision] <- post
}

schema Revision {
  post: -> Post
  diff: File?

  edit: Edit? <- revision
}

schema Tag {
  posts: [-> Post]

  likes: [Like] <- tag
}`,
			docs: map[string]string{
				"test/posts/1.kan": `# Post 1

body: x

## Likes

| id | tag | day |
|---|---|---|
| go | [[../tags/go]] | 2026-10-22 |

## Posts

- [[2]]
`,
				"test/tags/go.kan": `# Tag go

## Posts

- [[../posts/1]]
`,
				"test/edits/1-1.kan": `# Edit 1-1

revision: [[../revisions/1-1]]
note: fix
`,
				"test/lines/1-1.kan": `# Line 1-1

post: [[../posts/1]]

## Text

a
b
`,
				"test/revisions/1-1/diff.bin": "\x00",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, dir := importSQL(t, tt.sql)
			got, _ := os.ReadFile(filepath.Join(dir, "schema.kanonarch"))
			schema := string(got)
			i := strings.Index(schema, "\n\n")
			j := strings.LastIndex(schema, "\n\nschema Test {")
			if i < 0 || j < i {
				t.Fatalf("schema:\n%s", schema)
			}
			if body := schema[i+2 : j]; body != tt.schema {
				t.Errorf("schema:\n%s\nwant:\n%s", body, tt.schema)
			}
			if !slices.Equal(res.Notes, tt.notes) {
				t.Errorf("notes:\n%s\nwant:\n%s", strings.Join(res.Notes, "\n"), strings.Join(tt.notes, "\n"))
			}
			for name, want := range tt.docs {
				if got, _ := os.ReadFile(filepath.Join(dir, name)); string(got) != want {
					t.Errorf("%s:\n%s\nwant:\n%s", name, got, want)
				}
			}
		})
	}
}

// TestErrors checks what kan import refuses, and that it writes nothing then.
func TestErrors(t *testing.T) {
	tests := []struct{ name, sql, err string }{
		{"no table", `CREATE VIEW V AS SELECT 1;`, "the database has no table"},
		{"dangling", `CREATE TABLE T (ID INTEGER PRIMARY KEY, U INTEGER REFERENCES Nowhere);`,
			"`T.U` refers to `Nowhere`, which is not a table"},
		{"no key", `CREATE TABLE U (A INTEGER, B INTEGER, UNIQUE (A, B));
				CREATE TABLE T (ID INTEGER PRIMARY KEY, A INTEGER, FOREIGN KEY (A) REFERENCES U);`,
			"`T.A` refers to `U`, which has no primary key"},
		{"no row", `CREATE TABLE U (ID INTEGER PRIMARY KEY);
				CREATE TABLE T (ID INTEGER PRIMARY KEY, U INTEGER REFERENCES U); INSERT INTO T VALUES (1, 7);`,
			"`T.U`: no row of `U` has the key \"7\""},
		{"id field", `CREATE TABLE T (Id TEXT, Name TEXT);`,
			"the column `T.Id` gives the field `id`, the id of every object; rename the column"},
		{"key size", `CREATE TABLE U (A INTEGER, B INTEGER, PRIMARY KEY (A, B));
				CREATE TABLE T (ID INTEGER PRIMARY KEY, A INTEGER, FOREIGN KEY (A) REFERENCES U);`,
			"`T.A` is 1 column but refers to the 2 key columns of `U`"},
		{"key columns", `CREATE TABLE U (A INTEGER PRIMARY KEY, B INTEGER UNIQUE);
				CREATE TABLE T (ID INTEGER PRIMARY KEY, B INTEGER REFERENCES U (B));`,
			"`T.B` refers to `U.B`; kan import follows foreign keys to the primary key only"},
		{"same id", `CREATE TABLE T (Code TEXT PRIMARY KEY); INSERT INTO T VALUES ('A b'), ('a-B');`,
			"`T`: the keys \"A b\" and \"a-B\" both give the id `a-b`"},
		{"no id", `CREATE TABLE T (Code TEXT PRIMARY KEY); INSERT INTO T VALUES ('été'), ('!');`,
			"`T`: the key \"!\" gives no id"},
		{"null key", `CREATE TABLE T (Code TEXT PRIMARY KEY); INSERT INTO T VALUES (NULL);`,
			"`T`: a row has no key"},
		{"no parent", `CREATE TABLE P (ID INTEGER PRIMARY KEY);
				CREATE TABLE C (P INTEGER REFERENCES P, No INTEGER, Name TEXT, PRIMARY KEY (P, No));
				INSERT INTO C VALUES (NULL, 1, 'x');`,
			"`C`: a row has no key"},
		{"blob key", `CREATE TABLE T (Code BLOB PRIMARY KEY); INSERT INTO T VALUES (x'00');`,
			"`T`: a row has a file as key"},
		{"same field", `CREATE TABLE T (ID INTEGER PRIMARY KEY, FirstName TEXT, first_name TEXT);`,
			"`T.FirstName` and `T.first_name` both give the field `first-name`"},
		{"same schema", `CREATE TABLE Category (ID INTEGER PRIMARY KEY); CREATE TABLE Categories (ID INTEGER PRIMARY KEY);`,
			"the tables `Categories` and `Category` both give the schema `Category`"},
		{"native", `CREATE TABLE Texts (ID INTEGER PRIMARY KEY);`,
			"the table `Texts` gives the schema `Text`, a native type; rename the table"},
		{"result", `CREATE TABLE Tests (ID INTEGER PRIMARY KEY);`,
			"the table `Tests` gives the schema `Test`, the name of the result; import into another folder"},
		{"bad table", `CREATE TABLE "2024" (ID INTEGER PRIMARY KEY);`,
			"the table `2024` gives no schema name; rename the table"},
		{"bad column", `CREATE TABLE T (ID INTEGER PRIMARY KEY, "1st" TEXT);`,
			"the column `T.1st` gives no field name; rename the column"},
		{"same inverse", `CREATE TABLE U (ID INTEGER PRIMARY KEY, Ts TEXT, "Ts-by-u" TEXT);
				CREATE TABLE T (ID INTEGER PRIMARY KEY, U INTEGER REFERENCES U);`,
			"`U` already has a field `ts-by-u`, the inverse of `T.U`"},
		{"mixed blob", `CREATE TABLE T (ID INTEGER PRIMARY KEY, F BLOB); INSERT INTO T VALUES (1, x'00'), (2, 'a');`,
			"`T.F` mixes files with other values"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := writeSQL(t, tt.sql)
			dir := filepath.Join(t.TempDir(), "test")
			_, err := Import(src, dir)
			if err == nil || err.Error() != tt.err {
				t.Errorf("error %v, want %s", err, tt.err)
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Errorf("%s was written", dir)
			}
		})
	}
}

// TestSources checks the source and the folder given.
func TestSources(t *testing.T) {
	tmp := t.TempDir()
	text := filepath.Join(tmp, "notes.txt")
	os.WriteFile(text, []byte("hello"), 0o644)
	plain := filepath.Join(tmp, "plain")
	os.WriteFile(plain, nil, 0o644)
	bad := writeSQL(t, "CREATE TABL T;")
	full := filepath.Join(tmp, "full")
	os.MkdirAll(full, 0o755)
	os.WriteFile(filepath.Join(full, "x"), nil, 0o644)
	empty := filepath.Join(tmp, "empty")
	os.MkdirAll(empty, 0o755)
	tests := []struct{ src, dir, err string }{
		{filepath.Join(tmp, "none.sqlite"), filepath.Join(tmp, "a"), "open " + filepath.Join(tmp, "none.sqlite")},
		{text, filepath.Join(tmp, "a"), text + " is not a SQLite database nor a .sql script"},
		{filepath.Join(tmp, "none.sql"), filepath.Join(tmp, "a"), "open " + filepath.Join(tmp, "none.sql")},
		{bad, filepath.Join(tmp, "a"), bad + ": "},
		{"testdata/library.sql", full, full + " is not empty; kan import writes into a new folder"},
		{"testdata/library.sql", plain, plain + " is not a folder"},
		{"testdata/library.sql", filepath.Join(tmp, "Not A Slug"), "name the folder not-a-slug, not \"Not A Slug\""},
	}
	for _, tt := range tests {
		_, err := Import(tt.src, tt.dir)
		if err == nil || !strings.HasPrefix(err.Error(), tt.err) {
			t.Errorf("Import(%s, %s): %v, want %s…", tt.src, tt.dir, err, tt.err)
		}
	}
	if _, err := Import("testdata/library.sql", empty); err != nil {
		t.Errorf("an empty folder: %v", err)
	}
}

// TestProse checks how a multi-line text is written so that it reads back the same.
func TestProse(t *testing.T) {
	tests := []struct{ in, want string }{
		{"a\r\n  b  \n\n\n c", "a\nb\n\nc"},
		{"# not a heading\n| not a table\n- not\n* an\n+ item\n12. nor\n3) this", "\\# not a heading\n\\| not a table\n\\- not\n\\* an\n\\+ item\n12\\. nor\n3\\) this"},
		{"```go\n~~~\n-dash\n12.5 ok", "\\`\\`\\`go\n\\~~~\n-dash\n12.5 ok"},
		{"a [b] [[c]] {d} `e` \\f [g](h) ![i](j.png)", "a \\[b] \\[\\[c]] \\{d} \\`e\\` \\\\f \\[g](h) !\\[i](j.png)"},
		{"note: x\nb-2: y\nNote: z", "note\\: x\nb-2\\: y\nNote: z"},
	}
	for _, tt := range tests {
		if got := prose(tt.in); got != tt.want {
			t.Errorf("prose(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestProseLints writes a text that holds what Kanon reads in prose, lints the result and reads the text back.
func TestProseLints(t *testing.T) {
	body := "Use {name} and [[x]] or [t](u), ![i](j.png) and [b].\n" +
		"A \\ backslash, `code {y}` and a ` lone tick.\n" +
		"- item: not a key\n" +
		"note: not a key either\n\n" +
		"# not a heading\n" +
		"| nor a table |\n" +
		"1. nor a list"
	_, dir := importSQL(t, "CREATE TABLE T (ID INTEGER PRIMARY KEY, Body TEXT);\nINSERT INTO T VALUES (1, '"+
		strings.ReplaceAll(body, "'", "''")+"');")
	lintClean(t, dir)
	path := filepath.Join(dir, "test", "ts", "1.kan")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	d, diags := doc.Parse(path, string(src))
	if len(diags) > 0 || len(d.Root.Children) != 1 || len(d.Root.Children[0].Children) != 1 {
		t.Fatalf("%s doesn't read as one heading and one section: %v\n%s", path, diags, src)
	}
	body2 := d.Root.Children[0].Children[0]
	if len(body2.Keys) > 0 {
		t.Errorf("the section reads key lines: %v", body2.Keys[0].Text)
	}
	var paragraphs []string
	for _, b := range body2.Blocks {
		p, ok := b.(*doc.Paragraph)
		if !ok {
			t.Fatalf("the section reads a %T at %s", b, b.At())
		}
		var lines []string
		for _, l := range p.Lines {
			lines = append(lines, unescape(l.Text))
		}
		paragraphs = append(paragraphs, strings.Join(lines, "\n"))
	}
	if got := strings.Join(paragraphs, "\n\n"); got != body {
		t.Errorf("the text reads back as\n%s\nwant\n%s", got, body)
	}
}

// unescape drops the backslash before each character it escapes.
func unescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// TestWords checks how table and column names are cut into words, and the schema and the field a table gives. The
// field is plural whether the table is or not, so that the tables kan sql writes, singular, give back the same
// fields.
func TestWords(t *testing.T) {
	tests := []struct{ in, kebab, schema, field string }{
		{"Order Details", "order-details", "OrderDetail", "order-details"},
		{"order_detail", "order-detail", "OrderDetail", "order-details"},
		{"Categories", "categories", "Category", "categories"},
		{"category", "category", "Category", "categories"},
		{"EmployeeTerritories", "employee-territories", "EmployeeTerritory", "employee-territories"},
		{"CustomerCustomerDemo", "customer-customer-demo", "CustomerCustomerDemo", "customer-customer-demos"},
		{"HTMLPages", "html-pages", "HtmlPage", "html-pages"},
		{"Address2", "address2", "Address2", "address2"},
		{"Status", "status", "Status", "status"},
		{"Address", "address", "Address", "addresses"},
		{"Addresses", "addresses", "Address", "addresses"},
		{"Boxes", "boxes", "Box", "boxes"},
		{"Branches", "branches", "Branch", "branches"},
		{"Keys", "keys", "Key", "keys"},
		{"Analysis", "analysis", "Analysis", "analysis"},
		{"ship_via", "ship-via", "ShipVia", "ship-vias"},
		{"Prénom", "prenom", "Prenom", "prenoms"},
		{"T", "t", "T", "ts"},
	}
	for _, tt := range tests {
		if got := kebab(tt.in); got != tt.kebab {
			t.Errorf("kebab(%q) = %q, want %q", tt.in, got, tt.kebab)
		}
		if got := schemaName(tt.in); got != tt.schema {
			t.Errorf("schemaName(%q) = %q, want %q", tt.in, got, tt.schema)
		}
		if got := fieldName(tt.in); got != tt.field {
			t.Errorf("fieldName(%q) = %q, want %q", tt.in, got, tt.field)
		}
	}
}

// TestTree checks that tree skips the folders and the files it is told to, at the top of the tree only.
func TestTree(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"source/create.sql", "expected.txt", "a.kan", "a/expected.txt", "a/source/b.kan"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := slices.Sorted(maps.Keys(tree(t, root, "source", "expected.txt")))
	if want := []string{"a.kan", "a/expected.txt", "a/source/b.kan"}; !slices.Equal(got, want) {
		t.Errorf("tree = %q, want %q", got, want)
	}
}

// compareTrees compares two folders file by file, skipping the folders and files skip at the top of want.
func compareTrees(t *testing.T, want, got string, skip ...string) {
	t.Helper()
	wantFiles, gotFiles := tree(t, want, skip...), tree(t, got)
	for name, w := range wantFiles {
		g, ok := gotFiles[name]
		switch {
		case !ok:
			t.Errorf("%s is not written", name)
		case g != w:
			t.Errorf("%s:\n%s\nwant:\n%s", name, g, w)
		}
	}
	for name := range gotFiles {
		if _, ok := wantFiles[name]; !ok {
			t.Errorf("%s is written but not expected", name)
		}
	}
}

func tree(t *testing.T, root string, skip ...string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		switch {
		case slices.Contains(skip, filepath.ToSlash(rel)) && d.IsDir():
			return filepath.SkipDir
		case slices.Contains(skip, filepath.ToSlash(rel)), d.IsDir():
			return nil
		}
		b, err := os.ReadFile(path)
		files[filepath.ToSlash(rel)] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// importSQL imports a script into a folder named test.
func importSQL(t *testing.T, script string) (*Result, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "test")
	res, err := Import(writeSQL(t, script), dir)
	if err != nil {
		t.Fatal(err)
	}
	return res, dir
}

func writeSQL(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.sql")
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// run runs a script on a database file.
func run(t *testing.T, path, script string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(script); err != nil {
		t.Fatal(err)
	}
}
