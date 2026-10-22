package lint

import (
	"os"
	"testing"
)

const carnet = `
schema Carnet {
  titre:   Text @heading
  date:    Date?
  entrees: [Entree]?
  annexes: [Entree]?
}

schema Entree {
  titre: Text @heading
  corps: Prose? @body
}
`

const carnetDoc = "---\ntype: Carnet\n---\n# Carnet\n\ndate: 2026-10-22\n"

func TestFolders(t *testing.T) {
	runCases(t, []lintCase{
		{
			name:   "entries that insert a field of their parent",
			schema: carnet,
			doc:    carnetDoc,
			more: files{
				"doc/.git/":            "",
				"doc/entrees/.keep":    "",
				"doc/entrees/un.kan":   "# Un\n\nLe {date}, {titre}.\n",
				"doc/entrees/deux.kan": "# Deux\n\nLe {jour}.\n",
			},
			want: []string{"doc/entrees/deux.kan:3:5: error: {jour} inserts no field"},
		},
		{
			name:   "entries of another type, with a wrong name, and other files",
			schema: carnet,
			doc:    carnetDoc,
			more: files{
				"doc/entrees/un.kan":    "---\ntype: Carnet\n---\n# Un\n",
				"doc/entrees/Un 2.kan":  "# Un 2\n",
				"doc/entrees/notes.txt": "",
				"doc/zz/":               "",
			},
			want: []string{
				`doc/entrees/Un 2.kan: error: the file name is the object's id: write un-2.kan, not "Un 2.kan"`,
				"doc/entrees/notes.txt: warning: not a .kan file; ignored",
				"doc/entrees/un.kan:2:7: error: this folder holds `Entree` entries, not `Carnet`",
				"doc/zz: error: `Carnet` has no field `zz`",
			},
		},
		{
			name:   "a file in the folder of the object",
			schema: carnet,
			doc:    carnetDoc,
			more:   files{"doc/notes.txt": "", "doc/notes.kan": "# Notes\n"},
			want: []string{
				"doc/notes.kan: error: this document belongs to no list: move it into a list's folder, as in doc/entrees/notes.kan",
			},
		},
		{
			name:   "a folder for a field that is not a list of objects",
			schema: carnet,
			doc:    carnetDoc,
			more:   files{"doc/date/": ""},
			want:   []string{"doc/date: error: only a list of objects or of files can be a folder, and `date` is a `Date`"},
		},
		{
			name:   "a field written inline and as a folder",
			schema: carnet,
			doc:    carnetDoc + "\n## Entrees\n\n### Un {#un}\n",
			more:   files{"doc/entrees/deux.kan": "# Deux\n"},
			want:   []string{"doc/entrees: error: `entrees` is written twice: here and at doc.kan:8:4"},
		},
		{
			name:   "a folder that belongs to no entry",
			schema: carnet,
			doc:    carnetDoc,
			more: files{
				"doc/entrees/un.kan":  "# Un\n",
				"doc/entrees/un/":     "",
				"doc/entrees/deux/":   "",
				"doc/entrees/un/x/":   "",
				"doc/entrees/un/.x/":  "",
				"doc/annexes/a.kan":   "# A\n",
				"doc/annexes/a/b.kan": "",
			},
			want: []string{
				"doc/annexes/a/b.kan: error: this document belongs to no list: `Entree` has no list of objects",
				"doc/entrees/deux: error: this folder belongs to no entry: there is no deux.kan",
				"doc/entrees/un/x: error: `Entree` has no field `x`",
			},
		},
		{
			name:   "an entry with the id of an inline object, which is in another file",
			schema: carnet,
			doc:    carnetDoc + "\n## Annexes\n\n### Un {#un}\n",
			more:   files{"doc/entrees/un.kan": "# Un\n"},
		},
		{
			name:   "entries of two folders with the same ids, linked by their path",
			schema: carnet,
			doc:    carnetDoc,
			more: files{
				"doc/entrees/un.kan":   "# Un\n",
				"doc/entrees/deux.kan": "# Deux\n",
				"doc/annexes/un.kan":   "# Un\n\nVoir [[../entrees/un]], [[../entrees/deux]] et [[../../doc#deux]].\n",
			},
			want: []string{"doc/annexes/un.kan:3:48: error: unknown id `deux` in doc.kan"},
		},
	})
}

func TestUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every file")
	}
	t.Run("folder", func(t *testing.T) {
		r := lintLocked(t, "doc/entrees", files{
			"schema.kanonarch":   carnet,
			"doc.kan":            carnetDoc,
			"doc/entrees/un.kan": "# Un\n",
		})
		check(t, r, []string{"doc/entrees: error: cannot read the folder: permission denied"})
	})
	t.Run("folder of files", func(t *testing.T) {
		r := lintLocked(t, "doc/photos", files{
			"schema.kanonarch": maison,
			"doc.kan":          maisonDoc,
			"doc/photos/a.jpg": jpeg,
		})
		check(t, r, []string{"doc/photos: error: cannot read the folder: permission denied"})
	})
	t.Run("entry", func(t *testing.T) {
		r := lintLocked(t, "doc/entrees/un.kan", files{
			"schema.kanonarch":     carnet,
			"doc.kan":              carnetDoc,
			"doc/entrees/un.kan":   "# Un\n",
			"doc/entrees/deux.kan": "# Deux\n",
		})
		check(t, r, []string{"doc/entrees/un.kan: error: cannot read the file: permission denied"})
		checkFiles(t, r, "schema.kanonarch", "doc.kan", "doc/entrees/deux.kan", "doc/entrees/un.kan")
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
