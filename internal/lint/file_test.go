package lint

import "testing"

const maison = `
schema Maison {
  titre:  Text @heading
  plan:   File?
  photo:  Image?
  photos: [Image]?
  script: Code?
  visite: DateTime?
  corps:  Prose?
}
`

const maisonDoc = "---\ntype: Maison\n---\n# Maison\n\n"

// The first bytes of files, enough for their content to be recognized.
const (
	jpeg = "\xff\xd8\xff\xe0\x00\x10JFIF\x00"
	png  = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"
	pdf  = "%PDF-1.7\n"
	svg  = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 8"/>`
	sh   = "#!/bin/sh\necho ok\n"
)

func TestFiles(t *testing.T) {
	runCases(t, []lintCase{
		{
			name:   "files linked from key lines",
			schema: maison,
			doc: maisonDoc + "plan: [[doc/plan.pdf]]\nphoto: [[doc/salon.jpeg]]\n" +
				"photos: [[doc/salon.jpeg]], [[doc/logo.svg]]\nscript: [[doc/build.sh]]\nvisite: 2026-10-22 14:30\n",
			more: files{"doc/plan.pdf": pdf, "doc/salon.jpeg": jpeg, "doc/logo.svg": svg, "doc/build.sh": sh},
		},
		{
			name:   "missing files",
			schema: maison,
			doc:    maisonDoc + "plan: [[doc/plan.pfd]]\nphoto: [[zz.jpg]]\n",
			more:   files{"doc/plan.pdf": pdf},
			want: []string{
				"doc.kan:6:7: error: no file `doc/plan.pfd`; did you mean `doc/plan.pdf`?",
				"doc.kan:7:8: error: no file `zz.jpg`",
			},
		},
		{
			name:   "an image that is not one, and code that is not text",
			schema: maison,
			doc:    maisonDoc + "photo: [[doc/salon.jpg]]\nscript: [[doc/logo.png]]\n",
			more:   files{"doc/salon.jpg": pdf, "doc/logo.png": png},
			want: []string{
				"doc.kan:6:8: error: doc/salon.jpg is not an image: it holds application/pdf",
				"doc.kan:7:9: error: doc/logo.png is not source code: it holds image/png",
			},
		},
		{
			name:   "an extension that doesn't match the content",
			schema: maison,
			doc:    maisonDoc + "photo: [[doc/salon.png]]\nplan: [[doc/plan.txt]]\n",
			more:   files{"doc/salon.png": jpeg, "doc/plan.txt": pdf},
			want: []string{
				"doc.kan:6:8: warning: doc/salon.png holds a JPEG image; name it doc/salon.jpg",
				"doc.kan:7:7: warning: doc/plan.txt holds a PDF document; name it doc/plan.pdf",
			},
		},
		{
			name:   "a badly named file",
			schema: maison,
			doc:    maisonDoc + "plan: [[Docs/Plan RDC.PDF]]\n",
			want:   []string{`doc.kan:6:7: error: name the file docs/plan-rdc.pdf, not "Docs/Plan RDC.PDF"`},
		},
		{
			name:   "values that are not file links",
			schema: maison,
			doc:    maisonDoc + "plan: plan.pdf\nphoto: [[doc/salon]]\nscript: echo ok\n",
			want: []string{
				"doc.kan:6:7: error: this field holds a `File`: link to it, as in [[plan.pdf]]",
				"doc.kan:7:8: error: this field holds an `Image`: link to a file, with its extension, as in [[photo.jpg]]",
				"doc.kan:8:9: error: this field holds a `Code`: link to its file, as in [[script.sh]], or write it under its heading as a code block",
			},
		},
		{
			name:   "a file that can't be read",
			schema: maison,
			doc:    maisonDoc + "plan: [[plans/rdc.pdf]]\n",
			more:   files{"plans/rdc.pdf/": ""},
			want:   []string{"doc.kan:6:7: error: cannot read plans/rdc.pdf: is a directory"},
		},
		{
			name:   "a date and time",
			schema: maison,
			doc:    maisonDoc + "visite: 22/10/2026 14h30\n",
			want: []string{
				`doc.kan:6:9: error: write a date and time as YYYY-MM-DD HH:MM (2026-10-22 14:30), not "22/10/2026 14h30"`,
			},
		},
	})
}

func TestFileName(t *testing.T) {
	for name, want := range map[string]string{
		"plan.pdf":           "plan.pdf",
		"../Plans/RDC 1.PDF": "../plans/rdc-1.pdf",
		"photo.JPEG":         "photo.jpeg",
		"Été/salon.jpg":      "ete/salon.jpg",
	} {
		if got := fileName(name); got != want {
			t.Errorf("fileName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestFileLists(t *testing.T) {
	head := maisonDoc + "## Photos\n\n"
	runCases(t, []lintCase{
		{
			name:   "a list of file links",
			schema: maison,
			doc:    head + "- [[doc/salon.jpg]]\n- [[doc/cuisine.jpg]]\n",
			more:   files{"doc/salon.jpg": jpeg},
			want:   []string{"doc.kan:9:3: error: no file `doc/cuisine.jpg`"},
		},
		{
			name:   "text under a list of files",
			schema: maison,
			doc:    head + "Du texte.\n",
			want: []string{
				"doc.kan:8:1: error: `photos` is a list of `Image`: write one link per item, as in - [[photo.jpg]]",
			},
		},
		{
			name:   "a folder of images",
			schema: maison,
			doc:    maisonDoc,
			more: files{
				"doc/photos/a-salon.jpg":   jpeg,
				"doc/photos/b.png":         pdf,
				"doc/photos/Cuisine 2.JPG": jpeg,
				"doc/photos/readme":        "",
				"doc/photos/sub/":          "",
				"doc/photos/.cache/":       "",
			},
			want: []string{
				`doc/photos/Cuisine 2.JPG: error: name the file cuisine-2.jpg, not "Cuisine 2.JPG"`,
				"doc/photos/b.png: error: b.png is not an image: it holds application/pdf",
				"doc/photos/readme: error: `photos` is a list of `Image`: name each file with its extension, as in photo.jpg",
				"doc/photos/sub: error: `photos` is a list of `Image`: its folder holds files, not folders",
			},
		},
		{
			name:   "a list of files written inline and as a folder",
			schema: maison,
			doc:    head + "- [[doc/photos/a.jpg]]\n",
			more:   files{"doc/photos/a.jpg": jpeg},
			want:   []string{"doc/photos: error: `photos` is written twice: here and at doc.kan:6:4"},
		},
	})
}

func TestFilesRead(t *testing.T) {
	r := lint(t, files{
		"schema.kanonarch":   maison,
		"doc.kan":            maisonDoc + "photo: [[doc/photos/b.jpg]]\n\n## Corps\n\nVoir ![[doc/photos/b.jpg]].\n",
		"doc/photos/a.jpg":   jpeg,
		"doc/photos/b.jpg":   jpeg,
		"doc/photos/c.jpg/":  "",
		"doc/photos/d.jpg":   jpeg,
		"doc/photos-old.txt": "",
	})
	check(t, r, []string{"doc/photos/c.jpg: error: `photos` is a list of `Image`: its folder holds files, not folders"})
	checkFiles(t, r, "schema.kanonarch", "doc.kan", "doc/photos/b.jpg", "doc/photos/a.jpg", "doc/photos/d.jpg")
}

func TestCode(t *testing.T) {
	head := maisonDoc + "## Script\n\n"
	runCases(t, []lintCase{
		{
			name:   "a code block",
			schema: maison,
			doc:    head + "```sh\necho ok\n```\n",
		},
		{
			name:   "two code blocks",
			schema: maison,
			doc:    head + "```sh\necho 1\n```\n\n```sh\necho 2\n```\n",
			want:   []string{"doc.kan:12:1: error: `script` holds one code block; merge this one into the block at line 8"},
		},
		{
			name:   "text, a key line and a section under code",
			schema: maison,
			doc:    head + "langage: sh\n\nLancer :\n\n```sh\necho ok\n```\n\n### Suite\n",
			want: []string{
				"doc.kan:8:1: error: `script` is code: write it in a fenced block, between ``` lines",
				"doc.kan:10:1: error: `script` is code: write it in a fenced block, between ``` lines",
				"doc.kan:16:1: error: `script` is code: write it in a fenced block, between ``` lines",
			},
		},
		{
			name:   "no code",
			schema: maison,
			doc:    maisonDoc + "## Script\n",
			want:   []string{"doc.kan:6:4: error: `script` holds no code; write it in a fenced block, between ``` lines"},
		},
	})
}

func TestProseLinks(t *testing.T) {
	r := lint(t, files{
		"schema.kanonarch": maison,
		"autre.kan":        maisonDoc,
		"doc/salon.jpg":    jpeg,
		"doc/plan.pdf":     pdf,
		"doc.kan": maisonDoc + "## Corps\n\n" +
			`Voir ![[doc/salon.jpg]], [[doc/plan.pdf]], ![le salon](doc/salon.jpg), [voir {titr}](doc/plan.pdf#page=2), ` +
			`[site](https://kanon.dev), [haut](#top), [autre](autre), [l’autre](autre.kan "Autre") et ![absent](doc/zz.png), ` +
			"[[doc/zz.pdf]], [note] seule, [pas fermé](x\n" +
			"et [l’autre](autre#zz).\n",
	})
	check(t, r, []string{
		"doc.kan:8:79: error: {titr} inserts no field; did you mean {titre}?",
		"doc.kan:8:197: error: no file `doc/zz.png`",
		"doc.kan:8:220: error: no file `doc/zz.pdf`",
		"doc.kan:9:4: error: unknown id `zz` in autre.kan",
	})
	checkFiles(t, r, "schema.kanonarch", "doc.kan", "doc/salon.jpg", "doc/plan.pdf", "autre.kan")
}
