package sheet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadMistakes(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "d.csv")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		path, name string
		want       string
	}{
		{filepath.Join(dir, "absent.csv"), "", "error: cannot read the file: no such file or directory"},
		{folder, "", "error: cannot read the file: is a directory"},
		{writeFile(t, "a.csv", "id\n"), "feuil1", `error: a CSV file holds one sheet: write a.csv, not "a.csv#feuil1"`},
		{writeFile(t, "notes.ods", ""), "", "error: read a spreadsheet as .csv or .xlsx: save notes.ods as .xlsx"},
		{writeFile(t, "notes.xls", ""), "", "error: read a spreadsheet as .csv or .xlsx: save notes.xls as .xlsx"},
		{writeFile(t, "notes", ""), "", "error: read a spreadsheet as .csv or .xlsx: save notes as .xlsx"},
	}
	for _, tt := range tests {
		s, diags := Read(tt.path, tt.name)
		if s != nil {
			t.Errorf("Read(%s) = %v, want nil", tt.path, s)
		}
		wantDiags(t, diags, tt.want)
	}
}

func TestReadUppercaseExtension(t *testing.T) {
	s := readOK(t, writeFile(t, "A.CSV", "id\nlait\n"), "")
	if len(s.Rows) != 1 {
		t.Errorf("rows = %v", s.Rows)
	}
}
