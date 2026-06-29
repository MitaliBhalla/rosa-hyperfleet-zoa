package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the semantic version (set via -ldflags at build time).
	Version = "dev"
	// GitCommit is the short SHA of the commit (set via -ldflags at build time).
	GitCommit = "unknown"
	// BuildDate is the ISO 8601 timestamp of the build (set via -ldflags at build time).
	BuildDate = "unknown"
)

type Info struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

func Get() Info {
	return Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

func (i Info) String() string {
	return fmt.Sprintf("zoa %s (commit: %s, built: %s, %s, %s)",
		i.Version, i.GitCommit, i.BuildDate, i.GoVersion, i.Platform)
}

func (i Info) Short() string {
	return fmt.Sprintf("%s (%s)", i.Version, i.GitCommit)
}
