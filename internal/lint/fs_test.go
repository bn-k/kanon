package lint

import (
	"errors"
	"slices"
	"testing"
	"testing/fstest"
	"time"
)

const memSchema = `
schema Notebook {
  title:    Text @heading
  photo:    Image?
  readings: [Reading]?
  entries:  [Entry]?
  see:      -> Notebook?
}

schema Reading {
  value: Int
}

schema Entry {
  title: Text @heading
}
`

// brokenFS is a folder in memory whose broken.kan can't be read.
type brokenFS struct{ fstest.MapFS }

func (b brokenFS) ReadFile(name string) ([]byte, error) {
	if name == "broken.kan" {
		return nil, errors.New("the disk is gone")
	}
	return b.MapFS.ReadFile(name)
}

func TestFS(t *testing.T) {
	t.Chdir(t.TempDir()) // nothing on disk: what is read comes from memory
	mem := fstest.MapFS{
		"schema.kanonarch":    {Data: []byte(memSchema)},
		"photo.png":           {Data: []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR")},
		"readings.csv":        {Data: []byte("id,value\na,1\nb,2\n")},
		"doc/entries/one.kan": {Data: []byte("# One\n")},
		"doc/entries/two.kan": {Data: []byte("# Two\n")},
		"doc.kan": {Data: []byte("---\ntype: Notebook\n---\n# Notebook\n\nphoto: [[/photo.png]]\n\n## Readings\n\n[[readings.csv]]\n\n" +
			"## Entries\n\n- [[doc/entries/one]]\n- [[doc/entries/two]]\n")},
	}
	d, r := LoadWith("schema.kanonarch", "doc.kan", Options{Today: time.Now(), FS: mem})
	check(t, r, nil)
	checkFiles(t, r, "schema.kanonarch", "doc.kan", "photo.png", "readings.csv", "doc/entries/one.kan", "doc/entries/two.kan")
	var ids []string
	for _, f := range d.Root.Fields {
		for _, o := range f.Objects {
			ids = append(ids, o.ID)
		}
	}
	if want := []string{"a", "b", "one", "two"}; !slices.Equal(ids, want) {
		t.Errorf("objects %q, want %q", ids, want)
	}

	mem["doc.kan"] = &fstest.MapFile{Data: []byte("---\ntype: Notebook\n---\n# Notebook\n\nphoto: [[/photos.png]]\nsee: [[broken]]\n\n" +
		"## Readings\n\n[[readings.xlsx]]\n")}
	mem["readings.xlsx"] = &fstest.MapFile{Data: []byte("not a zip")}
	mem["broken.kan"] = &fstest.MapFile{}
	_, r = LoadWith("schema.kanonarch", "doc.kan", Options{FS: brokenFS{mem}})
	check(t, r, []string{
		"broken.kan: error: cannot read the file: the disk is gone",
		"doc.kan:6:8: error: no file `/photos.png`; did you mean `/photo.png`?",
		"readings.xlsx: error: not an xlsx workbook: save it from Excel or LibreOffice as .xlsx",
	})
}
