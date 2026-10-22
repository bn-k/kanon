package sqlexport

import (
	"database/sql"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/func-it/kanon/internal/sqlimport"
)

// nbsp is U+00A0, the non-breaking space, written so that the source stays ASCII.
var nbsp = string(rune(0xa0))

// TestRoundTrip exports the Northwind case to a database, imports that database and checks that it gives back the
// case byte for byte, its 28 non-breaking spaces included. It then reads the source of the case and the database,
// table by table, and checks that every value is the one the source holds, but for what kan import changes on the
// way: keys lowercased to make ids, Windows line breaks, the spaces around a text, and an empty text, which is no
// value.
func TestRoundTrip(t *testing.T) {
	const cases = "../../cases/northwind"
	tmp := t.TempDir()
	e := export(t, doc(t, cases+"/schema.kanonarch", cases+"/northwind.kan"))
	path := filepath.Join(tmp, "northwind.sqlite")
	if err := e.Database(path); err != nil {
		t.Fatal(err)
	}
	back := filepath.Join(tmp, "back", "northwind")
	if _, err := sqlimport.Import(path, back); err != nil {
		t.Fatal(err)
	}
	want, got := files(t, cases, "source", "expected.txt"), files(t, back)
	n := 0
	for name, w := range want {
		if strings.HasSuffix(name, ".kan") {
			n += strings.Count(w, nbsp)
		}
	}
	if n != 28 {
		t.Errorf("the case holds %d non-breaking spaces, want 28", n)
	}
	for name, w := range want {
		if g, ok := got[name]; !ok {
			t.Errorf("%s is not imported back", name)
		} else if g != w {
			gl, wl := strings.Split(g, "\n"), strings.Split(w, "\n")
			i := 0
			for i < min(len(gl), len(wl))-1 && gl[i] == wl[i] {
				i++
			}
			t.Errorf("%s:%d is imported back as %q, want %q", name, i+1, gl[i], wl[i])
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s is imported back but not in the case", name)
		}
	}

	src := open(t)
	script, err := os.ReadFile(cases + "/source/create.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.Exec(string(script)); err != nil {
		t.Fatal(err)
	}
	dst := openFile(t, path)
	for _, diff := range compareData(t, src, dst) {
		t.Error(diff)
	}

	if _, err := dst.Exec("UPDATE customer SET city = 'Lyon' WHERE id = 'alfki'; DELETE FROM shipper WHERE id = '3'"); err != nil {
		t.Fatal(err)
	}
	diffs := compareData(t, src, dst)
	wantDiffs := []string{
		"Shippers: 3 rows, shipper: 2",
		"Shippers: no row 3 in shipper",
		`customer.city: row alfki: "Lyon", want "Berlin" (1 values differ)`,
	}
	if !slices.Equal(diffs, wantDiffs) {
		t.Errorf("compareData after a change gives\n%s\nwant\n%s", strings.Join(diffs, "\n"), strings.Join(wantDiffs, "\n"))
	}
}

// files reads the files of a tree, by their path in it, but for the names in skip at its top.
func files(t *testing.T, root string, skip ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		switch {
		case slices.Contains(skip, rel) && d.IsDir():
			return filepath.SkipDir
		case slices.Contains(skip, rel), d.IsDir():
			return nil
		}
		b, err := os.ReadFile(path)
		out[filepath.ToSlash(rel)] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func openFile(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

// northwind maps the tables of the source of the Northwind case to the tables kan sql writes, and the columns whose
// name is not their name in snake_case: the keys, which are ids, and the foreign keys without an ID suffix. The
// product of an order detail is both its id and a link.
var northwind = []struct {
	src, dst string
	cols     map[string][]string
}{
	{"Categories", "category", map[string][]string{"CategoryID": {"id"}}},
	{"CustomerCustomerDemo", "customer_customer_demographics",
		map[string][]string{"CustomerTypeID": {"customer_demographic_id"}}},
	{"CustomerDemographics", "customer_demographic", map[string][]string{"CustomerTypeID": {"id"}}},
	{"Customers", "customer", map[string][]string{"CustomerID": {"id"}}},
	{"Employees", "employee", map[string][]string{"EmployeeID": {"id"}, "ReportsTo": {"reports_to_id"}}},
	{"EmployeeTerritories", "employee_territories", nil},
	{"Order Details", "order_detail", map[string][]string{"ProductID": {"id", "product_id"}}},
	{"Orders", "order", map[string][]string{"OrderID": {"id"}, "ShipVia": {"ship_via_id"}}},
	{"Products", "product", map[string][]string{"ProductID": {"id"}}},
	{"Regions", "region", map[string][]string{"RegionID": {"id"}}},
	{"Shippers", "shipper", map[string][]string{"ShipperID": {"id"}}},
	{"Suppliers", "supplier", map[string][]string{"SupplierID": {"id"}}},
	{"Territories", "territory", map[string][]string{"TerritoryID": {"id"}}},
}

var camel = regexp.MustCompile(`([a-z])([A-Z])`)

// compareData reads every table of src and its table in dst, and gives the values that differ, by column, with the
// first row where each column differs.
func compareData(t *testing.T, src, dst *sql.DB) []string {
	t.Helper()
	var diffs []string
	for _, m := range northwind {
		srcCols, srcRows := rows(t, src, m.src)
		dstCols, dstRows := rows(t, dst, m.dst)
		if len(srcRows) != len(dstRows) {
			diffs = append(diffs, fmt.Sprintf("%s: %d rows, %s: %d", m.src, len(srcRows), m.dst, len(dstRows)))
		}
		// to[i] are the columns of dst that hold the column i of src.
		to := make([][]int, len(srcCols))
		mapped := map[int]bool{}
		for i, c := range srcCols {
			names := m.cols[c]
			if names == nil {
				names = []string{strings.ToLower(camel.ReplaceAllString(strings.ReplaceAll(c, "ID", "Id"), "${1}_$2"))}
			}
			for _, n := range names {
				j := slices.Index(dstCols, n)
				if j < 0 {
					diffs = append(diffs, fmt.Sprintf("%s.%s: no column %s in %s", m.src, c, n, m.dst))
					continue
				}
				to[i] = append(to[i], j)
				mapped[j] = true
			}
		}
		for j, c := range dstCols {
			if !mapped[j] {
				diffs = append(diffs, fmt.Sprintf("%s.%s: no column of %s", m.dst, c, m.src))
			}
		}
		var keyCols []int
		for _, c := range primaryKey(t, dst, m.dst) {
			keyCols = append(keyCols, slices.Index(dstCols, c))
		}
		byKey := map[string][]any{}
		for _, r := range dstRows {
			byKey[key(r, keyCols)] = r
		}
		first := map[string]string{}
		count := map[string]int{}
		for _, r := range srcRows {
			want := make([]any, len(dstCols))
			for i, js := range to {
				for _, j := range js {
					want[j] = imported(r[i], dstCols[j] == "id" || strings.HasSuffix(dstCols[j], "_id"))
				}
			}
			k := key(want, keyCols)
			got, ok := byKey[k]
			if !ok {
				diffs = append(diffs, fmt.Sprintf("%s: no row %s in %s", m.src, k, m.dst))
				continue
			}
			for j, c := range dstCols {
				if !same(want[j], got[j]) {
					col := m.dst + "." + c
					if count[col] == 0 {
						first[col] = fmt.Sprintf("%s: row %s: %#v, want %#v", col, k, got[j], want[j])
					}
					count[col]++
				}
			}
		}
		for col, n := range count {
			diffs = append(diffs, fmt.Sprintf("%s (%d values differ)", first[col], n))
		}
	}
	var n int
	if err := dst.QueryRow("SELECT count(*) FROM sqlite_schema WHERE type = 'table'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != len(northwind) {
		diffs = append(diffs, fmt.Sprintf("the database holds %d tables, want %d", n, len(northwind)))
	}
	sort.Strings(diffs)
	return diffs
}

// primaryKey gives the columns of the primary key of a table, in their order in the key.
func primaryKey(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	rs, err := db.Query("SELECT name FROM pragma_table_info(?) WHERE pk > 0 ORDER BY pk", table)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close()
	var cols []string
	for rs.Next() {
		var c string
		if err := rs.Scan(&c); err != nil {
			t.Fatal(err)
		}
		cols = append(cols, c)
	}
	return cols
}

// rows reads the columns and every row of a table.
func rows(t *testing.T, db *sql.DB, table string) ([]string, [][]any) {
	t.Helper()
	rs, err := db.Query(`SELECT * FROM "` + table + `"`)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close()
	cols, _ := rs.Columns()
	var all [][]any
	for rs.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		all = append(all, vals)
	}
	if err := rs.Err(); err != nil {
		t.Fatal(err)
	}
	return cols, all
}

func key(r []any, cols []int) string {
	var parts []string
	for _, j := range cols {
		parts = append(parts, fmt.Sprint(r[j]))
	}
	return strings.Join(parts, "/")
}

var blankLines = regexp.MustCompile(`\n{3,}`)

// imported is v as kan import writes it, then kan sql: a key lowercased into an id, a text without Windows line
// breaks, spaces at its ends and at the ends of its lines, or extra blank lines, and no value for an empty text.
func imported(v any, id bool) any {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		x = strings.ReplaceAll(x, "\r\n", "\n")
		lines := strings.Split(x, "\n")
		for i, l := range lines {
			lines[i] = strings.TrimRight(l, " \t")
		}
		x = strings.TrimSpace(blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
		if x == "" {
			return nil
		}
		if id {
			return strings.ToLower(x)
		}
		return x
	}
	if id {
		return fmt.Sprint(v)
	}
	return v
}

// same tells whether two values of SQLite are the same: numbers by their value, whatever their storage class.
func same(want, got any) bool {
	if w, ok := number(want); ok {
		g, ok := number(got)
		return ok && w.Cmp(g) == 0
	}
	switch w := want.(type) {
	case time.Time:
		g, ok := got.(time.Time)
		return ok && w.Equal(g)
	case []byte:
		g, ok := got.([]byte)
		return ok && string(w) == string(g)
	}
	return want == got
}

func number(v any) (*big.Rat, bool) {
	switch x := v.(type) {
	case int64:
		return new(big.Rat).SetInt64(x), true
	case float64:
		return new(big.Rat).SetString(strconv.FormatFloat(x, 'g', -1, 64))
	}
	return nil, false
}
