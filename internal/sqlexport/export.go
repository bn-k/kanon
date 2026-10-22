package sqlexport

import (
	"bufio"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"os"
	"strconv"
	"strings"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/model"
	"github.com/bn-k/kanon/internal/schema"
	"github.com/bn-k/kanon/internal/value"

	_ "modernc.org/sqlite" // the driver "sqlite"
)

// Export is a document laid out in rows, ready to be written as a SQL script or as a SQLite database.
type Export struct {
	layout *Layout
	rows   []*row
}

// A row of a table: the columns it fills, in the order of the table, and their values. The columns it leaves out
// are NULL.
type row struct {
	table *table
	cols  []*column
	vals  []any // a string for TEXT, a num, or []byte for a BLOB
}

// num is a number as SQL writes it: 1250000.5.
type num string

// exporter lays out the rows of a document.
type exporter struct {
	*Layout
	rows  []*row
	keys  map[*model.Object][]any             // the primary key of each object's row
	seen  map[*table]map[string]*model.Object // the objects of each table, by their key
	diags diag.List
}

// New lays out the rows of d, in document order: each object's row, then the rows of its links and of its arrays
// of values, then the objects it holds. It reports the names of tables and columns that clash, two objects of a
// table with the same key, an object whose key needs the object holding it, and the files it cannot read.
func New(d *model.Doc) (*Export, diag.List) {
	l, ds := NewLayout(d.Schema)
	if l == nil {
		return nil, ds
	}
	x := &exporter{Layout: l, keys: map[*model.Object][]any{}, seen: map[*table]map[string]*model.Object{}}
	for o := range d.Objects() {
		x.object(o)
	}
	if len(x.diags) > 0 {
		x.diags.Sort()
		return nil, x.diags
	}
	return &Export{l, x.rows}, nil
}

// object adds the row of o, then those of its links and arrays of values.
func (x *exporter) object(o *model.Object) {
	t := x.schemas[o.Schema]
	if t == nil {
		return // a collection
	}
	vals := map[*column]any{}
	if h := o.Holder; h != nil {
		for i, c := range x.parents[h] {
			vals[c] = x.keys[o.Parent][i]
		}
	}
	if t.weak && o.Parent == nil {
		msg := fmt.Sprintf("`%s` rows are keyed by the `%s` that holds them, by `%s`: export the document that holds "+
			"this one", o.Schema.Name, t.parent.Name, t.holder.Name)
		x.diags.Errorf(o.Pos, "%s", msg)
		return
	}
	var key []any
	if t.weak {
		key = append(key, x.keys[o.Parent]...)
	}
	key = append(key, o.ID)
	x.keys[o] = key
	x.unique(t, o, key)
	vals[t.key[len(t.key)-1]] = o.ID
	var subs []*row
	for _, f := range o.Fields {
		fl := f.Field
		switch sub := x.fields[fl]; {
		case fl.Computed():
		case sub != nil:
			for i := range count(f) {
				sv := map[*column]any{}
				for j, c := range sub.cols[:len(key)] {
					sv[c] = key[j]
				}
				x.fill(sv, sub.values[fl], fl.Type, f, i)
				subs = append(subs, sub.row(sv))
			}
		case t.values[fl] != nil:
			x.fill(vals, t.values[fl], fl.Type, f, 0)
		}
	}
	x.rows = append(append(x.rows, t.row(vals)), subs...)
}

// unique reports o when another object of t has its key.
func (x *exporter) unique(t *table, o *model.Object, key []any) {
	var parts []string
	for _, k := range key {
		parts = append(parts, k.(string))
	}
	k := strings.Join(parts, "\x00")
	if x.seen[t] == nil {
		x.seen[t] = map[string]*model.Object{}
	}
	prev := x.seen[t][k]
	if prev == nil {
		x.seen[t][k] = o
		return
	}
	where := ", at " + prev.Pos.String()
	if prev.Pos.Line == 0 {
		where = ", as " + prev.File
	}
	msg := fmt.Sprintf("the id `%s` is already used by a `%s`%s; in SQL each `%s` needs an id of its own", o.ID,
		o.Schema.Name, where, o.Schema.Name)
	x.diags.Errorf(o.Pos, "%s", msg)
}

// count is the number of items of f: links, objects, files or values.
func count(f *model.Field) int {
	switch typ := f.Field.Type; {
	case typ.Link:
		return len(f.Links)
	case typ.Kind == value.File || typ.Kind == value.Image:
		return len(f.Files)
	}
	return len(f.Values)
}

// fill sets the columns cs of the item i of f, of type typ.
func (x *exporter) fill(vals map[*column]any, cs []*column, typ *schema.Type, f *model.Field, i int) {
	switch {
	case typ.Link:
		vals[cs[0]] = f.Links[i].ID
	case typ.Schema != nil:
		vals[cs[0]] = f.Objects[i].ID
	case typ.Enum != nil:
		vals[cs[0]] = f.Values[i].Raw
	case typ.Kind == value.File || typ.Kind == value.Image:
		b, err := os.ReadFile(f.Files[i])
		if err != nil {
			msg := fmt.Sprintf("cannot read %s: %v", f.Files[i], errors.Unwrap(err))
			x.diags.Errorf(f.Pos, "%s", msg)
		}
		vals[cs[0]] = b
	case typ.Kind == value.Code || typ.Kind == value.Prose && !typ.Array:
		vals[cs[0]] = f.Text
	default:
		v := f.Values[i]
		switch v.Kind {
		case value.Int, value.Number, value.Percent, value.Money, value.Quantity:
			vals[cs[0]] = num(decimal(v.Num))
			if v.Unit != "" {
				vals[cs[1]] = v.Unit
			}
		case value.Bool:
			vals[cs[0]] = num("0")
			if v.Bool {
				vals[cs[0]] = num("1")
			}
		default:
			vals[cs[0]] = v.Raw
		}
	}
}

// row gives the row of t that sets vals.
func (t *table) row(vals map[*column]any) *row {
	r := &row{table: t}
	for _, c := range t.cols {
		if v, ok := vals[c]; ok {
			r.cols = append(r.cols, c)
			r.vals = append(r.vals, v)
		}
	}
	return r
}

// insert is the INSERT statement of r, its values written as values.
func (r *row) insert(values []string) string {
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);", ident(r.table.name), idents(r.cols),
		strings.Join(values, ", "))
}

// decimal writes r as a decimal number, exactly when it is one: 1250000.5, -0.001.
func decimal(r *big.Rat) string {
	if r.IsInt() {
		return r.Num().String()
	}
	twos, fives, d := 0, 0, new(big.Int).Set(r.Denom())
	two, five, m := big.NewInt(2), big.NewInt(5), new(big.Int)
	for d.Cmp(big.NewInt(1)) != 0 {
		switch {
		case m.Mod(d, two).Sign() == 0:
			d.Div(d, two)
			twos++
		case m.Mod(d, five).Sign() == 0:
			d.Div(d, five)
			fives++
		default:
			return r.FloatString(30)
		}
	}
	return r.FloatString(max(twos, fives))
}

// SQL writes the script that creates the tables and inserts the rows, in one transaction.
func (e *Export) SQL(w io.Writer) error {
	b := bufio.NewWriter(w)
	b.WriteString("PRAGMA foreign_keys = ON;\nBEGIN;\n\n")
	b.WriteString(e.layout.DDL())
	if len(e.rows) > 0 {
		b.WriteString("\n")
	}
	for _, r := range e.rows {
		var vs []string
		for _, v := range r.vals {
			switch v := v.(type) {
			case string:
				vs = append(vs, literal(v))
			case num:
				vs = append(vs, string(v))
			case []byte:
				vs = append(vs, "X'"+hex.EncodeToString(v)+"'")
			}
		}
		b.WriteString(r.insert(vs) + "\n")
	}
	b.WriteString("\nCOMMIT;\n")
	return b.Flush()
}

// Tables is the number of tables of the export, Rows the number of its rows.
func (e *Export) Tables() int { return len(e.layout.tables) }
func (e *Export) Rows() int   { return len(e.rows) }

// CheckFile checks that path names no file: kan sql never overwrites one.
func CheckFile(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return exists(path)
	}
	return nil
}

func exists(path string) error {
	return fmt.Errorf("%s already exists; kan sql writes a new file: give -o another one", path)
}

// create creates the file path, which must not exist.
func create(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil, exists(path)
	}
	return f, err
}

// Script writes the SQL script to a new file at path.
func (e *Export) Script(path string) error {
	f, err := create(path)
	if err != nil {
		return err
	}
	err = e.SQL(f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
	}
	return err
}

// Database writes a new SQLite database at path, which must not exist. When SQLite refuses it, the file is
// removed.
func (e *Export) Database(path string) (err error) {
	f, err := create(path)
	if err != nil {
		return err
	}
	f.Close()
	db, _ := sql.Open("sqlite", path) // fails only for a driver not registered
	db.SetMaxOpenConns(1)
	err = e.write(db, e.layout.DDL())
	if cerr := db.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return fmt.Errorf("%s: %v", path, err)
	}
	return nil
}

// write creates the tables of db with ddl and inserts the rows, in one transaction.
func (e *Export) write(db *sql.DB, ddl string) error {
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(ddl); err != nil {
		return err
	}
	stmts := map[string]*sql.Stmt{}
	for _, r := range e.rows {
		q := r.insert(strings.Split(strings.Repeat("?", len(r.vals)), ""))
		s := stmts[q]
		if s == nil {
			if s, err = tx.Prepare(q); err != nil {
				return err
			}
			stmts[q] = s
		}
		args := make([]any, len(r.vals))
		for i, v := range r.vals {
			args[i] = v
			if n, ok := v.(num); ok {
				args[i] = arg(n)
			}
		}
		if _, err := s.Exec(args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// arg is the value of n for SQLite: an integer, or its text, which a NUMERIC column reads as a number.
func arg(n num) any {
	if i, err := strconv.ParseInt(string(n), 10, 64); err == nil {
		return i
	}
	return string(n)
}
