// Package schema reads .kanonarch files: units, enums, schemas, functions and rules.
package schema

import (
	"slices"

	"github.com/func-it/kanon/internal/diag"
	"github.com/func-it/kanon/internal/pattern"
	"github.com/func-it/kanon/internal/value"
)

// File is a parsed .kanonarch file.
type File struct {
	Path    string
	Units   []*Unit
	Enums   []*Enum
	Schemas []*Schema
	Fns     []*Fn
}

// Unit is a unit a Quantity may carry, beyond the built-in ones: unit pincee.
type Unit struct {
	Pos  diag.Pos
	Name string
}

// Enum is a closed set of names: enum Etat { neuf, bon, use }.
type Enum struct {
	Pos    diag.Pos
	Name   string
	Values []*EnumValue
}

// EnumValue is one name of an Enum.
type EnumValue struct {
	Pos  diag.Pos
	Name string
}

// Schema describes an object: its fields and the patterns a list item is read through.
type Schema struct {
	Pos      diag.Pos
	Name     string
	Reads    []*Reads
	Patterns []*pattern.Pattern // one per Reads, set by Check
	Fields   []*Field
}

// Reads is a sentence that reads or renders a schema, a function or a rule: reads fr "{a} est inférieur à {b}".
type Reads struct {
	Pos  diag.Pos // the string
	Lang string   // "" when none is written
	Text string
}

// Field is a field of a schema. A computed field has an Expr, an inverse field an Inverse; neither is written in
// documents.
type Field struct {
	Pos        diag.Pos
	Name       string
	Type       *Type
	Inverse    string // orders: [Order] <- employee: the field of Type that links here, or ""
	InversePos diag.Pos
	Annots     []*Annot
	Expr       Expr
}

// Annot is an annotation on a field: @heading, @body, @sections.
type Annot struct {
	Pos  diag.Pos
	Name string
}

// Type is a written type: Text, Partie, [Ligne], Duration?, -> Partie, [-> Territoire].
type Type struct {
	Pos      diag.Pos
	Name     string
	Array    bool
	Optional bool
	Link     bool // -> Name: the objects are written elsewhere and linked to

	// Set by Check: what Name stands for.
	Native bool
	Kind   value.Kind
	Schema *Schema
	Enum   *Enum

	value string // in an expression, an enum value written by name: it fits every enum that has it
}

// Fn is a pure function, or a rule when Rule is set.
type Fn struct {
	Pos    diag.Pos
	Rule   bool
	Name   string
	Params []*Param
	Result *Type // nil for a rule written without one: it is Bool
	Reads  []*Reads
	Body   Expr
}

// Param is a parameter of a function; Default is nil when it has none.
type Param struct {
	Pos     diag.Pos
	Name    string
	Type    *Type
	Default Expr
}

// Expr is an expression of a computed field, a function or a rule.
type Expr interface{ At() diag.Pos }

type node struct{ Pos diag.Pos }

func (n node) At() diag.Pos { return n.Pos }

type (
	// Ident is a name: a field, a parameter, an enum value, a function or a builtin.
	Ident struct {
		node
		Name string
	}
	// Number is a number without a unit: 0.01, 595.
	Number struct {
		node
		Text string
	}
	// Quantity is a number and its unit: 2 years, 0 min, 20 %, or an amount: 1 EUR (Money).
	Quantity struct {
		node
		Num     string
		Unit    string
		UnitPos diag.Pos
	}
	// String is a quoted string.
	String struct {
		node
		Value string
	}
	// Unary is -x or not x.
	Unary struct {
		node
		Op string
		X  Expr
	}
	// Binary is x op y.
	Binary struct {
		node
		Op   string
		X, Y Expr
	}
	// Member is x.name, or x?.name when Optional.
	Member struct {
		node
		X        Expr
		Name     string
		Optional bool
	}
	// Call is fn(args); Result is what it gives, set by Check, or nil when that isn't known.
	Call struct {
		node
		Fn     Expr
		Args   []Expr
		Result *Type
	}
	// Lambda is x => body, or (a, b) => body.
	Lambda struct {
		node
		Params []*Ident
		Body   Expr
	}
	// With is x with { name: value, … }: a copy of x with some fields changed.
	With struct {
		node
		X      Expr
		Fields []*WithField
	}
	WithField struct {
		Pos   diag.Pos
		Name  string
		Value Expr
	}
	// If is if cond then x else y.
	If struct {
		node
		Cond, Then, Else Expr
	}
)

// Schema returns the schema called name, or nil.
func (f *File) Schema(name string) *Schema {
	for _, s := range f.Schemas {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// SchemaNames returns the names of the schemas.
func (f *File) SchemaNames() []string {
	var ns []string
	for _, s := range f.Schemas {
		ns = append(ns, s.Name)
	}
	return ns
}

// UnitNames returns the names of the declared units.
func (f *File) UnitNames() []string {
	var ns []string
	for _, u := range f.Units {
		ns = append(ns, u.Name)
	}
	return ns
}

// Money reports whether q is an amount in f, as in 1 EUR: its unit is a currency code that names no unit.
func (q *Quantity) Money(f *File) bool {
	return value.Currency.MatchString(q.Unit) && !slices.Contains(value.BuiltinUnits, q.Unit) &&
		!slices.Contains(f.UnitNames(), q.Unit)
}

// Declares reports whether name is declared in f: a unit, an enum, a schema, a function or a rule.
func (f *File) Declares(name string) bool {
	for _, n := range f.DeclNames() {
		if n == name {
			return true
		}
	}
	return false
}

// DeclNames returns every name declared in f.
func (f *File) DeclNames() []string {
	ns := f.UnitNames()
	for _, e := range f.Enums {
		ns = append(ns, e.Name)
	}
	ns = append(ns, f.SchemaNames()...)
	for _, fn := range f.Fns {
		ns = append(ns, fn.Name)
	}
	return ns
}

// Field returns the field called name, or nil.
func (s *Schema) Field(name string) *Field {
	for _, f := range s.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// Annotated returns the field marked @name, or nil.
func (s *Schema) Annotated(name string) *Field {
	for _, f := range s.Fields {
		if f.Has(name) {
			return f
		}
	}
	return nil
}

// FieldNames returns the names of the fields.
func (s *Schema) FieldNames() []string {
	var ns []string
	for _, f := range s.Fields {
		ns = append(ns, f.Name)
	}
	return ns
}

// Computed reports whether f is computed rather than written: from an expression, or as an inverse.
func (f *Field) Computed() bool { return f.Expr != nil || f.Inverse != "" }

// Required reports whether a document must write f.
func (f *Field) Required() bool { return !f.Type.Optional && !f.Computed() }

// Has reports whether f is marked @name.
func (f *Field) Has(name string) bool {
	for _, a := range f.Annots {
		if a.Name == name {
			return true
		}
	}
	return false
}

// Resolved reports whether Check found what t stands for.
func (t *Type) Resolved() bool { return t.Native || t.Schema != nil || t.Enum != nil }

func (t *Type) String() string {
	s := t.Name
	if t.Link {
		s = "-> " + s
	}
	if t.Array {
		s = "[" + s + "]"
	}
	if t.Optional {
		s += "?"
	}
	return s
}

// Value returns the value called name, or nil.
func (e *Enum) Value(name string) *EnumValue {
	for _, v := range e.Values {
		if v.Name == name {
			return v
		}
	}
	return nil
}

// ValueNames returns the names of the values.
func (e *Enum) ValueNames() []string {
	var ns []string
	for _, v := range e.Values {
		ns = append(ns, v.Name)
	}
	return ns
}
