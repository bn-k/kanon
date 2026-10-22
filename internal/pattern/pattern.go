// Package pattern reads the reads patterns of schemas, such as "{quantite} [de] {nom}", and matches list items
// against them.
package pattern

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/func-it/kanon/internal/names"
)

// Pattern is a parsed reads pattern.
type Pattern struct {
	Text   string
	segs   []segment
	fields []string
}

type kind int

const (
	literal kind = iota
	space
	placeholder
	optional
)

// segment is one piece of a pattern: literal text, a space, a placeholder or an optional group.
type segment struct {
	kind  kind
	text  string    // the text of a literal, the field of a placeholder
	col   int       // where a placeholder starts in the pattern text
	group []segment // the content of an optional group
	last  bool      // a placeholder that ends the pattern, and takes the rest of the line
}

// Error is an error in a pattern; Col is the rune column within the pattern text, from 1.
type Error struct {
	Col int
	Msg string
}

func (e *Error) Error() string { return e.Msg }

func errorf(col int, format string, args ...any) error {
	return &Error{col, fmt.Sprintf(format, args...)}
}

// Parse reads a pattern.
func Parse(text string) (*Pattern, error) {
	ps := &parser{runes: []rune(text), seen: map[string]bool{}}
	segs, err := ps.seq(0)
	if err != nil {
		return nil, err
	}
	segs = trimSpaces(segs)
	if len(segs) == 0 {
		return nil, errorf(1, "empty pattern")
	}
	segs = absorb(segs)
	if _, err := checkSeparated(segs, nil); err != nil {
		return nil, err
	}
	if s := &segs[len(segs)-1]; s.kind == placeholder {
		s.last = true
	}
	return &Pattern{Text: text, segs: segs, fields: ps.fields}, nil
}

// Fields returns the placeholders of p, in order.
func (p *Pattern) Fields() []string { return slices.Clone(p.fields) }

type parser struct {
	runes  []rune
	i      int
	seen   map[string]bool
	fields []string
}

// seq reads segments up to the end of the text or, inside an optional group opened at column open, up to its "]".
func (p *parser) seq(open int) ([]segment, error) {
	var segs []segment
	for p.i < len(p.runes) {
		col := p.i + 1
		switch r := p.runes[p.i]; {
		case r == ']':
			if open == 0 {
				return nil, errorf(col, `unmatched "]"; write \] for a literal bracket`)
			}
			p.i++
			return segs, nil
		case r == '[':
			p.i++
			group, err := p.seq(col)
			if err != nil {
				return nil, err
			}
			if len(trimSpaces(group)) == 0 {
				return nil, errorf(col, "empty optional group")
			}
			segs = append(segs, segment{kind: optional, group: group})
		case r == '{':
			s, err := p.placeholder()
			if err != nil {
				return nil, err
			}
			segs = append(segs, s)
		case r == '}':
			return nil, errorf(col, `unmatched "}"; write \} for a literal brace`)
		case r == '\\':
			r, err := p.escape()
			if err != nil {
				return nil, err
			}
			segs = addText(segs, r)
		case unicode.IsSpace(r):
			for p.i < len(p.runes) && unicode.IsSpace(p.runes[p.i]) {
				p.i++
			}
			segs = append(segs, segment{kind: space})
		default:
			p.i++
			segs = addText(segs, r)
		}
	}
	if open > 0 {
		return nil, errorf(open, `unclosed optional group; close it with "]"`)
	}
	return segs, nil
}

// placeholder reads {name}, from its "{".
func (p *parser) placeholder() (segment, error) {
	col := p.i + 1
	end := p.i + 1
	for end < len(p.runes) && p.runes[end] != '}' && p.runes[end] != '{' {
		end++
	}
	if end == len(p.runes) || p.runes[end] == '{' {
		return segment{}, errorf(col, `unterminated placeholder; close it with "}"`)
	}
	name := string(p.runes[p.i+1 : end])
	p.i = end + 1
	switch {
	case name == "":
		return segment{}, errorf(col, "empty placeholder; write a field name between the braces, as in {nom}")
	case !names.IsName(name):
		if fix := names.Slugify(name); names.IsName(fix) {
			return segment{}, errorf(col, "write {%s}, not %q", fix, "{"+name+"}")
		}
		return segment{}, errorf(col, "%q is not a field name: use lowercase ASCII letters, digits and hyphens, starting with a letter", "{"+name+"}")
	case p.seen[name]:
		return segment{}, errorf(col, "field `%s` appears twice", name)
	}
	p.seen[name] = true
	p.fields = append(p.fields, name)
	return segment{kind: placeholder, text: name, col: col}, nil
}

// escape reads \{, \}, \[, \] or \\, from its backslash.
func (p *parser) escape() (rune, error) {
	col := p.i + 1
	if p.i+1 == len(p.runes) {
		return 0, errorf(col, `lone "\" at the end; write \\ for a backslash`)
	}
	r := p.runes[p.i+1]
	if !strings.ContainsRune(`{}[]\`, r) {
		return 0, errorf(col, `unknown escape "\%c"; write \\ for a backslash`, r)
	}
	p.i += 2
	return r, nil
}

// addText adds r to the literal that ends segs, or starts one.
func addText(segs []segment, r rune) []segment {
	if n := len(segs); n > 0 && segs[n-1].kind == literal {
		segs[n-1].text += string(r)
		return segs
	}
	return append(segs, segment{kind: literal, text: string(r)})
}

// trimSpaces drops the spaces at both ends of segs; the parser never puts two spaces in a row.
func trimSpaces(segs []segment) []segment {
	if len(segs) > 0 && segs[0].kind == space {
		segs = segs[1:]
	}
	if len(segs) > 0 && segs[len(segs)-1].kind == space {
		segs = segs[:len(segs)-1]
	}
	return segs
}

// absorb moves a space next to an optional group inside it, so that skipping the group leaves no double space: the
// space before the group or, for a group that starts its sequence, the space after it. A group glued to text
// (item[s] left) keeps its spaces outside.
func absorb(segs []segment) []segment {
	var out []segment
	for i := 0; i < len(segs); i++ {
		s := segs[i]
		if s.kind == optional {
			switch {
			case len(out) > 0 && out[len(out)-1].kind == space:
				out = out[:len(out)-1]
				if s.group[0].kind != space {
					s.group = append([]segment{{kind: space}}, s.group...)
				}
			case i == 0 && i+1 < len(segs) && segs[i+1].kind == space:
				i++
				if s.group[len(s.group)-1].kind != space {
					s.group = append(s.group, segment{kind: space})
				}
			}
			s.group = absorb(s.group)
		}
		out = append(out, s)
	}
	return out
}

// prev is a placeholder that may come right before a segment, with nothing between them.
type prev struct {
	name    string
	skipped bool // only when an optional group between them is skipped
}

// checkSeparated reports two placeholders that nothing separates, since where one ends would be ambiguous. before
// holds the placeholders that may come right before segs; it returns those that may come right after.
func checkSeparated(segs []segment, before []prev) ([]prev, error) {
	for _, s := range segs {
		switch s.kind {
		case literal, space:
			before = nil
		case placeholder:
			if len(before) > 0 {
				when := ""
				if before[0].skipped {
					when = " when the optional group is skipped"
				}
				return nil, errorf(s.col, "nothing separates `%s` from `%s`%s; put text or a space between them",
					before[0].name, s.text, when)
			}
			before = []prev{{name: s.text}}
		case optional:
			after, err := checkSeparated(s.group, before)
			if err != nil {
				return nil, err
			}
			next := slices.Clone(after)
			for _, b := range before {
				next = append(next, prev{b.name, true})
			}
			before = next
		}
	}
	return before, nil
}

// Match reads line through p. accept tells whether a value suits a field; greedy tells whether a field takes the
// longest value it accepts rather than the shortest. A nil accept takes any value; a nil greedy makes every field
// lazy. Fields of a skipped optional group are absent from the values.
func (p *Pattern) Match(line string, accept func(field, value string) bool, greedy func(field string) bool) (map[string]string, bool) {
	m := &matcher{line: strings.TrimSpace(line), accept: accept, greedy: greedy, values: map[string]string{}}
	if m.accept == nil {
		m.accept = func(string, string) bool { return true }
	}
	if m.greedy == nil {
		m.greedy = func(string) bool { return false }
	}
	if !m.seq(p.segs, 0, func(pos int) bool { return pos == len(m.line) }) {
		return nil, false
	}
	return m.values, true
}

type matcher struct {
	line   string
	accept func(field, value string) bool
	greedy func(field string) bool
	values map[string]string
}

// seq matches segs at pos, then calls k with the position after them, backtracking until k returns true. When it
// returns false, values are as they were.
func (m *matcher) seq(segs []segment, pos int, k func(int) bool) bool {
	if len(segs) == 0 {
		return k(pos)
	}
	s := segs[0]
	next := func(end int) bool { return m.seq(segs[1:], end, k) }
	switch s.kind {
	case literal:
		return strings.HasPrefix(m.line[pos:], s.text) && next(pos+len(s.text))
	case space:
		end := pos
		for end < len(m.line) {
			r, n := utf8.DecodeRuneInString(m.line[end:])
			if !unicode.IsSpace(r) {
				break
			}
			end += n
		}
		return (end > pos || m.atBreak(pos)) && next(end)
	case optional:
		return m.seq(s.group, pos, next) || next(pos)
	}
	return m.field(s, pos, next)
}

// atBreak reports whether a space may match nothing at pos: at either end of the trimmed line, or right after a
// space already matched.
func (m *matcher) atBreak(pos int) bool {
	if pos == 0 || pos == len(m.line) {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(m.line[:pos])
	return unicode.IsSpace(r)
}

// field matches the placeholder s at pos with each accepted value in turn.
func (m *matcher) field(s segment, pos int, next func(int) bool) bool {
	for _, end := range m.ends(s, pos) {
		v := m.line[pos:end]
		if !m.accept(s.text, v) {
			continue
		}
		m.values[s.text] = v
		if next(end) {
			return true
		}
		delete(m.values, s.text)
	}
	return false
}

// ends lists where the value of s may end, in the order to try them. A value is not empty, neither starts nor ends
// with a space, and never splits a rune.
func (m *matcher) ends(s segment, pos int) []int {
	rest := m.line[pos:]
	if r, _ := utf8.DecodeRuneInString(rest); rest == "" || unicode.IsSpace(r) {
		return nil
	}
	if s.last {
		return []int{len(m.line)}
	}
	var ends []int
	for i := 0; i < len(rest); {
		r, n := utf8.DecodeRuneInString(rest[i:])
		i += n
		if !unicode.IsSpace(r) {
			ends = append(ends, pos+i)
		}
	}
	if m.greedy(s.text) {
		slices.Reverse(ends)
	}
	return ends
}
