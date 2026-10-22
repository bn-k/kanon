package lint

import "testing"

const annonce = `
schema Annonce {
  titre:    Text @heading
  agent:    -> Agent
  contacts: [-> Agent]?
  adresse:  Adresse?
  pieces:   [Piece]?
  prix:     Money
  total:    Money = prix
}

schema Adresse {
  rue: Text
}

schema Piece reads "{nom}" {
  nom:  Text
  code: Text = nom
}

schema Agent {
  nom:      Text @heading
  annonces: [Annonce] <- agent
}

schema Agence {
  nom:    Text @heading
  agents: [Agent]?
}
`

// annonceDoc is the start of an Annonce document, up to its agent; its next line is line 8.
const annonceDoc = "---\ntype: Annonce\n---\n# Maison\n\nprix: 250 000.00 EUR\nagent: [[agents/claire]]\n"

var agents = files{
	"agents/claire.kan": "---\ntype: Agent\n---\n# Claire\n",
	"agents/paul.kan":   "---\ntype: Agent\n---\n# Paul\n",
}

func TestRelations(t *testing.T) {
	runCases(t, []lintCase{
		{
			name:   "links on key lines",
			schema: annonce,
			doc:    annonceDoc + "contacts: [[agents/claire]], [[agents/paul]]\n",
			more:   agents,
		},
		{
			name:   "a list of links under its heading",
			schema: annonce,
			doc:    annonceDoc + "\n## Contacts\n\n- [[agents/claire]]\n- [[agents/paul]]\n",
			more:   agents,
		},
		{
			name:   "a key line of links with mistakes",
			schema: annonce,
			doc:    annonceDoc + "contacts: [[agents/claire]], Paul, [[agents/clare]]\n",
			more:   agents,
			want: []string{
				"doc.kan:8:30: error: this field holds an `Agent`: link to it, as in [[paul]]",
				"doc.kan:8:36: error: no document `agents/clare`; did you mean `agents/claire`?",
			},
		},
		{
			name:   "a link written as an object",
			schema: annonce,
			doc:    "---\ntype: Annonce\n---\n# Maison\n\nprix: 250 000.00 EUR\n\n## Agent {#claire}\n\nnom: Claire\n",
			want: []string{
				`doc.kan:8:4: error: "Agent" names the field ` + "`agent`, a link to an `Agent` written as a key line: agent: [[…]]",
			},
		},
		{
			name:   "objects written under a list of links",
			schema: annonce,
			doc:    annonceDoc + "\n## Contacts\n\n### Claire {#claire}\n\nUne agente.\n",
			more:   agents,
			want: []string{
				"doc.kan:11:1: error: `contacts` is a list of links to `Agent`: write one link per item, as in - [[…]]",
			},
		},
		{
			name:   "an id on a link",
			schema: annonce,
			doc:    annonceDoc + "\n## Contacts\n\n- [[agents/paul]] {#paul}\n",
			more:   agents,
			want:   []string{"doc.kan:11:21: error: a link has no id; remove {#paul}"},
		},
		{
			name:   "an owned object written as a link",
			schema: annonce,
			doc:    annonceDoc + "adresse: [[adresses/rue]]\n\n## Adresse {#rue}\n\nrue: 12 rue des Lilas\n",
			more:   agents,
			want: []string{
				"doc.kan:8:1: error: `adresse` holds its own `Adresse`, not a link: write it under ## Adresse {#id}",
			},
		},
		{
			name:   "owned objects written as links",
			schema: annonce,
			doc:    annonceDoc + "\n## Pieces\n\n- [[pieces/salon]]\n- [[pieces/cuisine]]\n",
			more:   agents,
			want: []string{
				"doc.kan:11:1: error: `pieces` holds its own `Piece` objects, not links: write each one here, as a table, a list or one section per entry",
			},
		},
		{
			name:   "links to an object of another type, and to a file",
			schema: annonce,
			doc:    "---\ntype: Annonce\n---\n# Maison\n\nprix: 250 000.00 EUR\nagent: [[adresse]]\ncontacts: [[plan.pdf]]\n",
			more:   files{"adresse.kan": "---\ntype: Adresse\n---\nrue: 12 rue des Lilas\n"},
			want: []string{
				"doc.kan:7:8: error: this field holds an `Agent`; [[adresse]] is an `Adresse`",
				"doc.kan:8:11: error: this field holds an `Agent`; [[plan.pdf]] is a file",
			},
		},
		{
			name:   "inverses written, and computed fields, which may be",
			schema: annonce,
			doc: annonceDoc + "contacts: [[agents/paul]]\ntotal: 12.00 EUR\n\n## Pieces\n\n" +
				"| id | nom | code |\n|---|---|---|\n| salon | Salon | s |\n",
			more: files{
				"agents/claire.kan":       "---\ntype: Agent\n---\n# Claire\n\n## Annonces\n\n- [[../doc]]\n",
				"agents/claire/annonces/": "",
				"agents/paul.kan":         "---\ntype: Agent\n---\n# Paul\n\nannonces: [[../doc]]\n",
			},
			want: []string{
				"agents/claire.kan:6:4: error: `annonces` is the inverse of `Annonce.agent`, computed from the documents; remove it",
				"agents/claire/annonces: error: `annonces` is the inverse of `Annonce.agent`, computed from the documents; remove it",
				"agents/paul.kan:6:1: error: `annonces` is the inverse of `Annonce.agent`, computed from the documents; remove it",
			},
		},
		{
			name:   "an inverse as a column",
			schema: annonce,
			doc:    "---\ntype: Agence\n---\n# Lyon\n\n## Agents\n\n| id | nom | annonces |\n|---|---|---|\n| claire | Claire | [[x]] |\n",
			want: []string{
				"doc.kan:8:14: error: `annonces` is the inverse of `Annonce.agent`, computed from the documents; remove it",
			},
		},
	})
}
