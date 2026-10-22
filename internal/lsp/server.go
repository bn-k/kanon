package lsp

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/doc"
	"github.com/bn-k/kanon/internal/lint"
	"github.com/bn-k/kanon/internal/schema"
)

// server holds the open files and what checking them found. Every method runs on one goroutine.
type server struct {
	out       io.Writer
	open      map[string]string    // the text of the open files, saved or not, by path
	edits     int                  // counts the changes to the open files
	dirty     bool                 // the files changed since the last analysis
	pending   bool                 // the diagnostics may have changed since they were last published
	published map[string]diag.List // what the editor shows, by file

	// What the last analysis found, and the files it read.
	refs    []schema.Ref
	diags   map[string]diag.List
	lines   map[string][]string
	types   map[string]string
	schemas map[string]loaded
}

// loaded is a schema file as reading it found it.
type loaded struct {
	file  *schema.File
	diags diag.List
}

func newServer(out io.Writer) *server {
	return &server{out: out, open: map[string]string{}, published: map[string]diag.List{}, dirty: true}
}

// handle answers a request, or takes note of a notification.
func (s *server) handle(m *message) error {
	if m.err != nil {
		return write(s.out, failure{"2.0", m.ID, &rpcError{parseError, m.err.Error()}})
	}
	if !m.request() {
		if err := s.notified(m); err != nil {
			return s.log(err)
		}
		return nil
	}
	// Diagnostics go out before any answer, so that the editor never shows older ones than what it is told.
	if err := s.publish(); err != nil {
		return err
	}
	result, err := s.answer(m)
	if err != nil {
		var re *rpcError
		if !errors.As(err, &re) {
			re = &rpcError{internalError, err.Error()}
		}
		return write(s.out, failure{"2.0", m.ID, re})
	}
	return write(s.out, response{"2.0", m.ID, result})
}

// log shows err in the editor's log.
func (s *server) log(err error) error {
	return write(s.out, notification{"2.0", "window/logMessage", map[string]any{"type": 1, "message": "kan: " + err.Error()}})
}

type documentParams struct {
	TextDocument struct {
		URI  string `json:"uri"`
		Text string `json:"text"`
	} `json:"textDocument"`
	ContentChanges []struct {
		Text string `json:"text"`
	} `json:"contentChanges"`
}

// notified takes note of a file opened, changed, saved or closed. The server asks for whole texts, so the last
// change of a didChange holds the text.
func (s *server) notified(m *message) error {
	switch m.Method {
	case "textDocument/didOpen", "textDocument/didChange", "textDocument/didClose", "textDocument/didSave":
	default:
		return nil
	}
	p, err := decode[documentParams](m.Params)
	if err != nil {
		return err
	}
	path, err := toPath(p.TextDocument.URI)
	if err != nil {
		return err
	}
	switch m.Method {
	case "textDocument/didOpen":
		s.open[path] = p.TextDocument.Text
	case "textDocument/didChange":
		if len(p.ContentChanges) > 0 {
			s.open[path] = p.ContentChanges[len(p.ContentChanges)-1].Text
		}
	case "textDocument/didClose":
		delete(s.open, path)
	}
	s.edits++
	s.dirty, s.pending = true, true
	return nil
}

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// span is the Range of the protocol, which Go's range keyword keeps from naming.
type span struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type location struct {
	URI   string `json:"uri"`
	Range span   `json:"range"`
}

type positionParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position position `json:"position"`
	Context  struct {
		IncludeDeclaration bool `json:"includeDeclaration"`
	} `json:"context"`
	Query string `json:"query"`
}

var capabilities = map[string]any{
	"capabilities": map[string]any{
		"positionEncoding":          "utf-16",
		"textDocumentSync":          map[string]any{"openClose": true, "change": 1, "save": true},
		"definitionProvider":        true,
		"declarationProvider":       true,
		"referencesProvider":        true,
		"documentHighlightProvider": true,
		"hoverProvider":             true,
		"documentSymbolProvider":    true,
		"workspaceSymbolProvider":   true,
	},
	"serverInfo": map[string]any{"name": "kan"},
}

func (s *server) answer(m *message) (any, error) {
	switch m.Method {
	case "initialize":
		return capabilities, nil
	case "shutdown":
		return nil, nil
	case "workspace/symbol", "textDocument/documentSymbol", "textDocument/definition", "textDocument/declaration",
		"textDocument/references", "textDocument/documentHighlight", "textDocument/hover":
	default:
		return nil, &rpcError{methodNotFound, "no method " + m.Method}
	}
	p, err := decode[positionParams](m.Params)
	if err != nil {
		return nil, err
	}
	if m.Method == "workspace/symbol" {
		return s.workspaceSymbols(p.Query), nil
	}
	path, err := toPath(p.TextDocument.URI)
	if err != nil {
		return nil, err
	}
	s.analyze()
	if m.Method == "textDocument/documentSymbol" {
		return s.symbols(path), nil
	}
	ref, found := s.at(path, p.Position)
	switch m.Method {
	case "textDocument/definition", "textDocument/declaration":
		if !found {
			return nil, nil
		}
		return location{toURI(ref.Decl.File), s.span(ref.Decl, s.width(ref.Decl))}, nil
	case "textDocument/references":
		locs := []location{}
		for _, r := range s.refs {
			if found && r.Decl == ref.Decl && (p.Context.IncludeDeclaration || r.Pos != r.Decl) {
				locs = append(locs, location{toURI(r.Pos.File), s.span(r.Pos, r.Len)})
			}
		}
		return locs, nil
	case "textDocument/documentHighlight":
		highlights := []map[string]any{}
		for _, r := range s.refs {
			if found && r.Decl == ref.Decl && r.Pos.File == path {
				highlights = append(highlights, map[string]any{"range": s.span(r.Pos, r.Len)})
			}
		}
		return highlights, nil
	default: // textDocument/hover
		if !found {
			return nil, nil
		}
		return map[string]any{
			"contents": map[string]any{"kind": "markdown", "value": s.hover(path, ref.Decl)},
			"range":    s.span(ref.Pos, ref.Len),
		}, nil
	}
}

// at returns the name under p in the file at path; the shortest when names nest. A cursor just after a name is
// on it.
func (s *server) at(path string, p position) (schema.Ref, bool) {
	line := p.Line + 1
	col := runeCol(s.line(path, line), p.Character)
	var at schema.Ref
	found := false
	for _, r := range s.refs {
		if r.Pos.File == path && r.Pos.Line == line && r.Pos.Col <= col && col <= r.Pos.Col+r.Len && (!found || r.Len < at.Len) {
			at, found = r, true
		}
	}
	return at, found
}

// width returns how many runes the name declared at decl takes.
func (s *server) width(decl diag.Pos) int {
	for _, r := range s.refs {
		if r.Pos == decl && r.Decl == decl {
			return r.Len
		}
	}
	return 0
}

// hover tells what is declared at decl, seen from the file at path: the line that declares it, or the file.
func (s *server) hover(path string, decl diag.Pos) string {
	name := decl.File
	if rel, err := filepath.Rel(filepath.Dir(path), decl.File); err == nil {
		name = filepath.ToSlash(rel)
	}
	if decl.Line == 0 {
		return "`" + name + "`"
	}
	lang := strings.TrimPrefix(filepath.Ext(decl.File), ".")
	text := fmt.Sprintf("```%s\n%s\n```", lang, strings.TrimSpace(s.line(decl.File, decl.Line)))
	if decl.File != path {
		text += fmt.Sprintf("\n\n%s:%d", name, decl.Line)
	}
	return text
}

// publish sends the diagnostics of every file whose diagnostics changed, if the files changed since they were sent.
func (s *server) publish() error {
	if !s.pending {
		return nil
	}
	s.pending = false
	s.analyze()
	files := slices.Sorted(maps.Keys(s.diags))
	for file := range s.published {
		if _, ok := s.diags[file]; !ok {
			files = append(files, file)
		}
	}
	for _, file := range files {
		ds := s.diags[file]
		if slices.Equal(ds, s.published[file]) {
			continue
		}
		items := []map[string]any{}
		for _, d := range ds {
			severity := 1
			if d.Severity == diag.Warning {
				severity = 2
			}
			items = append(items, map[string]any{
				"range": s.span(d.Pos, s.extent(d.Pos)), "severity": severity, "source": "kan", "message": d.Msg,
			})
		}
		params := map[string]any{"uri": toURI(file), "diagnostics": items}
		if err := write(s.out, notification{"2.0", "textDocument/publishDiagnostics", params}); err != nil {
			return err
		}
		if len(ds) == 0 {
			delete(s.published, file)
		} else {
			s.published[file] = ds
		}
	}
	return nil
}

// extent returns how many runes a diagnostic at pos underlines: the name written there, or else the word.
func (s *server) extent(pos diag.Pos) int {
	n := 0
	for _, r := range s.refs {
		if r.Pos == pos {
			n = max(n, r.Len)
		}
	}
	if n > 0 || pos.Line == 0 {
		return n
	}
	rest := []rune(s.text(pos, utf8.RuneCountInString(s.line(pos.File, pos.Line))))
	if i := slices.IndexFunc(rest, unicode.IsSpace); i >= 0 {
		return i
	}
	return len(rest)
}

// text returns the n runes at pos, or fewer where its line ends.
func (s *server) text(pos diag.Pos, n int) string {
	line := []rune(s.line(pos.File, pos.Line))
	start := min(max(pos.Col-1, 0), len(line))
	return string(line[start:min(start+n, len(line))])
}

// analyze checks the open files, if they changed: each schema, and each document with its schema. A document
// without a type in its front matter is checked through the document that holds it.
func (s *server) analyze() {
	if !s.dirty {
		return
	}
	s.dirty = false
	s.refs = nil
	s.diags = map[string]diag.List{}
	s.lines = map[string][]string{}
	s.types = map[string]string{}
	s.schemas = map[string]loaded{}
	seenRefs := map[schema.Ref]bool{}
	seenDiags := map[diag.Diagnostic]bool{}
	add := func(ds diag.List, refs []schema.Ref) {
		for _, d := range ds {
			if !seenDiags[d] {
				seenDiags[d] = true
				s.diags[d.Pos.File] = append(s.diags[d.Pos.File], d)
			}
		}
		for _, r := range refs {
			if !seenRefs[r] {
				seenRefs[r] = true
				s.refs = append(s.refs, r)
			}
		}
	}
	type check struct{ schema, doc string }
	var checks []check
	queue := func(c check) {
		if !slices.Contains(checks, c) {
			checks = append(checks, c)
		}
	}
	for _, path := range slices.Sorted(maps.Keys(s.open)) {
		switch filepath.Ext(path) {
		case ".kanonarch":
			f, ds := s.schema(path)
			var refs []schema.Ref
			if f != nil {
				refs = f.Refs
			}
			add(ds, refs)
			for _, d := range s.documentsOf(path) {
				queue(check{path, d})
			}
		case ".kan":
			root := s.root(path)
			if sch := s.schemaOf(root); sch != "" {
				queue(check{sch, root})
				continue
			}
			var ds diag.List
			ds.Errorf(diag.Pos{File: path}, "no schema for this document: put a `.kanonarch` file in its folder or a folder above")
			add(ds, nil)
		}
	}
	today := time.Now()
	for _, c := range checks {
		_, r := lint.LoadWith(c.schema, c.doc, lint.Options{Today: today, ReadFile: s.read})
		add(r.Diags, r.Refs)
	}
	for _, ds := range s.diags {
		ds.Sort()
	}
}

// read returns the text of the file at path: what the editor holds when it is open.
func (s *server) read(path string) ([]byte, error) {
	if text, ok := s.open[path]; ok {
		return []byte(text), nil
	}
	return os.ReadFile(path)
}

// line returns the line n, from 1, of the file at path, or "" when there is no such line.
func (s *server) line(path string, n int) string {
	lines, ok := s.lines[path]
	if !ok {
		src, err := s.read(path)
		if err == nil {
			lines = strings.Split(string(src), "\n")
		}
		s.lines[path] = lines
	}
	if n < 1 || n > len(lines) {
		return ""
	}
	return strings.TrimSuffix(lines[n-1], "\r")
}

// schema reads the schema file at path once per analysis.
func (s *server) schema(path string) (*schema.File, diag.List) {
	l, ok := s.schemas[path]
	if !ok {
		l.file, l.diags = schema.LoadFrom(path, s.read)
		s.schemas[path] = l
	}
	return l.file, l.diags
}

// typeOf returns the type the document at path gives in its front matter, or "".
func (s *server) typeOf(path string) string {
	if t, ok := s.types[path]; ok {
		return t
	}
	t := ""
	if src, err := s.read(path); err == nil {
		d, _ := doc.Parse(path, string(src)) // the shape is checked with the document
		for _, k := range d.FrontMatter {
			if k.Key == "type" {
				t = k.Value
			}
		}
	}
	s.types[path] = t
	return t
}

// root returns the document to check for the document at path: itself when it has a type, or else the document
// that holds the folder it lies in, as clio.kan holds clio/entretiens/vidange.kan.
func (s *server) root(path string) string {
	if s.typeOf(path) != "" {
		return path
	}
	for dir := filepath.Dir(path); filepath.Dir(dir) != dir; dir = filepath.Dir(dir) {
		if owner := dir + ".kan"; s.typeOf(owner) != "" {
			return owner
		}
	}
	return path
}

// schemaOf returns the schema of the document at path: in the nearest folder, its own or one above, that holds
// .kanonarch files, the first that declares the document's type, or else the first. It returns "" when there is
// none.
func (s *server) schemaOf(path string) string {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		var found []string
		entries, _ := os.ReadDir(dir) // a folder that can't be read holds no schema
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".kanonarch" {
				found = append(found, filepath.Join(dir, e.Name()))
			}
		}
		for _, f := range found {
			if sch, _ := s.schema(f); sch != nil && sch.Schema(s.typeOf(path)) != nil {
				return f
			}
		}
		if len(found) > 0 {
			return found[0]
		}
		if filepath.Dir(dir) == dir {
			return ""
		}
	}
}

// documentsOf returns the documents of the folder of the schema at path that it is the schema of, so that its
// names lead to them.
func (s *server) documentsOf(path string) []string {
	var docs []string
	entries, _ := os.ReadDir(filepath.Dir(path)) // the folder of an open file; at worst no document leads there
	for _, e := range entries {
		d := filepath.Join(filepath.Dir(path), e.Name())
		if !e.IsDir() && filepath.Ext(d) == ".kan" && s.typeOf(d) != "" && s.schemaOf(d) == path {
			docs = append(docs, d)
		}
	}
	return docs
}

// span returns the range of the width runes at pos; a whole file is its start.
func (s *server) span(pos diag.Pos, width int) span {
	if pos.Line == 0 {
		return span{}
	}
	line := s.line(pos.File, pos.Line)
	return span{
		position{pos.Line - 1, utf16Col(line, pos.Col)},
		position{pos.Line - 1, utf16Col(line, pos.Col+width)},
	}
}

// utf16Col returns how many UTF-16 units come before the rune column col, from 1, of line.
func utf16Col(line string, col int) int {
	n := 0
	for i, r := range []rune(line) {
		if i+1 >= col {
			break
		}
		n += utf16.RuneLen(r)
	}
	return n
}

// runeCol returns the rune column, from 1, at the UTF-16 offset character of line.
func runeCol(line string, character int) int {
	col, n := 1, 0
	for _, r := range line {
		if n >= character {
			break
		}
		n += utf16.RuneLen(r)
		col++
	}
	return col
}

// symbol is a DocumentSymbol of the protocol.
type symbol struct {
	Name           string    `json:"name"`
	Detail         string    `json:"detail,omitempty"`
	Kind           int       `json:"kind"`
	Range          span      `json:"range"`
	SelectionRange span      `json:"selectionRange"`
	Children       []*symbol `json:"children,omitempty"`
}

// The kinds of symbols of the protocol.
const (
	kindField      = 8
	kindEnum       = 10
	kindFunction   = 12
	kindConstant   = 14
	kindString     = 15
	kindKey        = 20
	kindEnumMember = 22
	kindStruct     = 23
)

// symbols returns the outline of the file at path: the declarations of a schema, the sections and ids of a
// document.
func (s *server) symbols(path string) []*symbol {
	switch filepath.Ext(path) {
	case ".kanonarch":
		return s.schemaSymbols(path)
	case ".kan":
		return s.documentSymbols(path)
	}
	return []*symbol{}
}

func (s *server) schemaSymbols(path string) []*symbol {
	syms := []*symbol{}
	f, _ := s.schema(path) // its diagnostics are published
	if f == nil {
		return syms
	}
	named := func(name string, pos diag.Pos, kind int, detail string) *symbol {
		r := s.span(pos, utf8.RuneCountInString(name))
		return &symbol{Name: name, Detail: detail, Kind: kind, Range: r, SelectionRange: r}
	}
	for _, u := range f.Units {
		syms = append(syms, named(u.Name, u.Pos, kindConstant, "unit"))
	}
	for _, e := range f.Enums {
		sym := named(e.Name, e.Pos, kindEnum, "enum")
		for _, v := range e.Values {
			sym.Children = append(sym.Children, named(v.Name, v.Pos, kindEnumMember, ""))
		}
		syms = append(syms, enclose(sym))
	}
	for _, sc := range f.Schemas {
		sym := named(sc.Name, sc.Pos, kindStruct, "schema")
		for _, fd := range sc.Fields {
			sym.Children = append(sym.Children, named(fd.Name, fd.Pos, kindField, fd.Type.String()))
		}
		syms = append(syms, enclose(sym))
	}
	for _, fn := range f.Fns {
		detail := "fn"
		if fn.Rule {
			detail = "rule"
		}
		syms = append(syms, named(fn.Name, fn.Pos, kindFunction, detail))
	}
	slices.SortStableFunc(syms, func(a, b *symbol) int { return compare(a.SelectionRange.Start, b.SelectionRange.Start) })
	return syms
}

// enclose stretches the range of sym over its children's, as the protocol wants.
func enclose(sym *symbol) *symbol {
	for _, c := range sym.Children {
		if compare(c.Range.End, sym.Range.End) > 0 {
			sym.Range.End = c.Range.End
		}
	}
	return sym
}

func compare(a, b position) int {
	return cmp.Or(cmp.Compare(a.Line, b.Line), cmp.Compare(a.Character, b.Character))
}

// documentSymbols returns the sections of the document at path, each up to the next, with the ids written in
// them.
func (s *server) documentSymbols(path string) []*symbol {
	src, err := s.read(path)
	if err != nil {
		return []*symbol{}
	}
	d, _ := doc.Parse(path, string(src)) // its diagnostics are published
	type entry struct {
		line, level int // level is 0 for an id
		sym         *symbol
	}
	var entries []entry
	var walk func(*doc.Section)
	walk = func(sec *doc.Section) {
		for _, c := range sec.Children {
			h := c.Heading
			sel := s.span(h.TextPos, utf8.RuneCountInString(h.Text))
			entries = append(entries, entry{h.Pos.Line, h.Level, &symbol{
				Name: h.Text, Kind: kindString, Range: span{Start: position{h.Pos.Line - 1, 0}}, SelectionRange: sel,
			}})
			walk(c)
		}
	}
	walk(d.Root)
	for _, r := range s.refs {
		if r.Pos.File == path && r.Pos == r.Decl {
			sel := s.span(r.Pos, r.Len)
			entries = append(entries, entry{r.Pos.Line, 0, &symbol{Name: "#" + s.text(r.Pos, r.Len), Kind: kindKey, Range: sel, SelectionRange: sel}})
		}
	}
	// A heading comes before the id written on its line, which belongs to it.
	slices.SortStableFunc(entries, func(a, b entry) int { return cmp.Compare(a.line, b.line) })
	syms := []*symbol{}
	var stack []entry
	end := func(line int) position {
		return position{line - 1, utf16Col(s.line(path, line), utf8.RuneCountInString(s.line(path, line))+1)}
	}
	closeTo := func(level, line int) {
		for len(stack) > 0 && stack[len(stack)-1].level >= level {
			stack[len(stack)-1].sym.Range.End = end(line)
			stack = stack[:len(stack)-1]
		}
	}
	for _, e := range entries {
		if e.level > 0 {
			closeTo(e.level, e.line-1)
		}
		if len(stack) == 0 {
			syms = append(syms, e.sym)
		} else {
			parent := stack[len(stack)-1].sym
			parent.Children = append(parent.Children, e.sym)
		}
		if e.level > 0 {
			stack = append(stack, e)
		}
	}
	closeTo(1, len(strings.Split(string(src), "\n")))
	return syms
}

// workspaceSymbols returns the symbols of every file the analysis read whose name holds query, ignoring case.
func (s *server) workspaceSymbols(query string) []map[string]any {
	s.analyze()
	files := map[string]bool{}
	for path := range s.open {
		files[path] = true
	}
	for _, r := range s.refs {
		files[r.Pos.File] = true
	}
	found := []map[string]any{}
	var visit func(path, container string, syms []*symbol)
	visit = func(path, container string, syms []*symbol) {
		for _, sym := range syms {
			if strings.Contains(strings.ToLower(sym.Name), strings.ToLower(query)) {
				found = append(found, map[string]any{
					"name": sym.Name, "kind": sym.Kind, "containerName": container,
					"location": location{toURI(path), sym.SelectionRange},
				})
			}
			visit(path, sym.Name, sym.Children)
		}
	}
	for _, path := range slices.Sorted(maps.Keys(files)) {
		visit(path, filepath.Base(path), s.symbols(path))
	}
	return found
}
