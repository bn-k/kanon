package schema

import (
	"fmt"
	"strings"
	"testing"
)

// bigSchema is a schema of 100 groups, about 3 000 lines: in each an enum, an order with links, an inverse, a list
// of lines and computed fields, a function and a rule.
func bigSchema() string {
	var w strings.Builder
	w.WriteString("unit crate\n\n")
	for i := range 100 {
		fmt.Fprintf(&w, `enum Status%[1]d { open, paid, void }

schema Customer%[1]d {
  name:   Text
  orders: [Order%[1]d] <- customer
}

schema Line%[1]d {
  description: Text
  quantity:    Int
  unit-price:  Money

  amount: Money = unit-price * quantity
}

schema Order%[1]d {
  title:    Text @heading
  status:   Status%[1]d
  customer: -> Customer%[1]d
  ordered:  Date
  weight:   Quantity?
  lines:    [Line%[1]d]
  notes:    Prose?

  net:   Money = sum(lines.amount)
  vat:   Money = round(net * 20 %%, 0.01)
  gross: Money = net + vat
}

fn open-orders%[1]d(c: Customer%[1]d) -> Int
  reads en "the open orders of {c.name}"
  = count(c.orders, o => o.status == open)

rule few-open%[1]d(c: Customer%[1]d) -> Bool
  reads en "{c.name} has at most 10 open orders"
  = open-orders%[1]d(c) <= 10 and sum(c.orders.filter(o => o.gross > 0 EUR).map(o => o.net)) >= 0 EUR

`, i)
	}
	return w.String()
}

// BenchmarkParse parses a large schema.
func BenchmarkParse(b *testing.B) {
	src := bigSchema()
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		if _, ds := Parse("big.kanonarch", src); len(ds) > 0 {
			b.Fatal(ds)
		}
	}
}

// BenchmarkLoad parses and checks a large schema, as each lint run does.
func BenchmarkLoad(b *testing.B) {
	src := []byte(bigSchema())
	read := func(string) ([]byte, error) { return src, nil }
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		if _, ds := LoadFrom("big.kanonarch", read); len(ds) > 0 {
			b.Fatal(ds)
		}
	}
}
