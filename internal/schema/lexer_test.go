package schema

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/bn-k/kanon/internal/diag"
)

// render writes tokens back, a string quoted, separated by spaces.
func render(toks []token) string {
	var parts []string
	for _, t := range toks {
		switch t.kind {
		case tEOF:
		case tString:
			parts = append(parts, strconv.Quote(t.text))
		default:
			parts = append(parts, t.text)
		}
	}
	return strings.Join(parts, " ")
}

// diagStrings writes diagnostics as line:col: message.
func diagStrings(l diag.List) []string {
	var ss []string
	for _, d := range l {
		ss = append(ss, fmt.Sprintf("%d:%d: %s", d.Pos.Line, d.Pos.Col, d.Msg))
	}
	return ss
}

func TestLex(t *testing.T) {
	tests := []struct{ src, want string }{
		{"total-ht", "total-ht"},
		{"total - ht", "total - ht"},
		{"total -ht", "total - ht"},
		{"total- ht", "total - ht"},
		{"a-1 a--b a-_b", "a-1 a - - b a - _b"},
		{"prix_unitaire pincée _x x2", "prix_unitaire pincée _x x2"},
		{"12, 0.01 3. 4.x", "12 , 0.01 3 . 4 . x"},
		{`"a \"b\" \\ c"`, `"a \"b\" \\ c"`},
		{`"" "« x »"`, `"" "« x »"`},
		{"?? ?. == != <= >= => -> <-", "?? ?. == != <= >= => -> <-"},
		{"{ } ( ) [ ] : , . = < > + - * / ? @ %", "{ } ( ) [ ] : , . = < > + - * / ? @ %"},
		{"a??b x?.y a=>b a->b a<=b", "a ?? b x ?. y a => b a -> b a <= b"},
		{"a/b // a comment\nc", "a / b c"},
		{"// only a comment", ""},
		{"x\t\r\ny", "x y"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			toks, diags := lex("t", tt.src)
			if len(diags) > 0 {
				t.Fatalf("lex: %v", diags)
			}
			if got := render(toks); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
			if toks[len(toks)-1].kind != tEOF {
				t.Errorf("last token is not EOF")
			}
		})
	}
}

// TestLexGroups checks that a number keeps its thousands spaces, as in a document: 1 250.50, and that digits after a
// space belong to it even badly grouped.
func TestLexGroups(t *testing.T) {
	tests := []struct {
		src  string
		want []string
	}{
		{"1 250.50 EUR", []string{"1 250.50", "EUR"}},
		{"1250.50 EUR", []string{"1250.50", "EUR"}},
		{"12 345 678", []string{"12 345 678"}},
		{"1  250", []string{"1", "250"}},
		{"1 250 x", []string{"1 250", "x"}},
		{"1 25 EUR", []string{"1 25", "EUR"}}, // badly grouped, which the check reports
		{"x 12 0.01", []string{"x", "12 0.01"}},
		{"1 250 5", []string{"1 250 5"}},
	}
	for _, tt := range tests {
		toks, diags := lex("t", tt.src)
		if len(diags) > 0 {
			t.Errorf("lex(%q): %v", tt.src, diags)
		}
		var got []string
		for _, tok := range toks[:len(toks)-1] {
			got = append(got, tok.text)
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("lex(%q) = %q, want %q", tt.src, got, tt.want)
		}
	}
}

func TestLexPositions(t *testing.T) {
	toks, diags := lex("t", "é x\n\t\"«»\" 12 // c\n  ab")
	if len(diags) > 0 {
		t.Fatalf("lex: %v", diags)
	}
	want := []string{"1:1", "1:3", "2:2", "2:7", "3:3", "3:5"}
	var got []string
	for _, tok := range toks {
		got = append(got, fmt.Sprintf("%d:%d", tok.pos.Line, tok.pos.Col))
		if tok.pos.File != "t" {
			t.Errorf("File = %q, want t", tok.pos.File)
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLexErrors(t *testing.T) {
	const nbsp = "\xc2\xa0"
	tests := []struct {
		src, toks string
		want      []string
	}{
		{"a # b", "a b", []string{`1:3: unexpected character "#"`}},
		{"l'eau", "l eau", []string{`1:2: unexpected character "'"`}},
		{"a" + nbsp + "b", "a b", nil}, // Parse reports it: a schema is written in ASCII
		{`"abc`, `"abc"`, []string{"1:1: unterminated string"}},
		{"x \"abc\ny", `x "abc" y`, []string{"1:3: unterminated string"}},
		{`"abc\`, `"abc\\"`, []string{"1:1: unterminated string"}},
		{`"a\qb"`, `"a\\qb"`, []string{`1:3: unknown escape "\q"; write "\\" for a backslash`}},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			toks, diags := lex("t", tt.src)
			if got := render(toks); got != tt.toks {
				t.Errorf("tokens: got %s, want %s", got, tt.toks)
			}
			if got := diagStrings(diags); !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
