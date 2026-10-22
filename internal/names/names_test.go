package names

import "testing"

func TestIsSlug(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"crepes", true},
		{"f-2026-031", true},
		{"2025-06-10-vidange", true},
		{"0f8fad5b-d9cb-469f-a165-70867728950e", true},
		{"", false},
		{"-crepes", false},
		{"crepes-", false},
		{"double--hyphen", false},
		{"Crepes", false},
		{"crêpes", false},
		{"prix_unitaire", false},
		{"two words", false},
	}
	for _, tt := range tests {
		if got := IsSlug(tt.s); got != tt.want {
			t.Errorf("IsSlug(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestIsName(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"prix-unitaire", true},
		{"m2", true},
		{"2-roues", false},
		{"", false},
		{"Prix", false},
	}
	for _, tt := range tests {
		if got := IsName(tt.s); got != tt.want {
			t.Errorf("IsName(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestIsTypeName(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"Recette", true},
		{"EtatDesLieux", true},
		{"V2", true},
		{"", false},
		{"recette", false},
		{"Etat-des-lieux", false},
		{"Pièce", false},
	}
	for _, tt := range tests {
		if got := IsTypeName(tt.s); got != tt.want {
			t.Errorf("IsTypeName(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestLabel(t *testing.T) {
	tests := []struct{ name, want string }{
		{"zone-tendue", "Zone tendue"},
		{"nom", "Nom"},
		{"m2", "M2"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := Label(tt.name); got != tt.want {
			t.Errorf("Label(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestArticle(t *testing.T) {
	tests := []struct{ name, want string }{
		{"Image", "an `Image`"},
		{"Partie", "a `Partie`"},
		{"", "a ``"},
	}
	for _, tt := range tests {
		if got := Article(tt.name); got != tt.want {
			t.Errorf("Article(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestFold(t *testing.T) {
	tests := []struct{ s, want string }{
		{"Échéance", "echeance"},
		{"Œufs", "oeufs"},
		{"Straße", "strasse"},
		{"plain", "plain"},
	}
	for _, tt := range tests {
		if got := Fold(tt.s); got != tt.want {
			t.Errorf("Fold(%q) = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct{ s, want string }{
		{"Durée du bail", "duree-du-bail"},
		{"prix_unitaire", "prix-unitaire"},
		{"  État  des lieux ", "etat-des-lieux"},
		{"F-2026/031", "f-2026-031"},
		{"!!", ""},
	}
	for _, tt := range tests {
		if got := Slugify(tt.s); got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestClosest(t *testing.T) {
	candidates := []string{"vidange", "controle-technique", "pneus", "km", "Ingrédients"}
	tests := []struct{ s, want string }{
		{"vidage", "vidange"},
		{"kms", "km"},
		{"ingredients", "Ingrédients"},
		{"controle", "controle-technique"},
		{"pneu", "pneus"},
		{"freins", ""},
		{"co", ""},
		{"x", ""},
	}
	for _, tt := range tests {
		if got := Closest(tt.s, candidates); got != tt.want {
			t.Errorf("Closest(%q) = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestClosestPrefersNearest(t *testing.T) {
	if got := Closest("bon", []string{"bonne", "bons", "bon"}); got != "bon" {
		t.Errorf("Closest(bon) = %q, want the exact match", got)
	}
}
