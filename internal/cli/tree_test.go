package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// output is what kan lint and kan eval print with -format json, read back.
type output struct {
	OK          bool              `json:"ok"`
	Checked     []string          `json:"checked"`
	Diagnostics []json.RawMessage `json:"diagnostics"`
	Documents   []object          `json:"documents"`
}

// runJSON runs kan with args and reads back what it prints.
func runJSON(t *testing.T, args ...string) (output, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr)
	var out output
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil || stderr.Len() > 0 {
		t.Fatalf("Run %q: %v\nstdout %s\nstderr %s", args, err, stdout.String(), stderr.String())
	}
	return out, code
}

// get returns the field of o named name.
func get(t *testing.T, o object, name string) field {
	t.Helper()
	for _, f := range o.Fields {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("%s has no field %s", o.ID, name)
	return field{}
}

// TestEvalJSONFiles checks the fields of the listing case that the JSON cases leave out: files and images, a link
// written relative to its document, a @body, code, an object opened by a heading with an id, and a list of links.
func TestEvalJSONFiles(t *testing.T) {
	t.Chdir("../../cases/listing")
	out, code := runJSON(t, "eval", "-format", "json", "schema.kanonarch", "house-elm-grove.kan")
	if code != 0 || !out.OK || len(out.Documents) != 5 {
		t.Fatalf("eval = %d, ok %v, %d documents", code, out.OK, len(out.Documents))
	}
	house, helen := out.Documents[0], out.Documents[1]
	photos := []any{"house-elm-grove/photos/01-front.jpg", "house-elm-grove/photos/02-living-room.jpg",
		"house-elm-grove/photos/03-garden.png"}
	tests := []struct {
		f          field
		kind       string
		text, val  any
		start, end [2]int
	}{
		{get(t, house, "plan"), "file", "[[/house-elm-grove/ground-floor.pdf]]", "house-elm-grove/ground-floor.pdf",
			[2]int{17, 7}, [2]int{17, 43}},
		{get(t, house, "photos"), "image", []any{"[[/house-elm-grove/photos/01-front.jpg]]",
			"[[/house-elm-grove/photos/02-living-room.jpg]]", "[[/house-elm-grove/photos/03-garden.png]]"}, photos,
			[2]int{}, [2]int{}},
		{get(t, helen, "portrait"), "image", "[[/agents/helen-carter/portrait.jpg]]",
			"agents/helen-carter/portrait.jpg", [2]int{10, 11}, [2]int{10, 40}},
		{get(t, house, "co-agents"), "link", []any{"[[/agents/tom-ashby]]", "[[/agents/chloe-hart]]"},
			[]any{map[string]any{"file": "agents/tom-ashby.kan", "id": "tom-ashby"},
				map[string]any{"file": "agents/chloe-hart.kan", "id": "chloe-hart"}},
			[2]int{38, 1}, [2]int{39, 24}},
		{get(t, house, "virtual-tour"), "code", tour, map[string]any{"lang": "html", "text": tour},
			[2]int{55, 1}, [2]int{57, 4}},
	}
	for _, tt := range tests {
		f := tt.f
		if f.Kind != tt.kind || !reflect.DeepEqual(f.Text, tt.text) || !reflect.DeepEqual(f.Value, tt.val) {
			t.Errorf("%s: kind %s, text %v, value %v", f.Name, f.Kind, f.Text, f.Value)
		}
		if start, end := [2]int{f.Line, f.Col}, [2]int{f.EndLine, f.EndCol}; start != tt.start || end != tt.end {
			t.Errorf("%s at %v-%v, want %v-%v", f.Name, start, end, tt.start, tt.end)
		}
	}
	if d := get(t, house, "description"); d.Kind != "prose" || d.span != (span{19, 1, 28, 88}) {
		t.Errorf("description: kind %s at %v", d.Kind, d.span)
	}
	loc := get(t, house, "location")
	if o, ok := loc.Value.(map[string]any); loc.Text != "orchard-close" || !ok || o["line"] != 30.0 || loc.span != (span{}) {
		t.Errorf("location: text %v, value %v at %v", loc.Text, loc.Value, loc.span)
	}
}

const tour = `<iframe src="https://tour.example/elm-2026-118" width="640" height="360" allowfullscreen></iframe>`

// TestJSONErrors checks that kan lint -format json prints no documents, that kan eval prints none when lint finds
// errors before it evaluates, and that both say so with exit code 1 when they cannot print.
func TestJSONErrors(t *testing.T) {
	tmp := t.TempDir()
	files := map[string]string{
		"s.kanonarch": "schema Root { n: Int }\n",
		"ok.kan":      "---\ntype: Root\n---\n\n# Ok\n\nn: 1\n",
		"bad.kan":     "---\ntype: Root\n---\n\n# Bad\n\nn: many\n",
	}
	for path, src := range files {
		if err := os.WriteFile(filepath.Join(tmp, path), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, ok, bad := filepath.Join(tmp, "s.kanonarch"), filepath.Join(tmp, "ok.kan"), filepath.Join(tmp, "bad.kan")
	var stdout bytes.Buffer
	if code := Run([]string{"lint", "-format", "json", s, ok}, &stdout, &stdout); code != 0 {
		t.Errorf("lint = %d", code)
	}
	want := `{
  "ok": true,
  "checked": [
    "` + s + `",
    "` + ok + `"
  ],
  "diagnostics": []
}
`
	if stdout.String() != want {
		t.Errorf("lint printed\n%s\nwant\n%s", stdout.String(), want)
	}
	out, code := runJSON(t, "eval", "-format", "json", s, bad)
	if code != 1 || out.OK || len(out.Diagnostics) != 1 || out.Documents == nil || len(out.Documents) > 0 {
		t.Errorf("eval = %d, ok %v, %d diagnostics, documents %v", code, out.OK, len(out.Diagnostics), out.Documents)
	}
	for _, cmd := range []string{"lint", "eval"} {
		var stderr bytes.Buffer
		if code := Run([]string{cmd, "-format", "json", s, ok}, failingWriter{}, &stderr); code != 1 ||
			stderr.String() != "kan: broken pipe\n" {
			t.Errorf("%s: Run = %d, stderr %q", cmd, code, stderr.String())
		}
	}
}
