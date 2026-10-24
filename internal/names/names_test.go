package names

import "testing"

func TestIsSlug(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"crepes", true},
		{"inv-2026-031", true},
		{"2025-06-10-oil-change", true},
		{"0f8fad5b-d9cb-469f-a165-70867728950e", true},
		{"", false},
		{"-crepes", false},
		{"crepes-", false},
		{"double--hyphen", false},
		{"Crepes", false},
		{"crêpes", false},
		{"unit_price", false},
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
		{"unit-price", true},
		{"m2", true},
		{"2-wheels", false},
		{"", false},
		{"Price", false},
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
		{"Recipe", true},
		{"CheckInReport", true},
		{"V2", true},
		{"", false},
		{"recipe", false},
		{"Check-in-report", false},
		{"Café", false},
	}
	for _, tt := range tests {
		if got := IsTypeName(tt.s); got != tt.want {
			t.Errorf("IsTypeName(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestHeading(t *testing.T) {
	tests := []struct{ name, want string }{
		{"unit-price", "Unit price"},
		{"name", "Name"},
		{"m2", "M2"},
		{"", ""},
	}
	for _, tt := range tests {
		got := Heading(tt.name)
		if got != tt.want {
			t.Errorf("Heading(%q) = %q, want %q", tt.name, got, tt.want)
		}
		if tt.name != "" && Slugify(got) != tt.name {
			t.Errorf("Slugify(%q) = %q, want %q: a heading finds its field", got, Slugify(got), tt.name)
		}
	}
}

func TestTransliterate(t *testing.T) {
	tests := []struct {
		r    rune
		want string
		ok   bool
	}{
		{'é', "e", true},
		{'É', "E", true},
		{'œ', "oe", true},
		{'Œ', "Oe", true},
		{'ß', "ss", true},
		{'°', "o", true},
		{'’', "'", true},
		{'–', "-", true},
		{'«', "", false},
		{'€', "", false},
	}
	for _, tt := range tests {
		got, ok := Transliterate(tt.r)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Transliterate(%q) = %q, %v, want %q, %v", tt.r, got, ok, tt.want, tt.ok)
		}
	}
}

func TestArticle(t *testing.T) {
	tests := []struct{ name, want string }{
		{"Image", "an `Image`"},
		{"Party", "a `Party`"},
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
		{"Émigré", "emigre"},
		{"Œuvre", "oeuvre"},
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
		{"Rosé of the day", "rose-of-the-day"},
		{"unit_price", "unit-price"},
		{"  Émigré  writers ", "emigre-writers"},
		{"Table d'hôte", "table-d-hote"},
		{"Éclair", "eclair"},
		{"Zoë's café menu", "zoe-s-cafe-menu"},
		{"Zoë’s café-menu", "zoe-s-cafe-menu"},
		{"Unit price", "unit-price"},
		{"INV-2026/031", "inv-2026-031"},
		{"!!", ""},
	}
	for _, tt := range tests {
		if got := Slugify(tt.s); got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestClosest(t *testing.T) {
	candidates := []string{"service", "inspection-date", "tyres", "km", "Résumé"}
	tests := []struct{ s, want string }{
		{"servise", "service"},
		{"kms", "km"},
		{"resume", "Résumé"},
		{"inspection", "inspection-date"},
		{"tyre", "tyres"},
		{"brakes", ""},
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
	if got := Closest("car", []string{"cards", "cars", "car"}); got != "car" {
		t.Errorf("Closest(car) = %q, want the exact match", got)
	}
}
