package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

// client talks to a server as an editor does.
type client struct {
	t     *testing.T
	in    *io.PipeWriter
	msgs  chan map[string]json.RawMessage
	done  chan error
	id    int
	diags map[string][]diagnostic // the last diagnostics published, by URI
	logs  []string
}

type diagnostic struct {
	Range    span   `json:"range"`
	Severity int    `json:"severity"`
	Message  string `json:"message"`
}

func start(t *testing.T, delay time.Duration) *client {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &client{
		t: t, in: inW, msgs: make(chan map[string]json.RawMessage, 100), done: make(chan error, 1),
		diags: map[string][]diagnostic{},
	}
	go func() {
		c.done <- serve(inR, outW, delay)
		outW.Close()
	}()
	go func() {
		r := bufio.NewReader(outR)
		for {
			body, err := readBody(r)
			if err != nil {
				close(c.msgs)
				return
			}
			var m map[string]json.RawMessage
			if err := json.Unmarshal(body, &m); err != nil {
				panic(err)
			}
			c.msgs <- m
		}
	}()
	t.Cleanup(func() {
		inW.Close()
	})
	return c
}

func (c *client) send(v map[string]any) {
	c.t.Helper()
	v["jsonrpc"] = "2.0"
	body, err := json.Marshal(v)
	if err != nil {
		c.t.Fatal(err)
	}
	c.sendRaw(string(body))
}

func (c *client) sendRaw(body string) {
	c.t.Helper()
	if _, err := fmt.Fprintf(c.in, "Content-Length: %d\r\n\r\n%s", len(body), body); err != nil {
		c.t.Fatal(err)
	}
}

func (c *client) notify(method string, params any) {
	c.t.Helper()
	c.send(map[string]any{"method": method, "params": params})
}

// next returns the next message of the server, taking note of the notifications.
func (c *client) next() map[string]json.RawMessage {
	c.t.Helper()
	select {
	case m, ok := <-c.msgs:
		if !ok {
			c.t.Fatal("the server stopped")
		}
		var method string
		json.Unmarshal(m["method"], &method)
		switch method {
		case "textDocument/publishDiagnostics":
			var p struct {
				URI         string       `json:"uri"`
				Diagnostics []diagnostic `json:"diagnostics"`
			}
			if err := json.Unmarshal(m["params"], &p); err != nil {
				c.t.Fatal(err)
			}
			c.diags[p.URI] = p.Diagnostics
		case "window/logMessage":
			var p struct{ Message string }
			if err := json.Unmarshal(m["params"], &p); err != nil {
				c.t.Fatal(err)
			}
			c.logs = append(c.logs, p.Message)
		}
		return m
	case <-time.After(10 * time.Second):
		c.t.Fatal("no answer from the server")
		return nil
	}
}

// call sends a request and returns its result, or its error.
func (c *client) call(method string, params any) (json.RawMessage, *rpcError) {
	c.t.Helper()
	c.id++
	c.send(map[string]any{"id": c.id, "method": method, "params": params})
	return c.answer(fmt.Sprint(c.id))
}

// answer reads messages up to the answer to the request id.
func (c *client) answer(id string) (json.RawMessage, *rpcError) {
	c.t.Helper()
	for {
		m := c.next()
		if string(m["id"]) != id {
			continue
		}
		if e, ok := m["error"]; ok {
			var re rpcError
			if err := json.Unmarshal(e, &re); err != nil {
				c.t.Fatal(err)
			}
			return nil, &re
		}
		return m["result"], nil
	}
}

// result calls method and reads its result into a T.
func result[T any](c *client, method string, params any) T {
	c.t.Helper()
	raw, re := c.call(method, params)
	if re != nil {
		c.t.Fatalf("%s: %v", method, re)
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		c.t.Fatalf("%s: %v in %s", method, err, raw)
	}
	return v
}

func (c *client) open(path, text string) {
	c.t.Helper()
	c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": toURI(path), "text": text}})
}

func (c *client) exit() {
	c.t.Helper()
	c.notify("exit", nil)
	if err := <-c.done; err != nil {
		c.t.Fatal(err)
	}
}

// at returns the params of a request at the first needle in text, or the nth when n is given: the needle starts
// with the character it points at.
func at(t *testing.T, path, text, needle string, n ...int) map[string]any {
	t.Helper()
	return map[string]any{
		"textDocument": map[string]any{"uri": toURI(path)},
		"position":     where(t, text, needle, n...),
		"context":      map[string]any{"includeDeclaration": true},
	}
}

// where returns the position of the first needle in text, or the nth when n is given.
func where(t *testing.T, text, needle string, n ...int) position {
	t.Helper()
	i := -1
	for range 1 + slices.Max(append(n, 0)) {
		j := strings.Index(text[i+1:], needle)
		if j < 0 {
			t.Fatalf("no %q in %q", needle, text)
		}
		i += 1 + j
	}
	before := text[:i]
	start := strings.LastIndex(before, "\n") + 1
	return position{strings.Count(before, "\n"), len(utf16.Encode([]rune(before[start:])))}
}

// writeFiles writes files into dir, by their slash-separated path.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, text := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const quoteSchema = `enum State { new, worn }

schema Client {
  name: Text
}

schema Line reads "{name} {state}" {
  name:  Text
  state: State
}

schema Doc {
  title:  Text @heading
  client: -> Client
  state:  State
  lines:  [Line]
  notes:  Prose?
}

rule brand-new(d: Doc) -> Bool = d.state == new
`

const quote = `---
type: Doc
---
# Quote
client: [[clients/acme]]
state: new

## Lines

- screw worn {#l1}

## Notes

See 🚗 [[#l1]] and [[schema#brand-new]].
`

// quoteDir writes a quote whose document on disk is not what the editor holds.
func quoteDir(t *testing.T) (dir, schemaPath, docPath string) {
	dir = t.TempDir()
	writeFiles(t, dir, map[string]string{
		"schema.kanonarch": quoteSchema,
		"doc.kan":          "---\ntype: Doc\n---\n# Empty\n",
		"clients/acme.kan": "---\ntype: Client\n---\n# Acme\nname: Acme\n",
	})
	return dir, filepath.Join(dir, "schema.kanonarch"), filepath.Join(dir, "doc.kan")
}

func loc(path string, line, from, to int) location {
	return location{toURI(path), span{position{line, from}, position{line, to}}}
}

func TestDefinition(t *testing.T) {
	dir, schemaPath, docPath := quoteDir(t)
	c := start(t, time.Hour)
	result[map[string]any](c, "initialize", map[string]any{})
	c.open(docPath, quote)
	tests := []struct {
		needle string
		n      int
		want   location
	}{
		{"client:", 0, loc(schemaPath, 13, 2, 8)},
		{"Doc\n---", 0, loc(schemaPath, 11, 7, 10)},
		{"\n---\n#", 0, loc(schemaPath, 11, 7, 10)}, // just after the name
		{"acme]]", 0, location{toURI(filepath.Join(dir, "clients", "acme.kan")), span{}}},
		{"new", 0, loc(schemaPath, 0, 13, 16)},
		{"worn {", 0, loc(schemaPath, 0, 18, 22)},
		{"Lines", 0, loc(schemaPath, 15, 2, 7)},
		{"[[#l1]]", 0, loc(docPath, 9, 15, 17)},
		{"#l1]]", 0, loc(docPath, 9, 15, 17)},
		{"[[schema#brand-new]]", 0, loc(schemaPath, 19, 5, 14)},
	}
	for _, tt := range tests {
		for _, method := range []string{"textDocument/definition", "textDocument/declaration"} {
			got := result[location](c, method, at(t, docPath, quote, tt.needle, tt.n))
			if got != tt.want {
				t.Errorf("%s at %q = %v, want %v", method, tt.needle, got, tt.want)
			}
		}
	}
	for _, needle := range []string{"type:", "\n## Lines", "See"} {
		if got, _ := c.call("textDocument/definition", at(t, docPath, quote, needle)); string(got) != "null" {
			t.Errorf("definition at %q = %s, want null", needle, got)
		}
	}
	c.exit()
}

func TestReferences(t *testing.T) {
	_, schemaPath, docPath := quoteDir(t)
	c := start(t, time.Hour)
	c.open(docPath, quote)
	c.open(schemaPath, quoteSchema)
	all := []location{
		loc(docPath, 5, 0, 5),
		loc(schemaPath, 14, 2, 7),
		loc(schemaPath, 19, 35, 40),
	}
	declared := []location{all[0], all[2]}
	refs := func(includeDeclaration bool) []location {
		params := at(t, schemaPath, quoteSchema, "state:", 1)
		params["context"] = map[string]any{"includeDeclaration": includeDeclaration}
		got := result[[]location](c, "textDocument/references", params)
		slices.SortFunc(got, func(a, b location) int { return strings.Compare(fmt.Sprint(a), fmt.Sprint(b)) })
		return got
	}
	if got := refs(true); !slices.Equal(got, all) {
		t.Errorf("references = %v, want %v", got, all)
	}
	if got := refs(false); !slices.Equal(got, declared) {
		t.Errorf("references without the declaration = %v, want %v", got, declared)
	}
	if got := result[[]location](c, "textDocument/references", at(t, schemaPath, quoteSchema, "\n\nschema Client")); len(got) != 0 {
		t.Errorf("references of nothing = %v", got)
	}

	type highlight struct{ Range span }
	got := result[[]highlight](c, "textDocument/documentHighlight", at(t, docPath, quote, "l1}"))
	want := []highlight{{loc(docPath, 9, 15, 17).Range}, {loc(docPath, 13, 7, 14).Range}}
	if !slices.Equal(got, want) {
		t.Errorf("highlights = %v, want %v", got, want)
	}
	c.exit()
}

func TestHover(t *testing.T) {
	_, schemaPath, docPath := quoteDir(t)
	c := start(t, time.Hour)
	c.open(docPath, quote)
	type hover struct {
		Contents struct{ Kind, Value string }
		Range    span
	}
	tests := []struct {
		path, text, needle string
		want               string
		at                 span
	}{
		{docPath, quote, "new", "```kanonarch\nenum State { new, worn }\n```\n\nschema.kanonarch:1", loc(docPath, 5, 7, 10).Range},
		{docPath, quote, "[[clients", "`clients/acme.kan`", loc(docPath, 4, 8, 24).Range},
		{docPath, quote, "l1]]", "```kan\n- screw worn {#l1}\n```", loc(docPath, 13, 7, 14).Range},
		{schemaPath, quoteSchema, "State\n", "```kanonarch\nenum State { new, worn }\n```", loc(schemaPath, 8, 9, 14).Range},
	}
	for _, tt := range tests {
		got := result[hover](c, "textDocument/hover", at(t, tt.path, tt.text, tt.needle))
		if got.Contents.Kind != "markdown" || got.Contents.Value != tt.want || got.Range != tt.at {
			t.Errorf("hover at %q = %+v, want %q at %v", tt.needle, got, tt.want, tt.at)
		}
	}
	if got, _ := c.call("textDocument/hover", at(t, docPath, quote, "See")); string(got) != "null" {
		t.Errorf("hover at nothing = %s, want null", got)
	}
	c.exit()
}

func TestDiagnostics(t *testing.T) {
	dir, _, docPath := quoteDir(t)
	writeFiles(t, dir, map[string]string{"photo.jpg": "\x89PNG\r\n\x1a\n"})
	c := start(t, time.Hour)
	broken := strings.Replace(quote, "state: new", "state: old", 1) + "\n![photo](photo.jpg)\n"
	c.open(docPath, broken)
	result[any](c, "shutdown", nil)
	got := c.diags[toURI(docPath)]
	if len(got) != 2 {
		t.Fatalf("diagnostics = %+v, want 2", got)
	}
	if d := got[0]; d.Severity != 1 || d.Range != loc(docPath, 5, 7, 10).Range || !strings.Contains(d.Message, `"old"`) {
		t.Errorf("diagnostic = %+v, want an error on old", d)
	}
	if d := got[1]; d.Severity != 2 || d.Range != loc(docPath, 15, 0, 19).Range || !strings.Contains(d.Message, "photo.png") {
		t.Errorf("diagnostic = %+v, want a warning on the image", d)
	}

	// An edit that changes nothing publishes nothing; a fix clears the diagnostics.
	c.notify("textDocument/didChange", map[string]any{"textDocument": map[string]any{"uri": toURI(docPath)}, "contentChanges": []any{}})
	c.notify("textDocument/didSave", map[string]any{"textDocument": map[string]any{"uri": toURI(docPath)}})
	delete(c.diags, toURI(docPath))
	result[any](c, "shutdown", nil)
	if got, ok := c.diags[toURI(docPath)]; ok {
		t.Errorf("diagnostics published again: %+v", got)
	}
	c.notify("textDocument/didChange", map[string]any{
		"textDocument": map[string]any{"uri": toURI(docPath)}, "contentChanges": []any{map[string]any{"text": quote}},
	})
	result[any](c, "shutdown", nil)
	if got, ok := c.diags[toURI(docPath)]; !ok || len(got) != 0 {
		t.Errorf("diagnostics after the fix = %+v, %v; want none published", got, ok)
	}

	// Closing a document clears what it showed.
	c.notify("textDocument/didChange", map[string]any{
		"textDocument": map[string]any{"uri": toURI(docPath)}, "contentChanges": []any{map[string]any{"text": broken}},
	})
	result[any](c, "shutdown", nil)
	c.notify("textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": toURI(docPath)}})
	result[any](c, "shutdown", nil)
	if got := c.diags[toURI(docPath)]; len(got) != 0 {
		t.Errorf("diagnostics after closing = %+v", got)
	}
	c.exit()
}

func TestDiagnosticsAfterADelay(t *testing.T) {
	_, _, docPath := quoteDir(t)
	c := start(t, 0)
	c.open(docPath, strings.Replace(quote, "state: new", "state: old", 1))
	for len(c.diags[toURI(docPath)]) == 0 {
		c.next()
	}
	c.exit()
}

func TestSchemaDiscovery(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"a.kanonarch":           "schema Other { name: Text }",
		"b.kanonarch":           "schema Doc { name: Text }",
		"deep/down/doc.kan":     "---\ntype: Doc\n---\nname: x\n",
		"notes.kan":             "name: x\n",
		"lone/x/y/z/orphan.kan": "name: x\n",
	})
	c := start(t, time.Hour)
	doc := filepath.Join(dir, "deep", "down", "doc.kan")
	c.open(doc, "---\ntype: Doc\n---\nname: x\n")
	got := result[location](c, "textDocument/definition", at(t, doc, "---\ntype: Doc\n---\nname: x\n", "name"))
	if want := loc(filepath.Join(dir, "b.kanonarch"), 0, 13, 17); got != want {
		t.Errorf("definition = %v, want %v", got, want)
	}

	// A document without a type, held by no document, is checked against the first schema.
	notes := filepath.Join(dir, "notes.kan")
	c.open(notes, "name: x\n")
	result[any](c, "shutdown", nil)
	if len(c.diags[toURI(notes)]) == 0 {
		t.Errorf("no diagnostics for a document without a type")
	}
	c.exit()

	far := t.TempDir()
	orphan := filepath.Join(far, "orphan.kan")
	writeFiles(t, far, map[string]string{"orphan.kan": "name: x\n"})
	c = start(t, time.Hour)
	c.open(orphan, "name: x\n")
	result[any](c, "shutdown", nil)
	want := []diagnostic{{Severity: 1, Message: "no schema for this document: put a `.kanonarch` file in its folder or a folder above"}}
	if got := c.diags[toURI(orphan)]; !slices.Equal(got, want) {
		t.Errorf("diagnostics = %+v, want %+v", got, want)
	}
	c.exit()
}

func TestFolderDocuments(t *testing.T) {
	car, err := filepath.Abs(filepath.Join("..", "..", "cases", "car"))
	if err != nil {
		t.Fatal(err)
	}
	schemaPath := filepath.Join(car, "schema.kanonarch")
	visit := filepath.Join(car, "hatchback", "services", "2023-09-18-oil-change.kan")
	visitText, err := os.ReadFile(visit)
	if err != nil {
		t.Fatal(err)
	}
	schemaText, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}

	// An entry of a folder is checked through the document that holds the folder.
	c := start(t, time.Hour)
	c.open(visit, string(visitText))
	got := result[location](c, "textDocument/definition", at(t, visit, string(visitText), "mileage"))
	if want := loc(schemaPath, 6, 2, 9); got != want {
		t.Errorf("definition = %v, want %v", got, want)
	}
	c.exit()

	// The names of an open schema lead to the documents of its folder.
	c = start(t, time.Hour)
	c.open(schemaPath, string(schemaText))
	refs := result[[]location](c, "textDocument/references", at(t, schemaPath, string(schemaText), "mileage"))
	if !slices.Contains(refs, loc(visit, 3, 0, 7)) {
		t.Errorf("references = %v, want one in %s", refs, visit)
	}
	c.exit()
}

func TestSymbols(t *testing.T) {
	_, schemaPath, docPath := quoteDir(t)
	c := start(t, time.Hour)
	c.open(docPath, quote)
	outline := func(path string) string {
		var b strings.Builder
		var walk func([]*symbol)
		walk = func(syms []*symbol) {
			for i, s := range syms {
				if i > 0 {
					b.WriteString(" ")
				}
				fmt.Fprintf(&b, "%s:%d", s.Name, s.Kind)
				if s.Detail != "" {
					fmt.Fprintf(&b, "(%s)", s.Detail)
				}
				if len(s.Children) > 0 {
					b.WriteString("{")
					walk(s.Children)
					b.WriteString("}")
				}
			}
		}
		walk(result[[]*symbol](c, "textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": toURI(path)}}))
		return b.String()
	}
	tests := []struct{ path, want string }{
		{schemaPath, "State:10(enum){new:22 worn:22} Client:23(schema){name:8(Text)} Line:23(schema){name:8(Text) state:8(State)} " +
			"Doc:23(schema){title:8(Text) client:8(-> Client) state:8(State) lines:8([Line]) notes:8(Prose?)} brand-new:12(rule)"},
		{docPath, "Quote:15{Lines:15{#l1:20} Notes:15}"},
		{filepath.Join(filepath.Dir(docPath), "photo.png"), ""},
		{filepath.Join(filepath.Dir(docPath), "gone.kan"), ""},
		{filepath.Join(filepath.Dir(docPath), "gone.kanonarch"), ""},
	}
	for _, tt := range tests {
		if got := outline(tt.path); got != tt.want {
			t.Errorf("outline of %s =\n%s\nwant\n%s", filepath.Base(tt.path), got, tt.want)
		}
	}
	syms := result[[]*symbol](c, "textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": toURI(docPath)}})
	if got, want := syms[0].Children[0].Range, (span{position{7, 0}, position{10, 0}}); got != want {
		t.Errorf("range of Lines = %v, want %v", got, want)
	}
	if got, want := syms[0].Range, (span{position{3, 0}, position{14, 0}}); got != want {
		t.Errorf("range of Quote = %v, want %v", got, want)
	}
	syms = result[[]*symbol](c, "textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": toURI(schemaPath)}})
	if got, want := syms[1].Range, (span{position{2, 7}, position{3, 6}}); got != want {
		t.Errorf("range of Client = %v, want %v", got, want)
	}
	units := filepath.Join(filepath.Dir(docPath), "units.kanonarch")
	c.open(units, "unit pinch\n")
	if got, want := outline(units), "pinch:14(unit)"; got != want {
		t.Errorf("outline of units = %s, want %s", got, want)
	}

	type found struct {
		Name, ContainerName string
		Location            location
	}
	got := result[[]found](c, "workspace/symbol", map[string]any{"query": "NEW"})
	want := []found{
		{"new", "State", loc(schemaPath, 0, 13, 16)},
		{"brand-new", "schema.kanonarch", loc(schemaPath, 19, 5, 14)},
	}
	if !slices.Equal(got, want) {
		t.Errorf("workspace symbols = %+v, want %+v", got, want)
	}
	c.exit()
}

func TestProtocol(t *testing.T) {
	c := start(t, time.Hour)
	caps := result[map[string]map[string]any](c, "initialize", map[string]any{})
	if caps["capabilities"]["definitionProvider"] != true || caps["serverInfo"]["name"] != "kan" {
		t.Errorf("capabilities = %v", caps)
	}
	c.notify("initialized", map[string]any{})

	c.sendRaw("{")
	if _, re := c.answer("null"); re == nil || re.Code != parseError {
		t.Errorf("a message that isn't JSON: %v", re)
	}
	errs := []struct {
		method string
		params any
		code   int
	}{
		{"textDocument/rename", map[string]any{}, methodNotFound},
		{"textDocument/hover", "here", invalidParams},
		{"textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": "https://example.com/a.kan"}}, invalidParams},
		{"textDocument/hover", map[string]any{"textDocument": map[string]any{"uri": "%zz"}}, invalidParams},
	}
	for _, e := range errs {
		if _, re := c.call(e.method, e.params); re == nil || re.Code != e.code {
			t.Errorf("%s(%v) = %v, want the code %d", e.method, e.params, re, e.code)
		}
	}

	c.notify("textDocument/didOpen", 3)
	c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": "untitled:1"}})
	result[any](c, "shutdown", nil)
	if len(c.logs) != 2 || !strings.HasPrefix(c.logs[0], "kan: ") {
		t.Errorf("logs = %q", c.logs)
	}
	c.exit()
}

func TestServe(t *testing.T) {
	if err := Serve(strings.NewReader(""), io.Discard); err != nil {
		t.Errorf("Serve = %v", err)
	}
}

func TestTransport(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"Content-Type: x\r\n\r\n{}", "a message without Content-Length"},
		{"Content-Length: x\r\n\r\n", `a message with a bad Content-Length: "x"`},
		{"Content-Length: 10\r\n\r\n{}", "unexpected EOF"},
		{"Content-Length: 2\r\n", "unexpected EOF"},
		{"Content-Len", "unexpected EOF"},
	}
	for _, tt := range tests {
		err := serve(strings.NewReader(tt.in), io.Discard, time.Hour)
		if got := fmt.Sprint(err); (err == nil && tt.want != "") || (err != nil && got != tt.want) {
			t.Errorf("serve(%q) = %v, want %q", tt.in, err, tt.want)
		}
	}
}

// broken fails every write.
type broken struct{}

func (broken) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestWriteFails(t *testing.T) {
	_, _, docPath := quoteDir(t)
	open := fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"text":""}}}`, toURI(docPath))
	messages := map[string][]string{
		"answer":       {`{"jsonrpc":"2.0","id":1,"method":"shutdown"}`},
		"diagnostics":  {open, `{"jsonrpc":"2.0","id":1,"method":"shutdown"}`},
		"log":          {`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":3}`},
		"bad response": {`{`},
	}
	for name, msgs := range messages {
		var in strings.Builder
		for _, m := range msgs {
			fmt.Fprintf(&in, "Content-Length: %d\r\n\r\n%s", len(m), m)
		}
		if err := serve(strings.NewReader(in.String()), broken{}, time.Hour); fmt.Sprint(err) != "broken pipe" {
			t.Errorf("%s: serve = %v, want broken pipe", name, err)
		}
	}

	// Diagnostics published after the delay fail too.
	inR, inW := io.Pipe()
	defer inW.Close()
	done := make(chan error, 1)
	go func() { done <- serve(inR, broken{}, 0) }()
	fmt.Fprintf(inW, "Content-Length: %d\r\n\r\n%s", len(open), open)
	if err := <-done; fmt.Sprint(err) != "broken pipe" {
		t.Errorf("serve = %v, want broken pipe", err)
	}
}

func TestColumns(t *testing.T) {
	line := "é🚗x"
	for col, want := range map[int]int{1: 0, 2: 1, 3: 3, 4: 4, 9: 4} {
		if got := utf16Col(line, col); got != want {
			t.Errorf("utf16Col(%q, %d) = %d, want %d", line, col, got, want)
		}
	}
	for character, want := range map[int]int{0: 1, 1: 2, 3: 3, 4: 4, 9: 4} {
		if got := runeCol(line, character); got != want {
			t.Errorf("runeCol(%q, %d) = %d, want %d", line, character, got, want)
		}
	}
}
