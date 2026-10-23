// Package cli runs the kan command.
package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/lint"
	"github.com/bn-k/kanon/internal/lsp"
	"github.com/bn-k/kanon/internal/sqlexport"
	"github.com/bn-k/kanon/internal/sqlimport"
)

const usage = `usage: kan lint [-today YYYY-MM-DD] <schema.kanonarch> <document.kan>
       kan eval [-today YYYY-MM-DD] <schema.kanonarch> <document.kan>
       kan write [-n] [-today YYYY-MM-DD] [-format text|json] <schema.kanonarch> <document.kan>[=<new.txt>]...
       kan import <database.sqlite|dump.sql> <out-dir>
       kan sql <schema.kanonarch> <document.kan> [-o out.sql|out.sqlite|out.db]
       kan sql -q <query|@query.sql> [-format table|csv] <schema.kanonarch> <document.kan>
       kan lsp`

// stdin is what kan lsp reads the editor's messages from, and kan write the new text of a document.
var stdin io.Reader = os.Stdin

// Run runs kan with args, without the program name, and returns its exit code: 0 when the documents are valid, or
// the import, the SQL or the documents are written; 1 when they have errors, or the import or the SQL fails; 2 when
// kan is misused.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "lint":
			return runLint(args[1:], stdout, stderr)
		case "eval":
			return runEval(args[1:], stdout, stderr)
		case "write":
			return runWrite(args[1:], stdout, stderr)
		case "import":
			return runImport(args[1:], stdout, stderr)
		case "sql":
			return runSQL(args[1:], stdout, stderr)
		case "lsp":
			return runLSP(args[1:], stdout, stderr)
		}
	}
	fmt.Fprintln(stderr, usage)
	return 2
}

// runLSP serves an editor on stdin and stdout, until it exits.
func runLSP(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintf(stderr, "kan: lsp takes no argument, got %s\n%s\n", strings.Join(args, " "), usage)
		return 2
	}
	if err := lsp.Serve(stdin, stdout); err != nil {
		fmt.Fprintf(stderr, "kan: %v\n", err)
		return 1
	}
	return 0
}

func runLint(args []string, stdout, stderr io.Writer) int {
	schemaPath, docPath, today, err := lintArgs("lint", args)
	if err != nil {
		fmt.Fprintf(stderr, "kan: %v\n%s\n", err, usage)
		return 2
	}
	_, r := lint.LoadOn(schemaPath, docPath, today)
	for _, d := range r.Diags {
		fmt.Fprintln(stdout, d)
	}
	errs, warns := r.Diags.Count(diag.Error), r.Diags.Count(diag.Warning)
	if errs+warns == 0 {
		for _, f := range r.Files {
			fmt.Fprintf(stdout, "ok  %s\n", f)
		}
		return 0
	}
	fmt.Fprintln(stdout, summary(errs, warns))
	if errs > 0 {
		return 1
	}
	return 0
}

// runImport writes the schema and documents of a database into a new folder, then prints what it changed or left
// out, and what it wrote.
func runImport(args []string, stdout, stderr io.Writer) int {
	if err := importArgs(args); err != nil {
		fmt.Fprintf(stderr, "kan: %v\n%s\n", err, usage)
		return 2
	}
	src, dir := args[0], args[1]
	r, err := sqlimport.Import(src, dir)
	if err != nil {
		fmt.Fprintf(stderr, "kan: %v\n", err)
		return 1
	}
	for _, n := range r.Notes {
		fmt.Fprintf(stdout, "%s: note: %s\n", src, n)
	}
	fmt.Fprintf(stdout, "wrote %s: %s, %s, %s\n", dir, plural(r.Schemas, "schema"), plural(r.Documents, "document"),
		plural(r.Files, "file"))
	return 0
}

// importArgs checks the database and the folder of an import command: the database exists, and the folder is new.
func importArgs(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("import takes a database and a folder, got %s", plural(len(args), "argument"))
	}
	if _, err := os.Stat(args[0]); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s does not exist", args[0])
	}
	return sqlimport.CheckDir(args[1])
}

// runSQL lints a document, then prints its SQL script or writes it to the file named by -o: a script, or a SQLite
// database; or, with -q, runs a query over its tables and prints the rows. When the document has errors, it prints
// them and writes nothing.
func runSQL(args []string, stdout, stderr io.Writer) int {
	o, err := sqlArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "kan: %v\n%s\n", err, usage)
		return 2
	}
	out := o.out
	d, r := lint.Load(o.schema, o.doc)
	for _, d := range r.Diags {
		fmt.Fprintln(stderr, d)
	}
	if errs := r.Diags.Count(diag.Error); errs > 0 {
		fmt.Fprintln(stderr, summary(errs, r.Diags.Count(diag.Warning)))
		return 1
	}
	e, ds := sqlexport.New(d)
	if e == nil {
		for _, d := range ds {
			fmt.Fprintln(stderr, d)
		}
		fmt.Fprintln(stderr, summary(ds.Count(diag.Error), ds.Count(diag.Warning)))
		return 1
	}
	if o.query != "" {
		return runQuery(e, o, stdout, stderr)
	}
	switch filepath.Ext(out) {
	case "":
		err = e.SQL(stdout)
	case ".sql":
		err = e.Script(out)
	default:
		err = e.Database(out)
	}
	if err != nil {
		fmt.Fprintf(stderr, "kan: %v\n", err)
		return 1
	}
	if out != "" {
		fmt.Fprintf(stdout, "wrote %s: %s, %s\n", out, plural(e.Tables(), "table"), plural(e.Rows(), "row"))
	}
	return 0
}

// sqlOptions are the arguments of a sql command.
type sqlOptions struct {
	schema, doc string
	out         string // the file named by -o: new, and empty when the script goes to stdout
	query       string // the query of -q, empty without
	source      string // where the query is written: the file of -q @file, "query" for -q 'SELECT …'
	csv         bool   // -format csv
}

// sqlArgs reads the arguments of a sql command: the schema and the document, and -o, or -q with -format.
func sqlArgs(args []string) (sqlOptions, error) {
	var o sqlOptions
	var rest []string
	var format string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o":
			if i++; i == len(args) {
				return o, errors.New("-o takes a file: out.sql, out.sqlite or out.db")
			}
			o.out = args[i]
			switch filepath.Ext(o.out) {
			case ".sql", ".sqlite", ".db":
			default:
				return o, fmt.Errorf("-o names a .sql, .sqlite or .db file, not %s", o.out)
			}
		case "-q":
			if i++; i == len(args) || strings.TrimSpace(args[i]) == "" {
				return o, errors.New("-q takes a query: -q 'SELECT …' or -q @query.sql")
			}
			o.query, o.source = args[i], "query"
		case "-format":
			if i++; i == len(args) {
				return o, errors.New("-format takes table or csv")
			}
			if format = args[i]; format != "table" && format != "csv" {
				return o, fmt.Errorf("-format is table or csv, not %s", format)
			}
		default:
			rest = append(rest, args[i])
		}
	}
	var err error
	if o.schema, o.doc, err = paths("sql", rest); err != nil {
		return o, err
	}
	switch {
	case o.query != "" && o.out != "":
		return o, errors.New("-q prints the rows and writes no file: drop -o")
	case o.query == "" && format != "":
		return o, errors.New("-format goes with -q")
	}
	o.csv = format == "csv"
	if file, ok := strings.CutPrefix(o.query, "@"); ok {
		src, err := os.ReadFile(file)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return o, fmt.Errorf("%s does not exist", file)
		case err != nil:
			return o, fmt.Errorf("cannot read %s: %v", file, errors.Unwrap(err))
		case strings.TrimSpace(string(src)) == "":
			return o, fmt.Errorf("%s holds no query", file)
		}
		o.query, o.source = string(src), file
	}
	if o.out != "" {
		err = sqlexport.CheckFile(o.out)
	}
	return o, err
}

// runQuery runs the query of o over the tables of e and prints its rows, or what is wrong with the query.
func runQuery(e *sqlexport.Export, o sqlOptions, stdout, stderr io.Writer) int {
	res, ds := e.Query(o.source, o.query)
	if res == nil {
		for _, d := range ds {
			fmt.Fprintln(stderr, d)
		}
		return 1
	}
	var err error
	if o.csv {
		err = res.CSV(stdout)
	} else {
		err = res.Table(stdout)
	}
	if err != nil {
		fmt.Fprintf(stderr, "kan: %v\n", err)
		return 1
	}
	return 0
}

// paths returns the schema and the document of a command, in either order.
func paths(cmd string, args []string) (schemaPath, docPath string, err error) {
	if len(args) != 2 {
		return "", "", fmt.Errorf("%s takes a schema and a document, got %s", cmd, plural(len(args), "argument"))
	}
	a, b := args[0], args[1]
	if strings.HasSuffix(a, ".kan") && strings.HasSuffix(b, ".kanonarch") {
		a, b = b, a
	}
	switch {
	case !strings.HasSuffix(a, ".kanonarch"):
		return "", "", fmt.Errorf("%s is not a schema: a schema file ends in .kanonarch", a)
	case !strings.HasSuffix(b, ".kan"):
		return "", "", fmt.Errorf("%s is not a document: a document file ends in .kan", b)
	}
	return a, b, nil
}

// summary counts errors and warnings: "2 errors, 1 warning".
func summary(errs, warns int) string {
	var parts []string
	if errs > 0 {
		parts = append(parts, plural(errs, "error"))
	}
	if warns > 0 {
		parts = append(parts, plural(warns, "warning"))
	}
	return strings.Join(parts, ", ")
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
