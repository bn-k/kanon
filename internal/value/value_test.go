package value

import (
	"math/big"
	"testing"
	"time"
)

var units = NewUnits([]string{"pinch"})

func TestParseOK(t *testing.T) {
	tests := []struct {
		kind Kind
		s    string
		num  string // the exact amount, as a fraction, for kinds that have one
		unit string
	}{
		{Int, "4", "4/1", ""},
		{Int, "1 250", "1250/1", ""},
		{Int, "-3", "-3/1", ""},
		{Number, "12", "12/1", ""},
		{Number, "1 250.50", "2501/2", ""},
		{Money, "650.00 EUR", "650/1", "EUR"},
		{Money, "9 504.00 EUR", "9504/1", "EUR"},
		{Money, "9 504.00 EUR", "9504/1", "EUR"},
		{Percent, "20 %", "20/1", ""},
		{Percent, "5.5 %", "11/2", ""},
		{Quantity, "87 450 km", "87450/1", "km"},
		{Quantity, "48 m2", "48/1", "m2"},
		{Quantity, "1 pinch", "1/1", "pinch"},
		{Quantity, "4", "4/1", ""},
		{Text, "Riverside Garage", "", ""},
		{Prose, "The rent is {base-rent}.", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.kind.String()+" "+tt.s, func(t *testing.T) {
			v, err := Parse(tt.kind, tt.s, units)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if tt.num != "" && v.Num.String() != tt.num {
				t.Errorf("Num = %s, want %s", v.Num, tt.num)
			}
			if v.Unit != tt.unit {
				t.Errorf("Unit = %q, want %q", v.Unit, tt.unit)
			}
		})
	}
}

// TestParseSpaces checks that a text or a path keeps its non-breaking spaces, which a number reads as spaces.
func TestParseSpaces(t *testing.T) {
	tests := []struct {
		kind    Kind
		s, want string
	}{
		{Text, "  Ångström Way  8  ", "Ångström Way  8 "},
		{Prose, "Rent 650.00 EUR", "Rent 650.00 EUR"},
		{Money, "9 504.00 EUR ", "9 504.00 EUR"},
		{File, "plan A.pdf ", "plan A.pdf "},
		{Percent, "20 %", "20 %"},
		{Duration, " 3 years", "3 years"},
	}
	for _, tt := range tests {
		v, err := Parse(tt.kind, tt.s, units)
		if err != nil || v.Raw != tt.want {
			t.Errorf("Parse(%s, %q) = %q, %v; want %q", tt.kind, tt.s, v.Raw, err, tt.want)
		}
	}
}

func TestParseBool(t *testing.T) {
	for s, want := range map[string]bool{"yes": true, "no": false} {
		v, err := Parse(Bool, s, units)
		if err != nil || v.Bool != want {
			t.Errorf("Parse(Bool, %q) = %v, %v; want %v", s, v.Bool, err, want)
		}
	}
}

func TestParseDate(t *testing.T) {
	v, err := Parse(Date, "2026-09-01", units)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC); !v.Date.Equal(want) {
		t.Errorf("Date = %v, want %v", v.Date, want)
	}
}

func TestParseDateTime(t *testing.T) {
	plus2 := time.FixedZone("", 2*60*60)
	tests := []struct {
		s    string
		want time.Time
		zone string
	}{
		{"2026-10-22 14:30", time.Date(2026, 10, 22, 14, 30, 0, 0, time.UTC), ""},
		{"2026-10-22 14:30:00", time.Date(2026, 10, 22, 14, 30, 0, 0, time.UTC), ""},
		{"2026-10-22 14:30:05", time.Date(2026, 10, 22, 14, 30, 5, 0, time.UTC), ""},
		{"2026-10-22 14:30 UTC", time.Date(2026, 10, 22, 14, 30, 0, 0, time.UTC), "UTC"},
		{"2026-10-22 14:30:05 +02:00", time.Date(2026, 10, 22, 14, 30, 5, 0, plus2), "+02:00"},
		{"2026-10-22 14:30 -00:00", time.Date(2026, 10, 22, 14, 30, 0, 0, time.UTC), "-00:00"},
	}
	for _, tt := range tests {
		v, err := Parse(DateTime, tt.s, units)
		if err != nil {
			t.Errorf("Parse(DateTime, %q): %v", tt.s, err)
			continue
		}
		if !v.Date.Equal(tt.want) {
			t.Errorf("Parse(DateTime, %q) = %v, want %v", tt.s, v.Date, tt.want)
		}
		if v.Zone != tt.zone {
			t.Errorf("Parse(DateTime, %q).Zone = %q, want %q", tt.s, v.Zone, tt.zone)
		}
		if got := v.DateTimeText(); got != tt.s {
			t.Errorf("Parse(DateTime, %q).DateTimeText() = %q", tt.s, got)
		}
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		s    string
		want []Term
	}{
		{"15 min", []Term{{big.NewRat(15, 1), "min"}}},
		{"1 h 30 min", []Term{{big.NewRat(1, 1), "h"}, {big.NewRat(30, 1), "min"}}},
		{"3 years", []Term{{big.NewRat(3, 1), "year"}}},
		{"1 month", []Term{{big.NewRat(1, 1), "month"}}},
		{"1.5 days", []Term{{big.NewRat(3, 2), "day"}}},
	}
	for _, tt := range tests {
		v, err := Parse(Duration, tt.s, units)
		if err != nil {
			t.Errorf("Parse(Duration, %q): %v", tt.s, err)
			continue
		}
		if len(v.Terms) != len(tt.want) {
			t.Errorf("Parse(Duration, %q) = %d terms, want %d", tt.s, len(v.Terms), len(tt.want))
			continue
		}
		for i, term := range v.Terms {
			if term.Num.Cmp(tt.want[i].Num) != 0 || term.Unit != tt.want[i].Unit {
				t.Errorf("Parse(Duration, %q) term %d = %s %s, want %s %s",
					tt.s, i, term.Num, term.Unit, tt.want[i].Num, tt.want[i].Unit)
			}
		}
	}
}

func TestParseVersion(t *testing.T) {
	v, err := Parse(Version, "1.2.30", units)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Version) != 3 || v.Version[0] != 1 || v.Version[1] != 2 || v.Version[2] != 30 {
		t.Errorf("Version = %v, want [1 2 30]", v.Version)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		kind Kind
		s    string
		want string
	}{
		{Int, "", "no value"},
		{Int, "4.5", `write a whole number (4 or 1 250), not "4.5"`},
		{Int, "1250", ""},
		{Int, "1.250", `write a whole number (4 or 1 250), not "1.250"`},
		{Int, "four", `write a whole number (4 or 1 250), not "four"`},
		{Number, "1,5", `write 1.5, not "1,5"`},
		{Number, "1.250,50", `write 1 250.50, not "1.250,50"`},
		{Number, "1,250.50", `write 1 250.50, not "1,250.50"`},
		{Number, "twelve", `write a number (12 or 1 250.50), not "twelve"`},
		{Bool, "true", `write yes or no, not "true"`},
		{Date, "22/10/2026", `write a date as YYYY-MM-DD (2026-10-22), not "22/10/2026"`},
		{Date, "2026-02-30", `"2026-02-30" is not a real date`},
		{DateTime, "22/10/2026 2:30 pm", `write a date and time as YYYY-MM-DD HH:MM (2026-10-22 14:30), not "22/10/2026 2:30 pm"`},
		{DateTime, "2026-10-22", `write a date and time as YYYY-MM-DD HH:MM (2026-10-22 14:30), not "2026-10-22"`},
		{DateTime, "2026-10-22T14:30", `write 2026-10-22 14:30, not "2026-10-22T14:30"`},
		{DateTime, "2026-10-22T14:30:05Z", `write 2026-10-22 14:30:05 UTC, not "2026-10-22T14:30:05Z"`},
		{DateTime, "2026-10-22 14:30+02:00", `write 2026-10-22 14:30 +02:00, not "2026-10-22 14:30+02:00"`},
		{DateTime, "2026-10-22 14:30 CET", `write a time zone as UTC or an offset such as +02:00, not "CET"`},
		{DateTime, "2026-02-30 14:30", `"2026-02-30 14:30" is not a real date and time`},
		{DateTime, "2026-10-22 25:00", `"2026-10-22 25:00" is not a real date and time`},
		{Money, "650,00 €", `write 650.00 EUR, not "650,00 €"`},
		{Money, "€650", `write 650 EUR, not "€650"`},
		{Money, "EUR 650.00", `write 650.00 EUR, not "EUR 650.00"`},
		{Money, "650 euros", `write 650 EUR, not "650 euros"`},
		{Money, "650.00 eur", `write 650.00 EUR, not "650.00 eur"`},
		{Money, "650", `write an amount and a currency code (1 250.50 EUR), not "650"`},
		{Money, "plenty €", `write an amount and a currency code (1 250.50 EUR), not "plenty €"`},
		{Percent, "20%", `write 20 %, not "20%"`},
		{Percent, "20", `write 20 %, not "20"`},
		{Percent, "5,5 %", `write 5.5 %, not "5,5 %"`},
		{Percent, "twenty", `write a percentage (20 %), not "twenty"`},
		{Quantity, "15 000 kms", `unknown unit "kms"; did you mean km?`},
		{Quantity, "2 bowls", `unknown unit "bowls"; a schema declares its own units, as in: unit bowls`},
		{Quantity, "2 crêpes", `unknown unit "crêpes"; a schema declares its own units, as in: unit crepes`},
		{Quantity, "2 hand-fuls", `unknown unit "hand-fuls"; a schema declares its own units, as in: unit handfuls`},
		{Quantity, "12 5000", `write a number and its unit (48 m2 or 1 TB), not "12 5000"`},
		{Quantity, "km", `write a number and its unit (48 m2 or 1 TB), not "km"`},
		{Duration, "3 yrs", `unknown duration unit "yrs"; use min, h, days, weeks, months or years`},
		{Duration, "3", `write a duration, a number and its unit (15 min or 3 years), not "3"`},
		{Duration, "min", `write a duration, a number and its unit (15 min or 3 years), not "min"`},
		{Version, "v580", `write a version, numbers separated by dots (580 or 1.2.3), not "v580"`},
	}
	for _, tt := range tests {
		t.Run(tt.kind.String()+" "+tt.s, func(t *testing.T) {
			_, err := Parse(tt.kind, tt.s, units)
			switch {
			case tt.want == "" && err != nil:
				t.Errorf("unexpected error: %v", err)
			case tt.want != "" && err == nil:
				t.Errorf("no error, want %q", tt.want)
			case err != nil && err.Error() != tt.want:
				t.Errorf("error\n got %s\nwant %s", err, tt.want)
			}
		})
	}
}

func TestKindOf(t *testing.T) {
	for k := Text; k <= Code; k++ {
		got, ok := KindOf(k.String())
		if !ok || got != k {
			t.Errorf("KindOf(%q) = %v, %v", k.String(), got, ok)
		}
	}
	if _, ok := KindOf("Float"); ok {
		t.Error("KindOf(Float) is ok, want unknown")
	}
}

func TestUnitsNames(t *testing.T) {
	names := NewUnits(nil).Names()
	if len(names) != len(BuiltinUnits) {
		t.Errorf("%d names, want %d", len(names), len(BuiltinUnits))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("names are not sorted: %q before %q", names[i-1], names[i])
		}
	}
}
