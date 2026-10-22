package sqlexport

import (
	"bytes"
	"database/sql"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/lint"
	"github.com/bn-k/kanon/internal/model"
)

// doc loads a document that lints without errors.
func doc(t *testing.T, schemaPath, docPath string) *model.Doc {
	t.Helper()
	d, r := lint.Load(schemaPath, docPath)
	if d == nil || r.Diags.Count(diag.Error) > 0 {
		t.Fatalf("%s does not lint: %v", docPath, r.Diags)
	}
	return d
}

// export lays out a document that exports without errors.
func export(t *testing.T, d *model.Doc) *Export {
	t.Helper()
	e, ds := New(d)
	if len(ds) > 0 {
		t.Fatalf("New: %v", ds)
	}
	return e
}

// write writes files, by path, into the current folder.
func write(t *testing.T, files map[string]string) {
	t.Helper()
	for path, src := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// dump writes every row of every table of db, with the storage class of each value.
func dump(t *testing.T, db *sql.DB) string {
	t.Helper()
	tables, err := db.Query("SELECT name FROM sqlite_schema WHERE type = 'table' ORDER BY name")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for tables.Next() {
		var name string
		if err := tables.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	tables.Close()
	var b strings.Builder
	for _, name := range names {
		rows, err := db.Query("SELECT * FROM " + ident(name) + " ORDER BY rowid")
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			b.WriteString(name)
			for _, v := range vals {
				if bs, ok := v.([]byte); ok {
					v = fmt.Sprintf("%x", bs)
				}
				fmt.Fprintf(&b, " %T(%v)", v, v)
			}
			b.WriteString("\n")
		}
		rows.Close()
	}
	return b.String()
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestSQL checks the script of the catalog, every type of §4, against the one written by hand, then runs it.
func TestSQL(t *testing.T) {
	e := export(t, doc(t, "testdata/shop.kanonarch", "testdata/catalog.kan"))
	var b bytes.Buffer
	if err := e.SQL(&b); err != nil {
		t.Fatal(err)
	}
	want := "PRAGMA foreign_keys = ON;\nBEGIN;\n\n" + read(t, "testdata/shop.sql") + "\n" + read(t, "testdata/catalog.sql") +
		"\nCOMMIT;\n"
	compare(t, b.String(), want)
	db := open(t)
	if _, err := db.Exec(b.String()); err != nil {
		t.Fatal(err)
	}
}

// TestDatabase checks that the database written holds what the script does, value for value, storage class
// included, and that kan sql never overwrites a file.
func TestDatabase(t *testing.T) {
	e := export(t, doc(t, "testdata/shop.kanonarch", "testdata/catalog.kan"))
	var b bytes.Buffer
	if err := e.SQL(&b); err != nil {
		t.Fatal(err)
	}
	script := open(t)
	if _, err := script.Exec(b.String()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "shop.sqlite")
	if err := e.Database(path); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	want := dump(t, script)
	if !strings.Contains(want, "\nsupplier string(bolt) string(Bolt)\n") {
		t.Errorf("the script holds\n%s", want)
	}
	compare(t, dump(t, db), want)

	if e.Tables() != 15 || e.Rows() != 35 {
		t.Errorf("Tables, Rows = %d, %d, want 15, 35", e.Tables(), e.Rows())
	}
}

// TestNoOverwrite checks that kan sql never overwrites a file, and writes nothing where it cannot.
func TestNoOverwrite(t *testing.T) {
	e := export(t, doc(t, "testdata/shop.kanonarch", "testdata/catalog.kan"))
	dir := t.TempDir()
	script := filepath.Join(dir, "shop.sql")
	if err := CheckFile(script); err != nil {
		t.Errorf("CheckFile(%s) = %v", script, err)
	}
	if err := e.Script(script); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	e.SQL(&b)
	if read(t, script) != b.String() {
		t.Errorf("Script(%s) wrote another script than SQL", script)
	}
	for _, write := range []func(string) error{e.Script, e.Database} {
		err := write(script)
		if want := script + " already exists; kan sql writes a new file: give -o another one"; err == nil || err.Error() != want {
			t.Errorf("writing %s again = %v, want %q", script, err, want)
		}
		if err := CheckFile(script); err == nil || err.Error() != script+" already exists; kan sql writes a new file: give -o another one" {
			t.Errorf("CheckFile(%s) = %v", script, err)
		}
		if read(t, script) != b.String() {
			t.Errorf("writing %s again changed it", script)
		}
		if err := write(filepath.Join(dir, "nowhere", "shop.sqlite")); err == nil {
			t.Error("writing into a folder that does not exist: no error")
		}
	}
}

// TestDatabaseFails checks that a database SQLite refuses is not left behind: a CHECK fails at its INSERT, a
// foreign key at COMMIT.
func TestDatabaseFails(t *testing.T) {
	tests := []struct {
		name  string
		spoil func(shop *model.Object)
		want  string
	}{
		{"check", func(shop *model.Object) { shop.Get("condition").Values[0].Raw = "broken" }, "CHECK constraint failed"},
		{"foreign key", func(shop *model.Object) { shop.Get("manager").Links[0] = &model.Object{ID: "nobody"} },
			"FOREIGN KEY constraint failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := doc(t, "testdata/shop.kanonarch", "testdata/catalog.kan")
			tt.spoil(d.Root.Get("shops").Objects[0])
			path := filepath.Join(t.TempDir(), "shop.db")
			err := export(t, d).Database(path)
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.HasPrefix(err.Error(), path+": ") {
				t.Errorf("Database = %v, want %s: … %s", err, path, tt.want)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("%s is left behind: %v", path, err)
			}
		})
	}
}

// TestNorthwindDatabase checks that the Northwind case exports to a database SQLite accepts.
func TestNorthwindDatabase(t *testing.T) {
	e := export(t, doc(t, "../../cases/northwind/schema.kanonarch", "../../cases/northwind/northwind.kan"))
	path := filepath.Join(t.TempDir(), "northwind.sqlite")
	if err := e.Database(path); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM order_detail").Scan(&n); err != nil || n == 0 {
		t.Errorf("order_detail holds %d rows: %v", n, err)
	}
}

// TestDates checks that a Date and a DateTime, declared DATE and DATETIME, are stored as the text written, zone and
// seconds kept, by the script and by the database: SQLite gives those columns NUMERIC affinity, but leaves a text
// that is not a number as it is.
func TestDates(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, map[string]string{
		"schema.kanonarch": "schema Event { day: Date  starts: DateTime  ends: DateTime  seen: DateTime }\n",
		"e.kan": "---\ntype: Event\n---\n\n# E\n\nday: 2026-10-22\nstarts: 2026-10-22 14:30\n" +
			"ends: 2026-10-22 18:00:05 +02:00\nseen: 2026-10-22 16:00 UTC\n",
	})
	e := export(t, doc(t, "schema.kanonarch", "e.kan"))
	var b bytes.Buffer
	if err := e.SQL(&b); err != nil {
		t.Fatal(err)
	}
	script := open(t)
	if _, err := script.Exec(b.String()); err != nil {
		t.Fatal(err)
	}
	if err := e.Database("e.sqlite"); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "e.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const want = "text 2026-10-22|text 2026-10-22 14:30|text 2026-10-22 18:00:05 +02:00|text 2026-10-22 16:00 UTC"
	for name, db := range map[string]*sql.DB{"script": script, "database": db} {
		var got string
		err := db.QueryRow("SELECT typeof(day) || ' ' || day || '|' || typeof(starts) || ' ' || starts || '|' || " +
			"typeof(ends) || ' ' || ends || '|' || typeof(seen) || ' ' || seen FROM event").Scan(&got)
		if err != nil || got != want {
			t.Errorf("%s: event holds %q, %v, want %q", name, got, err, want)
		}
	}
}

// TestRefused checks what New reports: two objects of a table with one id, an object whose rows need the object
// holding it, names that clash, a file it cannot read.
func TestRefused(t *testing.T) {
	const schema = `schema Root { shops: [Shop]?  a: [P]?  b: [P]? }
schema Shop { name: Text  boss: -> P?  people: [P]?  orders: [Order]? }
schema P { name: Text }
schema Order { date: Date }
`
	tests := []struct {
		name  string
		files map[string]string
		doc   string
		want  []string
	}{
		{"two files", map[string]string{
			"root.kan":       "---\ntype: Root\n---\n\n# Root\n",
			"root/a/x.kan":   "# X\n\nname: X\n",
			"root/b/x.kan":   "# X\n\nname: X\n",
			"root/b/y.kan":   "# Y\n\nname: Y\n",
			"root/a/y-1.kan": "# Y\n\nname: Y\n",
		}, "root.kan", []string{
			"root/b/x.kan: error: the id `x` is already used by a `P`, as root/a/x.kan; in SQL each `P` needs an id of its own",
		}},
		{"two rows", map[string]string{
			"root.kan":            "---\ntype: Root\n---\n\n# Root\n",
			"root/shops/lyon.kan": "# Lyon\n\nname: Lyon\n\n## People\n\n| id | name |\n|---|---|\n| ana | Ana |\n| bo | Bo |\n",
			"root/shops/nice.kan": "# Nice\n\nname: Nice\n\n## People\n\n| id | name |\n|---|---|\n| bo | Bo |\n\n## Orders\n\n| id | date |\n|---|---|\n| ana | 2026-10-22 |\n",
		}, "root.kan", []string{
			"root/shops/nice.kan:9:1: error: the id `bo` is already used by a `P`, at root/shops/lyon.kan:10:1; in SQL each `P` needs an id of its own",
		}},
		{"held", map[string]string{
			"o1.kan": "---\ntype: Order\n---\n\n# O1\n\ndate: 2026-10-22\n",
		}, "o1.kan", []string{
			"o1.kan: error: `Order` rows are keyed by the `Shop` that holds them, by `orders`: export the document that holds this one",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			tt.files["schema.kanonarch"] = schema
			write(t, tt.files)
			e, ds := New(doc(t, "schema.kanonarch", tt.doc))
			var got []string
			for _, d := range ds {
				got = append(got, d.String())
			}
			if e != nil || strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("New = %v\n%s\nwant\n%s", e, strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestRefusedLayout(t *testing.T) {
	t.Chdir(t.TempDir())
	write(t, map[string]string{
		"schema.kanonarch": "schema Item { price: Money  price-currency: Text }\n",
		"i.kan":            "---\ntype: Item\n---\n\n# I\n\nprice: 1 EUR\nprice-currency: EUR\n",
	})
	e, ds := New(doc(t, "schema.kanonarch", "i.kan"))
	if e != nil || len(ds) != 1 || !strings.Contains(ds[0].Msg, "both give the column `price_currency`") {
		t.Errorf("New = %v, %v", e, ds)
	}
}

func TestUnreadable(t *testing.T) {
	d := doc(t, "testdata/shop.kanonarch", "testdata/catalog.kan")
	logo := d.Root.Get("shops").Objects[0].Get("logo")
	logo.Files[0] = "testdata/nowhere.svg"
	e, ds := New(d)
	want := diag.Diagnostic{Pos: logo.Pos, Severity: diag.Error, Msg: "cannot read testdata/nowhere.svg: no such file or directory"}
	if e != nil || len(ds) != 1 || ds[0] != want {
		t.Errorf("New = %v, %v, want %v", e, ds, want)
	}
}

func TestDecimal(t *testing.T) {
	tests := map[string]string{"12": "12", "-3/2": "-1.5", "2500001/2": "1250000.5", "1/1000": "0.001", "0": "0",
		"1/3": "0.333333333333333333333333333333"}
	for in, want := range tests {
		r, _ := new(big.Rat).SetString(in)
		if got := decimal(r); got != want {
			t.Errorf("decimal(%s) = %s, want %s", in, got, want)
		}
	}
}
