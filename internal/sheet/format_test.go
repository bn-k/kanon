package sheet

import (
	"math"
	"testing"
)

func TestParseFormat(t *testing.T) {
	tests := []struct {
		code string
		want format
	}{
		{"General", format{}},
		{"0.00", format{decimals: 2}},
		{"#,##0", format{}},
		{"0.00E+00", format{decimals: 2}},
		{"@", format{}},
		{"0%", format{kind: percent}},
		{"0.00%", format{kind: percent, decimals: 2}},
		{`0.0" %"`, format{decimals: 1}}, // a quoted % is only shown: the number is not a fraction
		{"dd/mm/yyyy", format{kind: clock, day: true}},
		{`dd\/mm\/yyyy`, format{kind: clock, day: true}},
		{"mm-dd-yy", format{kind: clock, day: true}},
		{"d-mmm", format{kind: clock, day: true}},
		{"mmmm", format{kind: clock, day: true}},
		{"[$-40C]d mmmm yyyy", format{kind: clock, day: true}},
		{"[$-F800]dddd, mmmm dd, yyyy", format{kind: clock, day: true}},
		{"m/d/yy h:mm", format{kind: clock, day: true, time: true}},
		{"yyyy-mm-dd hh:mm:ss", format{kind: clock, day: true, time: true}},
		{"h:mm AM/PM", format{kind: clock, time: true}},
		{"h:mm:ss A/P", format{kind: clock, time: true}},
		{"mm:ss", format{kind: clock, time: true}},
		{"hh:mm:ss.000", format{kind: clock, time: true}},
		{"[h]:mm:ss", format{kind: elapsed}},
		{"[mm]:ss", format{kind: elapsed}},
		{`#,##0.00\ "€"`, format{kind: money, currency: "EUR", decimals: 2}},
		{`#\ ##0.00\ \€`, format{kind: money, currency: "EUR", decimals: 2}},
		{`#,##0.00\ [$€-40C];[Red]\-#,##0.00\ [$€-40C]`, format{kind: money, currency: "EUR", decimals: 2}},
		{`[$$-409]#,##0.00`, format{kind: money, currency: "USD", decimals: 2}},
		{`"$"#,##0_);\("$"#,##0\)`, format{kind: money, currency: "USD"}},
		{`$#,##0.000`, format{kind: money, currency: "USD", decimals: 3}},
		{`[$£-809]#,##0`, format{kind: money, currency: "GBP"}},
		{`[$¥-411]#,##0`, format{kind: money, currency: "JPY"}},
		{`[$CHF] #,##0.00`, format{kind: money, currency: "CHF", decimals: 2}},
		{`#,##0.00 "EUR"`, format{kind: money, currency: "EUR", decimals: 2}},
		{`[$kr-41D] #,##0.00`, format{decimals: 2}},
		{`[$€-40C] #,##0 "USD" \£`, format{kind: money, currency: "EUR"}}, // the first currency
		{`_-* #,##0.00\ _€_-;\-* #,##0.00\ _€_-;_-* "-"??\ _€_-;_-@_-`, format{decimals: 2}},
		{`[Red][<0]0.00;0.00`, format{decimals: 2}},
		{`"Total: "0.0`, format{decimals: 1}},
		{`"day "0`, format{}},
	}
	for _, tt := range tests {
		if got := parseFormat(tt.code); got != tt.want {
			t.Errorf("parseFormat(%q) = %+v, want %+v", tt.code, got, tt.want)
		}
	}
}

func TestBuiltinFormats(t *testing.T) {
	tests := []struct {
		id   int
		want format
	}{
		{0, format{}},
		{2, format{decimals: 2}},
		{9, format{kind: percent}},
		{10, format{kind: percent, decimals: 2}},
		{14, format{kind: clock, day: true}},
		{20, format{kind: clock, time: true}},
		{22, format{kind: clock, day: true, time: true}},
		{46, format{kind: elapsed}},
		{49, format{}},
		{200, format{}}, // custom, but not declared
	}
	for _, tt := range tests {
		if got := numberFormat(tt.id, nil); got != tt.want {
			t.Errorf("numberFormat(%d) = %+v, want %+v", tt.id, got, tt.want)
		}
	}
	custom := map[int]string{164: "0.000"}
	if got := numberFormat(164, custom); got != (format{decimals: 3}) {
		t.Errorf("numberFormat(164) = %+v", got)
	}
}

func TestDecimal(t *testing.T) {
	tests := []struct {
		f     float64
		shift int
		want  string
	}{
		{0, 0, "0"},
		{1250.5, 0, "1250.5"},
		{0.1, 0, "0.1"},
		{0.1 + 0.2, 0, "0.3"},
		{-3, 0, "-3"},
		{1e20, 0, "100000000000000000000"},
		{1.5e-7, 0, "0.00000015"},
		{123456789012345678, 0, "123456789012346000"},
		{0.2, 2, "20"},
		{0.055, 2, "5.5"},
		{0.07, 2, "7"},
		{0.0001, 2, "0.01"},
		{-0.5, 2, "-50"},
		{math.Copysign(0, -1), 0, "0"},
	}
	for _, tt := range tests {
		if got := decimal(tt.f, tt.shift); got != tt.want {
			t.Errorf("decimal(%v, %d) = %q, want %q", tt.f, tt.shift, got, tt.want)
		}
	}
}

func TestNumberText(t *testing.T) {
	eur2 := format{kind: money, currency: "EUR", decimals: 2}
	tests := []struct {
		f        float64
		fm       format
		date1904 bool
		want     string
	}{
		{1250.5, format{}, false, "1250.5"},
		{1250.5, format{decimals: 2}, false, "1250.5"},
		{0.2, format{kind: percent}, false, "20 %"},
		{0.055, format{kind: percent, decimals: 2}, false, "5.5 %"},
		{1250.5, eur2, false, "1250.50 EUR"},
		{1250, eur2, false, "1250.00 EUR"},
		{-12.3, eur2, false, "-12.30 EUR"},
		{1250.125, eur2, false, "1250.125 EUR"},
		{1250, format{kind: money, currency: "USD"}, false, "1250 USD"},
		{1250.5, format{kind: money, currency: "USD"}, false, "1250.5 USD"},
		{46317, format{kind: clock, day: true}, false, "2026-10-22"},
		{46317.6041666667, format{kind: clock, day: true}, false, "2026-10-22"},
		{46317.6041666667, format{kind: clock, day: true, time: true}, false, "2026-10-22 14:30"},
		{46317.6042245370, format{kind: clock, day: true, time: true}, false, "2026-10-22 14:30:05"},
		{46317, format{kind: clock, day: true, time: true}, false, "2026-10-22 00:00"},
		{0.6041666667, format{kind: clock, time: true}, false, "14:30"},
		{0.6042245370, format{kind: clock, time: true}, false, "14:30:05"},
		{46317.6041666667, format{kind: clock, time: true}, false, "2026-10-22 14:30"},
		{1, format{kind: clock, day: true}, false, "1900-01-01"},
		{59, format{kind: clock, day: true}, false, "1900-02-28"},
		{61, format{kind: clock, day: true}, false, "1900-03-01"},
		{44855, format{kind: clock, day: true}, true, "2026-10-22"},
		{0, format{kind: clock, day: true}, true, "1904-01-01"},
		{1.104166666667, format{kind: elapsed}, false, "26 h 30 min"},
		{0.03125, format{kind: elapsed}, false, "45 min"},
		{2.0 / 24, format{kind: elapsed}, false, "2 h"},
		{0, format{kind: elapsed}, false, "0 min"},
		{30.0 / 86400, format{kind: elapsed}, false, "0.5 min"},
	}
	for _, tt := range tests {
		got, ok := numberText(tt.f, tt.fm, tt.date1904)
		if !ok || got != tt.want {
			t.Errorf("numberText(%v, %+v, %v) = %q, %v; want %q", tt.f, tt.fm, tt.date1904, got, ok, tt.want)
		}
	}
}

func TestNumberTextNotADate(t *testing.T) {
	day := format{kind: clock, day: true}
	for _, f := range []float64{-1, 60, 2958466, -0.5} {
		if got, ok := numberText(f, day, false); ok {
			t.Errorf("numberText(%v) = %q, want not a date", f, got)
		}
	}
	if got, ok := numberText(-1, format{kind: elapsed}, false); ok {
		t.Errorf("numberText(-1, elapsed) = %q, want not a duration", got)
	}
}

func TestISOText(t *testing.T) {
	tests := []struct{ s, want string }{
		{"2026-10-22", "2026-10-22"},
		{"2026-10-22T00:00:00", "2026-10-22"},
		{"2026-10-22T14:30:00", "2026-10-22 14:30"},
		{"2026-10-22T14:30:05.000", "2026-10-22 14:30:05"},
		{"2026-10-22T14:30:05Z", "2026-10-22 14:30:05"},
	}
	for _, tt := range tests {
		got, ok := isoText(tt.s)
		if !ok || got != tt.want {
			t.Errorf("isoText(%q) = %q, %v; want %q", tt.s, got, ok, tt.want)
		}
	}
	if got, ok := isoText("22/10/2026"); ok {
		t.Errorf("isoText(22/10/2026) = %q, want not a date", got)
	}
}
