// Package eval evaluates the computed fields and the rules of a schema over the model of a document. A computed
// value goes into the model as a field of its own, which kan sql leaves out; a rule that fails is an error at the
// object, in the words of its reads sentence.
package eval

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/model"
	"github.com/bn-k/kanon/internal/schema"
	"github.com/bn-k/kanon/internal/value"
)

// val is a value in an expression: nil for none, a value.Value, an enumVal, an *obj, or a []val for an array.
type val = any

// enumVal is a value of the enum e, by name.
type enumVal struct {
	e    *schema.Enum
	name string
}

// obj is an object in an expression: an object of the model, or a copy of one made by with, whose fields set
// overrides.
type obj struct {
	m     *model.Object
	made  bool // a copy made by with
	set   map[string]val
	slots map[*schema.Field]*slot
	mat   *model.Object // a copy, once it is put in the model
}

// slot holds the value of a computed field of an object, once it is evaluated.
type slot struct {
	state int
	v     val
}

const (
	busy = iota + 1
	done
	failed
)

// evalErr stops an evaluation that went wrong: msg says why, unless it is silent, already reported. cycle is a
// field that depends on itself.
type evalErr struct {
	msg    string
	silent bool
	cycle  bool
}

func fail(format string, args ...any) { panic(evalErr{msg: fmt.Sprintf(format, args...)}) }

// env holds the names an expression uses: the parameters of a function and of its lambdas, then the fields of
// self, the object a computed field belongs to.
type env struct {
	up   *env
	name string
	v    val
	self *obj
}

func (en *env) bind(name string, v val) *env { return &env{up: en, name: name, v: v, self: en.self} }

// lookup returns the parameter called name.
func (en *env) lookup(name string) (val, bool) {
	for ; en != nil; en = en.up {
		if en.name == name {
			return en.v, true
		}
	}
	return nil, false
}

type evaluator struct {
	file  *schema.File
	today value.Value
	objs  map[*model.Object]*obj
	diags diag.List
}

// Eval evaluates the computed fields of every object of d, and puts their values in its model; then it checks the
// rules that apply to each object. today is the value of today.
func Eval(d *model.Doc, today time.Time) diag.List {
	y, m, day := today.Date()
	e := &evaluator{
		file:  d.Schema,
		today: value.Value{Kind: value.Date, Date: time.Date(y, m, day, 0, 0, 0, 0, time.UTC)},
		objs:  map[*model.Object]*obj{},
	}
	for _, m := range slices.Collect(d.Objects()) {
		o := e.wrap(m)
		for _, f := range m.Schema.Fields {
			if f.Expr != nil {
				e.try(func() { e.field(o, f) })
			}
		}
		for _, fn := range e.file.Fns {
			if appliesTo(fn, m.Schema) {
				e.rule(fn, o)
			}
		}
		for _, f := range m.Schema.Fields {
			if s := o.slots[f]; s != nil && s.state == done {
				e.check(o, f, s.v)
				e.put(m, f, s.v)
			}
		}
	}
	return e.diags
}

// Computed returns the computed fields of the objects of d, as kan eval prints them: f-2026-031.kan: tva = 1 584.00 EUR.
func Computed(d *model.Doc) []string {
	var lines []string
	for o := range d.Objects() {
		for _, mf := range o.Fields {
			if mf.Field.Expr != nil {
				lines = append(lines, fmt.Sprintf("%s: %s = %s", o.Pos, mf.Field.Name, text(mf)))
			}
		}
	}
	return lines
}

// text returns what the field mf of the model holds, as written.
func text(mf *model.Field) string {
	var ws []string
	for _, v := range mf.Values {
		ws = append(ws, v.Raw)
	}
	for _, o := range slices.Concat(mf.Objects, mf.Links) {
		ws = append(ws, o.ID)
	}
	ws = append(ws, mf.Files...)
	if len(ws) == 0 {
		return mf.Text
	}
	return strings.Join(ws, ", ")
}

// wrap returns the object of an expression that stands for m, the same one each time.
func (e *evaluator) wrap(m *model.Object) *obj {
	o := e.objs[m]
	if o == nil {
		o = &obj{m: m}
		e.objs[m] = o
	}
	return o
}

// try runs f, which stops when an evaluation goes wrong, once reported.
func (e *evaluator) try(f func()) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(evalErr); !ok {
				panic(r)
			}
		}
	}()
	f()
}

// field returns the value of the computed field f of o, evaluated once. What goes wrong is reported at o, once.
func (e *evaluator) field(o *obj, f *schema.Field) val {
	if o.slots == nil {
		o.slots = map[*schema.Field]*slot{}
	}
	s := o.slots[f]
	switch {
	case s == nil:
		s = &slot{}
		o.slots[f] = s
	case s.state == done:
		return s.v
	case s.state == failed:
		panic(evalErr{silent: true})
	default:
		panic(evalErr{cycle: true})
	}
	s.state = busy
	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(evalErr)
			if !ok {
				panic(r)
			}
			s.state = failed
			switch {
			case err.cycle:
				e.diags.Errorf(o.m.Pos, "`%s` depends on itself, through the objects it links to", f.Name)
			case !err.silent:
				e.diags.Errorf(o.m.Pos, "%s", "`"+f.Name+"`: "+err.msg)
			}
			panic(evalErr{silent: true})
		}
	}()
	s.v = e.expr(f.Expr, &env{self: o})
	s.state = done
	return s.v
}

// get returns the field called name of o, or its id.
func (e *evaluator) get(o *obj, name string) val {
	if v, ok := o.set[name]; ok {
		return v
	}
	f := o.m.Schema.Field(name)
	switch {
	case f == nil:
		return value.Value{Kind: value.Text, Raw: o.m.ID}
	case f.Expr != nil:
		return e.field(o, f)
	}
	return e.written(o.m, f)
}

// written returns what the field f of m holds as written: none when it is absent, or an empty array.
func (e *evaluator) written(m *model.Object, f *schema.Field) val {
	var items []val
	if mf := m.Get(f.Name); mf != nil {
		items = e.items(mf)
	}
	switch {
	case f.Type.Array:
		if items == nil {
			items = []val{}
		}
		return items
	case len(items) == 0:
		return nil
	}
	return items[0]
}

// items returns the values the field mf of the model holds, by its type.
func (e *evaluator) items(mf *model.Field) []val {
	t := mf.Field.Type
	var vs []val
	switch {
	case t.Enum != nil:
		for _, v := range mf.Values {
			vs = append(vs, enumVal{t.Enum, v.Raw})
		}
	case t.Schema != nil:
		for _, o := range slices.Concat(mf.Objects, mf.Links) {
			vs = append(vs, e.wrap(o))
		}
	case t.Kind == value.File || t.Kind == value.Image:
		for _, p := range mf.Files {
			vs = append(vs, value.Value{Kind: t.Kind, Raw: p})
		}
	case t.Kind == value.Code || t.Kind == value.Prose && !t.Array:
		vs = append(vs, value.Value{Kind: t.Kind, Raw: mf.Text})
	default:
		for _, v := range mf.Values {
			vs = append(vs, v)
		}
	}
	return vs
}

// check reports a computed field written in the document with another value than the one computed.
func (e *evaluator) check(o *obj, f *schema.Field, v val) {
	mf := o.m.Get(f.Name)
	if mf != nil && !safeEqual(e.written(o.m, f), v) {
		e.diags.Errorf(mf.Pos, "%s", fmt.Sprintf("`%s` is %s, not \"%s\"", f.Name, show(v), text(mf)))
	}
}

// put sets the field f of m to v, in schema order; none, or an empty array, leaves it out.
func (e *evaluator) put(m *model.Object, f *schema.Field, v val) {
	mf := &model.Field{Field: f}
	if i := slices.IndexFunc(m.Fields, func(x *model.Field) bool { return x.Field == f }); i >= 0 {
		mf.Pos = m.Fields[i].Pos
		m.Fields = slices.Delete(m.Fields, i, i+1)
	}
	items, _ := v.([]val)
	if !f.Type.Array && v != nil {
		items = []val{v}
	}
	if len(items) == 0 {
		return
	}
	for _, it := range items {
		switch it := it.(type) {
		case *obj:
			if f.Computed() || f.Type.Link {
				mf.Links = append(mf.Links, e.materialize(it))
			} else {
				mf.Objects = append(mf.Objects, e.materialize(it))
			}
		case enumVal:
			mf.Values = append(mf.Values, value.Value{Kind: value.Text, Raw: it.name})
		case value.Value:
			it.Raw = Format(it)
			switch {
			case it.Kind == value.File || it.Kind == value.Image:
				mf.Files = append(mf.Files, it.Raw)
			case it.Kind == value.Code || it.Kind == value.Prose && !f.Type.Array:
				mf.Text = it.Raw
			default:
				mf.Values = append(mf.Values, it)
			}
		}
	}
	at := slices.Index(m.Schema.Fields, f)
	i := slices.IndexFunc(m.Fields, func(x *model.Field) bool { return slices.Index(m.Schema.Fields, x.Field) > at })
	if i < 0 {
		i = len(m.Fields)
	}
	m.Fields = slices.Insert(m.Fields, i, mf)
}

// materialize returns the object of the model o stands for: a copy made by with becomes an object of its own,
// its computed fields evaluated again.
func (e *evaluator) materialize(o *obj) *model.Object {
	if !o.made {
		return o.m
	}
	if o.mat != nil {
		return o.mat
	}
	m := &model.Object{Schema: o.m.Schema, ID: o.m.ID, Pos: o.m.Pos, File: o.m.File, Parent: o.m.Parent, Holder: o.m.Holder}
	o.mat = m
	for _, f := range m.Schema.Fields {
		v, ok := o.set[f.Name]
		switch {
		case ok:
		case f.Expr != nil:
			e.try(func() { v = e.field(o, f) })
		default:
			if mf := o.m.Get(f.Name); mf != nil {
				c := *mf
				m.Fields = append(m.Fields, &c)
			}
			continue
		}
		e.put(m, f, v)
	}
	return m
}

// appliesTo reports whether the rule fn applies to every object of s: its first parameter is one, and the others
// have a default.
func appliesTo(fn *schema.Fn, s *schema.Schema) bool {
	if !fn.Rule || len(fn.Params) == 0 {
		return false
	}
	if t := fn.Params[0].Type; t.Schema != s || t.Array || t.Optional {
		return false
	}
	for _, p := range fn.Params[1:] {
		if p.Default == nil {
			return false
		}
	}
	return true
}

// rule checks the rule fn on o: it fails when it gives no, and says so in the words of its reads sentence.
func (e *evaluator) rule(fn *schema.Fn, o *obj) {
	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(evalErr)
			if !ok {
				panic(r)
			}
			if !err.silent {
				e.diags.Errorf(o.m.Pos, "%s", "`"+fn.Name+"`: "+err.msg)
			}
		}
	}()
	en := (&env{}).bind(fn.Params[0].Name, o)
	for _, p := range fn.Params[1:] {
		en = en.bind(p.Name, e.expr(p.Default, en))
	}
	if !isBool(e.expr(fn.Body, en), false) {
		return
	}
	msg := "`" + fn.Name + "` fails"
	if len(fn.Reads) > 0 {
		msg += ": \"" + e.interpolate(fn.Reads[0].Text, en) + "\""
	}
	if why := e.explain(fn.Body, en); why != "" {
		msg += "; " + why
	}
	e.diags.Errorf(o.m.Pos, "%s", msg)
}

var placeholder = regexp.MustCompile(`\{[^{}]*\}`)

// interpolate fills the placeholders of a reads sentence, {v.nom}, with the values of en; one that names no field
// is left as written. The schema check has made sure that each starts with a parameter.
func (e *evaluator) interpolate(text string, en *env) string {
	return placeholder.ReplaceAllStringFunc(text, func(p string) string {
		path := strings.Split(strings.TrimSpace(p[1:len(p)-1]), ".")
		v, _ := en.lookup(path[0])
		for _, name := range path[1:] {
			if !hasField(v, name) {
				return p
			}
			v = e.member(v, name)
		}
		return show(v)
	})
}

// hasField reports whether v, or each item of v, has a field called name; none has every field.
func hasField(v val, name string) bool {
	switch v := v.(type) {
	case nil:
		return true
	case *obj:
		return name == "id" || v.m.Schema.Field(name) != nil
	case []val:
		return len(v) == 0 || hasField(v[0], name)
	}
	return false
}

// comparisonWords say why a comparison fails, for values and for dates.
var comparisonWords = map[string][2]string{
	"<":  {"is not less than", "is not before"},
	"<=": {"is more than", "is after"},
	">":  {"is not more than", "is not after"},
	">=": {"is less than", "is before"},
	"==": {"is not equal to", "is not equal to"},
	"!=": {"is equal to", "is equal to"},
}

// explain says why x, which gives no, does: 15 250 km is not less than 15 000 km. It returns "" when it cannot.
func (e *evaluator) explain(x schema.Expr, en *env) string {
	b, ok := x.(*schema.Binary)
	if !ok {
		return ""
	}
	switch b.Op {
	case "and":
		if isBool(e.expr(b.X, en), false) {
			return e.explain(b.X, en)
		}
		return e.explain(b.Y, en)
	case "or":
		var why []string
		for _, side := range []schema.Expr{b.X, b.Y} {
			if s := e.explain(side, en); s != "" {
				why = append(why, s)
			}
		}
		return strings.Join(why, " and ")
	}
	words, ok := comparisonWords[b.Op]
	if !ok {
		return ""
	}
	l, r := e.expr(b.X, en), e.expr(b.Y, en)
	if l == nil || r == nil {
		return ""
	}
	lv, _ := l.(value.Value)
	rv, _ := r.(value.Value)
	i := 0
	if isTime(lv.Kind) {
		i = 1
	}
	// A number compared with a version is shown as one: 2003, not 2 003.
	switch {
	case lv.Kind == value.Version && rv.Num != nil:
		r = asVersion(rv)
	case rv.Kind == value.Version && lv.Num != nil:
		l = asVersion(lv)
	}
	return show(l) + " " + words[i] + " " + show(r)
}

// with returns a copy of the object x, with some fields changed.
func (e *evaluator) with(w *schema.With, en *env) val {
	x := e.expr(w.X, en)
	if x == nil {
		return nil
	}
	o := x.(*obj)
	c := &obj{m: o.m, made: true, set: maps.Clone(o.set)}
	if c.set == nil {
		c.set = map[string]val{}
	}
	for _, f := range w.Fields {
		c.set[f.Name] = e.expr(f.Value, en)
	}
	return c
}
