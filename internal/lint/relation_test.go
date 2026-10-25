package lint

import "testing"

const listing = `
schema Listing {
  title:    Text @heading
  agent:    -> Agent
  contacts: [-> Agent]?
  address:  Address?
  rooms:    [Room]?
  price:    Money
  total:    Money = price
}

schema Address {
  street: Text
}

schema Room reads "{name}" {
  name: Text
  code: Text = name
}

schema Agent {
  name:     Text @heading
  listings: [Listing] <- agent
}

schema Agency {
  name:   Text @heading
  agents: [Agent]?
}
`

// listingDoc is the start of a Listing document, up to its agent; its next line is line 8.
const listingDoc = "---\ntype: Listing\n---\n# House\n\nprice: 250 000.00 EUR\nagent: [[agents/claire]]\n"

var agents = files{
	"agents/claire.kan": "---\ntype: Agent\n---\n# Claire\n",
	"agents/paul.kan":   "---\ntype: Agent\n---\n# Paul\n",
}

func TestRelations(t *testing.T) {
	runCases(t, []lintCase{
		{
			name:   "links on key lines",
			schema: listing,
			doc:    listingDoc + "contacts: [[agents/claire]], [[agents/paul]]\n",
			more:   agents,
		},
		{
			name:   "a list of links under its heading",
			schema: listing,
			doc:    listingDoc + "\n## Contacts\n\n- [[agents/claire]]\n- [[agents/paul]]\n",
			more:   agents,
		},
		{
			name:   "a key line of links with mistakes",
			schema: listing,
			doc:    listingDoc + "contacts: [[agents/claire]], Paul, [[agents/clare]]\n",
			more:   agents,
			want: []string{
				"doc.kan:8:30: error: this field holds an `Agent`: link to it, as in [[paul]]",
				"doc.kan:8:36: error: no document `agents/clare`; did you mean `agents/claire`?",
			},
		},
		{
			name:   "a key line of links with an empty item and a trailing comma",
			schema: listing,
			doc:    listingDoc + "contacts: [[agents/claire]],, [[agents/paul]],\n",
			more:   agents,
			want: []string{
				"doc.kan:8:29: error: `contacts` has an empty item: write a link before this comma, or remove it",
				"doc.kan:8:46: error: `contacts` ends with a comma: write a link after it, or remove it",
			},
		},
		{
			name:   "a link written as an object",
			schema: listing,
			doc:    "---\ntype: Listing\n---\n# House\n\nprice: 250 000.00 EUR\n\n## Agent {#claire}\n\nname: Claire\n",
			want: []string{
				`doc.kan:8:4: error: "Agent" names the field ` + "`agent`, a link to an `Agent` written as a key line: agent: [[…]]",
			},
		},
		{
			name:   "objects written under a list of links",
			schema: listing,
			doc:    listingDoc + "\n## Contacts\n\n### Claire {#claire}\n\nAn agent.\n",
			more:   agents,
			want: []string{
				"doc.kan:11:1: error: `contacts` is a list of links to `Agent`: write one link per item, as in - [[…]]",
			},
		},
		{
			name:   "an id on a link",
			schema: listing,
			doc:    listingDoc + "\n## Contacts\n\n- [[agents/paul]] {#paul}\n",
			more:   agents,
			want:   []string{"doc.kan:11:21: error: a link has no id; remove {#paul}"},
		},
		{
			name:   "an owned object written as a link",
			schema: listing,
			doc:    listingDoc + "address: [[addresses/main]]\n\n## Address {#main}\n\nstreet: 12 Lilac Lane\n",
			more:   agents,
			want: []string{
				"doc.kan:8:1: error: `address` holds its own `Address`, not a link: write it under ## Address {#id}",
			},
		},
		{
			name:   "owned objects written as links",
			schema: listing,
			doc:    listingDoc + "\n## Rooms\n\n- [[rooms/lounge]]\n- [[rooms/kitchen]]\n",
			more:   agents,
			want: []string{
				"doc.kan:11:1: error: `rooms` holds its own `Room` objects, not links: write each one here, as a table, a list or one section per entry, or as a file of doc/rooms/ that this list orders",
			},
		},
		{
			name:   "links to an object of another type, and to a file",
			schema: listing,
			doc:    "---\ntype: Listing\n---\n# House\n\nprice: 250 000.00 EUR\nagent: [[address]]\ncontacts: [[plan.pdf]]\n",
			more:   files{"address.kan": "---\ntype: Address\n---\nstreet: 12 Lilac Lane\n"},
			want: []string{
				"doc.kan:7:8: error: this field holds an `Agent`; [[address]] is an `Address`",
				"doc.kan:8:11: error: this field holds an `Agent`; [[plan.pdf]] is a file",
			},
		},
		{
			name:   "inverses written, and computed fields, which may be",
			schema: listing,
			doc: listingDoc + "contacts: [[agents/paul]]\ntotal: 12.00 EUR\n\n## Rooms\n\n" +
				"| id | name | code |\n|---|---|---|\n| lounge | Lounge | l |\n",
			more: files{
				"agents/claire.kan":       "---\ntype: Agent\n---\n# Claire\n\n## Listings\n\n- [[../doc]]\n",
				"agents/claire/listings/": "",
				"agents/paul.kan":         "---\ntype: Agent\n---\n# Paul\n\nlistings: [[../doc]]\n",
			},
			want: []string{
				"agents/claire.kan:6:4: error: `listings` is the inverse of `Listing.agent`, computed from the documents; remove it",
				"agents/claire/listings: error: `listings` is the inverse of `Listing.agent`, computed from the documents; remove it",
				"agents/paul.kan:6:1: error: `listings` is the inverse of `Listing.agent`, computed from the documents; remove it",
			},
		},
		{
			name:   "an inverse as a column",
			schema: listing,
			doc:    "---\ntype: Agency\n---\n# Bristol\n\n## Agents\n\n| id | name | listings |\n|---|---|---|\n| claire | Claire | [[x]] |\n",
			want: []string{
				"doc.kan:8:15: error: `listings` is the inverse of `Listing.agent`, computed from the documents; remove it",
			},
		},
	})
}
