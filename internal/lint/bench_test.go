package lint

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bn-k/kanon/internal/diag"
)

// The benchmarks lint two cases on disk and a ledger generated in memory, of 1 000 and 10 000 invoices, with and
// without an order list, to show how lint grows with the number of documents:
// go test -run '^$' -bench . -benchmem ./internal/lint

// benchToday is the date of today for the benchmarks.
var benchToday = time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)

func BenchmarkLintBoard(b *testing.B) {
	benchLint(b, "../../cases/board/schema.kanonarch", "../../cases/board/board.kan", Options{Today: benchToday})
}

func BenchmarkLintNorthwind(b *testing.B) {
	benchLint(b, "../../cases/northwind/schema.kanonarch", "../../cases/northwind/northwind.kan", Options{Today: benchToday})
}

func BenchmarkLintLedger(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			benchLint(b, "schema.kanonarch", "ledger.kan", Options{Today: benchToday, FS: newMemFS(ledger(n))})
		})
	}
}

// BenchmarkLintOrdered lints the ledger with an order list of its invoices.
func BenchmarkLintOrdered(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			benchLint(b, "schema.kanonarch", "ledger.kan", Options{Today: benchToday, FS: newMemFS(withOrder(ledger(n), n))})
		})
	}
}

// benchLint lints the document at docPath, which must have no error, over and over.
func benchLint(b *testing.B, schemaPath, docPath string, opts Options) {
	b.ReportAllocs()
	for b.Loop() {
		d, r := LoadWith(schemaPath, docPath, opts)
		if d == nil || r.Diags.Count(diag.Error) > 0 {
			b.Fatalf("%s does not lint: %v", docPath, r.Diags)
		}
	}
}

// TestLedger checks that a small ledger lints without a word, so that the benchmarks measure a folder that lints.
func TestLedger(t *testing.T) {
	d, r := LoadWith("schema.kanonarch", "ledger.kan", Options{Today: benchToday, FS: newMemFS(ledger(40))})
	check(t, r, nil)
	if got := len(r.Files); got != 42 {
		t.Errorf("%d files read, want 42", got)
	}
	if n := len(d.Root.Get("clients").Objects[0].Get("invoices").Links); n != 10 {
		t.Errorf("the first client has %d invoices, want 10", n)
	}
	if got := d.Root.Get("invoices").Objects[0].Get("gross").Values[0].Raw; got != "120.60 EUR" {
		t.Errorf("the first invoice's gross is %s, want 120.60 EUR", got)
	}
	d, r = LoadWith("schema.kanonarch", "ledger.kan", Options{Today: benchToday, FS: newMemFS(withOrder(ledger(40), 40))})
	check(t, r, nil)
	if got := d.Root.Get("invoices").Objects[0].ID; got != "inv-00039" {
		t.Errorf("the order list puts %s first, want inv-00039", got)
	}
}

// ledgerSchema is the schema of a ledger: a link, an inverse, an enum, computed fields and two rules.
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

// withOrder adds to the ledger of n invoices files an order list of its invoices, the last one first.
func withOrder(files map[string]string, n int) map[string]string {
	var w strings.Builder
	w.WriteString(files["ledger.kan"] + "\n## Invoices\n\n")
	for i := n - 1; i >= 0; i-- {
		fmt.Fprintf(&w, "- [[ledger/invoices/inv-%05d]]\n", i)
	}
	files["ledger.kan"] = w.String()
	return files
}

// memFS is a folder in memory. Unlike fstest.MapFS, which scans every file to list a folder, it finds a file or a
// folder in one lookup, as the disk does, so that a benchmark measures lint and not the folder.
type memFS struct {
	files map[string][]byte
	dirs  map[string][]fs.DirEntry // the entries of each folder, in the order of their names
}

// newMemFS returns the folder that holds files, by path.
func newMemFS(files map[string]string) memFS {
	m := memFS{files: map[string][]byte{}, dirs: map[string][]fs.DirEntry{}}
	for path, src := range files {
		m.files[path] = []byte(src)
		info := memInfo{name: filepath.Base(path), size: int64(len(src))}
		for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
			_, seen := m.dirs[dir]
			m.dirs[dir] = append(m.dirs[dir], fs.FileInfoToDirEntry(info))
			if seen || dir == "." {
				break
			}
			info = memInfo{name: filepath.Base(dir), dir: true}
		}
	}
	for _, es := range m.dirs {
		slices.SortFunc(es, func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	}
	return m
}

func (m memFS) ReadFile(name string) ([]byte, error) {
	if src, ok := m.files[name]; ok {
		return src, nil
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (m memFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if es, ok := m.dirs[name]; ok {
		return es, nil
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

func (m memFS) Stat(name string) (fs.FileInfo, error) {
	if src, ok := m.files[name]; ok {
		return memInfo{name: filepath.Base(name), size: int64(len(src))}, nil
	}
	if _, ok := m.dirs[name]; ok {
		return memInfo{name: filepath.Base(name), dir: true}, nil
	}
	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

// memInfo describes a file or a folder of a memFS.
type memInfo struct {
	name string
	size int64
	dir  bool
}

func (i memInfo) Name() string       { return i.name }
func (i memInfo) Size() int64        { return i.size }
func (i memInfo) IsDir() bool        { return i.dir }
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) Sys() any           { return nil }

func (i memInfo) Mode() fs.FileMode {
	if i.dir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
