package eval_test

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bn-k/kanon/internal/eval"
	"github.com/bn-k/kanon/internal/lint"
	"github.com/bn-k/kanon/internal/model"
)

// today is the day the rules of these tests take as today.
var today = time.Date(2026, 10, 22, 9, 30, 0, 0, time.UTC)

// head is the start of a document of type typ; its next line is line 6.
func head(typ string) string { return "---\ntype: " + typ + "\n---\n# Doc\n\n" }

// evalCase is a schema.kanonarch, a doc.kan and the files next to them, with what lint reports, or else the
// computed fields as kan eval prints them.
type evalCase struct {
	name   string
	schema string
	doc    string
	more   map[string]string
	want   []string
}

// run lints doc.kan against schema.kanonarch in a new folder, and returns its diagnostics, or else its computed
// fields, and its model.
func run(t *testing.T, c evalCase) ([]string, *model.Doc) {
	t.Helper()
	t.Chdir(t.TempDir())
	fs := map[string]string{"schema.kanonarch": c.schema, "doc.kan": c.doc}
	maps.Copy(fs, c.more)
	for path, src := range fs {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	d, r := lint.LoadOn("schema.kanonarch", "doc.kan", today)
	var got []string
	for _, x := range r.Diags {
		got = append(got, x.String())
	}
	if len(got) == 0 {
		got = eval.Computed(d)
	}
	return got, d
}

func runCases(t *testing.T, cases []evalCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := run(t, c)
			if !slices.Equal(got, c.want) {
				t.Errorf("output differs\n--- got\n%s\n--- want\n%s", strings.Join(got, "\n"), strings.Join(c.want, "\n"))
			}
		})
	}
}

func TestValues(t *testing.T) {
	runCases(t, []evalCase{
		{
			name: "numbers and amounts",
			schema: `
schema T {
  a: Int
  b: Number
  m: Money
  p: Percent

  i1: Int     = a + 2 * 3
  i2: Number  = a / 2
  i3: Number  = b - a
  i4: Int     = -a
  m1: Money   = m * 3 + m
  m2: Money   = m * p
  m3: Money   = round(m / 3, 0.01)
  m4: Number  = m / m
  m5: Money   = abs(m - m * 2)
  m6: Money   = m / 3
  p1: Percent = p + 5 %
  p2: Number  = a * p
  p3: Percent = p / 4 + p - 10 %
  r1: Number  = round(2.5, 1) + round(-2.5, 1)
  r2: Int     = abs(-a) + round(a, 5)
  l1: Money   = m + 1 EUR
  l2: Bool    = m > 1000 EUR
  l3: Money   = m + 1 250.50 EUR
  l4: Bool    = m < 1 000 000.00 EUR and 12 345 + 1 == 12346
  l5: Quantity = 15 000 km + 1 500 m
}
`,
			doc: head("T") + "a: 7\nb: 1.25\nm: 1 250.50 EUR\np: 20 %\n",
			want: []string{
				"doc.kan: i1 = 13",
				"doc.kan: i2 = 3.5",
				"doc.kan: i3 = -5.75",
				"doc.kan: i4 = -7",
				"doc.kan: m1 = 5 002.00 EUR",
				"doc.kan: m2 = 250.10 EUR",
				"doc.kan: m3 = 416.83 EUR",
				"doc.kan: m4 = 1",
				"doc.kan: m5 = 1 250.50 EUR",
				"doc.kan: m6 = 416.833333 EUR",
				"doc.kan: p1 = 25 %",
				"doc.kan: p2 = 1.4",
				"doc.kan: p3 = 15 %",
				"doc.kan: r1 = 0",
				"doc.kan: r2 = 12",
				"doc.kan: l1 = 1 251.50 EUR",
				"doc.kan: l2 = yes",
				"doc.kan: l3 = 2 501.00 EUR",
				"doc.kan: l4 = yes",
				"doc.kan: l5 = 15 001.5 km",
			},
		},
		{
			name:   "a unit named like a currency",
			schema: "unit EUR\nschema T {\n  q: Quantity\n  l: Quantity = q + 1 EUR\n}\n",
			doc:    head("T") + "q: 2 EUR\n",
			want:   []string{"doc.kan: l = 3 EUR"},
		},
		{
			name: "quantities, durations, dates and times",
			schema: `
schema T {
  q:      Quantity
  g:      Quantity
  n:      Quantity
  d:      Duration
  jour:   Date
  quand:  DateTime
  quand2: DateTime

  q1: Quantity = q + g
  q2: Quantity = g + q
  q3: Number   = q / g
  q4: Quantity = q * 1.5 - 500 g
  q5: Quantity = n + g
  q6: Quantity = 2 m2 + 1 ha
  q7: Quantity = g - n
  d1: Duration = d + 90 min
  d2: Duration = d * 3 + 1 year + 14 months
  d3: Number   = 3 h / 90 min
  d4: Duration = -d / 2
  d5: Duration = d - d
  d6: Duration = 1 week * 20 %
  d7: Number   = 1 year / 6 months
  t1: Date     = jour + 1 month
  t2: Date     = jour - 2 weeks
  t3: Duration = jour - (jour - 10 days)
  t4: DateTime = quand + 90 min
  t5: Duration = quand - (quand - 26 h)
  t6: Date     = 1 year + jour
  t7: DateTime = quand2 + 1 month
  t8: Date     = today - 1 day
}
`,
			doc: head("T") + "q: 2 kg\ng: 300 g\nn: 5\nd: 1 day\njour: 2026-01-31\nquand: 2026-10-22 14:30\n" +
				"quand2: 2026-10-22 14:30:05 +02:00\n",
			want: []string{
				"doc.kan: q1 = 2.3 kg",
				"doc.kan: q2 = 2 300 g",
				"doc.kan: q3 = 6.666667",
				"doc.kan: q4 = 2.5 kg",
				"doc.kan: q5 = 305 g",
				"doc.kan: q6 = 10 002 m2",
				"doc.kan: q7 = 295 g",
				"doc.kan: d1 = 1 day 1 h 30 min",
				"doc.kan: d2 = 2 years 2 months 3 days",
				"doc.kan: d3 = 2",
				"doc.kan: d4 = -12 h",
				"doc.kan: d5 = 0 min",
				"doc.kan: d6 = 1 day 9 h 36 min",
				"doc.kan: d7 = 2",
				"doc.kan: t1 = 2026-02-28",
				"doc.kan: t2 = 2026-01-17",
				"doc.kan: t3 = 10 days",
				"doc.kan: t4 = 2026-10-22 16:00",
				"doc.kan: t5 = 1 day 2 h",
				"doc.kan: t6 = 2027-01-31",
				"doc.kan: t7 = 2026-11-22 14:30:05 +02:00",
				"doc.kan: t8 = 2026-10-21",
			},
		},
		{
			name: "sums of nothing",
			schema: `
schema T {
  m:  Money
  q:  Quantity
  ms: [Money]?
  qs: [Quantity]?

  z1: Money    = sum(ms) + m
  z2: Money    = m - sum(ms)
  z3: Quantity = sum(qs) + q
  z4: Bool     = sum(ms) < m
  z5: Bool     = q > sum(qs)
  z6: Bool     = sum(ms) == m - m
  z7: Quantity = max(sum(qs), q)
  z8: Money    = sum(ms)
}
`,
			doc: head("T") + "m: 12.50 EUR\nq: 2 kg\n",
			want: []string{
				"doc.kan: z1 = 12.50 EUR",
				"doc.kan: z2 = 12.50 EUR",
				"doc.kan: z3 = 2 kg",
				"doc.kan: z4 = yes",
				"doc.kan: z5 = yes",
				"doc.kan: z6 = yes",
				"doc.kan: z7 = 2 kg",
				"doc.kan: z8 = 0.00",
			},
		},
		{
			name: "comparisons, logic and none",
			schema: `
enum Etat { neuf, bon, use }

schema T {
  a:    Int
  x:    Int?
  fait: Bool
  v:    Version
  s:    Text
  e:    Etat
  m:    Money
  d:    Duration
  tags: [Text]

  b1:  Bool  = a > 5 and not fait
  b2:  Bool? = x > 1
  b3:  Bool? = x > 1 and no
  b4:  Bool? = x > 1 or yes
  b5:  Bool? = x > 1 or no
  b6:  Bool  = x == none and none == x and a != none
  b7:  Bool  = tags == tags and tags != tags.map(t => t)
  b8:  Bool  = v < 595 and v >= 580.95 and 581 > v and v == v
  b9:  Bool  = s < "b" and s != "z"
  b10: Bool  = e == bon and e != neuf
  b11: Bool  = m >= m and m < m * 2 and m == m
  b12: Bool  = a == 7.0 and fait == no
  b13: Bool  = d > 30 days and d < 1 year and d == 1 month
  b14: Bool? = not (x > 1)
  b15: Bool  = yes and yes or no
  i1:  Int?  = x + 1
  i2:  Int   = (x ?? 4) + 1
  i3:  Int?  = if x > 1 then 1 else 2
  i4:  Int   = if fait then 1 else 2
  i5:  Text  = if a > 1 then s else "z"
  i6:  Int   = max(a, 3, 12)
  i7:  Int   = min(a, x ?? 9)
  i8:  Int?  = min(x, a)
  i9:  Int?  = -x
  e1:  Etat  = if fait then neuf else e
}
`,
			doc: head("T") + "a: 7\nfait: no\nv: 580.95\ns: a\ne: bon\nm: 10.00 EUR\nd: 1 month\n\n## Tags\n\n- a\n- b\n",
			want: []string{
				"doc.kan: b1 = yes",
				"doc.kan: b3 = no",
				"doc.kan: b4 = yes",
				"doc.kan: b6 = yes",
				"doc.kan: b7 = no",
				"doc.kan: b8 = yes",
				"doc.kan: b9 = yes",
				"doc.kan: b10 = yes",
				"doc.kan: b11 = yes",
				"doc.kan: b12 = yes",
				"doc.kan: b13 = yes",
				"doc.kan: b15 = yes",
				"doc.kan: i2 = 5",
				"doc.kan: i4 = 2",
				"doc.kan: i5 = a",
				"doc.kan: i6 = 12",
				"doc.kan: i7 = 7",
				"doc.kan: i8 = 7",
				"doc.kan: e1 = bon",
			},
		},
		{
			name: "files, prose, copies and none",
			schema: `
schema P {
  n:   Int
  r:   Duration?
  sub: [P]?
}

schema O {
  ls: [Int]
}

schema T {
  a:       Int
  x:       Int?
  y:       Int?
  m:       Money
  p:       Percent
  v:       Version
  d:       Duration
  resume:  Prose
  photo:   Image
  annexes: [File]
  vides:   [Text]?
  ps:      [P]
  o:       O?

  c1: Money    = p * m
  c2: Number   = d / 6 h
  c3: Duration = abs(-d) + abs(d)
  c4: Duration = sum(ps, i => i.r)
  c5: Bool     = v > 580.95
  c6: Int      = count(ps, i => i == ps.max-by(j => j.n))
  c7: Number   = a / -100000000
  c8: Money    = m / -100000000
  t1: Prose    = resume
  t2: Image    = photo
  t3: Int      = count(annexes) + count(vides)
  w1: O?       = o with { ls: ps.n }
  w2: [Int]?   = o?.ls
  w3: [Int]    = ps.map(i => ps.n)
  w4: [Int]?   = o?.ls?.filter(i => i > 1)
  w5: Int      = x ?? 3
  w6: Int?     = round(y, 1)
  w7: Int?     = abs(y)
  k1: [P]      = ps.map(i => i with { n: 9 })
  k2: [P]      = k1
  k3: [P]      = ps.map(i => i with { sub: ps })
  c9: Duration = 2 * d
}
`,
			doc: head("T") + "a: 1\nx: 4\nm: 1.00 EUR\np: 10 %\nv: 580.95.1\nd: 1 day\nresume: Un *résumé*.\n" +
				"photo: [[photo.png]]\n\n## Annexes\n\n- [[a.pdf]]\n\n## Ps\n\n| id | n |\n|---|---|\n| p1 | 1 |\n| p2 | 2 |\n",
			more: map[string]string{"photo.png": "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR", "a.pdf": "%PDF-1.4\n"},
			want: []string{
				"doc.kan: c1 = 0.10 EUR",
				"doc.kan: c2 = 4",
				"doc.kan: c3 = 2 days",
				"doc.kan: c4 = 0 min",
				"doc.kan: c5 = yes",
				"doc.kan: c6 = 1",
				"doc.kan: c7 = 0",
				"doc.kan: c8 = 0.00 EUR",
				"doc.kan: t1 = Un *résumé*.",
				"doc.kan: t2 = photo.png",
				"doc.kan: t3 = 1",
				"doc.kan: w3 = 1, 2, 1, 2",
				"doc.kan: w5 = 4",
				"doc.kan: k1 = p1, p2",
				"doc.kan: k2 = p1, p2",
				"doc.kan: k3 = p1, p2",
				"doc.kan: c9 = 2 days",
			},
		},
	})
}

const order = `
schema Item {
  n:     Int
  prix:  Money
  tag:   Text?

  total: Money = prix * n
}

schema Order {
  items: [Item]

  best:  Item?  = items.max-by(i => i.n)
  big:   [Item] = items.filter(i => i.n > 1)
  ns:    [Int]  = items.map(i => i.n)
  tags:  [Text] = items.tag
  s:     Money  = sum(items, i => i.total)
  s2:    Money  = sum(items.total)
  c:     Int    = count(items, i => i.n > 1) + count(items)
  top:   Int?   = best?.n
  m:     Int?   = max(items.n)
  zero:  Money  = sum(items.filter(i => i.n > 100).prix)
  half:  Money  = sum(items.map(i => cheap(i).total))
  ids:   [Text] = items.map(i => cheap(i).id)
  promo: [Item] = items.map(i => cheap(i))
  first: Text   = id
}

fn cheap(i: Item) -> Item = i with { prix: i.prix / 2 }
`

const orderDoc = "---\ntype: Order\n---\n# Commande\n\n## Items\n\n| id | n | prix | tag |\n|---|---|---|---|\n" +
	"| a | 1 | 10.00 EUR | x |\n| b | 3 | 2.50 EUR | |\n"

func TestObjects(t *testing.T) {
	c := evalCase{schema: order, doc: orderDoc}
	got, d := run(t, c)
	want := []string{
		"doc.kan: best = b",
		"doc.kan: big = b",
		"doc.kan: ns = 1, 3",
		"doc.kan: tags = x",
		"doc.kan: s = 17.50 EUR",
		"doc.kan: s2 = 17.50 EUR",
		"doc.kan: c = 3",
		"doc.kan: top = 3",
		"doc.kan: m = 3",
		"doc.kan: zero = 0.00",
		"doc.kan: half = 8.75 EUR",
		"doc.kan: ids = a, b",
		"doc.kan: promo = a, b",
		"doc.kan: first = doc",
		"doc.kan:10:1: total = 10.00 EUR",
		"doc.kan:11:1: total = 7.50 EUR",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("output differs\n--- got\n%s\n--- want\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// A copy made by with is an object of its own in the model, its fields computed again.
	promo := d.Root.Get("promo").Links[0]
	row := d.Root.Get("items").Objects[0]
	if promo == row || promo.ID != "a" || promo.Get("prix").Values[0].Raw != "5.00 EUR" ||
		promo.Get("total").Values[0].Raw != "5.00 EUR" || promo.Get("tag").Values[0].Raw != "x" {
		t.Errorf("promo = %+v", promo)
	}
	if !d.Root.Get("best").Field.Computed() || d.Root.Get("best").Links[0] != d.Root.Get("items").Objects[1] {
		t.Errorf("best = %+v", d.Root.Get("best"))
	}
}

func TestRules(t *testing.T) {
	runCases(t, []evalCase{
		{
			name: "rules that fail",
			schema: `
enum Etat { neuf, bon, use }

schema Ligne {
  n: Int
  e: Etat
}

schema T {
  a:      Int
  b:      Int
  quand:  Date
  lignes: [Ligne]
  tags:   [Text]
  x:      Int?
}

fn twice(x: Int, k: Int = 2) -> Int = x * k

rule plus(t: T, min: Int = 1) -> Bool
  reads "{t.a} is more than {min}"
  = t.a > min

rule both(t: T) -> Bool
  = t.a < 10 and twice(t.b, 3) < t.a

rule dates(t: T) -> Bool
  reads fr "{t.lignes} {t.tags} {t.x} {t.lignes.e} {t.nope} {t.a.b} { t }"
  = t.quand >= today

rule silent(t: T) -> Bool
  = not (t.a > 0)

rule holds(t: T) -> Bool
  = t.x > 1 and twice(t.a) == 2

rule pair(a: T, b: T) -> Bool
  = a.a > b.a

rule many(ls: [Ligne]) -> Bool
  = count(ls) > 5

rule ligne(l: Ligne) -> Bool
  reads "{l.e} {l.n}"
  = l.e != use and l.n < 5 or l.n > 100
`,
			doc: head("T") + "a: 1\nb: 1\nquand: 2026-10-01\n\n## Lignes\n\n| id | n | e |\n|---|---|---|\n" +
				"| l1 | 1 | neuf |\n| l2 | 2 | use |\n\n## Tags\n\n- a\n- b\n",
			want: []string{
				"doc.kan: error: `plus` fails: \"1 is more than 1\"; 1 is not more than 1",
				"doc.kan: error: `both` fails; 3 is not less than 1",
				"doc.kan: error: `dates` fails: \"l1, l2 a, b none neuf, use {t.nope} {t.a.b} doc\"; 2026-10-01 is before 2026-10-22",
				"doc.kan: error: `silent` fails",
				"doc.kan:15:1: error: `ligne` fails: \"use 2\"; use is equal to use and 2 is not more than 100",
			},
		},
		{
			name: "the words of a comparison",
			schema: `
schema T {
  a: Int
  j: Date
  v: Version
}

rule lt(t: T) -> Bool = t.a < 1
rule le(t: T) -> Bool = t.a <= 1
rule gt(t: T) -> Bool = t.a > 3
rule ge(t: T) -> Bool = t.a >= 3
rule eq(t: T) -> Bool = t.a == 3
rule ne(t: T) -> Bool = t.a != 2
rule jlt(t: T) -> Bool = t.j < t.j
rule jgt(t: T) -> Bool = t.j > t.j
rule jge(t: T) -> Bool = t.j >= t.j + 1 day
rule jle(t: T) -> Bool = t.j <= t.j - 1 day
rule vge(t: T) -> Bool = t.v >= 2003
rule vle(t: T) -> Bool = 2 003.1 <= t.v
`,
			doc: head("T") + "a: 2\nj: 2026-10-22\nv: 1907\n",
			want: []string{
				"doc.kan: error: `lt` fails; 2 is not less than 1",
				"doc.kan: error: `le` fails; 2 is more than 1",
				"doc.kan: error: `gt` fails; 2 is not more than 3",
				"doc.kan: error: `ge` fails; 2 is less than 3",
				"doc.kan: error: `eq` fails; 2 is not equal to 3",
				"doc.kan: error: `ne` fails; 2 is equal to 2",
				"doc.kan: error: `jlt` fails; 2026-10-22 is not before 2026-10-22",
				"doc.kan: error: `jgt` fails; 2026-10-22 is not after 2026-10-22",
				"doc.kan: error: `jge` fails; 2026-10-22 is before 2026-10-23",
				"doc.kan: error: `jle` fails; 2026-10-22 is after 2026-10-21",
				"doc.kan: error: `vge` fails; 1907 is less than 2003",
				"doc.kan: error: `vle` fails; 2003.1 is more than 1907",
			},
		},
		{
			name: "rules that cannot say why",
			schema: `
schema O {
  n: Int
}

schema T {
  x: Int?
  f: Bool?
  o: O?
}

rule none-is-not-3(t: T) -> Bool
  reads "{t.o.n}"
  = t.x == 3

rule defaults-to-no(t: T) -> Bool = t.f ?? no
`,
			doc: head("T"),
			want: []string{
				"doc.kan: error: `none-is-not-3` fails: \"none\"",
				"doc.kan: error: `defaults-to-no` fails",
			},
		},
		{
			name: "written computed fields",
			schema: `
schema T {
  a: Int
  m: Money

  b: Int   = a * 2
  c: Money = m * 2
  d: Money = m
  e: Int?  = if a > 5 then 1 else none
}
`,
			doc: head("T") + "a: 2\nm: 10.00 EUR\nb: 5\nc: 20.00 EUR\nd: 10.00 USD\ne: 3\n",
			want: []string{
				"doc.kan:8:1: error: `b` is 4, not \"5\"",
				"doc.kan:10:1: error: `d` is 10.00 EUR, not \"10.00 USD\"",
				"doc.kan:11:1: error: `e` is none, not \"3\"",
			},
		},
	})
}

func TestErrors(t *testing.T) {
	runCases(t, []evalCase{
		{
			name: "two currencies",
			schema: `
schema T {
  m: Money
  u: Money

  t:   Money = m + u
  t2:  Money = t * 2
  cmp: Bool  = m < u
  lit: Bool  = m > 1 USD
}

rule cheaper(t: T) -> Bool = t.u < t.m
rule reads-t(t: T) -> Bool = t.t2 > t.m
`,
			doc: head("T") + "m: 10.00 EUR\nu: 5.00 USD\n",
			want: []string{
				"doc.kan: error: `t`: 10.00 EUR and 5.00 USD are in two currencies; write them in one",
				"doc.kan: error: `cmp`: 10.00 EUR and 5.00 USD are in two currencies; write them in one",
				"doc.kan: error: `lit`: 10.00 EUR and 1.00 USD are in two currencies; write them in one",
				"doc.kan: error: `cheaper`: 5.00 USD and 10.00 EUR are in two currencies; write them in one",
			},
		},
		{
			name: "units that don't convert",
			schema: `
unit pincee

schema T {
  q: Quantity
  l: Quantity
  p: Quantity

  t:  Quantity = q + l
  t2: Quantity = q - p
  t3: Bool     = p > 1 pincee and q == l
}
`,
			doc: head("T") + "q: 2 kg\nl: 3 km\np: 2 pincee\n",
			want: []string{
				"doc.kan: error: `t`: 3 km doesn't convert to kg",
				"doc.kan: error: `t2`: 2 pincee doesn't convert to kg",
				"doc.kan: error: `t3`: 3 km doesn't convert to kg",
			},
		},
		{
			name: "dates that move by part of a day or of a month",
			schema: `
schema T {
  jour:  Date
  quand: DateTime
  d:     Duration

  f: Date     = jour + 12 h
  g: Date     = jour - d
  h: DateTime = quand + d
}
`,
			doc: head("T") + "jour: 2026-10-22\nquand: 2026-10-22 14:30\nd: 1.5 months\n",
			want: []string{
				"doc.kan: error: `f`: a date moves by whole days, not by 12 h",
				"doc.kan: error: `g`: a date moves by whole days, not by 1.5 months",
				"doc.kan: error: `h`: a date and time moves by whole months, not by 1.5 months",
			},
		},
		{
			name: "a division by zero",
			schema: `
schema T {
  a: Int
  m: Money

  x: Number = a / (a - a)
  y: Money  = m / 0
  z: Money  = round(m, 0)
  w: Number = m / (m - m)
}
`,
			doc: head("T") + "a: 2\nm: 10.00 EUR\n",
			want: []string{
				"doc.kan: error: `x`: a division by zero",
				"doc.kan: error: `y`: a division by zero",
				"doc.kan: error: `z`: a division by zero",
				"doc.kan: error: `w`: a division by zero",
			},
		},
		{
			name: "fields that depend on themselves through links",
			schema: `
schema A {
  other: -> A?

  x: Int = other?.y ?? 0
  y: Int = other?.x ?? 0
}
`,
			doc:  "---\ntype: A\n---\n# A\n\nother: [[b]]\n",
			more: map[string]string{"b.kan": "---\ntype: A\n---\n# B\n\nother: [[doc]]\n"},
			want: []string{
				"b.kan: error: `y` depends on itself, through the objects it links to",
				"b.kan: error: `x` depends on itself, through the objects it links to",
			},
		},
	})
}
