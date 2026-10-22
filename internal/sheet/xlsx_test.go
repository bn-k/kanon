package sheet

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/func-it/kanon/internal/diag"
)

const (
	mainNS = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"`
	relNS  = `xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`
	relURL = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
)

// book writes catalogue.xlsx, a workbook of the sheets named, the i-th in xl/worksheets/sheet<i>.xml. It adds
// the parts every workbook has, unless parts holds them: the package relationships, the workbook and its
// relationships, which name xl/sharedStrings.xml and xl/styles.xml when parts holds them.
func book(t *testing.T, parts map[string]string, sheets ...string) string {
	t.Helper()
	all := map[string]string{
		"_rels/.rels": rels(rel{"rId1", "officeDocument", "xl/workbook.xml"}),
	}
	var list, wbRels []string
	for i, name := range sheets {
		list = append(list, fmt.Sprintf(`<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, name, i+1, i+1))
		wbRels = append(wbRels, fmt.Sprintf(`<Relationship Id="rId%d" Type="%sworksheet" Target="worksheets/sheet%d.xml"/>`, i+1, relURL, i+1))
	}
	for _, p := range []string{"sharedStrings", "styles"} {
		if _, ok := parts["xl/"+p+".xml"]; ok {
			wbRels = append(wbRels, fmt.Sprintf(`<Relationship Id="r%s" Type="%s%s" Target="%s.xml"/>`, p, relURL, p, p))
		}
	}
	all["xl/workbook.xml"] = fmt.Sprintf(`<workbook %s %s><workbookPr/><sheets>%s</sheets></workbook>`, mainNS, relNS, strings.Join(list, ""))
	all["xl/_rels/workbook.xml.rels"] = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` + strings.Join(wbRels, "") + `</Relationships>`
	for name, content := range parts {
		all[name] = content
	}
	path := filepath.Join(t.TempDir(), "catalogue.xlsx")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for name, content := range all {
		if content == "" {
			continue // a part left out
		}
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(content))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return path
}

type rel struct{ id, typ, target string }

func rels(rs ...rel) string {
	var b strings.Builder
	b.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for _, r := range rs {
		fmt.Fprintf(&b, `<Relationship Id="%s" Type="%s%s" Target="%s"/>`, r.id, relURL, r.typ, r.target)
	}
	return b.String() + `</Relationships>`
}

// worksheet wraps rows into a worksheet.
func worksheet(rows ...string) string {
	return fmt.Sprintf(`<worksheet %s %s><dimension ref="A1"/><sheetData>%s</sheetData></worksheet>`, mainNS, relNS, strings.Join(rows, ""))
}

// row writes a row of cells.
func row(r int, cells ...string) string {
	return fmt.Sprintf(`<row r="%d" spans="1:7">%s</row>`, r, strings.Join(cells, ""))
}

// is writes a cell holding an inline string.
func is(ref, text string) string {
	return fmt.Sprintf(`<c r="%s" t="inlineStr"><is><t>%s</t></is></c>`, ref, text)
}

// styles writes the styles of a workbook: the custom number formats, then the number format of each style.
func styles(numFmts map[int]string, xfs ...int) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<styleSheet %s><numFmts count="%d">`, mainNS, len(numFmts))
	for id, code := range numFmts {
		fmt.Fprintf(&b, `<numFmt numFmtId="%d" formatCode="%s"/>`, id, strings.ReplaceAll(code, `"`, "&quot;"))
	}
	b.WriteString(`</numFmts><cellStyleXfs count="1"><xf numFmtId="0"/></cellStyleXfs><cellXfs>`)
	for _, id := range xfs {
		fmt.Fprintf(&b, `<xf numFmtId="%d" fontId="0" applyNumberFormat="1"/>`, id)
	}
	return b.String() + `</cellXfs></styleSheet>`
}

func sst(items ...string) string {
	return fmt.Sprintf(`<sst %s count="%d">%s</sst>`, mainNS, len(items), strings.Join(items, ""))
}

func TestXLSX(t *testing.T) {
	path := book(t, map[string]string{
		"xl/sharedStrings.xml": sst(
			`<si><t>lait</t></si>`,
			`<si><r><t xml:space="preserve">Crème </t></r><r><rPr><b/></rPr><t>brûlée</t></r></si>`,
			`<si><t>東京</t><rPh sb="0" eb="2"><t>トウキョウ</t></rPh><phoneticPr fontId="1"/></si>`,
			`<si><t>a_x000D_b</t></si>`,
		),
		"xl/styles.xml": styles(map[int]string{164: `#,##0.00\ "€"`}, 0, 164, 14, 9, 22, 20, 46),
		"xl/worksheets/sheet1.xml": worksheet(
			row(1, is("A1", "id"), is("B1", "nom"), is("C1", "prix"), is("D1", "date"), is("E1", "remise"), is("F1", "actif"), is("G1", "note")),
			row(2,
				`<c r="A2" t="s"><v>0</v></c>`,
				`<c r="B2" t="s"><v>1</v></c>`,
				`<c r="C2" s="1"><v>1250.5</v></c>`,
				`<c r="D2" s="2"><v>46317</v></c>`,
				`<c r="E2" s="3"><f>C2/C2/5</f><v>0.2</v></c>`,
				`<c r="F2" t="b"><v>1</v></c>`,
				`<c r="G2" t="str"><f>A2&amp;"x"</f><v>lait_x000D_x</v></c>`,
				`<c r="H2" s="1"/>`,
			),
			row(4,
				is("A4", "eau"),
				`<c r="C4"><v>0.1</v></c>`,
				`<c r="D4" s="4"><v>46317.604166666664</v></c>`,
				`<c r="E4" s="5"><v>0.60422453703703704</v></c>`,
				`<c r="F4" t="b"><v>0</v></c>`,
				`<c r="G4" s="6"><v>1.1041666666666667</v></c>`,
			),
			row(5, `<c r="A5" t="s"><v>2</v></c>`, is("B5", "_x005F_x000D_"), `<c r="C5" t="s"><v>3</v></c>`),
		),
	}, "produits")
	s := readOK(t, path, "produits")
	want := [][]string{
		{"id", "nom", "prix", "date", "remise", "actif", "note"},
		{"lait", "Crème brûlée", "1250.50 EUR", "2026-10-22", "20 %", "yes", "lait\rx"},
		{"eau", "", "0.1", "2026-10-22 14:30", "14:30:05", "no", "26 h 30 min"},
		{"東京", "_x000D_", "a\rb", "", "", "", ""},
	}
	if got := texts(s); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("cells =\n%q\nwant\n%q", got, want)
	}
	file := path + "#produits"
	if s.Path != path || s.Name != "produits" || s.Pos() != (diag.Pos{File: file}) {
		t.Errorf("Path, Name, Pos = %q, %q, %v", s.Path, s.Name, s.Pos())
	}
	tests := []struct {
		cell Cell
		line int
		col  int
		ref  string
	}{
		{s.Header[2], 1, 3, "C1"},
		{s.Rows[0][6], 2, 7, "G2"},
		{s.Rows[1][1], 4, 2, "B4"}, // skipped by the workbook
		{s.Rows[2][6], 5, 7, "G5"}, // past the end of its row
	}
	for _, tt := range tests {
		if tt.cell.Pos != (diag.Pos{File: file, Line: tt.line, Col: tt.col}) || tt.cell.Ref != tt.ref {
			t.Errorf("%q at %v %q, want %d:%d %s", tt.cell.Text, tt.cell.Pos, tt.cell.Ref, tt.line, tt.col, tt.ref)
		}
	}
}

func TestXLSXSheets(t *testing.T) {
	path := book(t, map[string]string{
		"xl/worksheets/sheet1.xml": worksheet(row(1, is("A1", "un"))),
		"xl/worksheets/sheet2.xml": worksheet(row(1, is("A1", "deux"))),
	}, "Feuil1", "produits")
	for name, want := range map[string]string{"": "un", "Feuil1": "un", "produits": "deux"} {
		s := readOK(t, path, name)
		if s.Header[0].Text != want {
			t.Errorf("sheet %q holds %q, want %q", name, s.Header[0].Text, want)
		}
		if s.Name == "" {
			t.Errorf("sheet %q has no name", name)
		}
	}
	for name, want := range map[string]string{
		"produit": `error: no sheet "produit" in catalogue.xlsx; did you mean "produits"?`,
		"clients": `error: no sheet "clients" in catalogue.xlsx; its sheets are "Feuil1", "produits"`,
	} {
		s, diags := Read(path, name)
		if s != nil {
			t.Errorf("Read(%q) = %v, want nil", name, s)
		}
		wantDiags(t, diags, want)
	}
}

func TestXLSX1904(t *testing.T) {
	path := book(t, map[string]string{
		"xl/workbook.xml": fmt.Sprintf(`<workbook %s %s><workbookPr date1904="1"/><sheets><sheet name="a" sheetId="1" r:id="rId1"/></sheets></workbook>`, mainNS, relNS),
		"xl/styles.xml":   styles(nil, 0, 14),
		"xl/worksheets/sheet1.xml": worksheet(
			row(1, is("A1", "date")),
			row(2, `<c r="A2" s="1"><v>44855</v></c>`),
		),
	}, "a")
	if got := readOK(t, path, "").Rows[0][0].Text; got != "2026-10-22" {
		t.Errorf("date = %q, want 2026-10-22", got)
	}
}

func TestXLSXCells(t *testing.T) {
	// Cells and rows without a reference follow the previous one; t="d" holds an ISO date.
	path := book(t, map[string]string{
		"xl/worksheets/sheet1.xml": worksheet(
			`<row><c t="inlineStr"><is><t>a</t></is></c><c r="C1" t="inlineStr"><is><t>c</t></is></c><c t="inlineStr"><is><r><t>d</t></r></is></c></row>`,
			`<row><c t="d"><v>2026-10-22T14:30:00</v></c><c t="inlineStr"/><c t="str"><f>""</f><v></v></c><c><f>1+1</f><v>2</v></c></row>`,
		),
	}, "a")
	s := readOK(t, path, "")
	want := [][]string{{"a", "", "c", "d"}, {"2026-10-22 14:30", "", "", "2"}}
	if got := texts(s); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("cells = %q, want %q", got, want)
	}
	if c := s.Rows[0][3]; c.Pos.Line != 2 || c.Ref != "D2" {
		t.Errorf("D2 at %v %q", c.Pos, c.Ref)
	}
}

func TestXLSXCellMistakes(t *testing.T) {
	path := book(t, map[string]string{
		"xl/sharedStrings.xml": sst(`<si><t>id</t></si>`),
		"xl/styles.xml":        styles(nil, 0, 14, 46),
		"xl/worksheets/sheet1.xml": worksheet(
			row(1, `<c r="A1" t="s"><v>0</v></c>`, is("B1", "n")),
			row(2, `<c r="A2" t="e"><f>VLOOKUP(1,X:X,1)</f><v>#N/A</v></c>`, `<c r="B2" s="1"><v>60</v></c>`),
			row(3, `<c r="A3" t="s"><v>4</v></c>`, `<c r="B3" s="2"><v>-1</v></c>`),
			row(4, `<c r="A4"><f>A1*2</f></c>`, `<c r="B4"><v>douze</v></c>`),
			row(5, `<c r="A5" t="d"><v>22/10/2026</v></c>`, `<c r="AA5" s="1"><v>-2</v></c>`),
		),
	}, "a")
	s, diags := Read(path, "")
	wantDiags(t, diags,
		"2:1: error: A2 holds the error #N/A; fix its formula, or enter its value",
		"2:2: error: B2 holds 60, which is not a real date: enter a date such as 2026-10-22",
		"3:1: error: the workbook is damaged: A3 points to shared string 4, which doesn't exist",
		"3:2: error: B3 holds -1, which is not a duration: enter one such as 1:30",
		"4:1: error: A4 holds a formula that was never computed: open the workbook in Excel or LibreOffice and save it again",
		`4:2: error: the workbook is damaged: B4 holds "douze" as a number`,
		`5:1: error: the workbook is damaged: A5 holds "22/10/2026" as a date`,
		"5:27: error: AA5 holds -2, which is not a real date: enter a date such as 2026-10-22",
	)
	if s == nil || len(s.Rows) != 4 {
		t.Fatalf("sheet = %v, want 4 rows", s)
	}
	for _, row := range s.Rows {
		if !row[0].Err {
			t.Errorf("%s holds no error", row[0].Ref)
		}
	}
	if got := s.Rows[0][0].Text; got != "#N/A" {
		t.Errorf("A2 = %q, want #N/A", got)
	}
}

func TestXLSXDamaged(t *testing.T) {
	sheet := worksheet(row(1, is("A1", "id")))
	tests := []struct {
		parts map[string]string
		want  string
	}{
		{map[string]string{"_rels/.rels": rels()}, "the workbook is damaged: _rels/.rels names no workbook"},
		{map[string]string{"_rels/.rels": "<Relationships"}, "the workbook is damaged: _rels/.rels: XML syntax error on line 1: unexpected EOF"},
		{map[string]string{"xl/workbook.xml": ""}, "the workbook is damaged: xl/workbook.xml is missing"},
		{map[string]string{"xl/_rels/workbook.xml.rels": ""}, "the workbook is damaged: xl/_rels/workbook.xml.rels is missing"},
		{map[string]string{"xl/worksheets/sheet1.xml": ""}, "the workbook is damaged: xl/worksheets/sheet1.xml is missing"},
		{map[string]string{"xl/worksheets/sheet1.xml": "<worksheet><sheetData><row r=\"x\"/></sheetData></worksheet>"}, `the workbook is damaged: xl/worksheets/sheet1.xml: strconv.ParseInt: parsing "x": invalid syntax`},
		{map[string]string{"xl/worksheets/sheet1.xml": sheet, "xl/_rels/workbook.xml.rels": rels()}, `the workbook is damaged: sheet "a" has no part`},
		{map[string]string{"xl/worksheets/sheet1.xml": sheet, "xl/sharedStrings.xml": "<sst>"}, "the workbook is damaged: xl/sharedStrings.xml: XML syntax error on line 1: unexpected EOF"},
		{map[string]string{"xl/worksheets/sheet1.xml": sheet, "xl/styles.xml": "<styleSheet>"}, "the workbook is damaged: xl/styles.xml: XML syntax error on line 1: unexpected EOF"},
		{map[string]string{"xl/workbook.xml": "<workbook><sheets/></workbook>"}, "the workbook holds no sheet"},
	}
	for _, tt := range tests {
		s, diags := Read(book(t, tt.parts, "a"), "")
		if s != nil {
			t.Errorf("%s: sheet %v, want nil", tt.want, s)
		}
		wantDiags(t, diags, "error: "+tt.want)
	}
}

func TestXLSXTooLarge(t *testing.T) {
	// A few kilobytes of zip can inflate to gigabytes of XML: Kanon stops reading past its limits.
	defer func(part, all int) { maxPartMiB, maxWorkbookMiB = part, all }(maxPartMiB, maxWorkbookMiB)
	maxPartMiB, maxWorkbookMiB = 1, 2
	blank := strings.Repeat(" ", 900<<10) // 900 KiB of spaces between the tags of a part
	tests := []struct {
		parts map[string]string
		want  string
	}{
		{
			map[string]string{"xl/worksheets/sheet1.xml": worksheet(row(1, is("A1", "id")), blank, blank)},
			"the workbook is too large to read: xl/worksheets/sheet1.xml holds more than 1 MiB of XML; save the sheet alone as .csv",
		},
		{
			map[string]string{
				"xl/sharedStrings.xml":     sst(blank),
				"xl/styles.xml":            "<styleSheet>" + blank + "</styleSheet>",
				"xl/worksheets/sheet1.xml": worksheet(row(1, is("A1", "id")), blank),
			},
			"the workbook is too large to read: its parts hold more than 2 MiB of XML; save the sheet alone as .csv",
		},
	}
	for _, tt := range tests {
		s, diags := Read(book(t, tt.parts, "a"), "")
		if s != nil {
			t.Errorf("%s: sheet %v, want nil", tt.want, s)
		}
		wantDiags(t, diags, "error: "+tt.want)
	}
	// Within the limits, the same parts read.
	maxPartMiB, maxWorkbookMiB = 2, 3
	readOK(t, book(t, tests[1].parts, "a"), "")
}

func TestXLSXNotAWorkbook(t *testing.T) {
	s, diags := Read(writeFile(t, "a.xlsx", "id,nom\n"), "")
	if s != nil {
		t.Errorf("sheet %v, want nil", s)
	}
	wantDiags(t, diags, "error: not an xlsx workbook: save it from Excel or LibreOffice as .xlsx")
	_, diags = Read(filepath.Join(t.TempDir(), "absent.xlsx"), "")
	wantDiags(t, diags, "error: cannot read the file: no such file or directory")
}

func TestXLSXPaths(t *testing.T) {
	// Strict OOXML has other namespaces; targets may be absolute; external relationships are skipped.
	strict := "http://purl.oclc.org/ooxml/officeDocument/relationships/"
	path := book(t, map[string]string{
		"_rels/.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="` + strict + `officeDocument" Target="/xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml": `<workbook xmlns="http://purl.oclc.org/ooxml/spreadsheetml/main" xmlns:r="` + strings.TrimSuffix(strict, "/") + `">` +
			`<sheets><sheet name="a" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId0" Type="` + strict + `hyperlink" Target="https://example.org/" TargetMode="External"/>` +
			`<Relationship Id="rId1" Type="` + strict + `worksheet" Target="/xl/feuilles/a.xml"/></Relationships>`,
		"xl/feuilles/a.xml": `<worksheet xmlns="http://purl.oclc.org/ooxml/spreadsheetml/main"><sheetData>` + row(1, is("A1", "id")) + `</sheetData></worksheet>`,
	}, "a")
	if got := readOK(t, path, "a").Header[0].Text; got != "id" {
		t.Errorf("A1 = %q, want id", got)
	}
}

func TestColumnName(t *testing.T) {
	for n, want := range map[int]string{1: "A", 26: "Z", 27: "AA", 52: "AZ", 703: "AAA"} {
		if got := columnName(n); got != want {
			t.Errorf("columnName(%d) = %q, want %q", n, got, want)
		}
		if got := column(want + "12"); got != n {
			t.Errorf("column(%s12) = %d, want %d", want, got, n)
		}
	}
	for _, ref := range []string{"", "12", "a1", "A", "A1x"} {
		if got := column(ref); got != 0 {
			t.Errorf("column(%q) = %d, want 0", ref, got)
		}
	}
}
