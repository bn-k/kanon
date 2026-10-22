package schema

import (
	"slices"

	"github.com/bn-k/kanon/internal/value"
)

// Methods are the methods of an array: xs.filter(x => …).
var Methods = []string{"filter", "map", "max-by"}

// Arith returns the kind of x op y, for the operators + - * /, and whether op applies to x and y. A percentage
// scales what it multiplies: 7 920.00 EUR * 20 % is 1 584.00 EUR.
func Arith(op string, x, y value.Kind) (value.Kind, bool) {
	switch op {
	case "+", "-":
		switch {
		case isNum(x) && isNum(y):
			return max(x, y), true
		case isScaled(x) && x == y:
			return x, true
		case isTime(x) && y == value.Duration:
			return x, true
		case op == "+" && x == value.Duration && isTime(y):
			return y, true
		case op == "-" && isTime(x) && x == y:
			return value.Duration, true
		}
	case "*":
		switch {
		case x == value.Percent && y == value.Percent:
		case y == value.Percent:
			return percentOf(x)
		case x == value.Percent:
			return percentOf(y)
		case isNum(x) && isNum(y):
			return max(x, y), true
		case isScaled(x) && isNum(y):
			return x, true
		case isNum(x) && isScaled(y):
			return y, true
		}
	case "/":
		switch {
		case isNum(x) && isNum(y), isScaled(x) && x == y:
			return value.Number, true
		case isScaled(x) && isNum(y):
			return x, true
		}
	}
	return 0, false
}

// percentOf returns the kind of a percentage of a value of kind k.
func percentOf(k value.Kind) (value.Kind, bool) {
	switch {
	case isNum(k):
		return value.Number, true
	case isScaled(k):
		return k, true
	}
	return 0, false
}

// Ordered reports whether a value of kind x compares with one of kind y by < and >. A version compares with a
// number: 580.95 < 595.
func Ordered(x, y value.Kind) bool {
	switch {
	case isNum(x) && isNum(y):
		return true
	case x == value.Version:
		return y == value.Version || isNum(y)
	case y == value.Version:
		return isNum(x)
	}
	return x == y && (isScaled(x) || x == value.Date || x == value.DateTime || x == value.Text)
}

func isNum(k value.Kind) bool { return k == value.Int || k == value.Number }

// isScaled reports whether k is a number with a unit, which adds only to its own kind.
func isScaled(k value.Kind) bool {
	return k == value.Money || k == value.Quantity || k == value.Duration || k == value.Percent
}

func isTime(k value.Kind) bool { return k == value.Date || k == value.DateTime }

// noneType is the type of none, which fits every optional type.
var noneType = &Type{Name: "none"}

func nativeType(k value.Kind) *Type { return &Type{Name: k.String(), Native: true, Kind: k} }

// typeOf returns the type of the values of the written type t in an expression, where a link stands for the object
// it links to; or nil when t isn't resolved.
func typeOf(t *Type) *Type {
	if t == nil || !t.Resolved() {
		return nil
	}
	v := *t
	v.Link = false
	return &v
}

// is reports whether t is a single value of one of kinds, none aside.
func (t *Type) is(kinds ...value.Kind) bool {
	return t != nil && t.Native && !t.Array && slices.Contains(kinds, t.Kind)
}

// maybe reports whether a value of type t may be none.
func maybe(t *Type) bool { return t == noneType || t != nil && t.Optional }

// opt returns t, optional or not.
func opt(t *Type, optional bool) *Type {
	if t == nil || t == noneType || t.Optional == optional {
		return t
	}
	o := *t
	o.Optional = optional
	return &o
}

// elem returns the type of the items of t, or t itself when it isn't an array.
func elem(t *Type) *Type {
	e := *t
	e.Array, e.Optional = false, false
	return &e
}

// arrayOf returns the type of an array of the values of t; an array of arrays is flattened, and none is left out.
func arrayOf(t *Type) *Type {
	a := *t
	a.Array, a.Optional = true, false
	return &a
}

// join returns the type that holds both the values of a and those of b, none and optionality aside: Int and Number
// join as Number.
func join(a, b *Type) (*Type, bool) {
	switch {
	case a.Array != b.Array:
	case a.Native && b.Native:
		if a.Kind == b.Kind || isNum(a.Kind) && isNum(b.Kind) {
			return nativeType(max(a.Kind, b.Kind)), true
		}
	case a.Schema != nil:
		return a, a.Schema == b.Schema
	case a.Enum != nil && b.Enum != nil:
		switch {
		case a.Enum == b.Enum && a.value != "":
			return b, true
		case a.Enum == b.Enum, b.value != "" && a.Enum.Value(b.value) != nil:
			return a, true
		case a.value != "" && b.Enum.Value(a.value) != nil:
			return b, true
		}
	}
	return nil, false
}

// fits reports whether a value of type got may stand where one of type want is expected. An unknown type fits.
func fits(want, got *Type) bool {
	switch {
	case want == nil || got == nil:
		return true
	case got == noneType:
		return want.Optional
	case got.Optional && !want.Optional, got.Array != want.Array:
		return false
	case want.Native:
		return got.Native && (got.Kind == want.Kind || want.Kind == value.Number && got.Kind == value.Int)
	case want.Schema != nil:
		return got.Schema == want.Schema
	}
	return got.Enum == want.Enum || got.value != "" && want.Enum.Value(got.value) != nil
}

// mismatch returns "" when got fits want, or else the end of a message that says what got is.
func mismatch(want, got *Type) string {
	if fits(want, got) {
		return ""
	}
	s := "`" + got.String() + "`"
	if got != noneType && fits(want, opt(got, false)) {
		s += "; add ?? and a value for none"
	}
	return s
}
