package schema

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/func-it/kanon/internal/diag"
)

type tokKind int

const (
	tEOF tokKind = iota
	tName
	tString
	tNumber
	tPunct
)

// token is a word of a .kanonarch file. The text of a string is its value, without quotes or escapes.
type token struct {
	kind tokKind
	text string
	pos  diag.Pos
}

var keywords = map[string]bool{
	"unit": true, "enum": true, "schema": true, "fn": true, "rule": true, "reads": true,
	"if": true, "then": true, "else": true, "and": true, "or": true, "not": true, "with": true,
}

var puncts2 = []string{"??", "?.", "==", "!=", "<=", ">=", "=>", "->", "<-"}

const puncts1 = "{}()[]:,.=<>+-*/?@%"

func (t token) isPunct(s string) bool { return t.kind == tPunct && t.text == s }

func (t token) isKeyword(s string) bool { return t.kind == tName && t.text == s }

// isName reports whether t is a name that isn't a keyword.
func (t token) isName() bool { return t.kind == tName && !keywords[t.text] }

// desc describes t in an error message.
func desc(t token) string {
	switch t.kind {
	case tEOF:
		return "end of file"
	case tString:
		return "a string"
	}
	return "`" + t.text + "`"
}

type lexer struct {
	src       []rune
	i         int
	line, col int
	path      string
	toks      []token
	diags     diag.List
}

// lex splits src into tokens, the last one tEOF. Columns count runes.
func lex(path, src string) ([]token, diag.List) {
	l := &lexer{src: []rune(src), line: 1, col: 1, path: path}
	for l.i < len(l.src) {
		l.next()
	}
	l.toks = append(l.toks, token{kind: tEOF, pos: l.pos()})
	return l.toks, l.diags
}

func (l *lexer) pos() diag.Pos { return diag.Pos{File: l.path, Line: l.line, Col: l.col} }

// peek returns the rune k runes ahead, or 0 past the end.
func (l *lexer) peek(k int) rune {
	if l.i+k < len(l.src) {
		return l.src[l.i+k]
	}
	return 0
}

func (l *lexer) advance() rune {
	r := l.src[l.i]
	l.i++
	if r == '\n' {
		l.line, l.col = l.line+1, 1
	} else {
		l.col++
	}
	return r
}

func (l *lexer) emit(kind tokKind, text string, pos diag.Pos) {
	l.toks = append(l.toks, token{kind, text, pos})
}

func (l *lexer) next() {
	pos := l.pos()
	switch r := l.peek(0); {
	case r == ' ' || r == '\t' || r == '\r' || r == '\n':
		l.advance()
	case r == '/' && l.peek(1) == '/':
		for l.i < len(l.src) && l.peek(0) != '\n' {
			l.advance()
		}
	case unicode.IsLetter(r) || r == '_':
		l.name(pos)
	case isDigit(r):
		l.number(pos)
	case r == '"':
		l.string(pos)
	default:
		l.punct(pos)
	}
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// name reads a name; a hyphen belongs to it only between letters or digits: total-ht.
func (l *lexer) name(pos diag.Pos) {
	start := l.i
	for {
		r := l.peek(0)
		if isWordRune(r) || r == '_' || r == '-' && isWordRune(l.peek(1)) {
			l.advance()
			continue
		}
		break
	}
	l.emit(tName, string(l.src[start:l.i]), pos)
}

// number reads a number, its thousands spaces kept as in a document: 1 250.50. Two numbers never follow each
// other, so digits after a space belong to the number even badly grouped, as in 1 25, which the check reports.
func (l *lexer) number(pos diag.Pos) {
	start := l.i
	for {
		for isDigit(l.peek(0)) {
			l.advance()
		}
		if l.peek(0) != ' ' || !isDigit(l.peek(1)) {
			break
		}
		l.advance()
	}
	if l.peek(0) == '.' && isDigit(l.peek(1)) {
		l.advance()
		for isDigit(l.peek(0)) {
			l.advance()
		}
	}
	l.emit(tNumber, string(l.src[start:l.i]), pos)
}

// string reads a string on one line, with the escapes \" and \\.
func (l *lexer) string(pos diag.Pos) {
	l.advance()
	var b strings.Builder
	for {
		if l.i == len(l.src) || l.peek(0) == '\n' {
			l.diags.Errorf(pos, "unterminated string")
			break
		}
		epos := l.pos()
		r := l.advance()
		if r == '"' {
			break
		}
		if r == '\\' {
			switch e := l.peek(0); e {
			case '"', '\\':
				b.WriteRune(l.advance())
				continue
			case 0, '\n':
			default:
				l.diags.Errorf(epos, "unknown escape \"\\%c\"; write \"\\\\\" for a backslash", e)
			}
		}
		b.WriteRune(r)
	}
	l.emit(tString, b.String(), pos)
}

func (l *lexer) punct(pos diag.Pos) {
	for _, p := range puncts2 {
		if l.peek(0) == rune(p[0]) && l.peek(1) == rune(p[1]) {
			l.advance()
			l.advance()
			l.emit(tPunct, p, pos)
			return
		}
	}
	r := l.advance()
	if r >= utf8.RuneSelf {
		return // reported by Parse: a schema is written in ASCII
	}
	if !strings.ContainsRune(puncts1, r) {
		l.diags.Errorf(pos, "unexpected character %q", string(r))
		return
	}
	l.emit(tPunct, string(r), pos)
}
