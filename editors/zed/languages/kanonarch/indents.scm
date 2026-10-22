(_
  "{"
  "}" @end) @indent

(_
  "("
  ")" @end) @indent

(_
  "["
  "]" @end) @indent

; The reads clauses and the body of a function or a rule go under its first line.
(function_declaration) @indent

(rule_declaration) @indent
