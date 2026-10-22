package schema

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"unicode"

	"github.com/func-it/kanon/internal/diag"
	"github.com/func-it/kanon/internal/names"
	"github.com/func-it/kanon/internal/pattern"
	"github.com/func-it/kanon/internal/value"
)

// Load reads, parses and checks the .kanonarch file at path. A file that doesn't parse isn't checked.
func Load(path string) (*File, diag.List) {
	src, err := os.ReadFile(path)
	if err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) {
			err = pe.Err
		}
		var diags diag.List
		diags.Errorf(diag.Pos{File: path}, "cannot read the file: %v", err)
		return nil, diags
	}
	f, diags := Parse(path, string(src))
	if diags.Count(diag.Error) == 0 {
		diags = append(diags, Check(f)...)
	}
	return f, diags
}

// Builtins are the names every expression may use.
var Builtins = []string{"today", "none", "yes", "no", "sum", "round", "min", "max", "count", "abs"}

// Annotations are the annotations a field may carry.
var Annotations = []string{"heading", "body", "sections"}

// Check resolves the types of f and validates its names, annotations, patterns and expressions. It sets
// Type.Native, Kind, Schema and Enum, and Schema.Patterns.
func Check(f *File) diag.List {
	c := &checker{f: f, deps: map[*Field][]*Field{}, calls: map[*Fn][]*Fn{}}
	c.decls()
	for _, u := range f.Units {
		c.unit(u)
	}
	for _, e := range f.Enums {
		c.enum(e)
	}
	c.global = c.globalScope()
	for _, s := range f.Schemas {
		c.schema(s)
	}
	for _, fn := range f.Fns {
		c.fn(fn)
	}
	cycles(c, f.Fns, func(fn *Fn) []*Fn { return c.calls[fn] }, func(fn *Fn) diag.Pos { return fn.Pos },
		func(fn *Fn) string { return fn.Name }, "calls itself", "; a function may not recurse")
	c.diags.Sort()
	return c.diags
}

type checker struct {
	f      *File
	global *scope
	diags  diag.List

	field  *Field              // the computed field being checked
	deps   map[*Field][]*Field // the fields of its schema each computed field reads
	caller *Fn                 // the function being checked
	calls  map[*Fn][]*Fn       // the functions each function calls
}

func (c *checker) errorf(pos diag.Pos, format string, args ...any) {
	c.diags.Errorf(pos, format, args...)
}

// suggest adds "did you mean" to msg when a candidate is near name.
func suggest(msg, name string, candidates []string) string {
	if s := names.Closest(name, candidates); s != "" {
		return msg + "; did you mean `" + s + "`?"
	}
	return msg
}

// decls reports a name declared twice, at the second declaration.
func (c *checker) decls() {
	type decl struct {
		name string
		pos  diag.Pos
	}
	var ds []decl
	for _, u := range c.f.Units {
		ds = append(ds, decl{u.Name, u.Pos})
	}
	for _, e := range c.f.Enums {
		ds = append(ds, decl{e.Name, e.Pos})
	}
	for _, s := range c.f.Schemas {
		ds = append(ds, decl{s.Name, s.Pos})
	}
	for _, fn := range c.f.Fns {
		ds = append(ds, decl{fn.Name, fn.Pos})
	}
	slices.SortStableFunc(ds, func(a, b decl) int {
		return cmp.Or(cmp.Compare(a.pos.Line, b.pos.Line), cmp.Compare(a.pos.Col, b.pos.Col))
	})
	first := map[string]diag.Pos{}
	for _, d := range ds {
		if p, ok := first[d.name]; ok {
			c.errorf(d.pos, "`%s` is already declared on line %d", d.name, p.Line)
			continue
		}
		first[d.name] = d.pos
	}
}

// name checks a name: a lowercase ASCII slug that starts with a letter.
func (c *checker) name(pos diag.Pos, name string) {
	if names.IsName(name) {
		return
	}
	if fix := names.Slugify(name); names.IsName(fix) {
		c.errorf(pos, "write %s, not %q", fix, name)
		return
	}
	c.errorf(pos, "%q is not a name: use lowercase ASCII letters, digits and hyphens, starting with a letter", name)
}

// typeName checks the name of an enum or a schema: ASCII PascalCase, and not a native type.
func (c *checker) typeName(pos diag.Pos, name string) {
	if _, ok := value.KindOf(name); ok {
		c.errorf(pos, "`%s` is a native type; choose another name", name)
		return
	}
	if names.IsTypeName(name) {
		return
	}
	if fix := pascal(name); fix != "" {
		c.errorf(pos, "type names are PascalCase: write %s, not %q", fix, name)
		return
	}
	c.errorf(pos, "%q is not a type name: use ASCII PascalCase, such as EtatDesLieux", name)
}

// pascal proposes a type name for s: etat-des-lieux becomes EtatDesLieux, Pièce becomes Piece; or "".
func pascal(s string) string {
	var b strings.Builder
	up := true
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			up = true
			continue
		}
		w := names.Fold(string(r))
		if up || unicode.IsUpper(r) {
			w = strings.ToUpper(w)
		}
		b.WriteString(w)
		up = false
	}
	if t := b.String(); names.IsTypeName(t) {
		return t
	}
	return ""
}

func (c *checker) unit(u *Unit) {
	switch {
	case !value.UnitName.MatchString(u.Name):
		if fix := strings.ReplaceAll(names.Slugify(u.Name), "-", ""); value.UnitName.MatchString(fix) {
			c.errorf(u.Pos, "write %s, not %q", fix, u.Name)
		} else {
			c.errorf(u.Pos, "%q is not a unit name: use ASCII letters and digits, starting with a letter", u.Name)
		}
	case slices.Contains(value.BuiltinUnits, u.Name):
		c.errorf(u.Pos, "`%s` is a built-in unit", u.Name)
	case value.DurationUnits[u.Name] != "":
		c.errorf(u.Pos, "`%s` is a duration unit", u.Name)
	}
}

func (c *checker) enum(e *Enum) {
	c.typeName(e.Pos, e.Name)
	seen := map[string]bool{}
	for _, v := range e.Values {
		c.name(v.Pos, v.Name)
		if seen[v.Name] {
			c.errorf(v.Pos, "`%s` is already a value of `%s`", v.Name, e.Name)
		}
		seen[v.Name] = true
	}
}

func (c *checker) schema(s *Schema) {
	c.typeName(s.Pos, s.Name)
	byName := map[string]bool{}
	byLabel := map[string]*Field{}
	annotated := map[string]*Field{}
	for _, f := range s.Fields {
		c.name(f.Pos, f.Name)
		switch {
		case f.Name == "id":
			c.errorf(f.Pos, "`id` is reserved: every object has one")
		case byName[f.Name]:
			c.errorf(f.Pos, "`%s` is already a field of `%s`", f.Name, s.Name)
		case byLabel[f.Label] != nil:
			c.errorf(f.Pos, "the label %q is already used by `%s`", f.Label, byLabel[f.Label].Name)
		default:
			byLabel[f.Label] = f
		}
		byName[f.Name] = true
		c.resolve(f.Type)
		if f.Inverse != "" {
			c.inverse(s, f)
		}
		c.annots(f, annotated)
	}
	c.patterns(s)
	vars := []*bind{{name: "id", t: nativeType(value.Text)}}
	for _, f := range s.Fields {
		vars = append(vars, &bind{name: f.Name, t: typeOf(f.Type), field: f})
	}
	sc := c.global.with(vars...)
	for _, f := range s.Fields {
		if f.Expr == nil {
			continue
		}
		c.field = f
		if m := mismatch(typeOf(f.Type), c.expr(f.Expr, sc)); m != "" {
			c.errorf(f.Pos, "`%s` is `%s`, but its expression gives %s", f.Name, f.Type, m)
		}
	}
	c.field = nil
	cycles(c, s.Fields, func(f *Field) []*Field { return c.deps[f] }, func(f *Field) diag.Pos { return f.Pos },
		func(f *Field) string { return f.Name }, "depends on itself", "")
}

// resolve finds what t names, and checks that only a schema is linked to.
func (c *checker) resolve(t *Type) {
	c.find(t)
	if t.Link && t.Resolved() && t.Schema == nil {
		plain := *t
		plain.Link = false
		c.errorf(t.Pos, "only a schema can be linked to: write %s, not %q", &plain, t.String())
	}
}

// find finds what t names: a native type, a schema or an enum.
func (c *checker) find(t *Type) {
	if k, ok := value.KindOf(t.Name); ok {
		t.Native, t.Kind = true, k
		return
	}
	if s := c.f.Schema(t.Name); s != nil {
		t.Schema = s
		return
	}
	for _, e := range c.f.Enums {
		if e.Name == t.Name {
			t.Enum = e
			return
		}
	}
	types := []string{}
	for k := value.Text; k <= value.Code; k++ {
		types = append(types, k.String())
	}
	for _, e := range c.f.Enums {
		types = append(types, e.Name)
	}
	types = append(types, c.f.SchemaNames()...)
	c.errorf(t.Pos, "%s", suggest(fmt.Sprintf("unknown type `%s`", t.Name), t.Name, types))
}

// inverse checks the inverse field f of s: [S] <- f holds the objects S whose field f links to s.
func (c *checker) inverse(s *Schema, f *Field) {
	t := f.Type
	switch {
	case !t.Resolved():
		return
	case t.Schema == nil:
		c.errorf(t.Pos, "an inverse holds the objects that link here, and `%s` is not a schema", t.Name)
		return
	case t.Link:
		plain := *t
		plain.Link = false
		c.errorf(t.Pos, "an inverse holds the objects that link here: write %s <- %s, not %q", &plain, f.Inverse,
			t.String()+" <- "+f.Inverse)
		return
	}
	target := t.Schema.Field(f.Inverse)
	if target == nil {
		msg := suggest(fmt.Sprintf("`%s` has no field `%s`", t.Name, f.Inverse), f.Inverse, t.Schema.FieldNames())
		c.errorf(f.InversePos, "%s", msg)
		return
	}
	if tt := target.Type; !tt.Link || tt.Name != s.Name {
		c.errorf(f.InversePos, "`%s.%s` is `%s`; an inverse needs a link to `%s` there: -> %[4]s, -> %[4]s? or [-> %[4]s]",
			t.Name, f.Inverse, tt, s.Name)
	}
}

// annotNeeds says what each annotation needs, and fits whether a type has it.
var annotNeeds = map[string]struct {
	what string
	fits func(t *Type) bool
}{
	"heading":  {"a `Text` field", func(t *Type) bool { return t.Native && t.Kind == value.Text && !t.Array }},
	"body":     {"a `Prose` field", func(t *Type) bool { return t.Native && t.Kind == value.Prose && !t.Array }},
	"sections": {"an array of a schema", func(t *Type) bool { return t.Array && t.Schema != nil && !t.Link }},
}

// annots checks the annotations of f; annotated maps each annotation to the field of the schema that has it.
func (c *checker) annots(f *Field, annotated map[string]*Field) {
	var seen []string
	for _, a := range f.Annots {
		need, ok := annotNeeds[a.Name]
		switch {
		case !ok:
			msg := fmt.Sprintf("unknown annotation `@%s`", a.Name)
			if s := names.Closest(a.Name, Annotations); s != "" {
				msg += "; did you mean `@" + s + "`?"
			}
			c.errorf(a.Pos, "%s", msg)
		case slices.Contains(seen, a.Name):
			c.errorf(a.Pos, "`%s` is already marked `@%s`", f.Name, a.Name)
		case annotated[a.Name] != nil:
			c.errorf(a.Pos, "`@%s` is already on `%s`", a.Name, annotated[a.Name].Name)
		case f.Type.Resolved() && !need.fits(f.Type):
			c.errorf(a.Pos, "`@%s` needs %s, not `%s`", a.Name, need.what, f.Type)
		}
		if ok && annotated[a.Name] == nil {
			annotated[a.Name] = f
		}
		seen = append(seen, a.Name)
	}
}

// patterns parses the reads patterns of s and checks their placeholders against its fields.
func (c *checker) patterns(s *Schema) {
	for _, r := range s.Reads {
		pt, err := pattern.Parse(r.Text)
		if err != nil {
			pos := r.Pos
			if pe := (*pattern.Error)(nil); errors.As(err, &pe) {
				pos = readsPos(r, pe.Col-1)
			}
			c.errorf(pos, "%s", err.Error())
			continue
		}
		s.Patterns = append(s.Patterns, pt)
		placeholders := pt.Fields()
		for _, name := range placeholders {
			pos := readsPos(r, placeholderIndex(r.Text, name))
			switch f := s.Field(name); {
			case f == nil:
				c.errorf(pos, "%s", suggest(fmt.Sprintf("unknown field `%s`", name), name, s.FieldNames()))
			case f.Computed():
				c.errorf(pos, "`%s` is computed: a pattern reads written fields", name)
			case f.Type.Array:
				c.errorf(pos, "`%s` is an array: a pattern reads single values", name)
			case f.Type.Link:
				c.errorf(pos, "`%s` links to a `%s`: a pattern reads values, not links", name, f.Type.Name)
			case f.Type.Schema != nil:
				c.errorf(pos, "`%s` is a `%s`: a pattern reads values, not objects", name, f.Type.Name)
			case f.Type.Native && f.Type.Kind >= value.File:
				c.errorf(pos, "`%s` is %s: a pattern reads values, not files", name, names.Article(f.Type.Name))
			}
		}
		for _, f := range s.Fields {
			if f.Required() && !slices.Contains(placeholders, f.Name) {
				c.errorf(r.Pos, "the pattern doesn't read `%s`, which is required", f.Name)
			}
		}
	}
}

// placeholderIndex returns the rune index of the placeholder {name} in text, or 0.
func placeholderIndex(text, name string) int {
	i := strings.Index(text, "{"+name)
	if i < 0 {
		return 0
	}
	return len([]rune(text[:i]))
}

// readsPos returns the position of the rune at index i of r.Text, in the file. The string starts with a quote,
// and each " or \ of its text was written with an escape, as two runes.
func readsPos(r *Reads, i int) diag.Pos {
	pos := r.Pos
	pos.Col++
	for k, ch := range []rune(r.Text) {
		if k == i {
			break
		}
		pos.Col++
		if ch == '"' || ch == '\\' {
			pos.Col++
		}
	}
	return pos
}

func (c *checker) fn(fn *Fn) {
	c.name(fn.Pos, fn.Name)
	c.caller = fn
	defer func() { c.caller = nil }()
	var params []string
	var vars []*bind
	defaulted := false
	for _, p := range fn.Params {
		c.name(p.Pos, p.Name)
		if slices.Contains(params, p.Name) {
			c.errorf(p.Pos, "`%s` is already a parameter of `%s`", p.Name, fn.Name)
		}
		c.resolve(p.Type)
		if p.Default != nil {
			if m := mismatch(typeOf(p.Type), c.expr(p.Default, c.global.with(vars...))); m != "" {
				c.errorf(p.Pos, "`%s` is `%s`, but its default gives %s", p.Name, p.Type, m)
			}
			defaulted = true
		} else if defaulted {
			c.errorf(p.Pos, "`%s` needs a default value, as it follows a parameter that has one", p.Name)
		}
		params = append(params, p.Name)
		vars = append(vars, &bind{name: p.Name, t: typeOf(p.Type)})
	}
	// A rule gives Bool, or none when what it reads is missing: then it holds.
	want, written := opt(nativeType(value.Bool), true), "Bool"
	if t := fn.Result; t != nil {
		c.resolve(t)
		switch {
		case !fn.Rule:
			want, written = typeOf(t), t.String()
		case !t.Resolved():
			want = nil
		case !t.is(value.Bool) || t.Optional:
			c.errorf(t.Pos, "a rule returns `Bool`, not `%s`", t)
			want = nil
		}
	}
	for _, r := range fn.Reads {
		c.fnReads(r, params)
	}
	if m := mismatch(want, c.expr(fn.Body, c.global.with(vars...))); m != "" {
		c.errorf(fn.Pos, "`%s` returns `%s`, but its expression gives %s", fn.Name, written, m)
	}
}

// fnReads checks that each {…} placeholder of a function's reads text starts with a parameter: {v.nom}.
func (c *checker) fnReads(r *Reads, params []string) {
	text := []rune(r.Text)
	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			continue
		}
		j := slices.Index(text[i:], '}')
		if j < 0 {
			c.errorf(readsPos(r, i), "missing `}` after `{`")
			return
		}
		inner := strings.TrimSpace(string(text[i+1 : i+j]))
		head, _, _ := strings.Cut(inner, ".")
		switch {
		case inner == "":
			c.errorf(readsPos(r, i), "empty placeholder `{}`")
		case !slices.Contains(params, head):
			c.errorf(readsPos(r, i), "%s", suggest(fmt.Sprintf("unknown parameter `%s`", head), head, params))
		}
		i += j
	}
}
