package version

import "testing"

func TestCurrentUsesBuildVariables(t *testing.T) {
	oldName, oldVersion, oldCommit, oldDate := Name, Version, Commit, Date
	defer func() {
		Name, Version, Commit, Date = oldName, oldVersion, oldCommit, oldDate
	}()

	Name = "test-name"
	Version = "v1.2.3"
	Commit = "abc123"
	Date = "2026-04-27T00:00:00Z"

	got := Current()
	if got.Name != Name || got.Version != Version || got.Commit != Commit || got.Date != Date {
		t.Fatalf("Current() = %+v", got)
	}
}
