package pattern

import (
	"errors"
	"maps"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/func-it/kanon/internal/value"
)

var units = value.NewUnits([]string{"pincee"})

// ingredient accepts a Quantity for quantite and any text for the other fields, as the linter would.
func ingredient(field, v string) bool {
	if field == "quantite" {
		_, err := value.Parse(value.Quantity, v, units)
		return err == nil
	}
	return v != ""
}

func greedyQuantite(field string) bool { return field == "quantite" }

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
		{"{}", 1, "empty placeholder; write a field name between the braces, as in {nom}"},
		{"{Prix}", 1, `write {prix}, not "{Prix}"`},
		{"  {Prix}", 3, `write {prix}, not "{Prix}"`},
		{"{prix unitaire}", 1, `write {prix-unitaire}, not "{prix unitaire}"`},
		{"a {123}", 3, `"{123}" is not a field name: use lowercase ASCII letters, digits and hyphens, starting with a letter`},
		{"{nom", 1, `unterminated placeholder; close it with "}"`},
		{"{a {b}", 1, `unterminated placeholder; close it with "}"`},
		{"a } b", 3, `unmatched "}"; write \} for a literal brace`},
		{"a ] b", 3, `unmatched "]"; write \] for a literal bracket`},
		{"[de {nom}", 1, `unclosed optional group; close it with "]"`},
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
		{"{quantite} [de] {nom}", []string{"quantite", "nom"}},
		{"  {nom}  ", []string{"nom"}},
		{"[{a} x] {b} [y [{c}]]", []string{"a", "b", "c"}},
		{"{prix-unitaire} EUR", []string{"prix-unitaire"}},
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
	runMatches(t, "{quantite} [de] {nom}", ingredient, greedyQuantite, []matchTest{
		{"250 g de farine", map[string]string{"quantite": "250 g", "nom": "farine"}},
		{"4 œufs", map[string]string{"quantite": "4", "nom": "œufs"}},
		{"50 cl de lait", map[string]string{"quantite": "50 cl", "nom": "lait"}},
		{"50 g de beurre fondu", map[string]string{"quantite": "50 g", "nom": "beurre fondu"}},
		{"1 pincee de sel", map[string]string{"quantite": "1 pincee", "nom": "sel"}},
		{"1 250 g de farine", map[string]string{"quantite": "1 250 g", "nom": "farine"}},
		{"  250 g  de\tfarine  ", map[string]string{"quantite": "250 g", "nom": "farine"}},
		{"de farine", nil},
		{"farine", nil},
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
	runMatches(t, "Total: {montant} EUR", anything, lazy, []matchTest{
		{"Total: 12.50 EUR", map[string]string{"montant": "12.50"}},
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
	runMatches(t, "[le] {nom}", anything, lazy, []matchTest{
		{"le chat", map[string]string{"nom": "chat"}},
		{"chat", map[string]string{"nom": "chat"}},
	})
	runMatches(t, "{n} item[s] left", anything, lazy, []matchTest{
		{"1 item left", map[string]string{"n": "1"}},
		{"2 items left", map[string]string{"n": "2"}},
		{"2 itemleft", nil},
	})
	runMatches(t, "{nom} [({note})]", anything, lazy, []matchTest{
		{"farine", map[string]string{"nom": "farine"}},
		{"farine (bio)", map[string]string{"nom": "farine", "note": "bio"}},
		{"farine ()", map[string]string{"nom": "farine ()"}},
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
	runMatches(t, "{n} [de [la]] {nom}", anything, lazy, []matchTest{
		{"2 de la farine", map[string]string{"n": "2", "nom": "farine"}},
		{"2 de farine", map[string]string{"n": "2", "nom": "farine"}},
		{"2 farine", map[string]string{"n": "2", "nom": "farine"}},
		{"2 la farine", map[string]string{"n": "2", "nom": "la farine"}},
	})
	runMatches(t, "{n} [[tres] {adj}] {nom}", anything, lazy, []matchTest{
		{"2 tres bonnes pommes", map[string]string{"n": "2", "adj": "bonnes", "nom": "pommes"}},
		{"2 bonnes pommes", map[string]string{"n": "2", "adj": "bonnes", "nom": "pommes"}},
	})
	runMatches(t, "{n} [[tres] {adj}] {nom}", anything, greedy, []matchTest{
		{"2 tres bonnes pommes", map[string]string{"n": "2 tres bonnes", "nom": "pommes"}},
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
