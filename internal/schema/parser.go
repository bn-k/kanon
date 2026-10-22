package schema

import (
	"github.com/func-it/kanon/internal/diag"
	"github.com/func-it/kanon/internal/names"
	"github.com/func-it/kanon/internal/value"
)

// Parse reads the declarations of a .kanonarch file. After a syntax error it skips to the next declaration, so
// that every error is reported; the declarations it skipped are left out of the File.
func Parse(path, src string) (*File, diag.List) {
	toks, diags := lex(path, src)
	p := &parser{toks: toks, diags: diags}
	f := &File{Path: path}
	for p.tok().kind != tEOF {
		p.decl(f)
	}
	p.diags.Sort()
	return f, p.diags
}

type parser struct {
	toks  []token
	i     int
	diags diag.List
}

// bail is what a syntax error panics with, to unwind to the declaration being parsed.
type bail struct{}

func (p *parser) tok() token { return p.peek(0) }

func (p *parser) peek(k int) token {
	if p.i+k < len(p.toks) {
		return p.toks[p.i+k]
	}
	return p.toks[len(p.toks)-1]
}

func (p *parser) advance() token {
	t := p.tok()
	if p.i < len(p.toks)-1 {
		p.i++
	}
	return t
}

func (p *parser) errorf(pos diag.Pos, format string, args ...any) {
	p.diags.Errorf(pos, format, args...)
}

// fail reports a syntax error; its caller panics with the result.
func (p *parser) fail(pos diag.Pos, format string, args ...any) bail {
	p.errorf(pos, format, args...)
	return bail{}
}

func (p *parser) accept(punct string) bool {
	if p.tok().isPunct(punct) {
		p.advance()
		return true
	}
	return false
}

func (p *parser) expect(punct string) token {
	t := p.tok()
	if !t.isPunct(punct) {
		panic(p.fail(t.pos, "expected `%s`, found %s", punct, desc(t)))
	}
	return p.advance()
}

func (p *parser) expectKeyword(kw string) {
	if t := p.tok(); !t.isKeyword(kw) {
		panic(p.fail(t.pos, "expected `%s`, found %s", kw, desc(t)))
	}
	p.advance()
}

func (p *parser) name() token {
	t := p.tok()
	if !t.isName() {
		panic(p.fail(t.pos, "expected a name, found %s", desc(t)))
	}
	return p.advance()
}

// atDecl reports whether the current token starts a declaration: a keyword at column 1.
func (p *parser) atDecl() bool {
	t := p.tok()
	switch t.text {
	case "unit", "enum", "schema", "fn", "rule":
		return t.kind == tName && t.pos.Col == 1
	}
	return false
}

func (p *parser) decl(f *File) {
	start := p.i
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(bail); !ok {
				panic(r)
			}
			if p.i == start {
				p.advance()
			}
			for p.tok().kind != tEOF && !p.atDecl() {
				p.advance()
			}
		}
	}()
	switch t := p.tok(); {
	case t.isKeyword("unit"):
		f.Units = append(f.Units, p.unit())
	case t.isKeyword("enum"):
		f.Enums = append(f.Enums, p.enum())
	case t.isKeyword("schema"):
		f.Schemas = append(f.Schemas, p.schema())
	case t.isKeyword("fn"), t.isKeyword("rule"):
		f.Fns = append(f.Fns, p.fn())
	default:
		panic(p.fail(t.pos, "expected a declaration (unit, enum, schema, fn or rule), found %s", desc(t)))
	}
}

func (p *parser) unit() *Unit {
	p.advance()
	n := p.name()
	u := &Unit{Pos: n.pos, Name: n.text}
	if p.tok().kind == tString {
		u.Label = p.advance().text
	}
	return u
}

func (p *parser) enum() *Enum {
	p.advance()
	n := p.name()
	e := &Enum{Pos: n.pos, Name: n.text}
	p.expect("{")
	for !p.accept("}") {
		t := p.tok()
		if !t.isName() {
			panic(p.fail(t.pos, "expected a value or `}`, found %s", desc(t)))
		}
		p.advance()
		v := &EnumValue{Pos: t.pos, Name: t.text}
		if p.tok().kind == tString {
			v.Label = p.advance().text
		}
		p.accept(",")
		e.Values = append(e.Values, v)
	}
	return e
}

func (p *parser) schema() *Schema {
	p.advance()
	n := p.name()
	s := &Schema{Pos: n.pos, Name: n.text, Label: n.text}
	if p.tok().kind == tString {
		s.Label = p.advance().text
	}
	s.Reads = p.reads()
	p.expect("{")
	for !p.accept("}") {
		s.Fields = append(s.Fields, p.field())
	}
	return s
}

// reads parses the reads clauses: reads [lang] "text".
func (p *parser) reads() []*Reads {
	var rs []*Reads
	for p.tok().isKeyword("reads") {
		p.advance()
		r := &Reads{}
		if p.tok().isName() {
			r.Lang = p.advance().text
		}
		t := p.tok()
		if t.kind != tString {
			panic(p.fail(t.pos, "expected the text to read, a string, found %s", desc(t)))
		}
		p.advance()
		r.Pos, r.Text = t.pos, t.text
		rs = append(rs, r)
	}
	return rs
}

func (p *parser) field() *Field {
	t := p.tok()
	if !t.isName() {
		panic(p.fail(t.pos, "expected a field or `}`, found %s", desc(t)))
	}
	p.advance()
	f := &Field{Pos: t.pos, Name: t.text, Label: names.Label(t.text)}
	p.expect(":")
	f.Type = p.typ()
	if p.accept("<-") {
		n := p.name()
		f.Inverse, f.InversePos = n.text, n.pos
	}
	labelled := false
	for {
		if l := p.tok(); l.kind == tString {
			p.advance()
			if labelled {
				p.errorf(l.pos, "`%s` already has a label", f.Name)
			} else {
				f.Label, labelled = l.text, true
			}
		} else if at := p.tok(); at.isPunct("@") {
			p.advance()
			n := p.tok()
			if n.kind != tName {
				panic(p.fail(n.pos, "expected an annotation name after `@`, found %s", desc(n)))
			}
			p.advance()
			f.Annots = append(f.Annots, &Annot{Pos: at.pos, Name: n.text})
		} else {
			break
		}
	}
	if p.accept("=") {
		f.Expr = p.expr()
	}
	p.accept(",")
	return f
}

// typ parses a type: Name, Name?, [Name] or [Name]?, each with -> before Name for a link.
func (p *parser) typ() *Type {
	array := p.accept("[")
	link := p.accept("->")
	n := p.tok()
	if !n.isName() {
		panic(p.fail(n.pos, "expected a type, found %s", desc(n)))
	}
	p.advance()
	t := &Type{Pos: n.pos, Name: n.text, Array: array, Link: link}
	if array {
		if q := p.tok(); q.isPunct("?") {
			p.advance()
			p.errorf(q.pos, "an array holds values; write [%s]? for an optional array", n.text)
		}
		p.expect("]")
	}
	t.Optional = p.accept("?")
	return t
}

func (p *parser) fn() *Fn {
	kw := p.advance()
	n := p.name()
	fn := &Fn{Pos: n.pos, Rule: kw.text == "rule", Name: n.text}
	fn.Params = p.params()
	if p.accept("->") {
		fn.Result = p.typ()
	} else if t := p.tok(); !fn.Rule {
		panic(p.fail(t.pos, "expected `->` and the result type, found %s", desc(t)))
	}
	fn.Reads = p.reads()
	p.expect("=")
	fn.Body = p.expr()
	return fn
}

func (p *parser) params() []*Param {
	var ps []*Param
	p.expect("(")
	for !p.accept(")") {
		n := p.name()
		p.expect(":")
		param := &Param{Pos: n.pos, Name: n.text, Type: p.typ()}
		if p.accept("=") {
			param.Default = p.expr()
		}
		ps = append(ps, param)
		if !p.accept(",") {
			p.expect(")")
			break
		}
	}
	return ps
}

// precedence is the binding power of the binary operators; all of them are left-associative.
var precedence = map[string]int{
	"??": 1, "or": 2, "and": 3,
	"==": 4, "!=": 4, "<": 4, "<=": 4, ">": 4, ">=": 4,
	"+": 5, "-": 5, "*": 6, "/": 6,
}

// notPrec is what not applies to: not a == b is not (a == b).
const notPrec = 4

func (p *parser) expr() Expr { return p.binary(1) }

func (p *parser) binary(min int) Expr {
	x := p.unary()
	for {
		op := p.tok()
		prec := 0
		if op.kind == tPunct || op.kind == tName {
			prec = precedence[op.text]
		}
		if prec == 0 || prec < min {
			return x
		}
		p.advance()
		x = &Binary{node{op.pos}, op.text, x, p.binary(prec + 1)}
	}
}

func (p *parser) unary() Expr {
	switch t := p.tok(); {
	case t.isPunct("-"):
		p.advance()
		return &Unary{node{t.pos}, "-", p.unary()}
	case t.isKeyword("not"):
		p.advance()
		return &Unary{node{t.pos}, "not", p.binary(notPrec)}
	}
	return p.postfix(p.primary())
}

// postfix parses what follows an operand, binding tightest: .name, ?.name, (args), with { … }.
func (p *parser) postfix(x Expr) Expr {
	for {
		switch t := p.tok(); {
		case t.isPunct("."), t.isPunct("?."):
			p.advance()
			n := p.tok()
			if n.kind != tName {
				panic(p.fail(n.pos, "expected a name after `%s`, found %s", t.text, desc(n)))
			}
			p.advance()
			x = &Member{node{n.pos}, x, n.text, t.text == "?."}
		case t.isPunct("("):
			p.advance()
			c := &Call{node: node{t.pos}, Fn: x}
			for !p.accept(")") {
				c.Args = append(c.Args, p.expr())
				if !p.accept(",") {
					p.expect(")")
					break
				}
			}
			x = c
		case t.isKeyword("with"):
			p.advance()
			x = p.with(t.pos, x)
		default:
			return x
		}
	}
}

func (p *parser) with(pos diag.Pos, x Expr) Expr {
	w := &With{node: node{pos}, X: x}
	p.expect("{")
	for !p.accept("}") {
		n := p.name()
		p.expect(":")
		w.Fields = append(w.Fields, &WithField{Pos: n.pos, Name: n.text, Value: p.expr()})
		if !p.accept(",") {
			p.expect("}")
			break
		}
	}
	return w
}

func (p *parser) primary() Expr {
	t := p.tok()
	switch {
	case t.kind == tNumber:
		p.advance()
		if u, ok := p.unitAfter(t); ok {
			p.advance()
			return &Quantity{node{t.pos}, t.text, u.text, u.pos}
		}
		return &Number{node{t.pos}, t.text}
	case t.kind == tString:
		p.advance()
		return &String{node{t.pos}, t.text}
	case t.isKeyword("if"):
		p.advance()
		e := &If{node: node{t.pos}, Cond: p.expr()}
		p.expectKeyword("then")
		e.Then = p.expr()
		p.expectKeyword("else")
		e.Else = p.expr()
		return e
	case t.isName():
		p.advance()
		id := &Ident{node{t.pos}, t.text}
		if p.accept("=>") {
			return &Lambda{node{t.pos}, []*Ident{id}, p.expr()}
		}
		return id
	case t.isPunct("("):
		if params, ok := p.lambdaParams(); ok {
			return &Lambda{node{t.pos}, params, p.expr()}
		}
		p.advance()
		x := p.expr()
		p.expect(")")
		return x
	}
	panic(p.fail(t.pos, "expected an expression, found %s", desc(t)))
}

// unitAfter reports whether the current token is the unit of the number num: % or a unit name on the same line,
// not followed by a colon, as the next field's name is.
func (p *parser) unitAfter(num token) (token, bool) {
	u := p.tok()
	if u.pos.Line != num.pos.Line {
		return u, false
	}
	if u.isPunct("%") {
		return u, true
	}
	return u, u.isName() && value.UnitName.MatchString(u.text) && !p.peek(1).isPunct(":")
}

// lambdaParams looks ahead for (a, b) =>; when it is there, it consumes it and returns the parameters.
func (p *parser) lambdaParams() ([]*Ident, bool) {
	var params []*Ident
	k := 1
	if p.peek(k).isPunct(")") {
		k++
	} else {
		for {
			n := p.peek(k)
			if !n.isName() {
				return nil, false
			}
			params = append(params, &Ident{node{n.pos}, n.text})
			k++
			if sep := p.peek(k); sep.isPunct(")") {
				k++
				break
			} else if !sep.isPunct(",") {
				return nil, false
			}
			k++
		}
	}
	if !p.peek(k).isPunct("=>") {
		return nil, false
	}
	p.i += k + 1
	return params, true
}
