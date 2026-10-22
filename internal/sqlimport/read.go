package sqlimport

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	_ "modernc.org/sqlite"
)

// A table is what kan import reads of a table: its columns, keys and rows.
type table struct {
	name    string
	sql     string // the CREATE TABLE statement
	noRowid bool   // WITHOUT ROWID
	cols    []*column
	fks     []*foreignKey
	unique  [][]int // the columns of each unique index, the primary key's included
	rows    [][]any // int64, float64, string, []byte or nil, in column order
	rowids  []int64 // the rowid of each row, for a table without primary key
}

type column struct {
	name, decl string
	notNull    bool
	pk         int // its position in the primary key, from 1, or 0
}

// A foreignKey links columns of a table to the primary key of another.
type foreignKey struct {
	table *table
	refer string   // the table referred to, as written
	from  []int    // the columns of table
	to    []string // the columns of refer, or none for its primary key
	// Once resolved:
	target *tbl
	key    []int // for each column of from, its column in target
}

const header = "SQLite format 3\x00"

// open opens a SQLite database read-only, or runs a .sql script into a database in memory.
func open(src string) (*sql.DB, error) {
	if strings.EqualFold(filepath.Ext(src), ".sql") {
		script, err := os.ReadFile(src)
		if err != nil {
			return nil, err
		}
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(1) // one connection, one database in memory
		if _, err := db.Exec(string(script)); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s: %v", src, err)
		}
		return db, nil
	}
	f, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	head := make([]byte, len(header))
	_, err = io.ReadFull(f, head)
	f.Close()
	if err != nil || !bytes.Equal(head, []byte(header)) {
		return nil, fmt.Errorf("%s is not a SQLite database nor a .sql script", src)
	}
	abs, err := filepath.Abs(src)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: abs, RawQuery: "mode=ro"}
	return sql.Open("sqlite", u.String())
}

// read reads the tables of a database, in the order of their names. It notes the views and virtual tables, which
// it leaves out.
func read(db *sql.DB) ([]*table, []string, error) {
	type entry struct {
		name, kind string
		noRowid    bool
	}
	var entries []entry
	err := query(db, `SELECT name, type, wr FROM pragma_table_list WHERE schema = 'main' ORDER BY name`, nil,
		func(scan func(...any) error) error {
			var e entry
			err := scan(&e.name, &e.kind, &e.noRowid)
			entries = append(entries, e)
			return err
		})
	if err != nil {
		return nil, nil, err
	}
	var tables []*table
	var notes []string
	for _, e := range entries {
		switch {
		case strings.HasPrefix(e.name, "sqlite_"), e.kind == "shadow":
		case e.kind == "view":
			notes = append(notes, fmt.Sprintf("the view `%s` is not imported", e.name))
		case e.kind == "virtual":
			notes = append(notes, fmt.Sprintf("the virtual table `%s` is not imported", e.name))
		default:
			t, err := readTable(db, e.name, e.noRowid)
			if err != nil {
				return nil, nil, err
			}
			tables = append(tables, t)
		}
	}
	return tables, notes, nil
}

func readTable(db *sql.DB, name string, noRowid bool) (*table, error) {
	t := &table{name: name, noRowid: noRowid}
	err := query(db, `SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = ?`, []any{name},
		func(scan func(...any) error) error { return scan(&t.sql) })
	if err != nil {
		return nil, err
	}
	err = query(db, `SELECT name, type, "notnull", pk FROM pragma_table_info(?) ORDER BY cid`, []any{name},
		func(scan func(...any) error) error {
			c := &column{}
			t.cols = append(t.cols, c)
			return scan(&c.name, &c.decl, &c.notNull, &c.pk)
		})
	if err != nil {
		return nil, err
	}
	index := func(col string) int {
		return slices.IndexFunc(t.cols, func(c *column) bool { return strings.EqualFold(c.name, col) })
	}
	byID := map[int]*foreignKey{}
	err = query(db, `SELECT id, "table", "from", "to" FROM pragma_foreign_key_list(?) ORDER BY id, seq`, []any{name},
		func(scan func(...any) error) error {
			var id int
			var target, from string
			var to sql.NullString
			if err := scan(&id, &target, &from, &to); err != nil {
				return err
			}
			fk := byID[id]
			if fk == nil {
				fk = &foreignKey{table: t, refer: target}
				byID[id] = fk
				t.fks = append(t.fks, fk)
			}
			fk.from = append(fk.from, index(from))
			if to.Valid {
				fk.to = append(fk.to, to.String)
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	// pragma_foreign_key_list lists the keys from the last written; order them by their first column instead.
	slices.SortStableFunc(t.fks, func(a, b *foreignKey) int { return a.from[0] - b.from[0] })
	var indexes []string
	err = query(db, `SELECT name FROM pragma_index_list(?) WHERE "unique" AND NOT partial`, []any{name},
		func(scan func(...any) error) error {
			var s string
			indexes = append(indexes, s)
			return scan(&indexes[len(indexes)-1])
		})
	if err != nil {
		return nil, err
	}
	for _, ix := range indexes {
		var cols []int
		err = query(db, `SELECT cid FROM pragma_index_info(?) ORDER BY seqno`, []any{ix},
			func(scan func(...any) error) error {
				var c int
				err := scan(&c)
				cols = append(cols, c)
				return err
			})
		if err != nil {
			return nil, err
		}
		t.unique = append(t.unique, cols)
	}
	if pk := t.key(); len(pk) > 0 {
		t.unique = append(t.unique, pk)
	}
	return t, t.readRows(db)
}

// readRows reads the rows in the order they were written: by rowid, or by primary key for a table WITHOUT
// ROWID. A unary plus on each column keeps the driver from turning DATE and DATETIME values into times.
func (t *table) readRows(db *sql.DB) error {
	var cols []string
	if !t.noRowid {
		cols = append(cols, "rowid")
	}
	for _, c := range t.cols {
		cols = append(cols, "+"+quote(c.name))
	}
	order := "rowid"
	if t.noRowid {
		var key []string
		for _, i := range t.key() {
			key = append(key, quote(t.cols[i].name))
		}
		order = strings.Join(key, ", ")
	}
	q := fmt.Sprintf("SELECT %s FROM %s ORDER BY %s", strings.Join(cols, ", "), quote(t.name), order)
	return query(db, q, nil, func(scan func(...any) error) error {
		row := make([]any, len(t.cols))
		var rowid int64
		var dest []any
		if !t.noRowid {
			dest = append(dest, &rowid)
		}
		for i := range row {
			dest = append(dest, &row[i])
		}
		err := scan(dest...)
		t.rows = append(t.rows, row)
		t.rowids = append(t.rowids, rowid)
		return err
	})
}

// key returns the columns of the primary key, in its order.
func (t *table) key() []int {
	var key []int
	for i, c := range t.cols {
		if c.pk > 0 {
			key = append(key, i)
		}
	}
	slices.SortFunc(key, func(a, b int) int { return t.cols[a].pk - t.cols[b].pk })
	return key
}

// query runs q and calls each for each row.
func query(db *sql.DB, q string, args []any, each func(scan func(...any) error) error) error {
	rows, err := db.Query(q, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := each(rows.Scan); err != nil {
			return err
		}
	}
	return rows.Err()
}

func quote(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }
