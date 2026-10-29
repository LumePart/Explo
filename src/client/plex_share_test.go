package client

import (
	"explo/src/config"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"explo/src/models"
	"explo/src/util"
)

type recordedReq struct {
	Method, Path, Title string
}

func playlistServer(t *testing.T, reqs *[]recordedReq) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*reqs = append(*reqs, recordedReq{r.Method, r.URL.Path, r.URL.Query().Get("title")})
		mu.Unlock()
		body := `{}`
		if r.Method == http.MethodPost && r.URL.Path == "/playlists" {
			body = `{"MediaContainer":{"Metadata":[{"ratingKey":"99"}]}}`
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("fake server failed to write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestPlexCreatePlaylist_PostsTitleAndAddsTracks(t *testing.T) {
	var reqs []recordedReq
	server := playlistServer(t, &reqs)

	plex := NewPlex(testConfig(server.URL, ""), util.NewHttp(util.HttpClientConfig{Timeout: 5}))
	tracks := []*models.Track{{ID: "/library/metadata/1"}, {ID: "/library/metadata/2"}}

	if err := plex.CreatePlaylist(tracks); err != nil {
		t.Fatalf("CreatePlaylist() = %v", err)
	}

	var posts, puts int
	for _, r := range reqs {
		switch {
		case r.Method == "POST" && r.Path == "/playlists":
			posts++
			if r.Title != "Weekly Exploration" {
				t.Errorf("title = %q, want Weekly Exploration", r.Title)
			}
		case r.Method == "PUT" && r.Path == "/playlists/99/items":
			puts++
		}
	}
	if posts != 1 || puts != 2 || len(reqs) != 3 {
		t.Errorf("requests = %+v, want 1 POST /playlists and 2 PUT /playlists/99/items", reqs)
	}
	if plex.Cfg.PlaylistID != "99" {
		t.Errorf("PlaylistID = %q, want 99", plex.Cfg.PlaylistID)
	}
}

// shareReq is a request seen by a recipient's fake Plex server.
type shareReq struct {
	Method, Path, Token string
}

type recipientServer struct {
	mu   sync.Mutex
	reqs []shareReq
	url  string
}

func (r *recipientServer) requests() []shareReq {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]shareReq(nil), r.reqs...)
}

// newRecipientServer fakes one recipient's view of Plex. playlistsJSON is the
// body of GET /playlists; failCreate makes POST /playlists return 500.
func newRecipientServer(t *testing.T, playlistsJSON string, failCreate bool) *recipientServer {
	t.Helper()
	return newFaultyRecipientServer(t, playlistsJSON, serverFaults{create: failCreate})
}

// serverFaults selects which requests a fake recipient server fails.
type serverFaults struct {
	create, adds, list, del, emptyKey bool
}

func newFaultyRecipientServer(t *testing.T, playlistsJSON string, f serverFaults) *recipientServer {
	t.Helper()
	rs := &recipientServer{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.mu.Lock()
		rs.reqs = append(rs.reqs, shareReq{r.Method, r.URL.Path, r.Header.Get("X-Plex-Token")})
		rs.mu.Unlock()
		body := `{}`
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/playlists":
			if f.list {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			body = playlistsJSON
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/items") && f.adds,
			r.Method == http.MethodDelete && f.del:
			w.WriteHeader(http.StatusInternalServerError)
			return
		case r.Method == http.MethodPost && r.URL.Path == "/playlists" && f.emptyKey:
			body = `{"MediaContainer":{"Metadata":[{"ratingKey":""}]}}`
		case r.Method == http.MethodPost && r.URL.Path == "/playlists":
			if f.create {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			body = `{"MediaContainer":{"Metadata":[{"ratingKey":"99"}]}}`
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	rs.url = server.URL
	return rs
}

const existingCopies = `{"MediaContainer":{"Metadata":[
	{"ratingKey":"7","title":"Gabe's Weekly-Exploration"},
	{"ratingKey":"8","title":"Gabe's Weekly-Exploration (old)"},
	{"ratingKey":"9","title":"Mix"}]}}`

// sharingPlex returns an owner Plex whose switcher maps names to clients
// pointed at the given servers, each with its own token.
func sharingPlex(servers map[string]*recipientServer) *Plex {
	hc := util.NewHttp(util.HttpClientConfig{Timeout: 5})
	owner := NewPlex(testConfig("http://owner.invalid", ""), hc)
	owner.Cfg.PlaylistDescr = "desc"
	owner.Cfg.PlaylistName = "Weekly-Exploration"
	owner.userSwitcher = func(name string) (*Plex, error) {
		rs, ok := servers[name]
		if !ok {
			return nil, fmt.Errorf("unable to find shared user: %s", name)
		}
		cfg := testConfig(rs.url, "")
		cfg.Creds.Headers = map[string]string{"X-Plex-Token": "tok-" + name}
		return NewPlex(cfg, hc), nil
	}
	return owner
}

var shareTracks = []*models.Track{{ID: "/library/metadata/1"}, {ID: ""}}

func indexOf(reqs []shareReq, method, path string) int {
	for i, r := range reqs {
		if r.Method == method && r.Path == path {
			return i
		}
	}
	return -1
}

func deletes(reqs []shareReq) []string {
	var out []string
	for _, r := range reqs {
		if r.Method == http.MethodDelete {
			out = append(out, r.Path)
		}
	}
	return out
}

func TestSharePlaylist_CreatesCopyThenDeletesOld(t *testing.T) {
	rs := newRecipientServer(t, existingCopies, false)
	plex := sharingPlex(map[string]*recipientServer{"ana": rs})
	if err := plex.SharePlaylist(shareTracks, "Gabe", []string{"ana"}, true); err != nil {
		t.Fatalf("SharePlaylist() = %v", err)
	}
	reqs := rs.requests()
	post, del := indexOf(reqs, "POST", "/playlists"), indexOf(reqs, "DELETE", "/playlists/7")
	if post < 0 || del < 0 || post > del {
		t.Errorf("want POST /playlists before DELETE /playlists/7, got %+v", reqs)
	}
	if indexOf(reqs, "PUT", "/playlists/99") < 0 {
		t.Errorf("summary not set on the new copy: %+v", reqs)
	}
}

func TestSharePlaylist_KeepsUnrelatedPlaylists(t *testing.T) {
	rs := newRecipientServer(t, existingCopies, false)
	plex := sharingPlex(map[string]*recipientServer{"ana": rs})
	if err := plex.SharePlaylist(shareTracks, "Gabe", []string{"ana"}, true); err != nil {
		t.Fatalf("SharePlaylist() = %v", err)
	}
	if d := deletes(rs.requests()); len(d) != 1 || d[0] != "/playlists/7" {
		t.Errorf("deletes = %v, want only /playlists/7", d)
	}
}

func TestSharePlaylist_ReplaceOffDeletesNothing(t *testing.T) {
	rs := newRecipientServer(t, existingCopies, false)
	plex := sharingPlex(map[string]*recipientServer{"ana": rs})
	if err := plex.SharePlaylist(shareTracks, "Gabe", []string{"ana"}, false); err != nil {
		t.Fatalf("SharePlaylist() = %v", err)
	}
	reqs := rs.requests()
	if indexOf(reqs, "GET", "/playlists") >= 0 || len(deletes(reqs)) != 0 {
		t.Errorf("replace=false sent GET or DELETE: %+v", reqs)
	}
	if indexOf(reqs, "POST", "/playlists") < 0 {
		t.Errorf("no copy created: %+v", reqs)
	}
}

func TestSharePlaylist_CreateFailureKeepsOldAndContinues(t *testing.T) {
	bad := newRecipientServer(t, existingCopies, true)
	good := newRecipientServer(t, existingCopies, false)
	plex := sharingPlex(map[string]*recipientServer{"ana": bad, "ben": good})
	err := plex.SharePlaylist(shareTracks, "Gabe", []string{"ana", "ben"}, true)
	if err == nil || !strings.Contains(err.Error(), "ana") {
		t.Fatalf("err = %v, want one naming ana", err)
	}
	if d := deletes(bad.requests()); len(d) != 0 {
		t.Errorf("failed recipient had deletes %v", d)
	}
	if indexOf(good.requests(), "POST", "/playlists") < 0 || indexOf(good.requests(), "DELETE", "/playlists/7") < 0 {
		t.Errorf("second recipient not processed: %+v", good.requests())
	}
}

func TestSharePlaylist_UnknownUserSkipped(t *testing.T) {
	rs := newRecipientServer(t, existingCopies, false)
	plex := sharingPlex(map[string]*recipientServer{"ben": rs})
	err := plex.SharePlaylist(shareTracks, "Gabe", []string{"ghost", "ben"}, true)
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Errorf("err = %v, want one naming ghost", err)
	}
	if indexOf(rs.requests(), "POST", "/playlists") < 0 {
		t.Errorf("ben not processed: %+v", rs.requests())
	}
}

func TestSharePlaylist_UsesRecipientToken(t *testing.T) {
	a := newRecipientServer(t, existingCopies, false)
	b := newRecipientServer(t, existingCopies, false)
	plex := sharingPlex(map[string]*recipientServer{"ana": a, "ben": b})
	if err := plex.SharePlaylist(shareTracks, "Gabe", []string{"ana", "ben"}, true); err != nil {
		t.Fatalf("SharePlaylist() = %v", err)
	}
	for name, rs := range map[string]*recipientServer{"ana": a, "ben": b} {
		reqs := rs.requests()
		if len(reqs) == 0 {
			t.Errorf("%s: no requests", name)
		}
		for _, r := range reqs {
			if r.Token != "tok-"+name {
				t.Errorf("%s: %s %s carried token %q", name, r.Method, r.Path, r.Token)
			}
		}
	}
}

func TestSharePlaylist_OwnerAccountRecipient(t *testing.T) {
	rs := newRecipientServer(t, `{}`, false)
	hc := util.NewHttp(util.HttpClientConfig{Timeout: 5})
	admin := NewPlex(testConfig(rs.url, ""), hc)
	plex := NewPlex(testConfig(rs.url, ""), hc)
	plex.AdminClient = admin
	plex.userSwitcher = func(string) (*Plex, error) { cp := *admin; return &cp, nil }
	if err := plex.SharePlaylist(shareTracks, "Gabe", []string{"owner"}, true); err != nil {
		t.Fatalf("SharePlaylist() = %v", err)
	}
	if indexOf(rs.requests(), "POST", "/playlists") < 0 {
		t.Errorf("no copy created: %+v", rs.requests())
	}
}

func TestSharePlaylist_NoAdminClient(t *testing.T) {
	rs := newRecipientServer(t, existingCopies, false)
	plex := NewPlex(testConfig(rs.url, ""), util.NewHttp(util.HttpClientConfig{Timeout: 5}))
	if err := plex.SharePlaylist(shareTracks, "Gabe", []string{"ana"}, true); err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if n := len(rs.requests()); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

type nonSharerAPI struct{ APIClient }

func TestClientSharePlaylist_NonSharerBackend(t *testing.T) {
	c := &Client{Cfg: &config.ClientConfig{ShareWith: []string{"ana"}}, API: nonSharerAPI{}}
	if err := c.SharePlaylist(shareTracks); err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}

type recordingSharer struct {
	nonSharerAPI
	calls int
}

func (r *recordingSharer) SharePlaylist([]*models.Track, string, []string, bool) error {
	r.calls++
	return nil
}

func TestClientSharePlaylist_EmptyListIsNoOp(t *testing.T) {
	api := &recordingSharer{}
	c := &Client{Cfg: &config.ClientConfig{}, API: api}
	if err := c.SharePlaylist(shareTracks); err != nil || api.calls != 0 {
		t.Errorf("err = %v, calls = %d; want nil, 0", err, api.calls)
	}
}

func TestSharePlaylist_AllAddsFailKeepsOldCopy(t *testing.T) {
	rs := newFaultyRecipientServer(t, existingCopies, serverFaults{adds: true})
	plex := sharingPlex(map[string]*recipientServer{"ana": rs})
	err := plex.SharePlaylist(shareTracks, "Gabe", []string{"ana"}, true)
	if err == nil || !strings.Contains(err.Error(), "ana") {
		t.Errorf("err = %v, want one naming ana", err)
	}
	if d := deletes(rs.requests()); len(d) != 0 {
		t.Errorf("deletes = %v, want none", d)
	}
}

func TestSharePlaylist_EmptyRatingKeyIsCreateFailure(t *testing.T) {
	rs := newFaultyRecipientServer(t, existingCopies, serverFaults{emptyKey: true})
	plex := sharingPlex(map[string]*recipientServer{"ana": rs})
	if err := plex.SharePlaylist(shareTracks, "Gabe", []string{"ana"}, true); err == nil {
		t.Error("err = nil, want create failure")
	}
	for _, r := range rs.requests() {
		if r.Method == http.MethodDelete || r.Method == http.MethodPut {
			t.Errorf("unexpected %s %s after empty rating key", r.Method, r.Path)
		}
	}
}

func TestSharePlaylist_ListFailureCreatesNothing(t *testing.T) {
	rs := newFaultyRecipientServer(t, existingCopies, serverFaults{list: true})
	plex := sharingPlex(map[string]*recipientServer{"ana": rs})
	if err := plex.SharePlaylist(shareTracks, "Gabe", []string{"ana"}, true); err == nil {
		t.Error("err = nil, want list failure")
	}
	if indexOf(rs.requests(), "POST", "/playlists") >= 0 {
		t.Errorf("copy created despite failed list: %+v", rs.requests())
	}
}

func TestSharePlaylist_DeleteFailureReportedAndContinues(t *testing.T) {
	bad := newFaultyRecipientServer(t, existingCopies, serverFaults{del: true})
	good := newRecipientServer(t, existingCopies, false)
	plex := sharingPlex(map[string]*recipientServer{"ana": bad, "ben": good})
	err := plex.SharePlaylist(shareTracks, "Gabe", []string{"ana", "ben"}, true)
	if err == nil || !strings.Contains(err.Error(), "ana") {
		t.Errorf("err = %v, want one naming ana", err)
	}
	if indexOf(good.requests(), "DELETE", "/playlists/7") < 0 {
		t.Errorf("ben not processed: %+v", good.requests())
	}
}
