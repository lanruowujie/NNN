package buildinfo

import "testing"

func TestCurrentHasStableDevelopmentDefaults(t *testing.T) {
	oldVersion, oldCommit, oldDate, oldDirty := Version, Commit, BuildDate, Dirty
	t.Cleanup(func() { Version, Commit, BuildDate, Dirty = oldVersion, oldCommit, oldDate, oldDirty })
	Version, Commit, BuildDate, Dirty = " 1.2.3 ", " abc123 ", " 2026-01-02T03:04:05Z ", "true"
	got := Current()
	if got.Version != "1.2.3" || got.Commit != "abc123" || got.BuildDate != "2026-01-02T03:04:05Z" || !got.Dirty {
		t.Fatalf("Current() = %#v", got)
	}
	if got.GoVersion == "" || got.WailsVersion != "2.15.0" || got.OS == "" || got.Architecture == "" {
		t.Fatalf("Current() omitted toolchain/platform fields: %#v", got)
	}
}
