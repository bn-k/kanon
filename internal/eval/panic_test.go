package eval

import (
	"testing"

	"github.com/func-it/kanon/internal/model"
	"github.com/func-it/kanon/internal/schema"
	"github.com/func-it/kanon/internal/value"
)

// TestBugsPanic checks that a panic that is not an evaluation going wrong, a bug, is not taken for one.
func TestBugsPanic(t *testing.T) {
	e := &evaluator{file: &schema.File{}, objs: map[*model.Object]*obj{}}
	o := e.wrap(&model.Object{Schema: &schema.Schema{Name: "T"}})
	unknown := &schema.Ident{Name: "zz"}
	tests := []struct {
		name string
		f    func()
	}{
		{"try", func() { e.try(func() { panic("bug") }) }},
		{"field", func() { e.field(o, &schema.Field{Name: "f", Expr: unknown}) }},
		{"rule", func() { e.rule(&schema.Fn{Name: "r", Params: []*schema.Param{{Name: "t"}}, Body: unknown}, o) }},
		{"safeEqual", func() { safeEqual(value.Value{Kind: value.Version}, value.Value{Kind: value.Int}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("%s recovered from a bug", tt.name)
				} else if _, ok := r.(evalErr); ok {
					t.Errorf("%s turned a bug into %+v", tt.name, r)
				}
			}()
			tt.f()
		})
	}
}
