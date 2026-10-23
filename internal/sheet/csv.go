package sheet

import (
	"encoding/csv"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/bn-k/kanon/internal/diag"
)

// bom is the byte order mark Excel writes at the start of a CSV file in UTF-8.
const bom = "\xef\xbb\xbf"

const quotesMsg = `write a double quote inside a cell as two, with the cell in double quotes: "say ""hi"" twice"`

// readCSV reads a CSV file in UTF-8. Its cells are separated by commas, or by semicolons when its first line holds
// semicolons and no comma, as French spreadsheets write them.
func readCSV(path string, read func(string) ([]byte, error), diags *diag.List) *Sheet {
	data, err := read(path)
	if err != nil {
		diags.Errorf(diag.Pos{File: path}, "%s", cannotRead(err))
		return nil
	}
	src := strings.TrimPrefix(string(data), bom)
	lines := strings.Split(src, "\n")
	// at turns the byte column of csv.Reader into a column in characters.
	at := func(line, col int) diag.Pos {
		text := lines[min(line, len(lines))-1]
		return diag.Pos{File: path, Line: line, Col: utf8.RuneCountInString(text[:min(col-1, len(text))]) + 1}
	}
	if i := invalidUTF8(src); i >= 0 {
		line := strings.Count(src[:i], "\n") + 1
		diags.Errorf(at(line, i-strings.LastIndexByte(src[:i], '\n')), "the file is not in UTF-8: save it as CSV UTF-8")
		return nil
	}

	r := csv.NewReader(strings.NewReader(src))
	r.FieldsPerRecord = -1
	if first, _, _ := strings.Cut(src, "\n"); !strings.Contains(first, ",") && strings.Contains(first, ";") {
		r.Comma = ';'
	}
	var rows [][]Cell
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			errors.As(err, &pe) // a csv.Reader reading a string fails only on its syntax
			if pe.Line > len(lines) || pe.Column > len(strings.TrimSuffix(lines[pe.Line-1], "\r")) {
				diags.Errorf(diag.Pos{File: path, Line: pe.StartLine, Col: 1}, "a double quote opened on this line is never closed")
			} else {
				diags.Errorf(at(pe.Line, pe.Column), "%s", quotesMsg)
			}
			break
		}
		row := make([]Cell, len(rec))
		for i, text := range rec {
			row[i] = Cell{Pos: at(r.FieldPos(i)), Text: text}
		}
		rows = append(rows, row)
	}
	// A missing cell is placed past the end of its row's last line.
	return table(path, "", rows, func(row []Cell, _ int) Cell {
		last := row[len(row)-1]
		line := last.Pos.Line + strings.Count(last.Text, "\n")
		text := strings.TrimSuffix(lines[line-1], "\r")
		return Cell{Pos: diag.Pos{File: path, Line: line, Col: utf8.RuneCountInString(text) + 1}}
	})
}

// invalidUTF8 returns the index of the first byte of s that isn't UTF-8, or -1.
func invalidUTF8(s string) int {
	for i, r := range s {
		if _, size := utf8.DecodeRuneInString(s[i:]); r == utf8.RuneError && size == 1 {
			return i
		}
	}
	return -1
}
