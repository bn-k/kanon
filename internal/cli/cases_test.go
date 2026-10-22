package cli

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the expected.txt of every case")

// splitArgs splits a command line at its spaces; an argument in single or double quotes may hold some, and
// the quotes are not part of it. There are no escapes: use the other quote.
func splitArgs(s string) []string {
	var args []string
	var cur strings.Builder
	var quote rune
	started := false
	for _, r := range s {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			cur.WriteRune(r)
		case r == '\'' || r == '"':
			quote, started = r, true
		case r == ' ':
			if started || cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteRune(r)
		}
	}
	if started || cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
}

// TestCases runs each case of cases/: expected.txt holds the command on its first line, after "$ ", then what kan
// prints, then "[exit N]" when the exit code isn't 0. A case whose output depends on the date says so with -today.
func TestCases(t *testing.T) {
	dirs, err := filepath.Glob("../../cases/*/expected.txt")
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no cases found: %v", err)
	}
	for _, path := range dirs {
		dir := filepath.Dir(path)
		t.Run(filepath.Base(dir), func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			cmd, _, _ := strings.Cut(string(src), "\n")
			args, ok := strings.CutPrefix(cmd, "$ kan ")
			if !ok {
				t.Fatalf("%s: the first line is the command, as in $ kan lint schema.kanonarch doc.kan", path)
			}
			t.Chdir(dir)
			var out bytes.Buffer
			code := Run(splitArgs(args), &out, &out)
			got := cmd + "\n" + out.String()
			if code != 0 {
				got += fmt.Sprintf("[exit %d]\n", code)
			}
			if *update {
				if err := os.WriteFile("expected.txt", []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			if got != string(src) {
				t.Errorf("output differs from %s\n--- got\n%s--- want\n%s", path, got, src)
			}
		})
	}
}
