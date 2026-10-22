package sqlimport

import (
	"strings"
	"unicode"

	"github.com/func-it/kanon/internal/names"
)

// words cuts a table or column name into lowercase ASCII words: at spaces and punctuation, before a capital that
// follows a small letter or starts a word after an acronym, and between a digit and a letter. OrderID gives order
// and id, HTMLPages html and pages, ship_via ship and via.
func words(s string) []string {
	type char struct {
		c     byte
		upper bool
	}
	var chars []char
	for _, r := range s {
		for _, c := range []byte(names.Fold(string(r))) {
			if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
				chars = append(chars, char{c, unicode.IsUpper(r)})
			} else {
				chars = append(chars, char{' ', false})
			}
		}
	}
	isLetter := func(c byte) bool { return c >= 'a' && c <= 'z' }
	isDigit := func(c byte) bool { return c >= '0' && c <= '9' }
	var ws []string
	var w []byte
	for i, ch := range chars {
		if ch.c == ' ' {
			if len(w) > 0 {
				ws = append(ws, string(w))
				w = nil
			}
			continue
		}
		if len(w) > 0 {
			prev := chars[i-1]
			next := i+1 < len(chars) && isLetter(chars[i+1].c) && !chars[i+1].upper
			if ch.upper && (!prev.upper && isLetter(prev.c) || prev.upper && next) ||
				isLetter(ch.c) && isDigit(prev.c) {
				ws = append(ws, string(w))
				w = nil
			}
		}
		w = append(w, ch.c)
	}
	if len(w) > 0 {
		ws = append(ws, string(w))
	}
	return ws
}

// kebab gives the field name of a table or a column: OrderDetails gives order-details.
func kebab(s string) string { return strings.Join(words(s), "-") }

// schemaName gives the schema of a table, singular PascalCase: Order Details gives OrderDetail.
func schemaName(s string) string {
	ws := words(s)
	if len(ws) == 0 {
		return ""
	}
	ws[len(ws)-1] = singular(ws[len(ws)-1])
	var b strings.Builder
	for _, w := range ws {
		b.WriteString(strings.ToUpper(w[:1]) + w[1:])
	}
	return b.String()
}

// fieldName gives the field of a table, its last word plural: Order Details and order_detail both give
// order-details.
func fieldName(s string) string {
	ws := words(s)
	if len(ws) > 0 {
		ws[len(ws)-1] = plural(singular(ws[len(ws)-1]))
	}
	return strings.Join(ws, "-")
}

// plural gives the plural of an English singular, as singular reads it back: category, address, box, branch, key,
// order. A word in -us or -is, or not ending in a letter, is left alone.
func plural(w string) string {
	last := w[len(w)-1]
	switch {
	case last < 'a' || last > 'z', strings.HasSuffix(w, "us"), strings.HasSuffix(w, "is"):
		return w
	case last == 'y' && len(w) > 1 && !strings.ContainsRune("aeiou", rune(w[len(w)-2])):
		return w[:len(w)-1] + "ies"
	case strings.HasSuffix(w, "ss"), last == 'x', strings.HasSuffix(w, "ch"), strings.HasSuffix(w, "sh"):
		return w + "es"
	}
	return w + "s"
}

// singular gives the singular of an English plural, for the common cases: categories, addresses, boxes,
// territories, orders. A word in -ss, -us or -is is left alone.
func singular(w string) string {
	switch {
	case strings.HasSuffix(w, "ies") && len(w) > 3:
		return w[:len(w)-3] + "y"
	case strings.HasSuffix(w, "sses"), strings.HasSuffix(w, "xes"), strings.HasSuffix(w, "ches"),
		strings.HasSuffix(w, "shes"):
		return w[:len(w)-2]
	case strings.HasSuffix(w, "s") && len(w) > 1 && !strings.HasSuffix(w, "ss") && !strings.HasSuffix(w, "us") &&
		!strings.HasSuffix(w, "is"):
		return w[:len(w)-1]
	}
	return w
}
