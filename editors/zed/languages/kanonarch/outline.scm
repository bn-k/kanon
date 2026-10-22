(unit_declaration
  "unit" @context
  name: (identifier) @name) @item

(enum_declaration
  "enum" @context
  name: (identifier) @name) @item

(schema_declaration
  "schema" @context
  name: (identifier) @name) @item

(field_declaration
  name: (identifier) @name) @item

(function_declaration
  "fn" @context
  name: (identifier) @name) @item

(rule_declaration
  "rule" @context
  name: (identifier) @name) @item
