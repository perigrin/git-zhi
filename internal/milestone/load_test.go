// ABOUTME: Tests for milestone load helpers: UnmarshalMilestone, LoadMilestone,
// ABOUTME: and LoadAllMilestones from storage refs.
package milestone_test

import (
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"

	"github.com/perigrin/git-zhi/internal/milestone"
	"github.com/perigrin/git-zhi/internal/storage"
)

func initMilestoneTestStore(t *testing.T) *storage.Store {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	store, err := storage.NewStore(repo)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func TestUnmarshalMilestone_RoundTrip(t *testing.T) {
	ms := &milestone.Milestone{
		Name:        "v0.1",
		Description: "Initial release",
		Created:     time.Now().Truncate(time.Second),
	}

	data, err := milestone.MarshalMilestone(ms)
	if err != nil {
		t.Fatalf("MarshalMilestone: %v", err)
	}

	got, err := milestone.UnmarshalMilestone(data)
	if err != nil {
		t.Fatalf("UnmarshalMilestone: %v", err)
	}

	if got.Name != ms.Name {
		t.Errorf("Name: got %q, want %q", got.Name, ms.Name)
	}
	if got.Description != ms.Description {
		t.Errorf("Description: got %q, want %q", got.Description, ms.Description)
	}
}

func TestUnmarshalMilestone_WithDue(t *testing.T) {
	due := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	ms := &milestone.Milestone{
		Name:    "v0.2",
		Due:     &due,
		Created: time.Now().Truncate(time.Second),
	}

	data, err := milestone.MarshalMilestone(ms)
	if err != nil {
		t.Fatalf("MarshalMilestone: %v", err)
	}

	got, err := milestone.UnmarshalMilestone(data)
	if err != nil {
		t.Fatalf("UnmarshalMilestone: %v", err)
	}

	if got.Due == nil {
		t.Fatal("expected Due to be set")
	}
	if !got.Due.Equal(due) {
		t.Errorf("Due: got %v, want %v", got.Due, due)
	}
}

func TestLoadMilestone(t *testing.T) {
	store := initMilestoneTestStore(t)

	ms := &milestone.Milestone{
		Name:    "v0.1",
		Created: time.Now().Truncate(time.Second),
	}
	data, err := milestone.MarshalMilestone(ms)
	if err != nil {
		t.Fatalf("MarshalMilestone: %v", err)
	}
	ref := milestone.RefPrefix + "v0.1"
	if err := store.WriteEntity(ref, "milestone.yaml", data, "create milestone"); err != nil {
		t.Fatalf("WriteEntity: %v", err)
	}

	got, err := milestone.LoadMilestone(store, "v0.1")
	if err != nil {
		t.Fatalf("LoadMilestone: %v", err)
	}
	if got.Name != "v0.1" {
		t.Errorf("Name: got %q, want %q", got.Name, "v0.1")
	}
}

func TestLoadMilestone_NotFound(t *testing.T) {
	store := initMilestoneTestStore(t)

	_, err := milestone.LoadMilestone(store, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent milestone, got nil")
	}
}

func TestLoadAllMilestones(t *testing.T) {
	store := initMilestoneTestStore(t)

	names := []string{"v0.1", "v0.2"}
	for _, name := range names {
		ms := &milestone.Milestone{
			Name:    name,
			Created: time.Now().Truncate(time.Second),
		}
		data, err := milestone.MarshalMilestone(ms)
		if err != nil {
			t.Fatalf("MarshalMilestone %s: %v", name, err)
		}
		ref := milestone.RefPrefix + name
		if err := store.WriteEntity(ref, "milestone.yaml", data, "create milestone"); err != nil {
			t.Fatalf("WriteEntity %s: %v", name, err)
		}
	}

	all, err := milestone.LoadAllMilestones(store)
	if err != nil {
		t.Fatalf("LoadAllMilestones: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 milestones, got %d", len(all))
	}
}

func TestValidateMilestoneName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errSub  string
	}{
		{"valid simple name", "v0.1", false, ""},
		{"valid hyphenated", "release-2026", false, ""},
		{"empty name", "", true, "must not be empty"},
		{"underscore reserved", "_", true, "collide"},
		{"contains slash", "foo/bar", true, "'/'"},
		{"path traversal", "..", true, "'..'"},
		{"embedded traversal", "foo..bar", true, "'..'"},
		{"traversal prefix", "../etc", true, "'/'"},
		{"contains space", "Controller Integration", true, "invalid git ref"},
		{"contains ampersand", "Controller&Integration", true, "invalid git ref"},
		{"contains colon", "foo:bar", true, "invalid git ref"},
		{"contains backslash", "foo\\bar", true, "invalid git ref"},
		{"contains tilde", "foo~1", true, "invalid git ref"},
		{"contains caret", "foo^bar", true, "invalid git ref"},
		{"contains question mark", "foo?bar", true, "invalid git ref"},
		{"contains asterisk", "foo*bar", true, "invalid git ref"},
		{"contains open bracket", "foo[bar", true, "invalid git ref"},
		{"starts with dot", ".hidden", true, "invalid git ref"},
		{"ends with dot", "foo.", true, "invalid git ref"},
		{"ends with .lock", "foo.lock", true, "invalid git ref"},
		{"contains control char", "foo\x01bar", true, "invalid git ref"},
		{"valid with dots", "v0.3.7", false, ""},
		{"valid with underscores", "my_milestone", false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := milestone.ValidateMilestoneName(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tc.input)
				}
				if tc.errSub != "" && !strings.Contains(err.Error(), tc.errSub) {
					t.Errorf("expected %q in error, got: %v", tc.errSub, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", tc.input, err)
				}
			}
		})
	}
}

func TestValidateMilestoneName_SuggestsAlternative(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantSuggest string
	}{
		{"space becomes hyphen", "Controller Integration", "Controller-Integration"},
		{"ampersand becomes hyphen", "Controller&Integration", "Controller-Integration"},
		{"mixed invalid chars collapse", "Controller & Integration", "Controller-Integration"},
		{"leading dot stripped", ".hidden", "hidden"},
		{"trailing dot stripped", "foo.", "foo"},
		{".lock suffix stripped", "foo.lock", "foo"},
		{"colon becomes hyphen", "foo:bar", "foo-bar"},
		{"multiple invalids collapse", "foo~~^bar", "foo-bar"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := milestone.ValidateMilestoneName(tc.input)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tc.input)
			}
			msg := err.Error()
			if !strings.Contains(msg, tc.wantSuggest) {
				t.Errorf("expected suggestion %q in error, got: %v", tc.wantSuggest, err)
			}
			if !strings.Contains(msg, "try") {
				t.Errorf("expected 'try' hint in error, got: %v", err)
			}
		})
	}
}

func TestLoadAllMilestones_Empty(t *testing.T) {
	store := initMilestoneTestStore(t)

	all, err := milestone.LoadAllMilestones(store)
	if err != nil {
		t.Fatalf("LoadAllMilestones on empty store: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("expected 0 milestones, got %d", len(all))
	}
}
