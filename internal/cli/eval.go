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
// whose rules fail still has its values. With -format json, it prints what lint found and every object of the
// documents, with all their fields.
func runEval(args []string, stdout, stderr io.Writer) int {
	o, err := lintArgs("eval", args)
	if err != nil {
		fmt.Fprintf(stderr, "kan: %v\n%s\n", err, usage)
		return 2
	}
	d, r := lint.LoadOn(o.schema, o.doc, o.today)
	if o.json {
		docs := newTree(o.schema).documents(d)
		return report(stdout, stderr, r, &docs)
	}
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

// report prints what lint found as JSON, for kan lint and kan eval with -format json: whether it found no error, the
// files it read, its diagnostics and, for eval, the objects of the documents. It returns the exit code.
func report(stdout, stderr io.Writer, r lint.Result, docs *[]object) int {
	ok := r.Diags.Count(diag.Error) == 0
	out := struct {
		OK          bool              `json:"ok"`
		Checked     []string          `json:"checked"`
		Diagnostics []diag.Diagnostic `json:"diagnostics"`
		Documents   *[]object         `json:"documents,omitempty"`
	}{ok, nonNil(r.Files), nonNil(r.Diags), docs}
	if err := printJSON(stdout, out); err != nil {
		fmt.Fprintf(stderr, "kan: %v\n", err)
		return 1
	}
	if !ok {
		return 1
	}
	return 0
}

// lintOptions are the arguments of a lint or eval command.
type lintOptions struct {
	schema, doc string
	today       time.Time // the date given by -today, or else the local date
	json        bool      // -format json
}

// lintArgs reads the arguments of a lint or eval command: -today and -format, the schema and the document.
func lintArgs(cmd string, args []string) (lintOptions, error) {
	o := lintOptions{today: time.Now()}
	var rest []string
	var err error
	for i := 0; i < len(args) && err == nil; i++ {
		switch args[i] {
		case "-today":
			i++
			o.today, err = todayArg(args, i)
		case "-format":
			i++
			o.json, err = formatArg(args, i)
		default:
			rest = append(rest, args[i])
		}
	}
	if err != nil {
		return o, err
	}
	o.schema, o.doc, err = paths(cmd, rest)
	return o, err
}

// todayArg reads args[i], the date after -today.
func todayArg(args []string, i int) (time.Time, error) {
	if i == len(args) {
		return time.Time{}, errors.New("-today takes a date, as in -today 2026-10-22")
	}
	today, err := time.ParseInLocation(time.DateOnly, args[i], time.Local)
	if err != nil {
		return today, fmt.Errorf("write -today 2026-10-22, not %q", args[i])
	}
	return today, nil
}

// formatArg reads args[i], the format after -format: whether it is json rather than text.
func formatArg(args []string, i int) (bool, error) {
	if i == len(args) {
		return false, errors.New("-format takes text or json")
	}
	switch args[i] {
	case "text":
		return false, nil
	case "json":
		return true, nil
	}
	return false, fmt.Errorf("-format is text or json, not %s", args[i])
}
