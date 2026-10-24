package schema

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// exprString writes e back fully parenthesized, so precedence shows: ((a + b) * c).
func exprString(e Expr) string {
	switch e := e.(type) {
	case nil:
		return "<nil>"
	case *Ident:
		return e.Name
	case *Number:
		return e.Text
	case *Quantity:
		return e.Num + " " + e.Unit
	case *String:
		return strconv.Quote(e.Value)
	case *Unary:
		if e.Op == "not" {
			return "(not " + exprString(e.X) + ")"
		}
		return "(" + e.Op + exprString(e.X) + ")"
	case *Binary:
		return "(" + exprString(e.X) + " " + e.Op + " " + exprString(e.Y) + ")"
	case *Member:
		dot := "."
		if e.Optional {
			dot = "?."
		}
		return exprString(e.X) + dot + e.Name
	case *Call:
		var args []string
		for _, a := range e.Args {
			args = append(args, exprString(a))
		}
		return exprString(e.Fn) + "(" + strings.Join(args, ", ") + ")"
	case *Lambda:
		var ps []string
		for _, p := range e.Params {
			ps = append(ps, p.Name)
		}
		if len(ps) == 1 {
			return "(" + ps[0] + " => " + exprString(e.Body) + ")"
		}
		return "((" + strings.Join(ps, ", ") + ") => " + exprString(e.Body) + ")"
	case *With:
		var fs []string
		for _, f := range e.Fields {
			fs = append(fs, f.Name+": "+exprString(f.Value))
		}
		return "(" + exprString(e.X) + " with {" + strings.Join(fs, ", ") + "})"
	case *If:
		return "(if " + exprString(e.Cond) + " then " + exprString(e.Then) + " else " + exprString(e.Else) + ")"
	}
	return fmt.Sprintf("<%T>", e)
}

func readsString(rs []*Reads) string {
	var b strings.Builder
	for _, r := range rs {
		b.WriteString(" reads")
		if r.Lang != "" {
			b.WriteString(" " + r.Lang)
		}
		b.WriteString(" " + strconv.Quote(r.Text))
	}
	return b.String()
}

// fileString writes f back in a canonical form.
func fileString(f *File) string {
	var b strings.Builder
	for _, u := range f.Units {
		fmt.Fprintf(&b, "unit %s\n", u.Name)
	}
	for _, e := range f.Enums {
		fmt.Fprintf(&b, "enum %s { %s }\n", e.Name, strings.Join(e.ValueNames(), ", "))
	}
	for _, s := range f.Schemas {
		fmt.Fprintf(&b, "schema %s%s {\n", s.Name, readsString(s.Reads))
		for _, fd := range s.Fields {
			fmt.Fprintf(&b, "  %s: %s", fd.Name, fd.Type)
			if fd.Inverse != "" {
				b.WriteString(" <- " + fd.Inverse)
			}
			for _, a := range fd.Annots {
				b.WriteString(" @" + a.Name)
			}
			if fd.Expr != nil {
				b.WriteString(" = " + exprString(fd.Expr))
			}
			b.WriteString("\n")
		}
		b.WriteString("}\n")
	}
	for _, fn := range f.Fns {
		kw := "fn"
		if fn.Rule {
			kw = "rule"
		}
		var ps []string
		for _, p := range fn.Params {
			s := p.Name + ": " + p.Type.String()
			if p.Default != nil {
				s += " = " + exprString(p.Default)
			}
			ps = append(ps, s)
		}
		fmt.Fprintf(&b, "%s %s(%s)", kw, fn.Name, strings.Join(ps, ", "))
		if fn.Result != nil {
			b.WriteString(" -> " + fn.Result.String())
		}
		fmt.Fprintf(&b, "%s = %s\n", readsString(fn.Reads), exprString(fn.Body))
	}
	return b.String()
}

// parseExpr parses src as the body of a function.
func parseExpr(t *testing.T, src string) Expr {
	t.Helper()
	f, diags := Parse("t.kanonarch", "fn t() -> Int = "+src)
	if len(diags) > 0 {
		t.Fatalf("Parse: %v", diags)
	}
	if len(f.Fns) != 1 {
		t.Fatalf("got %d functions, want 1", len(f.Fns))
	}
	return f.Fns[0].Body
}

func TestParseExpr(t *testing.T) {
	tests := []struct{ src, want string }{
		{"a + b * c", "(a + (b * c))"},
		{"(a + b) * c", "((a + b) * c)"},
		{"a - b - c", "((a - b) - c)"},
		{"a / b * c", "((a / b) * c)"},
		{"a ?? b or c", "(a ?? (b or c))"},
		{"x ?? y ?? z", "((x ?? y) ?? z)"},
		{"a or b and c", "(a or (b and c))"},
		{"a and b == c", "(a and (b == c))"},
		{"a == b + c", "(a == (b + c))"},
		{"a <= b != c", "((a <= b) != c)"},
		{"a < b > c >= d", "(((a < b) > c) >= d)"},
		{"-a * b", "((-a) * b)"},
		{"- a.b", "(-a.b)"},
		{"a * -b", "(a * (-b))"},
		{"not a == b", "(not (a == b))"},
		{"not a and b", "((not a) and b)"},
		{"a.b?.c(d, e,)", "a.b?.c(d, e)"},
		{"f()", "f()"},
		{"f(g(x))(y)", "f(g(x))(y)"},
		{"x with { a: 1, b: 2, }", "(x with {a: 1, b: 2})"},
		{"x with {}", "(x with {})"},
		{"a + b with { c: d }", "(a + (b with {c: d}))"},
		{"i => i.x", "(i => i.x)"},
		{"(a, b) => a + b", "((a, b) => (a + b))"},
		{"() => 1", "(() => 1)"},
		{"(a) + b", "(a + b)"},
		{"(a)", "a"},
		{"if a then 1 month else 3 months", "(if a then 1 month else 3 months)"},
		{"if a then 2 else b", "(if a then 2 else b)"},
		{"2 years", "2 years"},
		{"20 %", "20 %"},
		{"0 min + 0.5 kg", "(0 min + 0.5 kg)"},
		{"a * 2", "(a * 2)"},
		{`"s"`, `"s"`},
		{"net-total * vat-rate", "(net-total * vat-rate)"},
		{"net-total - net", "(net-total - net)"},
		{"v.services.filter(s => s.operation == op).max-by(s => s.date)",
			"v.services.filter((s => (s.operation == op))).max-by((s => s.date))"},
		{"last(v, safety-check)?.date + 2 years >= on",
			"((last(v, safety-check)?.date + 2 years) >= on)"},
		{"preparation + (resting ?? 0 min) + (cooking ?? 0 min)",
			"((preparation + (resting ?? 0 min)) + (cooking ?? 0 min))"},
		{"round(net-total * vat-rate, 0.01)", "round((net-total * vat-rate), 0.01)"},
		{"r with { portions: n, ingredients: r.ingredients.map(i => i with { quantity: i.quantity * n / r.portions }), }",
			"(r with {portions: n, ingredients: r.ingredients.map((i => (i with {quantity: ((i.quantity * n) / r.portions)})))})"},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			if got := exprString(parseExpr(t, tt.src)); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestParseDecls(t *testing.T) {
	src := `// A comment.
unit pinch
unit cup
enum Condition { mint, good worn, damaged, }
schema Ingredient reads "{quantity} [of] {name}" reads en "{name}" {
  quantity: Quantity
  name:     Text @heading
  notes:    [Prose]? @body @sections,
  total:    Money = a * b
}
schema Bid { terms: Terms  form: Form }
schema L {
  a: -> A
  b: -> A?
  c: [-> A]
  d: [-> A]?
  e: [A] <- f
  g: A? <- h
}
schema Q {
  a: Int = 2 b: Int
  c: Quantity = 2 kg
  d: Int = 2
  kg: Int
}
fn scale(r: Recipe, n: Int = 2,) -> Recipe reads en "{r.title}" = r with { portions: n }
rule ok(v: V) = yes
rule ok2() -> Bool reads "x" reads en "y" = no
`
	want := `unit pinch
unit cup
enum Condition { mint, good, worn, damaged }
schema Ingredient reads "{quantity} [of] {name}" reads en "{name}" {
  quantity: Quantity
  name: Text @heading
  notes: [Prose]? @body @sections
  total: Money = (a * b)
}
schema Bid {
  terms: Terms
  form: Form
}
schema L {
  a: -> A
  b: -> A?
  c: [-> A]
  d: [-> A]?
  e: [A] <- f
  g: A? <- h
}
schema Q {
  a: Int = 2
  b: Int
  c: Quantity = 2 kg
  d: Int = 2
  kg: Int
}
fn scale(r: Recipe, n: Int = 2) -> Recipe reads en "{r.title}" = (r with {portions: n})
rule ok(v: V) = yes
rule ok2() -> Bool reads "x" reads en "y" = no
`
	f, diags := Parse("t.kanonarch", src)
	if len(diags) > 0 {
		t.Fatalf("Parse: %v", diags)
	}
	if f.Path != "t.kanonarch" {
		t.Errorf("Path = %q", f.Path)
	}
	if got := fileString(f); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestParsePositions(t *testing.T) {
	f, diags := Parse("t.kanonarch", "schema S reads \"x\" {\n  a: [T]? @heading\n  b: Int = x.y + 1\n}\nunit u\nfn f(p: Int) -> Int = if p then (q, r) => s with { z: 1 } else -1")
	if len(diags) > 0 {
		t.Fatalf("Parse: %v", diags)
	}
	s := f.Schemas[0]
	b := s.Fields[1].Expr.(*Binary)
	m := b.X.(*Member)
	fn := f.Fns[0]
	iff := fn.Body.(*If)
	lam := iff.Then.(*Lambda)
	with := lam.Body.(*With)
	tests := []struct {
		what string
		got  any
		want string
	}{
		{"schema", s.Pos, "t.kanonarch:1:8"},
		{"reads", s.Reads[0].Pos, "t.kanonarch:1:16"},
		{"field", s.Fields[0].Pos, "t.kanonarch:2:3"},
		{"type", s.Fields[0].Type.Pos, "t.kanonarch:2:7"},
		{"annot", s.Fields[0].Annots[0].Pos, "t.kanonarch:2:11"},
		{"binary", b.At(), "t.kanonarch:3:16"},
		{"member", m.At(), "t.kanonarch:3:14"},
		{"ident", m.X.At(), "t.kanonarch:3:12"},
		{"number", b.Y.At(), "t.kanonarch:3:18"},
		{"unit", f.Units[0].Pos, "t.kanonarch:5:6"},
		{"fn", fn.Pos, "t.kanonarch:6:4"},
		{"param", fn.Params[0].Pos, "t.kanonarch:6:6"},
		{"if", iff.At(), "t.kanonarch:6:23"},
		{"lambda", lam.At(), "t.kanonarch:6:33"},
		{"lambda param", lam.Params[1].At(), "t.kanonarch:6:37"},
		{"with", with.At(), "t.kanonarch:6:45"},
		{"with field", with.Fields[0].Pos, "t.kanonarch:6:52"},
		{"unary", iff.Else.At(), "t.kanonarch:6:64"},
	}
	for _, tt := range tests {
		if got := fmt.Sprint(tt.got); got != tt.want {
			t.Errorf("%s at %s, want %s", tt.what, got, tt.want)
		}
	}
	q := parseExpr(t, "2 kg").(*Quantity)
	if q.UnitPos.Col != 19 || q.At().Col != 17 {
		t.Errorf("quantity at %v, unit at %v", q.At(), q.UnitPos)
	}
	c := parseExpr(t, "f(x)").(*Call)
	if c.At().Col != 18 {
		t.Errorf("call at %v, want col 18", c.At())
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		src  string
		want []string
	}{
		{"schema S { a Text }", []string{"1:14: expected `:`, found `Text`"}},
		{"schema S { a: [Text?] }", []string{"1:20: an array holds values; write [Text]? for an optional array"}},
		{`schema S { a: Text "A" "B" }`, []string{
			"1:20: a field holds no label: remove \"A\"; a heading in the document finds the field by its name, `a`",
			"1:24: a field holds no label: remove \"B\"; a heading in the document finds the field by its name, `a`",
		}},
		{`schema S { a: Text "É" }`, []string{
			"1:20: a field holds no label: remove \"É\"; a heading in the document finds the field by its name, `a`",
		}},
		{`schema S "S" {}`, []string{"1:10: a schema holds no label: remove \"S\"; a document writes `type: S`"}},
		{`unit u "u"`, []string{"1:8: a unit holds no label: remove \"u\"; the document writes `u`"}},
		{"schema S { a: }", []string{"1:15: expected a type, found `}`"}},
		{"schema S { a: -> }", []string{"1:18: expected a type, found `}`"}},
		{"schema S { a: [A] <- }", []string{"1:22: expected a name, found `}`"}},
		{"schema if {}", []string{"1:8: expected a name, found `if`"}},
		{"schema S reads {}", []string{"1:16: expected the text to read, a string, found `{`"}},
		{"schema S { a: Text @ }", []string{"1:22: expected an annotation name after `@`, found `}`"}},
		{"schema S { 12 }", []string{"1:12: expected a field or `}`, found `12`"}},
		{"foo", []string{"1:1: expected a declaration (unit, enum, schema, fn or rule), found `foo`"}},
		{`"x"`, []string{"1:1: expected a declaration (unit, enum, schema, fn or rule), found a string"}},
		{"enum E { a \"A\" \"B\" }", []string{
			"1:12: an enum value holds no label: remove \"A\"; the document writes `a`",
			"1:16: an enum value holds no label: remove \"B\"; the document writes `a`",
		}},
		{"enum E { \"A\" }", []string{"1:10: expected a value or `}`, found a string"}},
		{"enum E { a", []string{"1:11: expected a value or `}`, found end of file"}},
		{"unit", []string{"1:5: expected a name, found end of file"}},
		{"fn f() = 1", []string{"1:8: expected `->` and the result type, found `=`"}},
		{"fn f(a Int) -> Int = a", []string{"1:8: expected `:`, found `Int`"}},
		{"rule r( -> Bool = yes", []string{"1:9: expected a name, found `->`"}},
		{"rule r(a: Int b: Int) = yes", []string{"1:15: expected `)`, found `b`"}},
		{"rule r() yes", []string{"1:10: expected `=`, found `yes`"}},
		{"fn f() -> Int =", []string{"1:16: expected an expression, found end of file"}},
		{"fn f() -> Int = (a", []string{"1:19: expected `)`, found end of file"}},
		{"fn f() -> Int = a.", []string{"1:19: expected a name after `.`, found end of file"}},
		{"fn f() -> Int = a?.(", []string{"1:20: expected a name after `?.`, found `(`"}},
		{"fn f() -> Int = x with { a 1 }", []string{"1:28: expected `:`, found `1`"}},
		{"fn f() -> Int = x with a", []string{"1:24: expected `{`, found `a`"}},
		{"fn f() -> Int = x with { 1: a }", []string{"1:26: expected a name, found `1`"}},
		{"fn f() -> Int = x with { a: 1 b: 2 }", []string{"1:31: expected `}`, found `b`"}},
		{"fn f() -> Int = if a then b", []string{"1:28: expected `else`, found end of file"}},
		{"fn f() -> Int = if a b", []string{"1:22: expected `then`, found `b`"}},
		{"fn f() -> Int = g(a b)", []string{"1:21: expected `)`, found `b`"}},
		{"fn f() -> Int = then", []string{"1:17: expected an expression, found `then`"}},
		{"fn f() -> Int = a b", []string{"1:19: expected a declaration (unit, enum, schema, fn or rule), found `b`"}},
		{"fn f() -> Int = (a, b) + 1", []string{"1:19: expected `)`, found `,`"}},
		{"schema A { a: Text # }", []string{`1:20: unexpected character "#"`}},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			_, diags := Parse("t", tt.src)
			if got := diagStrings(diags); !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestParseASCII: a schema is written in ASCII, in its strings and comments too; one diagnostic per line at most,
// and none where a label already is the mistake.
func TestParseASCII(t *testing.T) {
	tests := []struct {
		src  string
		want []string
	}{
		{"schema S { é: Text }", []string{`1:12: a schema is written in ASCII: write "e", not "é"`}},
		{"// FAÇADE\nunit u", []string{`1:6: a schema is written in ASCII: write "C", not "Ç"`}},
		{"rule r() -> Bool reads \"5°\" = yes", []string{`1:26: a schema is written in ASCII: write "o", not "°"`}},
		{"// « x »", []string{`1:4: a schema is written in ASCII: rewrite without "«"`}},
		{"// éé\n// ü", []string{
			`1:4: a schema is written in ASCII: write "e", not "é"`,
			`2:4: a schema is written in ASCII: write "u", not "ü"`,
		}},
		{"// éa ü", []string{`1:4: a schema is written in ASCII: write "e", not "é"`}},
		{"schema S { a: Text « }", []string{`1:20: a schema is written in ASCII: rewrite without "«"`}},
		{"schema S { a Tèxt }", []string{
			"1:14: expected `:`, found `Tèxt`",
			`1:15: a schema is written in ASCII: write "e", not "è"`,
		}},
		{"schema S { a: Text \"é\" }", []string{
			"1:20: a field holds no label: remove \"é\"; a heading in the document finds the field by its name, `a`",
		}},
		{"schema S { a: Text }\n// à la carte", []string{`2:4: a schema is written in ASCII: write "a", not "à"`}},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			_, diags := Parse("t", tt.src)
			if got := diagStrings(diags); !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseRecovery(t *testing.T) {
	tests := []struct {
		src   string
		diags []string
		file  string
	}{
		{
			"schema A { a Text }\nschema B { b: }\nunit u",
			[]string{"1:14: expected `:`, found `Text`", "2:15: expected a type, found `}`"},
			"unit u\n",
		},
		{
			"schema A {\n  a: Text\nschema B { b: Text }",
			[]string{"3:1: expected a field or `}`, found `schema`"},
			"schema B {\n  b: Text\n}\n",
		},
		{
			"foo schema A {}\nunit u",
			[]string{"1:1: expected a declaration (unit, enum, schema, fn or rule), found `foo`"},
			"unit u\n",
		},
		{
			"a # b\nfn f() -> Int = (\nrule r() = yes",
			[]string{
				"1:1: expected a declaration (unit, enum, schema, fn or rule), found `a`",
				`1:3: unexpected character "#"`,
				"3:1: expected an expression, found `rule`",
			},
			"rule r() = yes\n",
		},
		{
			"schema A { a: [Text?] b: Text \"B\" }",
			[]string{
				"1:20: an array holds values; write [Text]? for an optional array",
				"1:31: a field holds no label: remove \"B\"; a heading in the document finds the field by its name, `b`",
			},
			"schema A {\n  a: [Text]\n  b: Text\n}\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			f, diags := Parse("t", tt.src)
			if got := diagStrings(diags); !slices.Equal(got, tt.diags) {
				t.Errorf("diags: got %q, want %q", got, tt.diags)
			}
			if got := fileString(f); got != tt.file {
				t.Errorf("file: got\n%s\nwant\n%s", got, tt.file)
			}
		})
	}
}
