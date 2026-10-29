package users

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Kelly W.", "kelly-w"},
		{"  ", ""},
		{"Ünï", "n"},
		{"A", "a"},
		{"a__b  c", "a-b-c"},
		{strings.Repeat("x", 40), strings.Repeat("x", 32)},
	}
	for _, c := range cases {
		if got := Slug(c.in); got != c.want {
			t.Errorf("Slug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func valid(id string) User {
	return User{ID: id, Name: "N", ListenBrainzUser: "lb", PlexUser: "plex"}
}

func TestValidate_RejectsBadIDs(t *testing.T) {
	for _, id := range []string{"../x", "A", "", strings.Repeat("a", 33), "-a"} {
		if err := Validate(valid(id), nil, false); err == nil {
			t.Errorf("id %q should be rejected", id)
		}
	}
	if err := Validate(valid(Slug("!!!")), nil, false); err == nil {
		t.Error("empty slug id should be rejected")
	}

	first := valid(Slug("Kelly"))
	if err := Validate(first, nil, false); err != nil {
		t.Fatalf("first create: %v", err)
	}
	dup := valid(Slug("kelly!"))
	if err := Validate(dup, []User{first}, false); err == nil {
		t.Error("duplicate id on create should be rejected")
	}
	if err := Validate(dup, []User{first}, true); err != nil {
		t.Errorf("same id when editing should pass: %v", err)
	}
}

func TestValidate_RequiredFields(t *testing.T) {
	for _, mut := range []func(*User){
		func(u *User) { u.Name = "" },
		func(u *User) { u.ListenBrainzUser = "" },
		func(u *User) { u.PlexUser = "" },
	} {
		u := valid("ok")
		mut(&u)
		if err := Validate(u, nil, false); err == nil {
			t.Errorf("expected error for %+v", u)
		}
	}
}

func TestValidate_TokenOptional(t *testing.T) {
	u := valid("ok")
	u.ListenBrainzToken = ""
	if err := Validate(u, nil, false); err != nil {
		t.Errorf("empty token should pass: %v", err)
	}
}

func TestEnabled(t *testing.T) {
	in := []User{{ID: "a", Enabled: true}, {ID: "b"}, {ID: "c", Enabled: true}}
	got := Enabled(in)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Errorf("Enabled = %+v", got)
	}
}

func TestStoreUpdate_RoundTripAnd0600(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	a := User{ID: "a", Name: "A", ListenBrainzUser: "la", PlexUser: "pa", Enabled: true}
	b := User{ID: "b", Name: "B", ListenBrainzUser: "lb", ListenBrainzToken: "tok", PlexUser: "pb"}
	if err := s.Update(func(us []User) ([]User, error) { return append(us, a), nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(us []User) ([]User, error) { return append(us, b), nil }); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []User{a, b}) {
		t.Errorf("Load = %+v", got)
	}
	fi, err := os.Stat(filepath.Join(dir, "users.json"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected only users.json, got %d entries", len(entries))
	}
}

func TestStoreUpdate_FnErrorSavesNothing(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	err := s.Update(func(us []User) ([]User, error) { return []User{{ID: "x"}}, fmt.Errorf("boom") })
	if err == nil {
		t.Fatal("expected error")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "users.json")); !os.IsNotExist(statErr) {
		t.Error("file should not exist")
	}
}

func TestStoreUpdate_ConcurrentUpdatesAllSurvive(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	const n = 25
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("u%d", i)
			if err := s.Update(func(us []User) ([]User, error) {
				return append(us, User{ID: id}), nil
			}); err != nil {
				t.Errorf("update %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != n {
		t.Fatalf("got %d users, want %d", len(got), n)
	}
	seen := map[string]bool{}
	for _, u := range got {
		seen[u.ID] = true
	}
	if len(seen) != n {
		t.Errorf("duplicate/missing ids: %d unique", len(seen))
	}
}

func TestLoad_MissingIsEmptyCorruptIsError(t *testing.T) {
	dir := t.TempDir()
	got, err := Load(dir)
	if err != nil || got != nil {
		t.Errorf("missing: got (%v, %v), want (nil, nil)", got, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "users.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Error("corrupt file should error")
	}
}

func TestValidate_ReservedIDAll(t *testing.T) {
	for _, id := range []string{"all", Slug("All"), Slug("all!"), Slug("ALL")} {
		u := User{ID: id, Name: "All", ListenBrainzUser: "lb", PlexUser: "p"}
		err := Validate(u, nil, false)
		if err == nil || !strings.Contains(err.Error(), "reserved") {
			t.Errorf("id %q: err = %v, want reserved error", id, err)
		}
		if err := Validate(u, nil, true); err == nil {
			t.Errorf("id %q editing: want error", id)
		}
	}
}

func TestIsReservedID(t *testing.T) {
	if !IsReservedID("all") || !IsReservedID("ALL") || IsReservedID("alice") {
		t.Error("IsReservedID wrong")
	}
}
