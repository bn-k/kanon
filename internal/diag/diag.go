// Package diag holds the diagnostics Kanon reports, written like a compiler's: file:line:col: severity: message.
package diag

import (
	"cmp"
	"fmt"
	"slices"
)

// Severity tells an error, which fails the check, from a warning, which doesn't.
type Severity int

const (
	Error Severity = iota
	Warning
)

func (s Severity) String() string {
	if s == Warning {
		return "warning"
	}
	return "error"
}

// Pos is a position in a file. Line and Col start at 1; a zero Line stands for a whole file or folder.
type Pos struct {
	File string
	Line int
	Col  int
}

func (p Pos) String() string {
	if p.Line == 0 {
		return p.File
	}
	return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Col)
}

// Diagnostic is one thing Kanon reports.
type Diagnostic struct {
	Pos      Pos
	Severity Severity
	Msg      string
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s: %s: %s", d.Pos, d.Severity, d.Msg)
}

// List collects diagnostics.
type List []Diagnostic

// Errorf adds an error at pos.
func (l *List) Errorf(pos Pos, format string, args ...any) {
	*l = append(*l, Diagnostic{pos, Error, fmt.Sprintf(format, args...)})
}

// Warnf adds a warning at pos.
func (l *List) Warnf(pos Pos, format string, args ...any) {
	*l = append(*l, Diagnostic{pos, Warning, fmt.Sprintf(format, args...)})
}

// Count returns the number of diagnostics of severity s.
func (l List) Count(s Severity) int {
	n := 0
	for _, d := range l {
		if d.Severity == s {
			n++
		}
	}
	return n
}

// Sort orders the list by file, line and column, keeping the order of diagnostics at the same position.
func (l List) Sort() {
	slices.SortStableFunc(l, func(a, b Diagnostic) int {
		return cmp.Or(
			cmp.Compare(a.Pos.File, b.Pos.File),
			cmp.Compare(a.Pos.Line, b.Pos.Line),
			cmp.Compare(a.Pos.Col, b.Pos.Col),
		)
	})
}
