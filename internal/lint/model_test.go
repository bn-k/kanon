package lint

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/func-it/kanon/internal/diag"
	"github.com/func-it/kanon/internal/model"
	"github.com/func-it/kanon/internal/value"
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
	d, dir := load(t, "invoice", "f-2026-031.kan")
	if d.Schema == nil || d.Schema.Schema("Facture") != d.Root.Schema {
		t.Errorf("Schema = %v", d.Schema)
	}
	checkModel(t, dumpModel(t, d, dir), `Facture f-2026-031 f-2026-031.kan
  numero: "F-2026-031"
  date: "2026-10-22"
  echeance: "2026-11-21"
  emetteur:
    Partie atelier-lune f-2026-031.kan:14
      nom: "Atelier Lune SARL"
      adresse: "8 quai des Brumes, 69002 Lyon"
  client: -> Partie dupont-batiment
  lignes:
    Ligne dev-api f-2026-031.kan:23
      designation: "Développement de l'API de réservation (jours)"
      quantite: "12"
      prix-unitaire: "650.00 EUR"
      montant: "7 800.00 EUR"
    Ligne hebergement f-2026-031.kan:24
      designation: "Hébergement, octobre à décembre (mois)"
      quantite: "3"
      prix-unitaire: "40.00 EUR"
      montant: "120.00 EUR"
  taux-tva: "20 %"
  conditions: "Paiement par virement au plus tard le {echeance}, soit {total-ttc}. En cas de retard, des pénalités au taux de\ntrois fois le taux d'intérêt légal sont exigibles, ainsi qu'une indemnité forfaitaire de 40 EUR pour frais de\nrecouvrement (Code de commerce, art. D441-5)."
  total-ht: "7 920.00 EUR"
  tva: "1 584.00 EUR"
  total-ttc: "9 504.00 EUR"
linked
Partie dupont-batiment clients/dupont-batiment.kan
  nom: "Dupont Bâtiment SAS"
  adresse: "14 avenue des Tilleuls, 38000 Grenoble"
`)
	l := d.Root.Get("lignes").Objects[0]
	if v := l.Get("prix-unitaire").Values[0]; v.Kind != value.Money || v.Num.RatString() != "650" || v.Unit != "EUR" {
		t.Errorf("prix-unitaire = %+v", v)
	}
	if d.Root.Get("client").Links[0] != d.Linked[0] {
		t.Errorf("client is not the linked document")
	}
}

func TestModelAnnonce(t *testing.T) {
	d, dir := load(t, "annonce", "maison-caluire.kan")
	checkModel(t, dumpModel(t, d, dir), `Annonce maison-caluire maison-caluire.kan
  titre: "Maison familiale avec jardin à Caluire-et-Cuire"
  reference: "CAL-2026-118"
  bien: "maison"
  prix: "645 000.00 EUR"
  surface: "112 m2"
  terrain: "480 m2"
  chambres: "3"
  dpe: "c"
  visite: "2026-10-25 10:30"
  agent: -> Agent sophie-bernard
  co-mandataires: -> Agent marc-petit -> Agent lea-morel
  vendeur: -> Vendeur famille-roux
  localisation:
    Localisation impasse-des-vergers maison-caluire.kan:30
      commune: "Caluire-et-Cuire"
      code-postal: "69300"
      quartier: "Montessuy"
  plan: maison-caluire/plan-rdc.pdf
  photos: maison-caluire/photos/01-facade.jpg maison-caluire/photos/02-sejour.jpg maison-caluire/photos/03-jardin.png
  description: "![[maison-caluire/photos/01-facade.jpg]]\n\nAu calme d'une impasse, à dix minutes du centre de Lyon, cette maison de {surface} sur un terrain de {terrain}\noffre trois chambres, un séjour lumineux ouvert sur le jardin, une buanderie et un garage.\n\n![Le séjour, plein sud](maison-caluire/photos/02-sejour.jpg)\n\nLe séjour, la cuisine et la chambre parentale sont de plain-pied (voir le\n[plan du rez-de-chaussée](maison-caluire/plan-rdc.pdf)). Prix de vente de {prix}, honoraires à la charge du\nvendeur. Visite sur rendez-vous avec [[agents/sophie-bernard]], prochaine visite le {visite}."
  pieces:
    Piece sejour maison-caluire.kan:45
      nom: "Séjour"
      niveau: "rez-de-chaussee"
      surface: "38 m2"
    Piece cuisine maison-caluire.kan:46
      nom: "Cuisine"
      niveau: "rez-de-chaussee"
      surface: "14 m2"
    Piece chambre-1 maison-caluire.kan:47
      nom: "Chambre parentale"
      niveau: "rez-de-chaussee"
      surface: "12 m2"
    Piece salle-de-bain maison-caluire.kan:48
      nom: "Salle de bain"
      niveau: "rez-de-chaussee"
      surface: "6 m2"
    Piece chambre-2 maison-caluire.kan:49
      nom: "Chambre"
      niveau: "etage"
      surface: "11 m2"
    Piece chambre-3 maison-caluire.kan:50
      nom: "Chambre"
      niveau: "etage"
      surface: "10 m2"
    Piece buanderie maison-caluire.kan:51
      nom: "Buanderie"
      niveau: "sous-sol"
      surface: "8 m2"
  visite-virtuelle: html "<iframe src=\"https://visite.example/cal-2026-118\" width=\"640\" height=\"360\" allowfullscreen></iframe>"
linked
Agent sophie-bernard agents/sophie-bernard.kan
  nom: "Sophie Bernard"
  agence: "Rhône Immobilier, Caluire"
  telephone: "+33 4 65 71 45 61"
  email: "sophie.bernard@rhone-immobilier.example"
  portrait: agents/sophie-bernard/portrait.jpg
  mandats: -> Annonce maison-caluire
linked
Vendeur famille-roux vendeurs/famille-roux.kan
  nom: "Famille Roux"
  adresse: "12 chemin des Vergers, 69300 Caluire-et-Cuire"
  annonce: -> Annonce maison-caluire
linked
Agent marc-petit agents/marc-petit.kan
  nom: "Marc Petit"
  agence: "Croix-Rousse Habitat"
  telephone: "+33 4 65 71 98 76"
  email: "m.petit@croix-rousse-habitat.example"
  co-mandats: -> Annonce maison-caluire
linked
Agent lea-morel agents/lea-morel.kan
  nom: "Léa Morel"
  agence: "Saône Conseil Immobilier"
  telephone: "+33 4 65 71 66 07"
  email: "lea.morel@saone-conseil.example"
  co-mandats: -> Annonce maison-caluire
`)
	v := d.Root.Get("visite").Values[0]
	if v.Kind != value.DateTime || v.Zone != "" || v.Seconds || v.DateTimeText() != "2026-10-25 10:30" {
		t.Errorf("visite = %+v", v)
	}
	if d.Root.Get("terrain") == nil || d.Linked[3].Get("portrait") != nil {
		t.Errorf("terrain is written, lea-morel's portrait is not")
	}
}

func TestModelFolder(t *testing.T) {
	d, dir := load(t, "car", "clio.kan", "../../cases/car/clio.kan: error: `vidange-a-jour` fails: "+
		"\"Renault Clio a roule moins de 15 000 km depuis sa derniere vidange\"; 15 250 km is not less than 15 000 km")
	es := d.Root.Get("entretiens")
	if es == nil || len(es.Objects) != 5 {
		t.Fatalf("entretiens = %v", es)
	}
	var got []string
	for _, e := range es.Objects {
		got = append(got, filepath.ToSlash(strings.TrimPrefix(e.File, dir+string(filepath.Separator)))+" "+e.ID)
	}
	want := "clio/entretiens/2023-04-05-controle-technique.kan 2023-04-05-controle-technique\n" +
		"clio/entretiens/2023-09-18-vidange.kan 2023-09-18-vidange\n" +
		"clio/entretiens/2024-07-02-pneus.kan 2024-07-02-pneus\n" +
		"clio/entretiens/2025-04-03-controle-technique.kan 2025-04-03-controle-technique\n" +
		"clio/entretiens/2025-06-10-vidange.kan 2025-06-10-vidange"
	if strings.Join(got, "\n") != want {
		t.Errorf("entretiens =\n%s\nwant\n%s", strings.Join(got, "\n"), want)
	}
	checkModel(t, dumpModel(t, &model.Doc{Root: es.Objects[2]}, dir), `Entretien 2024-07-02-pneus clio/entretiens/2024-07-02-pneus.kan
  date: "2024-07-02"
  kilometrage: "65 400 km"
  operation: "pneus"
  garage: "Garage du Centre"
  cout: "412.00 EUR"
`)
	if es.Objects[2].Parent != d.Root || es.Objects[2].Holder != es.Field || es.Pos.File != filepath.Join(dir, "clio", "entretiens") {
		t.Errorf("entretiens: Parent = %v, Holder = %v, Pos = %v", es.Objects[2].Parent, es.Objects[2].Holder, es.Pos)
	}
}

func TestModelSheet(t *testing.T) {
	write(t, files{
		"schema.kanonarch": mesures,
		"doc.kan":          lilas("mesures: [[mesures.csv]]\n"),
		"mesures.csv":      "id,date,piece,valeur,note,par\nm1,2026-10-01,salon,19.5,,[[claire]]\nm2,2026-10-02,cuisine,21,fenêtre ouverte,\n",
		"claire.kan":       "---\ntype: Personne\n---\n# Claire\n",
	})
	d, r := Load("schema.kanonarch", "doc.kan")
	check(t, r, nil)
	checkModel(t, dumpModel(t, d, "."), `Maison doc doc.kan
  nom: "Lilas"
  mesures:
    Mesure m1 doc.kan, mesures.csv:2:1
      date: "2026-10-01"
      piece: "salon"
      valeur: "19.5"
      par: -> Personne claire
    Mesure m2 doc.kan, mesures.csv:3:1
      date: "2026-10-02"
      piece: "cuisine"
      valeur: "21"
      note: "fenêtre ouverte"
linked
Personne claire claire.kan
  nom: "Claire"
`)
	ms := d.Root.Get("mesures")
	if ms.Pos != (diag.Pos{File: "doc.kan", Line: 6, Col: 1}) {
		t.Errorf("mesures: Pos = %v", ms.Pos)
	}
	if m := ms.Objects[1]; m.Parent != d.Root || m.Holder != ms.Field {
		t.Errorf("m2: Parent = %v, Holder = %v", m.Parent, m.Holder)
	}

	write(t, files{
		"schema.kanonarch": mesures,
		"doc.kan":          lilas("## Mesures\n\n[[mesures.xlsx#releves]]\n"),
		"mesures.xlsx":     workbook(t, "releves", "date,id,piece,valeur\n2026-10-01,m1,salon,19.5\n"),
	})
	d, r = Load("schema.kanonarch", "doc.kan")
	check(t, r, nil)
	checkModel(t, dumpModel(t, d, "."), `Maison doc doc.kan
  nom: "Lilas"
  mesures:
    Mesure m1 doc.kan, mesures.xlsx#releves:2:2
      date: "2026-10-01"
      piece: "salon"
      valeur: "19.5"
`)
}

const carnetModel = `
enum Etat { neuf, use }

schema Carnet {
  titre:       Text @heading
  etat:        Etat
  resume:      Prose
  notes:       Prose?
  script:      Code
  annexes:     [File]
  ingredients: [Ingredient]
  etapes:      [Prose]
  chapitres:   [Chapitre] @sections
  total:       Int = 1
}

schema Ingredient reads "{quantite} de {nom}" {
  quantite: Quantity
  nom:      Text
}

schema Chapitre {
  titre: Text @heading
  corps: Prose? @body
}
`

func TestModel(t *testing.T) {
	write(t, files{
		"schema.kanonarch": carnetModel,
		"doc.kan": `---
type: Carnet
---
# Mon carnet

etat: use
total: 1
notes: une note  libre
script: [[doc/run.sh]]
annexes: [[doc/a.pdf]], [[doc/b.txt]]

## Resume

statut: brouillon
- un
- deux

## Ingredients

- 2 kg de farine {#farine}

## Etapes

1. Mélanger.
2. Cuire {titre}.

## Premier chapitre {#un}

Texte du chapitre.

## Second {#deux}
`,
		"doc/run.sh": "echo hi\n",
		"doc/a.pdf":  "%PDF-1.4\n",
		"doc/b.txt":  "b\n",
	})
	d, r := Load("schema.kanonarch", "doc.kan")
	check(t, r, nil)
	checkModel(t, dumpModel(t, d, "."), `Carnet doc doc.kan
  titre: "Mon carnet"
  etat: "use"
  resume: "statut: brouillon\n- un\n- deux"
  notes: "une note  libre"
  script: sh "echo hi\n"
  annexes: doc/a.pdf doc/b.txt
  ingredients:
    Ingredient farine doc.kan:20
      quantite: "2 kg"
      nom: "farine"
  etapes: "Mélanger." "Cuire {titre}."
  chapitres:
    Chapitre un doc.kan:27
      titre: "Premier chapitre"
      corps: "Texte du chapitre."
    Chapitre deux doc.kan:31
      titre: "Second"
  total: "1"
`)
	if v := d.Root.Get("etapes").Values[0]; v.Kind != value.Prose {
		t.Errorf("etapes[0] = %+v, want a Prose value", v)
	}
	if v := d.Root.Get("etat").Values[0]; v.Kind != value.Text {
		t.Errorf("etat = %+v, want a Text value", v)
	}
}

const projets = `
schema Projet {
  nom:     Text @heading
  chef:    -> Personne
  membres: [-> Personne]?
  parrain: -> Personne?
  notes:   Prose? @body
}

schema Personne {
  nom:     Text @heading
  projets: [Projet] <- membres
  dirige:  [Projet] <- chef
  filleul: Projet? <- parrain
}
`

const alpha = `---
type: Projet
---
# Alpha

chef: [[ana]]
membres: [[ana]], [[ana]], [[bob]]
parrain: [[bob]]

Voir [[beta]].
`

func TestInverses(t *testing.T) {
	personne := func(nom string) string { return "---\ntype: Personne\n---\n# " + nom + "\n" }
	write(t, files{
		"schema.kanonarch": projets,
		"alpha.kan":        alpha,
		"beta.kan":         "---\ntype: Projet\n---\n# Beta\n\nchef: [[ana]]\n",
		"ana.kan":          personne("Ana"),
		"bob.kan":          personne("Bob"),
	})
	d, r := Load("schema.kanonarch", "alpha.kan")
	check(t, r, nil)
	checkModel(t, dumpModel(t, d, "."), `Projet alpha alpha.kan
  nom: "Alpha"
  chef: -> Personne ana
  membres: -> Personne ana -> Personne ana -> Personne bob
  parrain: -> Personne bob
  notes: "Voir [[beta]]."
linked
Personne ana ana.kan
  nom: "Ana"
  projets: -> Projet alpha
  dirige: -> Projet alpha -> Projet beta
linked
Personne bob bob.kan
  nom: "Bob"
  projets: -> Projet alpha
  filleul: -> Projet alpha
linked
Projet beta beta.kan
  nom: "Beta"
  chef: -> Personne ana
`)

	write(t, files{
		"schema.kanonarch": projets,
		"alpha.kan":        alpha,
		"beta.kan":         "---\ntype: Projet\n---\n# Beta\n\nchef: [[ana]]\nmembres: [[ana#chef]]\nparrain: [[bob]]\n",
		"ana.kan":          personne("Ana"),
		"bob.kan":          personne("Bob"),
	})
	d, r = Load("schema.kanonarch", "alpha.kan")
	check(t, r, []string{
		"beta.kan:7:10: error: unknown id `chef` in ana.kan",
		"beta.kan:8:10: error: [[bob]] is already the `parrain` of `alpha`, at alpha.kan:8:10; a `Personne` has one `filleul`",
	})
	if d != nil {
		t.Errorf("Load = %v, want no model", d)
	}
}

func TestLoadWithErrors(t *testing.T) {
	write(t, files{"schema.kanonarch": note, "doc.kan": "---\ntype: Note\n---\n# Hi\n\ndate: demain\n"})
	d, r := Load("schema.kanonarch", "doc.kan")
	if d != nil || r.Diags.Count(diag.Error) != 1 {
		t.Errorf("Load = %v, %v; want no model and one error", d, r.Diags)
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
