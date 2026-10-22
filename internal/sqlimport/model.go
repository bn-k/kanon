package sqlimport

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/func-it/kanon/internal/names"
	"github.com/func-it/kanon/internal/value"
)

type kind int

const (
	entity kind = iota // an array field of the collection, written as a folder
	weak               // an owned array of its parent, written as a table in the parent's documents
	join               // a [-> B] field of its parent
)

// A model is what kan import writes: the schemas of the tables and the ids of their rows.
type model struct {
	name   string // the result: northwind
	tables []*tbl // in the order of their names
}

// A tbl is a table with what kan import makes of it.
type tbl struct {
	*table
	kind    kind
	schema  string // the schema of an entity or weak table
	field   string // the kebab-case name of the table: its field in the collection, or in its parent
	types   []*colType
	notes   []string
	fields  []*field
	inverse []*field
	ids     []string                 // entity, weak: the id of each row
	index   map[string]int           // entity: the row of each id
	owner   *foreignKey              // weak, join: the key to the parent
	other   *foreignKey              // join: the key to the table linked to
	parents []string                 // weak, join: the id of each row's parent
	links   map[*foreignKey][]string // the id each row links to by each key, or ""
}

// A field is a field of a schema: a column, a link, a weak table, a join table or an inverse.
type field struct {
	name     string
	typ      string // as written in the schema, without the ? of an optional field
	optional bool
	col      int         // a column's index, or -1
	fk       *foreignKey // a link
	sub      *tbl        // a weak or join table
	from     string      // what gave the field, for messages
}

func build(tables []*table, name string) (*model, error) {
	if len(tables) == 0 {
		return nil, errors.New("the database has no table")
	}
	m := &model{name: name}
	byName := map[string]*tbl{}
	for _, t := range tables {
		bt := &tbl{table: t, field: fieldName(t.name), index: map[string]int{}, links: map[*foreignKey][]string{}}
		m.tables = append(m.tables, bt)
		byName[strings.ToLower(t.name)] = bt
	}
	for _, t := range m.tables {
		for _, fk := range t.fks {
			if err := resolve(fk, byName); err != nil {
				return nil, err
			}
		}
		for c := range t.cols {
			ct, note, err := infer(t.table, c)
			if err != nil {
				return nil, err
			}
			t.types = append(t.types, ct)
			if note != "" {
				t.notes = append(t.notes, note)
			}
		}
	}
	m.classify()
	if err := m.nameSchemas(); err != nil {
		return nil, err
	}
	for _, step := range []func(*tbl) error{(*tbl).identify, (*tbl).link, (*tbl).columns} {
		for _, t := range m.tables {
			if err := step(t); err != nil {
				return nil, err
			}
		}
	}
	for _, t := range m.tables {
		t.arrays(m.tables)
	}
	return m, m.inverses()
}

// resolve finds the table a foreign key refers to, and which of its columns.
func resolve(fk *foreignKey, byName map[string]*tbl) error {
	from := colNames(fk.table, fk.from)
	target := byName[strings.ToLower(fk.refer)]
	if target == nil {
		return fmt.Errorf("%s refers to `%s`, which is not a table", from, fk.refer)
	}
	pk := target.key()
	if len(pk) == 0 {
		return fmt.Errorf("%s refers to `%s`, which has no primary key", from, target.name)
	}
	fk.target, fk.key = target, pk
	if len(fk.to) == 0 {
		if len(pk) != len(fk.from) {
			return fmt.Errorf("%s is %s but refers to the %s of `%s`", from, count(len(fk.from), "column"),
				count(len(pk), "key column"), target.name)
		}
		return nil
	}
	fk.key = nil
	for _, to := range fk.to {
		fk.key = append(fk.key, slices.IndexFunc(target.cols, func(c *column) bool { return strings.EqualFold(c.name, to) }))
	}
	if !slices.Equal(slices.Sorted(slices.Values(fk.key)), slices.Sorted(slices.Values(pk))) {
		return fmt.Errorf("%s refers to %s; kan import follows foreign keys to the primary key only", from,
			colNames(target.table, fk.key))
	}
	return nil
}

// classify sorts the tables into join, weak and entity tables.
func (m *model) classify() {
	targeted := map[*tbl]bool{}
	for _, t := range m.tables {
		for _, fk := range t.fks {
			targeted[fk.target] = true
		}
	}
	for _, t := range m.tables {
		pk := t.key()
		if targeted[t] || len(pk) < 2 {
			continue
		}
		if len(pk) == len(t.cols) && len(t.fks) == 2 && disjoint(t.fks[0].from, t.fks[1].from) {
			t.kind, t.owner, t.other = join, t.fks[0], t.fks[1]
			if minPK(t.table, t.other.from) < minPK(t.table, t.owner.from) {
				t.owner, t.other = t.other, t.owner
			}
			continue
		}
		var owner *foreignKey
		for _, fk := range t.fks {
			inKey := len(fk.from) < len(pk) && !slices.ContainsFunc(fk.from, func(c int) bool { return t.cols[c].pk == 0 })
			if inKey && (owner == nil || minPK(t.table, fk.from) < minPK(t.table, owner.from)) {
				owner = fk
			}
		}
		if owner == nil || owner.target == t || slices.ContainsFunc(t.types, func(ct *colType) bool {
			return ct.native == "Prose" || ct.native == "File" || ct.native == "Image"
		}) {
			continue
		}
		t.kind, t.owner = weak, owner
	}
}

func disjoint(a, b []int) bool {
	return !slices.ContainsFunc(a, func(c int) bool { return slices.Contains(b, c) })
}

// minPK gives the first position in the primary key of the columns cols.
func minPK(t *table, cols []int) int {
	n := len(t.cols) + 1
	for _, c := range cols {
		n = min(n, t.cols[c].pk)
	}
	return n
}

// nameSchemas names the schemas of the entity and weak tables.
func (m *model) nameSchemas() error {
	result := schemaName(m.name)
	seen := map[string]*tbl{}
	for _, t := range m.tables {
		if t.kind == join {
			continue
		}
		t.schema = schemaName(t.name)
		_, native := value.KindOf(t.schema)
		switch {
		case !names.IsTypeName(t.schema):
			return fmt.Errorf("the table `%s` gives no schema name; rename the table", t.name)
		case native:
			return fmt.Errorf("the table `%s` gives the schema `%s`, a native type; rename the table", t.name, t.schema)
		case t.schema == result:
			return fmt.Errorf("the table `%s` gives the schema `%s`, the name of the result; import into another folder",
				t.name, t.schema)
		case seen[t.schema] != nil:
			return fmt.Errorf("the tables `%s` and `%s` both give the schema `%s`", seen[t.schema].name, t.name,
				t.schema)
		}
		seen[t.schema] = t
	}
	return nil
}

// idKey gives the columns that make the id of a row: the primary key, less the parent's columns in a weak table.
func (t *tbl) idKey() []int {
	key := t.key()
	if t.kind == weak {
		key = slices.DeleteFunc(key, func(c int) bool { return slices.Contains(t.owner.from, c) })
	}
	return key
}

// identify gives each row of an entity or weak table its id, unique in the table, and in its parent for a weak
// table.
func (t *tbl) identify() error {
	if t.kind == join {
		return nil
	}
	key := t.idKey()
	changed, example := 0, ""
	for i, row := range t.rows {
		raw := strconv.FormatInt(t.rowids[i], 10)
		id := raw
		if len(key) > 0 {
			var err error
			if raw, id, err = t.keyOf(row, key); err != nil {
				return err
			}
		}
		if id != raw {
			if changed == 0 {
				example = fmt.Sprintf("%q → %q", raw, id)
			}
			changed++
		}
		t.ids = append(t.ids, id)
		if t.kind == entity {
			if j, ok := t.index[id]; ok {
				return t.same(j, i)
			}
			t.index[id] = i
		}
	}
	if changed > 0 {
		verb := "keys change"
		if changed == 1 {
			verb = "key changes"
		}
		t.notes = slices.Insert(t.notes, 0, fmt.Sprintf("`%s`: %d %s to make ids, such as %s", t.name, changed, verb,
			example))
	}
	return nil
}

// keyOf gives the key of a row as written, and the id it makes: the slugs of its columns joined by hyphens.
func (t *tbl) keyOf(row []any, cols []int) (string, string, error) {
	var raws, slugs []string
	for _, c := range cols {
		switch v := row[c].(type) {
		case nil:
			return "", "", fmt.Errorf("`%s`: a row has no key", t.name)
		case []byte:
			return "", "", fmt.Errorf("`%s`: a row has a file as key", t.name)
		default:
			raw := plain(v)
			slug := names.Slugify(raw)
			if slug == "" {
				return "", "", fmt.Errorf("`%s`: the key %q gives no id", t.name, raw)
			}
			raws, slugs = append(raws, raw), append(slugs, slug)
		}
	}
	return strings.Join(raws, "-"), strings.Join(slugs, "-"), nil
}

// same reports two rows of t that give the same id.
func (t *tbl) same(i, j int) error {
	a, _, _ := t.keyOf(t.rows[i], t.idKey())
	b, _, _ := t.keyOf(t.rows[j], t.idKey())
	return fmt.Errorf("`%s`: the keys %q and %q both give the id `%s`", t.name, a, b, t.ids[i])
}

// link finds the row each foreign key of each row refers to; for a weak or join table, its parent first.
func (t *tbl) link() error {
	seen := map[[2]string]int{}
	for i, row := range t.rows {
		for _, fk := range t.fks {
			id, err := linked(fk, row)
			if err != nil {
				return err
			}
			t.links[fk] = append(t.links[fk], id)
		}
		if t.kind == entity {
			continue
		}
		parent := t.links[t.owner][i]
		if parent == "" {
			return fmt.Errorf("`%s`: a row has no key", t.name)
		}
		t.parents = append(t.parents, parent)
		if t.kind == weak {
			k := [2]string{parent, t.ids[i]}
			if j, ok := seen[k]; ok {
				return t.same(j, i)
			}
			seen[k] = i
		}
	}
	return nil
}

// linked gives the id of the row a foreign key of row refers to, or "" when its columns are NULL.
func linked(fk *foreignKey, row []any) (string, error) {
	values := make([]any, len(fk.target.cols))
	var raws []string
	for i, c := range fk.from {
		if row[c] == nil {
			return "", nil
		}
		values[fk.key[i]] = row[c]
		raws = append(raws, plain(row[c]))
	}
	_, id, err := fk.target.keyOf(values, fk.target.key())
	if _, ok := fk.target.index[id]; err != nil || !ok {
		return "", fmt.Errorf("%s: no row of `%s` has the key %q", colNames(fk.table, fk.from), fk.target.name,
			strings.Join(raws, ", "))
	}
	return id, nil
}

// columns makes the fields of the columns of an entity or weak table: one per column, but none for the columns
// of the id, and one per foreign key, where its first column is.
func (t *tbl) columns() error {
	if t.kind == join {
		return nil
	}
	inFK := map[int]bool{}
	for _, fk := range t.fks {
		for _, c := range fk.from {
			inFK[c] = true
		}
	}
	for c, col := range t.cols {
		for _, fk := range t.fks {
			if fk.from[0] != c || fk == t.owner {
				continue
			}
			ws := words(col.name)
			if len(fk.from) > 1 || len(ws) == 1 && ws[0] == "id" {
				ws = words(fk.target.schema)
			} else if len(ws) > 1 && ws[len(ws)-1] == "id" {
				ws = ws[:len(ws)-1]
			}
			optional := slices.ContainsFunc(fk.from, func(c int) bool { return !t.required(c) })
			t.fields = append(t.fields, &field{name: strings.Join(ws, "-"), typ: "-> " + fk.target.schema,
				optional: optional, col: c, fk: fk, from: colNames(t.table, []int{c})})
		}
		if inFK[c] || col.pk > 0 {
			continue
		}
		ct := t.types[c]
		f := &field{name: kebab(col.name), typ: ct.native, col: c, from: colNames(t.table, []int{c}),
			optional: !t.required(c) || ct.empties > 0}
		if ct.trimmed > 0 {
			t.notes = append(t.notes, fmt.Sprintf("%s: the spaces around %s are dropped", f.from,
				count(ct.trimmed, "value")))
		}
		if ct.tidied > 0 {
			t.notes = append(t.notes, fmt.Sprintf("%s: spaces at the ends of lines and extra blank lines are "+
				"dropped in %s", f.from, count(ct.tidied, "value")))
		}
		if ct.crlf > 0 {
			verb := "values have"
			if ct.crlf == 1 {
				verb = "value has"
			}
			t.notes = append(t.notes, fmt.Sprintf(`%s: %d %s Windows line breaks (\r\n), written as \n`, f.from,
				ct.crlf, verb))
		}
		if ct.empties > 0 {
			verb := "empty texts are"
			if ct.empties == 1 {
				verb = "empty text is"
			}
			t.notes = append(t.notes, fmt.Sprintf("%s: %d %s written as no value", f.from, ct.empties, verb))
		}
		t.fields = append(t.fields, f)
	}
	seen := map[string]*field{}
	for _, f := range t.fields {
		switch {
		case !names.IsName(f.name):
			return fmt.Errorf("the column %s gives no field name; rename the column", f.from)
		case f.name == "id":
			return fmt.Errorf("the column %s gives the field `id`, the id of every object; rename the column", f.from)
		case seen[f.name] != nil:
			return fmt.Errorf("%s and %s both give the field `%s`", seen[f.name].from, f.from, f.name)
		}
		seen[f.name] = f
	}
	return nil
}

// required reports whether column c can't be NULL.
func (t *tbl) required(c int) bool { return t.cols[c].notNull || t.cols[c].pk > 0 }

// arrays adds the fields of the weak and join tables held by t after its columns, in the order of their names.
func (t *tbl) arrays(tables []*tbl) {
	if t.kind == join {
		return
	}
	var fields []*field
	for _, sub := range tables {
		if sub.kind == entity || sub.owner.target != t {
			continue
		}
		f := &field{name: sub.field, typ: "[" + sub.schema + "]", col: -1, sub: sub}
		if sub.kind == join {
			f.name, f.typ = sub.other.target.field, "[-> "+sub.other.target.schema+"]"
		}
		f.optional = len(t.ids) == 0 ||
			slices.ContainsFunc(t.ids, func(id string) bool { return !slices.Contains(sub.parents, id) })
		fields = append(fields, f)
	}
	slices.SortStableFunc(fields, func(a, b *field) int { return strings.Compare(a.name, b.name) })
	t.fields = append(t.fields, fields...)
}

// inverses adds to each table the inverse of each link to it, named after the table that links, or
// <table>-by-<field> when that name is taken.
func (m *model) inverses() error {
	type inv struct {
		target *tbl
		source string // the kebab-case name of the table that links, to order the inverses
		col    int
		f      *field
	}
	var invs []inv
	for _, t := range m.tables {
		for _, f := range t.fields {
			if f.fk == nil {
				continue
			}
			i := inv{target: f.fk.target, source: t.field, col: f.col, f: &field{name: t.field,
				typ: "[" + t.schema + "] <- " + f.name, col: -1, from: colNames(t.table, f.fk.from)}}
			if t.oneToOne(f.fk.from) {
				i.f.name, i.f.typ = kebab(t.schema), t.schema+"? <- "+f.name
			}
			invs = append(invs, i)
		}
		if t.kind == join {
			parent, other := t.owner.target, t.other.target
			invs = append(invs, inv{target: other, source: t.field, col: t.other.from[0], f: &field{name: parent.field,
				typ: "[" + parent.schema + "] <- " + other.field, col: -1, from: colNames(t.table, t.other.from)}})
		}
	}
	slices.SortStableFunc(invs, func(a, b inv) int {
		if c := strings.Compare(a.source, b.source); c != 0 {
			return c
		}
		return a.col - b.col
	})
	for _, i := range invs {
		taken := func(name string) bool {
			return slices.ContainsFunc(append(slices.Clip(i.target.fields), i.target.inverse...), func(f *field) bool {
				return f.name == name
			})
		}
		if taken(i.f.name) {
			_, field, _ := strings.Cut(i.f.typ, " <- ")
			i.f.name = i.source + "-by-" + field
			if taken(i.f.name) {
				return fmt.Errorf("`%s` already has a field `%s`, the inverse of %s", i.target.schema, i.f.name,
					i.f.from)
			}
		}
		i.target.inverse = append(i.target.inverse, i.f)
	}
	return nil
}

// oneToOne reports whether the columns cols are unique in t: its primary key or a unique index.
func (t *tbl) oneToOne(cols []int) bool {
	set := slices.Sorted(slices.Values(cols))
	return slices.ContainsFunc(t.unique, func(u []int) bool { return slices.Equal(slices.Sorted(slices.Values(u)), set) })
}

// colNames writes columns for a message: `Orders.CustomerID`, `Notes.PostID, Revision`.
func colNames(t *table, cols []int) string {
	var ns []string
	for _, c := range cols {
		ns = append(ns, t.cols[c].name)
	}
	return fmt.Sprintf("`%s.%s`", t.name, strings.Join(ns, ", "))
}

// count writes a number of things: 1 value, 2 values.
func count(n int, thing string) string {
	if n == 1 {
		return "1 " + thing
	}
	return strconv.Itoa(n) + " " + thing + "s"
}
