package pattern

import (
	"errors"
	"maps"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/bn-k/kanon/internal/value"
)

var units = value.NewUnits([]string{"pinch"})

// ingredient accepts a Quantity for quantity and any text for the other fields, as the linter would.
func ingredient(field, v string) bool {
	if field == "quantity" {
		_, err := value.Parse(value.Quantity, v, units)
		return err == nil
	}
	return v != ""
}

func greedyQuantity(field string) bool { return field == "quantity" }

func anything(field, v string) bool { return true }

func lazy(field string) bool { return false }

func greedy(field string) bool { return true }

func mustParse(t *testing.T, text string) *Pattern {
	t.Helper()
	p, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse(%q): %v", text, err)
	}
	return p
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		text string
		col  int
		msg  string
	}{
		{"", 1, "empty pattern"},
		{"   ", 1, "empty pattern"},
		{"{}", 1, "empty placeholder; write a field name between the braces, as in {name}"},
		{"{Price}", 1, `write {price}, not "{Price}"`},
		{"  {Price}", 3, `write {price}, not "{Price}"`},
		{"{unit price}", 1, `write {unit-price}, not "{unit price}"`},
		{"a {123}", 3, `"{123}" is not a field name: use lowercase ASCII letters, digits and hyphens, starting with a letter`},
		{"{name", 1, `unterminated placeholder; close it with "}"`},
		{"{a {b}", 1, `unterminated placeholder; close it with "}"`},
		{"a } b", 3, `unmatched "}"; write \} for a literal brace`},
		{"a ] b", 3, `unmatched "]"; write \] for a literal bracket`},
		{"[of {name}", 1, `unclosed optional group; close it with "]"`},
		{"[[x] {a}", 1, `unclosed optional group; close it with "]"`},
		{"[]", 1, "empty optional group"},
		{"x [ ] y", 3, "empty optional group"},
		{"[[]]", 2, "empty optional group"},
		{`a \x`, 3, `unknown escape "\x"; write \\ for a backslash`},
		{`a \`, 3, `lone "\" at the end; write \\ for a backslash`},
		{"{a} {a}", 5, "field `a` appears twice"},
		{"{a} [{a}]", 6, "field `a` appears twice"},
		{"{a}{b}", 4, "nothing separates `a` from `b`; put text or a space between them"},
		{"{a}[{b}]", 5, "nothing separates `a` from `b`; put text or a space between them"},
		{"{a}[x]{b}", 7, "nothing separates `a` from `b` when the optional group is skipped; put text or a space between them"},
		{"{a} [x]{b}", 8, "nothing separates `a` from `b` when the optional group is skipped; put text or a space between them"},
		{"{a}[x][y]{b}", 10, "nothing separates `a` from `b` when the optional group is skipped; put text or a space between them"},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			p, err := Parse(tt.text)
			if err == nil {
				t.Fatalf("Parse = %+v, want an error", p)
			}
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("error %v is a %T, want *Error", err, err)
			}
			if e.Col != tt.col || e.Msg != tt.msg || e.Error() != tt.msg {
				t.Errorf("error = %d: %s, want %d: %s", e.Col, e.Msg, tt.col, tt.msg)
			}
		})
	}
}

func TestParseOK(t *testing.T) {
	tests := []struct {
		text   string
		fields []string
	}{
		{"{quantity} [of] {name}", []string{"quantity", "name"}},
		{"  {name}  ", []string{"name"}},
		{"[{a} x] {b} [y [{c}]]", []string{"a", "b", "c"}},
		{"{unit-price} EUR", []string{"unit-price"}},
		{"{a} x {b}", []string{"a", "b"}},
		{"{a}[ x]", []string{"a"}},
		{"{a} [x] [y] {b}", []string{"a", "b"}},
		{"just text", nil},
		{`\{\}\[\]\\`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			p := mustParse(t, tt.text)
			if p.Text != tt.text {
				t.Errorf("Text = %q, want %q", p.Text, tt.text)
			}
			if got := p.Fields(); !slices.Equal(got, tt.fields) {
				t.Errorf("Fields = %q, want %q", got, tt.fields)
			}
		})
	}
}

func TestFieldsIsACopy(t *testing.T) {
	p := mustParse(t, "{a} {b}")
	p.Fields()[0] = "z"
	if got := p.Fields(); got[0] != "a" {
		t.Errorf("Fields()[0] = %q after changing a returned slice, want a", got[0])
	}
}

type matchTest struct {
	line string
	want map[string]string // nil: no match
}

func runMatches(t *testing.T, text string, accept func(string, string) bool, greedy func(string) bool, tests []matchTest) {
	t.Helper()
	p := mustParse(t, text)
	for _, tt := range tests {
		t.Run(text+" "+tt.line, func(t *testing.T) {
			got, ok := p.Match(tt.line, accept, greedy)
			if ok != (tt.want != nil) || !maps.Equal(got, tt.want) {
				t.Errorf("Match(%q) = %q, %v; want %q, %v", tt.line, got, ok, tt.want, tt.want != nil)
			}
		})
	}
}

func TestMatchIngredient(t *testing.T) {
	runMatches(t, "{quantity} [of] {name}", ingredient, greedyQuantity, []matchTest{
		{"250 g of flour", map[string]string{"quantity": "250 g", "name": "flour"}},
		{"2 jalapeños", map[string]string{"quantity": "2", "name": "jalapeños"}},
		{"50 cl of milk", map[string]string{"quantity": "50 cl", "name": "milk"}},
		{"50 g of melted butter", map[string]string{"quantity": "50 g", "name": "melted butter"}},
		{"1 pinch of salt", map[string]string{"quantity": "1 pinch", "name": "salt"}},
		{"1 250 g of flour", map[string]string{"quantity": "1 250 g", "name": "flour"}},
		{"  250 g  of\tflour  ", map[string]string{"quantity": "250 g", "name": "flour"}},
		{"of flour", nil},
		{"flour", nil},
		{"250", nil},
		{"", nil},
		{"   ", nil},
	})
}

func TestMatchLiteral(t *testing.T) {
	runMatches(t, "{a} x {b}", anything, lazy, []matchTest{
		{"1 x 2", map[string]string{"a": "1", "b": "2"}},
		{"1 x 2 x 3", map[string]string{"a": "1", "b": "2 x 3"}},
		{"1 X 2", nil},
		{"1 y 2", nil},
		{"1x 2", nil},
		{"1 x", nil},
	})
	runMatches(t, "{a} x {b}", anything, greedy, []matchTest{
		{"1 x 2 x 3", map[string]string{"a": "1 x 2", "b": "3"}},
	})
	runMatches(t, "Total: {amount} EUR", anything, lazy, []matchTest{
		{"Total: 12.50 EUR", map[string]string{"amount": "12.50"}},
		{"Total: EUR", nil},
		{"Sum: 12.50 EUR", nil},
		{"Total: 12.50 EUR extra", nil},
	})
	runMatches(t, "just text", anything, lazy, []matchTest{
		{"just text", map[string]string{}},
		{"just  text", map[string]string{}},
		{"justtext", nil},
		{"just text!", nil},
	})
}

func TestMatchOptional(t *testing.T) {
	runMatches(t, "[the] {name}", anything, lazy, []matchTest{
		{"the cat", map[string]string{"name": "cat"}},
		{"cat", map[string]string{"name": "cat"}},
	})
	runMatches(t, "{n} item[s] left", anything, lazy, []matchTest{
		{"1 item left", map[string]string{"n": "1"}},
		{"2 items left", map[string]string{"n": "2"}},
		{"2 itemleft", nil},
	})
	runMatches(t, "{name} [({note})]", anything, lazy, []matchTest{
		{"flour", map[string]string{"name": "flour"}},
		{"flour (organic)", map[string]string{"name": "flour", "note": "organic"}},
		{"flour ()", map[string]string{"name": "flour ()"}},
	})
	runMatches(t, "{a} [x {b} y] z", anything, lazy, []matchTest{
		{"1 x 2 y z", map[string]string{"a": "1", "b": "2"}},
		{"1 x 2 z", map[string]string{"a": "1 x 2"}},
	})
	runMatches(t, "{a} [x] [y] {b}", anything, lazy, []matchTest{
		{"1 x y 2", map[string]string{"a": "1", "b": "2"}},
		{"1 y 2", map[string]string{"a": "1", "b": "2"}},
		{"1 2", map[string]string{"a": "1", "b": "2"}},
	})
	runMatches(t, "[a] [b]", anything, lazy, []matchTest{
		{"a b", map[string]string{}},
		{"a", map[string]string{}},
		{"b", map[string]string{}},
		{"", map[string]string{}},
		{"c", nil},
	})
	runMatches(t, "{a} [ x ] {b}", anything, lazy, []matchTest{
		{"1 x 2", map[string]string{"a": "1", "b": "2"}},
		{"1 2", map[string]string{"a": "1", "b": "2"}},
	})
}

func TestMatchNestedOptional(t *testing.T) {
	runMatches(t, "{n} [bags [of]] {name}", anything, lazy, []matchTest{
		{"2 bags of flour", map[string]string{"n": "2", "name": "flour"}},
		{"2 bags flour", map[string]string{"n": "2", "name": "flour"}},
		{"2 flour", map[string]string{"n": "2", "name": "flour"}},
		{"2 of flour", map[string]string{"n": "2", "name": "of flour"}},
	})
	runMatches(t, "{n} [[very] {adj}] {name}", anything, lazy, []matchTest{
		{"2 very good apples", map[string]string{"n": "2", "adj": "good", "name": "apples"}},
		{"2 good apples", map[string]string{"n": "2", "adj": "good", "name": "apples"}},
	})
	runMatches(t, "{n} [[very] {adj}] {name}", anything, greedy, []matchTest{
		{"2 very good apples", map[string]string{"n": "2 very good", "name": "apples"}},
	})
	runMatches(t, "[[x] y] {b}", anything, lazy, []matchTest{
		{"x y 1", map[string]string{"b": "1"}},
		{"y 1", map[string]string{"b": "1"}},
		{"1", map[string]string{"b": "1"}},
	})
	runMatches(t, "{a} [[x] y]", anything, lazy, []matchTest{
		{"1 x y", map[string]string{"a": "1"}},
		{"1 y", map[string]string{"a": "1"}},
		{"1", map[string]string{"a": "1"}},
	})
}

func TestMatchEscapes(t *testing.T) {
	runMatches(t, `\{{a}\} \[x\] \\`, anything, lazy, []matchTest{
		{`{1} [x] \`, map[string]string{"a": "1"}},
		{`1 [x] \`, nil},
	})
}

func TestMatchAccept(t *testing.T) {
	never := func(field, v string) bool { return false }
	runMatches(t, "{a} {b}", never, lazy, []matchTest{
		{"1 2", nil},
	})
	digits := func(field, v string) bool { return field != "a" || v == "12" }
	runMatches(t, "{a} {b}", digits, lazy, []matchTest{
		{"12 34", map[string]string{"a": "12", "b": "34"}},
		{"1 2", nil},
	})
	runMatches(t, "{a} {b}", nil, nil, []matchTest{
		{"1 2 3", map[string]string{"a": "1", "b": "2 3"}},
	})
}

func TestMatchValues(t *testing.T) {
	p := mustParse(t, "{a} x {b}")
	seen := map[string][]string{}
	accept := func(field, v string) bool {
		if !utf8.ValidString(v) {
			t.Errorf("accept(%q, %q): a split rune", field, v)
		}
		if v == "" || v[0] == ' ' || v[len(v)-1] == ' ' {
			t.Errorf("accept(%q, %q): empty or untrimmed", field, v)
		}
		seen[field] = append(seen[field], v)
		return true
	}
	got, ok := p.Match("œé œ x é ù", accept, lazy)
	want := map[string]string{"a": "œé œ", "b": "é ù"}
	if !ok || !maps.Equal(got, want) {
		t.Fatalf("Match = %q, %v; want %q", got, ok, want)
	}
	if !slices.Equal(seen["b"], []string{"é ù"}) {
		t.Errorf("b was offered %q, want only the rest of the line", seen["b"])
	}
}
