package model

import (
	"strings"
	"testing"

	"github.com/bn-k/kanon/internal/schema"
)

func TestGet(t *testing.T) {
	nom, lignes := &schema.Field{Name: "nom"}, &schema.Field{Name: "lignes"}
	o := &Object{Fields: []*Field{{Field: nom}, {Field: lignes}}}
	if got := o.Get("lignes"); got == nil || got.Field != lignes {
		t.Errorf("Get(lignes) = %v", got)
	}
	if got := o.Get("date"); got != nil {
		t.Errorf("Get(date) = %v, want nil", got)
	}
}

func TestObjects(t *testing.T) {
	f := &schema.Field{Name: "f"}
	obj := func(id string, children ...*Object) *Object {
		o := &Object{ID: id}
		if len(children) > 0 {
			o.Fields = []*Field{{Field: f, Objects: children}, {Field: f, Links: children}}
		}
		return o
	}
	d := &Doc{
		Root:   obj("root", obj("a", obj("a1"), obj("a2")), obj("b")),
		Linked: []*Object{obj("x", obj("x1")), obj("y")},
	}
	var ids []string
	for o := range d.Objects() {
		ids = append(ids, o.ID)
		if o.ID == "b" {
			break
		}
	}
	if got := strings.Join(ids, " "); got != "root a a1 a2 b" {
		t.Errorf("Objects() until b = %s", got)
	}
	ids = nil
	for o := range d.Objects() {
		ids = append(ids, o.ID)
	}
	if got := strings.Join(ids, " "); got != "root a a1 a2 b x x1 y" {
		t.Errorf("Objects() = %s", got)
	}
}
