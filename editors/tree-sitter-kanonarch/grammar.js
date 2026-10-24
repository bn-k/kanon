/**
 * Tree-sitter grammar of Kanon schemas (.kanonarch), for editors.
 *
 * The reference is the Go parser, internal/schema (lexer.go, parser.go): this grammar follows it and never
 * contradicts it. It reads every file the Go parser reads; it checks no types, names or units.
 */

/// <reference types="tree-sitter-cli/dsl" />
// @ts-check

// Binding powers, from internal/schema/parser.go: all binary operators are left-associative; `not` applies to a
// comparison (not a == b is not (a == b)); postfix operators bind tightest.
const PREC = {
  lambda: -1,
  if: 0,
  coalesce: 10, // ??
  or: 20,
  and: 30,
  not: 35,
  compare: 40,
  add: 50,
  multiply: 60,
  negate: 70,
  postfix: 80,
};

module.exports = grammar({
  name: 'kanonarch',

  extras: $ => [/\s/, $.comment],

  word: $ => $.identifier,

  conflicts: $ => [
    // A number followed by a name: `2 kg` is a quantity, but in `a: Int = 2 b: Int` the name starts the next
    // field. The Go parser looks for the colon after the name; here the branch that finds it wins.
    [$.quantity, $._expression],
    // `(a)` is a parenthesized name, `(a) => …` a function.
    [$._expression, $.lambda_parameters],
  ],

  supertypes: $ => [$._declaration, $._expression],

  rules: {
    source_file: $ => repeat($._declaration),

    _declaration: $ => choice(
      $.unit_declaration,
      $.enum_declaration,
      $.schema_declaration,
      $.function_declaration,
      $.rule_declaration,
    ),

    // unit pinch
    unit_declaration: $ => seq(
      'unit',
      field('name', $.identifier),
    ),

    // enum Condition { mint, good, worn }
    enum_declaration: $ => seq(
      'enum',
      field('name', $.identifier),
      field('body', $.enum_body),
    ),

    enum_body: $ => seq('{', repeat($.enum_value), '}'),

    enum_value: $ => seq(
      field('name', $.identifier),
      optional(','),
    ),

    // schema Ingredient reads "{quantity} [of] {name}" { … }
    schema_declaration: $ => seq(
      'schema',
      field('name', $.identifier),
      repeat($.reads_clause),
      field('body', $.schema_body),
    ),

    schema_body: $ => seq('{', repeat($.field_declaration), '}'),

    // reads en "{m.name} has an up-to-date BIOS"
    reads_clause: $ => seq(
      'reads',
      optional(field('language', $.identifier)),
      field('text', $.string),
    ),

    // net-total: Money @heading = sum(lines.amount),
    field_declaration: $ => seq(
      field('name', $.identifier),
      ':',
      field('type', $.type),
      optional($.inverse),
      repeat($.annotation),
      optional(seq('=', field('value', $._expression))),
      optional(','),
    ),

    // [Listing] <- agent
    inverse: $ => seq('<-', field('field', $.identifier)),

    annotation: $ => seq('@', field('name', $.identifier)),

    // T, T?, -> T, [T], [-> T], [T]?
    type: $ => seq(
      choice(
        seq('[', optional('->'), field('name', $.identifier), ']'),
        seq(optional('->'), field('name', $.identifier)),
      ),
      optional('?'),
    ),

    // fn last(v: Vehicle, op: Operation) -> Service? reads en "…" = …
    function_declaration: $ => seq(
      'fn',
      field('name', $.identifier),
      field('parameters', $.parameter_list),
      '->',
      field('result', $.type),
      repeat($.reads_clause),
      '=',
      field('body', $._expression),
    ),

    // A rule may leave out its result, which is Bool.
    rule_declaration: $ => seq(
      'rule',
      field('name', $.identifier),
      field('parameters', $.parameter_list),
      optional(seq('->', field('result', $.type))),
      repeat($.reads_clause),
      '=',
      field('body', $._expression),
    ),

    parameter_list: $ => seq('(', commaSep($.parameter), optional(','), ')'),

    parameter: $ => seq(
      field('name', $.identifier),
      ':',
      field('type', $.type),
      optional(seq('=', field('default', $._expression))),
    ),

    _expression: $ => choice(
      $.identifier,
      $.number,
      $.quantity,
      $.string,
      $.parenthesized_expression,
      $.unary_expression,
      $.binary_expression,
      $.member_expression,
      $.call_expression,
      $.with_expression,
      $.if_expression,
      $.lambda,
    ),

    parenthesized_expression: $ => seq('(', $._expression, ')'),

    unary_expression: $ => choice(
      prec(PREC.negate, seq(field('operator', '-'), field('operand', $._expression))),
      prec(PREC.not, seq(field('operator', 'not'), field('operand', $._expression))),
    ),

    binary_expression: $ => {
      const table = [
        [PREC.coalesce, '??'],
        [PREC.or, 'or'],
        [PREC.and, 'and'],
        [PREC.compare, choice('==', '!=', '<', '<=', '>', '>=')],
        [PREC.add, choice('+', '-')],
        [PREC.multiply, choice('*', '/')],
      ];
      return choice(...table.map(([p, op]) => prec.left(p, seq(
        field('left', $._expression),
        field('operator', op),
        field('right', $._expression),
      ))));
    },

    // o.f, o?.f
    member_expression: $ => prec(PREC.postfix, seq(
      field('object', $._expression),
      choice('.', '?.'),
      field('property', $.identifier),
    )),

    call_expression: $ => prec(PREC.postfix, seq(
      field('function', $._expression),
      field('arguments', $.argument_list),
    )),

    argument_list: $ => seq('(', commaSep($._expression), optional(','), ')'),

    // r with { portions: n, }
    with_expression: $ => prec(PREC.postfix, seq(
      field('object', $._expression),
      'with',
      '{',
      commaSep($.with_field),
      optional(','),
      '}',
    )),

    with_field: $ => seq(field('name', $.identifier), ':', field('value', $._expression)),

    // The else branch goes as far as it can: if c then a else b + 1 is if c then a else (b + 1).
    if_expression: $ => prec.right(PREC.if, seq(
      'if', field('condition', $._expression),
      'then', field('consequence', $._expression),
      'else', field('alternative', $._expression),
    )),

    // x => …, (a, b) => …, () => …
    lambda: $ => prec.right(PREC.lambda, seq(
      field('parameters', choice($.identifier, $.lambda_parameters)),
      '=>',
      field('body', $._expression),
    )),

    lambda_parameters: $ => seq('(', commaSep($.identifier), ')'),

    // 1 250.50 EUR, 15 000 km, 20 %, 3 years
    quantity: $ => seq(field('value', $.number), field('unit', $.unit)),

    unit: $ => choice('%', $.identifier),

    // A hyphen belongs to a name between letters or digits: net-total is a name, net - total a subtraction.
    identifier: _ => /[\p{L}_]([\p{L}\p{Nd}_]|-[\p{L}\p{Nd}])*/u,

    // Thousands are grouped with single spaces, as in a document: 1 250.50.
    number: _ => /[0-9]+( [0-9]+)*(\.[0-9]+)?/,

    // A string holds on one line, with the escapes \" and \\. A {placeholder} is what a reads text fills in.
    string: $ => seq(
      '"',
      repeat(choice($.string_content, $.escape_sequence, $.placeholder)),
      token.immediate('"'),
    ),

    string_content: _ => token.immediate(/[^"\\{\n]+|\{/),

    escape_sequence: _ => token.immediate(/\\["\\]/),

    placeholder: _ => token.immediate(prec(1, /\{[^{}"\\\n]*\}/)),

    comment: _ => token(seq('//', /[^\n]*/)),
  },
});

/**
 * Items separated by commas, maybe none.
 *
 * @param {RuleOrLiteral} rule
 * @returns {ChoiceRule}
 */
function commaSep(rule) {
  return optional(seq(rule, repeat(seq(',', rule))));
}
