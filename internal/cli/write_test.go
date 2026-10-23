package cli

import (
	"bytes"
	"encoding/json"
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
	for _, name := range []string{"schema.kanonarch", "liste.kan", "critique.kan", "liste-renommee.txt"} {
		src, err := os.ReadFile(filepath.Join("../../cases/write-dependent", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), src, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	return read(t, "liste.kan")
}

// withStdin makes text what kan reads on stdin, for the test.
func withStdin(t *testing.T, text string) {
	t.Helper()
	old := stdin
	stdin = strings.NewReader(text)
	t.Cleanup(func() { stdin = old })
}

const nouveauLivre = "| solaris | Solaris | Stanisław Lem |\n"

func TestWrite(t *testing.T) {
	liste := club(t)
	withStdin(t, liste+nouveauLivre)
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"write", "schema.kanonarch", "liste.kan"}, &stdout, &stderr); code != 0 {
		t.Fatalf("write = %d\n%s%s", code, stdout.String(), stderr.String())
	}
	if want := "wrote liste.kan\nok  critique.kan\n"; stdout.String() != want || stderr.Len() > 0 {
		t.Errorf("write printed %q and %q, want %q", stdout.String(), stderr.String(), want)
	}
	if got := read(t, "liste.kan"); got != liste+nouveauLivre {
		t.Errorf("liste.kan holds\n%s", got)
	}
	if left, _ := filepath.Glob(".kan-write-*"); len(left) > 0 {
		t.Errorf("write left %v", left)
	}
}

func TestWriteDry(t *testing.T) {
	liste := club(t)
	os.WriteFile("nouvelle.txt", []byte(liste+nouveauLivre), 0o644)
	var out bytes.Buffer
	if code := Run([]string{"write", "-n", "liste.kan=nouvelle.txt", "schema.kanonarch"}, &out, &out); code != 0 {
		t.Fatalf("write -n = %d\n%s", code, out.String())
	}
	if want := "ok  liste.kan\nok  critique.kan\n"; out.String() != want {
		t.Errorf("write -n printed %q, want %q", out.String(), want)
	}
	if read(t, "liste.kan") != liste {
		t.Error("write -n changed liste.kan")
	}
}

func TestWriteRefused(t *testing.T) {
	liste := club(t)
	var out bytes.Buffer
	if code := Run([]string{"write", "schema.kanonarch", "liste.kan=liste-renommee.txt"}, &out, &out); code != 1 {
		t.Fatalf("write = %d, want 1\n%s", code, out.String())
	}
	if read(t, "liste.kan") != liste {
		t.Error("a refused write changed liste.kan")
	}
}

// A document of the folder that is already wrong doesn't stop a write, as long as the write adds no error to it.
func TestWriteKeepsKnownErrors(t *testing.T) {
	liste := club(t)
	os.WriteFile("ancienne.kan", []byte("---\ntype: Critique\n---\n\n# Ancienne\n\nlivre: [[liste#hyperion]]\n"), 0o644)
	withStdin(t, liste+nouveauLivre)
	var out bytes.Buffer
	if code := Run([]string{"write", "schema.kanonarch", "liste.kan"}, &out, &out); code != 0 {
		t.Fatalf("write = %d\n%s", code, out.String())
	}
	if want := "wrote liste.kan\nok  ancienne.kan\nok  critique.kan\n"; out.String() != want {
		t.Errorf("write printed %q, want %q", out.String(), want)
	}
}

// Only the documents whose links lead to a written one are checked: a name in the text is not a link, nor a hidden
// folder part of the documents.
func TestWriteDependents(t *testing.T) {
	liste := club(t)
	os.WriteFile("journal.kan", []byte("---\ntype: Critique\n---\n\n# Sur une autre liste\n\nlivre: [[autre#x]]\n"), 0o644)
	os.WriteFile("autre.kan", []byte("---\ntype: Liste\n---\n\n# Autre\n\n## Livres\n\n| id | titre | auteur |\n|---|---|---|\n| x | X | Y |\n"), 0o644)
	os.Mkdir(".brouillons", 0o755)
	os.WriteFile(".brouillons/critique.kan", []byte("---\ntype: Critique\n---\n\nlivre: [[../liste#dune]]\n"), 0o644)
	os.Mkdir("journal", 0o755) // the folder of journal.kan: its entries have no type of their own
	os.WriteFile("journal/semaine.kan", []byte("# Semaine\n\nlivre: [[../liste#dune]]\n"), 0o644)
	os.WriteFile("journal/mois.kan", []byte("---\ntitre: Mois\n---\n\nlivre: [[../liste#dune]]\n"), 0o644)
	os.WriteFile("journal/an.kan", []byte("---\ntitre: An\n"), 0o644)
	withStdin(t, liste+nouveauLivre)
	var out bytes.Buffer
	if code := Run([]string{"write", "-n", "schema.kanonarch", "liste.kan"}, &out, &out); code != 0 {
		t.Fatalf("write -n = %d\n%s", code, out.String())
	}
	if want := "ok  liste.kan\nok  critique.kan\n"; out.String() != want {
		t.Errorf("write -n printed %q, want %q", out.String(), want)
	}
}

// Two documents written at once are checked together: the review may follow the book it links to.
func TestWriteTogether(t *testing.T) {
	club(t)
	critique := strings.Replace(read(t, "critique.kan"), "#dune]]", "#dune-messie]]", 1)
	os.WriteFile("critique.txt", []byte(critique), 0o644)
	var out bytes.Buffer
	code := Run([]string{"write", "schema.kanonarch", "liste.kan=liste-renommee.txt", "critique.kan=critique.txt"}, &out, &out)
	if code != 0 {
		t.Fatalf("write = %d\n%s", code, out.String())
	}
	if want := "wrote liste.kan\nwrote critique.kan\n"; out.String() != want {
		t.Errorf("write printed %q, want %q", out.String(), want)
	}
	if read(t, "critique.kan") != critique {
		t.Error("critique.kan was not written")
	}
}

func TestWriteNew(t *testing.T) {
	club(t)
	withStdin(t, "---\ntype: Critique\n---\n\n# Fondation\n\nlivre: [[../liste#fondation]]\n")
	var out bytes.Buffer
	if code := Run([]string{"write", "schema.kanonarch", "critiques/fondation.kan"}, &out, &out); code != 0 {
		t.Fatalf("write = %d\n%s", code, out.String())
	}
	info, err := os.Stat("critiques/fondation.kan")
	if err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("critiques/fondation.kan: %v, %v", info, err)
	}
}

func TestWriteWarnings(t *testing.T) {
	club(t)
	os.WriteFile("couverture.png", []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00"), 0o644) // a JPEG image
	withStdin(t, read(t, "critique.kan")+"\n![[couverture.png]]\n")
	var out bytes.Buffer
	code := Run([]string{"write", "-n", "schema.kanonarch", "critique.kan"}, &out, &out)
	if code != 0 || !strings.Contains(out.String(), ": warning: ") || !strings.HasSuffix(out.String(), "1 warning\nok  critique.kan\n") {
		t.Errorf("write -n = %d\n%s", code, out.String())
	}
}

func TestWriteJSON(t *testing.T) {
	club(t)
	var out bytes.Buffer
	code := Run([]string{"write", "-format", "json", "schema.kanonarch", "liste.kan=liste-renommee.txt"}, &out, &out)
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
		got.Diagnostics[0]["file"] != "critique.kan" || got.Diagnostics[0]["line"] != 7.0 {
		t.Errorf("write -format json printed\n%s", out.String())
	}

	out.Reset()
	withStdin(t, read(t, "liste.kan"))
	if code := Run([]string{"write", "-format", "json", "-today", "2026-10-23", "schema.kanonarch", "liste.kan"}, &out, &out); code != 0 {
		t.Fatalf("write = %d\n%s", code, out.String())
	}
	want := "{\n  \"ok\": true,\n  \"written\": [\n    \"liste.kan\"\n  ],\n  \"checked\": [\n    \"critique.kan\"\n  ],\n  \"diagnostics\": []\n}\n"
	if out.String() != want {
		t.Errorf("write -format json printed\n%s\nwant\n%s", out.String(), want)
	}
}

func TestWriteFails(t *testing.T) {
	club(t)
	os.Mkdir("verrou", 0o555)
	t.Cleanup(func() { os.Chmod("verrou", 0o755) })
	withStdin(t, "---\ntype: Critique\n---\n\n# Dune\n\nlivre: [[../liste#dune]]\n")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"write", "schema.kanonarch", "verrou/dune.kan"}, &stdout, &stderr); code != 1 {
		t.Fatalf("write = %d, want 1\n%s%s", code, stdout.String(), stderr.String())
	}
	if !strings.HasPrefix(stderr.String(), "kan: cannot write verrou/dune.kan: ") {
		t.Errorf("write printed %q", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	withStdin(t, read(t, "liste.kan"))
	if code := Run([]string{"write", "-format", "json", "schema.kanonarch", "liste.kan"}, failingWriter{}, &stderr); code != 1 {
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
	os.Mkdir("dossier.txt", 0o755)
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"schema.kanonarch"}, "write takes a schema and documents, got 1 argument"},
		{[]string{"liste.kan", "critique.kan"}, "write takes a schema: a schema file ends in .kanonarch"},
		{[]string{"schema.yaml", "liste.kan"}, "schema.yaml is not a schema: a schema file ends in .kanonarch"},
		{[]string{"schema.kanonarch", "liste.md"}, "liste.md is not a document: a document file ends in .kan"},
		{[]string{"schema.kanonarch", "liste.kan=a.txt", "./liste.kan=b.txt"}, "a.txt does not exist"},
		{[]string{"schema.kanonarch", "liste.kan=liste-renommee.txt", "./liste.kan=liste-renommee.txt"},
			"liste.kan is written twice"},
		{[]string{"schema.kanonarch", "liste.kan", "critique.kan"},
			"liste.kan and critique.kan both read stdin: give each but one its new text, as in critique.kan=new.txt"},
		{[]string{"schema.kanonarch", "liste.kan=dossier.txt"}, "cannot read dossier.txt: is a directory"},
		{[]string{"-format"}, "-format takes text or json"},
		{[]string{"-format", "csv", "schema.kanonarch", "liste.kan"}, "-format is text or json, not csv"},
		{[]string{"-format", "text", "schema.kanonarch"}, "write takes a schema and documents, got 1 argument"},
		{[]string{"schema.kanonarch", "liste.kan", "-today"}, "-today takes a date, as in -today 2026-10-22"},
		{[]string{"-today", "23/10/2026", "schema.kanonarch", "liste.kan"}, "write -today 2026-10-22, not \"23/10/2026\""},
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
	Run([]string{"write", "schema.kanonarch", "liste.kan"}, &stderr, &stderr)
	if !strings.HasPrefix(stderr.String(), "kan: cannot read stdin: ") {
		t.Errorf("write printed %q", stderr.String())
	}
}
