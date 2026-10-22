// Package lint checks Kanon documents against their schema: every field, value, id and link.
package lint

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/doc"
	"github.com/bn-k/kanon/internal/eval"
	"github.com/bn-k/kanon/internal/model"
	"github.com/bn-k/kanon/internal/names"
	"github.com/bn-k/kanon/internal/schema"
	"github.com/bn-k/kanon/internal/value"
)

// Result is what a lint run found.
type Result struct {
	Files []string // every file read, the schema first
	Diags diag.List
}

// Run checks the document at docPath, and every document it links to or holds in folders, against the schema at
// schemaPath. Its rules take the local date as today.
func Run(schemaPath, docPath string) Result {
	_, r := Load(schemaPath, docPath)
	return r
}

// Load is Run that keeps what it read: the model of the document, with its computed fields, or nil when it found an
// error before evaluating them. A document whose rules fail still has its model.
func Load(schemaPath, docPath string) (*model.Doc, Result) {
	return LoadOn(schemaPath, docPath, time.Now())
}

// LoadOn is Load on the date of today.
func LoadOn(schemaPath, docPath string, today time.Time) (*model.Doc, Result) {
	l := &linter{
		schemaPath: filepath.Clean(schemaPath),
		docs:       map[string]*file{},
		schemas:    map[string]*schema.File{},
		sniffed:    map[string]sniffed{},
		placed:     map[slot]link{},
	}
	l.files = append(l.files, l.schemaPath)
	s, ds := schema.Load(l.schemaPath)
	l.diags = append(l.diags, ds...)
	var d *model.Doc
	if s != nil && ds.Count(diag.Error) == 0 {
		l.schema = s
		l.schemas[l.schemaPath] = s
		l.units = value.NewUnits(s.UnitNames())
		f := l.load(docPath, nil, nil)
		l.resolveLinks()
		if f.root != nil {
			d = l.model(f)
			l.inverses(d)
			if l.diags.Count(diag.Error) > 0 {
				d = nil
			} else {
				l.diags = append(l.diags, eval.Eval(d, today)...)
			}
		}
	}
	l.diags.Sort()
	return d, Result{l.files, l.diags}
}

// model is the model of the document f: its root object, and the documents its links lead to.
func (l *linter) model(f *file) *model.Doc {
	d := &model.Doc{Schema: l.schema, Root: f.root.m}
	for _, path := range l.files {
		if g := l.docs[path]; g != nil && g != f && g.root != nil && g.root.m.Parent == nil {
			d.Linked = append(d.Linked, g.root.m)
		}
	}
	return d
}

// inverses fills the inverse fields of the objects of d: [S] <- f and S? <- f hold the objects whose field f links
// here, once each, in document order. An S? that more than one object links to is reported.
func (l *linter) inverses(d *model.Doc) {
	first := map[*model.Field]link{} // the link that fills an S?
	for _, src := range slices.Collect(d.Objects()) {
		for _, mf := range slices.Clone(src.Fields) { // an inverse may go into src's own fields
			for i, o := range mf.Links {
				if o != nil && mf.Field.Inverse == "" { // nil: not resolved, already reported
					l.back(src, mf, o, l.placed[slot{mf, i}], first)
				}
			}
		}
	}
}

// back adds src, whose field mf links to o by k, to the inverses of o that hold the objects linking by mf.
func (l *linter) back(src *model.Object, mf *model.Field, o *model.Object, k link, first map[*model.Field]link) {
	for _, inv := range o.Schema.Fields {
		if inv.Inverse == "" || inv.Type.Schema.Field(inv.Inverse) != mf.Field {
			continue
		}
		back := modelField(o, inv, diag.Pos{})
		switch n := len(back.Links); {
		case n > 0 && back.Links[n-1] == src:
		case n > 0 && !inv.Type.Array:
			l.diags.Errorf(k.pos, "[[%s]] is already the `%s` of `%s`%s; %s has one `%s`", k.target, mf.Field.Name,
				back.Links[0].ID, where(first[back].pos, k.pos), names.Article(o.Schema.Name), inv.Name)
		default:
			back.Links = append(back.Links, src)
			first[back] = k
		}
	}
}

type linter struct {
	schema     *schema.File
	schemaPath string
	units      value.Units
	diags      diag.List
	files      []string
	docs       map[string]*file        // by path
	schemas    map[string]*schema.File // the schema files links point to, by path
	sniffed    map[string]sniffed      // the other files linked to or held in folders, by path
	links      []link                  // waiting to be resolved, once every object is bound
	placed     map[slot]link           // the links resolved into the model, by the place of their target
}

// slot is the place of a link's target in the model: Links[at] of a field.
type slot struct {
	f  *model.Field
	at int
}

// file is a .kan file: one object and the objects written inside it, whose ids are unique within it. An entry of
// one of its folders is a file of its own, linked to by its path.
type file struct {
	path string
	doc  *doc.Doc
	root *object
	ids  map[string]*object
}

// object is an object bound to its schema.
type object struct {
	schema *schema.Schema
	file   *file
	id     string
	pos    diag.Pos            // where it starts: its heading, row or item; Line 0 for a whole file
	level  int                 // the level of its heading, 1 for a whole file; its fields go one level below
	parent *object             // the object that holds it; {field} looks there when its own schema has no such field
	filled map[string]diag.Pos // the fields written so far, and where
	m      *model.Object       // what is read of it
}

// link is a [[path#id]] waiting to be resolved; want is the schema its field expects, or nil in prose. The target
// it resolves to goes into into.Links[at], when into is not nil.
type link struct {
	target string
	pos    diag.Pos
	from   *file
	want   *schema.Schema
	into   *model.Field
	at     int
}

// load reads and binds the document at path, once. in is the array field whose folder holds it, and parent the
// object of that field, or nil for both.
func (l *linter) load(path string, in *schema.Field, parent *object) *file {
	path = filepath.Clean(path)
	if f, ok := l.docs[path]; ok {
		return f
	}
	f := &file{path: path, ids: map[string]*object{}}
	l.docs[path] = f
	l.files = append(l.files, path)
	src, err := os.ReadFile(path)
	if err != nil {
		l.diags.Errorf(diag.Pos{File: path}, "cannot read the file: %v", unwrap(err))
		return f
	}
	d, ds := doc.Parse(path, string(src))
	l.diags = append(l.diags, ds...)
	f.doc = d
	if s := l.docType(f, in); s != nil {
		l.root(f, s, in, parent)
	}
	return f
}

// docType returns the schema of a file: the one its front matter names, or the one its position in the folder of
// the field in imposes.
func (l *linter) docType(f *file, in *schema.Field) *schema.Schema {
	var want *schema.Schema
	if in != nil {
		want = in.Type.Schema
	}
	var s *schema.Schema
	named := false
	for _, k := range f.doc.FrontMatter {
		if k.Key != "type" {
			l.diags.Errorf(k.Pos, "unknown front matter key `%s`; the front matter holds type: only", k.Key)
			continue
		}
		named = true
		if s = l.schema.Schema(k.Value); s == nil {
			l.diags.Errorf(k.ValuePos, "%s", unknown("schema", k.Value, "", l.schema.SchemaNames()))
		} else if want != nil && s != want {
			l.diags.Errorf(k.ValuePos, "this folder holds `%s` entries, not `%s`", want.Name, s.Name)
			s = want // check it as what its folder holds
		}
	}
	switch {
	case named:
		return s
	case want != nil:
		return want
	}
	l.diags.Errorf(diag.Pos{File: f.path, Line: 1, Col: 1},
		"this document names no schema; begin it with a front matter: ---, then type: %s, then ---", l.example())
	return nil
}

// example is a schema name to show in messages.
func (l *linter) example() string {
	if len(l.schema.Schemas) == 0 {
		return "Name"
	}
	return l.schema.Schemas[len(l.schema.Schemas)-1].Name
}

// queue keeps a link of prose for resolveLinks.
func (l *linter) queue(target string, pos diag.Pos, from *file) {
	l.links = append(l.links, link{target: target, pos: pos, from: from})
}

// queueField keeps a link the field f of o holds for resolveLinks, with a place for its target in o's model.
func (l *linter) queueField(target string, pos diag.Pos, f *schema.Field, o *object) {
	mf := o.put(f, pos)
	mf.Links = append(mf.Links, nil)
	l.links = append(l.links, link{target, pos, o.file, f.Type.Schema, mf, len(mf.Links) - 1})
}

// resolveLinks checks every link; resolving one may load documents that hold more.
func (l *linter) resolveLinks() {
	for len(l.links) > 0 {
		k := l.links[0]
		l.links = l.links[1:]
		l.resolve(k)
	}
}

func (l *linter) resolve(k link) {
	path, id, hasID := strings.Cut(strings.TrimSpace(k.target), "#")
	if fileLink(path) {
		if k.want != nil {
			l.diags.Errorf(k.pos, "this field holds %s; [[%s]] is a file", names.Article(k.want.Name), k.target)
			return
		}
		l.file(path, filepath.Join(filepath.Dir(k.from.path), filepath.FromSlash(path)), k.pos, value.File)
		return
	}
	if hasID && !names.IsSlug(id) {
		l.diags.Errorf(k.pos, "an id is a slug: write #%s, not \"#%s\"", names.Slugify(id), id)
		return
	}
	f := k.from
	if path != "" {
		if !linkPath(path) {
			l.diags.Errorf(k.pos, "a link path is made of ids: write [[%s]], not \"[[%s]]\"", slugPath(k.target), k.target)
			return
		}
		base := filepath.Join(filepath.Dir(k.from.path), filepath.FromSlash(path))
		switch {
		case exists(base + ".kan"):
			f = l.load(base+".kan", nil, nil)
		case exists(base + ".kanonarch"):
			l.resolveDecl(base+".kanonarch", id, k)
			return
		default:
			l.diags.Errorf(k.pos, "%s", noDocument(path, base))
			return
		}
	}
	if f.root == nil {
		return // its schema is unknown: already reported
	}
	o := f.root
	if hasID {
		if o = f.ids[id]; o == nil {
			l.diags.Errorf(k.pos, "%s", unknown("id", id, f.path, slices.Sorted(maps.Keys(f.ids))))
			return
		}
	}
	if k.want != nil && o.schema != k.want {
		l.diags.Errorf(k.pos, "this field holds %s; [[%s]] is %s", names.Article(k.want.Name), k.target, names.Article(o.schema.Name))
		return
	}
	if k.into != nil {
		k.into.Links[k.at] = o.m
		l.placed[slot{k.into, k.at}] = k
	}
}

// resolveDecl checks a link to a declaration of a schema file: [[schema#pilote-compatible-pascal]].
func (l *linter) resolveDecl(path, id string, k link) {
	path = filepath.Clean(path)
	if k.want != nil {
		l.diags.Errorf(k.pos, "this field holds %s; [[%s]] is a schema file", names.Article(k.want.Name), k.target)
		return
	}
	s, ok := l.schemas[path]
	if !ok {
		var ds diag.List
		s, ds = schema.Load(path)
		l.schemas[path] = s
		l.files = append(l.files, path)
		l.diags = append(l.diags, ds...)
	}
	if s != nil && id != "" && !s.Declares(id) {
		l.diags.Errorf(k.pos, "%s", unknown("declaration", id, path, s.DeclNames()))
	}
}

// linkPath reports whether every segment of a link path is an id or "..".
func linkPath(path string) bool {
	for _, seg := range strings.Split(path, "/") {
		if seg != ".." && !names.IsSlug(seg) {
			return false
		}
	}
	return true
}

// slugPath proposes a link made of ids for target.
func slugPath(target string) string {
	path, id, hasID := strings.Cut(target, "#")
	segs := strings.Split(path, "/")
	for i, seg := range segs {
		if seg != ".." {
			segs[i] = names.Slugify(seg)
		}
	}
	s := strings.Join(segs, "/")
	if hasID {
		s += "#" + names.Slugify(id)
	}
	return s
}

// noDocument is the message for a link to a document that doesn't exist, with the nearest one in its folder.
func noDocument(path, base string) string {
	msg := fmt.Sprintf("no document `%s`", path)
	entries, _ := os.ReadDir(filepath.Dir(base))
	var docs []string
	for _, e := range entries {
		if n, ok := strings.CutSuffix(e.Name(), ".kan"); ok && !e.IsDir() {
			docs = append(docs, n)
		}
	}
	if c := names.Closest(filepath.Base(base), docs); c != "" {
		dir := filepath.ToSlash(filepath.Dir(path))
		if dir != "." {
			c = dir + "/" + c
		}
		msg += fmt.Sprintf("; did you mean `%s`?", c)
	}
	return msg
}

// unknown is the message for a name that doesn't exist in a file, or anywhere when in is empty, with the nearest
// candidate when there is one.
func unknown(what, name, in string, candidates []string) string {
	msg := fmt.Sprintf("unknown %s `%s`", what, name)
	if in != "" {
		msg += " in " + in
	}
	if c := names.Closest(name, candidates); c != "" {
		msg += fmt.Sprintf("; did you mean `%s`?", c)
	}
	return msg
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// unwrap drops the path from a file error, which the diagnostic already shows.
func unwrap(err error) error {
	return err.(*os.PathError).Err
}
