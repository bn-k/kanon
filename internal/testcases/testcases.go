// Package testcases walks the folder cases/ for the tests and the fuzz seeds of the other packages: the folders of
// the repository only. A folder that .gitignore names under cases/ stays out: it may hold data that must stay local,
// and a fuzz target writes a failing input, made from its seeds, to a testdata/ folder that is committed.
package testcases

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Walk walks root, a folder of the repository such as ../../cases, as filepath.WalkDir does, but for the folders
// that the .gitignore of root's parent names, as /cases/<name>/, and for what is neither a folder nor a regular file,
// such as a symbolic link. Without a .gitignore, no folder is left out.
func Walk(root string, fn fs.WalkDirFunc) error {
	ignore, err := os.ReadFile(filepath.Join(root, "..", ".gitignore"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	ignored := map[string]bool{}
	prefix := "/" + filepath.Base(root) + "/"
	for line := range strings.Lines(string(ignore)) {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), prefix); ok {
			ignored[strings.TrimSuffix(name, "/")] = true
		}
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return fn(path, d, err)
		case d.IsDir() && filepath.Dir(path) == filepath.Clean(root) && ignored[d.Name()]:
			return filepath.SkipDir
		case !d.IsDir() && !d.Type().IsRegular():
			return nil
		}
		return fn(path, d, nil)
	})
}
