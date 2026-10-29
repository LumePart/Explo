package main

import (
	"bytes"
	"errors"
	"explo/src/web/backend/users"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeUsers(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if content != "" {
		if err := os.WriteFile(filepath.Join(dir, "users.json"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

type recorder struct {
	calls [][]string
	fail  map[string]bool
}

func (r *recorder) run(args []string) error {
	r.calls = append(r.calls, append([]string(nil), args...))
	for i, a := range args {
		if a == "--user" && i+1 < len(args) && r.fail[args[i+1]] {
			return errors.New("boom")
		}
	}
	return nil
}

func TestRewriteUserArg(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"space all", []string{"-p", "x", "--user", "all"}, []string{"-p", "x", "--user", "bob"}},
		{"equals all", []string{"--user=all", "-p", "x"}, []string{"-p", "x", "--user", "bob"}},
		{"repeated", []string{"--user", "a", "-p", "x", "--user=b"}, []string{"-p", "x", "--user", "bob"}},
		{"none", []string{"-p", "x"}, []string{"-p", "x", "--user", "bob"}},
	}
	for _, c := range cases {
		if got := rewriteUserArg(c.in, "bob"); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestRunAllUsers_NoUsersFileRunsSingleUser(t *testing.T) {
	r := &recorder{}
	err := runAllUsers(writeUsers(t, ""), "weekly-exploration", []string{"-p", "x", "--user", "all"}, r.run)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.calls, [][]string{{"-p", "x"}}) {
		t.Errorf("calls: %v", r.calls)
	}
}

func TestRunAllUsers_EmptyListRunsSingleUser(t *testing.T) {
	r := &recorder{}
	if err := runAllUsers(writeUsers(t, "[]"), "p", []string{"--user=all"}, r.run); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 1 || len(r.calls[0]) != 0 {
		t.Errorf("calls: %v", r.calls)
	}
}

const twoUsers = `[
 {"id":"alice","enabled":true,"listenbrainz_token":"t1"},
 {"id":"off","enabled":false,"listenbrainz_token":"t"},
 {"id":"bob","enabled":true,"listenbrainz_token":"t2"}]`

func TestRunAllUsers_RunsEnabledInFileOrderSkipsDisabled(t *testing.T) {
	r := &recorder{}
	if err := runAllUsers(writeUsers(t, twoUsers), "p", []string{"-p", "x", "--user", "all"}, r.run); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"-p", "x", "--user", "alice"}, {"-p", "x", "--user", "bob"}}
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("got %v want %v", r.calls, want)
	}
}

func TestRunAllUsers_OneFailureContinuesAndReports(t *testing.T) {
	r := &recorder{fail: map[string]bool{"alice": true}}
	err := runAllUsers(writeUsers(t, twoUsers), "p", []string{"--user", "all"}, r.run)
	var ure *UserRunError
	if !errors.As(err, &ure) {
		t.Fatalf("want *UserRunError, got %v", err)
	}
	if !reflect.DeepEqual(ure.Failed, []string{"alice"}) || !strings.Contains(err.Error(), "alice") {
		t.Errorf("failed: %v / %v", ure.Failed, err)
	}
	if len(r.calls) != 2 {
		t.Errorf("second user must still run: %v", r.calls)
	}
}

func TestRunAllUsers_FailedIDsInFileOrder(t *testing.T) {
	r := &recorder{fail: map[string]bool{"alice": true, "bob": true}}
	err := runAllUsers(writeUsers(t, twoUsers), "p", nil, r.run)
	var ure *UserRunError
	if !errors.As(err, &ure) || !reflect.DeepEqual(ure.Failed, []string{"alice", "bob"}) {
		t.Errorf("got %v", err)
	}
}

func TestRunAllUsers_CorruptFileRunsNothing(t *testing.T) {
	r := &recorder{}
	err := runAllUsers(writeUsers(t, "{not json"), "p", nil, r.run)
	if err == nil || len(r.calls) != 0 {
		t.Errorf("err=%v calls=%v", err, r.calls)
	}
}

func TestRunAllUsers_UnreadableFileRunsNothing(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "users.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	r := &recorder{}
	if err := runAllUsers(dir, "p", nil, r.run); err == nil || len(r.calls) != 0 {
		t.Errorf("err=%v calls=%v", err, r.calls)
	}
}

func TestRunAllUsers_AllDisabledRunsNothing(t *testing.T) {
	r := &recorder{}
	err := runAllUsers(writeUsers(t, `[{"id":"a","enabled":false,"listenbrainz_token":"t"}]`), "p", nil, r.run)
	if err != nil || len(r.calls) != 0 {
		t.Errorf("err=%v calls=%v", err, r.calls)
	}
}

func TestRunAllUsers_TokenlessUserSkippedWithoutFailing(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(old)

	r := &recorder{}
	dir := writeUsers(t, `[{"id":"a","enabled":true},{"id":"b","enabled":true,"listenbrainz_token":"t"}]`)
	if err := runAllUsers(dir, "p", nil, r.run); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.calls, [][]string{{"--user", "b"}}) {
		t.Errorf("calls: %v", r.calls)
	}
	if !strings.Contains(buf.String(), "level=WARN") || !strings.Contains(buf.String(), "user=a") {
		t.Errorf("no warning: %s", buf.String())
	}
}

func TestRunAllUsers_CustomPlaylistRunsOwnerOnly(t *testing.T) {
	old := ownerLookup
	ownerLookup = func(_, _ string, _ []users.User) string { return "bob" }
	defer func() { ownerLookup = old }()
	r := &recorder{}
	if err := runAllUsers(writeUsers(t, twoUsers), "custom-1", []string{"--user", "all"}, r.run); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.calls, [][]string{{"--user", "bob"}}) {
		t.Errorf("calls: %v", r.calls)
	}
}

func TestOwnerLookupDefaultUsesPlaylistOwnership(t *testing.T) {
	dir := writeUsers(t, twoUsers)
	raw := `[{"id":"custom-1","user_id":"bob"},{"id":"custom-2"}]`
	if err := os.WriteFile(filepath.Join(dir, "custom-playlists.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ownerLookup(dir, "custom-1", nil); got != "bob" {
		t.Errorf("explicit owner: got %q", got)
	}
	if got := ownerLookup(dir, "custom-2", nil); got != "alice" {
		t.Errorf("legacy owner: got %q", got)
	}
}

func TestRunAllUsers_ReservedAllUserNeverSpawned(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(old)

	r := &recorder{}
	dir := writeUsers(t, `[{"id":"all","enabled":true,"listenbrainz_token":"t"},{"id":"b","enabled":true,"listenbrainz_token":"t"}]`)
	err := runAllUsers(dir, "p", []string{"--user", "all"}, r.run)
	if !reflect.DeepEqual(r.calls, [][]string{{"--user", "b"}}) {
		t.Errorf("calls: %v", r.calls)
	}
	var ue *UserRunError
	if !errors.As(err, &ue) || !reflect.DeepEqual(ue.Failed, []string{"all"}) {
		t.Errorf("err = %v, want UserRunError{all}", err)
	}
	if !strings.Contains(buf.String(), "reserved") {
		t.Errorf("no reserved log: %s", buf.String())
	}
}

func TestRunAllUsers_SingleUserFallbackErrorNotMultiUser(t *testing.T) {
	err := runAllUsers(writeUsers(t, ""), "p", nil, func([]string) error { return errors.New("boom") })
	if err == nil || strings.Contains(err.Error(), "multi-user") || !strings.Contains(err.Error(), "single-user") {
		t.Errorf("err = %v", err)
	}
}

func TestCacheUserID(t *testing.T) {
	if got := cacheUserID("custom-123", "alice"); got != "" {
		t.Errorf("custom: %q", got)
	}
	if got := cacheUserID("weekly-exploration", "alice"); got != "alice" {
		t.Errorf("builtin: %q", got)
	}
}
