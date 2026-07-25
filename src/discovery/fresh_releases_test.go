package discovery

import "testing"

func TestMatchesArtists(t *testing.T) {
	mbids := map[string]struct{}{"artist-1": {}}
	names := map[string]struct{}{"artist one": {}}

	cases := []struct {
		name string
		rel  freshRelease
		want bool
	}{
		{
			name: "mbid hit",
			rel:  freshRelease{ArtistMbids: []string{"artist-1"}},
			want: true,
		},
		{
			name: "normalized name hit",
			rel:  freshRelease{ArtistCreditName: " Artist One "},
			want: true,
		},
		{
			name: "miss",
			rel:  freshRelease{ArtistCreditName: "Artist Two", ArtistMbids: []string{"artist-2"}},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchesArtists(tc.rel, mbids, names); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPickFreshReleases(t *testing.T) {
	in := []freshRelease{
		{ReleaseMbid: "single", ReleaseGroupMbid: "single-group", ReleaseGroupPrimaryType: "Single", ReleaseDate: "2026-01-03"},
		{ReleaseMbid: "album-old", ReleaseGroupMbid: "album-group", ReleaseGroupPrimaryType: "Album", ReleaseDate: "2026-01-01", Confidence: 0.2},
		{ReleaseMbid: "album-new", ReleaseGroupMbid: "album-group", ReleaseGroupPrimaryType: "Album", ReleaseDate: "2026-01-02", Confidence: 0.8},
		{ReleaseMbid: "", ReleaseGroupMbid: "invalid-group", ReleaseGroupPrimaryType: "Album"},
		{ReleaseMbid: "ep", ReleaseGroupMbid: "ep-group", ReleaseGroupPrimaryType: "EP", ReleaseDate: "2026-01-02"},
	}
	got := pickFreshReleases(in)
	if len(got) != 3 {
		t.Fatalf("len=%d want 3: %+v", len(got), got)
	}
	if got[0].ReleaseMbid != "album-new" || got[1].ReleaseMbid != "ep" || got[2].ReleaseMbid != "single" {
		t.Fatalf("unexpected order: %#v", got)
	}
}
