package sqlexport

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unsafe"

	"modernc.org/libc"
	_ "modernc.org/sqlite"
	sqlite "modernc.org/sqlite/lib"

	"github.com/bn-k/kanon/internal/schema"
)

// lay parses and checks src, which must be valid, and lays it out.
func lay(t *testing.T, src string) (*Layout, []string) {
	t.Helper()
	f, diags := schema.Parse("t.kanonarch", src)
	if len(diags) == 0 {
		diags = schema.Check(f)
	}
	if len(diags) > 0 {
		t.Fatalf("schema: %v", diags)
	}
	l, diags := NewLayout(f)
	var ds []string
	for _, d := range diags {
		ds = append(ds, fmt.Sprintf("%d:%d: %s", d.Pos.Line, d.Pos.Col, d.Msg))
	}
	return l, ds
}

// open opens a new database in memory with its foreign keys checked.
func open(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	return db
}

// load creates the tables of ddl in a new database.
func load(t *testing.T, ddl string) *sql.DB {
	t.Helper()
	db := open(t)
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("the DDL doesn't load: %v", err)
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_schema WHERE type = 'table'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if want := strings.Count(ddl, "CREATE TABLE "); n != want {
		t.Errorf("%d tables loaded, want %d", n, want)
	}
	return db
}

func compare(t *testing.T, got, want string) {
	t.Helper()
	if got == want {
		return
	}
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := range max(len(g), len(w)) {
		var gl, wl string
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if gl != wl {
			t.Errorf("line %d:\n got %q\nwant %q", i+1, gl, wl)
			return
		}
	}
}

func TestDDL(t *testing.T) {
	tests := []struct{ schema, golden string }{
		{"testdata/shop.kanonarch", "testdata/shop.sql"},
		{"../../cases/northwind/schema.kanonarch", "testdata/northwind.sql"},
	}
	for _, tt := range tests {
		t.Run(filepath.Base(tt.golden), func(t *testing.T) {
			f, diags := schema.Load(tt.schema)
			if len(diags) > 0 {
				t.Fatalf("schema: %v", diags)
			}
			want, err := os.ReadFile(tt.golden)
			if err != nil {
				t.Fatal(err)
			}
			l, diags := NewLayout(f)
			if len(diags) > 0 {
				t.Fatalf("NewLayout: %v", diags)
			}
			got := l.DDL()
			compare(t, got, string(want))
			load(t, got)
		})
	}
}

// TestConstraints checks that the DDL keeps what Kanon checks: foreign keys at the end of the transaction, the
// values of a Bool and of an enum.
func TestConstraints(t *testing.T) {
	l, diags := lay(t, `
enum Size { small, large }
schema Box {
  open:   Bool
  size:   Size
  parent: -> Box?
}`)
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	db := load(t, l.DDL())
	tests := []struct {
		stmts []string
		err   string
	}{
		{[]string{"INSERT INTO box VALUES ('a', 1, 'small', 'b')", "INSERT INTO box VALUES ('b', 0, 'large', NULL)"}, ""},
		{[]string{"INSERT INTO box VALUES ('d', 2, 'small', NULL)"}, "CHECK constraint failed: open IN (0, 1)"},
		{[]string{"INSERT INTO box VALUES ('e', 1, 'medium', NULL)"}, "CHECK constraint failed: size IN ('small', 'large')"},
		{[]string{"INSERT INTO box VALUES ('f', NULL, 'small', NULL)"}, "NOT NULL constraint failed: box.open"},
		// Last: a failed COMMIT may leave the transaction open.
		{[]string{"INSERT INTO box VALUES ('c', 1, 'small', 'z')"}, "FOREIGN KEY constraint failed"},
	}
	for _, tt := range tests {
		tx, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range tt.stmts {
			if _, err = tx.Exec(s); err != nil {
				break
			}
		}
		if err == nil {
			err = tx.Commit()
		} else {
			tx.Rollback()
		}
		if got := fmt.Sprint(err); tt.err == "" && err != nil || tt.err != "" && !strings.Contains(got, tt.err) {
			t.Errorf("%v: got %v, want %q", tt.stmts, err, tt.err)
		}
	}
}

// TestLayouts covers the shapes of schemas the golden files don't: who is weak, and which parent columns a table
// gets.
func TestLayouts(t *testing.T) {
	tests := []struct{ name, src, want string }{
		{"cycle", "schema A { bs: [B]  cs: [C] }\nschema B { as: [A] }\nschema C { }", `CREATE TABLE a (
  id   TEXT NOT NULL,
  b_id TEXT REFERENCES b (id) DEFERRABLE INITIALLY DEFERRED,
  PRIMARY KEY (id)
);

CREATE TABLE b (
  id   TEXT NOT NULL,
  a_id TEXT REFERENCES a (id) DEFERRABLE INITIALLY DEFERRED,
  PRIMARY KEY (id)
);

CREATE TABLE c (
  a_id TEXT NOT NULL REFERENCES a (id) DEFERRABLE INITIALLY DEFERRED,
  id   TEXT NOT NULL,
  PRIMARY KEY (a_id, id)
);
`},
		{"self", "schema S { children: [S] }", `CREATE TABLE s (
  id   TEXT NOT NULL,
  s_id TEXT REFERENCES s (id) DEFERRABLE INITIALLY DEFERRED,
  PRIMARY KEY (id)
);
`},
		{"held twice", "schema A { b: B?  bs: [B] }\nschema B { x: Int }\nschema C { name: Text  bs: [B] }", `CREATE TABLE a (
  id   TEXT NOT NULL,
  b_id TEXT REFERENCES b (id) DEFERRABLE INITIALLY DEFERRED,
  PRIMARY KEY (id)
);

CREATE TABLE b (
  id   TEXT NOT NULL,
  a_id TEXT REFERENCES a (id) DEFERRABLE INITIALLY DEFERRED,
  c_id TEXT REFERENCES c (id) DEFERRABLE INITIALLY DEFERRED,
  x    INTEGER NOT NULL,
  PRIMARY KEY (id)
);

CREATE TABLE c (
  id   TEXT NOT NULL,
  name TEXT NOT NULL,
  PRIMARY KEY (id)
);
`},
		{"weak in a weak schema, linked to", "schema A { bs: [B] }\nschema B { cs: [C] }\nschema C { tags: [Text]  to: [-> A] }",
			`CREATE TABLE a (
  id TEXT NOT NULL,
  PRIMARY KEY (id)
);

CREATE TABLE b (
  a_id TEXT NOT NULL REFERENCES a (id) DEFERRABLE INITIALLY DEFERRED,
  id   TEXT NOT NULL,
  PRIMARY KEY (a_id, id)
);

CREATE TABLE c (
  a_id TEXT NOT NULL,
  b_id TEXT NOT NULL,
  id   TEXT NOT NULL,
  PRIMARY KEY (a_id, b_id, id),
  FOREIGN KEY (a_id, b_id) REFERENCES b (a_id, id) DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE c_tags (
  a_id  TEXT NOT NULL,
  b_id  TEXT NOT NULL,
  c_id  TEXT NOT NULL,
  value TEXT NOT NULL,
  FOREIGN KEY (a_id, b_id, c_id) REFERENCES c (a_id, b_id, id) DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE c_to (
  a_id  TEXT NOT NULL,
  b_id  TEXT NOT NULL,
  c_id  TEXT NOT NULL,
  to_id TEXT NOT NULL REFERENCES a (id) DEFERRABLE INITIALLY DEFERRED,
  PRIMARY KEY (a_id, b_id, c_id, to_id),
  FOREIGN KEY (a_id, b_id, c_id) REFERENCES c (a_id, b_id, id) DEFERRABLE INITIALLY DEFERRED
);
`},
		{"keywords", "schema Group { select: Int  from: Text?  to: -> Group? }", `CREATE TABLE "group" (
  id       TEXT NOT NULL,
  "select" INTEGER NOT NULL,
  "from"   TEXT,
  to_id    TEXT REFERENCES "group" (id) DEFERRABLE INITIALLY DEFERRED,
  PRIMARY KEY (id)
);
`},
		{"no table", "schema Empty { }\nschema Items { all: [Item]  n: Int = count(all) }\nschema Item { }", `CREATE TABLE empty (
  id TEXT NOT NULL,
  PRIMARY KEY (id)
);

CREATE TABLE item (
  id TEXT NOT NULL,
  PRIMARY KEY (id)
);
`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, diags := lay(t, tt.src)
			if len(diags) > 0 {
				t.Fatal(diags)
			}
			compare(t, l.DDL(), tt.want)
			load(t, l.DDL())
		})
	}
}

func TestClashes(t *testing.T) {
	tests := []struct {
		src  string
		want []string
	}{
		{"schema Item { price: Money  price-currency: Text }", []string{
			"1:29: the currency of `price` and `price-currency` both give the column `price_currency` of the " +
				"table `item`; rename `price-currency`",
		}},
		{"schema Item { area-unit: Text  area: Quantity }", []string{
			"1:32: `area-unit` and the unit of `area` both give the column `area_unit` of the table `item`; " +
				"rename `area`",
		}},
		{"schema Item { owner: -> Item  owner-id: Text }", []string{
			"1:31: `owner` and `owner-id` both give the column `owner_id` of the table `item`; rename `owner-id`",
		}},
		{"schema S { items: [I] }\nschema I { s: -> S  s-items: -> S }", []string{
			"2:21: the parent `S.items` and `s-items` both give the column `s_items_id` of the table `i`; " +
				"rename `s-items`",
		}},
		{"schema A { bs: [B] }\nschema B { cs: [C] }\nschema C { a: -> A }", []string{
			"3:12: the parent `A.bs` and `a` both give the column `a_id` of the table `c`; rename `a`",
		}},
		{"schema P { p: [-> P] }", []string{
			"1:12: the parent `P` and the links of `p` both give the column `p_id` of the table `p_p`; rename `p`",
		}},
		{"schema Shop { tags: [Text] }\nschema ShopTags { }", []string{
			"1:15: the schema `ShopTags` and `Shop.tags` both give the table `shop_tags`; rename `tags`",
		}},
		{"schema HTMLPage { }\nschema HtmlPage { }", []string{
			"2:8: the schema `HTMLPage` and the schema `HtmlPage` both give the table `html_page`; rename `HtmlPage`",
		}},
		{"schema SqliteStat { }", []string{
			"1:8: the schema `SqliteStat` gives the table `sqlite_stat`, a name SQLite keeps for itself; rename " +
				"`SqliteStat`",
		}},
	}
	for _, tt := range tests {
		l, got := lay(t, tt.src)
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s:\n got %q\nwant %q", tt.src, got, tt.want)
		}
		if l != nil {
			t.Errorf("%s: a layout despite the clashes", tt.src)
		}
	}
}

func TestSnake(t *testing.T) {
	tests := map[string]string{
		"EtatDesLieux":   "etat_des_lieux",
		"HTMLPage":       "html_page",
		"Page2":          "page2",
		"Page2Bis":       "page2_bis",
		"ABC":            "abc",
		"A":              "a",
		"prix-unitaire":  "prix_unitaire",
		"controle-2026":  "controle_2026",
		"OrderDetail":    "order_detail",
		"CustomerDemo":   "customer_demo",
		"IOStream":       "io_stream",
		"TVAIntra":       "tva_intra",
		"Version3D":      "version3_d",
		"customer-desc":  "customer_desc",
		"reports-to-id":  "reports_to_id",
		"x":              "x",
		"a-b-c":          "a_b_c",
		"MaisonCaluire2": "maison_caluire2",
	}
	for in, want := range tests {
		if got := snake(in); got != want {
			t.Errorf("snake(%q) = %q, want %q", in, got, want)
		}
	}
}

// The keyword SQLite gives back: global, so that they never move while SQLite writes them.
var (
	keywordName uintptr
	keywordLen  int32
)

// TestKeywords checks the keywords quoted against those of the SQLite linked in.
func TestKeywords(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	var want []string
	for i := range sqlite.Xsqlite3_keyword_count(tls) {
		sqlite.Xsqlite3_keyword_name(tls, i, uintptr(unsafe.Pointer(&keywordName)), uintptr(unsafe.Pointer(&keywordLen)))
		want = append(want, strings.ToLower(string(libc.GoBytes(keywordName, int(keywordLen)))))
	}
	slices.Sort(want)
	got := slices.Sorted(func(yield func(string) bool) {
		for k := range keywords {
			if !yield(k) {
				return
			}
		}
	})
	if !slices.Equal(got, want) {
		t.Errorf("keywords:\n got %q\nwant %q", got, want)
	}
	if ident("order") != `"order"` || ident("orders") != "orders" {
		t.Errorf(`ident("order") = %s, ident("orders") = %s`, ident("order"), ident("orders"))
	}
}
