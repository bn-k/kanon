package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// club copies the case write-dependent into a new folder, makes it the working directory, and returns the text of
// its list.
func club(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"schema.kanonarch", "list.kan", "review.kan", "list-renamed.txt"} {
		src, err := os.ReadFile(filepath.Join("../../cases/write-dependent", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), src, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	return read(t, "list.kan")
}

// withStdin makes text what kan reads on stdin, for the test.
func withStdin(t *testing.T, text string) {
	t.Helper()
	old := stdin
	stdin = strings.NewReader(text)
	t.Cleanup(func() { stdin = old })
}

const newBook = "| salt-and-iron | Salt and Iron | Ruth Okafor |\n"

func TestWrite(t *testing.T) {
	list := club(t)
	withStdin(t, list+newBook)
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"write", "schema.kanonarch", "list.kan"}, &stdout, &stderr); code != 0 {
		t.Fatalf("write = %d\n%s%s", code, stdout.String(), stderr.String())
	}
	if want := "wrote list.kan\nok  review.kan\n"; stdout.String() != want || stderr.Len() > 0 {
		t.Errorf("write printed %q and %q, want %q", stdout.String(), stderr.String(), want)
	}
	if got := read(t, "list.kan"); got != list+newBook {
		t.Errorf("list.kan holds\n%s", got)
	}
	if left, _ := filepath.Glob(".kan-write-*"); len(left) > 0 {
		t.Errorf("write left %v", left)
	}
}

func TestWriteDry(t *testing.T) {
	list := club(t)
	os.WriteFile("new.txt", []byte(list+newBook), 0o644)
	var out bytes.Buffer
	if code := Run([]string{"write", "-n", "list.kan=new.txt", "schema.kanonarch"}, &out, &out); code != 0 {
		t.Fatalf("write -n = %d\n%s", code, out.String())
	}
	if want := "ok  list.kan\nok  review.kan\n"; out.String() != want {
		t.Errorf("write -n printed %q, want %q", out.String(), want)
	}
	if read(t, "list.kan") != list {
		t.Error("write -n changed list.kan")
	}
}

func TestWriteRefused(t *testing.T) {
	list := club(t)
	var out bytes.Buffer
	if code := Run([]string{"write", "schema.kanonarch", "list.kan=list-renamed.txt"}, &out, &out); code != 1 {
		t.Fatalf("write = %d, want 1\n%s", code, out.String())
	}
	if read(t, "list.kan") != list {
		t.Error("a refused write changed list.kan")
	}
}

// A document of the folder that is already wrong doesn't stop a write, as long as the write adds no error to it.
func TestWriteKeepsKnownErrors(t *testing.T) {
	list := club(t)
	os.WriteFile("old.kan", []byte("---\ntype: Review\n---\n\n# An old review\n\nbook: [[list#lighthouse]]\n"), 0o644)
	withStdin(t, list+newBook)
	var out bytes.Buffer
	if code := Run([]string{"write", "schema.kanonarch", "list.kan"}, &out, &out); code != 0 {
		t.Fatalf("write = %d\n%s", code, out.String())
	}
	if want := "wrote list.kan\nok  old.kan\nok  review.kan\n"; out.String() != want {
		t.Errorf("write printed %q, want %q", out.String(), want)
	}
}

// Only the documents whose links lead to a written one are checked: a name in the text is not a link, nor a hidden
// folder part of the documents.
func TestWriteDependents(t *testing.T) {
	list := club(t)
	os.WriteFile("journal.kan", []byte("---\ntype: Review\n---\n\n# On another list\n\nbook: [[other#x]]\n"), 0o644)
	os.WriteFile("other.kan", []byte("---\ntype: ReadingList\n---\n\n# Other\n\n## Books\n\n| id | title | author |\n|---|---|---|\n| x | X | Y |\n"), 0o644)
	os.Mkdir(".drafts", 0o755)
	os.WriteFile(".drafts/review.kan", []byte("---\ntype: Review\n---\n\nbook: [[../list#tidewater]]\n"), 0o644)
	os.Mkdir("journal", 0o755) // the folder of journal.kan: its entries have no type of their own
	os.WriteFile("journal/week.kan", []byte("# Week\n\nbook: [[../list#tidewater]]\n"), 0o644)
	os.WriteFile("journal/month.kan", []byte("---\ntitle: Month\n---\n\nbook: [[../list#tidewater]]\n"), 0o644)
	os.WriteFile("journal/year.kan", []byte("---\ntitle: Year\n"), 0o644)
	withStdin(t, list+newBook)
	var out bytes.Buffer
	if code := Run([]string{"write", "-n", "schema.kanonarch", "list.kan"}, &out, &out); code != 0 {
		t.Fatalf("write -n = %d\n%s", code, out.String())
	}
	if want := "ok  list.kan\nok  review.kan\n"; out.String() != want {
		t.Errorf("write -n printed %q, want %q", out.String(), want)
	}
}

// Two documents written at once are checked together: the review may follow the book it links to.
func TestWriteTogether(t *testing.T) {
	club(t)
	review := strings.Replace(read(t, "review.kan"), "#tidewater]]", "#tidewater-rising]]", 1)
	os.WriteFile("review.txt", []byte(review), 0o644)
	var out bytes.Buffer
	code := Run([]string{"write", "schema.kanonarch", "list.kan=list-renamed.txt", "review.kan=review.txt"}, &out, &out)
	if code != 0 {
		t.Fatalf("write = %d\n%s", code, out.String())
	}
	if want := "wrote list.kan\nwrote review.kan\n"; out.String() != want {
		t.Errorf("write printed %q, want %q", out.String(), want)
	}
	if read(t, "review.kan") != review {
		t.Error("review.kan was not written")
	}
}

func TestWriteNew(t *testing.T) {
	club(t)
	withStdin(t, "---\ntype: Review\n---\n\n# The Glass Orchard\n\nbook: [[../list#glass-orchard]]\n")
	var out bytes.Buffer
	if code := Run([]string{"write", "schema.kanonarch", "reviews/glass-orchard.kan"}, &out, &out); code != 0 {
		t.Fatalf("write = %d\n%s", code, out.String())
	}
	info, err := os.Stat("reviews/glass-orchard.kan")
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("reviews/glass-orchard.kan: %v, %v", info, err)
	}
}

// board copies the case board into a new folder and makes it the working directory.
func board(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("../../cases/board")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
}

const releaseNotes = "# Release notes\n\nstatus: todo\nowner: [[/board#ana]]\ndue: 2026-10-28\n"

// An entry of a folder has no type of its own: it is checked through the document that owns the folder, with its
// new text in its place, in a folder that may not exist yet.
func TestWriteEntry(t *testing.T) {
	const path = "board/tasks/2026-10-28-release-notes.kan"
	for _, first := range []bool{false, true} {
		t.Run(fmt.Sprintf("first %v", first), func(t *testing.T) {
			board(t)
			if first {
				os.RemoveAll("board")
			}
			withStdin(t, releaseNotes)
			var out bytes.Buffer
			if code := Run([]string{"write", "-today", "2026-10-23", "schema.kanonarch", path}, &out, &out); code != 0 {
				t.Fatalf("write = %d\n%s", code, out.String())
			}
			if want := "wrote " + path + "\nok  board.kan\n"; out.String() != want {
				t.Errorf("write printed %q, want %q", out.String(), want)
			}
			if read(t, path) != releaseNotes {
				t.Errorf("%s was not written", path)
			}
		})
	}

	t.Run("known error", func(t *testing.T) {
		// The errors the owner had before are not the entry's.
		board(t)
		os.WriteFile("board.kan", []byte(strings.Replace(read(t, "board.kan"), "sam@example.com", "", 1)), 0o644)
		withStdin(t, releaseNotes)
		var out bytes.Buffer
		if code := Run([]string{"write", "-n", "-today", "2026-10-23", "schema.kanonarch", path}, &out, &out); code != 0 {
			t.Errorf("write -n beside a known error = %d\n%s", code, out.String())
		}

		// An entry no document owns names no schema.
		withStdin(t, releaseNotes)
		out.Reset()
		if code := Run([]string{"write", "-n", "schema.kanonarch", "notes/release.kan"}, &out, &out); code != 1 ||
			!strings.Contains(out.String(), "notes/release.kan:1:1: error: this document names no schema") {
			t.Errorf("write -n of an entry no document owns = %d\n%s", code, out.String())
		}
	})
}

// edited is the disk with the new texts in front of it.
func TestEdited(t *testing.T) {
	board(t)
	e := edited{abs("board/tasks/new.kan"): []byte(releaseNotes), abs("other/deep/new.kan"): []byte(releaseNotes)}
	if entries, err := e.ReadDir("board/tasks"); err != nil || len(entries) != 4 || entries[2].Name() != "2026-11-06-launch-email.kan" ||
		entries[3].Name() != "new.kan" || entries[3].IsDir() || entries[3].Type() != 0 {
		t.Errorf("ReadDir(board/tasks) = %v, %v", entries, err)
	}
	entries, err := e.ReadDir("other")
	if err != nil || len(entries) != 1 || entries[0].Name() != "deep" || !entries[0].IsDir() || entries[0].Type() != fs.ModeDir {
		t.Fatalf("ReadDir(other) = %v, %v", entries, err)
	}
	info, err := entries[0].Info()
	if err != nil || info.Name() != "deep" || !info.IsDir() || info.Mode() != fs.ModeDir|0o755 || !info.ModTime().IsZero() || info.Sys() != nil {
		t.Errorf("Info of other/deep = %v, %v", info, err)
	}
	if _, err := e.ReadDir("nowhere"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadDir(nowhere) = %v", err)
	}
	if info, err := e.Stat("other/deep/new.kan"); err != nil || info.IsDir() || info.Size() != int64(len(releaseNotes)) || info.Mode() != 0o644 {
		t.Errorf("Stat(other/deep/new.kan) = %v, %v", info, err)
	}
	if info, err := e.Stat("other/deep"); err != nil || !info.IsDir() {
		t.Errorf("Stat(other/deep) = %v, %v", info, err)
	}
	if info, err := e.Stat("board.kan"); err != nil || info.IsDir() {
		t.Errorf("Stat(board.kan) = %v, %v", info, err)
	}
	if _, err := e.Stat("nowhere"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat(nowhere) = %v", err)
	}
	if text, err := e.ReadFile("other/deep/new.kan"); err != nil || string(text) != releaseNotes {
		t.Errorf("ReadFile(other/deep/new.kan) = %q, %v", text, err)
	}
}

func TestWriteWarnings(t *testing.T) {
	club(t)
	os.WriteFile("cover.png", []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00"), 0o644) // a JPEG image
	withStdin(t, read(t, "review.kan")+"\n![[cover.png]]\n")
	var out bytes.Buffer
	code := Run([]string{"write", "-n", "schema.kanonarch", "review.kan"}, &out, &out)
	if code != 0 || !strings.Contains(out.String(), ": warning: ") || !strings.HasSuffix(out.String(), "1 warning\nok  review.kan\n") {
		t.Errorf("write -n = %d\n%s", code, out.String())
	}
}

func TestWriteJSON(t *testing.T) {
	club(t)
	var out bytes.Buffer
	code := Run([]string{"write", "-format", "json", "schema.kanonarch", "list.kan=list-renamed.txt"}, &out, &out)
	if code != 1 {
		t.Fatalf("write = %d, want 1\n%s", code, out.String())
	}
	var got struct {
		OK          bool
		Written     []string
		Checked     []string
		Diagnostics []map[string]any
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if got.OK || len(got.Written) != 0 || len(got.Checked) != 1 || len(got.Diagnostics) != 1 ||
		got.Diagnostics[0]["file"] != "review.kan" || got.Diagnostics[0]["line"] != 7.0 {
		t.Errorf("write -format json printed\n%s", out.String())
	}

	out.Reset()
	withStdin(t, read(t, "list.kan"))
	if code := Run([]string{"write", "-format", "json", "-today", "2026-10-23", "schema.kanonarch", "list.kan"}, &out, &out); code != 0 {
		t.Fatalf("write = %d\n%s", code, out.String())
	}
	want := "{\n  \"ok\": true,\n  \"written\": [\n    \"list.kan\"\n  ],\n  \"checked\": [\n    \"review.kan\"\n  ],\n  \"diagnostics\": []\n}\n"
	if out.String() != want {
		t.Errorf("write -format json printed\n%s\nwant\n%s", out.String(), want)
	}
}

func TestWriteFails(t *testing.T) {
	club(t)
	os.Mkdir("locked", 0o555)
	t.Cleanup(func() { os.Chmod("locked", 0o755) })
	withStdin(t, "---\ntype: Review\n---\n\n# Tidewater\n\nbook: [[../list#tidewater]]\n")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"write", "schema.kanonarch", "locked/tidewater.kan"}, &stdout, &stderr); code != 1 {
		t.Fatalf("write = %d, want 1\n%s%s", code, stdout.String(), stderr.String())
	}
	if !strings.HasPrefix(stderr.String(), "kan: cannot write locked/tidewater.kan: ") {
		t.Errorf("write printed %q", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	withStdin(t, read(t, "list.kan"))
	if code := Run([]string{"write", "-format", "json", "schema.kanonarch", "list.kan"}, failingWriter{}, &stderr); code != 1 {
		t.Errorf("write to a broken pipe = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "broken pipe") {
		t.Errorf("write printed %q", stderr.String())
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, os.ErrClosed }

func TestWriteUsage(t *testing.T) {
	club(t)
	os.Mkdir("folder.txt", 0o755)
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"schema.kanonarch"}, "write takes a schema and documents, got 1 argument"},
		{[]string{"list.kan", "review.kan"}, "write takes a schema: a schema file ends in .kanonarch"},
		{[]string{"schema.yaml", "list.kan"}, "schema.yaml is not a schema: a schema file ends in .kanonarch"},
		{[]string{"schema.kanonarch", "list.md"}, "list.md is not a document: a document file ends in .kan"},
		{[]string{"schema.kanonarch", "list.kan=a.txt", "./list.kan=b.txt"}, "a.txt does not exist"},
		{[]string{"schema.kanonarch", "list.kan=list-renamed.txt", "./list.kan=list-renamed.txt"},
			"list.kan is written twice"},
		{[]string{"schema.kanonarch", "list.kan", "review.kan"},
			"list.kan and review.kan both read stdin: give each but one its new text, as in review.kan=new.txt"},
		{[]string{"schema.kanonarch", "list.kan=folder.txt"}, "cannot read folder.txt: is a directory"},
		{[]string{"-format"}, "-format takes text or json"},
		{[]string{"-format", "csv", "schema.kanonarch", "list.kan"}, "-format is text or json, not csv"},
		{[]string{"-format", "text", "schema.kanonarch"}, "write takes a schema and documents, got 1 argument"},
		{[]string{"schema.kanonarch", "list.kan", "-today"}, "-today takes a date, as in -today 2026-10-22"},
		{[]string{"-today", "23/10/2026", "schema.kanonarch", "list.kan"}, "write -today 2026-10-22, not \"23/10/2026\""},
	}
	for _, tt := range tests {
		withStdin(t, "")
		var stdout, stderr bytes.Buffer
		if code := Run(append([]string{"write"}, tt.args...), &stdout, &stderr); code != 2 {
			t.Errorf("write %q = %d, want 2", tt.args, code)
		}
		if want := "kan: " + tt.want + "\n" + usage + "\n"; stderr.String() != want || stdout.Len() > 0 {
			t.Errorf("write %q printed %q, want %q", tt.args, stderr.String(), want)
		}
	}

	stdin = failingReader{}
	var stderr bytes.Buffer
	Run([]string{"write", "schema.kanonarch", "list.kan"}, &stderr, &stderr)
	if !strings.HasPrefix(stderr.String(), "kan: cannot read stdin: ") {
		t.Errorf("write printed %q", stderr.String())
	}
}
