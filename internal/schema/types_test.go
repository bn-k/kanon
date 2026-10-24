package schema

import (
	"slices"
	"testing"

	"github.com/bn-k/kanon/internal/value"
)

func TestTypesOK(t *testing.T) {
	tests := []string{
		"schema L { q: Number  p: Money  m: Money = p * q }",
		"schema F { l: [L]  t: Percent  net: Money = sum(l.m)  vat: Money = round(net * t, 0.01)  gross: Money = net + vat }\nschema L { m: Money }",
		"schema R { a: Duration  b: Duration?  t: Duration = a + (b ?? 0 min) }",
		"schema B { d: Date  n: Duration  end: Date = d + n  p: Duration = if yes then 1 month else 3 months  j: Duration = d - d }",
		"schema M { v: Version? }\nrule r(m: M) = m.v == none or m.v < 595",
		"schema I { q: Quantity }\nschema R { n: Int  is: [I] }\nfn scale(r: R, n: Int) -> R = r with { n: n, is: r.is.map(i => i with { q: i.q * n / r.n }) }",
		"fn f(x: Int, y: Number) -> Number = x + y * 2 - x / y",
		"fn f(xs: [Int]) -> Int? = max(xs)",
		"fn f(a: Date, b: Date) -> Date = max(a, b, a)",
		"fn f(xs: [Money]) -> Int = count(xs, x => x > x) + count(xs)",
		"fn f(xs: [Int]) -> Int = sum(xs, x => abs(x))",
		"fn f(xs: [Duration], d: Duration) -> Duration = sum(xs) + abs(d) + min(d, d)",
		"schema S { a: Int  b: Text = id }",
		"enum E { a }\nenum G { a }\nrule r(x: G) = x == a and a == x",
		"fn f(x: Int?) -> Int = -(x ?? 1)",
		"fn f(x: Bool?) -> Bool? = not x and x",
		"fn f(xs: [Int]) -> [Int] = xs.filter(x => x > 1)",
		"schema E { d: Date }\nfn f(es: [E]) -> E? = es.max-by(e => e.d)",
		"fn f(m: Money, p: Percent) -> Money = m * p + p * m",
		"fn f(a: Quantity, b: Quantity) -> Number = a / b",
		"fn f(p: Percent, n: Number) -> Number = n * p",
		"fn f(a: DateTime, d: Duration) -> DateTime = a + d - d",
		"fn f(a: DateTime, d: Duration) -> DateTime = d + a",
		"fn f(a: DateTime) -> Duration = a - a",
		"schema O { e: -> E }\nschema E { o: [O] <- e  n: Int = count(o) }",
		`fn f(t: Text) -> Bool = t == "x" and t != "y" and t < "z"`,
		"fn g() -> Int = 1\nfn f() -> Int = g() + g()",
		"schema S { a: Int = b  b: Int = c  c: Int }",
		"rule r(x: Int = 1) = x > 0",
		"schema E { d: Date }\nfn f(e: E?) -> Date? = e?.d",
		"schema E { d: Date? }\nschema V { es: [E] }\nfn f(v: V) -> [Date] = v.es.d",
		"schema E { d: [Date] }\nschema V { es: [E] }\nfn f(v: V) -> [Date] = v.es.d",
		"fn f(a: Int) -> Int? = if yes then a else none",
		"fn f(a: Int) -> Int? = if yes then none else a",
		"fn f(a: Int?) -> Bool? = a > 1 or none",
		"fn f() -> Int? = 1 + none",
		"schema S { a: Int }\nfn f(s: S, t: S) -> Bool = s == t",
		"fn f(a: [Int], b: [Int]) -> Bool = a == b",
		"fn f(v: Version) -> Bool = v >= 1.2 and v < v",
		"fn f(a: Int, b: Number) -> Number = if yes then a else b",
		"fn f(a: Int, b: Number) -> Number = max(a, b)",
		"fn f(a: Int) -> Int = max(none, a)",
		"fn f(a: Int?) -> Int? = (none ?? a) ?? none",
		"fn f(a: Int) -> Int? = none - a + (none * none)",
		"schema S { a: Int }\nfn f(s: S) -> Text = s.id",
		"rule r() = yes\nfn f() -> Bool = r()",
		"fn f(a: [Int]?) -> [Int]? = a?.filter(x => yes)",
		"enum E { a, b }\nrule r() = a == b",
	}
	for _, src := range tests {
		t.Run(src, func(t *testing.T) {
			if _, diags := check(t, src); len(diags) > 0 {
				t.Errorf("got %q", diags)
			}
		})
	}
}

func TestTypesErrors(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		// Operators.
		{"fn f(a: Money, b: Date) -> Money = a + b", "1:38: cannot add `Money` and `Date`"},
		{"fn f(a: Money, b: Date) -> Money = a - b", "1:38: cannot subtract `Date` from `Money`"},
		{"fn f(a: Money, b: Money) -> Money = a * b", "1:39: cannot multiply `Money` by `Money`"},
		{"fn f(a: Int, b: Date) -> Int = a / b", "1:34: cannot divide `Int` by `Date`"},
		{"fn f(a: Money, b: Date) -> Bool = a < b", "1:37: cannot compare `Money` with `Date`"},
		{"fn f(a: Text, b: Int) -> Bool = a == b", "1:35: cannot compare `Text` with `Int`"},
		{"fn f(a: Bool) -> Bool = a < a", "1:27: `Bool` values have no order"},
		{"fn f(a: Int) -> Bool = a and yes", "1:26: `and` takes `Bool` values, not `Int`"},
		{"fn f(a: Int) -> Bool = not a", "1:24: `not` takes a `Bool` value, not `Int`"},
		{"fn f(a: Text) -> Text = -a", "1:25: cannot negate `Text`"},
		{"fn f(a: Int) -> Int = if a then 1 else 2", "1:26: `if` takes a `Bool` condition, not `Int`"},
		{"fn f(a: Int, b: Date) -> Int = if yes then a else b", "1:32: both branches of `if` give one type, not `Int` and `Date`"},
		{"fn f(a: Int?, b: Date) -> Int = a ?? b", "1:35: both sides of `??` give one type, not `Int` and `Date`"},
		{"enum E { a }\nenum G { b }\nrule r(x: E) = x == b", "3:18: cannot compare `E` with `G`"},
		{"fn f(a: [Int], b: Int) -> Bool = a == b", "1:36: cannot compare `[Int]` with `Int`"},
		{"fn f(a: Int) -> Int = zz ?? a", "1:23: unknown name `zz`"},
		{"fn f(a: Int) -> Int = zz + a", "1:23: unknown name `zz`"},

		// What an expression gives.
		{"fn f(a: Int?) -> Int = a + 1", "1:4: `f` returns `Int`, but its expression gives `Int?`; add ?? and a value for none"},
		{"fn f() -> Int = none", "1:4: `f` returns `Int`, but its expression gives `none`"},
		{"rule r() = 1", "1:6: `r` returns `Bool`, but its expression gives `Int`"},
		{"schema S { a: Int  b: Text = a }", "1:20: `b` is `Text`, but its expression gives `Int`"},
		{"fn f(on: Date = 1) -> Bool = yes", "1:6: `on` is `Date`, but its default gives `Int`"},
		{"enum Op { a }\nfn g(op: Op) -> Int = 1\nfn f() -> Int = g(\"x\")", "3:19: `g` takes `Op` as `op`, not `Text`"},
		{"schema E { d: Date }\nschema V { es: [E] }\nfn f(v: V) -> Date = v.es.d", "3:4: `f` returns `Date`, but its expression gives `[Date]`"},
		{"schema R { a: Int }\nfn f(r: R) -> R = r with { a: yes }", "2:28: `a` is `Int`, but the value given is `Bool`"},

		// Fields and none.
		{"schema E { d: Date }\nfn f(e: E?) -> Date? = e.d", "2:26: may be none here: write `?.d`, not `.d`"},
		{"schema V { k: Int }\nfn f(v: V) -> Int = v.kk", "2:23: `V` has no field `kk`; did you mean `k`?"},
		{"fn f(a: Quantity) -> Int = a.n", "1:30: `Quantity` has no fields"},
		{"schema R { a: Int }\nfn f(r: R) -> R = r with { b: 1 }", "2:28: `R` has no field `b`; did you mean `a`?"},
		{"schema R { long: Int }\nfn f(r: R) -> R = r with { b: 1 }", "2:28: `R` has no field `b`"},
		{"fn f(a: Int) -> Int = a with { b: 1 }", "1:25: `with` makes a copy of an object, not of `Int`"},

		// Functions and methods.
		{"fn f(a: Int) -> Int = a.filter(x => x)", "1:25: `filter` works on an array, not on `Int`"},
		{"fn f(a: [Int]) -> [Int] = a.filtr(x => yes)", "1:29: unknown method `filtr`; did you mean `filter`?"},
		{"fn f(a: [Int]) -> [Int] = a.filter", "1:29: `filter` is a method: call it, as in xs.filter(…)"},
		{"fn f(a: [Int]) -> [Int] = a.filter(x => 1)", "1:41: `filter` takes a function that gives `Bool`, not `Int`"},
		{"fn f(a: [Int]) -> [Int] = a.filter(1)", "1:36: `filter` takes a function of one parameter, as in xs.filter(x => …)"},
		{"fn f(a: [Int]) -> [Int] = a.map((x, y) => x)", "1:33: `map` takes a function of one parameter, as in xs.map(x => …)"},
		{"fn f(a: [Int]) -> [Int] = a.filter(x => yes, 2)", "1:29: `filter` takes 1 argument, not 2"},
		{"schema E { d: Bool }\nfn f(es: [E]) -> E? = es.max-by(e => e.d)", "2:40: `max-by` takes a function that gives a value with an order, not `Bool`"},
		{"fn f(a: Int) -> Int = x => a", "1:23: `=>` writes a function, which goes only as an argument, as in xs.map(x => …)"},
		{"fn f() -> Int = sum", "1:17: `sum` is a function: call it, as in sum(…)"},
		{"fn g() -> Int = 1\nfn f() -> Int = g", "2:17: `g` is a function: call it, as in g(…)"},
		{"fn f(a: Int) -> Int = a(1)", "1:23: `a` is not a function"},
		{"fn g() -> Int = 1\nfn f() -> Int = g()(2)", "2:20: only a function or a method can be called"},
		{"schema S { a: Int }\nfn f(s: S) -> Int = s.a(1)", "2:24: only a function or a method can be called"},
		{"schema S { a: Int }\nfn f(s: S) -> Int = s.b(x => 1)", "2:23: `S` has no field `b`; did you mean `a`?"},
		{"fn f() -> Int = (-zz)(2)", "1:19: unknown name `zz`"},
		{"fn f(a: [Int]?) -> [Int]? = a.filter(x => yes)", "1:31: may be none here: write `?.filter`, not `.filter`"},
		{"fn f() -> Int = max(zz)", "1:21: unknown name `zz`"},

		// Builtins.
		{"fn f(a: Int) -> Int = sum(a)", "1:27: `sum` takes an array, not `Int`"},
		{"fn f(a: [Text]) -> Text = sum(a)", "1:31: `sum` adds numbers, amounts, quantities, durations or percentages, not `Text`"},
		{"fn f(xs: [Int]) -> Int = sum(xs, x => yes)", "1:39: `sum` adds numbers, amounts, quantities, durations or percentages, not `Bool`"},
		{"fn f(xs: [Int]) -> Int = sum(xs, 1)", "1:34: `sum` takes a function of one parameter as its second argument, as in sum(lines, l => l.amount)"},
		{"fn f(a: [Int]) -> Int = count(a, x => 1)", "1:39: `count` takes a function that gives `Bool`, not `Int`"},
		{"fn f(a: [Int]) -> Int = count(a, 1)", "1:34: `count` takes a function of one parameter as its second argument, as in count(xs, x => …)"},
		{"fn f() -> Int = round(1)", "1:17: `round` takes 2 arguments, not 1"},
		{"fn f(a: Duration) -> Duration = round(a, 1)", "1:39: `round` takes a number, an amount, a quantity or a percentage, not `Duration`"},
		{"fn f(a: Number, b: Money) -> Number = round(a, b)", "1:48: `round` takes a number as its step, not `Money`"},
		{"fn f(a: Int, b: Date) -> Int = max(a, b)", "1:32: `max` takes values of one type, not `Int` and `Date`"},
		{"fn f(a: Bool) -> Bool = max(a, a)", "1:25: `max` takes values that have an order, not `Bool`"},
		{"fn f(a: [Bool]) -> Bool? = min(a)", "1:28: `min` takes values that have an order, not `Bool`"},
		{"fn f() -> Int = max()", "1:17: `max` takes at least 1 argument, not 0"},
		{"fn f(a: Text) -> Text = abs(a)", "1:29: `abs` takes a number, an amount, a quantity, a duration or a percentage, not `Text`"},

		// Cycles.
		{"schema S { a: Int = a + 1 }", "1:12: `a` depends on itself"},
		{"schema S { a: Int = b  b: Int = a }", "1:12: `a` depends on itself, through `b`"},
		{"schema S { a: Int = b  b: Int = c  c: Int = a }", "1:12: `a` depends on itself, through `b` and `c`"},
		{"fn f(n: Int) -> Int = f(n)", "1:4: `f` calls itself; a function may not recurse"},
		{"fn f() -> Int = g()\nfn g() -> Int = h()\nfn h() -> Int = f()", "1:4: `f` calls itself, through `g` and `h`; a function may not recurse"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			if _, got := check(t, tt.src); !slices.Equal(got, []string{tt.want}) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCallResult checks that Check records what a call gives, so that the sum of no items is a zero of its kind.
func TestCallResult(t *testing.T) {
	f, diags := check(t, "schema L { m: Money }\nschema F { l: [L]  t: Money = sum(l.m) }")
	if len(diags) > 0 {
		t.Fatalf("Check: %v", diags)
	}
	call := f.Schema("F").Fields[1].Expr.(*Call)
	if r := call.Result; r == nil || !r.Native || r.Kind != value.Money || r.Array {
		t.Errorf("Result = %+v, want Money", r)
	}
}

func TestArith(t *testing.T) {
	tests := []struct {
		op   string
		x, y value.Kind
		want value.Kind
		ok   bool
	}{
		{"+", value.Int, value.Int, value.Int, true},
		{"+", value.Int, value.Number, value.Number, true},
		{"+", value.Money, value.Money, value.Money, true},
		{"+", value.Money, value.Int, 0, false},
		{"+", value.Date, value.Duration, value.Date, true},
		{"+", value.Duration, value.Date, value.Date, true},
		{"-", value.Duration, value.Date, 0, false},
		{"-", value.Date, value.Date, value.Duration, true},
		{"-", value.DateTime, value.Date, 0, false},
		{"*", value.Int, value.Int, value.Int, true},
		{"*", value.Money, value.Number, value.Money, true},
		{"*", value.Int, value.Quantity, value.Quantity, true},
		{"*", value.Money, value.Percent, value.Money, true},
		{"*", value.Percent, value.Int, value.Number, true},
		{"*", value.Percent, value.Percent, 0, false},
		{"*", value.Date, value.Int, 0, false},
		{"*", value.Date, value.Percent, 0, false},
		{"/", value.Int, value.Int, value.Number, true},
		{"/", value.Quantity, value.Int, value.Quantity, true},
		{"/", value.Duration, value.Duration, value.Number, true},
		{"/", value.Int, value.Money, 0, false},
		{"%", value.Int, value.Int, 0, false},
	}
	for _, tt := range tests {
		if got, ok := Arith(tt.op, tt.x, tt.y); got != tt.want || ok != tt.ok {
			t.Errorf("Arith(%s, %s, %s) = %s, %v; want %s, %v", tt.op, tt.x, tt.y, got, ok, tt.want, tt.ok)
		}
	}
}

func TestOrdered(t *testing.T) {
	tests := []struct {
		x, y value.Kind
		want bool
	}{
		{value.Int, value.Number, true},
		{value.Money, value.Money, true},
		{value.Text, value.Text, true},
		{value.Version, value.Int, true},
		{value.Number, value.Version, true},
		{value.Date, value.DateTime, false},
		{value.Bool, value.Bool, false},
		{value.Money, value.Int, false},
	}
	for _, tt := range tests {
		if got := Ordered(tt.x, tt.y); got != tt.want {
			t.Errorf("Ordered(%s, %s) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
}
