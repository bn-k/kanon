package cli

import (
	"bytes"
	"runtime"
	"runtime/debug"
	"testing"
)

func TestVersion(t *testing.T) {
	defer func(v string, read func() (*debug.BuildInfo, bool)) { version, readBuildInfo = v, read }(version, readBuildInfo)
	platform := runtime.Version() + ", " + runtime.GOOS + "/" + runtime.GOARCH
	revision := []debug.BuildSetting{{Key: "vcs.revision", Value: "0e8d235c97cd5202591864bc472bd0cc1fb5cb78"}}
	tests := []struct {
		version string
		info    *debug.BuildInfo
		want    string
	}{
		// A release: the version is set when it is built, from the commit of its tag.
		{"0.1.0", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: revision},
			"kan 0.1.0 (commit 0e8d235, " + platform + ")"},
		// go install github.com/bn-k/kanon/cmd/kan@v0.1.0: the module's version, and no commit.
		{"dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}, "kan 0.1.0 (" + platform + ")"},
		// go build in a clone: a version Go makes up for the commit, or none.
		{"dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.0.0-20261023201609-0e8d235c97cd+dirty"}, Settings: revision},
			"kan dev (commit 0e8d235, " + platform + ")"},
		{"dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.1-0.20261023201609-0e8d235c97cd"}, Settings: revision},
			"kan dev (commit 0e8d235, " + platform + ")"},
		{"dev", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: revision},
			"kan dev (commit 0e8d235, " + platform + ")"},
		{"dev", nil, "kan dev (" + platform + ")"},
	}
	for _, tt := range tests {
		version = tt.version
		readBuildInfo = func() (*debug.BuildInfo, bool) { return tt.info, tt.info != nil }
		for _, args := range [][]string{{"version"}, {"--version"}} {
			var stdout, stderr bytes.Buffer
			if code := Run(args, &stdout, &stderr); code != 0 || stdout.String() != tt.want+"\n" || stderr.Len() > 0 {
				t.Errorf("kan %s with version %q = %d, stdout %q, stderr %q, want %q",
					args[0], tt.version, code, stdout.String(), stderr.String(), tt.want)
			}
		}
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"version", "-v"}, &stdout, &stderr); code != 2 ||
		stderr.String() != "kan: version takes no argument, got -v\n"+usage+"\n" {
		t.Errorf("kan version -v = %d, stderr %q", code, stderr.String())
	}
}
