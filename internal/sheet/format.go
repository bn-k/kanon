package sheet

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// kind is what the number format of a cell says its number stands for.
type kind int

const (
	plain   kind = iota
	percent      // a fraction shown times 100: 0.2 is 20 %
	money        // an amount in currency
	clock        // a date serial: days since 1900 (or 1904), the time of day as a fraction
	elapsed      // a duration in days: [h]:mm
)

// format is what Kanon keeps of a number format.
type format struct {
	kind      kind
	day, time bool   // a clock shows a date, a time of day, or both
	currency  string // the ISO code of money
	decimals  int    // the decimals the format shows
}

// builtinFormats are the number formats every workbook knows by their id, without declaring them.
var builtinFormats = map[int]string{
	1: "0", 2: "0.00", 3: "#,##0", 4: "#,##0.00", 9: "0%", 10: "0.00%", 11: "0.00E+00",
	12: "# ?/?", 13: "# ??/??", 14: "mm-dd-yy", 15: "d-mmm-yy", 16: "d-mmm", 17: "mmm-yy",
	18: "h:mm AM/PM", 19: "h:mm:ss AM/PM", 20: "h:mm", 21: "h:mm:ss", 22: "m/d/yy h:mm",
	37: "#,##0 ;(#,##0)", 38: "#,##0 ;[Red](#,##0)", 39: "#,##0.00;(#,##0.00)", 40: "#,##0.00;[Red](#,##0.00)",
	45: "mm:ss", 46: "[h]:mm:ss", 47: "mmss.0", 48: "##0.0E+0", 49: "@",
}

// numberFormat returns the format of id, declared by the workbook in custom or built in.
func numberFormat(id int, custom map[int]string) format {
	if code, ok := custom[id]; ok {
		return parseFormat(code)
	}
	return parseFormat(builtinFormats[id])
}

// currencySigns maps the signs a format shows to their ISO code.
var currencySigns = map[string]string{"€": "EUR", "$": "USD", "£": "GBP", "¥": "JPY"}

// parseFormat reads the first section of a format code, the one for positive numbers: 0.00, dd/mm/yyyy,
// #,##0.00\ [$€-40C].
func parseFormat(code string) format {
	var f format
	var day, month, clockPart, pct, point bool
	rs := []rune(code)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		rest := strings.ToLower(string(rs[i:]))
		switch {
		case r == ';':
			i = len(rs)
		case r == '"':
			j := i + 1
			for j < len(rs) && rs[j] != '"' {
				j++
			}
			f.literal(string(rs[i+1 : min(j, len(rs))]))
			i = j
		case r == '\\':
			if i+1 < len(rs) {
				f.literal(string(rs[i+1]))
			}
			i++
		case r == '_' || r == '*': // padding the width of the next character, or filling with it
			i++
		case r == '[':
			j := i + 1
			for j < len(rs) && rs[j] != ']' {
				j++
			}
			in := string(rs[i+1 : min(j, len(rs))])
			if strings.HasPrefix(in, "$") {
				f.currencyCode(in[1:])
			} else if isElapsed(in) {
				f.kind = elapsed
			}
			i = j
		case strings.HasPrefix(rest, "general"):
			i += len("general") - 1
		case strings.HasPrefix(rest, "am/pm"):
			clockPart = true
			i += len("am/pm") - 1
		case strings.HasPrefix(rest, "a/p"):
			clockPart = true
			i += len("a/p") - 1
		case (r == 'E' || r == 'e') && i+1 < len(rs) && (rs[i+1] == '+' || rs[i+1] == '-'):
			point = false // the digits of the exponent are no decimals
			i++
		case r == 'y' || r == 'Y' || r == 'd' || r == 'D' || r == 'e':
			day = true
		case r == 'm' || r == 'M':
			month = true
		case r == 'h' || r == 'H' || r == 's' || r == 'S':
			clockPart = true
		case r == '%':
			pct = true
		case r == '.':
			point = true
		case point && (r == '0' || r == '#' || r == '?'):
			f.decimals++
		default:
			if c, ok := currencySigns[string(r)]; ok && f.currency == "" {
				f.currency = c
			}
		}
	}
	switch {
	case f.kind == elapsed:
		return format{kind: elapsed}
	case day || month || clockPart:
		return format{kind: clock, day: day || month && !clockPart, time: clockPart}
	case f.currency != "":
		f.kind = money
	case pct:
		f.kind = percent
	}
	return f
}

// literal notes a currency shown as text in a format: "€", \€ or "EUR".
func (f *format) literal(s string) {
	s = strings.TrimSpace(s)
	if f.currency != "" {
		return
	}
	if c, ok := currencySigns[s]; ok {
		f.currency = c
	} else if isCode(s) {
		f.currency = s
	}
}

// currencyCode reads a currency written in brackets, without its $: [$EUR], [$€-40C], [$$-409]. A locale alone,
// [$-40C], holds no currency.
func (f *format) currencyCode(s string) {
	if sign, _, _ := strings.Cut(s, "-"); sign != "" {
		f.literal(sign)
	}
}

func isCode(s string) bool {
	if len(s) != 3 {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// isElapsed tells [h], [mm] and [ss], the parts of a duration, from colors and conditions.
func isElapsed(s string) bool {
	s = strings.ToLower(s)
	return s != "" && strings.Trim(s, string(s[0])) == "" && strings.ContainsAny(s[:1], "hms")
}

// numberText writes the number f of a cell in the format fm as Kanon's canonical text. It returns false when the
// number is not a date or duration its format says it is.
func numberText(f float64, fm format, date1904 bool) (string, bool) {
	switch fm.kind {
	case percent:
		return decimal(f, 2) + " %", true
	case money:
		return moneyText(f, fm), true
	case clock:
		return clockText(f, fm, date1904)
	case elapsed:
		return duration(f)
	}
	return decimal(f, 0), true
}

// decimal writes f times 10^shift as a plain decimal, with no exponent and no float noise: like Excel, it keeps 15
// significant digits.
func decimal(f float64, shift int) string {
	if f == 0 {
		return "0"
	}
	mant, exp, _ := strings.Cut(strconv.FormatFloat(math.Abs(f), 'e', 14, 64), "e")
	digits := strings.TrimRight(strings.Replace(mant, ".", "", 1), "0")
	e, _ := strconv.Atoi(exp)
	point := e + 1 + shift // the digits before the point
	var s string
	switch {
	case point <= 0:
		s = "0." + strings.Repeat("0", -point) + digits
	case point >= len(digits):
		s = digits + strings.Repeat("0", point-len(digits))
	default:
		s = digits[:point] + "." + digits[point:]
	}
	if f < 0 {
		s = "-" + s
	}
	return s
}

// moneyText writes an amount with the decimals its format shows, or more when the amount has more.
func moneyText(f float64, fm format) string {
	s := decimal(f, 0)
	whole, frac, _ := strings.Cut(s, ".")
	if len(frac) < fm.decimals {
		frac += strings.Repeat("0", fm.decimals-len(frac))
	}
	if frac != "" {
		whole += "." + frac
	}
	return whole + " " + fm.currency
}

// serialTime turns a date serial into a time, to the nearest second. The 1900 system counts the 29 February 1900
// that never was, as Lotus 1-2-3 did: serial 60.
func serialTime(f float64, date1904 bool) (time.Time, bool) {
	secs := int64(math.Round(f * 86400))
	days := secs / 86400
	var base time.Time
	switch {
	case secs < 0 || days == 60 && !date1904:
		return time.Time{}, false
	case date1904:
		base = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
	case days < 60:
		base = time.Date(1899, 12, 31, 0, 0, 0, 0, time.UTC)
	default:
		base = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	}
	t := base.AddDate(0, 0, int(days)).Add(time.Duration(secs%86400) * time.Second)
	return t, t.Year() <= 9999
}

func clockText(f float64, fm format, date1904 bool) (string, bool) {
	t, ok := serialTime(f, date1904)
	if !ok {
		return "", false
	}
	switch {
	case !fm.time:
		return t.Format(time.DateOnly), true
	case !fm.day && f < 1:
		return timeOfDay(t), true
	}
	return t.Format(time.DateOnly) + " " + timeOfDay(t), true
}

// timeOfDay writes 14:30, or 14:30:05 when there are seconds.
func timeOfDay(t time.Time) string {
	if t.Second() != 0 {
		return t.Format(time.TimeOnly)
	}
	return t.Format("15:04")
}

// duration writes a number of days as hours and minutes, Kanon's smallest unit of time: 26 h 30 min.
func duration(f float64) (string, bool) {
	secs := int64(math.Round(f * 86400))
	if secs < 0 {
		return "", false
	}
	var terms []string
	if h := secs / 3600; h > 0 {
		terms = append(terms, fmt.Sprintf("%d h", h))
	}
	if m := secs % 3600; m > 0 || len(terms) == 0 {
		terms = append(terms, decimal(float64(m)/60, 0)+" min")
	}
	return strings.Join(terms, " "), true
}

// isoText writes a date of an ISO 8601 cell (t="d") as Kanon's canonical text.
func isoText(s string) (string, bool) {
	for _, layout := range []string{"2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05", time.DateOnly} {
		if t, err := time.Parse(layout, s); err == nil {
			if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 {
				return t.Format(time.DateOnly), true
			}
			return t.Format(time.DateOnly) + " " + timeOfDay(t), true
		}
	}
	return "", false
}
