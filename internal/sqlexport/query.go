package sqlexport

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/func-it/kanon/internal/diag"
	"github.com/func-it/kanon/internal/names"
)

// Result is the rows a query gives: each value is nil (NULL), an int64, a float64, a string or a []byte.
type Result struct {
	Columns []string
	Rows    [][]any
}

// Query runs the query q over the tables of the export, built in memory and read only, and returns its rows.
// What is wrong with q is reported at its position in a source called name: a query that writes, several
// statements, and the errors of SQLite, with the name nearest to a column or a table that does not exist.
func (e *Export) Query(name, q string) (*Result, diag.List) {
	var ds diag.List
	at := func(off int, format string, args ...any) {
		line, col := position(q, off)
		ds.Errorf(diag.Pos{File: name, Line: line, Col: col}, format, args...)
	}
	start, word, extra := scan(q)
	switch {
	case start == len(q):
		at(0, "the query is empty")
	case !reads[strings.ToLower(word)]:
		msg := fmt.Sprintf("a query only reads: write SELECT or WITH, not \"%s\"", word)
		at(start, "%s", msg)
	case extra >= 0:
		at(extra, "a query is one statement; remove what follows the first `;`")
	}
	if len(ds) > 0 {
		return nil, ds
	}
	db, _ := sql.Open("sqlite", ":memory:") // fails only for a driver not registered
	defer db.Close()
	db.SetMaxOpenConns(1) // an in-memory database lives in its connection
	err := e.write(db, e.layout.queryDDL())
	if err == nil {
		// Writing is refused from here, whatever the query says; the check of its first word comes first, since
		// a PRAGMA could undo this one.
		_, err = db.Exec("PRAGMA query_only = ON")
	}
	if err != nil {
		ds.Errorf(diag.Pos{File: name}, "%s", cleanError(err))
		return nil, ds
	}
	r, err := run(db, q)
	if err != nil {
		off, msg := e.explain(q, err)
		at(off, "%s", msg)
		return nil, ds
	}
	return r, nil
}

// reads are the statements a query may start with.
var reads = map[string]bool{"select": true, "with": true, "values": true, "explain": true}

// run runs the query q and reads all its rows.
func run(db *sql.DB, q string) (*Result, error) {
	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	r := &Result{Columns: cols}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		r.Rows = append(r.Rows, vals)
	}
	return r, rows.Err()
}

// scan finds, in q, the offset of its first word (or character), that word, and the offset of what follows its statement's `;`,
// or -1 when nothing does. Comments, strings and quoted names are skipped.
func scan(q string) (start int, word string, extra int) {
	i := skip(q, 0)
	start = i
	j := i
	for j < len(q) && (q[j] >= 'a' && q[j] <= 'z' || q[j] >= 'A' && q[j] <= 'Z') {
		j++
	}
	if word = q[i:j]; word == "" && i < len(q) {
		_, n := utf8.DecodeRuneInString(q[i:])
		word = q[i : i+n] // not a word: the character
	}
	for i < len(q) && q[i] != ';' {
		i = next(q, i)
	}
	if i = skip(q, i+1); i < len(q) {
		return start, word, i
	}
	return start, word, -1
}

// skip returns the offset of the first thing of q after i that is not a space or a comment.
func skip(q string, i int) int {
	for i < len(q) {
		switch {
		case q[i] == ' ' || q[i] == '\t' || q[i] == '\n' || q[i] == '\r':
			i++
		case strings.HasPrefix(q[i:], "--"):
			i = strings.Index(q[i:]+"\n", "\n") + i
		case strings.HasPrefix(q[i:], "/*"):
			end := strings.Index(q[i+2:], "*/")
			if end < 0 {
				return len(q)
			}
			i += end + 4
		default:
			return i
		}
	}
	return len(q)
}

// next returns the offset after the character, string, quoted name or comment of q at i.
func next(q string, i int) int {
	if j := skip(q, i); j > i {
		return j
	}
	closer := map[byte]byte{'\'': '\'', '"': '"', '`': '`', '[': ']'}[q[i]]
	if closer == 0 {
		return i + 1
	}
	for j := i + 1; j < len(q); j++ {
		if q[j] == closer {
			if closer != ']' && j+1 < len(q) && q[j+1] == closer {
				j++ // a doubled quote
				continue
			}
			return j + 1
		}
	}
	return len(q)
}

// position gives the line and the column, in characters, of the offset off of q.
func position(q string, off int) (line, col int) {
	line, col = 1, 1
	for _, r := range q[:min(off, len(q))] {
		if r == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
	}
	return line, col
}

var (
	sqliteMsg = regexp.MustCompile(`^SQL logic error: |\s*\(\d+\)$`)
	nearMsg   = regexp.MustCompile(`^near "(.*)": syntax error$`)
	namedMsg  = regexp.MustCompile(`^(no such column|no such table|no such function|ambiguous column name): (.+)$`)
)

// cleanError is the message of err as a diagnostic says it: SQLite's own, without its code, in lowercase.
func cleanError(err error) string {
	msg := sqliteMsg.ReplaceAllString(err.Error(), "")
	r, n := utf8.DecodeRuneInString(msg)
	return strings.ToLower(string(r)) + msg[n:]
}

// explain gives the offset in q and the message of an error of SQLite, in the words of the project: the name
// nearest to an unknown column or table, and the keyword that must be quoted.
func (e *Export) explain(q string, err error) (int, string) {
	msg := cleanError(err)
	if strings.Contains(msg, "readonly database") {
		return skip(q, 0), "a query only reads: this statement writes"
	}
	if m := nearMsg.FindStringSubmatch(msg); m != nil {
		off := find(q, m[1])
		if keywords[strings.ToLower(m[1])] && e.named(strings.ToLower(m[1])) {
			return off, fmt.Sprintf("`%s` is a table here but also a SQL keyword; write \"%s\" with double quotes", m[1], m[1])
		}
		return off, fmt.Sprintf("syntax error near `%s`", m[1])
	}
	if msg == "incomplete input" {
		return len(strings.TrimRight(q, " \t\r\n;")), "the query is incomplete"
	}
	m := namedMsg.FindStringSubmatch(msg)
	if m == nil {
		return skip(q, 0), msg
	}
	what, name := m[1], m[2]
	out := fmt.Sprintf("%s `%s`", what, name)
	if what == "ambiguous column name" {
		out = fmt.Sprintf("%s `%s`; write the table before it, as in `table.%s`", what, name, name)
	}
	_, short, _ := strings.Cut(name, ".")
	if short == "" {
		short = name
	}
	var pool []string
	switch what {
	case "no such column":
		pool = e.layout.columnNames()
	case "no such table":
		pool = e.layout.tableNames()
	}
	if c := names.Closest(short, pool); c != "" && c != short {
		out += fmt.Sprintf("; did you mean `%s`?", strings.Replace(name, short, c, 1))
	}
	return find(q, name), out
}

// named reports whether name is the name of a table or of a column.
func (e *Export) named(name string) bool {
	for _, n := range append(e.layout.tableNames(), e.layout.columnNames()...) {
		if n == name {
			return true
		}
	}
	return false
}

// find returns the offset of name in q, written as a word and ignoring case, outside strings, quoted names and
// comments. The first word of q is left out: it is the statement, not the name an error is about. The start of q
// when name is not there.
func find(q, name string) int {
	word := func(b byte) bool {
		return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
	}
	start := skip(q, 0)
	for i := start; i < len(q); {
		if j := next(q, i); j > i+1 {
			i = j // a string, a quoted name or a comment
			continue
		}
		end := i + len(name)
		if i > start && end <= len(q) && strings.EqualFold(q[i:end], name) &&
			(!word(q[i]) || !word(q[i-1])) && (end == len(q) || !word(q[end-1]) || !word(q[end])) {
			return i
		}
		i++
	}
	return start
}

// tableNames and columnNames are the names of the tables and of the columns of the layout, each once.
func (l *Layout) tableNames() []string {
	var ns []string
	for _, t := range l.tables {
		ns = append(ns, t.name)
	}
	return ns
}

func (l *Layout) columnNames() []string {
	var ns []string
	seen := map[string]bool{}
	for _, t := range l.tables {
		for _, c := range t.cols {
			if !seen[c.name] {
				seen[c.name] = true
				ns = append(ns, c.name)
			}
		}
	}
	return ns
}

// text writes a value of a result: NULL, a number as SQLite computed it, a text on one line, a BLOB by its size.
func text(v any) string {
	switch v := v.(type) {
	case nil:
		return "NULL"
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case []byte:
		if len(v) == 1 {
			return "<blob 1 byte>"
		}
		return fmt.Sprintf("<blob %d bytes>", len(v))
	}
	return v.(string)
}

// oneLine writes a text on one line, its line breaks and tabs escaped.
var oneLine = strings.NewReplacer("\r", `\r`, "\n", `\n`, "\t", `\t`)

// Table writes the result as an aligned table: the names of the columns, a rule, the rows, and their number.
// Numbers are aligned to the right, the rest to the left.
func (r *Result) Table(w io.Writer) error {
	n := len(r.Columns)
	cells := make([][]string, len(r.Rows))
	widths := make([]int, n)
	right := make([]bool, n)
	for j, c := range r.Columns {
		widths[j] = utf8.RuneCountInString(c)
		right[j] = len(r.Rows) > 0
	}
	for i, row := range r.Rows {
		cells[i] = make([]string, n)
		for j, v := range row {
			cells[i][j] = oneLine.Replace(text(v))
			widths[j] = max(widths[j], utf8.RuneCountInString(cells[i][j]))
			switch v.(type) {
			case int64, float64:
			default:
				right[j] = false
			}
		}
	}
	var b strings.Builder
	line := func(cells []string) {
		var parts []string
		for j, c := range cells {
			pad := strings.Repeat(" ", widths[j]-utf8.RuneCountInString(c))
			if right[j] {
				c = pad + c
			} else if j < n-1 {
				c += pad
			}
			parts = append(parts, c)
		}
		b.WriteString(strings.TrimRight(strings.Join(parts, "  "), " ") + "\n")
	}
	rule := make([]string, n)
	for j, wd := range widths {
		rule[j] = strings.Repeat("-", wd)
	}
	line(r.Columns)
	line(rule)
	for _, row := range cells {
		line(row)
	}
	word := "rows"
	if len(r.Rows) == 1 {
		word = "row"
	}
	fmt.Fprintf(&b, "%d %s\n", len(r.Rows), word)
	_, err := io.WriteString(w, b.String())
	return err
}

// CSV writes the result as comma-separated values, the names of the columns first; NULL is an empty value.
func (r *Result) CSV(w io.Writer) error {
	cw := csv.NewWriter(w)
	cw.Write(r.Columns)
	for _, row := range r.Rows {
		rec := make([]string, len(row))
		for j, v := range row {
			if v != nil {
				rec[j] = text(v)
			}
		}
		cw.Write(rec)
	}
	cw.Flush()
	return cw.Error()
}
