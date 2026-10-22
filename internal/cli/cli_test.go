package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUsage(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{nil, usage + "\n"},
		{[]string{"check", "a.kanonarch", "b.kan"}, usage + "\n"},
		{[]string{"lint", "a.kanonarch"}, "kan: lint takes a schema and a document, got 1 argument\n" + usage + "\n"},
		{[]string{"lint", "a.yaml", "b.kan"}, "kan: a.yaml is not a schema: a schema file ends in .kanonarch\n" + usage + "\n"},
		{[]string{"lint", "a.kanonarch", "b.md"}, "kan: b.md is not a document: a document file ends in .kan\n" + usage + "\n"},
		{[]string{"eval", "a.kanonarch"}, "kan: eval takes a schema and a document, got 1 argument\n" + usage + "\n"},
		{[]string{"lint", "-today", "2026-10-22", "a.kanonarch"},
			"kan: lint takes a schema and a document, got 1 argument\n" + usage + "\n"},
		{[]string{"lint", "a.kanonarch", "b.kan", "-today"},
			"kan: -today takes a date, as in -today 2026-10-22\n" + usage + "\n"},
		{[]string{"eval", "-today", "22/10/2026", "a.kanonarch", "b.kan"},
			"kan: write -today 2026-10-22, not \"22/10/2026\"\n" + usage + "\n"},
		{[]string{"import", library}, "kan: import takes a database and a folder, got 1 argument\n" + usage + "\n"},
		{[]string{"import", "nowhere.sqlite", "out"}, "kan: nowhere.sqlite does not exist\n" + usage + "\n"},
		{[]string{"import", library, "."}, "kan: . is not empty; kan import writes into a new folder\n" + usage + "\n"},
		{[]string{"import", library, "My Library"}, "kan: name the folder my-library, not \"My Library\"\n" + usage + "\n"},
		{[]string{"sql", shop}, "kan: sql takes a schema and a document, got 1 argument\n" + usage + "\n"},
		{[]string{"sql", shop, catalog, "-o"}, "kan: -o takes a file: out.sql, out.sqlite or out.db\n" + usage + "\n"},
		{[]string{"sql", "-o", "out.txt", shop, catalog}, "kan: -o names a .sql, .sqlite or .db file, not out.txt\n" + usage + "\n"},
		{[]string{"sql", "-q", "SELECT 1", shop, catalog, "-o", "out.db"},
			"kan: -q prints the rows and writes no file: drop -o\n" + usage + "\n"},
		{[]string{"sql", shop, catalog, "-q"}, "kan: -q takes a query: -q 'SELECT …' or -q @query.sql\n" + usage + "\n"},
		{[]string{"sql", "-q", "  ", shop, catalog}, "kan: -q takes a query: -q 'SELECT …' or -q @query.sql\n" + usage + "\n"},
		{[]string{"sql", "-q", "SELECT 1", shop, catalog, "-format"}, "kan: -format takes table or csv\n" + usage + "\n"},
		{[]string{"sql", "-q", "SELECT 1", "-format", "json", shop, catalog},
			"kan: -format is table or csv, not json\n" + usage + "\n"},
		{[]string{"sql", shop, catalog, "-format", "csv"}, "kan: -format goes with -q\n" + usage + "\n"},
		{[]string{"sql", "-q", "SELECT 1", shop}, "kan: sql takes a schema and a document, got 1 argument\n" + usage + "\n"},
		{[]string{"sql", "-q", "@nowhere.sql", shop, catalog}, "kan: nowhere.sql does not exist\n" + usage + "\n"},
		{[]string{"sql", "-q", "@../sqlexport", shop, catalog},
			"kan: cannot read ../sqlexport: is a directory\n" + usage + "\n"},
		{[]string{"sql", shop, catalog, "-o", "../sqlexport/testdata/shop.sql"},
			"kan: ../sqlexport/testdata/shop.sql already exists; kan sql writes a new file: give -o another one\n" + usage + "\n"},
	}
	for _, tt := range tests {
		var stdout, stderr bytes.Buffer
		if code := Run(tt.args, &stdout, &stderr); code != 2 {
			t.Errorf("Run(%q) = %d, want 2", tt.args, code)
		}
		if stdout.Len() > 0 {
			t.Errorf("Run(%q) wrote to stdout: %q", tt.args, stdout.String())
		}
		if stderr.String() != tt.want {
			t.Errorf("Run(%q) stderr\n got %q\nwant %q", tt.args, stderr.String(), tt.want)
		}
	}
}

const library = "../sqlimport/testdata/library.sql"

func TestImport(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "library")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"import", library, dir}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run = %d, want 0; stderr %q", code, stderr.String())
	}
	want := library + ": note: `Books`: 2 keys change to make ids, such as \"DUNE\" → \"dune\"\n" +
		library + ": note: `Books.Summary`: 1 empty text is written as no value\n" +
		library + ": note: `Members.Name`: the spaces around 1 value are dropped\n" +
		library + ": note: the view `Overdue` is not imported\n" +
		"wrote " + dir + ": 6 schemas, 12 documents, 3 files\n"
	if stdout.String() != want || stderr.Len() > 0 {
		t.Errorf("stdout\n got %q\nwant %q\nstderr %q", stdout.String(), want, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "library", "books", "dune.kan")); err != nil {
		t.Error(err)
	}
}

func TestImportFails(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "bad.sql")
	if err := os.WriteFile(src, []byte("CREATE TABLE T (A INTEGER PRIMARY KEY, B REFERENCES Nowhere);\nINSERT INTO T VALUES (1, 2);"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"import", src, filepath.Join(tmp, "out")}, &stdout, &stderr); code != 1 {
		t.Errorf("Run = %d, want 1", code)
	}
	want := "kan: `T.B` refers to `Nowhere`, which is not a table\n"
	if stdout.Len() > 0 || stderr.String() != want {
		t.Errorf("stdout %q\nstderr\n got %q\nwant %q", stdout.String(), stderr.String(), want)
	}
}

const (
	shop    = "../sqlexport/testdata/shop.kanonarch"
	catalog = "../sqlexport/testdata/catalog.kan"
)

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestSQL checks the script kan sql prints against the one written by hand in sqlexport, then writes it to each
// kind of file.
func TestSQL(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"sql", catalog, shop}, &stdout, &stderr); code != 0 {
		t.Fatalf("Run = %d, want 0; stderr %q", code, stderr.String())
	}
	script := "PRAGMA foreign_keys = ON;\nBEGIN;\n\n" + read(t, "../sqlexport/testdata/shop.sql") + "\n" +
		read(t, "../sqlexport/testdata/catalog.sql") + "\nCOMMIT;\n"
	if stdout.String() != script || stderr.Len() > 0 {
		t.Errorf("stdout\n got %q\nwant %q\nstderr %q", stdout.String(), script, stderr.String())
	}
	for _, name := range []string{"shop.sql", "shop.sqlite", "shop.db"} {
		out := filepath.Join(t.TempDir(), name)
		var stdout, stderr bytes.Buffer
		if code := Run([]string{"sql", shop, catalog, "-o", out}, &stdout, &stderr); code != 0 {
			t.Fatalf("Run -o %s = %d, want 0; stderr %q", name, code, stderr.String())
		}
		if want := "wrote " + out + ": 15 tables, 35 rows\n"; stdout.String() != want || stderr.Len() > 0 {
			t.Errorf("stdout\n got %q\nwant %q\nstderr %q", stdout.String(), want, stderr.String())
		}
		if got := read(t, out); name == "shop.sql" && got != script {
			t.Errorf("%s holds another script", out)
		} else if name != "shop.sql" && !strings.HasPrefix(got, "SQLite format 3\x00") {
			t.Errorf("%s is no SQLite database", out)
		}
	}
}

// TestSQLFails checks that kan sql writes nothing when the document has errors, lint's or its own, and says why
// it cannot write.
func TestSQLFails(t *testing.T) {
	tmp := t.TempDir()
	files := map[string]string{
		"s.kanonarch":  "schema Root { a: [P]  b: [P] }\nschema P { n: Int }\n",
		"bad.kan":      "---\ntype: P\n---\n\n# Bad\n\nn: many\n",
		"ok.kan":       "---\ntype: P\n---\n\n# Ok\n\nn: 1\n",
		"root.kan":     "---\ntype: Root\n---\n\n# Root\n",
		"root/a/x.kan": "# X\n\nn: 1\n",
		"root/b/x.kan": "# X\n\nn: 2\n",
	}
	for path, src := range files {
		path = filepath.Join(tmp, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := filepath.Join(tmp, "s.kanonarch")
	out := filepath.Join(tmp, "out.sqlite")
	tests := []struct {
		doc, out, want string
	}{
		{"bad.kan", out, filepath.Join(tmp, "bad.kan") + ":7:4: error: write a whole number (4 or 1 250), not \"many\"\n1 error\n"},
		{"root.kan", out, filepath.Join(tmp, "root/b/x.kan") + ": error: the id `x` is already used by a `P`, as " +
			filepath.Join(tmp, "root/a/x.kan") + "; in SQL each `P` needs an id of its own\n1 error\n"},
		{"ok.kan", filepath.Join(tmp, "nowhere", "out.db"),
			"kan: open " + filepath.Join(tmp, "nowhere", "out.db") + ": no such file or directory\n"},
	}
	for _, tt := range tests {
		var stdout, stderr bytes.Buffer
		if code := Run([]string{"sql", s, filepath.Join(tmp, tt.doc), "-o", tt.out}, &stdout, &stderr); code != 1 {
			t.Errorf("Run %s = %d, want 1", tt.doc, code)
		}
		if stdout.Len() > 0 || stderr.String() != tt.want {
			t.Errorf("Run %s: stdout %q\nstderr\n got %q\nwant %q", tt.doc, stdout.String(), stderr.String(), tt.want)
		}
		if _, err := os.Stat(tt.out); !os.IsNotExist(err) {
			t.Errorf("Run %s wrote %s", tt.doc, tt.out)
		}
	}
}

// TestQuery checks the rows kan sql -q prints, as a table and as CSV, for a query given or read from a file; and
// what it says, with exit code 1, when the query is wrong, a file is empty or the document has errors.
func TestQuery(t *testing.T) {
	tmp := t.TempDir()
	write := func(name, src string) string {
		path := filepath.Join(tmp, name)
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	byStaff := write("staff.sql", "-- the staff\nSELECT id, staff\nFROM shop\nORDER BY staff DESC;\n")
	empty := write("empty.sql", " \n")
	wrong := write("wrong.sql", "SELECT id,\n       nme\nFROM shop\n")
	bad := write("bad.kan", "---\ntype: Catalog\n---\n\n# Bad\n\nshops: many\n")
	// The diagnostics of a document with errors are those of kan sql without -q.
	var plain bytes.Buffer
	if code := Run([]string{"sql", shop, bad}, &plain, &plain); code != 1 || !strings.Contains(plain.String(), "2 errors") {
		t.Fatalf("kan sql %s = %d: %s", bad, code, plain.String())
	}
	lintErrors := plain.String()
	table := "id     staff\n-----  -----\nlyon      12\nparis      3\n2 rows\n"
	tests := []struct {
		name         string
		args         []string
		code         int
		stdout, errs string
	}{
		{"query", []string{"-q", "SELECT id, staff FROM shop ORDER BY id", shop, catalog}, 0, table, ""},
		{"documents first", []string{"-q", "SELECT id, staff FROM shop ORDER BY id", catalog, shop}, 0, table, ""},
		{"file", []string{"-q", "@" + byStaff, shop, catalog}, 0, table, ""},
		{"csv", []string{"-q", "SELECT id, staff, rating FROM shop ORDER BY id", "-format", "csv", shop, catalog}, 0,
			"id,staff,rating\nlyon,12,4.5\nparis,3,\n", ""},
		{"table", []string{"-q", "SELECT 1 AS one", "-format", "table", shop, catalog}, 0, "one\n---\n  1\n1 row\n", ""},
		{"sql error", []string{"-q", "SELECT nme FROM shop", shop, catalog}, 1, "",
			"query:1:8: error: no such column `nme`; did you mean `name`?\n"},
		{"sql error in a file", []string{"-q", "@" + wrong, shop, catalog}, 1, "",
			wrong + ":2:8: error: no such column `nme`; did you mean `name`?\n"},
		{"writes", []string{"-q", "DELETE FROM shop", shop, catalog}, 1, "",
			"query:1:1: error: a query only reads: write SELECT or WITH, not \"DELETE\"\n"},
		{"empty file", []string{"-q", "@" + empty, shop, catalog}, 2, "", ""},
		{"lint errors", []string{"-q", "SELECT 1", shop, bad}, 1, "", lintErrors},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(append([]string{"sql"}, tt.args...), &stdout, &stderr)
			if code != tt.code {
				t.Errorf("Run = %d, want %d; stderr %q", code, tt.code, stderr.String())
			}
			if code == 2 {
				want := "kan: " + empty + " holds no query\n" + usage + "\n"
				if stderr.String() != want {
					t.Errorf("stderr %q, want %q", stderr.String(), want)
				}
				return
			}
			if stdout.String() != tt.stdout || stderr.String() != tt.errs {
				t.Errorf("stdout\n got %q\nwant %q\nstderr\n got %q\nwant %q", stdout.String(), tt.stdout, stderr.String(),
					tt.errs)
			}
		})
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

// TestQueryBrokenPipe checks that kan sql -q says so, with exit code 1, when it cannot print its rows.
func TestQueryBrokenPipe(t *testing.T) {
	for _, format := range []string{"table", "csv"} {
		var stderr bytes.Buffer
		args := []string{"sql", "-q", "SELECT 1", "-format", format, shop, catalog}
		if code := Run(args, failingWriter{}, &stderr); code != 1 || stderr.String() != "kan: broken pipe\n" {
			t.Errorf("-format %s: Run = %d, stderr %q", format, code, stderr.String())
		}
	}
}

// TestWarnings checks that kan lint and kan eval succeed on a document that only has warnings, and that kan eval
// prints the computed fields of a document whose rules fail, then the errors.
func TestWarnings(t *testing.T) {
	tmp := t.TempDir()
	files := map[string]string{
		"s.kanonarch":   "schema Root { ps: [P]  n: Int = count(ps) }\nschema P { n: Int }\nrule few(r: Root) -> Bool = r.n < 1\n",
		"s2.kanonarch":  "schema Root { ps: [P]  n: Int = count(ps) }\nschema P { n: Int }\n",
		"root.kan":      "---\ntype: Root\n---\n\n# Root\n",
		"root/ps/x.kan": "# X\n\nn: 1\n",
		"root/ps/x.txt": "x",
	}
	for path, src := range files {
		path = filepath.Join(tmp, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	doc := filepath.Join(tmp, "root.kan")
	warning := filepath.Join(tmp, "root/ps/x.txt") + ": warning: not a .kan file; ignored\n"
	tests := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"lint", filepath.Join(tmp, "s2.kanonarch"), doc}, 0, warning + "1 warning\n"},
		{[]string{"eval", filepath.Join(tmp, "s2.kanonarch"), doc}, 0, doc + ": n = 1\n" + warning + "1 warning\n"},
		{[]string{"eval", filepath.Join(tmp, "s.kanonarch"), doc}, 1,
			doc + ": n = 1\n" + doc + ": error: `few` fails; 1 is not less than 1\n" + warning + "1 error, 1 warning\n"},
	}
	for _, tt := range tests {
		var stdout, stderr bytes.Buffer
		if code := Run(tt.args, &stdout, &stderr); code != tt.code {
			t.Errorf("Run %s = %d, want %d", tt.args[:2], code, tt.code)
		}
		if stdout.String() != tt.want || stderr.Len() > 0 {
			t.Errorf("Run %s: stderr %q\nstdout\n got %q\nwant %q", tt.args[:2], stderr.String(), stdout.String(), tt.want)
		}
	}
}

// TestToday checks that -today gives the rules their today, and that it is the local date without it.
func TestToday(t *testing.T) {
	t.Chdir(t.TempDir())
	files := map[string]string{
		"s.kanonarch": "schema Root { fin: Date  jour: Date = today }\nrule ouvert(r: Root) -> Bool = r.fin >= today\n",
		"root.kan":    "---\ntype: Root\n---\n\n# Root\n\nfin: 2099-12-31\n",
	}
	for path, src := range files {
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"eval", "-today", "2026-10-22", "s.kanonarch", "root.kan"}, 0, "root.kan: jour = 2026-10-22\n"},
		{[]string{"lint", "s.kanonarch", "-today", "2099-12-31", "root.kan"}, 0, "ok  s.kanonarch\nok  root.kan\n"},
		{[]string{"lint", "s.kanonarch", "root.kan", "-today", "2100-01-01"}, 1,
			"root.kan: error: `ouvert` fails; 2099-12-31 is before 2100-01-01\n1 error\n"},
		{[]string{"eval", "s.kanonarch", "root.kan"}, 0, "root.kan: jour = " + time.Now().Format(time.DateOnly) + "\n"},
	}
	for _, tt := range tests {
		var stdout, stderr bytes.Buffer
		if code := Run(tt.args, &stdout, &stderr); code != tt.code {
			t.Errorf("Run %s = %d, want %d", tt.args, code, tt.code)
		}
		if stdout.String() != tt.want || stderr.Len() > 0 {
			t.Errorf("Run %s: stderr %q\nstdout\n got %q\nwant %q", tt.args, stderr.String(), stdout.String(), tt.want)
		}
	}
}

func TestPathsInEitherOrder(t *testing.T) {
	for _, args := range [][]string{{"s.kanonarch", "d.kan"}, {"d.kan", "s.kanonarch"}} {
		s, d, err := paths("lint", args)
		if err != nil || s != "s.kanonarch" || d != "d.kan" {
			t.Errorf("paths(%q) = %q, %q, %v", args, s, d, err)
		}
	}
}

func TestSummary(t *testing.T) {
	tests := []struct {
		errs, warns int
		want        string
	}{
		{1, 0, "1 error"},
		{2, 1, "2 errors, 1 warning"},
		{0, 3, "3 warnings"},
	}
	for _, tt := range tests {
		if got := summary(tt.errs, tt.warns); got != tt.want {
			t.Errorf("summary(%d, %d) = %q, want %q", tt.errs, tt.warns, got, tt.want)
		}
	}
}
