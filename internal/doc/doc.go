// Package doc reads the shape of a .kan document: front matter, sections, key lines, paragraphs, lists, tables and
// code blocks, each with its exact position. It knows nothing about schemas; the linter binds the shape to one.
package doc

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/bn-k/kanon/internal/diag"
)

// Doc is the shape of a .kan file.
type Doc struct {
	Path           string
	HasFrontMatter bool
	FrontMatter    []*KeyLine // the key: value lines between the two --- lines
	Root           *Section   // Heading == nil; holds what comes before the first heading, and the top headings as Children
	lines          []string   // the source, for Text
}

// Text returns key lines and blocks as written, for a Prose field: each of their lines trimmed on the right, and
// one blank line wherever blank lines separate them.
func (d *Doc) Text(keys []*KeyLine, blocks []Block) string {
	var b strings.Builder
	last := 0
	add := func(first, end int) {
		switch {
		case last == 0:
		case first == last+1:
			b.WriteString("\n")
		default:
			b.WriteString("\n\n")
		}
		for j := first; j <= end; j++ {
			if j > first {
				b.WriteString("\n")
			}
			b.WriteString(trimRight(d.lines[j-1]))
		}
		last = end
	}
	for _, k := range keys {
		add(k.Pos.Line, k.Pos.Line)
	}
	for _, bl := range blocks {
		add(bl.At().Line, bl.end())
	}
	return b.String()
}

// Section is a heading and what follows it, up to the next heading of the same or a higher level.
type Section struct {
	Heading  *Heading   // nil for Root
	Keys     []*KeyLine // key: value lines at the start of the section
	Blocks   []Block    // paragraphs, lists, tables and code blocks, in order
	Children []*Section
}

// Level returns the level of the section's heading, 0 for Root.
func (s *Section) Level() int {
	if s.Heading == nil {
		return 0
	}
	return s.Heading.Level
}

// Heading is a line such as ## Issuer {#moon-studio}. ID is "" when no {#id} is written.
type Heading struct {
	Pos     diag.Pos
	Level   int
	Text    string
	TextPos diag.Pos
	ID      string
	IDPos   diag.Pos
}

// Line is a line of text, trimmed, with the position of its first rune.
type Line struct {
	Pos  diag.Pos
	Text string
}

// KeyLine is a key: value line.
type KeyLine struct {
	Line
	Key      string
	Value    string
	ValuePos diag.Pos // just after the colon when Value is empty
}

// Block is a paragraph, a list, a table or a code block.
type Block interface {
	At() diag.Pos
	end() int
}

// span holds the line a block ends on.
type span struct{ last int }

func (s span) end() int { return s.last }

// Paragraph is a run of consecutive lines of text.
type Paragraph struct {
	Lines []Line
	span
}

// At returns the position of the paragraph's first line.
func (p *Paragraph) At() diag.Pos { return p.Lines[0].Pos }

// Text returns the paragraph as written: its lines stay separate, joined with newlines. A renderer may reflow
// them; the value of a Prose field doesn't.
func (p *Paragraph) Text() string { return join(p.Lines, "\n") }

// Code is a fenced code block. Info is the text after the opening fence; Lines are kept raw.
type Code struct {
	Pos   diag.Pos
	Info  string
	Lines []Line
	span
}

// At returns the position of the opening fence.
func (c *Code) At() diag.Pos { return c.Pos }

// Text returns the code as written, its lines joined with newlines.
func (c *Code) Text() string { return join(c.Lines, "\n") }

// List is a run of bullet items, or of ordered items.
type List struct {
	Pos     diag.Pos
	Ordered bool
	Items   []*Item
	span
}

// At returns the position of the first item's marker.
func (l *List) At() diag.Pos { return l.Pos }

// Item is a list item. Pos is its marker; Lines are the text after the marker, then its continuation lines, without
// the {#id}.
type Item struct {
	Pos   diag.Pos
	Lines []Line
	ID    string
	IDPos diag.Pos
}

// Text returns the item's lines joined with single spaces.
func (it *Item) Text() string { return join(it.Lines, " ") }

// Table is a header, a separator line and rows. Rows may differ in width from the header when an error says so.
type Table struct {
	Pos    diag.Pos
	Header []Cell
	Rows   [][]Cell
	RowPos []diag.Pos
	span
}

// At returns the position of the header's first pipe.
func (t *Table) At() diag.Pos { return t.Pos }

// Cell is a table cell: its trimmed text, with \| read as a pipe, and the position of its first rune (or of the cell
// when it is empty).
type Cell struct {
	Pos  diag.Pos
	Text string
}

var (
	headingRe = regexp.MustCompile(`^(#{1,6})(?:[ \t]+(.*?))?[ \t]*$`)
	idRe      = regexp.MustCompile(`^(.*?)[ \t]*\{#([^{}]*)\}$`)
	keyRe     = regexp.MustCompile(`^([\p{Ll}0-9][\p{L}0-9_-]*):(?:[ \t]+(.*?))?[ \t]*$`)
	bulletRe  = regexp.MustCompile(`^[-*+][ \t]+(.*)$`)
	orderedRe = regexp.MustCompile(`^\d+[.)][ \t]+(.*)$`)
	sepRe     = regexp.MustCompile(`^:?-+:?$`)
)

const separatorHint = `expected a separator line such as "|---|" under the table header`

// Parse reads the shape of the document src, found at path. It reports every problem it finds, not only the first.
func Parse(path, src string) (*Doc, diag.List) {
	lines := strings.Split(src, "\n")
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	p := &parser{path: path, lines: lines}
	d := &Doc{Path: path, Root: &Section{}, lines: lines}
	p.frontMatter(d)
	p.body(d.Root)
	return d, p.diags
}

type parser struct {
	path  string
	lines []string
	i     int // the line being read
	diags diag.List
}

// pos returns the position of the byte off of line j.
func (p *parser) pos(j, off int) diag.Pos {
	return diag.Pos{File: p.path, Line: j + 1, Col: utf8.RuneCountInString(p.lines[j][:off]) + 1}
}

func (p *parser) frontMatter(d *Doc) {
	if trimRight(p.lines[0]) != "---" {
		return
	}
	end := 1
	for end < len(p.lines) && trimRight(p.lines[end]) != "---" {
		end++
	}
	if end == len(p.lines) {
		p.diags.Errorf(p.pos(0, 0), `unterminated front matter: no closing "---"`)
		p.i = 1
		return
	}
	d.HasFrontMatter = true
	for j := 1; j < end; j++ {
		if isBlank(p.lines[j]) {
			continue
		}
		if k := p.keyLine(j); k != nil {
			d.FrontMatter = append(d.FrontMatter, k)
		} else {
			p.diags.Errorf(p.pos(j, indent(p.lines[j])), `expected "key: value" in front matter, got %q`, trim(p.lines[j]))
		}
	}
	p.i = end + 1
}

func (p *parser) body(root *Section) {
	open := []*Section{root}
	for p.i < len(p.lines) {
		line := p.lines[p.i]
		cur := open[len(open)-1]
		switch {
		case isBlank(line):
			p.i++
		case headingRe.MatchString(line):
			s := &Section{Heading: p.heading()}
			for open[len(open)-1].Level() >= s.Level() {
				open = open[:len(open)-1]
			}
			parent := open[len(open)-1]
			parent.Children = append(parent.Children, s)
			open = append(open, s)
		case isFence(line):
			cur.Blocks = append(cur.Blocks, p.code())
		case isTableLine(line):
			cur.Blocks = append(cur.Blocks, p.table())
		case isItem(line):
			cur.Blocks = append(cur.Blocks, p.list())
		case len(cur.Blocks) == 0 && keyRe.MatchString(line):
			cur.Keys = append(cur.Keys, p.keyLine(p.i))
			p.i++
		default:
			cur.Blocks = append(cur.Blocks, p.paragraph())
		}
	}
}

// keyLine reads line j as a key line, or returns nil.
func (p *parser) keyLine(j int) *KeyLine {
	line := p.lines[j]
	m := keyRe.FindStringSubmatchIndex(line)
	if m == nil {
		return nil
	}
	k := &KeyLine{Line: Line{p.pos(j, 0), trimRight(line)}, Key: line[m[2]:m[3]], ValuePos: p.pos(j, m[3]+1)}
	if m[4] >= 0 && m[4] < m[5] {
		k.Value, k.ValuePos = line[m[4]:m[5]], p.pos(j, m[4])
	}
	return k
}

func (p *parser) heading() *Heading {
	line := p.lines[p.i]
	m := headingRe.FindStringSubmatchIndex(line)
	h := &Heading{Pos: p.pos(p.i, 0), Level: m[3] - m[2], TextPos: p.pos(p.i, m[3])}
	if m[4] >= 0 {
		h.Text, h.TextPos = line[m[4]:m[5]], p.pos(p.i, m[4])
	}
	h.ID, h.IDPos = p.takeID(&h.Text, h.TextPos)
	p.i++
	return h
}

// takeID removes the {#id} written at the end of *text, which starts at pos, and returns the id and its position.
func (p *parser) takeID(text *string, pos diag.Pos) (string, diag.Pos) {
	m := idRe.FindStringSubmatchIndex(*text)
	if m == nil {
		return "", diag.Pos{}
	}
	id, idPos := (*text)[m[4]:m[5]], advance(pos, (*text)[:m[4]])
	if id == "" {
		p.diags.Errorf(advance(pos, (*text)[:m[4]-2]), `empty id "{#}"`)
	}
	*text = (*text)[m[2]:m[3]]
	return id, idPos
}

func (p *parser) code() *Code {
	line := p.lines[p.i]
	fence := line[:len(line)-len(strings.TrimLeft(line, line[:1]))]
	c := &Code{Pos: p.pos(p.i, 0), Info: trim(line[len(fence):])}
	for p.i++; p.i < len(p.lines); p.i++ {
		l := p.lines[p.i]
		if strings.HasPrefix(l, fence) && strings.Trim(trimRight(l), fence[:1]) == "" {
			p.i++
			c.last = p.i
			return c
		}
		c.Lines = append(c.Lines, Line{p.pos(p.i, 0), l})
	}
	p.diags.Errorf(c.Pos, "unterminated code block: no closing %q", fence)
	c.last = p.i
	return c
}

func (p *parser) table() *Table {
	start := p.i
	for p.i < len(p.lines) && isTableLine(p.lines[p.i]) {
		p.i++
	}
	t := &Table{Pos: p.pos(start, indent(p.lines[start])), Header: p.cells(start)}
	width := func(j int, cells []Cell, what string) {
		if len(cells) != len(t.Header) {
			p.diags.Errorf(p.pos(j, indent(p.lines[j])), "table %s has %s, header has %d", what, count(len(cells)), len(t.Header))
		}
	}
	rows := start + 1
	switch sep := start + 1; {
	case sep == p.i:
		p.diags.Errorf(t.Pos, separatorHint)
	case !isSeparator(p.cells(sep)):
		p.diags.Errorf(p.pos(sep, indent(p.lines[sep])), separatorHint)
	default:
		width(sep, p.cells(sep), "separator")
		rows++
	}
	for j := rows; j < p.i; j++ {
		cells := p.cells(j)
		width(j, cells, "row")
		t.Rows = append(t.Rows, cells)
		t.RowPos = append(t.RowPos, p.pos(j, indent(p.lines[j])))
	}
	t.last = p.i
	return t
}

// cells splits table line j on unescaped pipes. The leading pipe is required, the trailing one optional.
func (p *parser) cells(j int) []Cell {
	line := p.lines[j]
	var cells []Cell
	start := indent(line) + 1
	for k := start; k <= len(line); k++ {
		if k < len(line) && line[k] == '\\' && k+1 < len(line) && line[k+1] == '|' {
			k++
			continue
		}
		if k < len(line) && line[k] != '|' {
			continue
		}
		raw := line[start:k]
		if k == len(line) && isBlank(raw) {
			break
		}
		off := start
		if !isBlank(raw) {
			off += indent(raw)
		}
		cells = append(cells, Cell{p.pos(j, off), strings.ReplaceAll(trim(raw), `\|`, "|")})
		start = k + 1
	}
	return cells
}

func (p *parser) list() *List {
	ordered := orderedRe.MatchString(p.lines[p.i])
	l := &List{Pos: p.pos(p.i, 0), Ordered: ordered}
	for {
		l.Items = append(l.Items, p.item())
		j := p.i
		for j < len(p.lines) && isBlank(p.lines[j]) {
			j++
		}
		if j == len(p.lines) || !isItem(p.lines[j]) || orderedRe.MatchString(p.lines[j]) != ordered {
			l.last = p.i
			return l
		}
		p.i = j
	}
}

func (p *parser) item() *Item {
	line := p.lines[p.i]
	m := bulletRe.FindStringSubmatchIndex(line)
	if m == nil {
		m = orderedRe.FindStringSubmatchIndex(line)
	}
	it := &Item{Pos: p.pos(p.i, 0), Lines: []Line{{p.pos(p.i, m[2]), trimRight(line[m[2]:])}}}
	for p.i++; p.i < len(p.lines) && isContinuation(p.lines[p.i]); p.i++ {
		l := p.lines[p.i]
		it.Lines = append(it.Lines, Line{p.pos(p.i, indent(l)), trim(l)})
	}
	last := &it.Lines[len(it.Lines)-1]
	it.ID, it.IDPos = p.takeID(&last.Text, last.Pos)
	if last.Text == "" && len(it.Lines) > 1 {
		it.Lines = it.Lines[:len(it.Lines)-1]
	}
	return it
}

func (p *parser) paragraph() *Paragraph {
	para := &Paragraph{}
	for p.i < len(p.lines) && !endsParagraph(p.lines[p.i]) {
		l := p.lines[p.i]
		para.Lines = append(para.Lines, Line{p.pos(p.i, indent(l)), trim(l)})
		p.i++
	}
	para.last = p.i
	return para
}

func endsParagraph(line string) bool {
	return isBlank(line) || headingRe.MatchString(line) || isTableLine(line) || isItem(line) || isFence(line)
}

func isFence(line string) bool {
	return strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")
}

func isTableLine(line string) bool { return strings.HasPrefix(line[indent(line):], "|") }

func isItem(line string) bool { return bulletRe.MatchString(line) || orderedRe.MatchString(line) }

func isContinuation(line string) bool {
	return !isBlank(line) && (strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t"))
}

func isSeparator(cells []Cell) bool {
	for _, c := range cells {
		if !sepRe.MatchString(c.Text) {
			return false
		}
	}
	return len(cells) > 0
}

// advance returns pos moved past the runes of s.
func advance(pos diag.Pos, s string) diag.Pos {
	pos.Col += utf8.RuneCountInString(s)
	return pos
}

func join(lines []Line, sep string) string {
	parts := make([]string, len(lines))
	for i, l := range lines {
		parts[i] = l.Text
	}
	return strings.Join(parts, sep)
}

func count(n int) string {
	if n == 1 {
		return "1 cell"
	}
	return fmt.Sprintf("%d cells", n)
}

func isBlank(s string) bool     { return trim(s) == "" }
func trim(s string) string      { return strings.Trim(s, " \t") }
func trimRight(s string) string { return strings.TrimRight(s, " \t") }
func indent(s string) int       { return len(s) - len(strings.TrimLeft(s, " \t")) }
