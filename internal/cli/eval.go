package cli

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/bn-k/kanon/internal/diag"
	"github.com/bn-k/kanon/internal/eval"
	"github.com/bn-k/kanon/internal/lint"
)

// runEval lints a document, then prints the value of each of its computed fields, and what lint found. A document
// whose rules fail still has its values.
func runEval(args []string, stdout, stderr io.Writer) int {
	schemaPath, docPath, today, err := lintArgs("eval", args)
	if err != nil {
		fmt.Fprintf(stderr, "kan: %v\n%s\n", err, usage)
		return 2
	}
	d, r := lint.LoadOn(schemaPath, docPath, today)
	if d != nil {
		for _, line := range eval.Computed(d) {
			fmt.Fprintln(stdout, line)
		}
	}
	for _, d := range r.Diags {
		fmt.Fprintln(stdout, d)
	}
	errs, warns := r.Diags.Count(diag.Error), r.Diags.Count(diag.Warning)
	if errs+warns > 0 {
		fmt.Fprintln(stdout, summary(errs, warns))
	}
	if errs > 0 {
		return 1
	}
	return 0
}

// lintArgs returns the schema, the document and the date of today of a lint or eval command: the date given by
// -today, or else the local date.
func lintArgs(cmd string, args []string) (schemaPath, docPath string, today time.Time, err error) {
	today = time.Now()
	var rest []string
	for i := 0; i < len(args); i++ {
		if args[i] != "-today" {
			rest = append(rest, args[i])
			continue
		}
		if i++; i == len(args) {
			return "", "", today, errors.New("-today takes a date, as in -today 2026-10-22")
		}
		if today, err = time.ParseInLocation(time.DateOnly, args[i], time.Local); err != nil {
			return "", "", today, fmt.Errorf("write -today 2026-10-22, not %q", args[i])
		}
	}
	schemaPath, docPath, err = paths(cmd, rest)
	return schemaPath, docPath, today, err
}
