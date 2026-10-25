package value

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// FuzzParse reads a text as a value of each kind Parse reads. It doesn't panic; an error is a message; a value
// read has its amount, and a date and time writes back as written; and the canonical form a message proposes in
// place of the text reads.
func FuzzParse(f *testing.F) {
	for _, s := range []string{
		"4", "-3", "1 250", "1 250.50", "1,5", "1.250,50", "1,250.50", "twelve", "yes", "no", "true",
		"2026-10-22", "22/10/2026", "2026-02-30", "2026-10-22 14:30", "2026-10-22 14:30:05 UTC",
		"2026-10-22 14:30 +02:00", "2026-10-22T14:30:05Z", "2026-10-22 14:30+02:00", "2026-10-22 14:30 CET",
		"650.00 EUR", "9 504.00 EUR", "650,00 €", "€650", "EUR 650.00", "650 euros", "plenty €",
		"20 %", "20%", "5,5 %", "87 450 km", "48 m2", "1 pinch", "15 000 kms", "2 bowls", "2 crêpes",
		"15 min", "1 h 30 min", "3 yrs", "580", "1.2.3", "v580", "  Ångström Way  8  ",
	} {
		f.Add(s)
	}
	addValues(f, "../../cases")
	f.Fuzz(func(t *testing.T, s string) {
		for k := Text; k < File; k++ {
			v, err := Parse(k, s, units)
			if err != nil {
				msg := err.Error()
				if r, _ := utf8.DecodeRuneInString(msg); msg == "" || unicode.IsUpper(r) || strings.HasSuffix(msg, ".") {
					t.Errorf("%s %q: %q is not a message", k, s, msg)
				}
				if _, decl, ok := strings.Cut(msg, "as in: unit "); ok && !UnitName.MatchString(decl) {
					t.Errorf("%s %q: %q proposes a unit name that a schema refuses", k, s, msg)
				}
				continue
			}
			switch k {
			case Int, Number, Money, Percent, Quantity:
				if v.Num == nil {
					t.Errorf("%s %q: no amount", k, s)
				}
			case Duration:
				for _, term := range v.Terms {
					if term.Num == nil {
						t.Errorf("%s %q: no amount in %s", k, s, term.Unit)
					}
				}
			case DateTime:
				if got := v.DateTimeText(); got != v.Raw {
					t.Errorf("%s %q: writes back as %q", k, s, got)
				}
			}
		}
		n := strings.TrimSpace(spaces.Replace(s))
		for _, fix := range []struct {
			kind Kind
			text string
		}{
			{Number, fixNumber(n)},
			{Money, fixMoney(n)},
			{Percent, fixPercent(n)},
		} {
			if _, err := Parse(fix.kind, fix.text, units); fix.text != "" && err != nil {
				t.Errorf("%s %q: proposes %q, which doesn't read: %v", fix.kind, s, fix.text, err)
			}
		}
		if fix := fixDateTime(s); fix != "" && !dateTimeRe.MatchString(fix) {
			t.Errorf("DateTime %q: proposes %q, which isn't a date and time", s, fix)
		}
	})
}

// addValues adds the values of the key lines and table cells of the documents under root to the seed corpus,
// once each. The Northwind sample is left out: its thousand documents hold values of the same few shapes.
func addValues(f *testing.F, root string) {
	seen := map[string]bool{}
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" && !seen[s] {
			seen[s] = true
			f.Add(s)
		}
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && d.Name() == "northwind":
			return filepath.SkipDir
		case d.IsDir() || filepath.Ext(path) != ".kan":
			return nil
		}
		src, err := os.ReadFile(path)
		for line := range strings.Lines(string(src)) {
			if cells, ok := strings.CutPrefix(strings.TrimSpace(line), "|"); ok {
				for cell := range strings.SplitSeq(cells, "|") {
					add(cell)
				}
			} else if _, v, ok := strings.Cut(line, ": "); ok {
				add(v)
			}
		}
		return err
	})
	if err != nil {
		f.Fatal(err)
	}
}
