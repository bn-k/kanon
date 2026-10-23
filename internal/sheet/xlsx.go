package sheet

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/names"
)

// The parts of a workbook Kanon reads. Tags name no namespace, so that Strict OOXML reads the same.
type (
	xmlRels struct {
		Rels []struct {
			ID     string `xml:"Id,attr"`
			Type   string `xml:"Type,attr"`
			Target string `xml:"Target,attr"`
			Mode   string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	xmlWorkbook struct {
		Pr struct {
			Date1904 string `xml:"date1904,attr"`
		} `xml:"workbookPr"`
		Sheets []struct {
			Name  string     `xml:"name,attr"`
			Attrs []xml.Attr `xml:",any,attr"` // r:id, in the namespace of relationships
		} `xml:"sheets>sheet"`
	}
	xmlStrings struct {
		Items []xmlText `xml:"si"`
	}
	// xmlText is a string, plain or in rich text runs; its phonetic runs (rPh) are left out.
	xmlText struct {
		T    string `xml:"t"`
		Runs []struct {
			T string `xml:"t"`
		} `xml:"r"`
	}
	xmlStyles struct {
		NumFmts []struct {
			ID   int    `xml:"numFmtId,attr"`
			Code string `xml:"formatCode,attr"`
		} `xml:"numFmts>numFmt"`
		CellXfs []struct {
			NumFmtID int `xml:"numFmtId,attr"`
		} `xml:"cellXfs>xf"`
	}
	xmlWorksheet struct {
		Rows []struct {
			R     int       `xml:"r,attr"`
			Cells []xmlCell `xml:"c"`
		} `xml:"sheetData>row"`
	}
	xmlCell struct {
		R  string   `xml:"r,attr"`
		T  string   `xml:"t,attr"`
		S  int      `xml:"s,attr"`
		F  *string  `xml:"f"`
		V  *string  `xml:"v"`
		Is *xmlText `xml:"is"`
	}
)

func (x xmlText) String() string {
	s := x.T
	for _, r := range x.Runs {
		s += r.T
	}
	return unescape(s)
}

// The most XML Kanon reads from a workbook, in one part and in all the parts it reads. A few kilobytes of zip can
// inflate to gigabytes, so the bytes are counted as they are read: the sizes a zip claims are not trusted.
var maxPartMiB, maxWorkbookMiB = 128, 256

// workbook is an xlsx file being read.
type workbook struct {
	zip      *zip.Reader
	path     string
	diags    *diag.List
	date1904 bool
	strings  []string
	formats  []format // by style, the s of a cell
	inflated int64    // the bytes read from all its parts
}

// counter reads a part of a workbook, counting its bytes against the limits.
type counter struct {
	r    io.Reader
	wb   *workbook
	name string
	n    int64
	err  error // the limit passed, if any
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	c.wb.inflated += int64(n)
	tooLarge := "the workbook is too large to read: %s more than %d MiB of XML; save the sheet alone as .csv"
	switch {
	case c.n > int64(maxPartMiB)<<20:
		c.err = fmt.Errorf(tooLarge, c.name+" holds", maxPartMiB)
	case c.wb.inflated > int64(maxWorkbookMiB)<<20:
		c.err = fmt.Errorf(tooLarge, "its parts hold", maxWorkbookMiB)
	}
	if c.err != nil {
		return n, c.err
	}
	return n, err
}

func readXLSX(file, name string, read func(string) ([]byte, error), diags *diag.List) *Sheet {
	at := diag.Pos{File: file}
	data, err := read(file)
	var z *zip.Reader
	if err == nil {
		z, err = zip.NewReader(bytes.NewReader(data), int64(len(data)))
	}
	if err != nil {
		if errors.Is(err, zip.ErrFormat) {
			diags.Errorf(at, "not an xlsx workbook: save it from Excel or LibreOffice as .xlsx")
		} else {
			diags.Errorf(at, "%s", cannotRead(err))
		}
		return nil
	}
	wb := &workbook{zip: z, path: file, diags: diags}
	s, err := wb.read(name)
	if err != nil {
		diags.Errorf(at, "%s", err)
		return nil
	}
	return s
}

// damaged is the error of a workbook that doesn't hold what every workbook holds.
func damaged(format string, args ...any) error {
	return fmt.Errorf("the workbook is damaged: "+format, args...)
}

func (wb *workbook) read(name string) (*Sheet, error) {
	root, err := wb.rels("")
	if err != nil {
		return nil, err
	}
	wbPath := root["officeDocument"]
	if wbPath == "" {
		return nil, damaged("_rels/.rels names no workbook")
	}
	var x xmlWorkbook
	if err := wb.part(wbPath, &x); err != nil {
		return nil, err
	}
	wb.date1904 = x.Pr.Date1904 == "1" || x.Pr.Date1904 == "true"
	rels, err := wb.rels(wbPath)
	if err != nil {
		return nil, err
	}
	if len(x.Sheets) == 0 {
		return nil, errors.New("the workbook holds no sheet")
	}
	i := 0
	if name != "" {
		var all []string
		for _, s := range x.Sheets {
			all = append(all, s.Name)
		}
		i = slices.Index(all, name)
		if i < 0 {
			if c := names.Closest(name, all); c != "" {
				return nil, fmt.Errorf("no sheet %q in %s; did you mean %q?", name, filepath.Base(wb.path), c)
			}
			return nil, fmt.Errorf("no sheet %q in %s; its sheets are %s", name, filepath.Base(wb.path), quoted(all))
		}
	}
	sheet := x.Sheets[i]
	target := ""
	for _, a := range sheet.Attrs {
		if a.Name.Local == "id" && a.Name.Space != "" {
			target = rels[a.Value]
		}
	}
	if target == "" {
		return nil, damaged("sheet %q has no part", sheet.Name)
	}
	if err := wb.readStrings(rels["sharedStrings"]); err != nil {
		return nil, err
	}
	if err := wb.readStyles(rels["styles"]); err != nil {
		return nil, err
	}
	var ws xmlWorksheet
	if err := wb.part(target, &ws); err != nil {
		return nil, err
	}
	return wb.table(sheet.Name, ws), nil
}

// rels reads the relationships of the part at source: their targets by id and by type (worksheet, styles…).
func (wb *workbook) rels(source string) (map[string]string, error) {
	dir, file := path.Split(source)
	var x xmlRels
	if err := wb.part(dir+"_rels/"+file+".rels", &x); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, r := range x.Rels {
		if r.Mode == "External" {
			continue
		}
		target := path.Join(dir, r.Target)
		if strings.HasPrefix(r.Target, "/") {
			target = r.Target[1:]
		}
		out[r.ID] = target
		out[path.Base(r.Type)] = target
	}
	return out, nil
}

// part reads the XML part called name into v.
func (wb *workbook) part(name string, v any) error {
	f, err := wb.zip.Open(name)
	if err != nil {
		return damaged("%s is missing", name)
	}
	defer f.Close()
	c := &counter{r: f, wb: wb, name: name}
	if err := xml.NewDecoder(c).Decode(v); err != nil {
		if c.err != nil {
			return c.err
		}
		return damaged("%s: %v", name, err)
	}
	return nil
}

func (wb *workbook) readStrings(name string) error {
	if name == "" {
		return nil
	}
	var x xmlStrings
	if err := wb.part(name, &x); err != nil {
		return err
	}
	for _, si := range x.Items {
		wb.strings = append(wb.strings, si.String())
	}
	return nil
}

func (wb *workbook) readStyles(name string) error {
	if name == "" {
		return nil
	}
	var x xmlStyles
	if err := wb.part(name, &x); err != nil {
		return err
	}
	custom := map[int]string{}
	for _, f := range x.NumFmts {
		custom[f.ID] = f.Code
	}
	for _, xf := range x.CellXfs {
		wb.formats = append(wb.formats, numberFormat(xf.NumFmtID, custom))
	}
	return nil
}

// table makes the sheet of a worksheet. The workbook leaves out empty cells and rows; a cell or a row without a
// reference follows the one before it.
func (wb *workbook) table(name string, ws xmlWorksheet) *Sheet {
	file := wb.path + "#" + name
	var rows [][]Cell
	r := 0
	for _, xr := range ws.Rows {
		r = max(xr.R, r+1)
		var row []Cell
		for _, xc := range xr.Cells {
			col := column(xc.R)
			if col == 0 {
				col = len(row) + 1
			}
			for len(row) < col-1 {
				row = append(row, cellAt(file, r, len(row)+1))
			}
			c := cellAt(file, r, col)
			c.Text, c.Err = wb.value(xc, c)
			row = append(row, c)
		}
		rows = append(rows, row)
	}
	return table(wb.path, name, rows, func(row []Cell, i int) Cell {
		return cellAt(file, row[0].Pos.Line, i+1)
	})
}

// cellAt makes the empty cell at row r and column col.
func cellAt(file string, r, col int) Cell {
	return Cell{Pos: diag.Pos{File: file, Line: r, Col: col}, Ref: columnName(col) + strconv.Itoa(r)}
}

// value writes the value of a cell as Kanon's canonical text. It reports a value that can't be read, and returns
// true then.
func (wb *workbook) value(xc xmlCell, c Cell) (string, bool) {
	fail := func(format string, args ...any) (string, bool) {
		wb.diags.Errorf(c.Pos, format, args...)
		return "", true
	}
	if xc.T == "inlineStr" {
		if xc.Is == nil {
			return "", false
		}
		return xc.Is.String(), false
	}
	if xc.V == nil {
		if xc.F != nil {
			return fail("%s holds a formula that was never computed: open the workbook in Excel or LibreOffice and save it again", c.Ref)
		}
		return "", false
	}
	v := *xc.V
	switch xc.T {
	case "s":
		i, err := strconv.Atoi(v)
		if err != nil || i < 0 || i >= len(wb.strings) {
			return fail("the workbook is damaged: %s points to shared string %s, which doesn't exist", c.Ref, v)
		}
		return wb.strings[i], false
	case "str":
		return unescape(v), false
	case "b":
		if v == "1" {
			return "yes", false
		}
		return "no", false
	case "e":
		wb.diags.Errorf(c.Pos, "%s holds the error %s; fix its formula, or enter its value", c.Ref, v)
		return v, true
	case "d":
		if s, ok := isoText(v); ok {
			return s, false
		}
		return fail("the workbook is damaged: %s holds %q as a date", c.Ref, v)
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fail("the workbook is damaged: %s holds %q as a number", c.Ref, v)
	}
	var fm format
	if xc.S >= 0 && xc.S < len(wb.formats) {
		fm = wb.formats[xc.S]
	}
	if s, ok := numberText(f, fm, wb.date1904); ok {
		return s, false
	}
	what := "a real date: enter a date such as 2026-10-22"
	if fm.kind == elapsed {
		what = "a duration: enter one such as 1:30"
	}
	wb.diags.Errorf(c.Pos, "%s holds %s, which is not %s", c.Ref, decimal(f, 0), what)
	return decimal(f, 0), true
}

// column returns the column number of a cell reference, 3 for C4, or 0 when ref is none.
func column(ref string) int {
	n, i := 0, 0
	for ; i < len(ref) && ref[i] >= 'A' && ref[i] <= 'Z'; i++ {
		n = n*26 + int(ref[i]-'A'+1)
	}
	if i == 0 || i == len(ref) {
		return 0
	}
	if _, err := strconv.Atoi(ref[i:]); err != nil {
		return 0
	}
	return n
}

// columnName returns the letters of column n: A, Z, AA.
func columnName(n int) string {
	s := ""
	for ; n > 0; n = (n - 1) / 26 {
		s = string(rune('A'+(n-1)%26)) + s
	}
	return s
}

// escaped is a character the workbook writes as _xHHHH_, such as a carriage return: _x000D_.
var escaped = regexp.MustCompile(`_x[0-9A-Fa-f]{4}_`)

func unescape(s string) string {
	if !strings.Contains(s, "_x") {
		return s
	}
	return escaped.ReplaceAllStringFunc(s, func(m string) string {
		n, _ := strconv.ParseUint(m[2:6], 16, 32)
		return string(rune(n))
	})
}

func quoted(list []string) string {
	q := make([]string, len(list))
	for i, s := range list {
		q[i] = strconv.Quote(s)
	}
	return strings.Join(q, ", ")
}
