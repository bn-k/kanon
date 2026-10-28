package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/doc"
	"github.com/bn-k/kanon/internal/lint"
	"github.com/bn-k/kanon/internal/model"
	"github.com/bn-k/kanon/internal/names"
	"github.com/bn-k/kanon/internal/schema"
	"github.com/bn-k/kanon/internal/value"
)

// setOptions are the arguments of a set command.
type setOptions struct {
	schema, doc string
	text        string       // the text of the document, as read
	sets        []assignment // in the order given
	today       time.Time
	dry         bool // -n: check, write nothing
	json        bool // -format json
}

// assignment is a field to set, path=value: a field of the document's root object, or of its object #id. An empty
// value removes the field.
type assignment struct {
	path, id, field, value string
}

// runSet changes fields of a document in its text, each by the least edit that does it, then checks the new text and
// writes it as kan write does: with the documents that link to it, and only when that finds no error. A change it
// can't make in place, it refuses, and it changes nothing.
func runSet(args []string, stdout, stderr io.Writer) int {
	o, err := setArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "kan: %v\n%s\n", err, usage)
		return 2
	}
	text, diags := apply(o)
	var written, checked []string
	if diags == nil {
		diags, checked = checkWrite(writeOptions{schema: o.schema, docs: []string{o.doc},
			texts: map[string][]byte{abs(o.doc): []byte(text)}, today: o.today})
		if diags.Count(diag.Error) == 0 && !o.dry && text != o.text {
			if err := writeFile(o.doc, []byte(text)); err != nil {
				fmt.Fprintf(stderr, "kan: %v\n", err)
				return 1
			}
			written = []string{o.doc}
		}
	}
	errs := diags.Count(diag.Error)
	out := outcome{OK: errs == 0, Written: written, Checked: checked, Diagnostics: diags}
	if o.json {
		out.Text = &text
	}
	if err := out.print(stdout, o.json, []string{o.doc}); err != nil {
		fmt.Fprintf(stderr, "kan: %v\n", err)
		return 1
	}
	if errs > 0 {
		return 1
	}
	return 0
}

// setArgs reads the arguments of a set command: -n, -today and -format, the schema, the document, and the fields to
// set, each written path=value.
func setArgs(args []string) (setOptions, error) {
	o := setOptions{today: time.Now()}
	var rest []string
	var err error
	for i := 0; i < len(args) && err == nil; i++ {
		switch arg := args[i]; {
		case arg == "-n":
			o.dry = true
		case arg == "-format":
			i++
			o.json, err = formatArg(args, i)
		case arg == "-today":
			i++
			o.today, err = todayArg(args, i)
		case strings.Contains(arg, "="):
			var a assignment
			a, err = assign(arg)
			if err == nil && slices.ContainsFunc(o.sets, func(b assignment) bool { return b.id == a.id && b.field == a.field }) {
				err = fmt.Errorf("%s is set twice", a.path)
			}
			o.sets = append(o.sets, a)
		default:
			rest = append(rest, arg)
		}
	}
	if err != nil {
		return o, err
	}
	if o.schema, o.doc, err = paths("set", rest); err != nil {
		return o, err
	}
	if len(o.sets) == 0 {
		return o, errors.New("set takes the fields to set after the document, as in status=done")
	}
	text, err := os.ReadFile(o.doc)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return o, fmt.Errorf("%s does not exist", o.doc)
	case err != nil:
		return o, fmt.Errorf("cannot read %s: %v", o.doc, errors.Unwrap(err))
	}
	o.text = string(text)
	return o, nil
}

// assign reads path=value: path is a field, status, or a field of an object, #ana.email; value is trimmed, and holds
// no line break.
func assign(arg string) (assignment, error) {
	path, v, _ := strings.Cut(arg, "=")
	a := assignment{path: path, field: path, value: strings.Trim(v, " \t")}
	if rest, ok := strings.CutPrefix(path, "#"); ok {
		i := strings.LastIndex(rest, ".")
		a.id, a.field = rest[:max(i, 0)], rest[i+1:]
		if i <= 0 {
			a.field = ""
		}
	}
	switch {
	case a.field == "" || strings.ContainsAny(a.field, ".# \t"):
		return a, fmt.Errorf("write the field to set as status or #id.status, not %q", path)
	case strings.ContainsAny(a.value, "\r\n"):
		return a, fmt.Errorf("the value of %s holds a line break: change text over lines with kan write", path)
	}
	return a, nil
}

// apply makes the changes of o in the text of its document, one after the other, and returns the new text. When
// some can't be made, it returns the text as read, and why for each; when lint can't read the document, what it
// finds.
func apply(o setOptions) (string, diag.List) {
	s, diags := load(o, o.text)
	if s == nil {
		return o.text, diags
	}
	text := o.text
	var refused diag.List
	for _, a := range o.sets {
		c, no := s.plan(a)
		if no == nil && c.edit != nil {
			next := strings.Join(c.edit(lines(text)), "")
			after, diags := load(o, next)
			if after == nil {
				return o.text, diags
			}
			if c.check != nil {
				no = c.check(after)
			}
			if no == nil {
				text, s = next, after
			}
		}
		if no != nil {
			refused = append(refused, *no)
		}
	}
	if refused != nil {
		refused.Sort()
		return o.text, refused
	}
	return text, nil
}

// setter is a document as kan set reads it: its objects, as lint reads them, and where they are written.
type setter struct {
	path  string // the document, as lint names it
	root  *model.Object
	objs  map[string]*model.Object // the objects written in the document, by id
	d     *doc.Doc
	top   int                     // the last line of the front matter, 0 without one
	secs  map[[2]int]*doc.Section // by the line and column of their heading
	rows  map[[2]int]row          // by the line and column of the row
	items map[[2]int]*doc.Item    // by the line and column of the item
}

// row is a row of a table: the table, and the index of the row.
type row struct {
	t *doc.Table
	i int
}

// at is where p is in its file.
func at(p diag.Pos) [2]int { return [2]int{p.Line, p.Col} }

// load reads the document of o with text as its text, as lint does, through the document that owns its folder if
// there is one. It returns nil when lint can't read it, with what lint finds.
func load(o setOptions, text string) (*setter, diag.List) {
	fsys := edited{abs(o.doc): []byte(text)}
	m, r := lint.LoadWith(o.schema, owner(o.doc, fsys), lint.Options{Today: o.today, FS: fsys, Partial: true})
	if m == nil {
		return nil, r.Diags
	}
	s := &setter{objs: map[string]*model.Object{}, secs: map[[2]int]*doc.Section{}, rows: map[[2]int]row{},
		items: map[[2]int]*doc.Item{}}
	for obj := range m.Objects() {
		switch {
		case abs(obj.File) != abs(o.doc):
		case obj.Pos.Line == 0:
			s.root = obj
		case s.objs[obj.ID] == nil:
			s.objs[obj.ID] = obj
		}
	}
	if s.root == nil {
		if r.Diags.Count(diag.Error) == 0 {
			r.Diags.Errorf(diag.Pos{File: o.doc}, "this document has no type and is in no folder of a list: give it "+
				"its type in its front matter")
		}
		return nil, r.Diags
	}
	s.path = s.root.Pos.File
	s.d, _ = doc.Parse(s.path, text)
	if s.d.HasFrontMatter {
		ls := lines(text)
		for s.top = 2; strings.TrimRight(content(ls[s.top-1]), " \t") != "---"; s.top++ {
		}
	}
	var walk func(sec *doc.Section)
	walk = func(sec *doc.Section) {
		if sec.Heading != nil {
			s.secs[at(sec.Heading.Pos)] = sec
		}
		for _, b := range sec.Blocks {
			switch b := b.(type) {
			case *doc.Table:
				for i, p := range b.RowPos {
					s.rows[at(p)] = row{b, i}
				}
			case *doc.List:
				for _, it := range b.Items {
					s.items[at(it.Pos)] = it
				}
			}
		}
		for _, c := range sec.Children {
			walk(c)
		}
	}
	walk(s.d.Root)
	return s, nil
}

// change is how an assignment changes the text: an edit of its lines, none when the text stays as it is, and for a
// list item, a check that it reads as meant after the edit.
type change struct {
	edit  func(ls []string) []string
	check func(after *setter) *diag.Diagnostic
}

// refuse is a change refused, at pos: it is an error, and nothing is changed.
func refuse(pos diag.Pos, format string, args ...any) *diag.Diagnostic {
	return &diag.Diagnostic{Pos: pos, Severity: diag.Error, Msg: fmt.Sprintf(format, args...)}
}

// plan returns how a changes the text, or why it can't. A value is set where the document writes it: a key line, a
// table cell, a heading or a list item. Prose, lists, objects and what the documents compute are refused: they are
// changed with kan write, or through the fields they come from.
func (s *setter) plan(a assignment) (change, *diag.Diagnostic) {
	o := s.root
	if a.id != "" {
		if o = s.objs[a.id]; o == nil {
			msg := fmt.Sprintf("unknown id `%s`", a.id)
			if c := names.Closest(a.id, slices.Sorted(maps.Keys(s.objs))); c != "" {
				msg += fmt.Sprintf("; did you mean `%s`?", c)
			}
			return change{}, refuse(diag.Pos{File: s.path}, "%s", msg)
		}
		if o.Pos.File != s.path {
			return change{}, refuse(diag.Pos{File: s.path}, "`#%s` is a row of %s; change it there", a.id, o.Pos.File)
		}
	}
	f := o.Schema.Field(a.field)
	if f == nil {
		return change{}, refuse(o.Pos, "%s", lint.UnknownField(a.field, o.Schema))
	}
	mf := o.Get(f.Name)
	where := o.Pos
	if written := mf != nil && mf.Pos != (diag.Pos{}); written {
		where = mf.Pos
	}
	t := f.Type
	switch {
	case f.Inverse != "":
		return change{}, refuse(o.Pos, "`%s` is the inverse of `%s.%s`, computed from the documents; set `%s` on each `%s` instead",
			f.Name, t.Name, f.Inverse, f.Inverse, t.Name)
	case f.Expr != nil && where == o.Pos:
		return change{}, refuse(o.Pos, "`%s` is computed; set the fields it is computed from instead", f.Name)
	case t.Schema != nil && !t.Link && t.Array:
		return change{}, refuse(where, "`%s` holds `%s` objects, not a value; set their fields one at a time, or change "+
			"them with kan write", f.Name, t.Name)
	case t.Schema != nil && !t.Link:
		return change{}, refuse(where, "`%s` holds %s, not a value; set its fields one at a time, or change it with kan "+
			"write", f.Name, names.Article(t.Name))
	case t.Native && t.Kind == value.Prose && !t.Array && f.Has("body"):
		return change{}, refuse(where, "`%s` is prose, written as text under the heading; change it with kan write", f.Name)
	case t.Native && t.Kind == value.Prose && !t.Array:
		return change{}, refuse(where, "`%s` is prose, written under its heading; change it with kan write", f.Name)
	}
	if r, ok := s.rows[at(o.Pos)]; ok {
		return s.cell(r, o, f, a.value)
	}
	if it := s.items[at(o.Pos)]; it != nil {
		return item(it, o, f, a.value)
	}
	sec := s.secs[at(o.Pos)]
	if o == s.root {
		sec = title(s.d)
	}
	if f.Has("heading") && (o.Holder == nil || o.Holder.Type.Array) {
		return s.heading(sec, o, f, a.value)
	}
	return s.key(sec, o, f, where, a.value)
}

// title returns the section that holds the fields of a document's root object: what its title holds, with what
// comes before it, or the whole document when it has no title.
func title(d *doc.Doc) *doc.Section {
	for _, c := range d.Root.Children {
		if c.Heading.Level == 1 {
			return &doc.Section{Heading: c.Heading, Keys: append(slices.Clip(d.Root.Keys), c.Keys...)}
		}
	}
	return d.Root
}

// needs is the refusal to remove the field f of o, which its schema requires.
func needs(pos diag.Pos, o *model.Object, f *schema.Field) *diag.Diagnostic {
	return refuse(pos, "`%s` needs `%s`, so it can't be removed", o.Schema.Name, f.Name)
}

// key sets the field f of o, written in the section sec, on its key line: it replaces the value, removes the line
// for an empty value, or adds the line where a person would write it, after the key lines of the fields before f in
// the schema. where is where f is written, if it is, or else o.
func (s *setter) key(sec *doc.Section, o *model.Object, f *schema.Field, where diag.Pos, v string) (change, *diag.Diagnostic) {
	var k *doc.KeyLine
	for _, kl := range sec.Keys {
		if kl.Key == f.Name {
			k = kl
			break
		}
	}
	t := f.Type
	switch {
	case k != nil && v == "" && f.Required():
		return change{}, needs(k.Pos, o, f)
	case k != nil && v == "":
		return change{edit: remove(k.Pos.Line)}, nil
	case k != nil && k.Value == "":
		return change{edit: rewrite(k.Pos.Line, f.Name+": "+v)}, nil
	case k != nil:
		return change{edit: splice(k.ValuePos, k.Value, v)}, nil
	case where != o.Pos && where.Line == 0:
		return change{}, refuse(where, "`%s` is the folder %s; change its files", f.Name, where.File)
	case t.Native && t.Kind == value.Code:
		return change{}, refuse(where, "`%s` is code, written under its heading; change it with kan write", f.Name)
	case t.Array && (where != o.Pos || textual(t)):
		return change{}, refuse(where, "`%s` is a list, written under its heading; change it with kan write", f.Name)
	case v == "" && f.Required():
		return change{}, needs(o.Pos, o, f)
	case v == "":
		return change{}, nil
	}
	line := f.Name + ": " + v
	order := slices.Index(o.Schema.Fields, f)
	var after *doc.KeyLine
	for _, kl := range sec.Keys {
		if g := o.Schema.Field(kl.Key); g != nil && slices.Index(o.Schema.Fields, g) < order {
			after = kl
		}
	}
	switch {
	case after != nil:
		return change{edit: add(after.Pos.Line, line)}, nil
	case len(sec.Keys) > 0:
		return change{edit: add(sec.Keys[0].Pos.Line-1, line)}, nil
	case sec.Heading != nil:
		return change{edit: apart(sec.Heading.Pos.Line, line)}, nil
	}
	return change{edit: apart(s.top, line)}, nil
}

// textual reports whether a value of type t is text, which may hold a comma: an array of it is never written on a
// key line.
func textual(t *schema.Type) bool {
	return t.Native && (t.Kind == value.Text || t.Kind == value.Prose)
}

// heading sets the field f of o marked @heading: the text of the heading of its section sec, or the title of the
// document, added when it has none and removed for an empty value. A section keeps its heading, with its id alone.
func (s *setter) heading(sec *doc.Section, o *model.Object, f *schema.Field, v string) (change, *diag.Diagnostic) {
	h := sec.Heading
	switch {
	case v == "" && f.Required() && h != nil && h.Text != "":
		return change{}, needs(h.TextPos, o, f)
	case v == "" && f.Required():
		return change{}, needs(o.Pos, o, f)
	case h == nil && v == "":
		return change{}, nil
	case h == nil:
		return change{edit: apart(s.top, "# "+v)}, nil
	case v == "" && h.ID == "":
		return change{edit: remove(h.Pos.Line)}, nil
	case h.Text != "" && v != "":
		return change{edit: splice(h.TextPos, h.Text, v)}, nil
	}
	line := strings.Repeat("#", h.Level)
	if v != "" {
		line += " " + v
	}
	if h.ID != "" {
		line += " {#" + h.ID + "}"
	}
	return change{edit: rewrite(h.Pos.Line, line)}, nil
}

// cell sets the field f of o, a table row r, in its cell. In a table aligned by hand, its pipes in the same columns
// on every line, the cell keeps them there, and the column widens on every line when the value doesn't fit.
func (s *setter) cell(r row, o *model.Object, f *schema.Field, v string) (change, *diag.Diagnostic) {
	j := slices.IndexFunc(r.t.Header, func(c doc.Cell) bool { return c.Text == f.Name })
	cells := r.t.Rows[r.i]
	switch {
	case v == "" && f.Required() && j >= 0 && j < len(cells):
		return change{}, needs(cells[j].Pos, o, f)
	case v == "" && f.Required():
		return change{}, needs(o.Pos, o, f)
	case j < 0 && v == "":
		return change{}, nil
	case j < 0:
		return change{}, refuse(o.Pos, "the table has no column `%s`; add it with kan write", f.Name)
	case j >= len(cells):
		return change{}, refuse(o.Pos, "this row has no cell for `%s`; fix it with kan write", f.Name)
	}
	old := strings.ReplaceAll(cells[j].Text, "|", `\|`)
	c := strings.ReplaceAll(v, "|", `\|`)
	first, last, n := r.t.Pos.Line-1, r.t.End()-1, r.t.RowPos[r.i].Line-1
	return change{edit: func(ls []string) []string {
		ps := pipes(ls[n])
		closing := j+1 < len(ps)
		aligned := true
		for m := first; m <= last; m++ {
			aligned = aligned && slices.Equal(pipes(ls[m]), ps)
		}
		rs := []rune(content(ls[n]))
		from, to := ps[j]+1, len(rs)
		if closing {
			to = ps[j+1]
		}
		seg := string(rs[from:to])
		var cell string
		switch {
		case !closing && c == "":
			cell = " |" // a last cell left empty, and without a closing pipe, would be no cell
		case !closing || old != "" && c != "" && !aligned:
			return splice(cells[j].Pos, old, c)(ls)
		case !aligned && c == "":
			cell = " "
		case !aligned:
			cell = " " + c + " "
		default:
			w := to - from
			lead := utf8.RuneCountInString(seg) - utf8.RuneCountInString(strings.TrimLeft(seg, " "))
			if strings.Trim(seg, " \t") == "" {
				lead = min(1, w)
			}
			if need := lead + utf8.RuneCountInString(c) + lead; need > w {
				for m := first; m <= last; m++ {
					if m != n {
						ls[m] = widen(ls[m], j, need-w)
					}
				}
				w = need
			}
			cell = strings.Repeat(" ", lead) + c
			cell += strings.Repeat(" ", w-utf8.RuneCountInString(cell))
		}
		ls[n] = string(rs[:from]) + cell + string(rs[to:]) + ending(ls[n])
		return ls
	}}, nil
}

// pipes returns the columns, from 0, of the pipes of a table line that separate its cells: not those escaped, \|.
func pipes(line string) []int {
	var ps []int
	rs := []rune(content(line))
	for i := 0; i < len(rs); i++ {
		switch {
		case rs[i] == '\\' && i+1 < len(rs) && rs[i+1] == '|':
			i++
		case rs[i] == '|':
			ps = append(ps, i)
		}
	}
	return ps
}

// widen widens the cell j of a table line by d: with dashes on the separator line, with spaces before its closing
// pipe on the others.
func widen(line string, j, d int) string {
	ps := pipes(line)
	rs := []rune(content(line))
	seg := string(rs[ps[j]+1 : ps[j+1]])
	at, fill := ps[j+1], strings.Repeat(" ", d)
	if s := strings.Trim(strings.Trim(seg, " \t"), ":"); s != "" && strings.Trim(s, "-") == "" {
		at, fill = ps[j]+1+strings.IndexRune(seg, '-')+1, strings.Repeat("-", d)
	}
	return string(rs[:at]) + fill + string(rs[at:]) + ending(line)
}

// item sets the field f of o, a list item it read through a pattern, where its value is written in the item. The
// check after the edit makes sure the item reads the new value, and the same values as before for its other fields.
func item(it *doc.Item, o *model.Object, f *schema.Field, v string) (change, *diag.Diagnostic) {
	mf := o.Get(f.Name)
	if mf != nil && len(mf.Values) == 1 && mf.Values[0].Raw != "" && v != "" {
		raw := mf.Values[0].Raw
		for _, l := range it.Lines {
			if i := strings.Index(l.Text, raw); i >= 0 {
				pos := diag.Pos{Line: l.Pos.Line, Col: l.Pos.Col + utf8.RuneCountInString(l.Text[:i])}
				return change{edit: splice(pos, raw, v), check: reads(o, f, v)}, nil
			}
		}
	}
	return change{}, refuse(o.Pos, "`%s` is read from the item; change the item with kan write", f.Name)
}

// reads returns the check that the list item of o reads v for its field f after the edit, and its other fields as
// they were. An item that no longer reads is not refused: lint says why.
func reads(o *model.Object, f *schema.Field, v string) func(*setter) *diag.Diagnostic {
	return func(after *setter) *diag.Diagnostic {
		n := after.objs[o.ID]
		if n == nil || len(n.Fields) == 0 {
			return nil
		}
		for _, g := range o.Schema.Fields {
			want := raw(o.Get(g.Name))
			if g == f {
				want = v
			}
			if !g.Computed() && raw(n.Get(g.Name)) != want {
				return refuse(o.Pos, "the item would read differently; change it with kan write")
			}
		}
		return nil
	}
}

// raw returns the value of mf as written, "" when it has none: a field of a list item holds one value at most.
func raw(mf *model.Field) string {
	if mf == nil || len(mf.Values) == 0 {
		return ""
	}
	return mf.Values[0].Raw
}

// lines splits text into its lines, each with its line ending; the last one has none when text doesn't end with
// one.
func lines(text string) []string {
	ls := strings.SplitAfter(text, "\n")
	if ls[len(ls)-1] == "" {
		ls = ls[:len(ls)-1]
	}
	return ls
}

// content returns line without its line ending, as doc.Parse reads it.
func content(line string) string {
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
}

// ending returns the line ending of line: "\n", "\r\n", or none for the last line of a text that has none.
func ending(line string) string { return line[len(content(line)):] }

// blank reports whether line holds nothing but spaces.
func blank(line string) bool { return strings.Trim(content(line), " \t") == "" }

// splice replaces old, written at pos, by text.
func splice(pos diag.Pos, old, text string) func([]string) []string {
	return func(ls []string) []string {
		rs := []rune(ls[pos.Line-1])
		i := pos.Col - 1
		ls[pos.Line-1] = string(rs[:i]) + text + string(rs[i+utf8.RuneCountInString(old):])
		return ls
	}
}

// rewrite replaces line n, from 1, by text.
func rewrite(n int, text string) func([]string) []string {
	return func(ls []string) []string {
		ls[n-1] = text + ending(ls[n-1])
		return ls
	}
}

// remove removes line n, from 1, and a blank line beside it when it would leave two blank lines in a row, or one
// at the start or the end of the text.
func remove(n int) func([]string) []string {
	return func(ls []string) []string {
		i := n - 1
		last := ending(ls[i]) == ""
		ls = slices.Delete(ls, i, i+1)
		switch {
		case i < len(ls) && blank(ls[i]) && (i == 0 || blank(ls[i-1])):
			ls = slices.Delete(ls, i, i+1)
		case i == len(ls) && i > 0 && blank(ls[i-1]):
			ls = slices.Delete(ls, i-1, i)
		}
		if last && len(ls) > 0 {
			ls[len(ls)-1] = content(ls[len(ls)-1])
		}
		return ls
	}
}

// add inserts text as a line after line n, from 1; 0 inserts it first.
func add(n int, text string) func([]string) []string {
	return func(ls []string) []string { return insert(ls, n, text) }
}

// apart inserts text as a line after line n, from 1, with a blank line before and after it where there is none: a
// key line under a heading, a title after the front matter.
func apart(n int, text string) func([]string) []string {
	return func(ls []string) []string {
		i := n
		news := []string{text}
		if i > 0 && !blank(ls[i-1]) {
			news = append([]string{""}, news...)
		}
		if i < len(ls) && !blank(ls[i]) {
			news = append(news, "")
		}
		return insert(ls, i, news...)
	}
}

// insert inserts lines at the index i of ls, with the line ending of the text; the last line of a text that ends
// without one still does.
func insert(ls []string, i int, lines ...string) []string {
	eol := "\n"
	if len(ls) > 0 && strings.HasSuffix(ls[0], "\r\n") {
		eol = "\r\n"
	}
	news := make([]string, len(lines))
	for j, l := range lines {
		news[j] = l + eol
	}
	if i == len(ls) && i > 0 && ending(ls[i-1]) == "" {
		ls[i-1] += eol
		news[len(news)-1] = lines[len(lines)-1]
	}
	return slices.Insert(ls, i, news...)
}
