package schema

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/pattern"
	"github.com/bn-k/kanon/internal/value"
)

// check parses src, which must be valid, and checks it.
func check(t *testing.T, src string) (*File, []string) {
	t.Helper()
	f, diags := Parse("t.kanonarch", src)
	if len(diags) > 0 {
		t.Fatalf("Parse: %v", diags)
	}
	return f, diagStrings(Check(f))
}

func TestCheckOK(t *testing.T) {
	tests := []string{
		"unit pincee\nfn f() -> Quantity = 2 pincee",
		"unit tasse\nunit kWatt",
		"fn f() -> Quantity = 2 kg + 300 g * 3\nfn g() -> Duration = 3 years + 0 min\nfn h() -> Percent = 20 %",
		"enum E { aa, bb }\nrule r(x: E) -> Bool = x == aa or x == bb",
		"rule r() = today == today and yes or no",
		"rule r(x: Int) -> Bool = x == none",
		"fn g(a: Int, b: Int = 1) -> Int = a\nfn f() -> Int = g(1) + g(1, 2)",
		"fn g() -> Int = 1\nfn f(v: [Int]) -> [Int] = v.map(g => g + 1)",
		"fn f(a: Int, b: Int = a) -> Int = b",
		"schema R { a: Int }\nfn f(r: R) -> R = r with { a: 1 }",
		"schema V { a: [V] }\nfn f(v: V) -> [V] = v.a.a",
		"schema S { a: Int  b: Text = id  c: Int = a + 1 }",
		"schema S { a: Int  xs: [Int]  b: Int = sum(xs) + count(xs) + max(a, a) + min(a, a) + abs(a) + round(a, 0.01) }",
		"schema S { t: Text @heading  c: Prose? @body  s: [S] @sections }",
		"schema S { b: Int  a: Int = if b > 1 then -b else (if not yes then 1 else 2) }",
		`fn f(v: Int) -> Int reads fr "{v.nom} et {v}" reads "{ v }" = v`,
		"enum E { a }\nschema S { e: E  l: [E]?  d: Date }",
		"schema S { a: DateTime  f: File?  i: [Image]  c: Code }",
		"schema E { chef: -> E?  equipe: [E] <- chef  zones: [-> Z] }\nschema Z { e: [E] <- zones }",
		"schema P { bureau: -> B? }\nschema B { p: P? <- bureau }",
		"schema O { e: -> E }\nschema E { o: [O] <- e  o2: [O]? <- e }\nfn f(o: -> O) -> -> E = o.e",
	}
	for _, src := range tests {
		t.Run(src, func(t *testing.T) {
			if _, diags := check(t, src); len(diags) > 0 {
				t.Errorf("got %q", diags)
			}
		})
	}
}

func TestCheckResolves(t *testing.T) {
	f, diags := check(t, "enum Etat { neuf }\nschema Clause { t: Text }\nschema Bail { clauses: [Clause]  etat: Etat?  debut: Date }\nfn f(b: Bail) -> Bool = yes")
	if len(diags) > 0 {
		t.Fatalf("Check: %v", diags)
	}
	bail := f.Schema("Bail")
	clauses, etat, debut := bail.Fields[0].Type, bail.Fields[1].Type, bail.Fields[2].Type
	if clauses.Schema != f.Schema("Clause") || !clauses.Array || clauses.Native {
		t.Errorf("clauses: %+v", clauses)
	}
	if etat.Enum != f.Enums[0] || !etat.Optional {
		t.Errorf("etat: %+v", etat)
	}
	if !debut.Native || debut.Kind != value.Date {
		t.Errorf("debut: %+v", debut)
	}
	fn := f.Fns[0]
	if fn.Params[0].Type.Schema != bail || fn.Result.Kind != value.Bool || !fn.Result.Native {
		t.Errorf("fn types: %+v %+v", fn.Params[0].Type, fn.Result)
	}
}

func TestCheckRelations(t *testing.T) {
	f, diags := check(t, "schema Facture { client: -> Partie }\nschema Partie { factures: [Facture] <- client }")
	if len(diags) > 0 {
		t.Fatalf("Check: %v", diags)
	}
	client, factures := f.Schema("Facture").Fields[0], f.Schema("Partie").Fields[0]
	if !client.Type.Link || client.Type.Schema != f.Schema("Partie") || !client.Required() || client.Computed() {
		t.Errorf("client: %+v", client.Type)
	}
	if factures.Type.Link || factures.Type.Schema != f.Schema("Facture") || factures.Required() || !factures.Computed() {
		t.Errorf("factures: %+v", factures.Type)
	}
}

func TestCheckErrors(t *testing.T) {
	tests := []struct {
		src  string
		want []string
	}{
		// Declarations.
		{"schema A {}\nenum A { x }", []string{"2:6: `A` is already declared on line 1"}},
		{"unit pincee\nfn pincee() -> Int = 1", []string{"2:4: `pincee` is already declared on line 1"}},
		{"schema Text {}", []string{"1:8: `Text` is a native type; choose another name"}},
		{"schema etat-des-lieux {}", []string{`1:8: type names are PascalCase: write EtatDesLieux, not "etat-des-lieux"`}},
		{"enum Piece_x { a }", []string{`1:6: type names are PascalCase: write PieceX, not "Piece_x"`}},
		{"schema _ {}", []string{`1:8: "_" is not a type name: use ASCII PascalCase, such as EtatDesLieux`}},
		{"unit g", []string{"1:6: `g` is a built-in unit"}},
		{"unit days", []string{"1:6: `days` is a duration unit"}},
		{"unit pince_e", []string{`1:6: write pincee, not "pince_e"`}},
		{"unit _", []string{`1:6: "_" is not a unit name: use ASCII letters and digits, starting with a letter`}},
		{"enum E { a, a }", []string{"1:13: `a` is already a value of `E`"}},
		{"enum E { Bon }", []string{`1:10: write bon, not "Bon"`}},
		{"enum E { _ }", []string{`1:10: "_" is not a name: use lowercase ASCII letters, digits and hyphens, starting with a letter`}},

		// Fields.
		{"schema S { id: Text }", []string{"1:12: `id` is reserved: every object has one"}},
		{"schema S { a: Text a: Int }", []string{"1:20: `a` is already a field of `S`"}},
		{"schema S { prix_unitaire: Money }", []string{`1:12: write prix-unitaire, not "prix_unitaire"`}},

		// Types.
		{"schema S { a: Txt }", []string{"1:15: unknown type `Txt`; did you mean `Text`?"}},
		{"schema S { a: [Partie] }\nschema Partei {}", []string{"1:16: unknown type `Partie`; did you mean `Partei`?"}},
		{"schema S { a: Zorglub }", []string{"1:15: unknown type `Zorglub`"}},
		{"schema S { a: Datetime }", []string{"1:15: unknown type `Datetime`; did you mean `DateTime`?"}},

		// Relations.
		{"schema S { a: -> Text }", []string{`1:18: only a schema can be linked to: write Text, not "-> Text"`}},
		{"enum E { x }\nschema S { a: [-> E]? }", []string{`2:19: only a schema can be linked to: write [E]?, not "[-> E]?"`}},
		{"schema S { a: -> Txt }", []string{"1:18: unknown type `Txt`; did you mean `Text`?"}},
		{"schema S { a: [Text] <- b }", []string{"1:16: an inverse holds the objects that link here, and `Text` is not a schema"}},
		{"schema O { e: -> S }\nschema S { o: [-> O] <- e }", []string{`2:19: an inverse holds the objects that link here: write [O] <- e, not "[-> O] <- e"`}},
		{"schema O { e: -> S }\nschema S { o: [O] <- ee }", []string{"2:22: `O` has no field `ee`; did you mean `e`?"}},
		{"schema O { e: S }\nschema S { o: [O] <- e }", []string{"2:22: `O.e` is `S`; an inverse needs a link to `S` there: -> S, -> S? or [-> S]"}},
		{"schema O { e: -> O }\nschema S { o: [O] <- e }", []string{"2:22: `O.e` is `-> O`; an inverse needs a link to `S` there: -> S, -> S? or [-> S]"}},
		{"schema S { a: [Zz] <- b }", []string{"1:16: unknown type `Zz`"}},

		// Annotations.
		{"schema S { a: Int @heading }", []string{"1:19: `@heading` needs a `Text` field, not `Int`"}},
		{"schema S { a: [Text] @heading }", []string{"1:22: `@heading` needs a `Text` field, not `[Text]`"}},
		{"schema S { a: Text @body }", []string{"1:20: `@body` needs a `Prose` field, not `Text`"}},
		{"schema S { a: [Text] @sections }", []string{"1:22: `@sections` needs an array of a schema, not `[Text]`"}},
		{"schema S { a: S @sections }", []string{"1:17: `@sections` needs an array of a schema, not `S`"}},
		{"schema S { a: [-> S] @sections }", []string{"1:22: `@sections` needs an array of a schema, not `[-> S]`"}},
		{"schema S { a: Text @headng }", []string{"1:20: unknown annotation `@headng`; did you mean `@heading`?"}},
		{"schema S { a: Text @foo }", []string{"1:20: unknown annotation `@foo`"}},
		{"schema S { a: Text @heading @heading }", []string{"1:29: `a` is already marked `@heading`"}},
		{"schema S { a: Text @heading b: Text @heading }", []string{"1:37: `@heading` is already on `a`"}},
		{"schema S { a: Txt @heading }", []string{"1:15: unknown type `Txt`; did you mean `Text`?"}},

		// Functions and rules.
		{"rule r() -> Int = 1", []string{"1:13: a rule returns `Bool`, not `Int`"}},
		{"rule r() -> Bool? = yes", []string{"1:13: a rule returns `Bool`, not `Bool?`"}},
		{"fn f(a: Txt) -> Int = 1", []string{"1:9: unknown type `Txt`; did you mean `Text`?"}},
		{"fn f() -> Txt = 1", []string{"1:11: unknown type `Txt`; did you mean `Text`?"}},
		{"fn f(a: Int, a: Int) -> Int = a", []string{"1:14: `a` is already a parameter of `f`"}},
		{"fn f(a: Int = 1, b: Int) -> Int = a", []string{"1:18: `b` needs a default value, as it follows a parameter that has one"}},
		{"fn Foo() -> Int = 1", []string{`1:4: write foo, not "Foo"`}},
		{"rule a_b() = yes", []string{`1:6: write a-b, not "a_b"`}},
		{"fn f(prixHT: Int) -> Int = prixHT", []string{`1:6: write prixht, not "prixHT"`}},
		{`fn f(v: Int) -> Int reads "{w.nom}" = v`, []string{"1:28: unknown parameter `w`; did you mean `v`?"}},
		{`fn f(v: Int) -> Int reads "a \"{w}\"" = v`, []string{"1:32: unknown parameter `w`; did you mean `v`?"}},
		{`fn f(v: Int) -> Int reads "{v" = v`, []string{"1:28: missing `}` after `{`"}},
		{`fn f(v: Int) -> Int reads "{}" = v`, []string{"1:28: empty placeholder `{}`"}},

		// Expressions.
		{"schema S {\n  quantite: Int\n  b: Int = quantte\n}", []string{"3:12: unknown name `quantte`; did you mean `quantite`?"}},
		{"fn f(a: Int, b: Int = zz, zz: Int = 1) -> Int = a", []string{"1:23: unknown name `zz`"}},
		{"fn f(v: [Int]) -> Int = v.map(x => x + yy)", []string{"1:40: unknown name `yy`"}},
		{"enum Op { vidange }\nrule r(x: Op) -> Bool = x == vidang", []string{"2:30: unknown name `vidang`; did you mean `vidange`?"}},
		{"fn f() -> Int = -zz.a + (not zz) + g(zz) + zz with { a: zz } + (if zz then zz else zz) + (q => zz)", []string{
			"1:18: unknown name `zz`", "1:30: unknown name `zz`", "1:36: unknown name `g`; did you mean `f`?",
			"1:38: unknown name `zz`", "1:44: unknown name `zz`", "1:57: unknown name `zz`", "1:68: unknown name `zz`",
			"1:76: unknown name `zz`", "1:84: unknown name `zz`",
			"1:91: `=>` writes a function, which goes only as an argument, as in xs.map(x => …)", "1:96: unknown name `zz`",
		}},
		{"fn g(a: Int, b: Int = 1) -> Int = a\nfn f() -> Int = g()", []string{"2:17: `g` takes 1 or 2 arguments, not 0"}},
		{"fn g(a: Int) -> Int = a\nfn f() -> Int = g(1, 2)", []string{"2:17: `g` takes 1 argument, not 2"}},
		{"fn g(a: Int, b: Int) -> Int = a\nfn f() -> Int = g(1)", []string{"2:17: `g` takes 2 arguments, not 1"}},
		{"fn g() -> Int = 1\nfn f() -> Int = g(1)", []string{"2:17: `g` takes no arguments, not 1"}},
		{"fn g(a: Int, b: Int = 1, c: Int = 2) -> Int = a\nfn f() -> Int = g()", []string{"2:17: `g` takes 1 to 3 arguments, not 0"}},
		{"fn f() -> Quantity = 2 pincee", []string{"1:24: unknown unit `pincee`; a schema declares its own units, as in: unit pincee"}},
		{"fn f() -> Quantity = 2 kgg", []string{"1:24: unknown unit `kgg`; did you mean `kg`?"}},
		{"fn f(d: Date) -> Date = d + 30 jours", []string{"1:32: unknown unit `jours`; a schema declares its own units, as in: unit jours"}},
		{"rule r(m: Money) -> Bool = m > 1 EUR and m * 2 < 1000.50 USD", nil},
		{"rule r(m: Money) -> Bool = m > 1 250.50 EUR + 1", []string{"1:45: cannot add `Money` and `Int`"}},
		{"rule r(d: Quantity) -> Bool = d > 15 000 km", nil},
		{"rule r(m: Money) -> Bool = m > 1 25 EUR", []string{`1:32: write 125, not "1 25"`}},
		{"rule r(m: Money) -> Bool = m > 1 25 EUR + 1", []string{
			`1:32: write 125, not "1 25"`,
			"1:41: cannot add `Money` and `Int`",
		}},
		{"fn f() -> Number = 1234 567 + 12 0.01", []string{
			`1:20: write 1 234 567, not "1234 567"`,
			`1:31: write 120.01, not "12 0.01"`,
		}},
		{"fn f(q: Quantity) -> Quantity = q + 1 EUR", []string{"1:35: cannot add `Quantity` and `Money`"}},
		{"unit EUR\nfn f(q: Quantity) -> Quantity = q + 1 EUR", nil},

		// Several errors, sorted.
		{"schema S { b: Txt }\nschema S { a: Int @x }", []string{
			"1:15: unknown type `Txt`; did you mean `Text`?",
			"2:8: `S` is already declared on line 1",
			"2:19: unknown annotation `@x`",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			if _, got := check(t, tt.src); !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadsPos(t *testing.T) {
	r := &Reads{Pos: diag.Pos{File: "t", Line: 3, Col: 10}, Text: `a"b\c`}
	for i, want := range []int{11, 12, 14, 15, 17} {
		if got := readsPos(r, i); got.Col != want || got.Line != 3 || got.File != "t" {
			t.Errorf("readsPos(%d) = %v, want col %d", i, got, want)
		}
	}
}

// TestCheckPatterns needs the pattern package.
func TestCheckPatterns(t *testing.T) {
	tests := []struct {
		src  string
		want []string
	}{
		{`schema S reads "{a} {zz}" { a: Text b: Int? }`, []string{"1:21: unknown field `zz`"}},
		{`schema S reads "{xs}" { xs: [Text] }`, []string{"1:17: `xs` is an array: a pattern reads single values"}},
		{"schema P { a: Text }\nschema S reads \"{p}\" { p: P }", []string{"2:17: `p` is a `P`: a pattern reads values, not objects"}},
		{`schema S reads "{a}" { a: Text b: Text c: Text? d: Text = a }`, []string{"1:16: the pattern doesn't read `b`, which is required"}},
		{`schema S reads "{a} {d}" { a: Text d: Text = a }`, []string{"1:21: `d` is computed: a pattern reads written fields"}},
		{"schema P { s: -> S }\nschema S reads \"{p}\" { p: [P] <- s }", []string{"2:17: `p` is computed: a pattern reads written fields"}},
		{"schema P { a: Text }\nschema S reads \"{p}\" { p: -> P }", []string{"2:17: `p` links to a `P`: a pattern reads values, not links"}},
		{`schema S reads "{p}" { p: Image }`, []string{"1:17: `p` is an `Image`: a pattern reads values, not files"}},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			if _, got := check(t, tt.src); !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}

	f, diags := check(t, `schema I reads "{quantite} [de] {nom}" reads fr "{nom}: {quantite}" { quantite: Quantity nom: Text }`)
	if len(diags) > 0 || len(f.Schemas[0].Patterns) != 2 {
		t.Errorf("got %q and %d patterns, want none and 2", diags, len(f.Schemas[0].Patterns))
	}

	// A syntax error in a pattern is reported where it is, inside the string.
	text := `x \"{a`
	_, err := pattern.Parse(`x "{a`)
	var pe *pattern.Error
	if !errors.As(err, &pe) {
		t.Fatalf("pattern.Parse(%q) = %v, want a *pattern.Error", text, err)
	}
	_, got := check(t, `schema S reads "`+text+`" { a: Text }`)
	want := []string{diagStrings(diag.List{{Pos: readsPos(&Reads{Pos: diag.Pos{Line: 1, Col: 16}, Text: `x "{a`}, pe.Col-1), Msg: pe.Msg}})[0]}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLoadCases(t *testing.T) {
	paths, _ := filepath.Glob("../../cases/*/schema.kanonarch")
	if len(paths) == 0 {
		t.Fatal("no cases found")
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			f, diags := Load(path)
			if len(diags) > 0 {
				t.Errorf("got %q", diagStrings(diags))
			}
			if f == nil || len(f.Schemas) == 0 {
				t.Errorf("no schemas read")
			}
		})
	}
}

func TestHelpers(t *testing.T) {
	f, diags := Load("../../cases/lease/schema.kanonarch")
	if len(diags) > 0 {
		t.Fatalf("Load: %v", diags)
	}
	bail := f.Schema("Bail")
	etat := f.Enums[1]
	switch {
	case !f.Declares("Bail") || !f.Declares("depot-plafonne") || f.Declares("Nope"):
		t.Errorf("Declares: %v", f.DeclNames())
	case bail.Annotated("heading") != bail.Field("titre") || bail.Annotated("body") != nil:
		t.Errorf("Annotated")
	case !slices.Contains(bail.FieldNames(), "zone-tendue") || len(bail.FieldNames()) != len(bail.Fields):
		t.Errorf("FieldNames: %q", bail.FieldNames())
	case !bail.Field("clauses").Has("sections") || bail.Field("titre").Has("sections"):
		t.Errorf("Has")
	case etat.Value("use") == nil || etat.Value("use").Name != "use" || etat.Value("nope") != nil:
		t.Errorf("Value")
	case !slices.Contains(etat.ValueNames(), "use") || len(etat.ValueNames()) != len(etat.Values):
		t.Errorf("ValueNames: %q", etat.ValueNames())
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.kanonarch")
	f, diags := Load(missing)
	if f != nil || len(diags) != 1 || diags[0].Pos != (diag.Pos{File: missing}) || diags[0].Msg != "cannot read the file: no such file or directory" {
		t.Errorf("missing file: got %v, %v", f, diags)
	}

	// A file that doesn't parse isn't checked: Txt isn't reported.
	bad := filepath.Join(dir, "bad.kanonarch")
	if err := os.WriteFile(bad, []byte("schema S { a Text }\nschema T { b: Txt }"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, diags = Load(bad)
	if got, want := diagStrings(diags), []string{"1:14: expected `:`, found `Text`"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if f == nil || f.Schema("T") == nil || diags[0].Pos.File != bad {
		t.Errorf("bad file: got %v", f)
	}

	good := filepath.Join(dir, "good.kanonarch")
	if err := os.WriteFile(good, []byte("schema T { b: Txt }"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, diags = Load(good); len(diags) != 1 {
		t.Errorf("good file: got %v, want one error", diags)
	}
}
