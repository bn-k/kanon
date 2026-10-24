package diag

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestString(t *testing.T) {
	tests := []struct {
		d    Diagnostic
		want string
	}{
		{Diagnostic{Pos{"crepes.kan", 4, 12}, Error, "unknown unit"}, "crepes.kan:4:12: error: unknown unit"},
		{Diagnostic{Pos{"hatchback/notes.txt", 0, 0}, Warning, "not a .kan file"}, "hatchback/notes.txt: warning: not a .kan file"},
	}
	for _, tt := range tests {
		if got := tt.d.String(); got != tt.want {
			t.Errorf("String() = %q, want %q", got, tt.want)
		}
	}
}

func TestJSON(t *testing.T) {
	tests := []struct {
		d    Diagnostic
		want string
	}{
		{Diagnostic{Pos{"crepes.kan", 4, 12}, Error, "unknown unit"},
			`{"file":"crepes.kan","line":4,"col":12,"severity":"error","message":"unknown unit"}`},
		{Diagnostic{Pos{"hatchback/notes.txt", 0, 0}, Warning, "not a .kan file"},
			`{"file":"hatchback/notes.txt","severity":"warning","message":"not a .kan file"}`},
	}
	for _, tt := range tests {
		got, err := json.Marshal(tt.d)
		if err != nil || string(got) != tt.want {
			t.Errorf("json.Marshal(%v) = %s, %v, want %s", tt.d, got, err, tt.want)
		}
	}
}

func TestCount(t *testing.T) {
	var l List
	l.Errorf(Pos{"a.kan", 1, 1}, "`%s` needs a value", "name")
	l.Warnf(Pos{"a.kan", 2, 1}, "ignored")
	l.Errorf(Pos{"a.kan", 3, 1}, "again")
	if got := l.Count(Error); got != 2 {
		t.Errorf("Count(Error) = %d, want 2", got)
	}
	if got := l.Count(Warning); got != 1 {
		t.Errorf("Count(Warning) = %d, want 1", got)
	}
	if l[0].Msg != "`name` needs a value" {
		t.Errorf("Msg = %q", l[0].Msg)
	}
}

func TestSort(t *testing.T) {
	l := List{
		{Pos{"b.kan", 1, 1}, Error, "b"},
		{Pos{"a.kan", 2, 5}, Error, "a2-first"},
		{Pos{"a.kan", 2, 5}, Error, "a2-second"},
		{Pos{"a.kan", 2, 1}, Error, "a2-col1"},
		{Pos{"a.kan", 0, 0}, Error, "a-file"},
	}
	l.Sort()
	var got []string
	for _, d := range l {
		got = append(got, d.Msg)
	}
	want := []string{"a-file", "a2-col1", "a2-first", "a2-second", "b"}
	if !slices.Equal(got, want) {
		t.Errorf("Sort order = %v, want %v", got, want)
	}
}
