package lint

import (
	"archive/zip"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// mesures is the schema of a house and the readings taken in it, an array of objects a spreadsheet can hold.
const mesures = `
schema Maison {
  nom:     Text @heading
  mesures: [Mesure]?
}

schema Mesure {
  date:   Date
  piece:  Text
  valeur: Number
  note:   Text?
  par:    -> Personne?
}

schema Personne {
  nom: Text @heading
}
`

// lilas is a document of a Maison, then what follows its title.
func lilas(rest string) string {
	return "---\ntype: Maison\n---\n# Lilas\n\n" + rest
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

const releves = "id,date,piece,valeur\nm1,2026-10-01,salon,19.5\nm2,2026-10-02,cuisine,21\n"

func TestSheetSources(t *testing.T) {
	for _, tt := range []struct {
		name, doc string
		more      files
		read      []string
	}{
		{"a key line", lilas("mesures: [[mesures.csv]]\n"), files{"mesures.csv": releves}, []string{"mesures.csv"}},
		{"under its heading", lilas("## Mesures\n\n[[mesures.csv]]\n"), files{"mesures.csv": releves}, []string{"mesures.csv"}},
		{"in the object's folder", lilas("mesures: [[doc/mesures.csv]]\n"), files{"doc/mesures.csv": releves}, []string{"doc/mesures.csv"}},
		{"cells with spaces", lilas("mesures: [[mesures.csv]]\n"), files{"mesures.csv": " id , date,piece,valeur\n m1 ,2026-10-01 , salon , 19.5\n"}, []string{"mesures.csv"}},
		{"a sheet of a workbook", lilas("mesures: [[mesures.xlsx#releves]]\n"), files{"mesures.xlsx": workbook(t, "notes", "x\n", "releves", releves)}, []string{"mesures.xlsx"}},
		{"the first sheet", lilas("mesures: [[mesures.xlsx]]\n"), files{"mesures.xlsx": workbook(t, "releves", releves)}, []string{"mesures.xlsx"}},
		{"a link in a cell", lilas("mesures: [[mesures.csv]]\n"), files{
			"mesures.csv": "id,date,piece,valeur,par\nm1,2026-10-01,salon,19.5,[[claire]]\n",
			"claire.kan":  "---\ntype: Personne\n---\n# Claire\n",
		}, []string{"mesures.csv", "claire.kan"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fs := files{"schema.kanonarch": mesures, "doc.kan": tt.doc}
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
	csv := func(src string) files { return files{"mesures.csv": src} }
	key := lilas("mesures: [[mesures.csv]]\n")
	runCases(t, []lintCase{
		{
			name:   "an unknown column",
			schema: mesures,
			doc:    key,
			more:   csv("id,date,piece,valeur,notes\nm1,2026-10-01,salon,19.5,x\n"),
			want:   []string{"mesures.csv:1:22: error: `Mesure` has no field `notes`; did you mean `note`?"},
		},
		{
			name:   "a missing column, once per sheet",
			schema: mesures,
			doc:    key,
			more:   csv("id,date,valeur\nm1,2026-10-01,19.5\nm2,2026-10-02,21\n"),
			want:   []string{"mesures.csv:1:1: error: a sheet of `Mesure` needs a column `piece`"},
		},
		{
			name:   "an empty cell",
			schema: mesures,
			doc:    key,
			more:   csv("id,date,piece,valeur\nm1,2026-10-01,,19.5\n"),
			want:   []string{"mesures.csv:2:15: error: `piece` needs a value"},
		},
		{
			name:   "no id column",
			schema: mesures,
			doc:    key,
			more:   csv("date,piece,valeur\n2026-10-01,salon,19.5\n"),
			want:   []string{"mesures.csv:1:1: error: a sheet of `Mesure` needs an id column"},
		},
		{
			name:   "an id used twice",
			schema: mesures,
			doc:    key,
			more:   csv("id,date,piece,valeur\nm1,2026-10-01,salon,19.5\nm1,2026-10-02,cuisine,21\n"),
			want:   []string{"mesures.csv:3:1: error: the id `m1` is already used, at line 2"},
		},
		{
			name:   "an id that is no slug",
			schema: mesures,
			doc:    key,
			more:   csv("id,date,piece,valeur\nMesure 1,2026-10-01,salon,19.5\n"),
			want:   []string{`mesures.csv:2:1: error: an id is a slug of lowercase ASCII letters, digits and hyphens: write mesure-1, not "Mesure 1"`},
		},
		{
			name:   "values in another format",
			schema: mesures,
			doc:    key,
			more:   csv("id,date,piece,valeur\nm1,14/07/2026,salon,\"19,5\"\n"),
			want: []string{
				`mesures.csv:2:4: error: write a date as YYYY-MM-DD (2026-10-22), not "14/07/2026"`,
				`mesures.csv:2:21: error: write 19.5, not "19,5"`,
			},
		},
		{
			name:   "a link in a cell to no document",
			schema: mesures,
			doc:    key,
			more: files{
				"mesures.csv": "id,date,piece,valeur,par\nm1,2026-10-01,salon,19.5,[[clair]]\n",
				"claire.kan":  "---\ntype: Personne\n---\n# Claire\n",
			},
			want: []string{"mesures.csv:2:26: error: no document `clair`; did you mean `claire`?"},
		},
		{
			name:   "a cell past the header",
			schema: mesures,
			doc:    key,
			more:   csv("id,date,piece,valeur\nm1,2026-10-01,salon,19.5,à revoir\nm2,2026-10-02,cuisine,21,idem\n"),
			want:   []string{"mesures.csv:2:26: error: this cell is in a column with no name: write a field name at the top of its column, or clear it"},
		},
		{
			name:   "a column with no name",
			schema: mesures,
			doc:    key,
			more:   csv("id,,date,piece,valeur\nm1,,2026-10-01,salon,19.5\nm2,x,2026-10-02,cuisine,21\n"),
			want:   []string{"mesures.csv:3:4: error: this cell is in a column with no name: write a field name at the top of its column, or clear it"},
		},
		{
			name:   "a column with no name in a table",
			schema: mesures,
			doc:    lilas("## Mesures\n\n| id | date | piece | valeur | |\n|---|---|---|---|---|\n| m1 | 2026-10-01 | salon | 19.5 | x |\n"),
			want:   []string{"doc.kan:10:36: error: this cell is in a column with no name: write a field name at the top of its column, or clear it"},
		},
		{
			name:   "an empty sheet",
			schema: mesures,
			doc:    key,
			more:   csv(""),
			want:   []string{"mesures.csv: error: the sheet is empty: write the id column and the fields of `Mesure` in its first row"},
		},
		{
			name:   "a key line and a table",
			schema: mesures,
			doc:    key + "\n## Mesures\n\n| id | date | piece | valeur |\n|---|---|---|---|\n| m3 | 2026-10-03 | salon | 20 |\n",
			more:   csv(releves),
			want:   []string{"doc.kan:8:4: error: `mesures` is already written, at line 6"},
		},
		{
			name:   "a key line and a folder",
			schema: mesures,
			doc:    key,
			more:   files{"mesures.csv": releves, "doc/mesures/m3.kan": "date: 2026-10-03\n"},
			want:   []string{"doc/mesures: error: `mesures` is written twice: here and at doc.kan:6:1"},
		},
		{
			name:   "the link and the entries under the heading",
			schema: mesures,
			doc:    lilas("## Mesures\n\n[[mesures.csv]]\n\n| id | date | piece | valeur |\n|---|---|---|---|\n| m3 | 2026-10-03 | salon | 20 |\n"),
			more:   csv(releves),
			want:   []string{"doc.kan:8:1: error: `mesures` is in [[mesures.csv]]: write the link alone under its heading, or the entries here, not both"},
		},
		{
			name:   "a link to a file that is no spreadsheet",
			schema: mesures,
			doc:    lilas("mesures: [[releves.ods]]\n"),
			want:   []string{"doc.kan:6:10: error: link `mesures` to a .csv or .xlsx file, not to releves.ods"},
		},
		{
			name:   "a link to no file",
			schema: mesures,
			doc:    lilas("mesures: [[mesure.csv]]\n"),
			more:   csv(releves),
			want:   []string{"doc.kan:6:10: error: no file `mesure.csv`; did you mean `mesures.csv`?"},
		},
		{
			name:   "a file name that is no id",
			schema: mesures,
			doc:    lilas("mesures: [[Mesures.CSV]]\n"),
			want:   []string{`doc.kan:6:10: error: name the file mesures.csv, not "Mesures.CSV"`},
		},
		{
			name:   "a list as a key",
			schema: mesures,
			doc:    lilas("mesures: m1, m2\n"),
			want:   []string{"doc.kan:6:1: error: `mesures` is a list: write it under its heading, ## Mesures, or link to a spreadsheet, as in mesures: [[mesures.csv]]"},
		},
		{
			name:   "a sheet of a CSV file",
			schema: mesures,
			doc:    lilas("mesures: [[mesures.csv#releves]]\n"),
			more:   csv(releves),
			want:   []string{`mesures.csv: error: a CSV file holds one sheet: write mesures.csv, not "mesures.csv#releves"`},
		},
		{
			name:   "a workbook",
			schema: mesures,
			doc:    lilas("mesures: [[mesures.xlsx#releves]]\n"),
			more:   files{"mesures.xlsx": workbook(t, "notes", "x\n", "releves", releves+"m1,2026-10-03,salon,vingt\nm4,2026-10-04,cuisine,#N/A\n#REF!,2026-10-05,salon,20\n")},
			want: []string{
				"mesures.xlsx#releves:4:1: error: the id `m1` is already used, in row 2",
				`mesures.xlsx#releves:4:4: error: write a number (12 or 1 250.50), not "vingt"`,
				"mesures.xlsx#releves:5:4: error: D5 holds the error #N/A; fix its formula, or enter its value",
				"mesures.xlsx#releves:6:1: error: A6 holds the error #REF!; fix its formula, or enter its value",
			},
		},
		{
			name:   "an error in the header",
			schema: mesures,
			doc:    lilas("mesures: [[mesures.xlsx]]\n"),
			more:   files{"mesures.xlsx": workbook(t, "releves", "id,date,piece,valeur,#N/A\nm1,2026-10-01,salon,19.5,x\n")},
			want:   []string{"mesures.xlsx#releves:1:5: error: E1 holds the error #N/A; fix its formula, or enter its value"},
		},
		{
			name:   "an error in the header, for a required column",
			schema: mesures,
			doc:    lilas("mesures: [[mesures.xlsx]]\n"),
			more:   files{"mesures.xlsx": workbook(t, "releves", "id,date,#REF!,valeur\nm1,2026-10-01,salon,19.5\n")},
			want:   []string{"mesures.xlsx#releves:1:3: error: C1 holds the error #REF!; fix its formula, or enter its value"},
		},
		{
			name:   "an error in the header, for the id column",
			schema: mesures,
			doc:    lilas("mesures: [[mesures.xlsx]]\n"),
			more:   files{"mesures.xlsx": workbook(t, "releves", "#REF!,date,piece,valeur\nm1,2026-10-01,salon,19.5\n")},
			want:   []string{"mesures.xlsx#releves:1:1: error: A1 holds the error #REF!; fix its formula, or enter its value"},
		},
		{
			name:   "no such sheet",
			schema: mesures,
			doc:    lilas("mesures: [[mesures.xlsx#releve]]\n"),
			more:   files{"mesures.xlsx": workbook(t, "notes", "x\n", "releves", releves)},
			want:   []string{`mesures.xlsx: error: no sheet "releve" in mesures.xlsx; did you mean "releves"?`},
		},
	})
}
