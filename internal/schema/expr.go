package schema

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/names"
	"github.com/bn-k/kanon/internal/value"
)

// scope holds the names an expression may use.
type scope struct {
	parent *scope
	vars   []*bind
}

// bind is a name in scope: a value of type t (nil when unknown), a function or rule fn, or a builtin function.
type bind struct {
	name    string
	t       *Type
	fn      *Fn
	builtin bool
	field   *Field   // when the name is a field of the schema being checked
	decl    diag.Pos // where the name is declared; zero for a builtin and for id
}

func (s *scope) with(vars ...*bind) *scope { return &scope{parent: s, vars: vars} }

// lookup returns the nearest bind of name, or nil.
func (s *scope) lookup(name string) *bind {
	for ; s != nil; s = s.parent {
		for _, b := range s.vars {
			if b.name == name {
				return b
			}
		}
	}
	return nil
}

// all returns the names in scope, the nearest first.
func (s *scope) all() []string {
	var ns []string
	for ; s != nil; s = s.parent {
		for _, b := range s.vars {
			ns = append(ns, b.name)
		}
	}
	return ns
}

// builtinValues are the types of the builtins that are values; the other builtins are functions.
var builtinValues = map[string]*Type{
	"today": nativeType(value.Date), "none": noneType, "yes": nativeType(value.Bool), "no": nativeType(value.Bool),
}

// globalScope holds the builtins, the values of every enum, and the functions and rules.
func (c *checker) globalScope() *scope {
	sc := &scope{}
	for _, name := range Builtins {
		t := builtinValues[name]
		sc.vars = append(sc.vars, &bind{name: name, t: t, builtin: t == nil})
	}
	for _, e := range c.f.Enums {
		for _, v := range e.Values {
			sc.vars = append(sc.vars, &bind{name: v.Name, t: &Type{Name: e.Name, Enum: e, value: v.Name}, decl: v.Pos})
		}
	}
	for _, fn := range c.f.Fns {
		sc.vars = append(sc.vars, &bind{name: fn.Name, fn: fn, decl: fn.Pos})
	}
	return sc
}

// expr checks e and returns its type: nil when it isn't known, as after an error, which is then reported once.
func (c *checker) expr(e Expr, sc *scope) *Type {
	switch e := e.(type) {
	case *Ident:
		return c.ident(e, sc)
	case *Number:
		c.number(e.Text, e.Pos)
		if strings.Contains(e.Text, ".") {
			return nativeType(value.Number)
		}
		return nativeType(value.Int)
	case *Quantity:
		c.number(e.Num, e.Pos)
		return c.quantity(e)
	case *String:
		return nativeType(value.Text)
	case *Unary:
		return c.unary(e, sc)
	case *Binary:
		return c.binary(e, sc)
	case *Member:
		return c.member(e, c.expr(e.X, sc))
	case *Call:
		e.Result = c.call(e, sc)
		return e.Result
	case *Lambda:
		c.errorf(e.Pos, "`=>` writes a function, which goes only as an argument, as in xs.map(x => …)")
		c.lambda(e, sc, nil)
	case *With:
		return c.with(e, sc)
	case *If:
		return c.ifExpr(e, sc)
	}
	return nil
}

func (c *checker) ident(e *Ident, sc *scope) *Type {
	b := sc.lookup(e.Name)
	switch {
	case b == nil:
		c.errorf(e.Pos, "%s", suggest(fmt.Sprintf("unknown name `%s`", e.Name), e.Name, sc.all()))
		return nil
	}
	c.ref(e.Pos, e.Name, b.decl)
	switch {
	case b.fn != nil || b.builtin:
		c.errorf(e.Pos, "`%s` is a function: call it, as in %[1]s(…)", e.Name)
		return nil
	case b.field != nil && c.field != nil:
		c.deps[c.field] = append(c.deps[c.field], b.field)
	}
	return b.t
}

func (c *checker) unary(e *Unary, sc *scope) *Type {
	x := c.expr(e.X, sc)
	switch {
	case x == nil || x == noneType:
		return x
	case e.Op == "not" && !x.is(value.Bool):
		c.errorf(e.Pos, "`not` takes a `Bool` value, not `%s`", x)
		return nil
	case e.Op == "-" && !x.is(value.Int, value.Number, value.Money, value.Quantity, value.Duration, value.Percent):
		c.errorf(e.Pos, "cannot negate `%s`", x)
		return nil
	}
	return x
}

// arithMsgs say that an operator doesn't apply to two types.
var arithMsgs = map[string]string{
	"+": "cannot add `%s` and `%s`",
	"-": "cannot subtract `%[2]s` from `%[1]s`",
	"*": "cannot multiply `%s` by `%s`",
	"/": "cannot divide `%s` by `%s`",
}

func (c *checker) binary(e *Binary, sc *scope) *Type {
	x, y := c.expr(e.X, sc), c.expr(e.Y, sc)
	boolean := nativeType(value.Bool)
	switch e.Op {
	case "and", "or":
		for _, t := range []*Type{x, y} {
			if t != nil && t != noneType && !t.is(value.Bool) {
				c.errorf(e.Pos, "`%s` takes `Bool` values, not `%s`", e.Op, t)
				return nil
			}
		}
		return opt(boolean, maybe(x) || maybe(y))
	case "??":
		switch {
		case x == nil || y == nil:
			return nil
		case x == noneType:
			return y
		case y == noneType:
			return x
		}
		t, ok := join(x, y)
		if !ok {
			c.errorf(e.Pos, "both sides of `??` give one type, not `%s` and `%s`", opt(x, false), opt(y, false))
			return nil
		}
		return opt(t, y.Optional)
	}
	if x == nil || y == nil {
		return nil
	}
	single := x.Native && y.Native && !x.Array && !y.Array
	switch e.Op {
	case "==", "!=":
		// A date is a day, a date and time an instant: one is almost never equal to the other.
		_, ok := join(x, y)
		switch {
		case ok || x == noneType || y == noneType:
		case single && isTime(x.Kind) && isTime(y.Kind):
			c.errorf(e.Pos, "cannot test a `%s` and a `%s` for equality: compare with >= and <, or compare two dates", x, y)
		case !(single && Ordered(x.Kind, y.Kind)):
			c.errorf(e.Pos, "cannot compare `%s` with `%s`", x, y)
		}
		return boolean
	case "<", "<=", ">", ">=":
		_, same := join(x, y)
		switch {
		case x == noneType || y == noneType || single && Ordered(x.Kind, y.Kind):
			return opt(boolean, maybe(x) || maybe(y))
		case same:
			c.errorf(e.Pos, "`%s` values have no order", opt(x, false))
		default:
			c.errorf(e.Pos, "cannot compare `%s` with `%s`", x, y)
		}
		return nil
	}
	switch {
	case x == noneType && y == noneType:
		return noneType
	case x == noneType:
		return opt(y, true)
	case y == noneType:
		return opt(x, true)
	case single:
		if k, ok := Arith(e.Op, x.Kind, y.Kind); ok {
			return opt(nativeType(k), x.Optional || y.Optional)
		}
	}
	c.errorf(e.Pos, "%s", fmt.Sprintf(arithMsgs[e.Op], x, y))
	return nil
}

// member checks x.name, where x is of type t: a field of an object, or of each object of an array.
func (c *checker) member(e *Member, t *Type) *Type {
	switch {
	case t == nil || t == noneType:
		return t
	case t.Optional && !e.Optional:
		c.errorf(e.Pos, "may be none here: write `?.%s`, not `.%s`", e.Name, e.Name)
	}
	var f *Field
	if t.Schema != nil {
		f = t.Schema.Field(e.Name)
	}
	var ft *Type
	switch {
	case f != nil:
		c.ref(e.Pos, e.Name, f.Pos)
		ft = typeOf(f.Type)
	case e.Name == "id" && t.Schema != nil:
		ft = nativeType(value.Text)
	case t.Array && slices.Contains(Methods, e.Name):
		c.errorf(e.Pos, "`%s` is a method: call it, as in xs.%[1]s(…)", e.Name)
		return nil
	case t.Schema == nil:
		c.errorf(e.Pos, "`%s` has no fields", elem(t))
		return nil
	default:
		msg := suggest(fmt.Sprintf("`%s` has no field `%s`", t.Name, e.Name), e.Name, t.Schema.FieldNames())
		c.errorf(e.Pos, "%s", msg)
		return nil
	}
	switch {
	case ft == nil:
		return nil
	case t.Array:
		return opt(arrayOf(ft), t.Optional)
	}
	return opt(ft, ft.Optional || t.Optional)
}

// call checks a call to a function, a builtin or a method, and returns what it gives.
func (c *checker) call(e *Call, sc *scope) *Type {
	switch fn := e.Fn.(type) {
	case *Ident:
		b := sc.lookup(fn.Name)
		switch {
		case b == nil:
			c.errorf(fn.Pos, "%s", suggest(fmt.Sprintf("unknown name `%s`", fn.Name), fn.Name, sc.all()))
		case b.builtin:
			return c.builtin(fn, e, sc)
		case b.fn != nil:
			c.ref(fn.Pos, fn.Name, b.decl)
			return c.callFn(fn, b.fn, e, sc)
		default:
			c.ref(fn.Pos, fn.Name, b.decl)
			c.errorf(fn.Pos, "`%s` is not a function", fn.Name)
		}
		c.args(e.Args, sc)
		return nil
	case *Member:
		x := c.expr(fn.X, sc)
		if x != nil && x != noneType && (x.Schema == nil || x.Schema.Field(fn.Name) == nil) &&
			(x.Array || slices.Contains(Methods, fn.Name)) {
			return c.method(fn, x, e, sc)
		}
		if c.member(fn, x) == nil {
			c.args(e.Args, sc)
			return nil
		}
	default:
		if c.expr(fn, sc) == nil {
			c.args(e.Args, sc)
			return nil
		}
	}
	c.errorf(e.Pos, "only a function or a method can be called")
	c.args(e.Args, sc)
	return nil
}

// args checks the arguments of a call that went wrong, for the errors they hold; a function is fine there.
func (c *checker) args(args []Expr, sc *scope) {
	for _, a := range args {
		if l, ok := a.(*Lambda); ok {
			c.lambda(l, sc, nil)
		} else {
			c.expr(a, sc)
		}
	}
}

// callFn checks a call to the function or rule fn, and records it for recursion.
func (c *checker) callFn(id *Ident, fn *Fn, e *Call, sc *scope) *Type {
	c.arity(id, fn, len(e.Args))
	for i, a := range e.Args {
		t := c.expr(a, sc)
		if i >= len(fn.Params) {
			continue
		}
		p := fn.Params[i]
		if m := mismatch(typeOf(p.Type), t); m != "" {
			c.errorf(a.At(), "`%s` takes `%s` as `%s`, not %s", fn.Name, p.Type, p.Name, m)
		}
	}
	if c.caller != nil {
		c.calls[c.caller] = append(c.calls[c.caller], fn)
	}
	if fn.Result == nil {
		return nativeType(value.Bool)
	}
	return typeOf(fn.Result)
}

// lambda checks the body of l, whose parameters are values of type t, and returns what it gives.
func (c *checker) lambda(l *Lambda, sc *scope, t *Type) *Type {
	var vars []*bind
	for _, p := range l.Params {
		c.decl(p.Pos, p.Name)
		vars = append(vars, &bind{name: p.Name, t: t, decl: p.Pos})
	}
	return c.expr(l.Body, sc.with(vars...))
}

// fnArg checks a, which must be a function of one parameter, a value of type t. It returns the function and what
// it gives, or a nil function when a is not one.
func (c *checker) fnArg(a Expr, sc *scope, t *Type) (*Lambda, *Type) {
	l, ok := a.(*Lambda)
	if !ok {
		c.expr(a, sc)
		return nil, nil
	}
	body := c.lambda(l, sc, t)
	if len(l.Params) != 1 {
		return nil, nil
	}
	return l, body
}

// ordered reports whether the values of t have an order, so that max can pick one.
func ordered(t *Type) bool { return t.Native && !t.Array && Ordered(t.Kind, t.Kind) }

// method checks xs.name(args), where xs is of type t.
func (c *checker) method(m *Member, t *Type, e *Call, sc *scope) *Type {
	switch {
	case !t.Array:
		c.errorf(m.Pos, "`%s` works on an array, not on `%s`", m.Name, t)
	case !slices.Contains(Methods, m.Name):
		c.errorf(m.Pos, "%s", suggest(fmt.Sprintf("unknown method `%s`", m.Name), m.Name, Methods))
	case len(e.Args) != 1:
		c.errorf(m.Pos, "`%s` takes 1 argument, not %d", m.Name, len(e.Args))
	default:
		if t.Optional && !m.Optional {
			c.errorf(m.Pos, "may be none here: write `?.%s`, not `.%s`", m.Name, m.Name)
		}
		l, body := c.fnArg(e.Args[0], sc, elem(t))
		if l == nil {
			c.errorf(e.Args[0].At(), "`%s` takes a function of one parameter, as in xs.%[1]s(x => …)", m.Name)
			return nil
		}
		if body == nil || body == noneType {
			return nil
		}
		switch m.Name {
		case "filter":
			if !body.is(value.Bool) {
				c.errorf(l.Body.At(), "`filter` takes a function that gives `Bool`, not `%s`", body)
			}
			return opt(arrayOf(t), t.Optional)
		case "map":
			return opt(arrayOf(body), t.Optional)
		}
		if !ordered(body) {
			c.errorf(l.Body.At(), "`max-by` takes a function that gives a value with an order, not `%s`", body)
		}
		return opt(elem(t), true)
	}
	c.args(e.Args, sc)
	return nil
}

// builtinArity is the least and the most number of arguments of each builtin function; -1 is no most.
var builtinArity = map[string][2]int{
	"sum": {1, 2}, "count": {1, 2}, "round": {2, 2}, "abs": {1, 1}, "min": {1, -1}, "max": {1, -1},
}

// builtin checks a call to a builtin function.
func (c *checker) builtin(id *Ident, e *Call, sc *scope) *Type {
	if n := builtinArity[id.Name]; len(e.Args) < n[0] || n[1] >= 0 && len(e.Args) > n[1] {
		c.errorf(id.Pos, "`%s` takes %s, not %d", id.Name, takes(n[0], n[1]), len(e.Args))
		c.args(e.Args, sc)
		return nil
	}
	switch id.Name {
	case "sum", "count":
		return c.aggregate(id, e, sc)
	case "round":
		x, step := c.expr(e.Args[0], sc), c.expr(e.Args[1], sc)
		if x != nil && x != noneType && !x.is(value.Int, value.Number, value.Money, value.Quantity, value.Percent) {
			c.errorf(e.Args[0].At(), "`round` takes a number, an amount, a quantity or a percentage, not `%s`", x)
			x = nil
		}
		if step != nil && !step.is(value.Int, value.Number) {
			c.errorf(e.Args[1].At(), "`round` takes a number as its step, not `%s`", step)
		}
		return x
	case "abs":
		x := c.expr(e.Args[0], sc)
		if x != nil && x != noneType && !x.is(value.Int, value.Number, value.Money, value.Quantity, value.Duration, value.Percent) {
			c.errorf(e.Args[0].At(), "`abs` takes a number, an amount, a quantity, a duration or a percentage, not `%s`", x)
			return nil
		}
		return x
	}
	return c.extremum(id, e, sc)
}

// aggregate checks sum(xs) and count(xs), or sum(xs, x => …) and count(xs, x => …).
func (c *checker) aggregate(id *Ident, e *Call, sc *scope) *Type {
	xs := c.expr(e.Args[0], sc)
	if xs != nil && !xs.Array {
		c.errorf(e.Args[0].At(), "`%s` takes an array, not `%s`", id.Name, xs)
		c.args(e.Args[1:], sc)
		return nil
	}
	var item *Type
	if xs != nil {
		item = elem(xs)
	}
	at := e.Args[0].At()
	if len(e.Args) == 2 {
		l, body := c.fnArg(e.Args[1], sc, item)
		if l == nil {
			example := map[string]string{"sum": "sum(lines, l => l.amount)", "count": "count(xs, x => …)"}[id.Name]
			c.errorf(e.Args[1].At(), "`%s` takes a function of one parameter as its second argument, as in %s", id.Name, example)
			return nil
		}
		item, at = body, l.Body.At()
	}
	if id.Name == "count" {
		if len(e.Args) == 2 && item != nil && item != noneType && !item.is(value.Bool) {
			c.errorf(at, "`count` takes a function that gives `Bool`, not `%s`", item)
		}
		return nativeType(value.Int)
	}
	switch {
	case item == nil:
		return nil
	case !item.is(value.Int, value.Number, value.Money, value.Quantity, value.Duration, value.Percent):
		c.errorf(at, "`sum` adds numbers, amounts, quantities, durations or percentages, not `%s`", item)
		return nil
	}
	return opt(item, false)
}

// extremum checks min and max: of their arguments, or of the items of one array, which may be empty.
func (c *checker) extremum(id *Ident, e *Call, sc *scope) *Type {
	var t *Type
	optional := true
	for i, a := range e.Args {
		at := c.expr(a, sc)
		switch {
		case at == nil || at == noneType:
			continue
		case len(e.Args) == 1 && at.Array:
			at = opt(elem(at), true)
		case i > 0 && t != nil:
			j, ok := join(t, at)
			if !ok {
				c.errorf(id.Pos, "`%s` takes values of one type, not `%s` and `%s`", id.Name, opt(t, false), opt(at, false))
				c.args(e.Args[i+1:], sc)
				return nil
			}
			at = opt(j, at.Optional)
		}
		t, optional = at, optional && at.Optional
	}
	switch {
	case t == nil:
		return nil
	case !ordered(t):
		c.errorf(id.Pos, "`%s` takes values that have an order, not `%s`", id.Name, opt(t, false))
		return nil
	}
	return opt(t, optional)
}

func (c *checker) with(e *With, sc *scope) *Type {
	x := c.expr(e.X, sc)
	if x != nil && x != noneType && (x.Schema == nil || x.Array) {
		c.errorf(e.Pos, "`with` makes a copy of an object, not of `%s`", x)
		x = nil
	}
	for _, wf := range e.Fields {
		t := c.expr(wf.Value, sc)
		if x == nil || x == noneType {
			continue
		}
		f := x.Schema.Field(wf.Name)
		if f == nil {
			c.errorf(wf.Pos, "%s", suggest(fmt.Sprintf("`%s` has no field `%s`", x.Name, wf.Name), wf.Name, x.Schema.FieldNames()))
			continue
		}
		c.ref(wf.Pos, wf.Name, f.Pos)
		if m := mismatch(typeOf(f.Type), t); m != "" {
			c.errorf(wf.Pos, "`%s` is `%s`, but the value given is %s", wf.Name, f.Type, m)
		}
	}
	return x
}

func (c *checker) ifExpr(e *If, sc *scope) *Type {
	cond := c.expr(e.Cond, sc)
	if cond != nil && cond != noneType && !cond.is(value.Bool) {
		c.errorf(e.Cond.At(), "`if` takes a `Bool` condition, not `%s`", cond)
	}
	a, b := c.expr(e.Then, sc), c.expr(e.Else, sc)
	switch {
	case a == nil || b == nil:
		return nil
	case a == noneType:
		return opt(b, true)
	case b == noneType:
		return opt(a, true)
	}
	t, ok := join(a, b)
	if !ok {
		c.errorf(e.Pos, "both branches of `if` give one type, not `%s` and `%s`", a, b)
		return nil
	}
	return opt(t, a.Optional || b.Optional || maybe(cond))
}

// arity checks the number of arguments of a call to fn: those without a default are needed.
func (c *checker) arity(id *Ident, fn *Fn, n int) {
	least := 0
	for _, p := range fn.Params {
		if p.Default == nil {
			least++
		}
	}
	if most := len(fn.Params); n < least || n > most {
		c.errorf(id.Pos, "`%s` takes %s, not %d", fn.Name, takes(least, most), n)
	}
}

// takes says how many arguments a function takes, from least to most; a most of -1 is no limit.
func takes(least, most int) string {
	switch {
	case most < 0:
		return fmt.Sprintf("at least %d argument%s", least, plural(least))
	case most == 0:
		return "no arguments"
	case least == most:
		return fmt.Sprintf("%d argument%s", most, plural(most))
	case most == least+1:
		return fmt.Sprintf("%d or %d arguments", least, most)
	}
	return fmt.Sprintf("%d to %d arguments", least, most)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// quantity checks the unit of a quantity literal, and returns its type: a percentage, a duration, a quantity, or an
// amount when its unit is a currency code that names no unit; nil when its unit is unknown, already reported.
// number reports a number badly grouped, as in 1 25, saying what to write; its type stays known.
func (c *checker) number(text string, pos diag.Pos) {
	if _, err := value.Parse(value.Number, text, nil); err != nil {
		c.errorf(pos, "%s", err.Error())
	}
}

func (c *checker) quantity(q *Quantity) *Type {
	u := q.Unit
	switch {
	case u == "%":
		return nativeType(value.Percent)
	case value.DurationUnits[u] != "":
		return nativeType(value.Duration)
	case q.Money(c.f):
		return nativeType(value.Money)
	}
	declared := c.f.UnitNames()
	if i := slices.Index(declared, u); i >= 0 {
		c.ref(q.UnitPos, u, c.f.Units[i].Pos)
	}
	if !slices.Contains(value.BuiltinUnits, u) && !slices.Contains(declared, u) {
		units := append(slices.Sorted(maps.Keys(value.DurationUnits)), value.NewUnits(declared).Names()...)
		if s := names.Closest(u, units); s != "" {
			c.errorf(q.UnitPos, "unknown unit `%s`; did you mean `%s`?", u, s)
		} else {
			c.errorf(q.UnitPos, "unknown unit `%s`; a schema declares its own units, as in: unit %s", u, u)
		}
		return nil
	}
	return nativeType(value.Quantity)
}

// cycle returns the shortest path from start back to itself through next, start left out, and whether there is
// one: a direct loop is an empty path.
func cycle[T comparable](start T, next func(T) []T) ([]T, bool) {
	prev := map[T]T{}
	seen := map[T]bool{start: true}
	queue := []T{start}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, m := range next(n) {
			if m == start {
				var path []T
				for ; n != start; n = prev[n] {
					path = append([]T{n}, path...)
				}
				return path, true
			}
			if !seen[m] {
				seen[m], prev[m] = true, n
				queue = append(queue, m)
			}
		}
	}
	return nil, false
}

// cycles reports each cycle among the nodes once, at its first node: "`a` depends on itself, through `b`", and
// then tail.
func cycles[T comparable](c *checker, nodes []T, next func(T) []T, pos func(T) diag.Pos, name func(T) string, msg, tail string) {
	reported := map[T]bool{}
	for _, n := range nodes {
		if reported[n] {
			continue
		}
		path, ok := cycle(n, next)
		if !ok {
			continue
		}
		reported[n] = true
		var through []string
		for _, p := range path {
			reported[p] = true
			through = append(through, "`"+name(p)+"`")
		}
		s := "`" + name(n) + "` " + msg
		if len(through) > 0 {
			s += ", through " + andList(through)
		}
		c.errorf(pos(n), "%s", s+tail)
	}
}

// andList joins items as "a, b and c".
func andList(items []string) string {
	if len(items) == 1 {
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}
