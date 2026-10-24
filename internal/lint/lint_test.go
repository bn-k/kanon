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
  title: Text @heading
  date:  Date?
  body:  Prose?
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
			doc:    "---\ntype: Note\ntitle: Hi\n---\n# Hi\n",
			want:   []string{"doc.kan:3:1: error: unknown front matter key `title`; the front matter holds type: only"},
		},
		{
			name:   "unknown schema",
			schema: note,
			doc:    "---\ntype: Nte\n---\n# Hi\n",
			want:   []string{"doc.kan:2:7: error: unknown schema `Nte`; did you mean `Note`?"},
		},
		{
			name:   "no front matter",
			schema: note + "schema Person { name: Text }\n",
			doc:    "# Hi\n",
			want: []string{
				"doc.kan:1:1: error: this document names no schema; begin it with a front matter: ---, then type: Person, then ---",
			},
		},
		{
			name:   "no front matter, and a schema file without schemas",
			schema: "unit pinch\n",
			doc:    "# Hi\n",
			want: []string{
				"doc.kan:1:1: error: this document names no schema; begin it with a front matter: ---, then type: Name, then ---",
			},
		},
	})
}

const linked = `
schema Note {
  title:  Text @heading
  author: -> Person?
  body:   Prose?
}

schema Person {
  name: Text @heading
}

unit pinch
`

func TestLinks(t *testing.T) {
	person := "---\ntype: Person\n---\n# Sarah Miller\n"
	runCases(t, []lintCase{
		{
			name:   "a value that is not a link",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\nauthor: Sarah Miller\n",
			want:   []string{"doc.kan:6:9: error: this field holds a `Person`: link to it, as in [[sarah-miller]]"},
		},
		{
			name:   "a link to an object of the right type",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\nauthor: [[sarah]]\n",
			more:   files{"sarah.kan": person},
		},
		{
			name:   "a link to an object of another type",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\nauthor: [[other]]\n",
			more:   files{"other.kan": "---\ntype: Note\n---\n# Other\n"},
			want:   []string{"doc.kan:6:9: error: this field holds a `Person`; [[other]] is a `Note`"},
		},
		{
			name:   "a link to a schema file from a typed field",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\nauthor: [[schema]]\n",
			want:   []string{"doc.kan:6:9: error: this field holds a `Person`; [[schema]] is a schema file"},
		},
		{
			name:   "links to declarations of the schema",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\n## Body\n\nIn [[schema#pinch]], not in [[schema#pinc]], per [[schema]].\n",
			want:   []string{"doc.kan:8:29: error: unknown declaration `pinc` in schema.kanonarch; did you mean `pinch`?"},
		},
		{
			name:   "a link path that is not made of ids",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\n## Body\n\nSee [[My Notes/Other]] and [[../Other#x]].\n",
			want: []string{
				`doc.kan:8:5: error: a link path is made of ids: write [[my-notes/other]], not "[[My Notes/Other]]"`,
				`doc.kan:8:28: error: a link path is made of ids: write [[../other#x]], not "[[../Other#x]]"`,
			},
		},
		{
			name:   "a link id that is not a slug",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\n## Body\n\nSee [[sarah#Sarah M]].\n",
			more:   files{"sarah.kan": person},
			want:   []string{`doc.kan:8:5: error: an id is a slug: write #sarah-m, not "#Sarah M"`},
		},
		{
			name:   "a link to an unknown id",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\n## Body\n\nSee [[#doc]], [[#dc]] and [[sarah#bob]].\n",
			more:   files{"sarah.kan": person},
			want: []string{
				"doc.kan:8:15: error: unknown id `dc` in doc.kan; did you mean `doc`?",
				"doc.kan:8:27: error: unknown id `bob` in sarah.kan",
			},
		},
		{
			name:   "links to missing documents",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\n## Body\n\nSee [[sarh]], [[people/sarh]] and [[zz]].\n",
			more:   files{"sarah.kan": person, "people/sarah.kan": person, "people/sara.kan/": ""},
			want: []string{
				"doc.kan:8:5: error: no document `sarh`; did you mean `sarah`?",
				"doc.kan:8:15: error: no document `people/sarh`; did you mean `people/sarah`?",
				"doc.kan:8:35: error: no document `zz`",
			},
		},
		{
			name:   "a link to a document that names no schema",
			schema: linked,
			doc:    "---\ntype: Note\n---\n# Hi\n\nauthor: [[empty]]\n",
			more:   files{"empty.kan": "# Empty\n"},
			want: []string{
				"empty.kan:1:1: error: this document names no schema; begin it with a front matter: ---, then type: Person, then ---",
			},
		},
	})
}

func TestLinkedTwice(t *testing.T) {
	r := lint(t, files{
		"schema.kanonarch": linked,
		"sarah.kan":        "---\ntype: Person\n---\n# Sarah Miller\n",
		"doc.kan":          "---\ntype: Note\n---\n# Hi\n\nauthor: [[sarah]]\n\n## Body\n\nBy [[sarah]] and [[sarah#sarah]].\n",
	})
	check(t, r, nil)
	checkFiles(t, r, "schema.kanonarch", "doc.kan", "sarah.kan")
}

func TestLinkToAnotherSchemaFile(t *testing.T) {
	r := lint(t, files{
		"schema.kanonarch": linked,
		"rules.kanonarch":  "unit spoon\n",
		"doc.kan":          "---\ntype: Note\n---\n# Hi\n\n## Body\n\nOne [[rules#spoon]], two [[rules#spon]].\n",
	})
	check(t, r, []string{"doc.kan:8:26: error: unknown declaration `spon` in rules.kanonarch; did you mean `spoon`?"})
	checkFiles(t, r, "schema.kanonarch", "doc.kan", "rules.kanonarch")
}

func TestSlugPath(t *testing.T) {
	tests := []struct{ target, want string }{
		{"My Notes/Other", "my-notes/other"},
		{"../Lease#Café", "../lease#cafe"},
		{"lease", "lease"},
	}
	for _, tt := range tests {
		if got := slugPath(tt.target); got != tt.want {
			t.Errorf("slugPath(%q) = %q, want %q", tt.target, got, tt.want)
		}
	}
}
