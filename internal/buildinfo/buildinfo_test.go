package buildinfo

import (
	"strings"
	"testing"
)

func TestCurrentContainsRuntimeIdentity(t *testing.T) {
	info := Current()
	if info.Version == "" || info.Commit == "" || info.BuildDate == "" || info.GoVersion == "" {
		t.Fatalf("build identity contains empty field: %+v", info)
	}
	if !strings.Contains(info.String(), "commit=") {
		t.Fatalf("summary does not include commit: %q", info.String())
	}
}
