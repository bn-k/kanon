; Kanon schemas (.kanonarch).

(comment) @comment

(number) @number

(string) @string

(escape_sequence) @string.escape

(placeholder) @embedded

(identifier) @variable

; Declarations

(schema_declaration
  name: (identifier) @type)

(enum_declaration
  name: (identifier) @type)

(enum_value
  name: (identifier) @constant)

(unit_declaration
  name: (identifier) @type)

(function_declaration
  name: (identifier) @function)

(rule_declaration
  name: (identifier) @function)

(field_declaration
  name: (identifier) @property)

(inverse
  field: (identifier) @property)

(annotation) @attribute

(reads_clause
  language: (identifier) @label)

(parameter
  name: (identifier) @variable.parameter)

(lambda
  parameters: (identifier) @variable.parameter)

(lambda_parameters
  (identifier) @variable.parameter)

; Types

(type
  name: (identifier) @type)

((type
  name: (identifier) @type.builtin)
  (#any-of? @type.builtin
    "Text" "Prose" "Int" "Number" "Bool" "Date" "DateTime" "Duration" "Money" "Percent" "Quantity" "Version"
    "File" "Image" "Code"))

(unit) @type

; Expressions

(member_expression
  property: (identifier) @property)

(with_field
  name: (identifier) @property)

(call_expression
  function: (identifier) @function)

(call_expression
  function: (member_expression
    property: (identifier) @function.method))

((call_expression
  function: (identifier) @function.builtin)
  (#any-of? @function.builtin "sum" "count" "round" "abs" "min" "max"))

((identifier) @boolean
  (#any-of? @boolean "yes" "no"))

((identifier) @constant.builtin
  (#any-of? @constant.builtin "today" "none"))

; Keywords and punctuation

[
  "unit"
  "enum"
  "schema"
  "fn"
  "rule"
  "reads"
  "with"
] @keyword

[
  "if"
  "then"
  "else"
] @keyword

[
  "and"
  "or"
  "not"
] @keyword.operator

[
  "+"
  "-"
  "*"
  "/"
  "=="
  "!="
  "<"
  "<="
  ">"
  ">="
  "??"
  "?."
  "?"
  "->"
  "<-"
  "=>"
  "="
] @operator

[
  "("
  ")"
  "["
  "]"
  "{"
  "}"
] @punctuation.bracket

[
  ","
  ":"
  "."
] @punctuation.delimiter

"@" @punctuation.special
