// Package sqlexport writes a Kanon document as SQLite: a table per schema, a row per object, laid out as in
// docs/data.md §4.
package sqlexport

import (
	"fmt"
	"strings"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/schema"
	"github.com/bn-k/kanon/internal/value"
)

// Layout is how the objects of a schema file are laid out in tables.
type Layout struct {
	tables  []*table                    // in the order of the DDL: each schema's table, then those of its fields
	schemas map[*schema.Schema]*table   // the table of each schema, but a collection
	fields  map[*schema.Field]*table    // the join or value table of a field
	parents map[*schema.Field][]*column // the columns that hold, in the table of a row, the object holding it by a field
}

// A table holds the objects of a schema, the links of a [-> T] field, or the values of an array of values.
type table struct {
	name   string
	schema *schema.Schema // the schema of its rows; nil for a join or a value table
	field  *schema.Field  // the field of a join or a value table
	weak   bool           // its ids are unique within their parent only
	holder *schema.Field  // the field that holds the rows of a weak table
	parent *schema.Schema // and the schema of that field
	cols   []*column
	key    []*column     // the primary key: the id, the key of the parent and the id, or every column
	fks    []*foreignKey // those of several columns; a column of one is its column's refers
	values map[*schema.Field][]*column
	origin
}

// A column of a table.
type column struct {
	name    string
	typ     string // TEXT, INTEGER, NUMERIC, BLOB, DATE or DATETIME
	notNull bool
	check   []string // the values it may hold, as SQL literals
	refers  *table   // the table it is a foreign key to, by its id
	origin
}

// origin is what gives a table or a column, for messages.
type origin struct {
	what   string   // the schema `Shop`, `Shop.tags`; `price`, the currency of `price`, the parent `Shop.orders`
	pos    diag.Pos // where to rename it
	rename string   // what to rename
}

// A foreignKey of several columns: the key of a row of a weak table.
type foreignKey struct {
	cols   []*column
	refers *table
}

// layout lays out the tables of a schema file.
type layout struct {
	*Layout
	f          *schema.File
	owner      map[*schema.Field]*schema.Schema   // the schema of each field that holds objects
	arrays     map[*schema.Schema][]*schema.Field // the owned array fields that hold each schema
	single     map[*schema.Schema]bool            // held by a field of one object
	linked     map[*schema.Schema]bool
	collection map[*schema.Schema]bool
	fieldCols  map[*schema.Schema][]*column
	keys       map[*schema.Schema][]*column
	diags      diag.List
}

// NewLayout lays out the tables of the schemas of f, which must have been checked. It reports the tables and the
// columns whose names clash, and returns no layout then.
func NewLayout(f *schema.File) (*Layout, diag.List) {
	l := &layout{
		Layout: &Layout{schemas: map[*schema.Schema]*table{}, fields: map[*schema.Field]*table{},
			parents: map[*schema.Field][]*column{}},
		f: f, owner: map[*schema.Field]*schema.Schema{}, arrays: map[*schema.Schema][]*schema.Field{},
		single: map[*schema.Schema]bool{}, linked: map[*schema.Schema]bool{}, collection: map[*schema.Schema]bool{},
		fieldCols: map[*schema.Schema][]*column{}, keys: map[*schema.Schema][]*column{},
	}
	l.scan()
	for _, s := range f.Schemas {
		if !l.collection[s] {
			l.schemas[s] = &table{name: snake(s.Name), schema: s, values: map[*schema.Field][]*column{},
				origin: origin{"the schema `" + s.Name + "`", s.Pos, s.Name}}
		}
	}
	for _, s := range f.Schemas {
		if t := l.schemas[s]; t != nil {
			l.fieldCols[s] = l.fieldColumns(t)
		}
	}
	var subs []*table
	for _, s := range f.Schemas {
		t := l.schemas[s]
		if t == nil {
			continue
		}
		t.key = l.key(s)
		t.cols = append(t.cols, t.key...)
		if !t.weak {
			for _, h := range l.arrays[s] {
				if !l.collection[l.owner[h]] {
					t.cols = append(t.cols, l.parent(t, h, false)...)
				}
			}
		}
		t.cols = append(t.cols, l.fieldCols[s]...)
		l.tables = append(l.tables, t)
		for _, fl := range s.Fields {
			if !fl.Computed() && fl.Type.Array && (fl.Type.Link || fl.Type.Schema == nil) {
				sub := l.sub(t, fl)
				l.fields[fl] = sub
				l.tables = append(l.tables, sub)
				subs = append(subs, sub)
			}
		}
	}
	l.check(subs)
	if len(l.diags) > 0 {
		l.diags.Sort()
		return nil, l.diags
	}
	return l.Layout, nil
}

// scan finds which fields hold and link to each schema, and which schemas are collections.
func (l *layout) scan() {
	for _, s := range l.f.Schemas {
		for _, fl := range s.Fields {
			t := fl.Type
			switch {
			case fl.Computed() || t.Schema == nil:
			case t.Link:
				l.linked[t.Schema] = true
			case t.Array:
				l.owner[fl] = s
				l.arrays[t.Schema] = append(l.arrays[t.Schema], fl)
			default:
				l.owner[fl] = s
				l.single[t.Schema] = true
			}
		}
	}
	for _, s := range l.f.Schemas {
		written, objects := 0, 0
		for _, fl := range s.Fields {
			if !fl.Computed() {
				written++
				if fl.Type.Array && !fl.Type.Link && fl.Type.Schema != nil {
					objects++
				}
			}
		}
		l.collection[s] = written > 0 && objects == written && len(l.arrays[s]) == 0 && !l.single[s] && !l.linked[s]
	}
}

// heldOnce reports whether s is only ever held by one owned array field, of another schema that is not a
// collection, and never linked to nor held by a single field.
func (l *layout) heldOnce(s *schema.Schema) bool {
	if len(l.arrays[s]) != 1 || l.single[s] || l.linked[s] {
		return false
	}
	p := l.owner[l.arrays[s][0]]
	return p != s && !l.collection[p]
}

// isWeak reports whether s is weak: held once, by a schema that is not held, through its parents, by s.
func (l *layout) isWeak(s *schema.Schema) bool {
	if !l.heldOnce(s) {
		return false
	}
	p := l.owner[l.arrays[s][0]]
	for range l.f.Schemas {
		if !l.heldOnce(p) {
			return true
		}
		p = l.owner[l.arrays[p][0]]
		if p == s {
			return false
		}
	}
	return true
}

// key gives the primary key of the table of s: its id, after the key of its parent when s is weak.
func (l *layout) key(s *schema.Schema) []*column {
	if k, ok := l.keys[s]; ok {
		return k
	}
	t := l.schemas[s]
	id := &column{name: "id", typ: "TEXT", notNull: true}
	k := []*column{id}
	if l.isWeak(s) {
		t.weak, t.holder = true, l.arrays[s][0]
		t.parent = l.owner[t.holder]
		k = append(l.parent(t, t.holder, true), id)
	}
	l.keys[s] = k
	return k
}

// parent gives the columns of t that hold the object holding a row by the field h: the key of its table, its id
// named <parent>_id, or <parent>_<field>_id when the parent has two such fields or t has a column <parent>_id.
func (l *layout) parent(t *table, h *schema.Field, notNull bool) []*column {
	p := l.owner[h]
	base := snake(p.Name)
	twice := 0
	for _, fl := range l.arrays[t.schema] {
		if l.owner[fl] == p {
			twice++
		}
	}
	taken := func(name string) bool {
		for _, c := range append(l.fieldCols[t.schema], t.cols...) {
			if c.name == name {
				return true
			}
		}
		return false
	}
	if twice > 1 || taken(base+"_id") {
		base += "_" + snake(h.Name)
	}
	cols := l.ref(t, l.schemas[p], base, notNull, origin{"the parent `" + p.Name + "." + h.Name + "`", h.Pos, h.Name})
	l.parents[h] = cols
	return cols
}

// ref gives the columns of t that refer to a row of p: those of its parent's key, if p is weak, then base_id.
func (l *layout) ref(t, p *table, base string, notNull bool, o origin) []*column {
	var cols []*column
	key := l.key(p.schema)
	for _, k := range key[:len(key)-1] {
		cols = append(cols, &column{name: k.name, typ: "TEXT", notNull: notNull, origin: k.origin})
	}
	id := &column{name: base + "_id", typ: "TEXT", notNull: notNull, origin: o}
	cols = append(cols, id)
	if p.weak {
		t.fks = append(t.fks, &foreignKey{cols, p})
	} else {
		id.refers = p
	}
	return cols
}

// fieldColumns gives the columns of the fields of t's schema that hold one value or one object.
func (l *layout) fieldColumns(t *table) []*column {
	var cols []*column
	for _, fl := range t.schema.Fields {
		if fl.Computed() || fl.Type.Array {
			continue
		}
		cs := l.columns(snake(fl.Name), fl.Type, fl.Required(), origin{"`" + fl.Name + "`", fl.Pos, fl.Name})
		t.values[fl] = cs
		cols = append(cols, cs...)
	}
	return cols
}

// sub gives the table of the field fl of t: a join table for [-> T], a value table for an array of values.
func (l *layout) sub(t *table, fl *schema.Field) *table {
	s := t.schema
	sub := &table{name: t.name + "_" + snake(fl.Name), field: fl, values: map[*schema.Field][]*column{},
		origin: origin{"`" + s.Name + "." + fl.Name + "`", fl.Pos, fl.Name}}
	ref := l.ref(sub, t, t.name, true, origin{"the parent `" + s.Name + "`", fl.Pos, fl.Name})
	sub.cols = ref
	o := origin{"the values of `" + fl.Name + "`", fl.Pos, fl.Name}
	name := "value"
	if fl.Type.Link {
		o.what = "the links of `" + fl.Name + "`"
		name = snake(fl.Type.Schema.Name)
		for _, c := range ref {
			if c.name == name+"_id" {
				name = snake(fl.Name)
			}
		}
	}
	vals := l.columns(name, fl.Type, true, o)
	sub.values[fl] = vals
	sub.cols = append(sub.cols, vals...)
	if fl.Type.Link {
		sub.key = sub.cols
	}
	return sub
}

// columns gives the columns of a value of type typ, named after name.
func (l *layout) columns(name string, typ *schema.Type, notNull bool, o origin) []*column {
	c := &column{name: name, notNull: notNull, origin: o}
	switch {
	case typ.Enum != nil:
		c.typ = "TEXT"
		for _, v := range typ.Enum.Values {
			c.check = append(c.check, literal(v.Name))
		}
	case typ.Schema != nil:
		c.name, c.typ, c.refers = name+"_id", "TEXT", l.schemas[typ.Schema]
	default:
		c.typ = types[typ.Kind]
	}
	cols := []*column{c}
	switch {
	case typ.Schema != nil || typ.Enum != nil:
	case typ.Kind == value.Bool:
		c.check = []string{"0", "1"}
	case typ.Kind == value.Money:
		cols = append(cols, &column{name: name + "_currency", typ: "TEXT", notNull: notNull,
			origin: origin{"the currency of " + o.what, o.pos, o.rename}})
	case typ.Kind == value.Quantity:
		cols = append(cols, &column{name: name + "_unit", typ: "TEXT",
			origin: origin{"the unit of " + o.what, o.pos, o.rename}})
	}
	return cols
}

// types gives the SQLite type of each native type. Date and DateTime are declared as such, so that SQLite tools
// and kan import read them back as dates; their values are text, as written.
var types = map[value.Kind]string{
	value.Text: "TEXT", value.Prose: "TEXT", value.Code: "TEXT", value.Duration: "TEXT", value.Version: "TEXT",
	value.Date: "DATE", value.DateTime: "DATETIME",
	value.Int: "INTEGER", value.Bool: "INTEGER",
	value.Number: "NUMERIC", value.Percent: "NUMERIC", value.Money: "NUMERIC", value.Quantity: "NUMERIC",
	value.File: "BLOB", value.Image: "BLOB",
}

// check reports the tables and the columns whose names clash, and the tables named like SQLite's own.
func (l *layout) check(subs []*table) {
	seen := map[string]*table{}
	for _, t := range append(l.schemaTables(), subs...) {
		var msg string
		if strings.HasPrefix(t.name, "sqlite_") {
			msg = fmt.Sprintf("%s gives the table `%s`, a name SQLite keeps for itself; rename `%s`", t.what, t.name,
				t.rename)
		} else if u := seen[t.name]; u != nil {
			msg = fmt.Sprintf("%s and %s both give the table `%s`; rename `%s`", u.what, t.what, t.name, t.rename)
		}
		if msg != "" {
			l.diags.Errorf(t.pos, "%s", msg)
		}
		seen[t.name] = t
	}
	for _, t := range l.tables {
		cols := map[string]*column{}
		for _, c := range t.cols {
			if d := cols[c.name]; d != nil {
				msg := fmt.Sprintf("%s and %s both give the column `%s` of the table `%s`; rename `%s`", d.what, c.what,
					c.name, t.name, c.rename)
				l.diags.Errorf(c.pos, "%s", msg)
			}
			cols[c.name] = c
		}
	}
}

func (l *layout) schemaTables() []*table {
	var ts []*table
	for _, s := range l.f.Schemas {
		if t := l.schemas[s]; t != nil {
			ts = append(ts, t)
		}
	}
	return ts
}
