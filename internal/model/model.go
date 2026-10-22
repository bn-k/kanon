// Package model is the bound model of a document: its objects and the values of their fields, as lint has read
// them. It holds only documents that lint found without errors; kan sql exports it.
package model

import (
	"iter"

	"github.com/func-it/kanon/internal/diag"
	"github.com/func-it/kanon/internal/schema"
	"github.com/func-it/kanon/internal/value"
)

// Doc is a document, every document it holds in folders, and every document its links lead to.
type Doc struct {
	Schema *schema.File
	Root   *Object
	Linked []*Object // the documents its links lead to, outside its folders, in the order they were read
}

// Objects returns every object of the document, depth first in document order: the root and the objects it holds,
// then each linked document and the objects it holds. kan sql inserts them in that order.
func (d *Doc) Objects() iter.Seq[*Object] {
	return func(yield func(*Object) bool) {
		for _, o := range append([]*Object{d.Root}, d.Linked...) {
			if !o.walk(yield) {
				return
			}
		}
	}
}

// walk yields o, then the objects it holds; it reports whether to go on.
func (o *Object) walk(yield func(*Object) bool) bool {
	if !yield(o) {
		return false
	}
	for _, f := range o.Fields {
		for _, c := range f.Objects {
			if !c.walk(yield) {
				return false
			}
		}
	}
	return true
}

// Object is an object bound to its schema: a whole file, a section, a table row, a row of a spreadsheet or a list
// item.
type Object struct {
	Schema *schema.Schema
	ID     string
	Pos    diag.Pos      // its heading, row or item, the cell of its id for a row of a spreadsheet; Line 0 for a whole file
	File   string        // the .kan file that holds it, or that links to its spreadsheet
	Parent *Object       // the object that holds it; nil for the root
	Holder *schema.Field // the field of Parent that holds it; nil for the root
	Fields []*Field      // in schema order; an optional field not written is absent, and so is an empty inverse
}

// Get returns the field of o named name, or nil when it is not written.
func (o *Object) Get(name string) *Field {
	for _, f := range o.Fields {
		if f.Field.Name == name {
			return f
		}
	}
	return nil
}

// Field is a field written in an object, an inverse computed from the objects that link to it, or a computed field
// (Field.Computed()), whose value eval sets, in place of the one written if any; unwritten, it has no Pos. One of
// its values is set, by the field's type:
//
//   - Values for a native type other than File, Image, Prose and Code, or an enum: one value, or one per item of
//     an array of values, in order. An enum value is a value.Text whose Raw is the name of the enum value. An
//     array of Prose, [Prose], has one value.Prose per item, its text in Raw.
//   - Objects for an owned object, T or [T], in order.
//   - Links for a link, -> T or [-> T]: the targets, in order; and for an inverse, [S] <- f or S? <- f, the
//     objects whose field f links here, in document order.
//   - Files for File, Image, [File] and [Image]: the paths on disk, in order, ready to be read.
//   - Text for Prose, its text as written (lines kept, blocks separated by "\n\n"), and for Code, the text of
//     its fence or of its linked file, with Lang its language: the info string of the fence or the extension of
//     the file, "" when there is none.
type Field struct {
	Field   *schema.Field
	Pos     diag.Pos // its key line or heading, or the folder that holds it; none for an inverse
	Values  []value.Value
	Objects []*Object
	Links   []*Object
	Files   []string
	Text    string
	Lang    string
}
