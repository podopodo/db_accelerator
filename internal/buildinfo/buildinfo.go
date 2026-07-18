// Package buildinfo exposes immutable build metadata injected by release builds.
package buildinfo

import (
	"fmt"
	"runtime"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Info is the public, non-secret build identity.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
}

// Current returns the process build identity.
func Current() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
	}
}

func (i Info) String() string {
	return fmt.Sprintf("%s commit=%s built=%s go=%s", i.Version, i.Commit, i.BuildDate, i.GoVersion)
}
