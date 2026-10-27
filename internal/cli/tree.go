package cli

import (
	"cmp"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/doc"
	"github.com/bn-k/kanon/internal/eval"
	"github.com/bn-k/kanon/internal/model"
	"github.com/bn-k/kanon/internal/schema"
	"github.com/bn-k/kanon/internal/value"
)

// object is an object as kan eval -format json prints it: its id, its schema, the file and the line it is written
// on, none for a whole file, and every field of its schema, in schema order.
type object struct {
	ID     string  `json:"id"`
	Type   string  `json:"type"`
	File   string  `json:"file"`
	Line   int     `json:"line,omitempty"`
	Fields []field `json:"fields"`
}

// field is a field of an object as kan eval -format json prints it. Text is its value in Kanon's canonical text,
// Value the same value typed for a program; both are null for a field not written, and [] for an empty array.
// Its span says where its value is written, when it is: not for a field that holds objects, which say where they
// are, nor for an inverse or a computed field not written.
type field struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // as the schema writes it: Money, [Line], -> Party, Text?
	Kind     string `json:"kind"` // object, link, enum, or the native type in lowercase: text, money, date…
	Computed bool   `json:"computed,omitempty"`
	Inverse  bool   `json:"inverse,omitempty"`
	Text     any    `json:"text"`
	Value    any    `json:"value"`
	span
}

// span is where a value is written: from line:col to end_line:end_col, the column just after its last rune.
type span struct {
	Line    int `json:"line,omitempty"`
	Col     int `json:"col,omitempty"`
	EndLine int `json:"end_line,omitempty"`
	EndCol  int `json:"end_col,omitempty"`
}

// target is the value of a link: the file that holds the object it leads to, and its id.
type target struct {
	File string `json:"file"`
	ID   string `json:"id"`
}

// code is the value of a Code field: its language, "" when none is given, and its text.
type code struct {
	Lang string `json:"lang"`
	Text string `json:"text"`
}

// tree turns the model of a document into the objects kan eval -format json prints.
type tree struct {
	dir   string            // the folder of the schema, where a link written [[/…]] starts
	files map[string]*index // the .kan files read, to find where values are written
}

func newTree(schemaPath string) *tree {
	return &tree{dir: abs(filepath.Dir(schemaPath)), files: map[string]*index{}}
}

// documents returns the document of d, then each document its links lead to; none when d is nil.
func (t *tree) documents(d *model.Doc) []object {
	objs := []object{}
	if d == nil {
		return objs
	}
	for _, o := range append([]*model.Object{d.Root}, d.Linked...) {
		objs = append(objs, t.object(o))
	}
	return objs
}

func (t *tree) object(o *model.Object) object {
	obj := object{ID: o.ID, Type: o.Schema.Name, File: holder(o), Line: o.Pos.Line, Fields: []field{}}
	for _, f := range o.Schema.Fields {
		obj.Fields = append(obj.Fields, t.field(f, o.Get(f.Name)))
	}
	return obj
}

// holder returns the file that holds o: its .kan file, or the spreadsheet of its row.
func holder(o *model.Object) string { return cmp.Or(o.Pos.File, o.File) }

// field returns the field f of an object, mf in its model, nil when it isn't written.
func (t *tree) field(f *schema.Field, mf *model.Field) field {
	fd := field{Name: f.Name, Type: f.Type.String(), Kind: kindOf(f), Computed: f.Expr != nil, Inverse: f.Inverse != ""}
	if mf == nil {
		if f.Type.Array {
			fd.Text, fd.Value = []any{}, []any{}
		}
		return fd
	}
	var texts, values []any
	for _, v := range mf.Values {
		texts, values = append(texts, eval.Format(v)), append(values, eval.JSON(v))
	}
	for _, o := range mf.Objects {
		texts, values = append(texts, o.ID), append(values, t.object(o))
	}
	for _, o := range mf.Links {
		texts, values = append(texts, t.link(o)), append(values, target{holder(o), o.ID})
	}
	for _, path := range mf.Files {
		texts, values = append(texts, "[["+t.rel(path)+"]]"), append(values, path)
	}
	switch {
	case f.Type.Native && f.Type.Kind == value.Code:
		texts, values = []any{mf.Text}, []any{code{mf.Lang, mf.Text}}
	case f.Type.Native && f.Type.Kind == value.Prose && !f.Type.Array:
		texts, values = []any{mf.Text}, []any{mf.Text}
	}
	switch {
	case f.Type.Array:
		fd.Text, fd.Value = nonNil(texts), nonNil(values)
	case len(texts) > 0:
		fd.Text, fd.Value = texts[0], values[0]
	}
	if fd.Kind != "object" && strings.HasSuffix(mf.Pos.File, ".kan") {
		fd.span = t.index(mf.Pos.File).span(f, mf)
	}
	return fd
}

// kindOf returns what the field f holds: objects of its own, links to objects written elsewhere (a computed field
// or an inverse that gives objects gives links), an enum value, or a native value.
func kindOf(f *schema.Field) string {
	t := f.Type
	switch {
	case t.Link || t.Schema != nil && f.Computed():
		return "link"
	case t.Schema != nil:
		return "object"
	case t.Enum != nil:
		return "enum"
	}
	return strings.ToLower(t.Kind.String())
}

// link returns a link to o as written anywhere under the schema's folder: [[/board#ana]], or [[/board]] to a whole
// file.
func (t *tree) link(o *model.Object) string {
	s := t.rel(strings.TrimSuffix(o.File, ".kan"))
	if o.Pos.Line > 0 {
		s += "#" + o.ID
	}
	return "[[" + s + "]]"
}

// rel returns path from the schema's folder, as a link writes it: /board/tasks.
func (t *tree) rel(path string) string {
	r, _ := filepath.Rel(t.dir, abs(path))
	return "/" + filepath.ToSlash(r)
}

// index returns the index of the .kan file at path, read once.
func (t *tree) index(path string) *index {
	x, ok := t.files[path]
	if !ok {
		src, _ := os.ReadFile(path)
		x = newIndex(path, string(src))
		t.files[path] = x
	}
	return x
}

// index finds where the values of the fields of a .kan file are written, from the positions lint gives the
// fields: a key line, a table cell, a heading, the first block of a @body or a list item.
type index struct {
	d      *doc.Doc
	keys   map[diag.Pos]*doc.KeyLine
	cells  map[diag.Pos]string
	heads  map[diag.Pos]*doc.Section // by the position of the heading's text
	blocks map[diag.Pos][]doc.Block  // by the position of a block: it and the blocks after it in its section
	items  map[diag.Pos]*doc.Item    // by the position of the item's text
}

func newIndex(path, src string) *index {
	d, _ := doc.Parse(path, src)
	x := &index{
		d:      d,
		keys:   map[diag.Pos]*doc.KeyLine{},
		cells:  map[diag.Pos]string{},
		heads:  map[diag.Pos]*doc.Section{},
		blocks: map[diag.Pos][]doc.Block{},
		items:  map[diag.Pos]*doc.Item{},
	}
	var walk func(s *doc.Section)
	walk = func(s *doc.Section) {
		if s.Heading != nil {
			x.heads[s.Heading.TextPos] = s
		}
		for _, k := range s.Keys {
			x.keys[k.Pos] = k
		}
		for i, b := range s.Blocks {
			x.blocks[b.At()] = s.Blocks[i:]
			switch b := b.(type) {
			case *doc.Table:
				for _, row := range b.Rows {
					for _, c := range row {
						x.cells[c.Pos] = c.Text
					}
				}
			case *doc.List:
				for _, it := range b.Items {
					x.items[it.Lines[0].Pos] = it
				}
			}
		}
		for _, c := range s.Children {
			walk(c)
		}
	}
	walk(d.Root)
	return x
}

// span returns where the value of the field f, mf in the model, is written: the value of its key line, its cell,
// its heading's text for a @heading, what its heading holds, its @body, or its part of a list item.
func (x *index) span(f *schema.Field, mf *model.Field) span {
	p := mf.Pos
	if k := x.keys[p]; k != nil {
		return run(k.ValuePos, k.Value)
	}
	if text, ok := x.cells[p]; ok {
		return run(p, strings.ReplaceAll(text, "|", `\|`))
	}
	if s := x.heads[p]; s != nil {
		if f.Has("heading") {
			return run(p, s.Heading.Text)
		}
		return x.through(s.Keys, s.Blocks)
	}
	if bs := x.blocks[p]; bs != nil {
		return x.through(nil, bs)
	}
	if it := x.items[p]; it != nil {
		for _, v := range mf.Values {
			for _, l := range it.Lines {
				if i := strings.Index(l.Text, v.Raw); i >= 0 {
					return run(diag.Pos{Line: l.Pos.Line, Col: l.Pos.Col + utf8.RuneCountInString(l.Text[:i])}, v.Raw)
				}
			}
		}
		last := it.Lines[len(it.Lines)-1].Pos.Line
		return span{p.Line, p.Col, last, x.d.LineEnd(last).Col}
	}
	return span{}
}

// through returns the span from the first of keys and blocks to the end of the last.
func (x *index) through(keys []*doc.KeyLine, blocks []doc.Block) span {
	var from diag.Pos
	last := 0
	if len(keys) > 0 {
		from, last = keys[0].Pos, keys[len(keys)-1].Pos.Line
	}
	if len(blocks) > 0 {
		if last == 0 {
			from = blocks[0].At()
		}
		last = blocks[len(blocks)-1].End()
	}
	if last == 0 {
		return span{}
	}
	return span{from.Line, from.Col, last, x.d.LineEnd(last).Col}
}

// run returns the span of text written at p, on one line.
func run(p diag.Pos, text string) span {
	return span{p.Line, p.Col, p.Line, p.Col + utf8.RuneCountInString(text)}
}
