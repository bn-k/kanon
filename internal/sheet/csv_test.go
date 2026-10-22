package sheet

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/func-it/kanon/internal/diag"
)

// writeFile writes content to a file called name in a new folder and returns its path.
func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// texts returns the text of every cell, row by row, the header first.
func texts(s *Sheet) [][]string {
	var out [][]string
	for _, row := range append([][]Cell{s.Header}, s.Rows...) {
		var r []string
		for _, c := range row {
			r = append(r, c.Text)
		}
		out = append(out, r)
	}
	return out
}

// readOK reads a sheet that must hold no mistake.
func readOK(t *testing.T, path, name string) *Sheet {
	t.Helper()
	s, diags := Read(path, name)
	if len(diags) > 0 || s == nil {
		t.Fatalf("Read(%s) = %v, %v", path, s, diags)
	}
	return s
}

// wantDiags checks the diagnostics, written as line:col: severity: message, the file left out.
func wantDiags(t *testing.T, diags diag.List, want ...string) {
	t.Helper()
	var got []string
	for _, d := range diags {
		got = append(got, strings.TrimPrefix(strings.TrimPrefix(d.String(), d.Pos.File+":"), " "))
	}
	if !slices.Equal(got, want) {
		t.Errorf("diagnostics:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestCSV(t *testing.T) {
	path := writeFile(t, "mesures.csv", bom+"id,pièce,surface\nsejour,Séjour,\"32,5\"\ncuisine,\"Cuisine\nouverte\",12\n")
	s := readOK(t, path, "")
	want := [][]string{{"id", "pièce", "surface"}, {"sejour", "Séjour", "32,5"}, {"cuisine", "Cuisine\nouverte", "12"}}
	if got := texts(s); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("cells = %q, want %q", got, want)
	}
	if s.Path != path || s.Name != "" || s.Pos() != (diag.Pos{File: path}) {
		t.Errorf("Path, Name, Pos = %q, %q, %v", s.Path, s.Name, s.Pos())
	}
	tests := []struct {
		cell Cell
		line int
		col  int
	}{
		{s.Header[0], 1, 1}, // the byte order mark doesn't count
		{s.Header[2], 1, 10},
		{s.Rows[0][2], 2, 15}, // in characters: é is one
		{s.Rows[1][1], 3, 9},
		{s.Rows[1][2], 4, 10}, // after the cell on two lines
	}
	for _, tt := range tests {
		if tt.cell.Pos != (diag.Pos{File: path, Line: tt.line, Col: tt.col}) || tt.cell.Ref != "" {
			t.Errorf("%q at %v %q, want %d:%d", tt.cell.Text, tt.cell.Pos, tt.cell.Ref, tt.line, tt.col)
		}
	}
}

func TestCSVSemicolons(t *testing.T) {
	// French Excel exports separate cells with semicolons, as the comma is their decimal point.
	s := readOK(t, writeFile(t, "a.csv", "id;prix\r\nlait;1,05\r\n"), "")
	want := [][]string{{"id", "prix"}, {"lait", "1,05"}}
	if got := texts(s); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("cells = %q, want %q", got, want)
	}
	// A comma in the header line keeps the comma as the separator.
	s = readOK(t, writeFile(t, "b.csv", "id,note\nlait,a;b\n"), "")
	if got := s.Rows[0][1].Text; got != "a;b" {
		t.Errorf("cell = %q, want a;b", got)
	}
}

func TestCSVRows(t *testing.T) {
	// Empty rows are dropped, empty cells past the header too; short rows get empty cells.
	path := writeFile(t, "a.csv", "id;nom;;\n\n;;;\nlait;;;\neau\n  ;\npain;b;c\n")
	s := readOK(t, path, "")
	want := [][]string{{"id", "nom"}, {"lait", ""}, {"eau", ""}, {"pain", "b", "c"}}
	if got := texts(s); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("cells = %q, want %q", got, want)
	}
	if got := s.Rows[0][1].Pos; got != (diag.Pos{File: path, Line: 4, Col: 6}) {
		t.Errorf("empty cell at %v, want 4:6", got)
	}
	if got := s.Rows[1][1].Pos; got != (diag.Pos{File: path, Line: 5, Col: 4}) {
		t.Errorf("missing cell at %v, want 5:4, past the end of the line", got)
	}
	if got := s.Rows[2][0].Pos; got != (diag.Pos{File: path, Line: 7, Col: 1}) {
		t.Errorf("row at %v, want 7:1", got)
	}
}

func TestCSVEmpty(t *testing.T) {
	for _, content := range []string{"", bom, "\n\n", ";;\n"} {
		s := readOK(t, writeFile(t, "a.csv", content), "")
		if s.Header != nil || s.Rows != nil {
			t.Errorf("%q: header %v, rows %v; want none", content, s.Header, s.Rows)
		}
	}
}

func TestCSVMistakes(t *testing.T) {
	quotes := `write a double quote inside a cell as two, with the cell in double quotes: "say ""hi"" twice"`
	tests := []struct {
		content string
		rows    int // the rows read before the mistake; -1 when the file can't be read at all
		want    string
	}{
		{"id,nom\né,b\"c\n", 0, "2:4: error: " + quotes},
		{"id,nom\na,\"b\"x,c\n", 0, "2:5: error: " + quotes},
		{"id,nom\na,b\nc,\"d\ne\n", 1, "3:1: error: a double quote opened on this line is never closed"},
		{"id,nom\na,b\nc,\"d", 1, "3:1: error: a double quote opened on this line is never closed"},
		{"id,nom\nécu,\xe9t\xe9\n", -1, "2:5: error: the file is not in UTF-8: save it as CSV UTF-8"},
	}
	for _, tt := range tests {
		s, diags := Read(writeFile(t, "a.csv", tt.content), "")
		wantDiags(t, diags, tt.want)
		if tt.rows < 0 && s != nil || tt.rows >= 0 && (s == nil || len(s.Rows) != tt.rows) {
			t.Errorf("%q: sheet %v, want %d rows", tt.content, s, tt.rows)
		}
	}
}
