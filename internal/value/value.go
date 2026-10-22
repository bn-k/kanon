// Package value reads the values written in Kanon documents, in their canonical English form only:
// 2026-09-01, 1 250.50 EUR, 20 %, 87 450 km, 3 years, yes.
package value

import (
	"errors"
	"fmt"
	"maps"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/func-it/kanon/internal/names"
)

// Kind is a native type of Kanon.
type Kind int

const (
	Text Kind = iota
	Prose
	Int
	Number
	Bool
	Date
	DateTime
	Duration
	Money
	Percent
	Quantity
	Version
	File // a link to a file; File, Image and Code are checked by the linter, not by Parse
	Image
	Code
)

var kindNames = [...]string{
	"Text", "Prose", "Int", "Number", "Bool", "Date", "DateTime", "Duration", "Money", "Percent", "Quantity", "Version",
	"File", "Image", "Code",
}

func (k Kind) String() string { return kindNames[k] }

// KindOf returns the native type called name.
func KindOf(name string) (Kind, bool) {
	for k, n := range kindNames {
		if n == name {
			return Kind(k), true
		}
	}
	return 0, false
}

// Value is a value read from a document. Amounts are exact decimals, never floats.
type Value struct {
	Kind    Kind
	Raw     string
	Num     *big.Rat  // Int, Number, Money, Percent, Quantity
	Unit    string    // the unit of a Quantity, the currency of Money
	Date    time.Time // a Date, or a DateTime; a DateTime without a zone is a floating local time, held in UTC
	Zone    string    // the zone of a DateTime as written: UTC, an offset such as +02:00, or "" for a local time
	Seconds bool      // a DateTime written with its seconds
	Bool    bool
	Terms   []Term // a Duration: 1 h 30 min has two terms
	Version []int
}

// Term is one amount of a Duration, in one of DurationUnits.
type Term struct {
	Num  *big.Rat
	Unit string
}

// BuiltinUnits are the units any Quantity may carry; a schema declares its own with `unit`.
var BuiltinUnits = []string{
	"mg", "g", "kg", "ml", "cl", "dl", "l", "mm", "cm", "m", "km", "m2", "ha", "m3",
	"B", "KB", "MB", "GB", "TB", "W", "kW", "kWh",
}

// DurationUnits maps each unit of a Duration, singular or plural, to its singular.
var DurationUnits = map[string]string{
	"min": "min", "h": "h",
	"day": "day", "days": "day", "week": "week", "weeks": "week",
	"month": "month", "months": "month", "year": "year", "years": "year",
}

// UnitName is the shape of a unit name: ASCII letters and digits, starting with a letter (m2, kWh, pincee).
var UnitName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

// Currency is the shape of a currency code: three uppercase ASCII letters (EUR).
var Currency = regexp.MustCompile(`^[A-Z]{3}$`)

// Units is the set of units a Quantity may carry.
type Units map[string]bool

// NewUnits returns the built-in units plus the declared ones.
func NewUnits(declared []string) Units {
	u := Units{}
	for _, n := range BuiltinUnits {
		u[n] = true
	}
	for _, n := range declared {
		u[n] = true
	}
	return u
}

// Names returns the units in sorted order.
func (u Units) Names() []string { return slices.Sorted(maps.Keys(u)) }

var (
	numberRe   = regexp.MustCompile(`^-?(?:\d{1,3}(?: \d{3})+|\d+)(?:\.\d+)?$`)
	intRe      = regexp.MustCompile(`^-?(?:\d{1,3}(?: \d{3})+|\d+)$`)
	plainRe    = regexp.MustCompile(`^-?\d+(?:\.\d+)?$`)
	dateRe     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	dateTimeRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}(?::\d{2})?)(?: (\S+))?$`)
	looseRe    = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2}(?::\d{2})?) ?(Z|UTC|[+-]\d{2}:\d{2})?$`)
	offsetRe   = regexp.MustCompile(`^[+-]\d{2}:\d{2}$`)
	versionRe  = regexp.MustCompile(`^\d+(?:\.\d+)*$`)
	spaces     = strings.NewReplacer(" ", " ", " ", " ")
)

// Parse reads s as a value of kind k. Its error tells what to write instead. A number, an amount, a percentage, a
// quantity or a duration reads non-breaking spaces as spaces; the other kinds keep their content as written.
func Parse(k Kind, s string, units Units) (Value, error) {
	switch k {
	case Int, Number, Money, Percent, Quantity, Duration:
		s = strings.TrimSpace(spaces.Replace(s))
	default:
		s = strings.Trim(s, " \t\r\n")
	}
	v := Value{Kind: k, Raw: s}
	if s == "" {
		return v, errors.New("no value")
	}
	switch k {
	case Int:
		if !intRe.MatchString(s) {
			fix := fixNumber(s)
			if strings.Contains(fix, ".") {
				fix = ""
			}
			return v, wrong("a whole number (4 or 1 250)", s, fix)
		}
		v.Num = rat(s)
	case Number:
		if !numberRe.MatchString(s) {
			return v, wrong("a number (12 or 1 250.50)", s, fixNumber(s))
		}
		v.Num = rat(s)
	case Bool:
		switch s {
		case "yes":
			v.Bool = true
		case "no":
		default:
			return v, wrong("yes or no", s, "")
		}
	case Date:
		t, err := time.Parse(time.DateOnly, s)
		if err != nil {
			if dateRe.MatchString(s) {
				return v, fmt.Errorf("%q is not a real date", s)
			}
			return v, wrong("a date as YYYY-MM-DD (2026-10-22)", s, "")
		}
		v.Date = t
	case DateTime:
		return parseDateTime(v)
	case Money:
		amount, code, _ := cutLast(s)
		if !numberRe.MatchString(amount) || !Currency.MatchString(code) {
			return v, wrong("an amount and a currency code (1 250.50 EUR)", s, fixMoney(s))
		}
		v.Num, v.Unit = rat(amount), code
	case Percent:
		amount, ok := strings.CutSuffix(s, " %")
		if !ok || !numberRe.MatchString(amount) {
			return v, wrong("a percentage (20 %)", s, fixPercent(s))
		}
		v.Num = rat(amount)
	case Quantity:
		return parseQuantity(v, units)
	case Duration:
		return parseDuration(v)
	case Version:
		if !versionRe.MatchString(s) {
			return v, wrong("a version, numbers separated by dots (580 or 1.2.3)", s, "")
		}
		for _, part := range strings.Split(s, ".") {
			n, _ := strconv.Atoi(part)
			v.Version = append(v.Version, n)
		}
	}
	return v, nil
}

// parseDateTime reads 2026-10-22 14:30, with seconds or not, then UTC, an offset such as +02:00, or no zone.
func parseDateTime(v Value) (Value, error) {
	m := dateTimeRe.FindStringSubmatch(v.Raw)
	if m == nil {
		return v, wrong("a date and time as YYYY-MM-DD HH:MM (2026-10-22 14:30)", v.Raw, fixDateTime(v.Raw))
	}
	layout := "2006-01-02 15:04"
	v.Seconds = len(m[1]) > len(layout)
	if v.Seconds {
		layout += ":05"
	}
	s, zone := m[1], m[2]
	switch {
	case offsetRe.MatchString(zone):
		layout += " -07:00"
		s += " " + zone
	case zone != "" && zone != "UTC":
		return v, fmt.Errorf("write a time zone as UTC or an offset such as +02:00, not %q", zone)
	}
	t, err := time.Parse(layout, s)
	if err != nil {
		return v, fmt.Errorf("%q is not a real date and time", v.Raw)
	}
	v.Date, v.Zone = t, zone
	return v, nil
}

// DateTimeText writes a DateTime back as it was written: 2026-10-22 14:30, or 2026-10-22 14:30:05 +02:00.
func (v Value) DateTimeText() string {
	layout := "2006-01-02 15:04"
	if v.Seconds {
		layout += ":05"
	}
	s := v.Date.Format(layout)
	if v.Zone != "" {
		s += " " + v.Zone
	}
	return s
}

// fixDateTime rewrites a date and time written the ISO way (2026-10-22T14:30:05Z, 14:30+02:00) in the canonical
// form, or returns "".
func fixDateTime(s string) string {
	m := looseRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	fix := m[1] + " " + m[2]
	switch m[3] {
	case "":
	case "Z":
		fix += " UTC"
	default:
		fix += " " + m[3]
	}
	return fix
}

func parseQuantity(v Value, units Units) (Value, error) {
	s := v.Raw
	if numberRe.MatchString(s) {
		v.Num = rat(s)
		return v, nil
	}
	amount, unit, ok := cutLast(s)
	if !ok || !numberRe.MatchString(amount) {
		return v, wrong("a number and its unit (48 m2 or 1 TB)", s, "")
	}
	if !units[unit] {
		if c := names.Closest(unit, units.Names()); c != "" {
			return v, fmt.Errorf("unknown unit %q; did you mean %s?", unit, c)
		}
		decl := unit
		if !UnitName.MatchString(decl) {
			decl = names.Slugify(decl)
		}
		return v, fmt.Errorf("unknown unit %q; a schema declares its own units, as in: unit %s", unit, decl)
	}
	v.Num, v.Unit = rat(amount), unit
	return v, nil
}

func parseDuration(v Value) (Value, error) {
	words := strings.Fields(v.Raw)
	for i := 0; i < len(words); {
		j := i
		for j < len(words) && startsNumber(words[j]) {
			j++
		}
		amount := strings.Join(words[i:j], " ")
		if j == i || j == len(words) || !numberRe.MatchString(amount) {
			return v, wrong("a duration, a number and its unit (15 min or 3 years)", v.Raw, "")
		}
		unit, ok := DurationUnits[words[j]]
		if !ok {
			return v, fmt.Errorf("unknown duration unit %q; use min, h, days, weeks, months or years", words[j])
		}
		v.Terms = append(v.Terms, Term{rat(amount), unit})
		i = j + 1
	}
	return v, nil
}

func startsNumber(w string) bool {
	return w != "" && (w[0] >= '0' && w[0] <= '9' || w[0] == '-')
}

// wrong is the error for a value that isn't in its canonical form; fix, when known, is that form.
func wrong(want, got, fix string) error {
	if fix != "" && fix != got {
		want = fix
	}
	return fmt.Errorf("write %s, not %q", want, got)
}

func cutLast(s string) (string, string, bool) {
	i := strings.LastIndexByte(s, ' ')
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+1:], true
}

func rat(s string) *big.Rat {
	r, _ := new(big.Rat).SetString(strings.ReplaceAll(s, " ", ""))
	return r
}

// fixNumber rewrites a number written another way (650,00, 1.250,50 or 1,250.50) in the canonical form, or
// returns "".
func fixNumber(s string) string {
	t := strings.ReplaceAll(s, " ", "")
	if i := strings.LastIndexByte(t, ','); i >= 0 {
		if strings.LastIndexByte(t, '.') > i {
			t = strings.ReplaceAll(t, ",", "")
		} else {
			t = strings.ReplaceAll(t[:i], ".", "") + "." + t[i+1:]
		}
	}
	if !plainRe.MatchString(t) {
		return ""
	}
	return group(t)
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

var currencySigns = []struct{ sign, code string }{{"€", "EUR"}, {"$", "USD"}, {"£", "GBP"}, {"¥", "JPY"}}

var currencyWords = map[string]string{"euro": "EUR", "euros": "EUR", "dollar": "USD", "dollars": "USD"}

// fixMoney rewrites an amount written with a currency sign or word (650,00 €, EUR 650) in the canonical form,
// or returns "".
func fixMoney(s string) string {
	code := ""
	for _, c := range currencySigns {
		if strings.Contains(s, c.sign) {
			code, s = c.code, strings.ReplaceAll(s, c.sign, " ")
			break
		}
	}
	words := strings.Fields(s)
	if code == "" && len(words) > 1 {
		for _, k := range []int{len(words) - 1, 0} {
			w := words[k]
			if c, ok := currencyWords[strings.ToLower(w)]; ok {
				code = c
			} else if Currency.MatchString(strings.ToUpper(w)) {
				code = strings.ToUpper(w)
			} else {
				continue
			}
			words = slices.Delete(words, k, k+1)
			break
		}
	}
	if code == "" {
		return ""
	}
	n := fixNumber(strings.Join(words, " "))
	if n == "" {
		return ""
	}
	return n + " " + code
}

// fixPercent rewrites a percentage written without its sign or space (20, 20%) in the canonical form, or
// returns "".
func fixPercent(s string) string {
	t := strings.TrimSpace(strings.TrimSuffix(s, "%"))
	if numberRe.MatchString(t) {
		return t + " %"
	}
	if n := fixNumber(t); n != "" {
		return n + " %"
	}
	return ""
}
