package lint

import (
	"os"
	"testing"
)

const notebook = `
schema Notebook {
  title:      Text @heading
  date:       Date?
  entries:    [Entry]?
  appendices: [Entry]?
}

schema Entry {
  title: Text @heading
  body:  Prose? @body
}
`

const notebookDoc = "---\ntype: Notebook\n---\n# Notebook\n\ndate: 2026-10-22\n"

func TestFolders(t *testing.T) {
	runCases(t, []lintCase{
		{
			name:   "entries that insert a field of their parent",
			schema: notebook,
			doc:    notebookDoc,
			more: files{
				"doc/.git/":           "",
				"doc/entries/.keep":   "",
				"doc/entries/one.kan": "# One\n\nOn {date}, {title}.\n",
				"doc/entries/two.kan": "# Two\n\nOn {day}.\n",
			},
			want: []string{"doc/entries/two.kan:3:5: error: {day} inserts no field"},
		},
		{
			name:   "entries of another type, with a wrong name, and other files",
			schema: notebook,
			doc:    notebookDoc,
			more: files{
				"doc/entries/one.kan":   "---\ntype: Notebook\n---\n# One\n",
				"doc/entries/One 2.kan": "# One 2\n",
				"doc/entries/notes.txt": "",
				"doc/zz/":               "",
			},
			want: []string{
				`doc/entries/One 2.kan: error: the file name is the object's id: write one-2.kan, not "One 2.kan"`,
				"doc/entries/notes.txt: warning: not a .kan file; ignored",
				"doc/entries/one.kan:2:7: error: this folder holds `Entry` entries, not `Notebook`",
				"doc/zz: error: `Notebook` has no field `zz`",
			},
		},
		{
			name:   "a file in the folder of the object",
			schema: notebook,
			doc:    notebookDoc,
			more:   files{"doc/notes.txt": "", "doc/notes.kan": "# Notes\n"},
			want: []string{
				"doc/notes.kan: error: this document belongs to no list: move it into a list's folder, as in doc/entries/notes.kan",
			},
		},
		{
			name:   "a folder close to a required list counts as it written",
			schema: "schema Journal {\n  title: Text @heading\n  entries: [Entry]\n}\n\nschema Entry {\n  title: Text @heading\n}\n",
			doc:    "---\ntype: Journal\n---\n# Journal\n",
			more:   files{"doc/entrie/one.kan": "# One\n"},
			want:   []string{"doc/entrie: error: `Journal` has no field `entrie`; did you mean `entries`?"},
		},
		{
			name:   "a folder for a field that is not a list of objects",
			schema: notebook,
			doc:    notebookDoc,
			more:   files{"doc/date/": ""},
			want:   []string{"doc/date: error: only a list of objects or of files can be a folder, and `date` is a `Date`"},
		},
		{
			name:   "a field written inline and as a folder",
			schema: notebook,
			doc:    notebookDoc + "\n## Entries\n\n### One {#one}\n",
			more:   files{"doc/entries/two.kan": "# Two\n"},
			want:   []string{"doc/entries: error: `entries` is written twice: here and at doc.kan:8:4"},
		},
		{
			name:   "a folder that belongs to no entry",
			schema: notebook,
			doc:    notebookDoc,
			more: files{
				"doc/entries/one.kan":    "# One\n",
				"doc/entries/one/":       "",
				"doc/entries/two/":       "",
				"doc/entries/one/x/":     "",
				"doc/entries/one/.x/":    "",
				"doc/appendices/a.kan":   "# A\n",
				"doc/appendices/a/b.kan": "",
			},
			want: []string{
				"doc/appendices/a/b.kan: error: this document belongs to no list: `Entry` has no list of objects",
				"doc/entries/one/x: error: `Entry` has no field `x`",
				"doc/entries/two: error: this folder belongs to no entry: there is no two.kan",
			},
		},
		{
			name:   "an entry with the id of an inline object, which is in another file",
			schema: notebook,
			doc:    notebookDoc + "\n## Appendices\n\n### One {#one}\n",
			more:   files{"doc/entries/one.kan": "# One\n"},
		},
		{
			name:   "entries of two folders with the same ids, linked by their path",
			schema: notebook,
			doc:    notebookDoc,
			more: files{
				"doc/entries/one.kan":    "# One\n",
				"doc/entries/two.kan":    "# Two\n",
				"doc/appendices/one.kan": "# One\n\nSee [[../entries/one]], [[../entries/two]] and [[../../doc#two]].\n",
			},
			want: []string{"doc/appendices/one.kan:3:48: error: unknown id `two` in doc.kan"},
		},
	})
}

func TestUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every file")
	}
	t.Run("folder", func(t *testing.T) {
		r := lintLocked(t, "doc/entries", files{
			"schema.kanonarch":    notebook,
			"doc.kan":             notebookDoc,
			"doc/entries/one.kan": "# One\n",
		})
		check(t, r, []string{"doc/entries: error: cannot read the folder: permission denied"})
	})
	t.Run("folder of files", func(t *testing.T) {
		r := lintLocked(t, "doc/photos", files{
			"schema.kanonarch": house,
			"doc.kan":          houseDoc,
			"doc/photos/a.jpg": jpeg,
		})
		check(t, r, []string{"doc/photos: error: cannot read the folder: permission denied"})
	})
	t.Run("entry", func(t *testing.T) {
		r := lintLocked(t, "doc/entries/one.kan", files{
			"schema.kanonarch":    notebook,
			"doc.kan":             notebookDoc,
			"doc/entries/one.kan": "# One\n",
			"doc/entries/two.kan": "# Two\n",
		})
		check(t, r, []string{"doc/entries/one.kan: error: cannot read the file: permission denied"})
		checkFiles(t, r, "schema.kanonarch", "doc.kan", "doc/entries/one.kan", "doc/entries/two.kan")
	})
}

// lintLocked lints fs with the file or folder at locked made unreadable.
func lintLocked(t *testing.T, locked string, fs files) Result {
	t.Helper()
	write(t, fs)
	info, err := os.Stat(locked)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, info.Mode().Perm()) }) // before the folder is removed
	return Run("schema.kanonarch", "doc.kan")
}
