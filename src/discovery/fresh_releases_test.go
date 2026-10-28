package discovery

import "testing"

func TestPickFreshReleases(t *testing.T) {
	in := []freshRelease{
		{ReleaseMbid: "single", ReleaseGroupMbid: "single-group", ReleaseGroupPrimaryType: "Single", ReleaseDate: "2026-01-03"},
		{ReleaseMbid: "album-old", ReleaseGroupMbid: "album-group", ReleaseGroupPrimaryType: "Album", ReleaseDate: "2026-01-01", Confidence: 0.2},
		{ReleaseMbid: "album-new", ReleaseGroupMbid: "album-group", ReleaseGroupPrimaryType: "Album", ReleaseDate: "2026-01-02", Confidence: 0.8},
		{ReleaseMbid: "", ReleaseGroupMbid: "invalid-group", ReleaseGroupPrimaryType: "Album"},
		{ReleaseMbid: "ep", ReleaseGroupMbid: "ep-group", ReleaseGroupPrimaryType: "EP", ReleaseDate: "2026-01-02"},
	}
	got := pickFreshReleases(in, 8)
	if len(got) != 3 {
		t.Fatalf("len=%d want 3: %+v", len(got), got)
	}
	if got[0].ReleaseMbid != "album-new" || got[1].ReleaseMbid != "ep" || got[2].ReleaseMbid != "single" {
		t.Fatalf("unexpected order: %#v", got)
	}
	if got := pickFreshReleases(in, 2); len(got) != 2 {
		t.Fatalf("limited len=%d want 2", len(got))
	}
	if got := pickFreshReleases(in, 0); len(got) != 3 {
		t.Fatalf("unlimited len=%d want 3", len(got))
	}
}
