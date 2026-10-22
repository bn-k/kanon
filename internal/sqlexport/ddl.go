package sqlexport

import (
	"fmt"
	"strings"
)

const deferred = " DEFERRABLE INITIALLY DEFERRED"

// DDL returns the statements that create the tables, one blank line between them.
func (l *Layout) DDL() string { return l.ddl(false) }

// queryDDL is the DDL of the database a query runs on: the same tables, but DATE and DATETIME columns are
// declared TEXT, which they store anyway. The driver gives a DATE as a time, and the text as written is wanted.
func (l *Layout) queryDDL() string { return l.ddl(true) }

func (l *Layout) ddl(text bool) string {
	var b strings.Builder
	for i, t := range l.tables {
		if i > 0 {
			b.WriteString("\n")
		}
		t.create(&b, text)
	}
	return b.String()
}

// create writes the CREATE TABLE statement of t, its column types aligned; with text, its dates as TEXT.
func (t *table) create(b *strings.Builder, text bool) {
	width := 0
	for _, c := range t.cols {
		width = max(width, len(ident(c.name)))
	}
	var lines []string
	for _, c := range t.cols {
		typ := c.typ
		if text && (typ == "DATE" || typ == "DATETIME") {
			typ = "TEXT"
		}
		s := fmt.Sprintf("%-*s %s", width, ident(c.name), typ)
		if c.notNull {
			s += " NOT NULL"
		}
		if c.check != nil {
			s += fmt.Sprintf(" CHECK (%s IN (%s))", ident(c.name), strings.Join(c.check, ", "))
		}
		if c.refers != nil {
			s += " REFERENCES " + ident(c.refers.name) + " (id)" + deferred
		}
		lines = append(lines, s)
	}
	if t.key != nil {
		lines = append(lines, "PRIMARY KEY ("+idents(t.key)+")")
	}
	for _, fk := range t.fks {
		lines = append(lines, "FOREIGN KEY ("+idents(fk.cols)+") REFERENCES "+ident(fk.refers.name)+" ("+
			idents(fk.refers.key)+")"+deferred)
	}
	fmt.Fprintf(b, "CREATE TABLE %s (\n  %s\n);\n", ident(t.name), strings.Join(lines, ",\n  "))
}

func idents(cols []*column) string {
	var ns []string
	for _, c := range cols {
		ns = append(ns, ident(c.name))
	}
	return strings.Join(ns, ", ")
}

// snake gives the name of a table or a column: EtatDesLieux gives etat_des_lieux, HTMLPage html_page,
// prix-unitaire prix_unitaire.
func snake(s string) string {
	lower := func(c byte) bool { return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' }
	upper := func(c byte) bool { return c >= 'A' && c <= 'Z' }
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '-':
			b.WriteByte('_')
		case upper(c):
			if i > 0 && (lower(s[i-1]) || upper(s[i-1]) && i+1 < len(s) && s[i+1] >= 'a' && s[i+1] <= 'z') {
				b.WriteByte('_')
			}
			b.WriteByte(c - 'A' + 'a')
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// ident writes a name in SQL: as it is, or quoted when it is a keyword of SQLite.
func ident(name string) string {
	if keywords[name] {
		return `"` + name + `"`
	}
	return name
}

// literal writes s as a SQL string.
func literal(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// keywords are the keywords of SQLite (sqlite.org/lang_keywords.html), in lowercase like the names Kanon gives.
var keywords = map[string]bool{}

func init() {
	for _, k := range strings.Fields(`
		abort action add after all alter always analyze and as asc attach autoincrement before begin between by
		cascade case cast check collate column commit conflict constraint create cross current current_date
		current_time current_timestamp database default deferrable deferred delete desc detach distinct do drop
		each else end escape except exclude exclusive exists explain fail filter first following for foreign from
		full generated glob group groups having if ignore immediate in index indexed initially inner insert instead
		intersect into is isnull join key last left like limit match materialized natural no not nothing notnull
		null nulls of offset on or order others outer over partition plan pragma preceding primary query raise
		range recursive references regexp reindex release rename replace restrict returning right rollback row
		rows savepoint select set table temp temporary then ties to transaction trigger unbounded union unique
		update using vacuum values view virtual when where window with without`) {
		keywords[k] = true
	}
}
