// Package sqlimport writes a Kanon schema and a tree of documents from a SQLite database: kan import. The rules are
// in docs/data.md, section 4.
package sqlimport

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/func-it/kanon/internal/names"
)

// Result is what an import wrote.
type Result struct {
	Name                      string   // the name of the result, the base name of its folder: northwind
	Notes                     []string // what the import changed or left out
	Schemas, Documents, Files int
}

// Import reads the SQLite database or the .sql script src and writes its schema and documents into dir, which
// must not exist or be empty. It writes nothing when it fails.
func Import(src, dir string) (*Result, error) {
	name := folderName(dir)
	if err := CheckDir(dir); err != nil {
		return nil, err
	}
	db, err := open(src)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tables, notes, err := read(db)
	if err != nil {
		return nil, err
	}
	m, err := build(tables, name)
	if err != nil {
		return nil, err
	}
	res := &Result{Name: name, Schemas: 1}
	for _, t := range m.tables {
		res.Notes = append(res.Notes, t.notes...)
		if t.kind != join {
			res.Schemas++
		}
	}
	res.Notes = append(res.Notes, notes...)
	for _, f := range m.write() {
		path := filepath.Join(dir, filepath.FromSlash(f.path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, f.data, 0o644); err != nil {
			return nil, err
		}
		if strings.HasSuffix(f.path, ".kan") {
			res.Documents++
		} else if f.path != "schema.kanonarch" {
			res.Files++
		}
	}
	return res, nil
}

// CheckDir checks that kan import can write into dir: its name is a name, and it doesn't exist or it is an empty
// folder.
func CheckDir(dir string) error {
	if name := folderName(dir); !names.IsName(name) {
		return fmt.Errorf("name the folder %s, not %q", names.Slugify(name), name)
	}
	info, err := os.Stat(dir)
	switch {
	case os.IsNotExist(err):
		return nil
	case err != nil:
		return err
	case !info.IsDir():
		return fmt.Errorf("%s is not a folder", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("%s is not empty; kan import writes into a new folder", dir)
	}
	return nil
}

// folderName gives the name of the folder dir, "." included: the name of the result.
func folderName(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return filepath.Base(dir)
}
