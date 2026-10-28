package lint

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/model"
	"github.com/bn-k/kanon/internal/value"
)

// dumpModel writes the model of d as text: each object with its file and line (and for a row of a spreadsheet, its
// cell), each field with its values, links, files, language and text, and the objects it holds below it. Paths are
// relative to dir.
func dumpModel(t *testing.T, d *model.Doc, dir string) string {
	t.Helper()
	var b strings.Builder
	rel := func(path string) string {
		r, err := filepath.Rel(dir, path)
		if err != nil {
			t.Fatal(err)
		}
		return filepath.ToSlash(r)
	}
	var dump func(o *model.Object, in string)
	dump = func(o *model.Object, in string) {
		at := rel(o.File)
		switch {
		case o.Pos.File != o.File: // a row of a spreadsheet
			at += fmt.Sprintf(", %s:%d:%d", rel(o.Pos.File), o.Pos.Line, o.Pos.Col)
		case o.Pos.Line > 0:
			at += fmt.Sprintf(":%d", o.Pos.Line)
		}
		fmt.Fprintf(&b, "%s%s %s %s\n", in, o.Schema.Name, o.ID, at)
		for _, f := range o.Fields {
			fmt.Fprintf(&b, "%s  %s:", in, f.Field.Name)
			for _, v := range f.Values {
				fmt.Fprintf(&b, " %q", v.Raw)
			}
			for _, k := range f.Links {
				fmt.Fprintf(&b, " -> %s %s", k.Schema.Name, k.ID)
			}
			for _, p := range f.Files {
				fmt.Fprintf(&b, " %s", rel(p))
			}
			if f.Lang != "" {
				fmt.Fprintf(&b, " %s", f.Lang)
			}
			if f.Text != "" {
				fmt.Fprintf(&b, " %q", f.Text)
			}
			b.WriteString("\n")
			for _, c := range f.Objects {
				if c.Parent != o || c.Holder != f.Field {
					t.Errorf("%s %s: Parent = %v, Holder = %v", c.Schema.Name, c.ID, c.Parent, c.Holder)
				}
				dump(c, in+"    ")
			}
		}
	}
	dump(d.Root, "")
	for _, o := range d.Linked {
		b.WriteString("linked\n")
		dump(o, "")
	}
	return b.String()
}

var casesDay = time.Date(2026, 10, 22, 0, 0, 0, 0, time.Local)

// load loads a case of cases/ on 2026-10-22, the day the cases were written, and checks that it finds the
// diagnostics want, none by default, and that its documents have no parent.
func load(t *testing.T, name, document string, want ...string) (*model.Doc, string) {
	t.Helper()
	dir := filepath.Join("..", "..", "cases", name)
	d, r := LoadOn(filepath.Join(dir, "schema.kanonarch"), filepath.Join(dir, document), casesDay)
	check(t, r, want)
	if d == nil {
		t.Fatalf("Load(%s) = nil", name)
	}
	for _, o := range append([]*model.Object{d.Root}, d.Linked...) {
		if o.Parent != nil || o.Holder != nil {
			t.Errorf("%s %s has a parent", o.Schema.Name, o.ID)
		}
	}
	return d, dir
}

func checkModel(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("model differs\n--- got\n%s\n--- want\n%s", got, want)
	}
}

func TestModelInvoice(t *testing.T) {
	d, dir := load(t, "invoice", "inv-2026-031.kan")
	if d.Schema == nil || d.Schema.Schema("Invoice") != d.Root.Schema {
		t.Errorf("Schema = %v", d.Schema)
	}
	checkModel(t, dumpModel(t, d, dir), `Invoice inv-2026-031 inv-2026-031.kan
  number: "INV-2026-031"
  date: "2026-10-22"
  due: "2026-11-21"
  issuer:
    Party halfmoon-studio inv-2026-031.kan:14
      name: "Halfmoon Studio Ltd"
      address: "8 Mist Quay, Larkhaven"
  client: -> Party ashgrove-builders
  lines:
    Line booking-api inv-2026-031.kan:23
      description: "Booking API development (days)"
      quantity: "12"
      unit-price: "650.00 EUR"
      amount: "7 800.00 EUR"
    Line hosting inv-2026-031.kan:24
      description: "Hosting, October to December (months)"
      quantity: "3"
      unit-price: "40.00 EUR"
      amount: "120.00 EUR"
  vat-rate: "20 %"
  terms: "Please pay {gross-total} by bank transfer no later than {due}, quoting {number} as the reference. Late payment\nbears interest at three times the statutory rate, plus a fixed charge of 40 EUR for recovery costs, which the\nlaw allows between businesses."
  net-total: "7 920.00 EUR"
  vat: "1 584.00 EUR"
  gross-total: "9 504.00 EUR"
linked
Party ashgrove-builders clients/ashgrove-builders.kan
  name: "Ashgrove Builders Ltd"
  address: "14 Linden Avenue, Fernhollow"
`)
	l := d.Root.Get("lines").Objects[0]
	if v := l.Get("unit-price").Values[0]; v.Kind != value.Money || v.Num.RatString() != "650" || v.Unit != "EUR" {
		t.Errorf("unit-price = %+v", v)
	}
	if d.Root.Get("client").Links[0] != d.Linked[0] {
		t.Errorf("client is not the linked document")
	}
}

func TestModelListing(t *testing.T) {
	d, dir := load(t, "listing", "house-elm-grove.kan")
	checkModel(t, dumpModel(t, d, dir), `Listing house-elm-grove house-elm-grove.kan
  title: "Family house with a garden in Elm Grove"
  reference: "ELM-2026-118"
  property: "house"
  price: "545 000.00 GBP"
  area: "112 m2"
  plot: "480 m2"
  bedrooms: "3"
  energy-rating: "c"
  viewing: "2026-10-25 10:30"
  agent: -> Agent helen-carter
  co-agents: -> Agent tom-ashby -> Agent chloe-hart
  seller: -> Seller fenwick-family
  location:
    Location orchard-close house-elm-grove.kan:30
      town: "Millbrook"
      postcode: "MB4 2QT"
      district: "Elm Grove"
  plan: house-elm-grove/ground-floor.pdf
  photos: house-elm-grove/photos/01-front.jpg house-elm-grove/photos/02-living-room.jpg house-elm-grove/photos/03-garden.png
  description: "![[house-elm-grove/photos/01-front.jpg]]\n\nAt the end of a quiet close, ten minutes from the centre of Millbrook and its cafés, this {area} house on a\n{plot} plot has three bedrooms, a bright living room opening onto the garden, a utility room and a garage.\n\n![The living room, south-facing](house-elm-grove/photos/02-living-room.jpg)\n\nThe living room, the kitchen and the main bedroom are all on the ground floor (see the\n[ground floor plan](house-elm-grove/ground-floor.pdf)). Offers in the region of {price}, with no onward\nchain. Viewings by appointment with [[agents/helen-carter]], the next one on {viewing}."
  rooms:
    Room living-room house-elm-grove.kan:45
      name: "Living room"
      level: "ground-floor"
      area: "38 m2"
    Room kitchen house-elm-grove.kan:46
      name: "Kitchen"
      level: "ground-floor"
      area: "14 m2"
    Room bedroom-1 house-elm-grove.kan:47
      name: "Main bedroom"
      level: "ground-floor"
      area: "12 m2"
    Room bathroom house-elm-grove.kan:48
      name: "Bathroom"
      level: "ground-floor"
      area: "6 m2"
    Room bedroom-2 house-elm-grove.kan:49
      name: "Bedroom"
      level: "upper-floor"
      area: "11 m2"
    Room bedroom-3 house-elm-grove.kan:50
      name: "Bedroom"
      level: "upper-floor"
      area: "10 m2"
    Room utility-room house-elm-grove.kan:51
      name: "Utility room"
      level: "basement"
      area: "8 m2"
  virtual-tour: html "<iframe src=\"https://tour.example/elm-2026-118\" width=\"640\" height=\"360\" allowfullscreen></iframe>"
linked
Agent helen-carter agents/helen-carter.kan
  name: "Helen Carter"
  agency: "Elm Grove Estates, Millbrook"
  phone: "+44 1632 960 461"
  email: "helen.carter@elm-grove-estates.example"
  portrait: agents/helen-carter/portrait.jpg
  listings: -> Listing house-elm-grove
linked
Seller fenwick-family sellers/fenwick-family.kan
  name: "The Fenwick family"
  address: "12 Orchard Close, Millbrook MB4 2QT"
  listing: -> Listing house-elm-grove
linked
Agent tom-ashby agents/tom-ashby.kan
  name: "Tom Ashby"
  agency: "Riverside Homes"
  phone: "+44 1632 960 876"
  email: "t.ashby@riverside-homes.example"
  co-listings: -> Listing house-elm-grove
linked
Agent chloe-hart agents/chloe-hart.kan
  name: "Chloë Hart"
  agency: "Millbrook Property Partners"
  phone: "+44 1632 960 607"
  email: "chloe.hart@millbrook-property.example"
  co-listings: -> Listing house-elm-grove
`)
	v := d.Root.Get("viewing").Values[0]
	if v.Kind != value.DateTime || v.Zone != "" || v.Seconds || v.DateTimeText() != "2026-10-25 10:30" {
		t.Errorf("viewing = %+v", v)
	}
	if d.Root.Get("plot") == nil || d.Linked[3].Get("portrait") != nil {
		t.Errorf("plot is written, chloe-hart's portrait is not")
	}
}

func TestModelFolder(t *testing.T) {
	d, dir := load(t, "car", "hatchback.kan", "../../cases/car/hatchback.kan: error: `oil-change-up-to-date` fails: "+
		"\"Grey hatchback has covered less than 15 000 km since its last oil change\"; 15 250 km is not less than 15 000 km")
	ss := d.Root.Get("services")
	if ss == nil || len(ss.Objects) != 5 {
		t.Fatalf("services = %v", ss)
	}
	var got []string
	for _, s := range ss.Objects {
		got = append(got, filepath.ToSlash(strings.TrimPrefix(s.File, dir+string(filepath.Separator)))+" "+s.ID)
	}
	want := "hatchback/services/2023-04-05-inspection.kan 2023-04-05-inspection\n" +
		"hatchback/services/2023-09-18-oil-change.kan 2023-09-18-oil-change\n" +
		"hatchback/services/2024-07-02-tyres.kan 2024-07-02-tyres\n" +
		"hatchback/services/2025-04-03-inspection.kan 2025-04-03-inspection\n" +
		"hatchback/services/2025-06-10-oil-change.kan 2025-06-10-oil-change"
	if strings.Join(got, "\n") != want {
		t.Errorf("services =\n%s\nwant\n%s", strings.Join(got, "\n"), want)
	}
	checkModel(t, dumpModel(t, &model.Doc{Root: ss.Objects[2]}, dir), `Service 2024-07-02-tyres hatchback/services/2024-07-02-tyres.kan
  date: "2024-07-02"
  mileage: "65 400 km"
  operation: "tyres"
  garage: "Central Garage"
  cost: "412.00 EUR"
`)
	if ss.Objects[2].Parent != d.Root || ss.Objects[2].Holder != ss.Field || ss.Pos.File != filepath.Join(dir, "hatchback", "services") {
		t.Errorf("services: Parent = %v, Holder = %v, Pos = %v", ss.Objects[2].Parent, ss.Objects[2].Holder, ss.Pos)
	}
}

func TestModelSuppliers(t *testing.T) {
	d, dir := load(t, "suppliers", "register.kan")
	ss := d.Root.Get("suppliers")
	if ss == nil || len(ss.Objects) != 3 {
		t.Fatalf("suppliers = %v", ss)
	}
	// Values written on a key line, separated by commas, are the items they would be under a heading.
	checkModel(t, dumpModel(t, &model.Doc{Root: ss.Objects[0]}, dir), `Supplier harbour-cables register/suppliers/harbour-cables.kan
  name: "Harbour Cables"
  norms: "iso-9001" "iso-14001"
  audits: "2025-03-12" "2026-03-10"
  sites: "Unit 4, Quarry Lane, Eastbrook" "Dock Road, Saltmere"
  norm-count: "2"
  last-audit: "2026-03-10"
`)
	if a := ss.Objects[0].Get("audits"); a.Values[1].Kind != value.Date || a.Pos.Line != 4 {
		t.Errorf("audits = %+v", a)
	}
}

func TestModelSheet(t *testing.T) {
	write(t, files{
		"schema.kanonarch": readings,
		"doc.kan":          lilac("readings: [[readings.csv]]\n"),
		"readings.csv":     "id,date,room,value,note,by\nm1,2026-10-01,lounge,19.5,,[[claire]]\nm2,2026-10-02,kitchen,21,draught from the café,\n",
		"claire.kan":       "---\ntype: Person\n---\n# Claire\n",
	})
	d, r := Load("schema.kanonarch", "doc.kan")
	check(t, r, nil)
	checkModel(t, dumpModel(t, d, "."), `House doc doc.kan
  name: "Lilac"
  readings:
    Reading m1 doc.kan, readings.csv:2:1
      date: "2026-10-01"
      room: "lounge"
      value: "19.5"
      by: -> Person claire
    Reading m2 doc.kan, readings.csv:3:1
      date: "2026-10-02"
      room: "kitchen"
      value: "21"
      note: "draught from the café"
linked
Person claire claire.kan
  name: "Claire"
`)
	rs := d.Root.Get("readings")
	if rs.Pos != (diag.Pos{File: "doc.kan", Line: 6, Col: 1}) {
		t.Errorf("readings: Pos = %v", rs.Pos)
	}
	if m := rs.Objects[1]; m.Parent != d.Root || m.Holder != rs.Field {
		t.Errorf("m2: Parent = %v, Holder = %v", m.Parent, m.Holder)
	}

	write(t, files{
		"schema.kanonarch": readings,
		"doc.kan":          lilac("## Readings\n\n[[readings.xlsx#autumn]]\n"),
		"readings.xlsx":    workbook(t, "autumn", "date,id,room,value\n2026-10-01,m1,lounge,19.5\n"),
	})
	d, r = Load("schema.kanonarch", "doc.kan")
	check(t, r, nil)
	checkModel(t, dumpModel(t, d, "."), `House doc doc.kan
  name: "Lilac"
  readings:
    Reading m1 doc.kan, readings.xlsx#autumn:2:2
      date: "2026-10-01"
      room: "lounge"
      value: "19.5"
`)
}

const notebookModel = `
enum State { new, worn }

schema Notebook {
  title:       Text @heading
  state:       State
  summary:     Prose
  notes:       Prose?
  script:      Code
  appendices:  [File]
  ingredients: [Ingredient]
  steps:       [Prose]
  chapters:    [Chapter] @sections
  total:       Int = 1
}

schema Ingredient reads "{quantity} of {name}" {
  quantity: Quantity
  name:     Text
}

schema Chapter {
  title: Text @heading
  body:  Prose? @body
}
`

func TestModel(t *testing.T) {
	write(t, files{
		"schema.kanonarch": notebookModel,
		"doc.kan": `---
type: Notebook
---
# My notebook

state: worn
total: 1
notes: a free  note
script: [[doc/run.sh]]
appendices: [[doc/a.pdf]], [[doc/b.txt]]

## Summary

status: draft
- one
- two

## Ingredients

- 2 kg of flour {#flour}

## Steps

1. Sauté the onions.
2. Bake {title}.

## First chapter {#one}

Text of the chapter.

## Second {#two}
`,
		"doc/run.sh": "echo hi\n",
		"doc/a.pdf":  "%PDF-1.4\n",
		"doc/b.txt":  "b\n",
	})
	d, r := Load("schema.kanonarch", "doc.kan")
	check(t, r, nil)
	checkModel(t, dumpModel(t, d, "."), `Notebook doc doc.kan
  title: "My notebook"
  state: "worn"
  summary: "status: draft\n- one\n- two"
  notes: "a free  note"
  script: sh "echo hi\n"
  appendices: doc/a.pdf doc/b.txt
  ingredients:
    Ingredient flour doc.kan:20
      quantity: "2 kg"
      name: "flour"
  steps: "Sauté the onions." "Bake {title}."
  chapters:
    Chapter one doc.kan:27
      title: "First chapter"
      body: "Text of the chapter."
    Chapter two doc.kan:31
      title: "Second"
  total: "1"
`)
	if v := d.Root.Get("steps").Values[0]; v.Kind != value.Prose {
		t.Errorf("steps[0] = %+v, want a Prose value", v)
	}
	if v := d.Root.Get("state").Values[0]; v.Kind != value.Text {
		t.Errorf("state = %+v, want a Text value", v)
	}
}

const projects = `
schema Project {
  name:    Text @heading
  lead:    -> Person
  members: [-> Person]?
  sponsor: -> Person?
  notes:   Prose? @body
}

schema Person {
  name:        Text @heading
  projects:    [Project] <- members
  leads:       [Project] <- lead
  sponsorship: Project? <- sponsor
}
`

const alpha = `---
type: Project
---
# Alpha

lead: [[ana]]
members: [[ana]], [[ana]], [[bob]]
sponsor: [[bob]]

See [[beta]].
`

func TestInverses(t *testing.T) {
	person := func(name string) string { return "---\ntype: Person\n---\n# " + name + "\n" }
	write(t, files{
		"schema.kanonarch": projects,
		"alpha.kan":        alpha,
		"beta.kan":         "---\ntype: Project\n---\n# Beta\n\nlead: [[ana]]\n",
		"ana.kan":          person("Ana"),
		"bob.kan":          person("Bob"),
	})
	d, r := Load("schema.kanonarch", "alpha.kan")
	check(t, r, nil)
	checkModel(t, dumpModel(t, d, "."), `Project alpha alpha.kan
  name: "Alpha"
  lead: -> Person ana
  members: -> Person ana -> Person ana -> Person bob
  sponsor: -> Person bob
  notes: "See [[beta]]."
linked
Person ana ana.kan
  name: "Ana"
  projects: -> Project alpha
  leads: -> Project alpha -> Project beta
linked
Person bob bob.kan
  name: "Bob"
  projects: -> Project alpha
  sponsorship: -> Project alpha
linked
Project beta beta.kan
  name: "Beta"
  lead: -> Person ana
`)

	write(t, files{
		"schema.kanonarch": projects,
		"alpha.kan":        alpha,
		"beta.kan":         "---\ntype: Project\n---\n# Beta\n\nlead: [[ana]]\nmembers: [[ana#lead]]\nsponsor: [[bob]]\n",
		"ana.kan":          person("Ana"),
		"bob.kan":          person("Bob"),
	})
	d, r = Load("schema.kanonarch", "alpha.kan")
	check(t, r, []string{
		"beta.kan:7:10: error: unknown id `lead` in ana.kan",
		"beta.kan:8:10: error: [[bob]] is already the `sponsor` of `alpha`, at alpha.kan:8:10; a `Person` has one `sponsorship`",
	})
	if d != nil {
		t.Errorf("Load = %v, want no model", d)
	}
}

func TestLoadWithErrors(t *testing.T) {
	write(t, files{"schema.kanonarch": note, "doc.kan": "---\ntype: Note\n---\n# Hi\n\ndate: tomorrow\n"})
	d, r := Load("schema.kanonarch", "doc.kan")
	if d != nil || r.Diags.Count(diag.Error) != 1 {
		t.Errorf("Load = %v, %v; want no model and one error", d, r.Diags)
	}
	// Partial keeps the model: the field whose value is wrong is there, where it is written, with no value.
	d, r = LoadWith("schema.kanonarch", "doc.kan", Options{Partial: true})
	if d == nil || r.Diags.Count(diag.Error) != 1 {
		t.Fatalf("LoadWith Partial = %v, %v; want a model and one error", d, r.Diags)
	}
	if f := d.Root.Get("date"); f == nil || f.Pos.Line != 6 || len(f.Values) != 0 {
		t.Errorf("date = %+v, want it written on line 6, with no value", f)
	}
	write(t, files{"schema.kanonarch": note})
	if d, _ := Load("schema.kanonarch", "doc.kan"); d != nil {
		t.Errorf("Load of a missing document = %v, want nil", d)
	}
	write(t, files{"schema.kanonarch": "schema note {}\n", "doc.kan": "# Hi\n"})
	if d, _ := Load("schema.kanonarch", "doc.kan"); d != nil {
		t.Errorf("Load with a wrong schema = %v, want nil", d)
	}
}

func TestModelNorthwind(t *testing.T) {
	if testing.Short() {
		t.Skip("Northwind is long")
	}
	d, _ := load(t, "northwind", "northwind.kan")
	count := map[string]int{}
	links := 0
	for o := range d.Objects() {
		count[o.Schema.Name]++
		if f := o.Get("territories"); o.Schema.Name == "Employee" && f != nil {
			links += len(f.Links)
		}
	}
	if count["Order"] != 830 || count["OrderDetail"] != 2155 || links != 49 || len(d.Linked) != 0 {
		t.Errorf("%d orders, %d order details, %d employee territories, %d linked; want 830, 2155, 49, 0",
			count["Order"], count["OrderDetail"], links, len(d.Linked))
	}
	employees := d.Root.Get("employees").Objects
	davolio, fuller := employees[0], employees[1]
	if got := davolio.Get("reports-to").Links[0]; got != fuller {
		t.Errorf("davolio reports to %v", got)
	}
	// The inverses, in document order.
	if got := fuller.Get("employees").Links; len(got) != 5 || got[0] != davolio {
		t.Errorf("fuller's employees = %v, want 5 with davolio first", got)
	}
	inverses := map[string]int{}
	for o := range d.Objects() {
		for _, f := range o.Fields {
			if f.Field.Inverse != "" {
				inverses[o.Schema.Name+"."+f.Field.Name] += len(f.Links)
			}
		}
	}
	if inverses["Employee.orders"] != 830 || inverses["Customer.orders"] != 830 || inverses["Product.order-details"] != 2155 ||
		inverses["Territory.employees"] != 49 {
		t.Errorf("inverses = %v", inverses)
	}
}
