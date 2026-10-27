package eval

import (
	"cmp"
	"encoding/json"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bn-k/kanon/internal/schema"
	"github.com/bn-k/kanon/internal/value"
)

// arith returns x op y, for the operators + - * /, whose kinds the schema check has matched.
func arith(op string, x, y value.Value) value.Value {
	k, _ := schema.Arith(op, x.Kind, y.Kind)
	switch {
	case isTime(x.Kind) && isTime(y.Kind):
		return since(x, y).value()
	case isTime(x.Kind):
		return shift(x, durOf(y), op == "-")
	case isTime(y.Kind):
		return shift(y, durOf(x), false)
	case x.Kind == value.Duration || y.Kind == value.Duration:
		return durArith(op, x, y)
	case op == "*" && x.Kind == value.Percent:
		return value.Value{Kind: k, Num: mul(y.Num, factor(x)), Unit: y.Unit}
	case op == "*" && y.Kind == value.Percent:
		return value.Value{Kind: k, Num: mul(x.Num, factor(y)), Unit: x.Unit}
	}
	r := value.Value{Kind: k}
	switch op {
	case "+", "-":
		unit, n := unite(x, y)
		if op == "-" {
			n = neg(n)
		}
		r.Num, r.Unit = add(x.Num, n), unit
	case "*":
		r.Num, r.Unit = mul(x.Num, y.Num), cmp.Or(x.Unit, y.Unit)
	default:
		d := y.Num
		if x.Kind == y.Kind {
			_, d = unite(x, y)
		} else {
			r.Unit = x.Unit
		}
		r.Num = quo(x.Num, d)
	}
	return r
}

// factor is what a number or a percentage multiplies by: 20 % is 0.2.
func factor(v value.Value) *big.Rat {
	if v.Kind == value.Percent {
		return quo(v.Num, rint(100))
	}
	return v.Num
}

// conversion is the dimension of a unit, and how many of the dimension's base unit it is.
type conversion struct {
	dim    string
	factor *big.Rat
}

var conversions = map[string]conversion{}

func init() {
	for _, c := range []struct{ dim, units string }{
		{"mass", "mg 0.001 g 1 kg 1000"},
		{"volume", "ml 0.001 cl 0.01 dl 0.1 l 1 m3 1000"},
		{"length", "mm 0.001 cm 0.01 m 1 km 1000"},
		{"area", "m2 1 ha 10000"},
		{"data", "B 1 KB 1000 MB 1000000 GB 1000000000 TB 1000000000000"},
		{"power", "W 1 kW 1000"},
		{"energy", "kWh 1"},
	} {
		ws := strings.Fields(c.units)
		for i := 0; i < len(ws); i += 2 {
			conversions[ws[i]] = conversion{c.dim, rat(ws[i+1])}
		}
	}
}

// unite returns the unit of x + y, and the amount of y in it: amounts in one currency, a quantity converted to the
// unit of x. A value without a unit, as an empty sum, takes the other's.
func unite(x, y value.Value) (string, *big.Rat) {
	switch {
	case y.Unit == "" || x.Unit == y.Unit:
		return x.Unit, y.Num
	case x.Unit == "":
		return y.Unit, y.Num
	case x.Kind == value.Money:
		fail("%s and %s are in two currencies; write them in one", Format(x), Format(y))
	}
	cx, okx := conversions[x.Unit]
	cy, oky := conversions[y.Unit]
	if !okx || !oky || cx.dim != cy.dim {
		fail("%s doesn't convert to %s", Format(y), x.Unit)
	}
	return x.Unit, quo(mul(y.Num, cy.factor), cx.factor)
}

// dur is a Duration in months and minutes, which convert into each other only to compare: a month is the twelfth
// of a mean year.
type dur struct{ months, mins *big.Rat }

var (
	minutesIn     = map[string]int64{"min": 1, "h": 60, "day": 1440, "week": 10080}
	minutesAMonth = big.NewRat(438291, 10)
)

func durOf(v value.Value) dur {
	d := dur{new(big.Rat), new(big.Rat)}
	for _, t := range v.Terms {
		switch t.Unit {
		case "year":
			d.months = add(d.months, mul(t.Num, rint(12)))
		case "month":
			d.months = add(d.months, t.Num)
		default:
			d.mins = add(d.mins, mul(t.Num, rint(minutesIn[t.Unit])))
		}
	}
	return d
}

func (d dur) neg() dur { return dur{neg(d.months), neg(d.mins)} }

func (d dur) scale(f *big.Rat) dur { return dur{mul(d.months, f), mul(d.mins, f)} }

// minutes returns d in minutes, to compare.
func (d dur) minutes() *big.Rat { return add(d.mins, mul(d.months, minutesAMonth)) }

// parts returns d in years, months, days, h and min, the zero ones left out, and whether d is negative.
func (d dur) parts() ([]value.Term, bool) {
	negative := d.minutes().Sign() < 0
	if negative {
		d = d.neg()
	}
	var ts []value.Term
	part := func(n *big.Rat, unit string) {
		if n.Sign() != 0 {
			ts = append(ts, value.Term{Num: n, Unit: unit})
		}
	}
	if d.months.IsInt() {
		years := trunc(quo(d.months, rint(12)))
		part(years, "year")
		part(add(d.months, neg(mul(years, rint(12)))), "month")
	} else {
		part(d.months, "month")
	}
	days := trunc(quo(d.mins, rint(1440)))
	rest := add(d.mins, neg(mul(days, rint(1440))))
	hours := trunc(quo(rest, rint(60)))
	part(days, "day")
	part(hours, "h")
	part(add(rest, neg(mul(hours, rint(60)))), "min")
	return ts, negative
}

// value returns d as a Duration.
func (d dur) value() value.Value {
	ts, negative := d.parts()
	if negative {
		for i := range ts {
			ts[i].Num = neg(ts[i].Num)
		}
	}
	return value.Value{Kind: value.Duration, Terms: ts}
}

func (d dur) String() string {
	ts, negative := d.parts()
	if len(ts) == 0 {
		return "0 min"
	}
	var ws []string
	for _, t := range ts {
		unit := t.Unit
		if (unit == "year" || unit == "month" || unit == "day") && t.Num.Cmp(rint(1)) != 0 {
			unit += "s"
		}
		ws = append(ws, number(t.Num)+" "+unit)
	}
	s := strings.Join(ws, " ")
	if negative {
		s = "-" + s
	}
	return s
}

// iso writes d in ISO 8601: P1Y6M, PT1H45M, P2DT3H, PT0M when it is zero, a minus before it when it is negative.
func (d dur) iso() string {
	ts, negative := d.parts()
	var date, clock string
	for _, t := range ts {
		n := decimal(t.Num, 0)
		switch t.Unit {
		case "year":
			date += n + "Y"
		case "month":
			date += n + "M"
		case "day":
			date += n + "D"
		case "h":
			clock += n + "H"
		default:
			clock += n + "M"
		}
	}
	s := "P" + date
	if clock != "" || date == "" {
		s += "T" + cmp.Or(clock, "0M")
	}
	if negative {
		s = "-" + s
	}
	return s
}

// durArith returns x op y, where x or y is a Duration.
func durArith(op string, x, y value.Value) value.Value {
	switch {
	case op == "+" || op == "-":
		a, b := durOf(x), durOf(y)
		if op == "-" {
			b = b.neg()
		}
		return dur{add(a.months, b.months), add(a.mins, b.mins)}.value()
	case op == "/" && y.Kind == value.Duration:
		return value.Value{Kind: value.Number, Num: quo(durOf(x).minutes(), durOf(y).minutes())}
	}
	d, f := durOf(x), factor(y)
	if x.Kind != value.Duration {
		d, f = durOf(y), factor(x)
	}
	if op == "/" {
		f = quo(rint(1), f)
	}
	return d.scale(f).value()
}

// shift returns the date, or date and time, t moved by d, or back by d. A date moves by whole days and months,
// a date and time by whole months and any time; a month past the end of the next keeps to its last day.
func shift(t value.Value, d dur, back bool) value.Value {
	days := quo(d.mins, rint(1440))
	switch {
	case t.Kind == value.Date && !(d.months.IsInt() && days.IsInt()):
		fail("a date moves by whole days, not by %s", d)
	case !d.months.IsInt():
		fail("a date and time moves by whole months, not by %s", d)
	}
	if back {
		d, days = d.neg(), neg(days)
	}
	t.Date = addMonths(t.Date, int(d.months.Num().Int64()))
	if t.Kind == value.Date {
		t.Date = t.Date.AddDate(0, 0, int(days.Num().Int64()))
	} else {
		t.Date = t.Date.Add(time.Duration(trunc(mul(d.mins, rint(int64(time.Minute)))).Num().Int64()))
	}
	return t
}

// addMonths returns t n months later, on the last day of that month when it is shorter.
func addMonths(t time.Time, n int) time.Time {
	y, m, day := t.Date()
	first := time.Date(y, m+time.Month(n), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	return first.AddDate(0, 0, min(day, first.AddDate(0, 1, -1).Day())-1)
}

// since returns the time from y to x, dates or dates and times.
func since(x, y value.Value) dur {
	return dur{new(big.Rat), big.NewRat(int64(x.Date.Sub(y.Date)), int64(time.Minute))}
}

// round returns v rounded to a multiple of step, half away from zero.
func round(v value.Value, step *big.Rat) value.Value {
	q := quo(v.Num, step)
	half := big.NewRat(int64(q.Sign()), 2)
	v.Num = mul(trunc(add(q, half)), step)
	return v
}

func abs(v value.Value) value.Value {
	if v.Kind == value.Duration {
		if d := durOf(v); d.minutes().Sign() < 0 {
			return d.neg().value()
		}
		return v
	}
	v.Num = new(big.Rat).Abs(v.Num)
	return v
}

// zero returns the zero of kind k: an amount without a currency, a quantity without a unit.
func zero(k value.Kind) value.Value {
	if k == value.Duration {
		return dur{new(big.Rat), new(big.Rat)}.value()
	}
	return value.Value{Kind: k, Num: new(big.Rat)}
}

// compare returns -1, 0 or 1 as x is less than, equal to or more than y, whose kinds have an order. A date is held
// at 00:00 UTC and a date and time without a zone in UTC, so a date compares with a date and time as the start of
// its day, and as 00:00 UTC with one that has a zone.
func compare(x, y value.Value) int {
	switch {
	case x.Kind == value.Version || y.Kind == value.Version:
		a, b := version(x), version(y)
		for i := range max(len(a), len(b)) {
			if c := cmp.Compare(at(a, i), at(b, i)); c != 0 {
				return c
			}
		}
		return 0
	case x.Kind == value.Duration:
		return durOf(x).minutes().Cmp(durOf(y).minutes())
	case isTime(x.Kind):
		return x.Date.Compare(y.Date)
	case x.Num != nil && y.Num != nil:
		_, n := unite(x, y)
		return x.Num.Cmp(n)
	}
	return strings.Compare(x.Raw, y.Raw)
}

// version returns the numbers of a version, or of a number compared with one: 580.95 is 580 and 95.
func version(v value.Value) []int {
	if v.Kind == value.Version {
		return v.Version
	}
	var ns []int
	for _, p := range strings.Split(asVersion(v).Raw, ".") {
		n, _ := strconv.Atoi(p)
		ns = append(ns, n)
	}
	return ns
}

// asVersion returns a number compared with a version as written for a version, without thousands groups: 2 003 is
// 2003.
func asVersion(v value.Value) value.Value {
	return value.Value{Kind: value.Version, Raw: strings.ReplaceAll(Format(v), " ", "")}
}

func at(ns []int, i int) int {
	if i < len(ns) {
		return ns[i]
	}
	return 0
}

// equal reports whether x and y are the same value; none is equal to none only.
func equal(x, y val) bool {
	switch x := x.(type) {
	case nil:
		return y == nil
	case enumVal:
		y, ok := y.(enumVal)
		return ok && x.name == y.name
	case *obj:
		y, ok := y.(*obj)
		return ok && x.m == y.m
	case []val:
		y, ok := y.([]val)
		return ok && slices.EqualFunc(x, y, equal)
	}
	a, b := x.(value.Value)
	v, ok := y.(value.Value)
	switch {
	case !b || !ok:
		return false
	case a.Kind == value.Bool || v.Kind == value.Bool:
		return a.Kind == v.Kind && a.Bool == v.Bool
	}
	return compare(a, v) == 0
}

// safeEqual is equal for values that may not compare, as amounts in two currencies, which are then not equal.
func safeEqual(x, y val) (eq bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(evalErr); !ok {
				panic(r)
			}
			eq = false
		}
	}()
	return equal(x, y)
}

// Format writes v in its canonical form: 1 584.00 EUR, 2.3 kg, 1 h 45 min.
func Format(v value.Value) string {
	switch v.Kind {
	case value.Int, value.Number:
		return number(v.Num)
	case value.Money:
		return withUnit(money(v.Num), v.Unit)
	case value.Percent:
		return number(v.Num) + " %"
	case value.Quantity:
		return withUnit(number(v.Num), v.Unit)
	case value.Duration:
		return durOf(v).String()
	case value.Date:
		return v.Date.Format(time.DateOnly)
	case value.DateTime:
		return v.DateTimeText()
	case value.Bool:
		if v.Bool {
			return "yes"
		}
		return "no"
	}
	return v.Raw
}

func withUnit(s, unit string) string {
	if unit == "" {
		return s
	}
	return s + " " + unit
}

// number writes r with up to 6 decimals, its thousands apart: 1 250.5.
func number(r *big.Rat) string { return group(decimal(r, 0)) }

// money writes an amount with 2 to 6 decimals: 1 250.50.
func money(r *big.Rat) string { return group(decimal(r, 2)) }

// decimal writes r as a plain decimal, with at least least decimals and at most 6: 1250.5, or 1250.50 with 2.
func decimal(r *big.Rat, least int) string {
	whole, frac, _ := strings.Cut(r.FloatString(6), ".")
	frac = strings.TrimRight(frac, "0")
	frac += strings.Repeat("0", max(0, least-len(frac)))
	if whole == "-0" && strings.Trim(frac, "0") == "" {
		whole = "0"
	}
	if frac == "" {
		return whole
	}
	return whole + "." + frac
}

// JSON returns v as a program reads it: a number for Int, Number and Percent (20 for 20 %); for Money and Quantity,
// the amount as a plain decimal in a string, exact, with its currency or unit; a bool for Bool; ISO 8601 for a date,
// a date and time (without a zone for a local time) and a duration; the text of any other value.
func JSON(v value.Value) any {
	switch v.Kind {
	case value.Int, value.Number, value.Percent:
		return json.Number(decimal(v.Num, 0))
	case value.Money:
		return map[string]string{"amount": decimal(v.Num, 2), "currency": v.Unit}
	case value.Quantity:
		return map[string]string{"amount": decimal(v.Num, 0), "unit": v.Unit}
	case value.Bool:
		return v.Bool
	case value.Date:
		return v.Date.Format(time.DateOnly)
	case value.DateTime:
		if v.Zone == "" {
			return v.Date.Format("2006-01-02T15:04:05")
		}
		return v.Date.Format(time.RFC3339)
	case value.Duration:
		return durOf(v).iso()
	}
	return v.Raw
}

// group writes the thousands of a plain number apart: 1250.50 becomes 1 250.50.
func group(t string) string {
	var b strings.Builder
	if rest, ok := strings.CutPrefix(t, "-"); ok {
		b.WriteByte('-')
		t = rest
	}
	whole, frac, hasFrac := strings.Cut(t, ".")
	for i := range len(whole) {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteByte(whole[i])
	}
	if hasFrac {
		b.WriteString("." + frac)
	}
	return b.String()
}

// show writes v in a message: none, the name of an enum value, the id of an object, the items of an array.
func show(v val) string {
	switch v := v.(type) {
	case nil:
		return "none"
	case enumVal:
		return v.name
	case *obj:
		return v.m.ID
	case []val:
		var ws []string
		for _, x := range v {
			ws = append(ws, show(x))
		}
		return strings.Join(ws, ", ")
	}
	return Format(v.(value.Value))
}

func isTime(k value.Kind) bool { return k == value.Date || k == value.DateTime }

func rat(s string) *big.Rat {
	r, _ := new(big.Rat).SetString(strings.ReplaceAll(s, " ", ""))
	return r
}

func rint(n int64) *big.Rat { return new(big.Rat).SetInt64(n) }

func add(a, b *big.Rat) *big.Rat { return new(big.Rat).Add(a, b) }

func mul(a, b *big.Rat) *big.Rat { return new(big.Rat).Mul(a, b) }

func neg(a *big.Rat) *big.Rat { return new(big.Rat).Neg(a) }

// quo returns a / b; b is not zero.
func quo(a, b *big.Rat) *big.Rat {
	if b.Sign() == 0 {
		fail("a division by zero")
	}
	return new(big.Rat).Quo(a, b)
}

// trunc returns the whole part of r, toward zero.
func trunc(r *big.Rat) *big.Rat {
	return new(big.Rat).SetInt(new(big.Int).Quo(r.Num(), r.Denom()))
}
