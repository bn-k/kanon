package testcases

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// walked returns the paths Walk passes to its function under root, relative to root and with forward slashes.
func walked(t *testing.T, root string) []string {
	t.Helper()
	var got []string
	err := Walk(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		got = append(got, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// write writes text to the file name under dir, and the folders it is in.
func write(t *testing.T, dir, name, text string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWalk(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".gitignore", "# local data\n/cases/secret/\n  /cases/notes\n/other/kept/\ncases/kept/\n")
	write(t, dir, "cases/secret/a.kan", "")
	write(t, dir, "cases/notes/b.kan", "")
	write(t, dir, "cases/kept/c.kan", "")
	write(t, dir, "cases/kept/secret/d.kan", "")
	root := filepath.Join(dir, "cases")
	if err := os.Symlink(filepath.Join(root, "kept"), filepath.Join(root, "kept", "link")); err != nil {
		t.Log("no symbolic link:", err)
	}
	want := []string{".", "kept", "kept/c.kan", "kept/secret", "kept/secret/d.kan"}
	if got := walked(t, root); !slices.Equal(got, want) {
		t.Errorf("Walk = %q, want %q", got, want)
	}
}

func TestWalkWithoutGitignore(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "cases/secret/a.kan", "")
	want := []string{".", "secret", "secret/a.kan"}
	if got := walked(t, filepath.Join(dir, "cases")); !slices.Equal(got, want) {
		t.Errorf("Walk = %q, want %q", got, want)
	}
}

func TestWalkErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".gitignore"), 0o755); err != nil {
		t.Fatal(err)
	}
	called := false
	err := Walk(filepath.Join(dir, "cases"), func(string, fs.DirEntry, error) error {
		called = true
		return nil
	})
	if err == nil || called {
		t.Errorf("Walk with a folder .gitignore = %v, called %v; want an error and no call", err, called)
	}

	missing := filepath.Join(t.TempDir(), "cases")
	err = Walk(missing, func(path string, d fs.DirEntry, err error) error { return err })
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Walk of a missing folder = %v, want %v", err, fs.ErrNotExist)
	}
}

func TestWalkCases(t *testing.T) {
	n := 0
	err := Walk("../../cases", func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.Name() == "expected.txt" {
			n++
		}
		return err
	})
	if err != nil || n == 0 {
		t.Errorf("Walk of cases/: %v, %d expected.txt", err, n)
	}
}
