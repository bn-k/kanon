package eval

import (
	"slices"
	"strings"

	"github.com/func-it/kanon/internal/schema"
	"github.com/func-it/kanon/internal/value"
)

// expr evaluates x in en. The schema check has made sure that x is well typed.
func (e *evaluator) expr(x schema.Expr, en *env) val {
	switch x := x.(type) {
	case *schema.Ident:
		return e.ident(x.Name, en)
	case *schema.Number:
		k := value.Int
		if strings.Contains(x.Text, ".") {
			k = value.Number
		}
		return value.Value{Kind: k, Raw: x.Text, Num: rat(x.Text)}
	case *schema.Quantity:
		return e.quantity(x)
	case *schema.String:
		return value.Value{Kind: value.Text, Raw: x.Value}
	case *schema.Unary:
		return e.unary(x, en)
	case *schema.Binary:
		return e.binary(x, en)
	case *schema.Member:
		return e.member(e.expr(x.X, en), x.Name)
	case *schema.Call:
		return e.call(x, en)
	case *schema.With:
		return e.with(x, en)
	}
	i := x.(*schema.If)
	switch cond := e.expr(i.Cond, en); {
	case cond == nil:
		return nil
	case isBool(cond, true):
		return e.expr(i.Then, en)
	}
	return e.expr(i.Else, en)
}

// ident returns the value called name: a parameter, a field of self or its id, a builtin value or an enum value.
func (e *evaluator) ident(name string, en *env) val {
	if v, ok := en.lookup(name); ok {
		return v
	}
	if o := en.self; o != nil && (name == "id" || o.m.Schema.Field(name) != nil) {
		return e.get(o, name)
	}
	switch name {
	case "today":
		return e.today
	case "none":
		return nil
	case "yes", "no":
		return boolean(name == "yes")
	}
	for _, en := range e.file.Enums {
		if en.Value(name) != nil {
			return enumVal{en, name}
		}
	}
	panic("eval: unknown name " + name)
}

// quantity returns the value of a quantity literal: a percentage, a duration, an amount or a quantity.
func (e *evaluator) quantity(q *schema.Quantity) value.Value {
	n := rat(q.Num)
	switch u := value.DurationUnits[q.Unit]; {
	case q.Unit == "%":
		return value.Value{Kind: value.Percent, Num: n}
	case u != "":
		return value.Value{Kind: value.Duration, Terms: []value.Term{{Num: n, Unit: u}}}
	case q.Money(e.file):
		return value.Value{Kind: value.Money, Num: n, Unit: q.Unit}
	}
	return value.Value{Kind: value.Quantity, Num: n, Unit: q.Unit}
}

func (e *evaluator) unary(u *schema.Unary, en *env) val {
	x := e.expr(u.X, en)
	if x == nil {
		return nil
	}
	v := x.(value.Value)
	switch {
	case u.Op == "not":
		return boolean(!v.Bool)
	case v.Kind == value.Duration:
		return durOf(v).neg().value()
	}
	v.Num = neg(v.Num)
	return v
}

func (e *evaluator) binary(b *schema.Binary, en *env) val {
	switch b.Op {
	case "and", "or":
		// Either side may decide: no for and, yes for or; otherwise none, when a side is none.
		decides := b.Op == "or"
		l := e.expr(b.X, en)
		if isBool(l, decides) {
			return boolean(decides)
		}
		r := e.expr(b.Y, en)
		switch {
		case isBool(r, decides):
			return boolean(decides)
		case l == nil || r == nil:
			return nil
		}
		return boolean(!decides)
	case "??":
		if l := e.expr(b.X, en); l != nil {
			return l
		}
		return e.expr(b.Y, en)
	}
	l, r := e.expr(b.X, en), e.expr(b.Y, en)
	switch b.Op {
	case "==":
		return boolean(equal(l, r))
	case "!=":
		return boolean(!equal(l, r))
	}
	if l == nil || r == nil {
		return nil
	}
	x, y := l.(value.Value), r.(value.Value)
	switch b.Op {
	case "<":
		return boolean(compare(x, y) < 0)
	case "<=":
		return boolean(compare(x, y) <= 0)
	case ">":
		return boolean(compare(x, y) > 0)
	case ">=":
		return boolean(compare(x, y) >= 0)
	}
	return arith(b.Op, x, y)
}

// member returns the field called name of x, or of each object of x, an array, leaving none out.
func (e *evaluator) member(x val, name string) val {
	switch x := x.(type) {
	case *obj:
		return e.get(x, name)
	case []val:
		out := []val{}
		for _, item := range x {
			out = appendFlat(out, e.member(item, name))
		}
		return out
	}
	return nil
}

// appendFlat appends v to out, or the items of v when it is an array; none is left out.
func appendFlat(out []val, v val) []val {
	switch v := v.(type) {
	case nil:
		return out
	case []val:
		return append(out, v...)
	}
	return append(out, v)
}

// call evaluates a call to a builtin, a function or a rule, or a method.
func (e *evaluator) call(c *schema.Call, en *env) val {
	if m, ok := c.Fn.(*schema.Member); ok {
		return e.method(m, c.Args[0].(*schema.Lambda), en)
	}
	name := c.Fn.(*schema.Ident).Name
	if slices.Contains(schema.Builtins, name) {
		return e.builtin(name, c, en)
	}
	i := slices.IndexFunc(e.file.Fns, func(fn *schema.Fn) bool { return fn.Name == name })
	fn := e.file.Fns[i]
	// The arguments are evaluated where the call is, the defaults with the parameters before them.
	inner := &env{}
	for i, p := range fn.Params {
		var v val
		if i < len(c.Args) {
			v = e.expr(c.Args[i], en)
		} else {
			v = e.expr(p.Default, inner)
		}
		inner = inner.bind(p.Name, v)
	}
	return e.expr(fn.Body, inner)
}

// apply calls the function l on x.
func (e *evaluator) apply(l *schema.Lambda, en *env, x val) val {
	return e.expr(l.Body, en.bind(l.Params[0].Name, x))
}

// method evaluates xs.filter(…), xs.map(…) and xs.max-by(…).
func (e *evaluator) method(m *schema.Member, l *schema.Lambda, en *env) val {
	xs := e.expr(m.X, en)
	if xs == nil {
		return nil
	}
	out := []val{}
	var best, key val
	for _, x := range xs.([]val) {
		v := e.apply(l, en, x)
		switch m.Name {
		case "filter":
			if isBool(v, true) {
				out = append(out, x)
			}
		case "map":
			out = appendFlat(out, v)
		default:
			if v != nil && (key == nil || compare(v.(value.Value), key.(value.Value)) > 0) {
				best, key = x, v
			}
		}
	}
	if m.Name == "max-by" {
		return best
	}
	return out
}

// builtin evaluates a call to a builtin function.
func (e *evaluator) builtin(name string, c *schema.Call, en *env) val {
	if name == "sum" || name == "count" {
		return e.aggregate(name, c, en)
	}
	var args []val
	for _, a := range c.Args {
		args = append(args, e.expr(a, en))
	}
	switch name {
	case "round":
		if args[0] == nil {
			return nil
		}
		return round(args[0].(value.Value), args[1].(value.Value).Num)
	case "abs":
		if args[0] == nil {
			return nil
		}
		return abs(args[0].(value.Value))
	}
	// min and max: of the arguments, or of the items of one array.
	if xs, ok := args[0].([]val); ok && len(args) == 1 {
		args = xs
	}
	var best val
	for _, a := range args {
		if a == nil {
			continue
		}
		if best == nil {
			best = a
			continue
		}
		if c := compare(a.(value.Value), best.(value.Value)); name == "max" && c > 0 || name == "min" && c < 0 {
			best = a
		}
	}
	return best
}

// aggregate evaluates sum(xs) and count(xs), or sum(xs, x => …) and count(xs, x => …). An empty sum is the zero
// of what it adds.
func (e *evaluator) aggregate(name string, c *schema.Call, en *env) val {
	xs, _ := e.expr(c.Args[0], en).([]val)
	n := 0
	var total val
	for _, x := range xs {
		if len(c.Args) == 2 {
			x = e.apply(c.Args[1].(*schema.Lambda), en, x)
		}
		switch {
		case name == "count":
			if len(c.Args) == 1 || isBool(x, true) {
				n++
			}
		case x == nil:
		case total == nil:
			total = x
		default:
			total = arith("+", total.(value.Value), x.(value.Value))
		}
	}
	switch {
	case name == "count":
		return value.Value{Kind: value.Int, Num: rint(int64(n))}
	case total == nil:
		return zero(c.Result.Kind)
	}
	return total
}

func boolean(b bool) value.Value { return value.Value{Kind: value.Bool, Bool: b} }

// isBool reports whether v is the Bool b.
func isBool(v val, b bool) bool {
	x, ok := v.(value.Value)
	return ok && x.Kind == value.Bool && x.Bool == b
}
