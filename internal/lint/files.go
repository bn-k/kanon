package lint

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/names"
	"github.com/bn-k/kanon/internal/schema"
	"github.com/bn-k/kanon/internal/value"
)

// sniffed is what a file holds, by its content: a media type such as image/jpeg, or why it can't be read.
type sniffed struct {
	media string
	err   error
}

// media are the types whose extension is checked: a file holding one is named with one of its extensions.
var media = map[string]struct {
	desc string
	exts []string // the first is the one to give
}{
	"image/jpeg":      {"a JPEG image", []string{"jpg", "jpeg"}},
	"image/png":       {"a PNG image", []string{"png"}},
	"image/gif":       {"a GIF image", []string{"gif"}},
	"image/webp":      {"a WebP image", []string{"webp"}},
	"image/bmp":       {"a BMP image", []string{"bmp"}},
	"image/svg+xml":   {"an SVG image", []string{"svg"}},
	"application/pdf": {"a PDF document", []string{"pdf"}},
}

// fileExamples are file names to show in messages, by the type of their field.
var fileExamples = map[value.Kind]string{value.File: "plan.pdf", value.Image: "photo.jpg", value.Code: "script.sh"}

// fileKind reports whether a field of type t holds files: File, Image or Code, or a list of them.
func fileKind(t *schema.Type) bool {
	return t.Native && t.Kind >= value.File
}

// fileLink reports whether a link path names a file: its last segment has an extension other than .kan and
// .kanonarch.
func fileLink(p string) bool {
	ext := path.Ext(p)
	return len(ext) > 1 && ext != ".kan" && ext != ".kanonarch"
}

// fileName returns the name to give a file: segments that are ids, and an extension of lowercase ASCII letters
// and digits.
func fileName(name string) string {
	segs := strings.Split(name, "/")
	for i, seg := range segs {
		switch {
		case seg == "..":
		case i < len(segs)-1:
			segs[i] = names.Slugify(seg)
		default:
			ext := path.Ext(seg)
			segs[i] = names.Slugify(strings.TrimSuffix(seg, ext)) + "." + strings.ReplaceAll(names.Slugify(ext), "-", "")
		}
	}
	return strings.Join(segs, "/")
}

// file checks a file linked to, or held in a folder: name is how it is written, path where it is, and k the type
// of its field: File, Image or Code. It reports whether the file can be read as one.
func (l *linter) file(name, path string, pos diag.Pos, k value.Kind) bool {
	if fix := fileName(name); fix != name {
		l.diags.Errorf(pos, "name the file %s, not \"%s\"", fix, name)
		return false
	}
	got, err := l.sniff(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		l.diags.Errorf(pos, "%s", l.noFile(name, path))
		return false
	case err != nil:
		l.diags.Errorf(pos, "cannot read %s: %v", name, err)
		return false
	case k == value.Image && !strings.HasPrefix(got, "image/"):
		l.diags.Errorf(pos, "%s is not an image: it holds %s", name, got)
		return false
	case k == value.Code && !strings.HasPrefix(got, "text/"):
		l.diags.Errorf(pos, "%s is not source code: it holds %s", name, got)
		return false
	}
	ext := filepath.Ext(name)
	if m, ok := media[got]; ok && !slices.Contains(m.exts, ext[1:]) {
		l.diags.Warnf(pos, "%s holds %s; name it %s.%s", name, m.desc, strings.TrimSuffix(name, ext), m.exts[0])
	}
	return true
}

// fileRef checks a link to a file, as file does, written at pos and width runes wide; a link to a file that can be
// read leads to it.
func (l *linter) fileRef(name, path string, pos diag.Pos, width int, k value.Kind) bool {
	if !l.file(name, path, pos, k) {
		return false
	}
	l.span(pos, width, diag.Pos{File: path})
	return true
}

// sniff returns what the file at path holds, reading it once.
func (l *linter) sniff(path string) (string, error) {
	if s, ok := l.sniffed[path]; ok {
		return s.media, s.err
	}
	got, err := sniff(path, l.fs)
	l.sniffed[path] = sniffed{got, err}
	if err == nil {
		l.files = append(l.files, path)
	}
	return got, err
}

// sniff tells what a file holds from its first 512 bytes, as net/http does, and recognizes SVG images. On the disk
// it reads only them.
func sniff(path string, fsys FS) (string, error) {
	var head []byte
	if fsys == Disk {
		f, err := os.Open(path)
		if err != nil {
			return "", unwrap(err)
		}
		defer f.Close()
		head = make([]byte, 512)
		n, err := io.ReadFull(f, head)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return "", unwrap(err)
		}
		head = head[:n]
	} else {
		data, err := fsys.ReadFile(path)
		if err != nil {
			return "", unwrap(err)
		}
		head = data[:min(len(data), 512)]
	}
	if filepath.Ext(path) == ".svg" && bytes.Contains(head, []byte("<svg")) {
		return "image/svg+xml", nil
	}
	got, _, _ := strings.Cut(http.DetectContentType(head), ";")
	return got, nil
}

// noFile is the message for a link to a file that doesn't exist, with the nearest one in its folder.
func (l *linter) noFile(name, path string) string {
	msg := fmt.Sprintf("no file `%s`", name)
	entries, _ := l.fs.ReadDir(filepath.Dir(path))
	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	if c := names.Closest(filepath.Base(path), files); c != "" {
		msg += fmt.Sprintf("; did you mean `%s`?", strings.TrimSuffix(name, filepath.Base(path))+c)
	}
	return msg
}

// fileEntries checks the files of a folder that holds the array of files f of o, in the order of their names.
func (l *linter) fileEntries(o *object, f *schema.Field, dir string) {
	entries, err := l.fs.ReadDir(dir)
	if err != nil {
		l.diags.Errorf(diag.Pos{File: dir}, "cannot read the folder: %v", unwrap(err))
		return
	}
	for _, e := range entries {
		name := e.Name()
		path := filepath.Join(dir, name)
		pos := diag.Pos{File: path}
		switch {
		case strings.HasPrefix(name, "."):
		case e.IsDir():
			l.diags.Errorf(pos, "`%s` is %s: its folder holds files, not folders", f.Name, kind(f.Type))
		case !fileLink(name):
			l.diags.Errorf(pos, "`%s` is %s: name each file with its extension, as in %s", f.Name, kind(f.Type), fileExamples[f.Type.Kind])
		default:
			l.file(name, path, pos, f.Type.Kind)
			mf := o.put(f, pos)
			mf.Files = append(mf.Files, path)
		}
	}
}
