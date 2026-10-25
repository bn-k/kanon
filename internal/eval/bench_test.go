package eval_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/eval"
	"github.com/bn-k/kanon/internal/lint"
)

// BenchmarkEval evaluates the computed fields and the rules of a ledger of 1 000 and 10 000 invoices, the folder
// that BenchmarkLintLedger of internal/lint lints. Lint evaluates it once; each run evaluates it again, and puts the
// same values in the model.
func BenchmarkEval(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			b.Chdir(b.TempDir())
			for path, src := range ledger(n) {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					b.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
					b.Fatal(err)
				}
			}
			d, r := lint.LoadOn("schema.kanonarch", "ledger.kan", today)
			if d == nil || r.Diags.Count(diag.Error) > 0 {
				b.Fatalf("the ledger does not lint: %v", r.Diags)
			}
			b.ReportAllocs()
			for b.Loop() {
				if ds := eval.Eval(d, today); len(ds) > 0 {
					b.Fatalf("the ledger does not evaluate: %v", ds)
				}
			}
		})
	}
}

// ledgerSchema and ledger are those of internal/lint/bench_test.go: a test file can't be shared between packages.
const ledgerSchema = `// A ledger of invoices, generated for the benchmarks.

enum Status { open, paid, void }

schema Ledger {
  name:     Text @heading
  clients:  [Client]
  invoices: [Invoice]
}

schema Client {
  name:     Text
  invoices: [Invoice] <- client
}

schema Invoice {
  title:  Text @heading
  status: Status
  client: -> Client
  issued: Date
  net:    Money
  notes:  Prose?

  vat:   Money = round(net * 20 %, 0.01)
  gross: Money = net + vat
}

rule few-open(c: Client) -> Bool
  reads en "{c.name} has at most 100 open invoices"
  = count(c.invoices, i => i.status == open) <= 100

rule not-empty(i: Invoice) -> Bool
  reads en "an invoice is for more than nothing"
  = i.gross > 0 EUR
`

// ledger returns the files of a ledger of n invoices, by path: the schema, ledger.kan with a table of one client per
// ten invoices, and ledger/invoices/ with one file per invoice, which links to its client twice.
func ledger(n int) map[string]string {
	files := map[string]string{"schema.kanonarch": ledgerSchema}
	var w strings.Builder
	w.WriteString("---\ntype: Ledger\n---\n\n# Ledger\n\n## Clients\n\n| id | name |\n|---|---|\n")
	for c := range (n + 9) / 10 {
		fmt.Fprintf(&w, "| c-%d | Client %d |\n", c, c)
	}
	files["ledger.kan"] = w.String()
	statuses := []string{"open", "paid", "void"}
	for i := range n {
		files[fmt.Sprintf("ledger/invoices/inv-%05d.kan", i)] = fmt.Sprintf("# Invoice %d\n\nstatus: %s\nclient: [[/ledger#c-%d]]\n"+
			"issued: 2026-%02d-%02d\nnet: %d.50 EUR\n\n## Notes\n\nWork for [[/ledger#c-%d]], as agreed.\n",
			i, statuses[i%3], i/10, i%12+1, i%28+1, 100+i%900, i/10)
	}
	return files
}
