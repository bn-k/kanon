// Package sheet reads a CSV file, or a sheet of an xlsx workbook, as rows of cells holding Kanon's canonical text:
// a number as 1250.5, a date as 2026-10-22, an amount as 1250.50 EUR, a percentage as 20 %, a boolean as yes.
package sheet

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/func-it/kanon/internal/diag"
)

// Sheet is a table read from a spreadsheet: its first row holds the field names, every other row an object.
type Sheet struct {
	Path   string
	Name   string // the sheet of a workbook; "" for a CSV file
	Header []Cell // the first row holding a value; nil when the sheet holds none
	Rows   [][]Cell
}

// Cell is a cell of a sheet. Rows holding no value are left out; every row has at least as many cells as the
// header, and more only when a cell past the header holds a value.
type Cell struct {
	Pos  diag.Pos // mesures.csv:4:12, line and column in characters; catalogue.xlsx#produits:4:3, row and column
	Ref  string   // the name of a cell of a workbook, C4; "" in a CSV file
	Text string   // in canonical form, or as written for text
	Err  bool     // the cell holds an error (#N/A) or a value that can't be read, already reported
}

// Pos is the position of the whole sheet: mesures.csv or catalogue.xlsx#produits.
func (s *Sheet) Pos() diag.Pos {
	if s.Name == "" {
		return diag.Pos{File: s.Path}
	}
	return diag.Pos{File: s.Path + "#" + s.Name}
}

// Read reads the CSV file at path, or the sheet called name of the xlsx workbook at path: its first sheet when name
// is "". It returns nil when the file can't be read at all; a cell that can't be read is reported, marked Err and
// kept.
func Read(path, name string) (*Sheet, diag.List) {
	var diags diag.List
	at := diag.Pos{File: path}
	base := filepath.Base(path)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv":
		if name != "" {
			diags.Errorf(at, "a CSV file holds one sheet: write %s, not %q", base, base+"#"+name)
			return nil, diags
		}
		return readCSV(path, &diags), diags
	case ".xlsx", ".xlsm":
		return readXLSX(path, name, &diags), diags
	}
	diags.Errorf(at, "read a spreadsheet as .csv or .xlsx: save %s as .xlsx", base)
	return nil, diags
}

// cannotRead is the message for a file that can't be opened.
func cannotRead(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	return "cannot read the file: " + err.Error()
}

// table makes a sheet of the rows read. Rows holding no value are dropped; the first one left is the header, and
// the empty cells past its end are dropped too. A row shorter than the header gets the missing cells from blank,
// which makes the empty cell at index i of row.
func table(path, name string, rows [][]Cell, blank func(row []Cell, i int) Cell) *Sheet {
	s := &Sheet{Path: path, Name: name}
	for _, row := range rows {
		if !slices.ContainsFunc(row, filled) {
			continue
		}
		if s.Header == nil {
			s.Header = trim(row, 0)
			continue
		}
		row = trim(row, len(s.Header))
		for i := len(row); i < len(s.Header); i++ {
			row = append(row, blank(row, i))
		}
		s.Rows = append(s.Rows, row)
	}
	return s
}

func filled(c Cell) bool { return c.Err || strings.TrimSpace(c.Text) != "" }

// trim drops the empty cells at the end of row, past its first n.
func trim(row []Cell, n int) []Cell {
	end := len(row)
	for end > n && !filled(row[end-1]) {
		end--
	}
	return row[:end]
}
