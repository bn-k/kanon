package sqlexport

import (
	"database/sql"
	"os"
	"testing"
)

const (
	northwindSchema = "../../cases/northwind/schema.kanonarch"
	northwindDoc    = "../../cases/northwind/northwind.kan"
)

// BenchmarkDatabase lays out the rows of Northwind and writes them to a new SQLite database in memory.
func BenchmarkDatabase(b *testing.B) {
	d := doc(b, northwindSchema, northwindDoc)
	b.ReportAllocs()
	for b.Loop() {
		e := export(b, d)
		db, _ := sql.Open("sqlite", ":memory:")
		db.SetMaxOpenConns(1) // an in-memory database lives in its connection
		err := e.write(db, e.layout.DDL())
		db.Close()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkQuery runs the query of cases/northwind-query over Northwind as kan sql -q does: in a new database.
func BenchmarkQuery(b *testing.B) {
	q, err := os.ReadFile("../../cases/northwind-query/revenue.sql")
	if err != nil {
		b.Fatal(err)
	}
	e := export(b, doc(b, northwindSchema, northwindDoc))
	b.ReportAllocs()
	for b.Loop() {
		r, ds := e.Query("revenue.sql", string(q))
		if len(ds) > 0 || len(r.Rows) != 5 {
			b.Fatalf("the query fails: %v", ds)
		}
	}
}
