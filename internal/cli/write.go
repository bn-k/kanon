package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/lint"
)

// writeOptions are the arguments of a write command.
type writeOptions struct {
	schema string
	docs   []string          // the documents to write, in the order given
	texts  map[string][]byte // their new texts, by absolute path
	today  time.Time
	dry    bool // -n: check, write nothing
	json   bool // -format json
}

// runWrite lints the new texts of documents in their place, with every document of the schema's folder that links
// to one of them, and writes them only when that finds no error: none in the new texts, and none in a document that
// links to them which it didn't have before. A folder of documents that was valid stays valid. With -n it writes
// nothing.
func runWrite(args []string, stdout, stderr io.Writer) int {
	o, err := writeArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "kan: %v\n%s\n", err, usage)
		return 2
	}
	diags, dependents := checkWrite(o)
	errs := diags.Count(diag.Error)
	var written []string
	if errs == 0 && !o.dry {
		for _, path := range o.docs {
			if err := writeFile(path, o.texts[abs(path)]); err != nil {
				fmt.Fprintf(stderr, "kan: %v\n", err)
				return 1
			}
			written = append(written, path)
		}
	}
	if o.json {
		if err := writeJSON(stdout, errs == 0, written, dependents, diags); err != nil {
			fmt.Fprintf(stderr, "kan: %v\n", err)
			return 1
		}
	} else {
		for _, d := range diags {
			fmt.Fprintln(stdout, d)
		}
		if warns := diags.Count(diag.Warning); errs+warns > 0 {
			fmt.Fprintln(stdout, summary(errs, warns))
		}
		if errs == 0 {
			for _, path := range o.docs {
				if o.dry {
					fmt.Fprintf(stdout, "ok  %s\n", path)
				} else {
					fmt.Fprintf(stdout, "wrote %s\n", path)
				}
			}
			for _, path := range dependents {
				fmt.Fprintf(stdout, "ok  %s\n", path)
			}
		}
	}
	if errs > 0 {
		return 1
	}
	return 0
}

// checkWrite lints the new texts of o, then the documents of the schema's folder that link to them, and returns
// what it found, and those documents.
func checkWrite(o writeOptions) (diag.List, []string) {
	read := func(path string) ([]byte, error) {
		if text, ok := o.texts[abs(path)]; ok {
			return text, nil
		}
		return os.ReadFile(path)
	}
	after := lint.Options{Today: o.today, ReadFile: read}
	var diags diag.List
	seen := map[string]bool{}
	add := func(ds diag.List, skip map[string]bool) {
		for _, d := range ds {
			if s := d.String(); !seen[s] && !skip[s] {
				seen[s] = true
				diags = append(diags, d)
			}
		}
	}
	for _, path := range o.docs {
		_, r := lint.LoadWith(o.schema, path, after)
		add(r.Diags, nil)
	}
	var dependents []string
	for _, path := range linking(o) {
		_, r := lint.LoadWith(o.schema, path, after)
		if !slices.ContainsFunc(r.Files, func(f string) bool { _, ok := o.texts[abs(f)]; return ok }) {
			continue // it names a document, but its links lead elsewhere
		}
		dependents = append(dependents, path)
		if r.Diags.Count(diag.Error) == 0 {
			continue
		}
		_, before := lint.LoadWith(o.schema, path, lint.Options{Today: o.today})
		known := map[string]bool{}
		for _, d := range before.Diags {
			known[d.String()] = true
		}
		add(r.Diags, known)
	}
	diags.Sort()
	return diags, dependents
}

// linking returns the documents of the schema's folder, other than those o writes, whose text links to a document
// by the name of one of them: those whose lint may change. Hidden folders are left out.
func linking(o writeOptions) []string {
	var stems []string
	for _, path := range o.docs {
		stems = append(stems, strings.TrimSuffix(filepath.Base(path), ".kan"))
	}
	var found []string
	filepath.WalkDir(filepath.Dir(o.schema), func(path string, e fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return nil // an unreadable folder holds nothing to check
		case e.IsDir() && path != filepath.Dir(o.schema) && strings.HasPrefix(e.Name(), "."):
			return filepath.SkipDir
		case e.IsDir() || filepath.Ext(path) != ".kan":
			return nil
		}
		if _, ok := o.texts[abs(path)]; ok {
			return nil
		}
		text, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if typed(string(text)) && strings.Contains(string(text), "[[") &&
			slices.ContainsFunc(stems, func(s string) bool { return strings.Contains(string(text), s) }) {
			found = append(found, path)
		}
		return nil
	})
	return found
}

// typed tells a document whose front matter names its type, which is checked on its own, from an entry of a
// folder, which is checked through the document that owns the folder.
func typed(text string) bool {
	front, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return false
	}
	front, _, ok = strings.Cut(front, "\n---")
	return ok && (strings.HasPrefix(front, "type:") || strings.Contains(front, "\ntype:"))
}

// writeFile replaces the file at path by text, through a file of the same folder renamed over it, so that a reader
// sees the old text or the new one, never a part. A new file is readable by all, an old one keeps its mode.
func writeFile(path string, text []byte) error {
	if err := replace(path, text); err != nil {
		return fmt.Errorf("cannot write %s: %v", path, err)
	}
	return nil
}

// replace is writeFile, without the path in its error.
func replace(path string, text []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(dir, ".kan-write-*")
	if err != nil {
		return errors.Unwrap(err)
	}
	_, err = tmp.Write(text)
	err = errors.Join(err, tmp.Close(), os.Chmod(tmp.Name(), mode))
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

// writeJSON prints the outcome of a write for a program: whether it is ok, the documents written and checked, and
// the diagnostics.
func writeJSON(w io.Writer, ok bool, written, checked []string, diags diag.List) error {
	out := struct {
		OK          bool              `json:"ok"`
		Written     []string          `json:"written"`
		Checked     []string          `json:"checked"`
		Diagnostics []diag.Diagnostic `json:"diagnostics"`
	}{ok, nonNil(written), nonNil(checked), nonNil(diags)}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// nonNil is s, or an empty slice for nil: JSON says [] rather than null.
func nonNil[S ~[]E, E any](s S) S {
	if s == nil {
		return S{}
	}
	return s
}

// writeArgs reads the arguments of a write command: -n, -today and -format, the schema, and the documents, each
// with the file of its new text after "=", one of them at most without it, whose text is read from stdin.
func writeArgs(args []string) (writeOptions, error) {
	o := writeOptions{texts: map[string][]byte{}, today: time.Now()}
	var rest []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-n":
			o.dry = true
		case "-format":
			if i++; i == len(args) {
				return o, errors.New("-format takes text or json")
			}
			switch args[i] {
			case "text":
			case "json":
				o.json = true
			default:
				return o, fmt.Errorf("-format is text or json, not %s", args[i])
			}
		case "-today":
			if i++; i == len(args) {
				return o, errors.New("-today takes a date, as in -today 2026-10-22")
			}
			var err error
			if o.today, err = time.ParseInLocation(time.DateOnly, args[i], time.Local); err != nil {
				return o, fmt.Errorf("write -today 2026-10-22, not %q", args[i])
			}
		default:
			rest = append(rest, args[i])
		}
	}
	if len(rest) < 2 {
		return o, fmt.Errorf("write takes a schema and documents, got %s", plural(len(rest), "argument"))
	}
	at := slices.IndexFunc(rest, func(arg string) bool { return strings.HasSuffix(arg, ".kanonarch") })
	switch {
	case at >= 0:
		o.schema = rest[at]
		rest = slices.Delete(rest, at, at+1)
	case !strings.HasSuffix(rest[0], ".kan") && !strings.Contains(rest[0], ".kan="):
		return o, fmt.Errorf("%s is not a schema: a schema file ends in .kanonarch", rest[0])
	default:
		return o, errors.New("write takes a schema: a schema file ends in .kanonarch")
	}
	fromStdin := ""
	for _, arg := range rest {
		path, src, hasSrc := strings.Cut(arg, "=")
		path = filepath.Clean(path)
		if !strings.HasSuffix(path, ".kan") {
			return o, fmt.Errorf("%s is not a document: a document file ends in .kan", path)
		}
		if _, ok := o.texts[abs(path)]; ok {
			return o, fmt.Errorf("%s is written twice", path)
		}
		var text []byte
		var err error
		switch {
		case !hasSrc && fromStdin != "":
			return o, fmt.Errorf("%s and %s both read stdin: give each but one its new text, as in %s=new.txt",
				fromStdin, path, path)
		case !hasSrc:
			fromStdin = path
			if text, err = io.ReadAll(stdin); err != nil {
				return o, fmt.Errorf("cannot read stdin: %v", err)
			}
		default:
			text, err = os.ReadFile(src)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				return o, fmt.Errorf("%s does not exist", src)
			case err != nil:
				return o, fmt.Errorf("cannot read %s: %v", src, errors.Unwrap(err))
			}
		}
		o.docs = append(o.docs, path)
		o.texts[abs(path)] = text
	}
	return o, nil
}

// abs is the absolute form of path, which names a file whichever way it is written.
func abs(path string) string {
	a, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return a
}
