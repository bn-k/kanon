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
			doc:    "---\ntype: Note\n---\ndate: 2026-10-22\n\n## Corps\n\nAvant le titre.\n\n# Hi\n",
			want:   []string{"doc.kan:4:1: error: this comes before the title; move it under # Hi"},
		},
		{
			name:   "a section before the title",
			schema: note,
			doc:    "---\ntype: Note\n---\n## Corps\n\nAvant le titre.\n\n# Hi\n",
			want:   []string{"doc.kan:4:1: error: this comes before the title; move it under # Hi"},
		},
		{
			name:   "text before the title",
			schema: "schema Clause {\n  titre: Text @heading\n  corps: Prose @body\n}\n",
			doc:    "---\ntype: Clause\n---\nAvant le titre.\n\n# Hi\n",
			want:   []string{"doc.kan:4:1: error: this comes before the title; move it under # Hi"},
		},
		{
			name:   "no title",
			schema: note,
			doc:    "---\ntype: Note\n---\ndate: 2026-10-22\n",
			want:   []string{"doc.kan: error: `Note` needs `titre`; write it as the title: # …"},
		},
	})
}

func TestStrayText(t *testing.T) {
	runCases(t, []lintCase{
		{
			name:   "with prose fields",
			schema: "schema Note {\n  titre: Text @heading\n  corps: Prose?\n  notes: Prose?\n}\n",
			doc:    "---\ntype: Note\n---\n# Hi\n\nDu texte.\n",
			want:   []string{"doc.kan:6:1: error: this text fills no field; write it under ## Corps or ## Notes"},
		},
		{
			name:   "without a prose field",
			schema: "schema Tag {\n  nom: Text @heading\n}\n",
			doc:    "---\ntype: Tag\n---\n# Hi\n\nDu texte.\n",
			want:   []string{"doc.kan:6:1: error: this text fills no field: `Tag` has no field for free text"},
		},
	})
}

func TestHeadings(t *testing.T) {
	runCases(t, []lintCase{
		{
			name:   "unknown heading with a suggestion",
			schema: note,
			doc:    "---\ntype: Note\n---\n# Hi\n\n## Corp\n",
			want:   []string{`doc.kan:6:4: error: unknown heading "Corp"; did you mean "Corps"?`},
		},
		{
			name:   "unknown heading, and the headings the schema takes",
			schema: "schema Note {\n  titre: Text @heading\n  corps: Prose?\n  tags:  [Text]?\n}\n",
			doc:    "---\ntype: Note\n---\n# Hi\n\n## Divers\n",
			want:   []string{"doc.kan:6:4: error: unknown heading \"Divers\"; `Note` takes ## Corps, ## Tags"},
		},
		{
			name:   "unknown heading in a schema that takes none",
			schema: "schema Tag {\n  nom: Text @heading\n}\n",
			doc:    "---\ntype: Tag\n---\n# Hi\n\n## Divers\n",
			want:   []string{`doc.kan:6:4: error: unknown heading "Divers"`},
		},
		{
			name:   "an id on a heading that is not an object",
			schema: note,
			doc:    "---\ntype: Note\n---\n# Hi\n\n## Corps {#c}\n\nDu texte.\n",
			want:   []string{"doc.kan:6:12: error: only an object has an id, and `corps` is a `Prose`; remove {#c}"},
		},
		{
			name:   "the label of a scalar field",
			schema: note,
			doc:    "---\ntype: Note\n---\n# Hi\n\n## Date\n",
			want:   []string{"doc.kan:6:4: error: \"Date\" is the label of `date`, a `Date` written as a key line: date: 2026-10-22"},
		},
	})
}

const liste = `
schema Liste {
  titre:  Text @heading
  etapes: [Int]
}
`

func TestValues(t *testing.T) {
	head := "---\ntype: Liste\n---\n# Hi\n\n## Etapes\n\n"
	runCases(t, []lintCase{
		{
			name:   "a list of values",
			schema: liste,
			doc:    head + "1. 4\n2. 12\n",
		},
		{
			name:   "key lines",
			schema: liste,
			doc:    head + "nb: 3\nx: 4\n",
			want:   []string{"doc.kan:8:1: error: `etapes` is a list of `Int`: write one item per value"},
		},
		{
			name:   "a paragraph",
			schema: liste,
			doc:    head + "Quatre.\n",
			want:   []string{"doc.kan:8:1: error: `etapes` is a list of `Int`: write one item per value"},
		},
		{
			name:   "an id on an item, and a wrong value",
			schema: liste,
			doc:    head + "- 1 {#un}\n- deux\n",
			want: []string{
				"doc.kan:8:7: error: an `Int` is a value, not an object: it has no id; remove {#un}",
				`doc.kan:9:3: error: write a whole number (4 or 1 250), not "deux"`,
			},
		},
		{
			name:   "a subsection",
			schema: liste,
			doc:    head + "- 1\n\n### Plus\n",
			want:   []string{"doc.kan:10:1: error: `etapes` is a list of `Int`: write one item per value"},
		},
		{
			name:   "an id on the heading",
			schema: liste,
			doc:    "---\ntype: Liste\n---\n# Hi\n\n## Etapes {#e}\n\n- 1\n",
			want:   []string{"doc.kan:6:13: error: only an object has an id, and `etapes` is a list of `Int`; remove {#e}"},
		},
	})
}

const recette = `
schema Recette {
  titre:       Text @heading
  ingredients: [Ingredient]
}

schema Ingredient reads "{quantite} [de] {nom}" {
  quantite: Quantity
  nom:      Text
}
`

func TestObjects(t *testing.T) {
	head := "---\ntype: Recette\n---\n# Crêpes\n\n## Ingredients\n\n"
	runCases(t, []lintCase{
		{
			name:   "a list read through a pattern",
			schema: recette,
			doc:    head + "- 250 g de farine {#farine}\n- farine {#sel}\n- 2 g de sel\n",
			want: []string{
				`doc.kan:9:3: error: this item doesn't read as "{quantite} [de] {nom}"`,
				"doc.kan:10:1: error: every object has an id: this `Ingredient` needs one; write its id at the end, as in - 2 g de sel {#id}",
			},
		},
		{
			name: "a list read through a pattern with an enum",
			schema: `
schema Maison {
  titre:  Text @heading
  pieces: [Piece]
}

enum Etat { bon, use }

schema Piece reads "{nom}: {etat}" {
  nom:  Text
  etat: Etat
}
`,
			doc:  "---\ntype: Maison\n---\n# Hi\n\n## Pieces\n\n- cuisine: bon {#cuisine}\n- salon: neuf {#salon}\n",
			want: []string{`doc.kan:9:3: error: this item doesn't read as "{nom}: {etat}"`},
		},
		{
			name:   "a list of a schema without a pattern",
			schema: "schema Recette {\n  titre: Text @heading\n  ingredients: [Ingredient]\n}\n\nschema Ingredient {\n  nom: Text\n}\n",
			doc:    head + "- farine {#farine}\n",
			want: []string{
				"doc.kan:8:1: error: `Ingredient` has no reads pattern, so a list can't hold it; write a table or one section per entry",
			},
		},
		{
			name:   "key lines and a paragraph",
			schema: recette,
			doc:    head + "farine: 250 g\nsel: 2 g\n\nDe la farine.\n",
			want: []string{
				"doc.kan:8:1: error: `ingredients` is a list of `Ingredient`: write a table, a list or one section per entry",
				"doc.kan:11:1: error: `ingredients` is a list of `Ingredient`: write a table, a list or one section per entry",
			},
		},
		{
			name:   "a code block",
			schema: recette,
			doc:    head + "```\nfarine: 250 g\n```\n",
			want:   []string{"doc.kan:8:1: error: `ingredients` is a list of `Ingredient`: write a table, a list or one section per entry"},
		},
		{
			name:   "one section per entry",
			schema: recette,
			doc:    head + "### Farine {#farine}\n\nquantite: 250 g\nnom: farine\n\n### Sel\n\nquantite: 2 g\n",
			want: []string{
				"doc.kan:13:1: error: every object has an id: this `Ingredient` needs one; give it an id, as in ### Sel {#id}",
				"doc.kan:13:1: error: `Ingredient` needs `nom`; write nom: …",
			},
		},
	})
}

const stock = `
schema Stock {
  titre:  Text @heading
  pieces: [Piece]
}

schema Piece {
  nom:  Text
  etat: Text?
  tags: [Text]?
}
`

func TestTables(t *testing.T) {
	head := "---\ntype: Stock\n---\n# Hi\n\n## Pieces\n\n"
	runCases(t, []lintCase{
		{
			name:   "a table",
			schema: stock,
			doc:    head + "| id | nom | etat |\n|---|---|---|\n| a | Cuisine | |\n| b | Salon | bon |\n",
		},
		{
			name:   "a column written twice, and a list column",
			schema: stock,
			doc:    head + "| id | nom | nom | tags |\n|---|---|---|---|\n| a | x | y | z |\n",
			want: []string{
				"doc.kan:8:14: error: the column `nom` is written twice",
				"doc.kan:8:20: error: a cell holds one value, and `tags` is a list",
			},
		},
		{
			name:   "no id column, and a required column missing",
			schema: stock,
			doc:    head + "| etat |\n|---|\n| bon |\n",
			want: []string{
				"doc.kan:8:1: error: a table of `Piece` needs an id column",
				"doc.kan:8:1: error: a table of `Piece` needs a column `nom`",
			},
		},
		{
			name:   "an empty required cell, a wrong id and a duplicate id",
			schema: stock,
			doc:    head + "| id | nom |\n|---|---|\n| a |  |\n| B | Salon |\n| a | Cave |\n",
			want: []string{
				"doc.kan:10:6: error: `nom` needs a value",
				`doc.kan:11:3: error: an id is a slug of lowercase ASCII letters, digits and hyphens: write b, not "B"`,
				"doc.kan:12:3: error: the id `a` is already used, at line 10",
			},
		},
		{
			name:   "a row wider than its header",
			schema: stock,
			doc:    head + "| id | nom |\n|---|---|\n| a | Cuisine | en trop |\n",
			want:   []string{"doc.kan:10:1: error: table row has 3 cells, header has 2"},
		},
		{
			name:   "a row narrower than its header",
			schema: stock,
			doc:    head + "| id | nom |\n|---|---|\n| a |\n",
			want:   []string{"doc.kan:10:1: error: table row has 1 cell, header has 2"},
		},
		{
			name:   "an unknown column",
			schema: stock,
			doc:    head + "| id | nom | etats |\n|---|---|---|\n| a | Cuisine | bon |\n",
			want:   []string{"doc.kan:8:14: error: `Piece` has no field `etats`; did you mean `etat`?"},
		},
		{
			name: "fields a table can't hold",
			schema: `
schema Stock {
  titre:  Text @heading
  pieces: [Piece]
}

schema Piece {
  nom:   Text
  tags:  [Text]
  lieu:  Lieu
  notes: Prose?
}

schema Lieu {
  rue: Text
}
`,
			doc: head + "| id | nom | lieu | notes |\n|---|---|---|---|\n| a | Cuisine | [[x]] | Au {nom} |\n",
			want: []string{
				"doc.kan:8:1: error: a table can't hold `tags`, which `Piece` needs: write one section per entry",
				"doc.kan:8:14: error: a cell holds a value or a link, and `lieu` holds its own `Lieu`: write one section per entry",
			},
		},
	})
}

func TestProse(t *testing.T) {
	head := "---\ntype: Note\n---\n# Hi\n\n## Corps\n\n"
	runCases(t, []lintCase{
		{
			name:   "key lines and a subsection",
			schema: note,
			doc:    head + "note: voir {dat}\n\n### Plus\n",
			want: []string{
				"doc.kan:8:13: error: {dat} inserts no field; did you mean {date}?",
				"doc.kan:10:1: error: `corps` is prose and has no sections; use a list or bold text instead",
			},
		},
		{
			name:   "lists and tables",
			schema: note,
			doc:    head + "- le {dat}\n\n| a |\n|---|\n| {titr} |\n",
			want: []string{
				"doc.kan:8:7: error: {dat} inserts no field; did you mean {date}?",
				"doc.kan:12:4: error: {titr} inserts no field; did you mean {titre}?",
			},
		},
		{
			name:   "unclosed link",
			schema: note,
			doc:    head + "Voir [[autre et {zz}.\n",
			want:   []string{"doc.kan:8:6: error: unclosed link; close it with ]]"},
		},
		{
			name:   "unclosed brace",
			schema: note,
			doc:    head + "Le {date et [[#zz]].\n",
			want:   []string{`doc.kan:8:4: error: unclosed {; close it with }, or write \{ for a brace`},
		},
		{
			name:   "escapes and code spans",
			schema: note,
			doc:    head + "Une \\{accolade\\}, du code `{x} [[y` et é{date}.\n",
		},
		{
			name:   "an insertion after a code span",
			schema: note,
			doc:    head + "Le `code` puis {zz}.\n",
			want:   []string{"doc.kan:8:17: error: {zz} inserts no field"},
		},
	})
}

const fiche = `
schema Fiche {
  titre:  Text @heading
  date:   Date?
  auteur: Personne?
  source: -> Personne?
  tags:   [Text]?
}

schema Personne {
  nom: Text?
  bio: Prose? @body
}
`

func TestKeyLines(t *testing.T) {
	head := "---\ntype: Fiche\n---\n# Hi\n\n"
	runCases(t, []lintCase{
		{
			name:   "an empty value",
			schema: fiche,
			doc:    head + "date:\nsource:\n",
			want: []string{
				"doc.kan:6:1: error: `date` has no value; write one, as in date: 2026-10-22",
				"doc.kan:7:1: error: `source` has no value; write one, as in source: [[…]]",
			},
		},
		{
			name:   "a list as a key",
			schema: fiche,
			doc:    head + "tags: a, b\n",
			want:   []string{"doc.kan:6:1: error: `tags` is a list: write it under its heading, ## Tags"},
		},
		{
			name:   "a required list as a key counts as written",
			schema: liste,
			doc:    "---\ntype: Liste\n---\n# Hi\n\netapes: 1, 2\n",
			want:   []string{"doc.kan:6:1: error: `etapes` is a list: write it under its heading, ## Etapes"},
		},
		{
			name:   "an id as a key",
			schema: fiche,
			doc:    head + "id: hi\n",
			want:   []string{"doc.kan:6:1: error: an object's id is written in its heading ({#id}) or its file name, not as a key"},
		},
		{
			name:   "a field written twice",
			schema: fiche,
			doc:    head + "date: 2026-10-22\ndate: 2026-10-23\n",
			want:   []string{"doc.kan:7:1: error: `date` is already written, at line 6"},
		},
		{
			name:   "a misspelled key, then the right one",
			schema: fiche,
			doc:    head + "dates: 2026-10-22\ndate: 2026-10-23\n",
			want:   []string{"doc.kan:6:1: error: `Fiche` has no field `dates`; did you mean `date`?"},
		},
		{
			name:   "a sub-object written as a link, then under its heading",
			schema: fiche,
			doc:    head + "auteur: [[#claire]]\n\n## Auteur {#claire}\n",
			want: []string{
				"doc.kan:6:1: error: `auteur` holds its own `Personne`, not a link: write it under ## Auteur {#id}",
			},
		},
		{
			name:   "a sub-object written twice",
			schema: fiche,
			doc:    head + "## Auteur {#claire}\n\n## Auteur {#paul}\n",
			want:   []string{"doc.kan:8:4: error: `auteur` is already written, at line 6"},
		},
	})
}

func TestInsertions(t *testing.T) {
	head := "---\ntype: Fiche\n---\n# Hi\n\ndate: 2026-10-22\n\n## Auteur {#claire}\n\n"
	runCases(t, []lintCase{
		{
			name:   "a field of the object that holds it",
			schema: fiche,
			doc:    head + "Écrit le {date.annee}, à propos de {titre}, par {nom}.\n",
		},
		{
			name:   "a field of no object",
			schema: fiche,
			doc:    head + "Écrit le {jour}, par {nm}.\n",
			want: []string{
				"doc.kan:10:11: error: {jour} inserts no field",
				"doc.kan:10:23: error: {nm} inserts no field; did you mean {nom}?",
			},
		},
	})
}

func TestIDs(t *testing.T) {
	runCases(t, []lintCase{
		{
			name:   "the id of the file",
			schema: fiche,
			doc:    "---\ntype: Fiche\n---\n# Hi\n\n## Auteur {#doc}\n",
			want:   []string{"doc.kan:6:13: error: the id `doc` is already used, as doc.kan"},
		},
		{
			name: "an id used twice",
			schema: `
schema Bail {
  titre:     Text @heading
  bailleur:  Personne
  locataire: Personne
}

schema Personne {
  nom: Text?
}
`,
			doc:  "---\ntype: Bail\n---\n# Hi\n\n## Bailleur {#claire}\n\n## Locataire {#claire}\n",
			want: []string{"doc.kan:8:16: error: the id `claire` is already used, at line 6"},
		},
	})
}

const bail = `
enum Etat { neuf, bon, use "usé" }

schema Bail {
  titre:   Text @heading
  cles:    Int "clés"
  pieces:  Int?
  etat:    Etat?
  seul:    Seul?
  loyer:   Money?
  corps:   Prose?
  lignes:  [Ligne]?
  clauses: [Clause] @sections
}

enum Seul { oui }

schema Ligne {
  nom:   Text
  cles:  Int "Clés"
}

schema Clause {
  titre: Text @heading
  corps: Prose @body
}
`

func TestSections(t *testing.T) {
	head := "---\ntype: Bail\n---\n# Bail\n\ncles: 2\n\n"
	runCases(t, []lintCase{
		{
			name:   "sections",
			schema: bail,
			doc:    head + "## Durée {#duree}\n\nTrois ans.\n\n## Congé {#conge}\n\nAvec un préavis.\n",
		},
		{
			name:   "no section",
			schema: bail,
			doc:    head,
		},
		{
			name:   "the label of a scalar field",
			schema: bail,
			doc:    head + "## Loyer {#loyer}\n\nDu texte.\n",
			want: []string{
				`doc.kan:8:4: error: "Loyer" is the label of ` + "`loyer`, a `Money` written as a key line: loyer: 1 250.50 EUR; to start a section, give it another title",
			},
		},
		{
			name:   "a section without its body",
			schema: bail,
			doc:    head + "## Durée {#duree}\n",
			want:   []string{"doc.kan:8:1: error: `Clause` needs `corps`; write it as text under the heading"},
		},
	})
}

func TestNamesAndValues(t *testing.T) {
	head := "---\ntype: Bail\n---\n# Bail\n\n"
	runCases(t, []lintCase{
		{
			name:   "a label as a key",
			schema: bail,
			doc:    head + "clés: 2\n",
			want:   []string{`doc.kan:6:1: error: a key or a column is a name, not a label: write cles, not "clés"`},
		},
		{
			name:   "a name with accents as a key",
			schema: bail,
			doc:    head + "cles: 2\npièces: 3\n",
			want:   []string{`doc.kan:7:1: error: names are ASCII: write pieces, not "pièces"`},
		},
		{
			name:   "an unknown key",
			schema: bail,
			doc:    head + "cles: 2\nloyers: 850.00 EUR\nzz: 1\n",
			want: []string{
				"doc.kan:7:1: error: `Bail` has no field `loyers`; did you mean `loyer`?",
				"doc.kan:8:1: error: `Bail` has no field `zz`",
			},
		},
		{
			name:   "a label as a column",
			schema: bail,
			doc:    head + "cles: 2\n\n## Lignes\n\n| id | nom | Clés |\n|---|---|---|\n| a | Cuisine | 2 |\n",
			want:   []string{`doc.kan:10:14: error: a key or a column is a name, not a label: write cles, not "Clés"`},
		},
		{
			name:   "enum values",
			schema: bail,
			doc:    head + "cles: 2\netat: usé\n",
			want:   []string{`doc.kan:7:7: error: write use, not "usé"`},
		},
		{
			name:   "an enum value close to another",
			schema: bail,
			doc:    head + "cles: 2\netat: neu\n",
			want:   []string{`doc.kan:7:7: error: "neu" is not an ` + "`Etat`; did you mean neuf?"},
		},
		{
			name:   "an enum value far from all",
			schema: bail,
			doc:    head + "cles: 2\netat: correct\nseul: non\n",
			want: []string{
				`doc.kan:7:7: error: "correct" is not an ` + "`Etat`: write neuf, bon or use",
				`doc.kan:8:7: error: "non" is not a ` + "`Seul`: write oui",
			},
		},
		{
			name:   "prose in a key line",
			schema: bail,
			doc:    head + "cles: 2\ncorps: avec {cle}\n",
			want:   []string{"doc.kan:7:14: error: {cle} inserts no field; did you mean {cles}?"},
		},
	})
}

const requis = `
schema Fiche {
  titre:  Text @heading
  auteur: Personne
  lignes: [Ligne]
  etapes: [Text]
  corps:  Prose
  date:   Date
}

schema Personne {
  nom:   Text @heading
  notes: [Text]
  bio:   Prose
}

schema Ligne {
  nom: Text
}
`

func TestRequire(t *testing.T) {
	runCases(t, []lintCase{
		{
			name:   "in a file",
			schema: requis,
			doc:    "---\ntype: Fiche\n---\n",
			want: []string{
				"doc.kan: error: `Fiche` needs `titre`; write it as the title: # …",
				"doc.kan: error: `Fiche` needs `auteur`; write it under ## Auteur {#id}",
				"doc.kan: error: `Fiche` needs `lignes`; write it under ## Lignes, or as the folder doc/lignes/",
				"doc.kan: error: `Fiche` needs `etapes`; write it under ## Etapes",
				"doc.kan: error: `Fiche` needs `corps`; write it under ## Corps",
				"doc.kan: error: `Fiche` needs `date`; write date: 2026-10-22",
			},
		},
		{
			name:   "in an object under a heading",
			schema: requis,
			doc:    "---\ntype: Fiche\n---\n# Hi\n\ndate: 2026-10-22\n\n## Auteur {#claire}\n\n## Lignes\n\n## Etapes\n\n## Corps\n",
			want: []string{
				"doc.kan:8:1: error: `Personne` needs `nom`; write nom: …",
				"doc.kan:8:1: error: `Personne` needs `notes`; write it under ### Notes",
				"doc.kan:8:1: error: `Personne` needs `bio`; write it under ### Bio",
			},
		},
		{
			name:   "in an entry of a list",
			schema: requis,
			doc:    "---\ntype: Fiche\n---\n# Hi\n\n## Lignes\n\n| id | nom |\n|---|---|\n| a |  |\n",
			want: []string{
				"doc.kan: error: `Fiche` needs `auteur`; write it under ## Auteur {#id}",
				"doc.kan: error: `Fiche` needs `etapes`; write it under ## Etapes",
				"doc.kan: error: `Fiche` needs `corps`; write it under ## Corps",
				"doc.kan: error: `Fiche` needs `date`; write date: 2026-10-22",
				"doc.kan:10:6: error: `nom` needs a value",
			},
		},
		{
			name:   "a body in a file",
			schema: "schema Clause {\n  titre: Text @heading\n  corps: Prose @body\n}\n",
			doc:    "---\ntype: Clause\n---\n# Hi\n",
			want:   []string{"doc.kan: error: `Clause` needs `corps`; write it as text under the title"},
		},
	})
}

func TestExamples(t *testing.T) {
	r := lint(t, files{
		"schema.kanonarch": `
enum Etat { neuf, bon, use }

schema Tout {
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
  k: Etat
  l: DateTime
  m: File
  n: Image
  o: Code
  p: [Image]
  q: -> Tout
  r: [-> Tout]
}
`,
		"doc.kan": "---\ntype: Tout\n---\n",
	})
	check(t, r, []string{
		"doc.kan: error: `Tout` needs `a`; write a: …",
		"doc.kan: error: `Tout` needs `b`; write b: 4",
		"doc.kan: error: `Tout` needs `c`; write c: 12.5",
		"doc.kan: error: `Tout` needs `d`; write d: yes or no",
		"doc.kan: error: `Tout` needs `e`; write e: 2026-10-22",
		"doc.kan: error: `Tout` needs `f`; write f: 3 years",
		"doc.kan: error: `Tout` needs `g`; write g: 1 250.50 EUR",
		"doc.kan: error: `Tout` needs `h`; write h: 20 %",
		"doc.kan: error: `Tout` needs `i`; write i: 48 m2",
		"doc.kan: error: `Tout` needs `j`; write j: 1.2",
		"doc.kan: error: `Tout` needs `k`; write k: neuf, bon or use",
		"doc.kan: error: `Tout` needs `l`; write l: 2026-10-22 14:30",
		"doc.kan: error: `Tout` needs `m`; write m: [[plan.pdf]]",
		"doc.kan: error: `Tout` needs `n`; write n: [[photo.jpg]]",
		"doc.kan: error: `Tout` needs `o`; write it under ## O as a code block, or link to its file: o: [[script.sh]]",
		"doc.kan: error: `Tout` needs `p`; write it under ## P, or as the folder doc/p/",
		"doc.kan: error: `Tout` needs `q`; write q: [[…]]",
		"doc.kan: error: `Tout` needs `r`; write r: [[…]], [[…]]",
	})
}

const niveaux = `
schema Fiche {
  titre:  Text @heading
  auteur: Personne?
}

schema Personne {
  nom:   Text?
  notes: [Text]?
  bio:   Prose?
  aussi: [Personne]?
}
`

func TestLevels(t *testing.T) {
	head := "---\ntype: Fiche\n---\n# Hi\n\n## Auteur {#claire}\n\n"
	runCases(t, []lintCase{
		{
			name:   "the headings a sub-object takes",
			schema: niveaux,
			doc:    head + "### Divers\n",
			want:   []string{"doc.kan:8:5: error: unknown heading \"Divers\"; `Personne` takes ### Notes, ### Bio, ### Aussi"},
		},
		{
			name:   "text and a list as a key in a sub-object",
			schema: niveaux,
			doc:    head + "notes: a, b\n\nDu texte.\n",
			want: []string{
				"doc.kan:8:1: error: `notes` is a list: write it under its heading, ### Notes",
				"doc.kan:10:1: error: this text fills no field; write it under ### Bio",
			},
		},
		{
			name:   "an entry without an id, in a sub-object",
			schema: niveaux,
			doc:    head + "### Aussi\n\n#### Paul\n\n##### Zz\n",
			want: []string{
				"doc.kan:10:1: error: every object has an id: this `Personne` needs one; give it an id, as in #### Paul {#id}",
				"doc.kan:12:7: error: unknown heading \"Zz\"; `Personne` takes ##### Notes, ##### Bio, ##### Aussi",
			},
		},
	})
}
