package sheet

import (
	"testing"

	"github.com/func-it/kanon/internal/diag"
)

// The workbooks in testdata were saved by Microsoft Excel and published on data.gouv.fr: see testdata/README.md.

// cell finds the cell at ref in a sheet, the header included.
func cell(t *testing.T, s *Sheet, ref string) Cell {
	t.Helper()
	for _, row := range append([][]Cell{s.Header}, s.Rows...) {
		for _, c := range row {
			if c.Ref == ref {
				return c
			}
		}
	}
	t.Fatalf("no cell %s", ref)
	return Cell{}
}

func TestRealBudget(t *testing.T) {
	path := "testdata/boe-budget-primitif-2026.xlsx"
	s := readOK(t, path, "")
	if s.Name != "Nature" || len(s.Header) != 23 || len(s.Rows) != 986 {
		t.Errorf("sheet %q: %d columns, %d rows; want Nature, 23, 986", s.Name, len(s.Header), len(s.Rows))
	}
	tests := []struct{ ref, want string }{
		{"B1", "Dépenses/Recettes"},
		{"F1", "Désignation"},
		{"F2", "Autres matériels de bureau et mobiliers"},
		{"G2", "2093.47 EUR"},    // #\ ##0.00\ \€
		{"H2", "97364.29 EUR"},   // #,##0.00\ [$€-40C];[Red]\-#,##0.00\ [$€-40C]
		{"I2", "0.00 EUR"},       // zero keeps the decimals of its format
		{"I6", "-261300.00 EUR"}, // negative: the first section of the format still says EUR
	}
	for _, tt := range tests {
		if got := cell(t, s, tt.ref).Text; got != tt.want {
			t.Errorf("%s = %q, want %q", tt.ref, got, tt.want)
		}
	}
	file := path + "#Nature"
	if got := cell(t, s, "G2"); got.Pos != (diag.Pos{File: file, Line: 2, Col: 7}) {
		t.Errorf("G2 at %v, want %s:2:7", got.Pos, file)
	}
	if s, _ := Read(path, "Nature"); s == nil || len(s.Rows) != 986 {
		t.Errorf("Read(%s, Nature) = %v", path, s)
	}
}

func TestRealDebt(t *testing.T) {
	// The title of the sheet sits in A3 and its column names in row 7: the first row is the header.
	s := readOK(t, "testdata/lorient-dette-2015.xlsx", "")
	if s.Name != "Sheet1" || len(s.Rows) != 87 {
		t.Errorf("sheet %q, %d rows; want Sheet1, 87", s.Name, len(s.Rows))
	}
	tests := []struct{ ref, want string }{
		{"A3", "ETAT DE LA DETTE - BUDGET PRINCIPAL"},
		{"A7", "N° FICHE"},
		{"S7", "FRAIS"},
		{"A9", "10228"},
		{"C9", "REAMENAGEMENT OCTOBRE 1999"},
		{"I9", "6.05305355248087"}, // 15 significant digits, as Excel shows
		{"K9", "2000-02-01"},       // dd\/mm\/yyyy
		{"M9", "6627610.4"},
	}
	for _, tt := range tests {
		if got := cell(t, s, tt.ref).Text; got != tt.want {
			t.Errorf("%s = %q, want %q", tt.ref, got, tt.want)
		}
	}
}
