// Package names holds Kanon's rules for names and ids: ASCII slugs, the headings that find them, and suggestions for typos.
package names

import (
	"strings"
	"unicode"
)

// IsSlug reports whether s is an ASCII slug: lowercase letters and digits, single hyphens between them.
// Ids are slugs: crepes, inv-2026-031, 2025-06-10-oil-change.
func IsSlug(s string) bool {
	if s == "" || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		case c == '-' && s[i-1] != '-':
		default:
			return false
		}
	}
	return true
}

// IsName reports whether s is a name: a slug that starts with a letter (unit-price, oil-change).
func IsName(s string) bool {
	return IsSlug(s) && s[0] >= 'a' && s[0] <= 'z'
}

// IsTypeName reports whether s is a type name: ASCII PascalCase (Recipe, CheckInReport).
func IsTypeName(s string) bool {
	if s == "" || s[0] < 'A' || s[0] > 'Z' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// Heading returns the heading that opens the field of a name: unit-price is "Unit price". Slugify turns it
// back into the name.
func Heading(name string) string {
	if name == "" {
		return ""
	}
	s := strings.ReplaceAll(name, "-", " ")
	return strings.ToUpper(s[:1]) + s[1:]
}

// Article returns `name` in backticks, preceded by a or an: an `Image`, a `Party`.
func Article(name string) string {
	if name != "" && strings.ContainsRune("AEIOU", rune(name[0])) {
		return "an `" + name + "`"
	}
	return "a `" + name + "`"
}

var accents = map[rune]string{
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'ā': "a", 'æ': "ae", 'ç': "c",
	'è': "e", 'é': "e", 'ê': "e", 'ë': "e", 'ē': "e", 'ì': "i", 'í': "i", 'î': "i", 'ï': "i", 'ñ': "n",
	'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o", 'ø': "o", 'œ': "oe",
	'ù': "u", 'ú': "u", 'û': "u", 'ü': "u", 'ý': "y", 'ÿ': "y", 'ß': "ss",
}

// Fold lowercases s and drops its accents, to match what someone wrote with what was expected.
func Fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if a, ok := accents[r]; ok {
			b.WriteString(a)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// punctuation holds the plain ASCII a few signs are written with.
var punctuation = map[rune]string{'°': "o", '’': "'", '‘': "'", '–': "-", '—': "-"}

// Transliterate returns the plain ASCII that stands for r: "e" for "é", "E" for "É", "o" for "°". It reports
// false when there is none.
func Transliterate(r rune) (string, bool) {
	if s, ok := punctuation[r]; ok {
		return s, true
	}
	s, ok := accents[unicode.ToLower(r)]
	if ok && unicode.IsUpper(r) {
		s = strings.ToUpper(s[:1]) + s[1:]
	}
	return s, ok
}

// Slugify proposes a slug for s: "Rosé of the day" becomes rose-of-the-day, unit_price becomes unit-price.
// It is also how a heading finds its field: "Unit price" opens unit-price, "Zoë's café" opens zoe-s-cafe.
func Slugify(s string) string {
	var b strings.Builder
	cut := false
	for _, r := range Fold(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if cut && b.Len() > 0 {
				b.WriteByte('-')
			}
			cut = false
			b.WriteRune(r)
		} else {
			cut = true
		}
	}
	return b.String()
}

// Closest returns the candidate nearest to s, ignoring case and accents, or "" when none is near enough.
func Closest(s string, candidates []string) string {
	fs := []rune(Fold(s))
	best, bestDist := "", 0
	for _, c := range candidates {
		fc := []rune(Fold(c))
		d := distance(fs, fc)
		prefix := len(fs) >= 3 && len(fc) > len(fs) && string(fc[:len(fs)]) == string(fs)
		if d > max(1, len(fs)/3) && !prefix {
			continue
		}
		if best == "" || d < bestDist {
			best, bestDist = c, d
		}
	}
	return best
}

// distance is the Levenshtein distance between a and b.
func distance(a, b []rune) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
