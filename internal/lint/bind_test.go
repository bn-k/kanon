package lint

import "testing"

func TestTitle(t *testing.T) {
	runCases(t, []lintCase{
		{
			name:   "a second title",
			schema: note,
			doc:    "---\ntype: Note\n---\n# Hi\n# Again\n",
			want:   []string{"doc.kan:5:1: error: a document has one title; make this heading a section: ## Again"},
		},
		{
			name:   "an id on the title",
			schema: note,
			doc:    "---\ntype: Note\n---\n# Hi {#hi}\n",
			want:   []string{"doc.kan:4:8: error: a file's id is its name, `doc`; remove {#hi}"},
		},
		{
			name:   "content before the title",
			schema: note,
			doc:    "---\ntype: Note\n---\ndate: 2026-10-22\n\n## Body\n\nBefore the title.\n\n# Hi\n",
			want:   []string{"doc.kan:4:1: error: this comes before the title; move it under # Hi"},
		},
		{
			name:   "a section before the title",
			schema: note,
			doc:    "---\ntype: Note\n---\n## Body\n\nBefore the title.\n\n# Hi\n",
			want:   []string{"doc.kan:4:1: error: this comes before the title; move it under # Hi"},
		},
		{
			name:   "text before the title",
			schema: "schema Clause {\n  title: Text @heading\n  body: Prose @body\n}\n",
			doc:    "---\ntype: Clause\n---\nBefore the title.\n\n# Hi\n",
			want:   []string{"doc.kan:4:1: error: this comes before the title; move it under # Hi"},
		},
		{
			name:   "no title",
			schema: note,
			doc:    "---\ntype: Note\n---\ndate: 2026-10-22\n",
			want:   []string{"doc.kan: error: `Note` needs `title`; write it as the title: # …"},
		},
	})
}

func TestStrayText(t *testing.T) {
	runCases(t, []lintCase{
		{
			name:   "with prose fields",
			schema: "schema Note {\n  title: Text @heading\n  body: Prose?\n  notes: Prose?\n}\n",
			doc:    "---\ntype: Note\n---\n# Hi\n\nSome text.\n",
			want:   []string{"doc.kan:6:1: error: this text fills no field; write it under ## Body or ## Notes"},
		},
		{
			name:   "without a prose field",
			schema: "schema Tag {\n  name: Text @heading\n}\n",
			doc:    "---\ntype: Tag\n---\n# Hi\n\nSome text.\n",
			want:   []string{"doc.kan:6:1: error: this text fills no field: `Tag` has no field for free text"},
		},
	})
}

func TestHeadings(t *testing.T) {
	runCases(t, []lintCase{
		{
			name:   "unknown heading with a suggestion",
			schema: note,
			doc:    "---\ntype: Note\n---\n# Hi\n\n## Bod\n",
			want:   []string{`doc.kan:6:4: error: unknown heading "Bod"; did you mean ` + "`body`?"},
		},
		{
			name:   "unknown heading, and the headings the schema takes",
			schema: "schema Note {\n  title: Text @heading\n  body: Prose?\n  tags:  [Text]?\n}\n",
			doc:    "---\ntype: Note\n---\n# Hi\n\n## Misc\n",
			want:   []string{"doc.kan:6:4: error: unknown heading \"Misc\"; `Note` takes ## Body, ## Tags"},
		},
		{
			name:   "unknown heading in a schema that takes none",
			schema: "schema Tag {\n  name: Text @heading\n}\n",
			doc:    "---\ntype: Tag\n---\n# Hi\n\n## Misc\n",
			want:   []string{`doc.kan:6:4: error: unknown heading "Misc"`},
		},
		{
			name:   "a heading close to a required field counts as it written, and what it holds is not read",
			schema: chapters,
			doc:    "---\ntype: Book\n---\n# Hi\n\n## Chapter one {#one}\n\n### Step\n\n- x\n",
			want:   []string{`doc.kan:8:5: error: unknown heading "Step"; did you mean ` + "`steps`?"},
		},
		{
			name:   "a heading close to a field, then the right one",
			schema: chapters,
			doc:    "---\ntype: Book\n---\n# Hi\n\n## Chapter one {#one}\n\n### Step\n\n### Steps\n\n- 1\n",
			want:   []string{`doc.kan:8:5: error: unknown heading "Step"; did you mean ` + "`steps`?"},
		},
		{
			name:   "a heading far from every field leaves the field missing",
			schema: chapters,
			doc:    "---\ntype: Book\n---\n# Hi\n\n## Chapter one {#one}\n\n### How to\n\n- 1\n",
			want: []string{
				"doc.kan:6:1: error: `Chapter` needs `steps`; write it under ### Steps",
				"doc.kan:8:5: error: unknown heading \"How to\"; `Chapter` takes ### Steps",
			},
		},
		{
			name:   "an id on a heading that is not an object",
			schema: note,
			doc:    "---\ntype: Note\n---\n# Hi\n\n## Body {#c}\n\nSome text.\n",
			want:   []string{"doc.kan:6:11: error: only an object has an id, and `body` is a `Prose`; remove {#c}"},
		},
		{
			name:   "a heading that names a scalar field",
			schema: note,
			doc:    "---\ntype: Note\n---\n# Hi\n\n## Date\n",
			want:   []string{"doc.kan:6:4: error: \"Date\" names the field `date`, a `Date` written as a key line: date: 2026-10-22"},
		},
	})
}

// chapters holds its chapters as @sections: a heading in a chapter names a field of the chapter, or nothing.
const chapters = `
schema Book {
  title:    Text @heading
  chapters: [Chapter] @sections
}

schema Chapter {
  title: Text @heading
  steps: [Int]
}
`

const list = `
schema List {
  title: Text @heading
  steps: [Int]
}
`

func TestValues(t *testing.T) {
	head := "---\ntype: List\n---\n# Hi\n\n## Steps\n\n"
	runCases(t, []lintCase{
		{
			name:   "a list of values",
			schema: list,
			doc:    head + "1. 4\n2. 12\n",
		},
		{
			name:   "key lines",
			schema: list,
			doc:    head + "n: 3\nx: 4\n",
			want:   []string{"doc.kan:8:1: error: `steps` is a list of `Int`: write one item per value"},
		},
		{
			name:   "a paragraph",
			schema: list,
			doc:    head + "Four.\n",
			want:   []string{"doc.kan:8:1: error: `steps` is a list of `Int`: write one item per value"},
		},
		{
			name:   "an id on an item, and a wrong value",
			schema: list,
			doc:    head + "- 1 {#one}\n- two\n",
			want: []string{
				"doc.kan:8:7: error: an `Int` is a value, not an object: it has no id; remove {#one}",
				`doc.kan:9:3: error: write a whole number (4 or 1 250), not "two"`,
			},
		},
		{
			name:   "a subsection",
			schema: list,
			doc:    head + "- 1\n\n### More\n",
			want:   []string{"doc.kan:10:1: error: `steps` is a list of `Int`: write one item per value"},
		},
		{
			name:   "an id on the heading",
			schema: list,
			doc:    "---\ntype: List\n---\n# Hi\n\n## Steps {#e}\n\n- 1\n",
			want:   []string{"doc.kan:6:12: error: only an object has an id, and `steps` is a list of `Int`; remove {#e}"},
		},
	})
}

const recipe = `
schema Recipe {
  title:       Text @heading
  ingredients: [Ingredient]
}

schema Ingredient reads "{quantity} [of] {name}" {
  quantity: Quantity
  name:     Text
}
`

func TestObjects(t *testing.T) {
	head := "---\ntype: Recipe\n---\n# Crêpes\n\n## Ingredients\n\n"
	runCases(t, []lintCase{
		{
			name:   "a list read through a pattern",
			schema: recipe,
			doc:    head + "- 250 g of flour {#flour}\n- flour {#salt}\n- 2 g of salt\n",
			want: []string{
				`doc.kan:9:3: error: this item doesn't read as "{quantity} [of] {name}"`,
				"doc.kan:10:1: error: every object has an id: this `Ingredient` needs one; write its id at the end, as in - 2 g of salt {#id}",
			},
		},
		{
			name: "a list read through a pattern with an enum",
			schema: `
schema House {
  title: Text @heading
  rooms: [Room]
}

enum State { good, worn }

schema Room reads "{name}: {state}" {
  name:  Text
  state: State
}
`,
			doc:  "---\ntype: House\n---\n# Hi\n\n## Rooms\n\n- kitchen: good {#kitchen}\n- lounge: new {#lounge}\n",
			want: []string{`doc.kan:9:3: error: this item doesn't read as "{name}: {state}"`},
		},
		{
			name:   "a list of a schema without a pattern",
			schema: "schema Recipe {\n  title: Text @heading\n  ingredients: [Ingredient]\n}\n\nschema Ingredient {\n  name: Text\n}\n",
			doc:    head + "- flour {#flour}\n",
			want: []string{
				"doc.kan:8:1: error: `Ingredient` has no reads pattern, so a list can't hold it; write a table or one section per entry",
			},
		},
		{
			name:   "key lines and a paragraph",
			schema: recipe,
			doc:    head + "flour: 250 g\nsalt: 2 g\n\nSome flour.\n",
			want: []string{
				"doc.kan:8:1: error: `ingredients` is a list of `Ingredient`: write a table, a list or one section per entry",
				"doc.kan:11:1: error: `ingredients` is a list of `Ingredient`: write a table, a list or one section per entry",
			},
		},
		{
			name:   "a code block",
			schema: recipe,
			doc:    head + "```\nflour: 250 g\n```\n",
			want:   []string{"doc.kan:8:1: error: `ingredients` is a list of `Ingredient`: write a table, a list or one section per entry"},
		},
		{
			name:   "one section per entry",
			schema: recipe,
			doc:    head + "### Flour {#flour}\n\nquantity: 250 g\nname: flour\n\n### Salt\n\nquantity: 2 g\n",
			want: []string{
				"doc.kan:13:1: error: every object has an id: this `Ingredient` needs one; give it an id, as in ### Salt {#id}",
				"doc.kan:13:1: error: `Ingredient` needs `name`; write name: …",
			},
		},
	})
}

const stock = `
schema Stock {
  title: Text @heading
  items: [Item]
}

schema Item {
  name:  Text
  state: Text?
  tags:  [Text]?
}
`

func TestTables(t *testing.T) {
	head := "---\ntype: Stock\n---\n# Hi\n\n## Items\n\n"
	runCases(t, []lintCase{
		{
			name:   "a table",
			schema: stock,
			doc:    head + "| id | name | state |\n|---|---|---|\n| a | Chair | |\n| b | Table | good |\n",
		},
		{
			name:   "a column written twice, and a list column",
			schema: stock,
			doc:    head + "| id | name | name | tags |\n|---|---|---|---|\n| a | x | y | z |\n",
			want: []string{
				"doc.kan:8:15: error: the column `name` is written twice",
				"doc.kan:8:22: error: a cell holds one value, and `tags` is a list",
			},
		},
		{
			name:   "no id column, and a required column missing",
			schema: stock,
			doc:    head + "| state |\n|---|\n| good |\n",
			want: []string{
				"doc.kan:8:1: error: a table of `Item` needs an id column",
				"doc.kan:8:1: error: a table of `Item` needs a column `name`",
			},
		},
		{
			name:   "an empty required cell, a wrong id and a duplicate id",
			schema: stock,
			doc:    head + "| id | name |\n|---|---|\n| a |  |\n| B | Table |\n| a | Shelf |\n",
			want: []string{
				"doc.kan:10:6: error: `name` needs a value",
				`doc.kan:11:3: error: an id is a slug of lowercase ASCII letters, digits and hyphens: write b, not "B"`,
				"doc.kan:12:3: error: the id `a` is already used, at line 10",
			},
		},
		{
			name:   "a row wider than its header",
			schema: stock,
			doc:    head + "| id | name |\n|---|---|\n| a | Chair | extra |\n",
			want:   []string{"doc.kan:10:1: error: table row has 3 cells, header has 2"},
		},
		{
			name:   "a row narrower than its header",
			schema: stock,
			doc:    head + "| id | name |\n|---|---|\n| a |\n",
			want:   []string{"doc.kan:10:1: error: table row has 1 cell, header has 2"},
		},
		{
			name:   "an unknown column",
			schema: stock,
			doc:    head + "| id | name | states |\n|---|---|---|\n| a | Chair | good |\n",
			want:   []string{"doc.kan:8:15: error: `Item` has no field `states`; did you mean `state`?"},
		},
		{
			name: "fields a table can't hold",
			schema: `
schema Stock {
  title: Text @heading
  items: [Item]
}

schema Item {
  name:  Text
  tags:  [Text]
  place: Place
  notes: Prose?
}

schema Place {
  street: Text
}
`,
			doc: head + "| id | name | place | notes |\n|---|---|---|---|\n| a | Chair | [[x]] | In {name} |\n",
			want: []string{
				"doc.kan:8:1: error: a table can't hold `tags`, which `Item` needs: write one section per entry",
				"doc.kan:8:15: error: a cell holds a value or a link, and `place` holds its own `Place`: write one section per entry",
			},
		},
	})
}

func TestProse(t *testing.T) {
	head := "---\ntype: Note\n---\n# Hi\n\n## Body\n\n"
	runCases(t, []lintCase{
		{
			name:   "key lines and a subsection",
			schema: note,
			doc:    head + "note: see {dat}\n\n### More\n",
			want: []string{
				"doc.kan:8:12: error: {dat} inserts no field; did you mean {date}?",
				"doc.kan:10:1: error: `body` is prose and has no sections; use a list or bold text instead",
			},
		},
		{
			name:   "lists and tables",
			schema: note,
			doc:    head + "- on {dat}\n\n| a |\n|---|\n| {titl} |\n",
			want: []string{
				"doc.kan:8:7: error: {dat} inserts no field; did you mean {date}?",
				"doc.kan:12:4: error: {titl} inserts no field; did you mean {title}?",
			},
		},
		{
			name:   "unclosed link",
			schema: note,
			doc:    head + "See [[other and {zz}.\n",
			want:   []string{"doc.kan:8:5: error: unclosed link; close it with ]]"},
		},
		{
			name:   "unclosed brace",
			schema: note,
			doc:    head + "The {date and [[#zz]].\n",
			want:   []string{`doc.kan:8:5: error: unclosed {; close it with }, or write \{ for a brace`},
		},
		{
			name:   "escapes and code spans",
			schema: note,
			doc:    head + "Escaped \\{braces\\}, code `{x} [[y` and café{date}.\n",
		},
		{
			name:   "an insertion after a code span",
			schema: note,
			doc:    head + "The `code` then {zz}.\n",
			want:   []string{"doc.kan:8:18: error: {zz} inserts no field"},
		},
	})
}

const card = `
schema Card {
  title:  Text @heading
  date:   Date?
  author: Person?
  source: -> Person?
  tags:   [Text]?
}

schema Person {
  name: Text?
  bio:  Prose? @body
}
`

func TestKeyLines(t *testing.T) {
	head := "---\ntype: Card\n---\n# Hi\n\n"
	runCases(t, []lintCase{
		{
			name:   "an empty value",
			schema: card,
			doc:    head + "date:\nsource:\n",
			want: []string{
				"doc.kan:6:1: error: `date` has no value; write one, as in date: 2026-10-22",
				"doc.kan:7:1: error: `source` has no value; write one, as in source: [[…]]",
			},
		},
		{
			name:   "a list as a key",
			schema: card,
			doc:    head + "tags: a, b\n",
			want:   []string{"doc.kan:6:1: error: `tags` is a list: write it under its heading, ## Tags"},
		},
		{
			name:   "a required list as a key counts as written",
			schema: list,
			doc:    "---\ntype: List\n---\n# Hi\n\nsteps: 1, 2\n",
			want:   []string{"doc.kan:6:1: error: `steps` is a list: write it under its heading, ## Steps"},
		},
		{
			name:   "an id as a key",
			schema: card,
			doc:    head + "id: hi\n",
			want:   []string{"doc.kan:6:1: error: an object's id is written in its heading ({#id}) or its file name, not as a key"},
		},
		{
			name:   "a field written twice",
			schema: card,
			doc:    head + "date: 2026-10-22\ndate: 2026-10-23\n",
			want:   []string{"doc.kan:7:1: error: `date` is already written, at line 6"},
		},
		{
			name:   "a misspelled key, then the right one",
			schema: card,
			doc:    head + "dates: 2026-10-22\ndate: 2026-10-23\n",
			want:   []string{"doc.kan:6:1: error: `Card` has no field `dates`; did you mean `date`?"},
		},
		{
			name:   "a sub-object written as a link, then under its heading",
			schema: card,
			doc:    head + "author: [[#sarah]]\n\n## Author {#sarah}\n",
			want: []string{
				"doc.kan:6:1: error: `author` holds its own `Person`, not a link: write it under ## Author {#id}",
			},
		},
		{
			name:   "a sub-object written twice",
			schema: card,
			doc:    head + "## Author {#sarah}\n\n## Author {#paul}\n",
			want:   []string{"doc.kan:8:4: error: `author` is already written, at line 6"},
		},
	})
}

func TestInsertions(t *testing.T) {
	head := "---\ntype: Card\n---\n# Hi\n\ndate: 2026-10-22\n\n## Author {#sarah}\n\n"
	runCases(t, []lintCase{
		{
			name:   "a field of the object that holds it",
			schema: card,
			doc:    head + "Café notes of {date.year}, about {title}, by {name}.\n",
		},
		{
			name:   "a field of no object",
			schema: card,
			doc:    head + "Café notes of {day}, by {nme}.\n",
			want: []string{
				"doc.kan:10:16: error: {day} inserts no field",
				"doc.kan:10:26: error: {nme} inserts no field; did you mean {name}?",
			},
		},
	})
}

func TestIDs(t *testing.T) {
	runCases(t, []lintCase{
		{
			name:   "the id of the file",
			schema: card,
			doc:    "---\ntype: Card\n---\n# Hi\n\n## Author {#doc}\n",
			want:   []string{"doc.kan:6:13: error: the id `doc` is already used, as doc.kan"},
		},
		{
			name: "an id used twice",
			schema: `
schema Lease {
  title:    Text @heading
  landlord: Person
  tenant:   Person
}

schema Person {
  name: Text?
}
`,
			doc:  "---\ntype: Lease\n---\n# Hi\n\n## Landlord {#sarah}\n\n## Tenant {#sarah}\n",
			want: []string{"doc.kan:8:13: error: the id `sarah` is already used, at line 6"},
		},
	})
}

const menu = `
enum Style { flambe, saute, puree }

schema Menu {
  title:   Text @heading
  entrees: Int
  roses:   Int?
  style:   Style?
  service: Service?
  price:   Money?
  creme-brulee: Money?
  body:    Prose?
  lines:   [Line]?
  courses: [Course] @sections
}

enum Service { included }

schema Line {
  name:    Text
  entrees: Int
}

schema Course {
  title: Text @heading
  body:  Prose @body
}
`

func TestSections(t *testing.T) {
	head := "---\ntype: Menu\n---\n# Menu\n\nentrees: 2\n\n"
	runCases(t, []lintCase{
		{
			name:   "sections",
			schema: menu,
			doc:    head + "## Café {#cafe}\n\nThree blends.\n\n## Crêpes {#crepes}\n\nWith a sauce.\n",
		},
		{
			name:   "no section",
			schema: menu,
			doc:    head,
		},
		{
			name:   "a heading that names a scalar field",
			schema: menu,
			doc:    head + "## Price {#price}\n\nSome text.\n",
			want: []string{
				`doc.kan:8:4: error: "Price" names the field ` + "`price`, a `Money` written as a key line: price: 1 250.50 EUR; to start a section, give it another title",
			},
		},
		{
			name:   "a heading with accents names a scalar field",
			schema: menu,
			doc:    head + "## Crème  brûlée {#creme}\n\nSome text.\n",
			want: []string{
				`doc.kan:8:4: error: "Crème  brûlée" names the field ` + "`creme-brulee`, a `Money` written as a key line: creme-brulee: 1 250.50 EUR; to start a section, give it another title",
			},
		},
		{
			name:   "a section without its body",
			schema: menu,
			doc:    head + "## Café {#cafe}\n",
			want:   []string{"doc.kan:8:1: error: `Course` needs `body`; write it as text under the heading"},
		},
	})
}

func TestNamesAndValues(t *testing.T) {
	head := "---\ntype: Menu\n---\n# Menu\n\n"
	runCases(t, []lintCase{
		{
			name:   "a key with accents",
			schema: menu,
			doc:    head + "entrées: 2\n",
			want:   []string{`doc.kan:6:1: error: names are lowercase ASCII: write entrees, not "entrées"`},
		},
		{
			name:   "a name with accents as a key",
			schema: menu,
			doc:    head + "entrees: 2\nrosés: 3\n",
			want:   []string{`doc.kan:7:1: error: names are lowercase ASCII: write roses, not "rosés"`},
		},
		{
			name:   "an unknown key",
			schema: menu,
			doc:    head + "entrees: 2\nprices: 12.00 EUR\nzz: 1\n",
			want: []string{
				"doc.kan:7:1: error: `Menu` has no field `prices`; did you mean `price`?",
				"doc.kan:8:1: error: `Menu` has no field `zz`",
			},
		},
		{
			name:   "a title as a column",
			schema: menu,
			doc:    head + "entrees: 2\n\n## Lines\n\n| id | name | Entrées |\n|---|---|---|\n| a | Soup | 2 |\n",
			want:   []string{`doc.kan:10:15: error: names are lowercase ASCII: write entrees, not "Entrées"`},
		},
		{
			name:   "enum values",
			schema: menu,
			doc:    head + "entrees: 2\nstyle: sauté\n",
			want:   []string{`doc.kan:7:8: error: write saute, not "sauté"`},
		},
		{
			name:   "an enum value close to another",
			schema: menu,
			doc:    head + "entrees: 2\nstyle: flamb\n",
			want:   []string{`doc.kan:7:8: error: "flamb" is not a ` + "`Style`; did you mean flambe?"},
		},
		{
			name:   "an enum value far from all",
			schema: menu,
			doc:    head + "entrees: 2\nstyle: baked\nservice: extra\n",
			want: []string{
				`doc.kan:7:8: error: "baked" is not a ` + "`Style`: write flambe, saute or puree",
				`doc.kan:8:10: error: "extra" is not a ` + "`Service`: write included",
			},
		},
		{
			name:   "prose in a key line",
			schema: menu,
			doc:    head + "entrees: 2\nbody: with {entree}\n",
			want:   []string{"doc.kan:7:13: error: {entree} inserts no field; did you mean {entrees}?"},
		},
	})
}

const required = `
schema Card {
  title:  Text @heading
  author: Person
  lines:  [Line]
  steps:  [Text]
  body:   Prose
  date:   Date
}

schema Person {
  name:  Text @heading
  notes: [Text]
  bio:   Prose
}

schema Line {
  name: Text
}
`

func TestRequire(t *testing.T) {
	runCases(t, []lintCase{
		{
			name:   "in a file",
			schema: required,
			doc:    "---\ntype: Card\n---\n",
			want: []string{
				"doc.kan: error: `Card` needs `title`; write it as the title: # …",
				"doc.kan: error: `Card` needs `author`; write it under ## Author {#id}",
				"doc.kan: error: `Card` needs `lines`; write it under ## Lines, or as the folder doc/lines/",
				"doc.kan: error: `Card` needs `steps`; write it under ## Steps",
				"doc.kan: error: `Card` needs `body`; write it under ## Body",
				"doc.kan: error: `Card` needs `date`; write date: 2026-10-22",
			},
		},
		{
			name:   "in an object under a heading",
			schema: required,
			doc:    "---\ntype: Card\n---\n# Hi\n\ndate: 2026-10-22\n\n## Author {#sarah}\n\n## Lines\n\n## Steps\n\n## Body\n",
			want: []string{
				"doc.kan:8:1: error: `Person` needs `name`; write name: …",
				"doc.kan:8:1: error: `Person` needs `notes`; write it under ### Notes",
				"doc.kan:8:1: error: `Person` needs `bio`; write it under ### Bio",
			},
		},
		{
			name:   "in an entry of a list",
			schema: required,
			doc:    "---\ntype: Card\n---\n# Hi\n\n## Lines\n\n| id | name |\n|---|---|\n| a |  |\n",
			want: []string{
				"doc.kan: error: `Card` needs `author`; write it under ## Author {#id}",
				"doc.kan: error: `Card` needs `steps`; write it under ## Steps",
				"doc.kan: error: `Card` needs `body`; write it under ## Body",
				"doc.kan: error: `Card` needs `date`; write date: 2026-10-22",
				"doc.kan:10:6: error: `name` needs a value",
			},
		},
		{
			name:   "a body in a file",
			schema: "schema Clause {\n  title: Text @heading\n  body: Prose @body\n}\n",
			doc:    "---\ntype: Clause\n---\n# Hi\n",
			want:   []string{"doc.kan: error: `Clause` needs `body`; write it as text under the title"},
		},
	})
}

func TestExamples(t *testing.T) {
	r := lint(t, files{
		"schema.kanonarch": `
enum Condition { new, good, worn }

schema All {
  a: Text
  b: Int
  c: Number
  d: Bool
  e: Date
  f: Duration
  g: Money
  h: Percent
  i: Quantity
  j: Version
  k: Condition
  l: DateTime
  m: File
  n: Image
  o: Code
  p: [Image]
  q: -> All
  r: [-> All]
}
`,
		"doc.kan": "---\ntype: All\n---\n",
	})
	check(t, r, []string{
		"doc.kan: error: `All` needs `a`; write a: …",
		"doc.kan: error: `All` needs `b`; write b: 4",
		"doc.kan: error: `All` needs `c`; write c: 12.5",
		"doc.kan: error: `All` needs `d`; write d: yes or no",
		"doc.kan: error: `All` needs `e`; write e: 2026-10-22",
		"doc.kan: error: `All` needs `f`; write f: 3 years",
		"doc.kan: error: `All` needs `g`; write g: 1 250.50 EUR",
		"doc.kan: error: `All` needs `h`; write h: 20 %",
		"doc.kan: error: `All` needs `i`; write i: 48 m2",
		"doc.kan: error: `All` needs `j`; write j: 1.2",
		"doc.kan: error: `All` needs `k`; write k: new, good or worn",
		"doc.kan: error: `All` needs `l`; write l: 2026-10-22 14:30",
		"doc.kan: error: `All` needs `m`; write m: [[plan.pdf]]",
		"doc.kan: error: `All` needs `n`; write n: [[photo.jpg]]",
		"doc.kan: error: `All` needs `o`; write it under ## O as a code block, or link to its file: o: [[script.sh]]",
		"doc.kan: error: `All` needs `p`; write it under ## P, or as the folder doc/p/",
		"doc.kan: error: `All` needs `q`; write q: [[…]]",
		"doc.kan: error: `All` needs `r`; write r: [[…]], [[…]]",
	})
}

const levels = `
schema Card {
  title:  Text @heading
  author: Person?
}

schema Person {
  name:  Text?
  notes: [Text]?
  bio:   Prose?
  also:  [Person]?
}
`

func TestLevels(t *testing.T) {
	head := "---\ntype: Card\n---\n# Hi\n\n## Author {#sarah}\n\n"
	runCases(t, []lintCase{
		{
			name:   "the headings a sub-object takes",
			schema: levels,
			doc:    head + "### Misc\n",
			want:   []string{"doc.kan:8:5: error: unknown heading \"Misc\"; `Person` takes ### Notes, ### Bio, ### Also"},
		},
		{
			name:   "text and a list as a key in a sub-object",
			schema: levels,
			doc:    head + "notes: a, b\n\nSome text.\n",
			want: []string{
				"doc.kan:8:1: error: `notes` is a list: write it under its heading, ### Notes",
				"doc.kan:10:1: error: this text fills no field; write it under ### Bio",
			},
		},
		{
			name:   "an entry without an id, in a sub-object",
			schema: levels,
			doc:    head + "### Also\n\n#### Paul\n\n##### Zz\n",
			want: []string{
				"doc.kan:10:1: error: every object has an id: this `Person` needs one; give it an id, as in #### Paul {#id}",
				"doc.kan:12:7: error: unknown heading \"Zz\"; `Person` takes ##### Notes, ##### Bio, ##### Also",
			},
		},
	})
}
