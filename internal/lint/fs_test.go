package lint

import (
	"errors"
	"slices"
	"testing"
	"testing/fstest"
	"time"
)

const memSchema = `
schema Carnet {
  titre:   Text @heading
  photo:   Image?
  mesures: [Mesure]?
  entrees: [Entree]?
  voir:    -> Carnet?
}

schema Mesure {
  valeur: Int
}

schema Entree {
  titre: Text @heading
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
		"schema.kanonarch":     {Data: []byte(memSchema)},
		"photo.png":            {Data: []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR")},
		"mesures.csv":          {Data: []byte("id,valeur\na,1\nb,2\n")},
		"doc/entrees/un.kan":   {Data: []byte("# Un\n")},
		"doc/entrees/deux.kan": {Data: []byte("# Deux\n")},
		"doc.kan": {Data: []byte("---\ntype: Carnet\n---\n# Carnet\n\nphoto: [[/photo.png]]\n\n## Mesures\n\n[[mesures.csv]]\n\n" +
			"## Entrees\n\n- [[doc/entrees/deux]]\n- [[doc/entrees/un]]\n")},
	}
	d, r := LoadWith("schema.kanonarch", "doc.kan", Options{Today: time.Now(), FS: mem})
	check(t, r, nil)
	checkFiles(t, r, "schema.kanonarch", "doc.kan", "photo.png", "mesures.csv", "doc/entrees/deux.kan", "doc/entrees/un.kan")
	var ids []string
	for _, f := range d.Root.Fields {
		for _, o := range f.Objects {
			ids = append(ids, o.ID)
		}
	}
	if want := []string{"a", "b", "deux", "un"}; !slices.Equal(ids, want) {
		t.Errorf("objects %q, want %q", ids, want)
	}

	mem["doc.kan"] = &fstest.MapFile{Data: []byte("---\ntype: Carnet\n---\n# Carnet\n\nphoto: [[/photos.png]]\nvoir: [[broken]]\n\n" +
		"## Mesures\n\n[[mesures.xlsx]]\n")}
	mem["mesures.xlsx"] = &fstest.MapFile{Data: []byte("not a zip")}
	mem["broken.kan"] = &fstest.MapFile{}
	_, r = LoadWith("schema.kanonarch", "doc.kan", Options{FS: brokenFS{mem}})
	check(t, r, []string{
		"broken.kan: error: cannot read the file: the disk is gone",
		"doc.kan:6:8: error: no file `/photos.png`; did you mean `/photo.png`?",
		"mesures.xlsx: error: not an xlsx workbook: save it from Excel or LibreOffice as .xlsx",
	})
}
