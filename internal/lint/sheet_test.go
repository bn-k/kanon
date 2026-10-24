package lint

import (
	"archive/zip"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// readings is the schema of a house and the readings taken in it, an array of objects a spreadsheet can hold.
const readings = `
schema House {
  name:     Text @heading
  readings: [Reading]?
}

schema Reading {
  date:  Date
  room:  Text
  value: Number
  note:  Text?
  by:    -> Person?
}

schema Person {
  name: Text @heading
}
`

// lilac is a document of a House, then what follows its title.
func lilac(rest string) string {
	return "---\ntype: House\n---\n# Lilac\n\n" + rest
}

// sheetCell is a cell of a workbook written by workbook: a number when it reads as one, an error when it starts
// with #, else a string.
func sheetCell(ref, text string) string {
	if _, err := strconv.ParseFloat(text, 64); err == nil {
		return fmt.Sprintf(`<c r="%s"><v>%s</v></c>`, ref, text)
	}
	if strings.HasPrefix(text, "#") {
		return fmt.Sprintf(`<c r="%s" t="e"><v>%s</v></c>`, ref, text)
	}
	return fmt.Sprintf(`<c r="%s" t="inlineStr"><is><t>%s</t></is></c>`, ref, text)
}

// workbook is an xlsx file holding sheets, by name in order, each a CSV text: one line per row, cells separated by
// commas, an empty cell left out.
func workbook(t *testing.T, sheets ...string) string {
	t.Helper()
	const (
		main = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"`
		rels = `xmlns="http://schemas.openxmlformats.org/package/2006/relationships"`
		rel  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	)
	parts := map[string]string{
		"_rels/.rels": fmt.Sprintf(`<Relationships %s><Relationship Id="rId1" Type="%s/officeDocument" Target="xl/workbook.xml"/></Relationships>`, rels, rel),
	}
	var list, links []string
	for i := 0; i < len(sheets); i += 2 {
		n := i/2 + 1
		list = append(list, fmt.Sprintf(`<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, sheets[i], n, n))
		links = append(links, fmt.Sprintf(`<Relationship Id="rId%d" Type="%s/worksheet" Target="worksheets/sheet%d.xml"/>`, n, rel, n))
		var rows strings.Builder
		for r, line := range strings.Split(strings.TrimSuffix(sheets[i+1], "\n"), "\n") {
			fmt.Fprintf(&rows, `<row r="%d">`, r+1)
			for c, text := range strings.Split(line, ",") {
				if text != "" {
					rows.WriteString(sheetCell(fmt.Sprintf("%c%d", 'A'+c, r+1), text))
				}
			}
			rows.WriteString("</row>")
		}
		parts[fmt.Sprintf("xl/worksheets/sheet%d.xml", n)] = fmt.Sprintf(`<worksheet %s><sheetData>%s</sheetData></worksheet>`, main, rows.String())
	}
	parts["xl/workbook.xml"] = fmt.Sprintf(`<workbook %s xmlns:r="%s"><sheets>%s</sheets></workbook>`, main, rel, strings.Join(list, ""))
	parts["xl/_rels/workbook.xml.rels"] = fmt.Sprintf(`<Relationships %s>%s</Relationships>`, rels, strings.Join(links, ""))
	var b strings.Builder
	z := zip.NewWriter(&b)
	for name, content := range parts {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(content))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

const readingRows = "id,date,room,value\nm1,2026-10-01,lounge,19.5\nm2,2026-10-02,kitchen,21\n"

func TestSheetSources(t *testing.T) {
	for _, tt := range []struct {
		name, doc string
		more      files
		read      []string
	}{
		{"a key line", lilac("readings: [[readings.csv]]\n"), files{"readings.csv": readingRows}, []string{"readings.csv"}},
		{"under its heading", lilac("## Readings\n\n[[readings.csv]]\n"), files{"readings.csv": readingRows}, []string{"readings.csv"}},
		{"in the object's folder", lilac("readings: [[doc/readings.csv]]\n"), files{"doc/readings.csv": readingRows}, []string{"doc/readings.csv"}},
		{"cells with spaces", lilac("readings: [[readings.csv]]\n"), files{"readings.csv": " id , date,room,value\n m1 ,2026-10-01 , lounge , 19.5\n"}, []string{"readings.csv"}},
		{"a sheet of a workbook", lilac("readings: [[readings.xlsx#autumn]]\n"), files{"readings.xlsx": workbook(t, "notes", "x\n", "autumn", readingRows)}, []string{"readings.xlsx"}},
		{"the first sheet", lilac("readings: [[readings.xlsx]]\n"), files{"readings.xlsx": workbook(t, "autumn", readingRows)}, []string{"readings.xlsx"}},
		{"a link in a cell", lilac("readings: [[readings.csv]]\n"), files{
			"readings.csv": "id,date,room,value,by\nm1,2026-10-01,lounge,19.5,[[claire]]\n",
			"claire.kan":   "---\ntype: Person\n---\n# Claire\n",
		}, []string{"readings.csv", "claire.kan"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fs := files{"schema.kanonarch": readings, "doc.kan": tt.doc}
			for path, src := range tt.more {
				fs[path] = src
			}
			r := lint(t, fs)
			check(t, r, nil)
			checkFiles(t, r, append([]string{"schema.kanonarch", "doc.kan"}, tt.read...)...)
		})
	}
}

func TestSheetErrors(t *testing.T) {
	csv := func(src string) files { return files{"readings.csv": src} }
	key := lilac("readings: [[readings.csv]]\n")
	runCases(t, []lintCase{
		{
			name:   "an unknown column",
			schema: readings,
			doc:    key,
			more:   csv("id,date,room,value,notes\nm1,2026-10-01,lounge,19.5,x\n"),
			want:   []string{"readings.csv:1:20: error: `Reading` has no field `notes`; did you mean `note`?"},
		},
		{
			name:   "a missing column, once per sheet",
			schema: readings,
			doc:    key,
			more:   csv("id,date,value\nm1,2026-10-01,19.5\nm2,2026-10-02,21\n"),
			want:   []string{"readings.csv:1:1: error: a sheet of `Reading` needs a column `room`"},
		},
		{
			name:   "an empty cell",
			schema: readings,
			doc:    key,
			more:   csv("id,date,room,value\nm1,2026-10-01,,19.5\n"),
			want:   []string{"readings.csv:2:15: error: `room` needs a value"},
		},
		{
			name:   "no id column",
			schema: readings,
			doc:    key,
			more:   csv("date,room,value\n2026-10-01,lounge,19.5\n"),
			want:   []string{"readings.csv:1:1: error: a sheet of `Reading` needs an id column"},
		},
		{
			name:   "an id used twice",
			schema: readings,
			doc:    key,
			more:   csv("id,date,room,value\nm1,2026-10-01,lounge,19.5\nm1,2026-10-02,kitchen,21\n"),
			want:   []string{"readings.csv:3:1: error: the id `m1` is already used, at line 2"},
		},
		{
			name:   "an id that is no slug",
			schema: readings,
			doc:    key,
			more:   csv("id,date,room,value\nReading 1,2026-10-01,lounge,19.5\n"),
			want:   []string{`readings.csv:2:1: error: an id is a slug of lowercase ASCII letters, digits and hyphens: write reading-1, not "Reading 1"`},
		},
		{
			name:   "values in another format",
			schema: readings,
			doc:    key,
			more:   csv("id,date,room,value\nm1,14/07/2026,lounge,\"19,5\"\n"),
			want: []string{
				`readings.csv:2:4: error: write a date as YYYY-MM-DD (2026-10-22), not "14/07/2026"`,
				`readings.csv:2:22: error: write 19.5, not "19,5"`,
			},
		},
		{
			name:   "a link in a cell to no document",
			schema: readings,
			doc:    key,
			more: files{
				"readings.csv": "id,date,room,value,by\nm1,2026-10-01,lounge,19.5,[[clair]]\n",
				"claire.kan":   "---\ntype: Person\n---\n# Claire\n",
			},
			want: []string{"readings.csv:2:27: error: no document `clair`; did you mean `claire`?"},
		},
		{
			name:   "a cell past the header",
			schema: readings,
			doc:    key,
			more:   csv("id,date,room,value\nm1,2026-10-01,lounge,19.5,to check\nm2,2026-10-02,kitchen,21,same\n"),
			want:   []string{"readings.csv:2:27: error: this cell is in a column with no name: write a field name at the top of its column, or clear it"},
		},
		{
			name:   "a column with no name",
			schema: readings,
			doc:    key,
			more:   csv("id,,date,room,value\nm1,,2026-10-01,lounge,19.5\nm2,x,2026-10-02,kitchen,21\n"),
			want:   []string{"readings.csv:3:4: error: this cell is in a column with no name: write a field name at the top of its column, or clear it"},
		},
		{
			name:   "a column with no name in a table",
			schema: readings,
			doc:    lilac("## Readings\n\n| id | date | room | value | |\n|---|---|---|---|---|\n| m1 | 2026-10-01 | lounge | 19.5 | x |\n"),
			want:   []string{"doc.kan:10:37: error: this cell is in a column with no name: write a field name at the top of its column, or clear it"},
		},
		{
			name:   "an empty sheet",
			schema: readings,
			doc:    key,
			more:   csv(""),
			want:   []string{"readings.csv: error: the sheet is empty: write the id column and the fields of `Reading` in its first row"},
		},
		{
			name:   "a key line and a table",
			schema: readings,
			doc:    key + "\n## Readings\n\n| id | date | room | value |\n|---|---|---|---|\n| m3 | 2026-10-03 | lounge | 20 |\n",
			more:   csv(readingRows),
			want:   []string{"doc.kan:8:4: error: `readings` is already written, at line 6"},
		},
		{
			name:   "a key line and a folder",
			schema: readings,
			doc:    key,
			more:   files{"readings.csv": readingRows, "doc/readings/m3.kan": "date: 2026-10-03\n"},
			want:   []string{"doc/readings: error: `readings` is written twice: here and at doc.kan:6:1"},
		},
		{
			name:   "the link and the entries under the heading",
			schema: readings,
			doc:    lilac("## Readings\n\n[[readings.csv]]\n\n| id | date | room | value |\n|---|---|---|---|\n| m3 | 2026-10-03 | lounge | 20 |\n"),
			more:   csv(readingRows),
			want:   []string{"doc.kan:8:1: error: `readings` is in [[readings.csv]]: write the link alone under its heading, or the entries here, not both"},
		},
		{
			name:   "a link to a file that is no spreadsheet",
			schema: readings,
			doc:    lilac("readings: [[readings.ods]]\n"),
			want:   []string{"doc.kan:6:11: error: link `readings` to a .csv or .xlsx file, not to readings.ods"},
		},
		{
			name:   "a link to no file",
			schema: readings,
			doc:    lilac("readings: [[reading.csv]]\n"),
			more:   csv(readingRows),
			want:   []string{"doc.kan:6:11: error: no file `reading.csv`; did you mean `readings.csv`?"},
		},
		{
			name:   "a file name that is no id",
			schema: readings,
			doc:    lilac("readings: [[Readings.CSV]]\n"),
			want:   []string{`doc.kan:6:11: error: name the file readings.csv, not "Readings.CSV"`},
		},
		{
			name:   "a list as a key",
			schema: readings,
			doc:    lilac("readings: m1, m2\n"),
			want:   []string{"doc.kan:6:1: error: `readings` is a list: write it under its heading, ## Readings, or link to a spreadsheet, as in readings: [[readings.csv]]"},
		},
		{
			name:   "a sheet of a CSV file",
			schema: readings,
			doc:    lilac("readings: [[readings.csv#autumn]]\n"),
			more:   csv(readingRows),
			want:   []string{`readings.csv: error: a CSV file holds one sheet: write readings.csv, not "readings.csv#autumn"`},
		},
		{
			name:   "a workbook",
			schema: readings,
			doc:    lilac("readings: [[readings.xlsx#autumn]]\n"),
			more:   files{"readings.xlsx": workbook(t, "notes", "x\n", "autumn", readingRows+"m1,2026-10-03,lounge,twenty\nm4,2026-10-04,kitchen,#N/A\n#REF!,2026-10-05,lounge,20\n")},
			want: []string{
				"readings.xlsx#autumn:4:1: error: the id `m1` is already used, in row 2",
				`readings.xlsx#autumn:4:4: error: write a number (12 or 1 250.50), not "twenty"`,
				"readings.xlsx#autumn:5:4: error: D5 holds the error #N/A; fix its formula, or enter its value",
				"readings.xlsx#autumn:6:1: error: A6 holds the error #REF!; fix its formula, or enter its value",
			},
		},
		{
			name:   "an error in the header",
			schema: readings,
			doc:    lilac("readings: [[readings.xlsx]]\n"),
			more:   files{"readings.xlsx": workbook(t, "autumn", "id,date,room,value,#N/A\nm1,2026-10-01,lounge,19.5,x\n")},
			want:   []string{"readings.xlsx#autumn:1:5: error: E1 holds the error #N/A; fix its formula, or enter its value"},
		},
		{
			name:   "an error in the header, for a required column",
			schema: readings,
			doc:    lilac("readings: [[readings.xlsx]]\n"),
			more:   files{"readings.xlsx": workbook(t, "autumn", "id,date,#REF!,value\nm1,2026-10-01,lounge,19.5\n")},
			want:   []string{"readings.xlsx#autumn:1:3: error: C1 holds the error #REF!; fix its formula, or enter its value"},
		},
		{
			name:   "an error in the header, for the id column",
			schema: readings,
			doc:    lilac("readings: [[readings.xlsx]]\n"),
			more:   files{"readings.xlsx": workbook(t, "autumn", "#REF!,date,room,value\nm1,2026-10-01,lounge,19.5\n")},
			want:   []string{"readings.xlsx#autumn:1:1: error: A1 holds the error #REF!; fix its formula, or enter its value"},
		},
		{
			name:   "no such sheet",
			schema: readings,
			doc:    lilac("readings: [[readings.xlsx#autum]]\n"),
			more:   files{"readings.xlsx": workbook(t, "notes", "x\n", "autumn", readingRows)},
			want:   []string{`readings.xlsx: error: no sheet "autum" in readings.xlsx; did you mean "autumn"?`},
		},
	})
}
