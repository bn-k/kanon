package sqlexport

import (
	"bytes"
	"errors"
	"math"
	"strings"
	"testing"
)

// catalogExport is the export of the catalog document: two shops, lyon and paris.
func catalogExport(t *testing.T) *Export {
	t.Helper()
	return export(t, doc(t, "testdata/shop.kanonarch", "testdata/catalog.kan"))
}

func TestQueryRows(t *testing.T) {
	e := catalogExport(t)
	tests := []struct {
		name, query, want string
	}{
		{"nulls and numbers", "SELECT id, staff, rating, area_unit FROM shop ORDER BY id",
			"id     staff  rating  area_unit\n" +
				"-----  -----  ------  ---------\n" +
				"lyon      12  4.5     m2\n" +
				"paris      3  NULL    NULL\n" +
				"2 rows\n"},
		{"aggregation", "SELECT count(*) AS shops, sum(staff) AS staff FROM shop",
			"shops  staff\n" +
				"-----  -----\n" +
				"    2     15\n" +
				"1 row\n"},
		{"no rows", "SELECT id, name FROM shop WHERE staff > 100",
			"id  name\n" +
				"--  ----\n" +
				"0 rows\n"},
		{"a join through a link", "SELECT s.id AS shop, p.name AS manager FROM shop s JOIN person p ON p.id = s.manager_id ORDER BY s.id",
			"shop   manager\n" +
				"-----  -------\n" +
				"lyon   Ana\n" +
				"paris  Zed\n" +
				"2 rows\n"},
		{"dates and money as written", "SELECT opened, updated, revenue, revenue_currency FROM shop WHERE id = 'lyon'",
			"opened" + strings.Repeat(" ", 6) + "updated" + strings.Repeat(" ", 23) + "revenue  revenue_currency\n" +
				strings.Repeat("-", 10) + "  " + strings.Repeat("-", 26) + "  " + strings.Repeat("-", 9) + "  " +
				strings.Repeat("-", 16) + "\n" +
				"2024-03-01  2026-10-22 14:30:05 +02:00  1250000.5  EUR\n" +
				"1 row\n"},
		{"values", "VALUES (1, 'a'), (2, 'b')",
			"column1  column2\n" +
				"-------  -------\n" +
				"      1  a\n" +
				"      2  b\n" +
				"2 rows\n"},
		{"with, comments, and a semicolon", "-- all\n/* the shops */ WITH s AS (SELECT id FROM shop) SELECT count(*) AS n FROM s ; -- done",
			"n\n-\n2\n1 row\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, ds := e.Query("q", tt.query)
			if len(ds) > 0 {
				t.Fatalf("Query: %v", ds)
			}
			var b bytes.Buffer
			if err := r.Table(&b); err != nil {
				t.Fatal(err)
			}
			if b.String() != tt.want {
				t.Errorf("got\n%swant\n%s", b.String(), tt.want)
			}
		})
	}
}

// TestQuerySemicolons checks that a semicolon in a string, a quoted name or a comment ends no statement.
func TestQuerySemicolons(t *testing.T) {
	e := catalogExport(t)
	for _, q := range []string{
		"SELECT ';' AS \"a;b\", 'it''s;' AS c, 1 AS [d;e] /* ; */",
		"SELECT 1 AS `a;b` -- ; no more",
		"SELECT 1;",
		"SELECT 1 /* never closed ;",
	} {
		if r, ds := e.Query("q", q); len(ds) > 0 || len(r.Rows) != 1 {
			t.Errorf("Query(%q) = %v, %v; want one row", q, r, ds)
		}
	}
}

func TestQueryDiagnostics(t *testing.T) {
	e := catalogExport(t)
	tests := []struct{ query, want string }{
		{"", "q:1:1: error: the query is empty"},
		{" -- nothing\n", "q:1:1: error: the query is empty"},
		{";", "q:1:1: error: a query only reads: write SELECT or WITH, not \";\""},
		{"DELETE FROM shop", "q:1:1: error: a query only reads: write SELECT or WITH, not \"DELETE\""},
		{"/* c */\n  insert into shop (id) values ('x')", "q:2:3: error: a query only reads: write SELECT or WITH, not \"insert\""},
		{"PRAGMA query_only = OFF", "q:1:1: error: a query only reads: write SELECT or WITH, not \"PRAGMA\""},
		{"ATTACH 'x.db' AS x", "q:1:1: error: a query only reads: write SELECT or WITH, not \"ATTACH\""},
		{"(SELECT 1)", "q:1:1: error: a query only reads: write SELECT or WITH, not \"(\""},
		{"WITH x AS (SELECT 1) DELETE FROM shop", "q:1:1: error: a query only reads: this statement writes"},
		{"SELECT 1; SELECT 2", "q:1:11: error: a query is one statement; remove what follows the first `;`"},
		{"SELECT 1;\n-- fine\nSELECT 2;", "q:3:1: error: a query is one statement; remove what follows the first `;`"},
		{"SELECT nme FROM shop", "q:1:8: error: no such column `nme`; did you mean `name`?"},
		{"SELECT 1,\n  nme FROM shop", "q:2:3: error: no such column `nme`; did you mean `name`?"},
		{"SELECT s.nme FROM shop s", "q:1:8: error: no such column `s.nme`; did you mean `s.name`?"},
		{"SELECT x FROM shop", "q:1:8: error: no such column `x`"},
		{"SELECT * FROM shp", "q:1:15: error: no such table `shp`; did you mean `shop`?"},
		{"SELECT * FROM nothing_like_it", "q:1:15: error: no such table `nothing_like_it`"},
		{"SELECT nope(1)", "q:1:8: error: no such function `nope`"},
		{"SELECT id FROM shop, person", "q:1:8: error: ambiguous column name `id`; write the table before it, as in `table.id`"},
		{"SELECT FROM shop", "q:1:8: error: syntax error near `FROM`"},
		{"SELECT * FROM select", "q:1:15: error: syntax error near `select`"},
		{"SELECT * FROM shop WHERE", "q:1:25: error: the query is incomplete"},
		{"SELECT * FROM shop WHERE ", "q:1:25: error: the query is incomplete"},
		{"SELECT * FROM \"order\" WHERE order = 1", "q:1:29: error: `order` is a table here but also a SQL keyword; write \"order\" with double quotes"},
		{"SELECT abs(-9223372036854775808)", "q:1:1: error: integer overflow"},
		{"SELECT 'abc", "q:1:1: error: unrecognized token: \"'abc\""},
	}
	for _, tt := range tests {
		r, ds := e.Query("q", tt.query)
		if r != nil || len(ds) != 1 || ds[0].String() != tt.want {
			t.Errorf("Query(%q) = %v, %v\nwant %s", tt.query, r, ds, tt.want)
		}
	}
}

// TestQueryFails checks that a query reports an export that SQLite refuses, at the whole source.
func TestQueryFails(t *testing.T) {
	e := catalogExport(t)
	e.rows = append(e.rows, e.rows[0])
	r, ds := e.Query("q", "SELECT 1")
	want := "q: error: constraint failed: UNIQUE constraint failed: shop.id"
	if r != nil || len(ds) != 1 || ds[0].String() != want {
		t.Errorf("Query = %v, %v\nwant %s", r, ds, want)
	}
}

func TestFind(t *testing.T) {
	for _, tt := range []struct {
		q, name string
		want    int
	}{
		{"SELECT idx, id", "id", 12},
		{"SELECT ID", "id", 7},
		{"SELECT 1", "zzz", 0},
		{"  SELECT 1", "zzz", 2},
		{"SELECT xid, id_", "id", 0},
		{"SELECT 'id', \"id\", /* id */ id", "id", 28},
		{"SELECT * FROM select", "select", 14},
		{"SELECT a)", ")", 8},
	} {
		if got := find(tt.q, tt.name); got != tt.want {
			t.Errorf("find(%q, %q) = %d, want %d", tt.q, tt.name, got, tt.want)
		}
	}
}

func TestTable(t *testing.T) {
	tests := []struct {
		name string
		r    *Result
		want string
	}{
		{"aligned by type", &Result{[]string{"name", "qty", "price"}, [][]any{
			{"Café", int64(12), 1250.5}, {"Crème brûlée", int64(3), 20.0}}},
			"name          qty   price\n" +
				"------------  ---  ------\n" +
				"Café           12  1250.5\n" +
				"Crème brûlée    3      20\n" +
				"2 rows\n"},
		{"values of every kind", &Result{[]string{"v"}, [][]any{
			{nil}, {"a\nb\t\r"}, {[]byte{0}}, {[]byte{1, 2}}}},
			"v\n" +
				"--------------\n" +
				"NULL\n" +
				"a\\nb\\t\\r\n" +
				"<blob 1 byte>\n" +
				"<blob 2 bytes>\n" +
				"4 rows\n"},
		{"floats are not in exponent form", &Result{[]string{"f"}, [][]any{{1e20}, {math.Nextafter(0.3, 1)}}},
			strings.Repeat(" ", 20) + "f\n" +
				strings.Repeat("-", 21) + "\n" +
				"100000000000000000000\n" +
				"  0.30000000000000004\n" +
				"2 rows\n"},
		{"no rows", &Result{[]string{"a", "b"}, nil}, "a  b\n-  -\n0 rows\n"},
		{"one row", &Result{[]string{"a"}, [][]any{{int64(1)}}}, "a\n-\n1\n1 row\n"},
	}
	for _, tt := range tests {
		var b bytes.Buffer
		if err := tt.r.Table(&b); err != nil {
			t.Fatal(err)
		}
		if b.String() != tt.want {
			t.Errorf("%s: got\n%swant\n%s", tt.name, b.String(), tt.want)
		}
	}
}

func TestCSV(t *testing.T) {
	r := &Result{[]string{"a", "b"}, [][]any{{"x,y", nil}, {int64(1), "q\"r"}, {2.5, []byte{1}}}}
	var b bytes.Buffer
	if err := r.CSV(&b); err != nil {
		t.Fatal(err)
	}
	want := "a,b\n\"x,y\",\n1,\"q\"\"r\"\n2.5,<blob 1 byte>\n"
	if b.String() != want {
		t.Errorf("got %q, want %q", b.String(), want)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestResultWriteErrors(t *testing.T) {
	r := &Result{[]string{"a"}, [][]any{{"x"}}}
	if err := r.Table(failingWriter{}); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("Table error = %v", err)
	}
	if err := r.CSV(failingWriter{}); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("CSV error = %v", err)
	}
}
