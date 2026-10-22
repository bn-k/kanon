package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/doc"
	"github.com/bn-k/kanon/internal/model"
	"github.com/bn-k/kanon/internal/names"
	"github.com/bn-k/kanon/internal/schema"
	"github.com/bn-k/kanon/internal/sheet"
	"github.com/bn-k/kanon/internal/value"
)

// root binds a whole file to s: its title, its content, then the folders next to it. in is the field of parent
// whose folder holds it, or nil for both.
func (l *linter) root(f *file, s *schema.Schema, in *schema.Field, parent *object) {
	id := strings.TrimSuffix(filepath.Base(f.path), ".kan")
	o := &object{schema: s, file: f, id: id, pos: diag.Pos{File: f.path}, level: 1, parent: parent, filled: map[string]diag.Pos{}}
	o.m = &model.Object{Schema: s, ID: id, Pos: o.pos, File: f.path}
	f.root = o
	if parent != nil {
		parent.hold(in, o)
	}
	if !names.IsSlug(id) {
		l.diags.Errorf(o.pos, "the file name is the object's id: write %s.kan, not \"%s\"", names.Slugify(id), filepath.Base(f.path))
	} else {
		f.ids[id] = o
	}
	sec := l.title(f.doc, o)
	l.object(sec, o)
	l.folder(o, strings.TrimSuffix(f.path, ".kan"))
	l.require(o)
}

// title returns the content of a document: what its H1 holds, plus anything written before it, which is reported.
// The H1's text fills the field marked @heading.
func (l *linter) title(d *doc.Doc, o *object) *doc.Section {
	var h1 *doc.Section
	var before []*doc.Section
	for _, c := range d.Root.Children {
		switch {
		case c.Heading.Level > 1 && h1 == nil:
			before = append(before, c)
		case h1 != nil:
			l.diags.Errorf(c.Heading.Pos, "a document has one title; make this heading a section: ## %s", c.Heading.Text)
		default:
			h1 = c
		}
	}
	if h1 == nil {
		return d.Root
	}
	if pos, ok := beforeTitle(d.Root, before); ok {
		l.diags.Errorf(pos, "this comes before the title; move it under # %s", h1.Heading.Text)
	}
	if h1.Heading.ID != "" {
		l.diags.Errorf(h1.Heading.IDPos, "a file's id is its name, `%s`; remove {#%s}", o.id, h1.Heading.ID)
	}
	l.headingText(h1.Heading, o)
	return &doc.Section{
		Heading:  h1.Heading,
		Keys:     append(slices.Clip(d.Root.Keys), h1.Keys...),
		Blocks:   append(slices.Clip(d.Root.Blocks), h1.Blocks...),
		Children: append(before, h1.Children...),
	}
}

// headingText fills the field of o marked @heading, if there is one, with the text of the heading h.
func (l *linter) headingText(h *doc.Heading, o *object) {
	if hf := o.schema.Annotated("heading"); hf != nil && h.Text != "" {
		l.mark(o, hf, h.TextPos)
		v, _ := value.Parse(value.Text, h.Text, l.units)
		mf := o.put(hf, h.TextPos)
		mf.Values = append(mf.Values, v)
	}
}

// beforeTitle returns where the content written before the title starts, if there is any.
func beforeTitle(root *doc.Section, before []*doc.Section) (diag.Pos, bool) {
	switch {
	case len(root.Keys) > 0:
		return root.Keys[0].Pos, true
	case len(root.Blocks) > 0:
		return root.Blocks[0].At(), true
	case len(before) > 0:
		return before[0].Heading.Pos, true
	}
	return diag.Pos{}, false
}

// object binds the content of a section to o: key lines, text for its @body, headings for its fields or its
// @sections.
func (l *linter) object(sec *doc.Section, o *object) {
	for _, k := range sec.Keys {
		l.keyLine(k, o)
	}
	if len(sec.Blocks) > 0 {
		if body := o.schema.Annotated("body"); body != nil {
			l.mark(o, body, sec.Blocks[0].At())
			l.prose(sec.Blocks, o)
			o.put(body, sec.Blocks[0].At()).Text = o.file.doc.Text(nil, sec.Blocks)
		} else {
			l.stray(sec.Blocks[0], o)
		}
	}
	for _, c := range sec.Children {
		l.heading(c, o)
	}
}

// stray reports text that fills no field.
func (l *linter) stray(b doc.Block, o *object) {
	var prose []string
	for _, f := range o.schema.Fields {
		if !f.Computed() && !f.Type.Array && f.Type.Native && f.Type.Kind == value.Prose {
			prose = append(prose, o.under(f))
		}
	}
	if len(prose) > 0 {
		l.diags.Errorf(b.At(), "this text fills no field; write it under %s", strings.Join(prose, " or "))
		return
	}
	l.diags.Errorf(b.At(), "this text fills no field: `%s` has no field for free text", o.schema.Name)
}

// heading binds a heading under o: the field it names, or else an entry of o's @sections. A heading names a field
// when its text, folded, is the field's name: "Dépôt de garantie" names depot-de-garantie.
func (l *linter) heading(c *doc.Section, o *object) {
	h := c.Heading
	slug := names.Slugify(h.Text)
	if f := o.schema.Field(slug); f != nil {
		l.field(c, f, o)
		return
	}
	if f := o.schema.Annotated("sections"); f != nil {
		if _, ok := o.filled[f.Name]; !ok {
			o.filled[f.Name] = h.Pos
		}
		l.section(c, f, o)
		return
	}
	msg := fmt.Sprintf("unknown heading \"%s\"", h.Text)
	if c := names.Closest(slug, o.schema.FieldNames()); c != "" {
		msg += fmt.Sprintf("; did you mean `%s`?", c)
	} else if hs := headings(o); len(hs) > 0 {
		msg += fmt.Sprintf("; `%s` takes %s", o.schema.Name, strings.Join(hs, ", "))
	}
	l.diags.Errorf(h.TextPos, "%s", msg)
}

// headings lists the headings o takes: its fields that are written under one.
func headings(o *object) []string {
	var hs []string
	for _, f := range o.schema.Fields {
		if !f.Computed() && !scalar(f.Type) {
			hs = append(hs, o.under(f))
		}
	}
	return hs
}

// scalar reports whether a field of type t is written as a key line, not under a heading.
func scalar(t *schema.Type) bool {
	return !t.Array && (t.Link || t.Schema == nil && t.Kind != value.Prose && t.Kind != value.Code)
}

// owned reports whether a field of type t holds its own objects, written inside the document, not linked to.
func owned(t *schema.Type) bool {
	return t.Schema != nil && !t.Link
}

// inverse is the message for an inverse written in a document. A field computed by an expression may be written,
// to be checked against its computation; an inverse may not.
func inverse(f *schema.Field) string {
	return fmt.Sprintf("`%s` is the inverse of `%s.%s`, computed from the documents; remove it", f.Name, f.Type.Name, f.Inverse)
}

// field binds the section under a heading that names the field f of o.
func (l *linter) field(c *doc.Section, f *schema.Field, o *object) {
	h := c.Heading
	t := f.Type
	switch {
	case f.Inverse != "":
		l.diags.Errorf(h.TextPos, "%s", inverse(f))
		return
	case scalar(t):
		msg := fmt.Sprintf("\"%s\" names the field `%s`, %s written as a key line: %s: %s", h.Text, f.Name, kind(t), f.Name, example(t))
		if o.schema.Annotated("sections") != nil {
			msg += "; to start a section, give it another title"
		}
		l.diags.Errorf(h.TextPos, "%s", msg)
		reported(o, f)
		return
	}
	l.mark(o, f, h.TextPos)
	if h.ID != "" && (t.Array || !owned(t)) {
		l.diags.Errorf(h.IDPos, "only an object has an id, and `%s` is %s; remove {#%s}", f.Name, kind(t), h.ID)
	}
	switch {
	case t.Array && owned(t):
		l.objects(c, f, o)
	case t.Array:
		l.values(c, f, o)
	case owned(t):
		sub := l.object0(f, o, h.Pos, h.Level)
		l.register(sub, h.ID, h.IDPos, h.Pos, fmt.Sprintf("give it an id, as in %s {#id}", atLevel(h.Level, h.Text)))
		l.object(c, sub)
		l.require(sub)
	case t.Kind == value.Code:
		l.code(c, f, o)
	default: // prose
		for _, k := range c.Keys {
			l.scan(k.Text, k.Pos, o)
		}
		l.prose(c.Blocks, o)
		o.put(f, h.TextPos).Text = o.file.doc.Text(c.Keys, c.Blocks)
		for _, sub := range c.Children {
			l.diags.Errorf(sub.Heading.Pos, "`%s` is prose and has no sections; use a list or bold text instead", f.Name)
		}
	}
}

// code binds the section of the Code field f of o: one fenced block, whose info string names its language.
func (l *linter) code(c *doc.Section, f *schema.Field, o *object) {
	wrong := func(pos diag.Pos) {
		l.diags.Errorf(pos, "`%s` is code: write it in a fenced block, between ``` lines", f.Name)
	}
	for _, k := range c.Keys {
		wrong(k.Pos)
		break
	}
	var first *doc.Code
	for _, b := range c.Blocks {
		code, ok := b.(*doc.Code)
		switch {
		case !ok:
			wrong(b.At())
		case first != nil:
			l.diags.Errorf(b.At(), "`%s` holds one code block; merge this one into the block at line %d", f.Name, first.Pos.Line)
		default:
			first = code
			mf := o.put(f, c.Heading.TextPos)
			mf.Text = code.Text()
			if lang := strings.Fields(code.Info); len(lang) > 0 {
				mf.Lang = lang[0]
			}
		}
	}
	for _, sub := range c.Children {
		wrong(sub.Heading.Pos)
	}
	if len(c.Keys)+len(c.Blocks)+len(c.Children) == 0 {
		l.diags.Errorf(c.Heading.TextPos, "`%s` holds no code; write it in a fenced block, between ``` lines", f.Name)
	}
}

// objects binds the entries of an array of objects written under its heading: tables, lists read through the
// schema's patterns, or one section per entry; or else a link to the spreadsheet that holds them, alone.
func (l *linter) objects(c *doc.Section, f *schema.Field, o *object) {
	es := f.Type.Schema
	level := c.Heading.Level + 1
	wrong := func(pos diag.Pos) {
		l.diags.Errorf(pos, "`%s` is a list of `%s`: write a table, a list or one section per entry", f.Name, es.Name)
	}
	for _, k := range c.Keys {
		wrong(k.Pos)
		break
	}
	for _, b := range c.Blocks {
		switch b := b.(type) {
		case *doc.Table:
			l.table(b, f, o, level)
		case *doc.List:
			if _, ok := linkValue(b.Items[0].Text()); ok {
				l.diags.Errorf(b.Pos, "`%s` holds its own `%s` objects, not links: write each one here, as a table, a list or one section per entry", f.Name, es.Name)
				continue
			}
			l.items(b, f, o, level)
		case *doc.Paragraph:
			target, _ := linkValue(b.Lines[0].Text)
			switch {
			case len(b.Lines) > 1 || !sheetLink(b.Lines[0].Text):
				wrong(b.At())
			case len(c.Keys)+len(c.Blocks)+len(c.Children) > 1:
				l.diags.Errorf(b.At(), "`%s` is in [[%s]]: write the link alone under its heading, or the entries here, not both", f.Name, target)
			default:
				l.spreadsheet(target, b.At(), f, o, level)
			}
		default:
			wrong(b.At())
		}
	}
	for _, sub := range c.Children {
		l.section(sub, f, o)
	}
}

// section binds a section that is an entry of the array in of parent: its heading gives the id and fills the
// @heading field.
func (l *linter) section(c *doc.Section, in *schema.Field, parent *object) {
	h := c.Heading
	o := l.object0(in, parent, h.Pos, h.Level)
	l.register(o, h.ID, h.IDPos, h.Pos, fmt.Sprintf("give it an id, as in %s {#id}", atLevel(h.Level, h.Text)))
	l.headingText(h, o)
	l.object(c, o)
	l.require(o)
}

// table binds the rows of a table, entries of the array in of parent.
func (l *linter) table(t *doc.Table, in *schema.Field, parent *object, level int) {
	l.grid(grid{what: "table", pos: t.Pos, header: t.Header, rows: t.Rows, starts: t.RowPos}, in, parent, level)
}

// spreadsheet binds the rows of the spreadsheet linked to at pos, entries of the array in of parent: target is
// mesures.csv, or catalogue.xlsx#produits for one sheet of a workbook, its first sheet without one.
func (l *linter) spreadsheet(target string, pos diag.Pos, in *schema.Field, parent *object, level int) {
	name, sheetName, _ := strings.Cut(target, "#")
	switch strings.ToLower(filepath.Ext(name)) {
	case ".csv", ".xlsx", ".xlsm":
	default:
		l.diags.Errorf(pos, "link `%s` to a .csv or .xlsx file, not to %s", in.Name, name)
		return
	}
	path := filepath.Join(filepath.Dir(parent.file.path), filepath.FromSlash(name))
	if !l.file(name, path, pos, value.File) {
		return
	}
	s, ds := sheet.Read(path, sheetName)
	l.diags = append(l.diags, ds...)
	switch {
	case s == nil: // reported
	case s.Header == nil:
		l.diags.Errorf(s.Pos(), "the sheet is empty: write the id column and the fields of `%s` in its first row", in.Type.Name)
	default:
		l.grid(sheetGrid(s), in, parent, level)
	}
}

// grid is the entries of an array written as a table or held in a sheet: a header of field names and an id
// column, then one object per row.
type grid struct {
	what   string   // table or sheet, for messages
	pos    diag.Pos // where it starts
	header []doc.Cell
	rows   [][]doc.Cell
	starts []diag.Pos        // where each row starts; nil for a sheet, whose rows start at their id
	broken map[diag.Pos]bool // the cells that can't be read, already reported
}

// sheetGrid is the grid of a sheet: its cells trimmed, its header as wide as its widest row.
func sheetGrid(s *sheet.Sheet) grid {
	g := grid{what: "sheet", pos: s.Header[0].Pos, broken: map[diag.Pos]bool{}}
	cells := func(row []sheet.Cell) []doc.Cell {
		cs := make([]doc.Cell, len(row))
		for i, c := range row {
			cs[i] = doc.Cell{Pos: c.Pos, Text: strings.TrimSpace(c.Text)}
			if c.Err {
				g.broken[c.Pos] = true
			}
		}
		return cs
	}
	g.header = cells(s.Header)
	for _, row := range s.Rows {
		g.rows = append(g.rows, cells(row))
		for len(g.header) < len(row) {
			g.header = append(g.header, doc.Cell{})
		}
	}
	return g
}

// grid binds the rows of g, entries of the array in of parent: one object per row, an id column and one column
// per field. A field without a column is reported once, for the whole grid, and not again for each row.
func (l *linter) grid(g grid, in *schema.Field, parent *object, level int) {
	s := in.Type.Schema
	cols := make([]*schema.Field, len(g.header))
	idCol := -1
	seen := map[string]bool{}
	unread := false // a header cell can't be read: it may stand for any column, so none is reported missing
	for i, h := range g.header {
		switch f := s.Field(h.Text); {
		case g.broken[h.Pos]:
			unread = true
		case h.Text == "":
			l.nameless(g, i)
		case seen[h.Text]:
			l.diags.Errorf(h.Pos, "the column `%s` is written twice", h.Text)
		case h.Text == "id":
			idCol = i
		case f == nil:
			msg, meant := unknownField(h.Text, s)
			l.diags.Errorf(h.Pos, "%s", msg)
			if meant != nil {
				seen[meant.Name] = true // reported once, not again as a missing column
			}
		case f.Inverse != "":
			l.diags.Errorf(h.Pos, "%s", inverse(f))
		case f.Type.Array:
			l.diags.Errorf(h.Pos, "a cell holds one value, and `%s` is a list", f.Name)
		case owned(f.Type):
			l.diags.Errorf(h.Pos, "a cell holds a value or a link, and `%s` holds its own `%s`: write one section per entry", f.Name, f.Type.Name)
		default:
			cols[i] = f
		}
		seen[h.Text] = true
	}
	if idCol < 0 && !unread {
		l.diags.Errorf(g.pos, "a %s of `%s` needs an id column", g.what, s.Name)
	}
	var absent []*schema.Field
	for _, f := range s.Fields {
		if slices.Contains(cols, f) {
			continue
		}
		absent = append(absent, f)
		switch {
		case seen[f.Name] || !f.Required():
		case f.Type.Array || owned(f.Type):
			l.diags.Errorf(g.pos, "a %s can't hold `%s`, which `%s` needs: write one section per entry", g.what, f.Name, s.Name)
		case unread:
		default:
			l.diags.Errorf(g.pos, "a %s of `%s` needs a column `%s`", g.what, s.Name, f.Name)
		}
	}
	for r, row := range g.rows {
		hasID := idCol >= 0 && idCol < len(row)
		var start diag.Pos
		switch {
		case g.starts != nil:
			start = g.starts[r]
		case hasID:
			start = row[idCol].Pos
		default:
			start = row[0].Pos
		}
		o := l.object0(in, parent, start, level)
		if hasID && !g.broken[row[idCol].Pos] {
			l.register(o, row[idCol].Text, row[idCol].Pos, row[idCol].Pos, "write its id in the id column")
		}
		for _, f := range absent {
			reported(o, f)
		}
		for i, f := range cols {
			switch {
			case f == nil:
			case i >= len(row) || g.broken[row[i].Pos]: // reported: a row narrower than its header, a cell that can't be read
				reported(o, f)
			case row[i].Text != "":
				l.mark(o, f, row[i].Pos)
				l.value(f, row[i].Text, row[i].Pos, o)
			case f.Required():
				l.diags.Errorf(row[i].Pos, "`%s` needs a value", f.Name)
				reported(o, f)
			}
		}
		l.require(o)
	}
}

// nameless reports the first value in the column i of g, whose header holds no name. An empty column is left
// alone.
func (l *linter) nameless(g grid, i int) {
	for _, row := range g.rows {
		if i < len(row) && row[i].Text != "" {
			l.diags.Errorf(row[i].Pos, "this cell is in a column with no name: write a field name at the top of its column, or clear it")
			return
		}
	}
}

// items binds the items of a list, entries of the array in of parent, each read through one of the schema's reads
// patterns.
func (l *linter) items(list *doc.List, in *schema.Field, parent *object, level int) {
	s := in.Type.Schema
	if len(s.Patterns) == 0 {
		l.diags.Errorf(list.Pos, "`%s` has no reads pattern, so a list can't hold it; write a table or one section per entry", s.Name)
		return
	}
	accept := func(field, v string) bool { return l.valid(s.Field(field).Type, v) }
	greedy := func(field string) bool { t := s.Field(field).Type; return !t.Native || t.Kind != value.Text }
	for _, it := range list.Items {
		o := l.object0(in, parent, it.Pos, level)
		l.register(o, it.ID, it.IDPos, it.Pos, fmt.Sprintf("write its id at the end, as in - %s {#id}", it.Text()))
		pos := it.Lines[0].Pos
		vals, ok := match(s, it.Text(), accept, greedy)
		if !ok {
			var ps []string
			for _, p := range s.Patterns {
				ps = append(ps, `"`+p.Text+`"`)
			}
			l.diags.Errorf(pos, "this item doesn't read as %s", strings.Join(ps, " or "))
			continue
		}
		for _, f := range s.Fields {
			if v, ok := vals[f.Name]; ok {
				l.mark(o, f, pos)
				l.value(f, v, pos, o)
			}
		}
		l.require(o)
	}
}

// match reads text through the first of the schema's patterns that fits.
func match(s *schema.Schema, text string, accept func(string, string) bool, greedy func(string) bool) (map[string]string, bool) {
	for _, p := range s.Patterns {
		if vals, ok := p.Match(text, accept, greedy); ok {
			return vals, true
		}
	}
	return nil, false
}

// values binds an array of plain values, links or files, written as a list: the steps of a recipe.
func (l *linter) values(c *doc.Section, f *schema.Field, o *object) {
	t := f.Type
	links := t.Link || fileKind(t)
	wrong := func(pos diag.Pos) {
		if links {
			l.diags.Errorf(pos, "`%s` is %s: write one link per item, as in - %s", f.Name, kind(t), linkExample(t))
			return
		}
		l.diags.Errorf(pos, "`%s` is %s: write one item per value", f.Name, kind(t))
	}
	for _, k := range c.Keys {
		wrong(k.Pos)
		break
	}
	for _, b := range c.Blocks {
		list, ok := b.(*doc.List)
		if !ok {
			wrong(b.At())
			continue
		}
		for _, it := range list.Items {
			switch {
			case it.ID != "" && links:
				l.diags.Errorf(it.IDPos, "a link has no id; remove {#%s}", it.ID)
			case it.ID != "":
				l.diags.Errorf(it.IDPos, "%s is a value, not an object: it has no id; remove {#%s}", names.Article(t.Name), it.ID)
			}
			l.value(f, it.Text(), it.Lines[0].Pos, o)
		}
	}
	for _, sub := range c.Children {
		wrong(sub.Heading.Pos)
	}
}

// prose checks the {field} insertions and the links of free text.
func (l *linter) prose(blocks []doc.Block, o *object) {
	for _, b := range blocks {
		switch b := b.(type) {
		case *doc.Paragraph:
			for _, line := range b.Lines {
				l.scan(line.Text, line.Pos, o)
			}
		case *doc.List:
			for _, it := range b.Items {
				for _, line := range it.Lines {
					l.scan(line.Text, line.Pos, o)
				}
			}
		case *doc.Table:
			for _, row := range b.Rows {
				for _, cell := range row {
					l.scan(cell.Text, cell.Pos, o)
				}
			}
		}
	}
}

// keyLine binds a key: value line to the field of o it names.
func (l *linter) keyLine(k *doc.KeyLine, o *object) {
	f := o.schema.Field(k.Key)
	switch {
	case f == nil:
		msg, meant := unknownField(k.Key, o.schema)
		l.diags.Errorf(k.Pos, "%s", msg)
		if meant != nil {
			reported(o, meant)
		}
	case f.Inverse != "":
		l.diags.Errorf(k.Pos, "%s", inverse(f))
	case f.Type.Array && owned(f.Type) && sheetLink(k.Value):
		l.mark(o, f, k.Pos)
		target, _ := linkValue(k.Value)
		l.spreadsheet(target, k.ValuePos, f, o, o.level+2)
	case f.Type.Array && !f.Type.Link && !fileKind(f.Type):
		msg := fmt.Sprintf("`%s` is a list: write it under its heading, %s", f.Name, o.under(f))
		if owned(f.Type) {
			msg += fmt.Sprintf(", or link to a spreadsheet, as in %s: [[%s.csv]]", f.Name, f.Name)
		}
		l.diags.Errorf(k.Pos, "%s", msg)
		reported(o, f)
	case owned(f.Type):
		l.diags.Errorf(k.Pos, "`%s` holds its own `%s`, not a link: write it under %s {#id}", f.Name, f.Type.Name, o.under(f))
		reported(o, f)
	case k.Value == "":
		l.mark(o, f, k.Pos)
		l.diags.Errorf(k.Pos, "`%s` has no value; write one, as in %s: %s", f.Name, f.Name, example(f.Type))
	case f.Type.Array:
		l.mark(o, f, k.Pos)
		for _, v := range split(k.Value, k.ValuePos) {
			l.value(f, v.text, v.pos, o)
		}
	default:
		l.mark(o, f, k.Pos)
		l.value(f, k.Value, k.ValuePos, o)
	}
}

// piece is one value of a key line that holds several, and where it starts.
type piece struct {
	text string
	pos  diag.Pos
}

// split cuts the value of a key line at the commas between its links: [[a]], [[b]].
func split(s string, pos diag.Pos) []piece {
	var ps []piece
	rs := []rune(s)
	start, depth := 0, 0
	for i := 0; i <= len(rs); i++ {
		switch {
		case i == len(rs) || rs[i] == ',' && depth == 0:
			text := string(rs[start:i])
			lead := len([]rune(text)) - len([]rune(strings.TrimLeft(text, " ")))
			ps = append(ps, piece{strings.TrimSpace(text), diag.Pos{File: pos.File, Line: pos.Line, Col: pos.Col + start + lead}})
			start = i + 1
		case strings.HasPrefix(string(rs[i:]), "[["):
			depth++
		case strings.HasPrefix(string(rs[i:]), "]]") && depth > 0:
			depth--
		}
	}
	return ps
}

// unknownField is the message for a key, a column or a folder that names no field of s. When the name plainly
// stands for a field, its name with accents or capitals, meant is that field.
func unknownField(name string, s *schema.Schema) (msg string, meant *schema.Field) {
	if name == "id" {
		return "an object's id is written in its heading ({#id}) or its file name, not as a key", nil
	}
	if f := s.Field(names.Slugify(name)); f != nil && !names.IsName(name) {
		return fmt.Sprintf("names are ASCII: write %s, not \"%s\"", f.Name, name), f
	}
	msg = fmt.Sprintf("`%s` has no field `%s`", s.Name, name)
	if c := names.Closest(name, s.FieldNames()); c != "" {
		msg += fmt.Sprintf("; did you mean `%s`?", c)
	}
	return msg, nil
}

// value checks a value written for the field f of o against its type, and adds it to o's model.
func (l *linter) value(f *schema.Field, s string, pos diag.Pos, o *object) {
	t := f.Type
	switch {
	case t.Enum != nil:
		if t.Enum.Value(s) == nil {
			l.diags.Errorf(pos, "%s", enumError(t.Enum, s))
			return
		}
		mf := o.put(f, pos)
		mf.Values = append(mf.Values, value.Value{Kind: value.Text, Raw: s})
	case t.Link:
		target, ok := linkValue(s)
		if !ok {
			l.diags.Errorf(pos, "this field holds %s: link to it, as in [[%s]]", names.Article(t.Schema.Name), names.Slugify(s))
			return
		}
		l.queueField(target, pos, f, o)
	case fileKind(t):
		l.fileValue(f, s, pos, o)
	case t.Kind == value.Prose:
		l.scan(s, pos, o)
		mf := o.put(f, pos)
		if t.Array {
			mf.Values = append(mf.Values, value.Value{Kind: value.Prose, Raw: s})
		} else {
			mf.Text = s
		}
	default:
		v, err := value.Parse(t.Kind, s, l.units)
		if err != nil {
			l.diags.Errorf(pos, "%s", err.Error())
			return
		}
		mf := o.put(f, pos)
		mf.Values = append(mf.Values, v)
	}
}

// fileValue checks a value of the File, Image or Code field f of o: a link to a file next to the document. The
// model holds its path, or for Code its text.
func (l *linter) fileValue(f *schema.Field, s string, pos diag.Pos, o *object) {
	t := f.Type
	target, ok := linkValue(s)
	switch {
	case !ok && t.Kind == value.Code:
		l.diags.Errorf(pos, "this field holds %s: link to its file, as in [[%s]], or write it under its heading as a code block",
			names.Article(t.Name), fileExamples[t.Kind])
	case !ok:
		l.diags.Errorf(pos, "this field holds %s: link to it, as in [[%s]]", names.Article(t.Name), fileExamples[t.Kind])
	case !fileLink(target):
		l.diags.Errorf(pos, "this field holds %s: link to a file, with its extension, as in [[%s]]", names.Article(t.Name), fileExamples[t.Kind])
	default:
		path := filepath.Join(filepath.Dir(o.file.path), filepath.FromSlash(target))
		l.file(target, path, pos, t.Kind)
		mf := o.put(f, pos)
		if t.Kind != value.Code {
			mf.Files = append(mf.Files, path)
			return
		}
		src, _ := os.ReadFile(path) // an unreadable file is reported
		mf.Text = string(src)
		mf.Lang = strings.TrimPrefix(filepath.Ext(path), ".")
	}
}

// valid reports whether s is a value of type t, without reporting anything.
func (l *linter) valid(t *schema.Type, s string) bool {
	if t.Enum != nil {
		return t.Enum.Value(s) != nil
	}
	_, err := value.Parse(t.Kind, s, l.units)
	return err == nil
}

// linkValue returns the target of a value that is exactly one link: [[clients/dupont-batiment]].
func linkValue(s string) (string, bool) {
	inner, ok := strings.CutPrefix(s, "[[")
	if !ok {
		return "", false
	}
	inner, ok = strings.CutSuffix(inner, "]]")
	return inner, ok && inner != "" && !strings.Contains(inner, "]]")
}

// sheetLink reports whether s is exactly one link to a file, as to a spreadsheet: [[mesures.csv]],
// [[catalogue.xlsx#produits]].
func sheetLink(s string) bool {
	target, ok := linkValue(s)
	name, _, _ := strings.Cut(target, "#")
	return ok && fileLink(name)
}

// enumError is the message for a value that isn't one of the enum's.
func enumError(e *schema.Enum, s string) string {
	for _, v := range e.Values {
		if names.Slugify(s) == v.Name {
			return fmt.Sprintf("write %s, not \"%s\"", v.Name, s)
		}
	}
	if c := names.Closest(s, e.ValueNames()); c != "" {
		return fmt.Sprintf("\"%s\" is not %s; did you mean %s?", s, names.Article(e.Name), c)
	}
	return fmt.Sprintf("\"%s\" is not %s: write %s", s, names.Article(e.Name), either(e.ValueNames()))
}

// either joins values for a message: neuf, bon or use.
func either(vs []string) string {
	if len(vs) < 2 {
		return strings.Join(vs, "")
	}
	return strings.Join(vs[:len(vs)-1], ", ") + " or " + vs[len(vs)-1]
}

// scan checks the {field} insertions and the links of a line of prose. A backslash escapes the next character;
// text between backticks is code and is not read.
func (l *linter) scan(text string, pos diag.Pos, o *object) {
	rs := []rune(text)
	at := func(i int) diag.Pos { return diag.Pos{File: pos.File, Line: pos.Line, Col: pos.Col + i} }
	code := false
	for i := 0; i < len(rs); i++ {
		switch {
		case rs[i] == '\\':
			i++
		case rs[i] == '`':
			code = !code
		case code:
		case rs[i] == '[' || rs[i] == '!' && i+1 < len(rs) && rs[i+1] == '[':
			end, ok := l.link(rs, i, at, o)
			if !ok {
				return
			}
			i = end
		case rs[i] == '{':
			end := index(rs, i+1, "}")
			if end < 0 {
				l.diags.Errorf(at(i), "unclosed {; close it with }, or write \\{ for a brace")
				return
			}
			l.insert(string(rs[i+1:end]), at(i+1), o)
			i = end
		}
	}
}

// link checks the link that starts at rs[i]: [[target]] and ![[target]], or Markdown's [text](target) and
// ![alt](target). It returns where the link ends, i when there is none, and false when it is never closed.
func (l *linter) link(rs []rune, i int, at func(int) diag.Pos, o *object) (int, bool) {
	start := i
	image := rs[i] == '!'
	if image {
		i++
	}
	if i+1 < len(rs) && rs[i+1] == '[' {
		end := index(rs, i+2, "]]")
		if end < 0 {
			l.diags.Errorf(at(i), "unclosed link; close it with ]]")
			return 0, false
		}
		l.queue(string(rs[i+2:end]), at(start), o.file)
		return end + 1, true
	}
	text := index(rs, i+1, "]")
	if text < 0 || text+1 == len(rs) || rs[text+1] != '(' {
		return start, true
	}
	end := index(rs, text+2, ")")
	if end < 0 {
		return start, true
	}
	l.scan(string(rs[i+1:text]), at(i+1), o)
	target, _, _ := strings.Cut(strings.TrimSpace(string(rs[text+2:end])), " ") // drop a title: (path "Title")
	l.target(target, at(start), image, o)
	return end, true
}

// scheme starts the address of a link to another site: https:, mailto:…
var scheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

// target checks the target of a Markdown link: a file next to the document, or a document. Links to another site
// and to a place in the page are not checked.
func (l *linter) target(target string, pos diag.Pos, image bool, o *object) {
	path, id, hasID := strings.Cut(target, "#")
	path, _, _ = strings.Cut(path, "?")
	switch {
	case path == "" || scheme.MatchString(path):
	case fileLink(path):
		k := value.File
		if image {
			k = value.Image
		}
		l.file(path, filepath.Join(filepath.Dir(o.file.path), filepath.FromSlash(path)), pos, k)
	case hasID:
		l.queue(strings.TrimSuffix(path, ".kan")+"#"+id, pos, o.file)
	default:
		l.queue(strings.TrimSuffix(path, ".kan"), pos, o.file)
	}
}

// index returns the index of sub in rs from i, or -1.
func index(rs []rune, i int, sub string) int {
	if j := strings.Index(string(rs[i:]), sub); j >= 0 {
		return i + len([]rune(string(rs[i:])[:j]))
	}
	return -1
}

// insert checks a {field} insertion: the field belongs to o or to an object that holds it.
func (l *linter) insert(name string, pos diag.Pos, o *object) {
	root, _, _ := strings.Cut(name, ".")
	var all []string
	for p := o; p != nil; p = p.parent {
		if p.schema.Field(root) != nil {
			return
		}
		all = append(all, p.schema.FieldNames()...)
	}
	msg := fmt.Sprintf("{%s} inserts no field", name)
	if c := names.Closest(root, all); c != "" {
		msg += fmt.Sprintf("; did you mean {%s}?", c)
	}
	l.diags.Errorf(pos, "%s", msg)
}

// object0 returns a new object held by the field in of parent, not yet registered; level is the level of its
// heading.
func (l *linter) object0(in *schema.Field, parent *object, pos diag.Pos, level int) *object {
	s := in.Type.Schema
	o := &object{schema: s, file: parent.file, pos: pos, level: level, parent: parent, filled: map[string]diag.Pos{}}
	o.m = &model.Object{Schema: s, Pos: pos, File: parent.file.path}
	parent.hold(in, o)
	return o
}

// hold adds o to the objects the field in of p holds, in p's model.
func (p *object) hold(in *schema.Field, o *object) {
	o.m.Parent, o.m.Holder = p.m, in
	mf := p.put(in, o.pos)
	mf.Objects = append(mf.Objects, o.m)
}

// put returns the field f of o's model, added when it is not there yet; pos is where it is written. A computed field
// written in the document is kept, for eval to check against its value; an inverse is reported before.
func (o *object) put(f *schema.Field, pos diag.Pos) *model.Field {
	return modelField(o.m, f, pos)
}

// modelField returns the field f of m, added in schema order when it is not there yet, at pos.
func modelField(m *model.Object, f *schema.Field, pos diag.Pos) *model.Field {
	at := slices.Index(m.Schema.Fields, f)
	i := len(m.Fields)
	for k, mf := range m.Fields {
		if j := slices.Index(m.Schema.Fields, mf.Field); j == at {
			return mf
		} else if j > at {
			i = k
			break
		}
	}
	mf := &model.Field{Field: f, Pos: pos}
	m.Fields = slices.Insert(m.Fields, i, mf)
	return mf
}

// under is the heading of the field f of o: one level below o's own.
func (o *object) under(f *schema.Field) string {
	return atLevel(o.level+1, names.Heading(f.Name))
}

// atLevel is a heading of the given level: ### Notes.
func atLevel(level int, text string) string {
	return strings.Repeat("#", level) + " " + text
}

// register gives o its written id, checked for shape and uniqueness within its file. how says how to write one.
func (l *linter) register(o *object, id string, idPos, pos diag.Pos, how string) {
	switch prev := o.file.ids[id]; {
	case id == "":
		l.diags.Errorf(pos, "every object has an id: this %s needs one; %s", "`"+o.schema.Name+"`", how)
	case !names.IsSlug(id):
		l.diags.Errorf(idPos, "an id is a slug of lowercase ASCII letters, digits and hyphens: write %s, not \"%s\"", names.Slugify(id), id)
	case prev != nil:
		l.diags.Errorf(idPos, "the id `%s` is already used%s", id, where(prev.pos, idPos))
	default:
		o.id, o.m.ID = id, id
		o.file.ids[id] = o
	}
}

// mark records that the field f of o is written at pos; a field is written once.
func (l *linter) mark(o *object, f *schema.Field, pos diag.Pos) {
	if prev, ok := o.filled[f.Name]; ok && prev.File != "" {
		l.diags.Errorf(pos, "`%s` is already written%s", f.Name, where(prev, pos))
		return
	}
	o.filled[f.Name] = pos
	o.put(f, pos)
}

// reported counts the field f of o as written, with no place: writing it went wrong and is already reported, so
// it is not reported again as missing, nor as written twice when it is then written right.
func reported(o *object, f *schema.Field) {
	if _, ok := o.filled[f.Name]; !ok {
		o.filled[f.Name] = diag.Pos{}
	}
}

// where tells where prev is, seen from pos. A sheet of a workbook, catalogue.xlsx#produits, has rows, not lines.
func where(prev, pos diag.Pos) string {
	switch {
	case prev.Line == 0:
		return ", as " + prev.File
	case prev.File != pos.File:
		return ", at " + prev.String()
	case strings.Contains(filepath.Base(prev.File), "#"):
		return fmt.Sprintf(", in row %d", prev.Line)
	}
	return fmt.Sprintf(", at line %d", prev.Line)
}

// require reports every field o must write and doesn't.
func (l *linter) require(o *object) {
	for _, f := range o.schema.Fields {
		if _, ok := o.filled[f.Name]; ok || !f.Required() || f.Has("sections") {
			continue
		}
		l.diags.Errorf(o.pos, "`%s` needs `%s`; %s", o.schema.Name, f.Name, howTo(f, o))
	}
}

// howTo tells how to write the field f of o.
func howTo(f *schema.Field, o *object) string {
	t := f.Type
	switch {
	case f.Has("heading") && o.pos.Line == 0:
		return "write it as the title: # …"
	case f.Has("body") && o.pos.Line == 0:
		return "write it as text under the title"
	case f.Has("body"):
		return "write it as text under the heading"
	case t.Array && (owned(t) || fileKind(t)) && o.pos.Line == 0:
		return fmt.Sprintf("write it under %s, or as the folder %s/%s/", o.under(f), o.id, f.Name)
	case t.Array && !t.Link, t.Native && t.Kind == value.Prose:
		return fmt.Sprintf("write it under %s", o.under(f))
	case owned(t):
		return fmt.Sprintf("write it under %s {#id}", o.under(f))
	case t.Native && t.Kind == value.Code:
		return fmt.Sprintf("write it under %s as a code block, or link to its file: %s: [[%s]]", o.under(f), f.Name, fileExamples[t.Kind])
	}
	return fmt.Sprintf("write %s: %s", f.Name, example(t))
}

var examples = map[value.Kind]string{
	value.Text: "…", value.Prose: "…", value.Int: "4", value.Number: "12.5", value.Bool: "yes or no",
	value.Date: "2026-10-22", value.DateTime: "2026-10-22 14:30", value.Duration: "3 years", value.Money: "1 250.50 EUR",
	value.Percent: "20 %", value.Quantity: "48 m2", value.Version: "1.2",
}

// example is a value of type t, to show in messages.
func example(t *schema.Type) string {
	switch {
	case t.Enum != nil:
		return either(t.Enum.ValueNames())
	case t.Array && (t.Link || fileKind(t)):
		return "[[…]], [[…]]"
	case t.Link, fileKind(t):
		return linkExample(t)
	}
	return examples[t.Kind]
}

// linkExample is a link a field of type t holds: [[…]] to a document, [[photo.jpg]] to a file.
func linkExample(t *schema.Type) string {
	if fileKind(t) {
		return "[[" + fileExamples[t.Kind] + "]]"
	}
	return "[[…]]"
}

// kind describes a type in a message: a Date, a list of Ligne, a link to a Partie.
func kind(t *schema.Type) string {
	switch {
	case t.Array && t.Link:
		return "a list of links to `" + t.Name + "`"
	case t.Array:
		return "a list of `" + t.Name + "`"
	case t.Link:
		return "a link to " + names.Article(t.Name)
	}
	return names.Article(t.Name)
}

// folder binds the folder next to a file object: one subfolder per array of objects, holding one file per entry,
// or per array of files, holding the files. Other files, such as the ones its fields link to, are left alone.
func (l *linter) folder(o *object, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		path := filepath.Join(dir, name)
		pos := diag.Pos{File: path}
		f := o.schema.Field(name)
		switch {
		case strings.HasPrefix(name, "."):
		case !e.IsDir():
			if strings.HasSuffix(name, ".kan") {
				l.diags.Errorf(pos, "this document belongs to no list: %s", lists(o, dir, name))
			}
		case f == nil:
			msg, _ := unknownField(name, o.schema)
			l.diags.Errorf(pos, "%s", msg)
		case f.Inverse != "":
			l.diags.Errorf(pos, "%s", inverse(f))
		case !f.Type.Array || !owned(f.Type) && !fileKind(f.Type):
			l.diags.Errorf(pos, "only a list of objects or of files can be a folder, and `%s` is %s", f.Name, kind(f.Type))
		default:
			if prev, ok := o.filled[f.Name]; ok && prev.File != "" {
				l.diags.Errorf(pos, "`%s` is written twice: here and%s", f.Name, strings.TrimPrefix(where(prev, pos), ","))
				continue
			}
			o.filled[f.Name] = pos
			o.put(f, pos)
			if owned(f.Type) {
				l.entries(o, f, path)
			} else {
				l.fileEntries(o, f, path)
			}
		}
	}
}

// lists tells where the document name, in the folder dir of o, could go: the folder of a list of objects of o.
func lists(o *object, dir, name string) string {
	for _, f := range o.schema.Fields {
		if f.Type.Array && owned(f.Type) && !f.Computed() {
			return "move it into a list's folder, as in " + filepath.ToSlash(filepath.Join(dir, f.Name, name))
		}
	}
	return fmt.Sprintf("`%s` has no list of objects", o.schema.Name)
}

// entries binds the files of a folder that holds the array f of o, in the order of their ids. Each entry is a file
// of its own: its id may be used in another folder, or in o's file.
func (l *linter) entries(o *object, f *schema.Field, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		l.diags.Errorf(diag.Pos{File: dir}, "cannot read the folder: %v", unwrap(err))
		return
	}
	ids := map[string]bool{}
	var files, folders []string
	for _, e := range entries {
		name := e.Name()
		switch id, isKan := strings.CutSuffix(name, ".kan"); {
		case strings.HasPrefix(name, "."):
		case e.IsDir():
			folders = append(folders, name)
		case isKan:
			ids[id] = true
			files = append(files, id)
		default:
			l.diags.Warnf(diag.Pos{File: filepath.Join(dir, name)}, "not a .kan file; ignored")
		}
	}
	slices.Sort(files)
	for _, id := range files {
		l.load(filepath.Join(dir, id+".kan"), f, o)
	}
	for _, name := range folders {
		if !ids[name] {
			l.diags.Errorf(diag.Pos{File: filepath.Join(dir, name)}, "this folder belongs to no entry: there is no %s.kan", name)
		}
	}
}
