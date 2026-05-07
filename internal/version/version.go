package version

import "fmt"

// Build-time values populated via -ldflags.
var (
	Name    = "agent-memory"
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// Info describes the running build.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// Current returns the current build information.
func Current() Info {
	return Info{
		Name:    Name,
		Version: Version,
		Commit:  Commit,
		Date:    Date,
	}
}

// String returns a human-readable version string.
func (i Info) String() string {
	return fmt.Sprintf("%s version=%s commit=%s date=%s", i.Name, i.Version, i.Commit, i.Date)
}
