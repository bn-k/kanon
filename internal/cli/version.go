package cli

import (
	"fmt"
	"io"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
)

// version is the version of kan, set when a release is built:
// -ldflags "-X github.com/bn-k/kanon/internal/cli.version=0.1.0". Otherwise it is the module's version, when kan
// was installed with go install …@v0.1.0, or dev.
var version = "dev"

// readBuildInfo is debug.ReadBuildInfo; the tests replace it.
var readBuildInfo = debug.ReadBuildInfo

// untagged matches the version Go stamps on a build from a commit that no tag names, or from changes not yet
// committed: v0.0.0-20261023201609-0e8d235c97cd, v0.1.1-0.20261023201609-0e8d235c97cd+dirty.
var untagged = regexp.MustCompile(`[0-9]{14}-[0-9a-f]{12}|\+dirty$`)

// runVersion prints the version of kan, the commit it was built from when it is known, Go's version and the
// platform, so that a bug report says what it was found with.
func runVersion(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintf(stderr, "kan: version takes no argument, got %s\n%s\n", strings.Join(args, " "), usage)
		return 2
	}
	v, built := version, ""
	if info, ok := readBuildInfo(); ok {
		if v == "dev" && strings.HasPrefix(info.Main.Version, "v") && !untagged.MatchString(info.Main.Version) {
			v = strings.TrimPrefix(info.Main.Version, "v")
		}
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				built = "commit " + s.Value[:7] + ", "
			}
		}
	}
	fmt.Fprintf(stdout, "kan %s (%s%s, %s/%s)\n", v, built, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	return 0
}
