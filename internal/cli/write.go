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
	out := outcome{OK: errs == 0, Written: written, Checked: dependents, Diagnostics: diags}
	if err := out.print(stdout, o.json, o.docs); err != nil {
		fmt.Fprintf(stderr, "kan: %v\n", err)
		return 1
	}
	if errs > 0 {
		return 1
	}
	return 0
}

// outcome is what a write or a set did: whether it found no error, the documents it wrote, the documents it checked
// besides, the new text of the document of a set with -format json, and the diagnostics.
type outcome struct {
	OK          bool              `json:"ok"`
	Written     []string          `json:"written"`
	Checked     []string          `json:"checked"`
	Text        *string           `json:"text,omitempty"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
}

// print prints out as JSON for a program, or else for a person: the diagnostics and their count then, when there is
// no error, each of docs, written or only checked, and each document checked besides.
func (out outcome) print(w io.Writer, json bool, docs []string) error {
	if json {
		out.Written, out.Checked, out.Diagnostics = nonNil(out.Written), nonNil(out.Checked), nonNil(out.Diagnostics)
		return printJSON(w, out)
	}
	diags := diag.List(out.Diagnostics)
	for _, d := range diags {
		fmt.Fprintln(w, d)
	}
	if errs, warns := diags.Count(diag.Error), diags.Count(diag.Warning); errs+warns > 0 {
		fmt.Fprintln(w, summary(errs, warns))
	}
	if !out.OK {
		return nil
	}
	for _, path := range docs {
		if slices.Contains(out.Written, path) {
			fmt.Fprintf(w, "wrote %s\n", path)
		} else {
			fmt.Fprintf(w, "ok  %s\n", path)
		}
	}
	for _, path := range out.Checked {
		fmt.Fprintf(w, "ok  %s\n", path)
	}
	return nil
}

// checkWrite lints the new texts of o, an entry of a folder through the document that owns the folder, then the
// documents of the schema's folder that link to them, and returns what it found, and the documents it checked
// besides the new texts.
func checkWrite(o writeOptions) (diag.List, []string) {
	after := lint.Options{Today: o.today, FS: edited(o.texts)}
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
	var dependents []string
	for _, path := range o.docs {
		root := owner(path, after.FS)
		if root == path {
			_, r := lint.LoadWith(o.schema, path, after)
			add(r.Diags, nil)
			continue
		}
		if slices.Contains(dependents, root) {
			continue
		}
		dependents = append(dependents, root)
		_, r := lint.LoadWith(o.schema, root, after)
		add(r.Diags, known(o, root, r.Diags))
	}
	for _, path := range linking(o) {
		if slices.Contains(dependents, path) {
			continue
		}
		_, r := lint.LoadWith(o.schema, path, after)
		if !slices.ContainsFunc(r.Files, func(f string) bool { _, ok := o.texts[abs(f)]; return ok }) {
			continue // it names a document, but its links lead elsewhere
		}
		dependents = append(dependents, path)
		add(r.Diags, known(o, path, r.Diags))
	}
	diags.Sort()
	return diags, dependents
}

// known returns the diagnostics the document at path had before the write, when the write leaves it with errors:
// they are not the write's doing, unless they are in a document it writes.
func known(o writeOptions, path string, after diag.List) map[string]bool {
	k := map[string]bool{}
	if after.Count(diag.Error) == 0 {
		return k
	}
	_, before := lint.LoadWith(o.schema, path, lint.Options{Today: o.today})
	for _, d := range before.Diags {
		if _, written := o.texts[abs(d.Pos.File)]; !written {
			k[d.String()] = true
		}
	}
	return k
}

// owner returns the document to lint for the document at path: itself when its front matter names its type, or
// else the nearest document that owns a folder it lies in, as board.kan owns board/tasks/brief.kan.
func owner(path string, fsys lint.FS) string {
	if text, _ := fsys.ReadFile(path); typed(string(text)) {
		return path
	}
	for dir := filepath.Dir(path); filepath.Dir(dir) != dir; dir = filepath.Dir(dir) {
		if text, err := fsys.ReadFile(dir + ".kan"); err == nil && typed(string(text)) {
			return dir + ".kan"
		}
	}
	return path
}

// edited is the disk with the new texts of a write in front of it, by absolute path: a new document is in its
// folder, and so is a new folder that holds one.
type edited map[string][]byte

func (e edited) ReadFile(name string) ([]byte, error) {
	if text, ok := e[abs(name)]; ok {
		return text, nil
	}
	return os.ReadFile(name)
}

func (e edited) Stat(name string) (fs.FileInfo, error) {
	if text, ok := e[abs(name)]; ok {
		return entry{name: filepath.Base(name), size: int64(len(text))}, nil
	}
	info, err := os.Stat(name)
	if err != nil && e.holds(abs(name)) {
		return entry{name: filepath.Base(name), dir: true}, nil
	}
	return info, err
}

func (e edited) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := os.ReadDir(name)
	prefix := abs(name) + string(filepath.Separator)
	for path := range e {
		rel, ok := strings.CutPrefix(path, prefix)
		if !ok {
			continue
		}
		first, _, deeper := strings.Cut(rel, string(filepath.Separator))
		if !slices.ContainsFunc(entries, func(d fs.DirEntry) bool { return d.Name() == first }) {
			entries, err = append(entries, entry{name: first, dir: deeper}), nil
		}
	}
	slices.SortFunc(entries, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, err
}

// holds reports whether a new text lies inside the folder at dir, an absolute path.
func (e edited) holds(dir string) bool {
	for path := range e {
		if strings.HasPrefix(path, dir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// entry is a file or a folder that a write adds, as its folder lists it and as Stat describes it.
type entry struct {
	name string
	dir  bool
	size int64
}

func (e entry) Name() string               { return e.name }
func (e entry) IsDir() bool                { return e.dir }
func (e entry) Type() fs.FileMode          { return e.Mode().Type() }
func (e entry) Info() (fs.FileInfo, error) { return e, nil }
func (e entry) Size() int64                { return e.size }
func (e entry) ModTime() time.Time         { return time.Time{} }
func (e entry) Sys() any                   { return nil }

func (e entry) Mode() fs.FileMode {
	if e.dir {
		return fs.ModeDir | 0o755
	}
	return 0o644
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

// printJSON prints v as indented JSON, its strings as they are: <, > and & are not escaped.
func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
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
	var err error
	for i := 0; i < len(args) && err == nil; i++ {
		switch args[i] {
		case "-n":
			o.dry = true
		case "-format":
			i++
			o.json, err = formatArg(args, i)
		case "-today":
			i++
			o.today, err = todayArg(args, i)
		default:
			rest = append(rest, args[i])
		}
	}
	if err != nil {
		return o, err
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
