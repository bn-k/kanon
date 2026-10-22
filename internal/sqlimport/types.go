package sqlimport

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// A colType is the native type a column is imported as.
type colType struct {
	native  string   // Text, Prose, Int, Number, Bool, Date, DateTime, File or Image
	exts    []string // File and Image: the extension of each row's file, from its content
	empties int      // Text and Prose: the empty texts, written as no value
	trimmed int      // Text: the values with spaces around them
	tidied  int      // Prose: the values with spaces at the ends of lines or extra blank lines
	crlf    int      // Prose: the values with \r\n or \r line breaks
}

var (
	// dateRe reads the dates and times SQLite writes: 2026-10-22, 2026-10-22 14:30:05.000, 2026-10-22T14:30Z.
	dateRe   = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})(?:[T ](\d{2}:\d{2})(?::(\d{2})(?:\.(\d+))?)?)? ?(Z|UTC|[+-]\d{2}:\d{2})?$`)
	checkRe  = `(?i)CHECK\s*\(\s*["\x60\[]?%s["\x60\]]?\s+IN\s*\(\s*0\s*,\s*1\s*\)\s*\)`
	typeName = regexp.MustCompile(`^[A-Za-z ]+`)
	newlines = strings.NewReplacer("\r\n", "\n", "\r", "\n")
)

// infer gives the type of column c from its declaration, checked against its values. A note says when the values
// overrule the declaration.
func infer(t *table, c int) (*colType, string, error) {
	col := t.cols[c]
	decl := strings.ToUpper(strings.TrimSpace(typeName.FindString(col.decl)))
	affinity := affinityOf(decl)
	isBool := decl == "BOOLEAN" || decl == "BOOL" || decl == "BIT" ||
		regexp.MustCompile(fmt.Sprintf(checkRe, regexp.QuoteMeta(col.name))).MatchString(t.sql)
	isDate := decl == "DATE" || decl == "DATETIME" || decl == "TIMESTAMP"
	var values []any
	var ints, nums, texts, blobs int
	for _, row := range t.rows {
		switch row[c].(type) {
		case nil:
			continue
		case int64:
			ints++
			nums++
		case float64:
			nums++
		case string:
			texts++
		case []byte:
			blobs++
		}
		values = append(values, row[c])
	}
	name := fmt.Sprintf("`%s.%s`", t.name, col.name)
	switch {
	case len(values) == 0:
		return &colType{native: declared(decl, affinity, isBool)}, "", nil
	case blobs > 0:
		if blobs < len(values) {
			return nil, "", fmt.Errorf("%s mixes files with other values", name)
		}
		return files(t, c), "", nil
	case isBool && ints == len(values) && zeroOne(values):
		return &colType{native: "Bool"}, "", nil
	case isDate && texts == len(values):
		if ct, fraction := dates(values); ct != nil {
			return ct, "", nil
		} else if fraction {
			return text(values), fmt.Sprintf("%s holds fractions of a second, so it is imported as `Text`", name), nil
		}
	case isBool || isDate:
	case affinity == "INTEGER" && ints == len(values):
		return &colType{native: "Int"}, "", nil
	case (affinity == "REAL" || affinity == "NUMERIC") && nums == len(values):
		return &colType{native: "Number"}, "", nil
	case affinity == "TEXT" && texts == len(values):
		return text(values), "", nil
	}
	ct := text(values)
	switch {
	case ints == len(values):
		ct = &colType{native: "Int"}
	case nums == len(values):
		ct = &colType{native: "Number"}
	}
	note := ""
	if decl != "" {
		note = fmt.Sprintf("%s is declared `%s` but holds other values, so it is imported as `%s`", name, col.decl,
			ct.native)
	}
	return ct, note, nil
}

// affinityOf gives the affinity SQLite gives a declared type: INTEGER, TEXT, BLOB, REAL or NUMERIC.
func affinityOf(decl string) string {
	switch {
	case strings.Contains(decl, "INT"):
		return "INTEGER"
	case strings.Contains(decl, "CHAR"), strings.Contains(decl, "CLOB"), strings.Contains(decl, "TEXT"):
		return "TEXT"
	case strings.Contains(decl, "BLOB"), decl == "":
		return "BLOB"
	case strings.Contains(decl, "REAL"), strings.Contains(decl, "FLOA"), strings.Contains(decl, "DOUB"):
		return "REAL"
	}
	return "NUMERIC"
}

// declared gives the type of a column without values, from its declaration only.
func declared(decl, affinity string, isBool bool) string {
	switch {
	case isBool:
		return "Bool"
	case decl == "DATE":
		return "Date"
	case decl == "DATETIME" || decl == "TIMESTAMP":
		return "DateTime"
	case affinity == "INTEGER":
		return "Int"
	case affinity == "REAL" || affinity == "NUMERIC":
		return "Number"
	case affinity == "BLOB" && decl != "":
		return "File"
	}
	return "Text"
}

func zeroOne(values []any) bool {
	for _, v := range values {
		if n := v.(int64); n != 0 && n != 1 {
			return false
		}
	}
	return true
}

// text types text values: Prose when one spans several lines, else Text. Numbers among them are written as
// numbers.
func text(values []any) *colType {
	ct := &colType{native: "Text"}
	for _, v := range values {
		s, ok := v.(string)
		if !ok {
			continue
		}
		n := newlines.Replace(s)
		t := strings.TrimSpace(n)
		switch {
		case t == "":
			ct.empties++
			continue
		case strings.Contains(t, "\n"):
			ct.native = "Prose"
		case t != s:
			ct.trimmed++
		}
		if n != s {
			ct.crlf++
		}
		if strings.Join(layout(n), "\n") != n {
			ct.tidied++
		}
	}
	if ct.native == "Prose" {
		ct.trimmed = 0
	} else {
		ct.tidied, ct.crlf = 0, 0
	}
	return ct
}

// dates types date values: Date when each is at midnight in no time zone, else DateTime. It returns nil when a
// value isn't a date, or has fractions of a second.
func dates(values []any) (*colType, bool) {
	ct := &colType{native: "Date"}
	for _, v := range values {
		m := dateRe.FindStringSubmatch(v.(string))
		if m == nil || !realDate(m) {
			return nil, false
		}
		if strings.Trim(m[4], "0") != "" {
			return nil, true
		}
		if m[2] != "" && m[2] != "00:00" || m[3] != "" && m[3] != "00" || m[5] != "" {
			ct.native = "DateTime"
		}
	}
	return ct, false
}

func realDate(m []string) bool {
	s, layout := m[1], time.DateOnly
	if m[2] != "" {
		s, layout = s+" "+m[2], layout+" 15:04"
	}
	if m[3] != "" {
		s, layout = s+":"+m[3], layout+":05"
	}
	_, err := time.Parse(layout, s)
	return err == nil
}

// formatDate writes a date, or a date and time, in the canonical form: 2026-10-22 14:30, with its seconds when
// they aren't zero, and UTC or its offset.
func formatDate(s string, native string) string {
	m := dateRe.FindStringSubmatch(s)
	if native == "Date" {
		return m[1]
	}
	out := m[1] + " " + m[2]
	if m[2] == "" {
		out += "00:00"
	}
	if m[3] != "" && m[3] != "00" {
		out += ":" + m[3]
	}
	switch m[5] {
	case "":
	case "Z", "UTC":
		out += " UTC"
	default:
		out += " " + m[5]
	}
	return out
}

// files types a column of files: Image when each is an image, else File.
func files(t *table, c int) *colType {
	ct := &colType{native: "Image", exts: make([]string, len(t.rows))}
	for i, row := range t.rows {
		b, ok := row[c].([]byte)
		if !ok {
			continue
		}
		kind, _, _ := strings.Cut(http.DetectContentType(b), ";")
		if (kind == "text/xml" || kind == "text/plain") && strings.Contains(string(b), "<svg") {
			kind = "image/svg+xml"
		}
		if !strings.HasPrefix(kind, "image/") {
			ct.native = "File"
		}
		ct.exts[i] = extensions[kind]
		if ct.exts[i] == "" {
			ct.exts[i] = "bin"
		}
	}
	return ct
}

var extensions = map[string]string{
	"image/jpeg": "jpg", "image/png": "png", "image/gif": "gif", "image/webp": "webp", "image/bmp": "bmp",
	"image/x-icon": "ico", "image/svg+xml": "svg", "application/pdf": "pdf", "application/zip": "zip",
	"application/x-gzip": "gz", "text/plain": "txt", "text/html": "html", "text/xml": "xml", "audio/mpeg": "mp3",
	"video/mp4": "mp4",
}

// number writes a number with its thousands apart: 1 250.5.
func number(v any) string {
	var s string
	switch n := v.(type) {
	case int64:
		s = strconv.FormatInt(n, 10)
	case float64:
		s = strconv.FormatFloat(n, 'f', -1, 64)
	}
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	}
	whole, frac, hasFrac := strings.Cut(s, ".")
	var b strings.Builder
	b.WriteString(sign)
	for i := range len(whole) {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteByte(whole[i])
	}
	if hasFrac {
		b.WriteString("." + frac)
	}
	return b.String()
}

// plain writes a value as text: numbers without spaces.
func plain(v any) string {
	switch n := v.(type) {
	case int64:
		return strconv.FormatInt(n, 10)
	case float64:
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	return v.(string)
}
