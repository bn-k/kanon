package lint

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// files is a tree to lint, by path relative to the folder of the test. A path that ends with / is an empty folder.
type files map[string]string

// lintCase is a schema.kanonarch, a doc.kan and the files next to them, with the diagnostics a lint run reports.
type lintCase struct {
	name   string
	schema string
	doc    string
	more   files
	want   []string
}

// lint writes fs into a new folder and lints doc.kan there against schema.kanonarch.
func lint(t *testing.T, fs files) Result {
	t.Helper()
	write(t, fs)
	return Run("schema.kanonarch", "doc.kan")
}

// write writes fs into a new folder and makes it the current one.
func write(t *testing.T, fs files) {
	t.Helper()
	t.Chdir(t.TempDir())
	for path, src := range fs {
		if dir, ok := strings.CutSuffix(path, "/"); ok {
			mkdir(t, dir)
			continue
		}
		mkdir(t, filepath.Dir(path))
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

// runCases lints each case and compares its diagnostics, one string each.
func runCases(t *testing.T, cases []lintCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fs := files{"schema.kanonarch": c.schema, "doc.kan": c.doc}
			maps.Copy(fs, c.more)
			check(t, lint(t, fs), c.want)
		})
	}
}

// check compares the diagnostics of r with want.
func check(t *testing.T, r Result, want []string) {
	t.Helper()
	var got []string
	for _, d := range r.Diags {
		got = append(got, d.String())
	}
	if !slices.Equal(got, want) {
		t.Errorf("diagnostics differ\n--- got\n%s\n--- want\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// checkFiles compares the files r read with want.
func checkFiles(t *testing.T, r Result, want ...string) {
	t.Helper()
	if !slices.Equal(r.Files, want) {
		t.Errorf("Files = %q, want %q", r.Files, want)
	}
}

const note = `
schema Note {
  titre: Text @heading
  date:  Date?
  corps: Prose?
}
`

func TestRun(t *testing.T) {
	t.Run("a valid document", func(t *testing.T) {
		r := lint(t, files{"schema.kanonarch": note, "doc.kan": "---\ntype: Note\n---\n# Hi\n\ndate: 2026-10-22\n"})
		check(t, r, nil)
		checkFiles(t, r, "schema.kanonarch", "doc.kan")
	})
	t.Run("a schema with errors stops the run", func(t *testing.T) {
		r := lint(t, files{"schema.kanonarch": "schema note {}\n", "doc.kan": "# Hi\n"})
		check(t, r, []string{`schema.kanonarch:1:8: error: type names are PascalCase: write Note, not "note"`})
		checkFiles(t, r, "schema.kanonarch")
	})
	t.Run("a missing document", func(t *testing.T) {
		r := lint(t, files{"schema.kanonarch": note})
		check(t, r, []string{"doc.kan: error: cannot read the file: no such file or directory"})
		checkFiles(t, r, "schema.kanonarch", "doc.kan")
	})
}

func TestFrontMatter(t *testing.T) {
	runCases(t, []lintCase{
		{
			name:   "unknown key",
			schema: note,
			doc:    "---\ntype: Note\ntitre: Hi\n---\n# Hi\n",
			want:   []string{"doc.kan:3:1: error: unknown front matter key `titre`; the front matter holds type: only"},
		},
		{
			name:   "unknown schema",
			schema: note,
			doc:    "---\ntype: Nte\n---\n# Hi\n",
			want:   []string{"doc.kan:2:7: error: unknown schema `Nte`; did you mean `Note`?"},
		},
		{
			name:   "no front matter",
			schema: note + "schema Personne { nom: Text }\n",
			doc:    "# Hi\n",
			want: []string{
				"doc.kan:1:1: error: this document names no schema; begin it with a front matter: ---, then type: Personne, then ---",
			},
		},
		{
			name:   "no front matter, and a schema file without schemas",
			schema: "unit pincee\n",
			doc:    "# Hi\n",
			want: []string{
				"doc.kan:1:1: error: this document names no schema; begin it with a front matter: ---, then type: Name, then ---",
			},
		},
	})
}

const linked = `
schema Note {
  titre:  Text @heading
  auteur: -> Personne?
  corps:  Prose?
}

schema Personne {
  nom: Text @heading
}

unit pincee
`

func TestLinks(t *testing.T) {
	personne := "---\ntype: Personne\n---\n# Claire Martin\n"
	runCases(t, []lintCase{
		{
			name:   "a value that is not a link",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\nauteur: Claire Martin\n",
			want:   []string{"doc.kan:6:9: error: this field holds a `Personne`: link to it, as in [[claire-martin]]"},
		},
		{
			name:   "a link to an object of the right type",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\nauteur: [[claire]]\n",
			more:   files{"claire.kan": personne},
		},
		{
			name:   "a link to an object of another type",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\nauteur: [[autre]]\n",
			more:   files{"autre.kan": "---\ntype: Note\n---\n# Autre\n"},
			want:   []string{"doc.kan:6:9: error: this field holds a `Personne`; [[autre]] is a `Note`"},
		},
		{
			name:   "a link to a schema file from a typed field",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\nauteur: [[schema]]\n",
			want:   []string{"doc.kan:6:9: error: this field holds a `Personne`; [[schema]] is a schema file"},
		},
		{
			name:   "links to declarations of the schema",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\n## Corps\n\nEn [[schema#pincee]], pas en [[schema#pince]], selon [[schema]].\n",
			want:   []string{"doc.kan:8:30: error: unknown declaration `pince` in schema.kanonarch; did you mean `pincee`?"},
		},
		{
			name:   "a link path that is not made of ids",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\n## Corps\n\nVoir [[Mes Notes/Autre]] et [[../Autre#x]].\n",
			want: []string{
				`doc.kan:8:6: error: a link path is made of ids: write [[mes-notes/autre]], not "[[Mes Notes/Autre]]"`,
				`doc.kan:8:29: error: a link path is made of ids: write [[../autre#x]], not "[[../Autre#x]]"`,
			},
		},
		{
			name:   "a link id that is not a slug",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\n## Corps\n\nVoir [[claire#Claire M]].\n",
			more:   files{"claire.kan": personne},
			want:   []string{`doc.kan:8:6: error: an id is a slug: write #claire-m, not "#Claire M"`},
		},
		{
			name:   "a link to an unknown id",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\n## Corps\n\nVoir [[#doc]], [[#dc]] et [[claire#bob]].\n",
			more:   files{"claire.kan": personne},
			want: []string{
				"doc.kan:8:16: error: unknown id `dc` in doc.kan; did you mean `doc`?",
				"doc.kan:8:27: error: unknown id `bob` in claire.kan",
			},
		},
		{
			name:   "links to missing documents",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\n## Corps\n\nVoir [[clare]], [[gens/clare]] et [[zz]].\n",
			more:   files{"claire.kan": personne, "gens/claire.kan": personne, "gens/clara.kan/": ""},
			want: []string{
				"doc.kan:8:6: error: no document `clare`; did you mean `claire`?",
				"doc.kan:8:17: error: no document `gens/clare`; did you mean `gens/claire`?",
				"doc.kan:8:35: error: no document `zz`",
			},
		},
		{
			name:   "a link to a document that names no schema",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\nauteur: [[vide]]\n",
			more:   files{"vide.kan": "# Vide\n"},
			want: []string{
				"vide.kan:1:1: error: this document names no schema; begin it with a front matter: ---, then type: Personne, then ---",
			},
		},
	})
}

func TestLinkedTwice(t *testing.T) {
	r := lint(t, files{
		"schema.kanonarch": linked,
		"claire.kan":       "---\ntype: Personne\n---\n# Claire Martin\n",
		"doc.kan":          "---\ntype: Note\n---\n# Hi\n\nauteur: [[claire]]\n\n## Corps\n\nPar [[claire]] et [[claire#claire]].\n",
	})
	check(t, r, nil)
	checkFiles(t, r, "schema.kanonarch", "doc.kan", "claire.kan")
}

func TestLinkToAnotherSchemaFile(t *testing.T) {
	r := lint(t, files{
		"schema.kanonarch": linked,
		"regles.kanonarch": "unit tasse\n",
		"doc.kan":          "---\ntype: Note\n---\n# Hi\n\n## Corps\n\nUne [[regles#tasse]], deux [[regles#tase]].\n",
	})
	check(t, r, []string{"doc.kan:8:28: error: unknown declaration `tase` in regles.kanonarch; did you mean `tasse`?"})
	checkFiles(t, r, "schema.kanonarch", "doc.kan", "regles.kanonarch")
}

func TestSlugPath(t *testing.T) {
	tests := []struct{ target, want string }{
		{"Mes Notes/Autre", "mes-notes/autre"},
		{"../Bail#Clé", "../bail#cle"},
		{"bail", "bail"},
	}
	for _, tt := range tests {
		if got := slugPath(tt.target); got != tt.want {
			t.Errorf("slugPath(%q) = %q, want %q", tt.target, got, tt.want)
		}
	}
}
