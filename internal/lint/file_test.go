package lint

import "testing"

const house = `
schema House {
  title:  Text @heading
  plan:   File?
  photo:  Image?
  photos: [Image]?
  script: Code?
  visit:  DateTime?
  body:   Prose?
}
`

const houseDoc = "---\ntype: House\n---\n# House\n\n"

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
			schema: house,
			doc: houseDoc + "plan: [[doc/plan.pdf]]\nphoto: [[doc/lounge.jpeg]]\n" +
				"photos: [[doc/lounge.jpeg]], [[doc/logo.svg]]\nscript: [[doc/build.sh]]\nvisit: 2026-10-22 14:30\n",
			more: files{"doc/plan.pdf": pdf, "doc/lounge.jpeg": jpeg, "doc/logo.svg": svg, "doc/build.sh": sh},
		},
		{
			name:   "missing files",
			schema: house,
			doc:    houseDoc + "plan: [[doc/plan.pfd]]\nphoto: [[zz.jpg]]\n",
			more:   files{"doc/plan.pdf": pdf},
			want: []string{
				"doc.kan:6:7: error: no file `doc/plan.pfd`; did you mean `doc/plan.pdf`?",
				"doc.kan:7:8: error: no file `zz.jpg`",
			},
		},
		{
			name:   "an image that is not one, and code that is not text",
			schema: house,
			doc:    houseDoc + "photo: [[doc/lounge.jpg]]\nscript: [[doc/logo.png]]\n",
			more:   files{"doc/lounge.jpg": pdf, "doc/logo.png": png},
			want: []string{
				"doc.kan:6:8: error: doc/lounge.jpg is not an image: it holds application/pdf",
				"doc.kan:7:9: error: doc/logo.png is not source code: it holds image/png",
			},
		},
		{
			name:   "an extension that doesn't match the content",
			schema: house,
			doc:    houseDoc + "photo: [[doc/lounge.png]]\nplan: [[doc/plan.txt]]\n",
			more:   files{"doc/lounge.png": jpeg, "doc/plan.txt": pdf},
			want: []string{
				"doc.kan:6:8: warning: doc/lounge.png holds a JPEG image; name it doc/lounge.jpg",
				"doc.kan:7:7: warning: doc/plan.txt holds a PDF document; name it doc/plan.pdf",
			},
		},
		{
			name:   "a badly named file",
			schema: house,
			doc:    houseDoc + "plan: [[Docs/Ground Floor.PDF]]\n",
			want:   []string{`doc.kan:6:7: error: name the file docs/ground-floor.pdf, not "Docs/Ground Floor.PDF"`},
		},
		{
			name:   "values that are not file links",
			schema: house,
			doc:    houseDoc + "plan: plan.pdf\nphoto: [[doc/lounge]]\nscript: echo ok\n",
			want: []string{
				"doc.kan:6:7: error: this field holds a `File`: link to it, as in [[plan.pdf]]",
				"doc.kan:7:8: error: this field holds an `Image`: link to a file, with its extension, as in [[photo.jpg]]",
				"doc.kan:8:9: error: this field holds a `Code`: link to its file, as in [[script.sh]], or write it under its heading as a code block",
			},
		},
		{
			name:   "a file that can't be read",
			schema: house,
			doc:    houseDoc + "plan: [[plans/ground.pdf]]\n",
			more:   files{"plans/ground.pdf/": ""},
			want:   []string{"doc.kan:6:7: error: cannot read plans/ground.pdf: is a directory"},
		},
		{
			name:   "a date and time",
			schema: house,
			doc:    houseDoc + "visit: 22/10/2026 2:30 pm\n",
			want: []string{
				`doc.kan:6:8: error: write a date and time as YYYY-MM-DD HH:MM (2026-10-22 14:30), not "22/10/2026 2:30 pm"`,
			},
		},
	})
}

func TestFileName(t *testing.T) {
	for name, want := range map[string]string{
		"plan.pdf":             "plan.pdf",
		"../Plans/Floor 1.PDF": "../plans/floor-1.pdf",
		"photo.JPEG":           "photo.jpeg",
		"Éclair/lounge.jpg":    "eclair/lounge.jpg",
	} {
		if got := fileName(name); got != want {
			t.Errorf("fileName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestFileLists(t *testing.T) {
	head := houseDoc + "## Photos\n\n"
	runCases(t, []lintCase{
		{
			name:   "a list of file links",
			schema: house,
			doc:    head + "- [[doc/lounge.jpg]]\n- [[doc/kitchen.jpg]]\n",
			more:   files{"doc/lounge.jpg": jpeg},
			want:   []string{"doc.kan:9:3: error: no file `doc/kitchen.jpg`"},
		},
		{
			name:   "text under a list of files",
			schema: house,
			doc:    head + "Some text.\n",
			want: []string{
				"doc.kan:8:1: error: `photos` is a list of `Image`: write one link per item, as in - [[photo.jpg]]",
			},
		},
		{
			name:   "a folder of images",
			schema: house,
			doc:    houseDoc,
			more: files{
				"doc/photos/a-lounge.jpg":  jpeg,
				"doc/photos/b.png":         pdf,
				"doc/photos/Kitchen 2.JPG": jpeg,
				"doc/photos/readme":        "",
				"doc/photos/sub/":          "",
				"doc/photos/.cache/":       "",
			},
			want: []string{
				`doc/photos/Kitchen 2.JPG: error: name the file kitchen-2.jpg, not "Kitchen 2.JPG"`,
				"doc/photos/b.png: error: b.png is not an image: it holds application/pdf",
				"doc/photos/readme: error: `photos` is a list of `Image`: name each file with its extension, as in photo.jpg",
				"doc/photos/sub: error: `photos` is a list of `Image`: its folder holds files, not folders",
			},
		},
		{
			name:   "a list of files written inline and as a folder",
			schema: house,
			doc:    head + "- [[doc/photos/a.jpg]]\n",
			more:   files{"doc/photos/a.jpg": jpeg},
			want:   []string{"doc/photos: error: `photos` is written twice: here and at doc.kan:6:4"},
		},
	})
}

func TestFilesRead(t *testing.T) {
	r := lint(t, files{
		"schema.kanonarch":   house,
		"doc.kan":            houseDoc + "photo: [[doc/photos/b.jpg]]\n\n## Body\n\nSee ![[doc/photos/b.jpg]].\n",
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
	head := houseDoc + "## Script\n\n"
	runCases(t, []lintCase{
		{
			name:   "a code block",
			schema: house,
			doc:    head + "```sh\necho ok\n```\n",
		},
		{
			name:   "two code blocks",
			schema: house,
			doc:    head + "```sh\necho 1\n```\n\n```sh\necho 2\n```\n",
			want:   []string{"doc.kan:12:1: error: `script` holds one code block; merge this one into the block at line 8"},
		},
		{
			name:   "text, a key line and a section under code",
			schema: house,
			doc:    head + "language: sh\n\nRun:\n\n```sh\necho ok\n```\n\n### Next\n",
			want: []string{
				"doc.kan:8:1: error: `script` is code: write it in a fenced block, between ``` lines",
				"doc.kan:10:1: error: `script` is code: write it in a fenced block, between ``` lines",
				"doc.kan:16:1: error: `script` is code: write it in a fenced block, between ``` lines",
			},
		},
		{
			name:   "no code",
			schema: house,
			doc:    houseDoc + "## Script\n",
			want:   []string{"doc.kan:6:4: error: `script` holds no code; write it in a fenced block, between ``` lines"},
		},
	})
}

func TestProseLinks(t *testing.T) {
	r := lint(t, files{
		"schema.kanonarch": house,
		"other.kan":        houseDoc,
		"doc/lounge.jpg":   jpeg,
		"doc/plan.pdf":     pdf,
		"doc.kan": houseDoc + "## Body\n\n" +
			`See ![[doc/lounge.jpg]], [[doc/plan.pdf]], ![the lounge](doc/lounge.jpg), [see {titl}](doc/plan.pdf#page=2), ` +
			`[site](https://kanon.dev), [top](#top), [other](other), [the other’s](other.kan "Other") and ![missing](doc/zz.png), ` +
			"[[doc/zz.pdf]], [note] alone, [unclosed café](x\n" +
			"and [the other’s](other#zz).\n",
	})
	check(t, r, []string{
		"doc.kan:8:81: error: {titl} inserts no field; did you mean {title}?",
		"doc.kan:8:203: error: no file `doc/zz.png`",
		"doc.kan:8:227: error: no file `doc/zz.pdf`",
		"doc.kan:9:5: error: unknown id `zz` in other.kan",
	})
	checkFiles(t, r, "schema.kanonarch", "doc.kan", "doc/lounge.jpg", "doc/plan.pdf", "other.kan")
}
