// ABOUTME: Tests for the Jira Cloud REST API v3 client.
// ABOUTME: Uses httptest.NewServer to exercise real HTTP logic without live credentials.
package client_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/perigrin/git-zhi/internal/jira/client"
)

// jiraHandler returns an http.HandlerFunc that routes to stub Jira endpoints.
func jiraHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		// Require Basic auth on every call.
		_, _, ok := r.BasicAuth()
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Unauthorized"})
			return
		}

		path := r.URL.Path

		switch {
		// GET /rest/api/3/issue/{key}
		case strings.HasPrefix(path, "/rest/api/3/issue/") && !strings.Contains(path, "/transitions") && r.Method == http.MethodGet:
			key := strings.TrimPrefix(path, "/rest/api/3/issue/")
			if key == "NOTFOUND-1" {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "Issue does not exist"})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(issueFixture(key))

		// GET /rest/api/3/issue/{key}/transitions
		case strings.HasSuffix(path, "/transitions") && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"transitions": []map[string]string{
					{"id": "11", "name": "To Do"},
					{"id": "21", "name": "In Progress"},
					{"id": "31", "name": "Done"},
				},
			})

		// POST /rest/api/3/issue/{key}/transitions
		case strings.HasSuffix(path, "/transitions") && r.Method == http.MethodPost:
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			transition, ok := body["transition"].(map[string]interface{})
			if !ok || transition["id"] == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		// POST /rest/api/3/search (JQL search with pagination)
		case path == "/rest/api/3/search" && r.Method == http.MethodPost:
			var req map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			startAt := int(0)
			if v, ok := req["startAt"].(float64); ok {
				startAt = int(v)
			}
			maxResults := 50
			if v, ok := req["maxResults"].(float64); ok {
				maxResults = int(v)
			}

			// Return 3 issues across two pages of 2.
			allIssues := []interface{}{
				issueFixture("PROJ-1"),
				issueFixture("PROJ-2"),
				issueFixture("PROJ-3"),
			}
			total := len(allIssues)
			end := startAt + maxResults
			if end > total {
				end = total
			}
			page := allIssues[startAt:end]

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"startAt":    startAt,
				"maxResults": maxResults,
				"total":      total,
				"issues":     page,
			})

		// PUT /rest/api/3/issue/{key}
		case strings.HasPrefix(path, "/rest/api/3/issue/") && r.Method == http.MethodPut:
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// issueFixture returns a Jira-shaped issue JSON object for a given key.
func issueFixture(key string) map[string]interface{} {
	return map[string]interface{}{
		"key": key,
		"fields": map[string]interface{}{
			"summary": "Extract config service",
			"description": map[string]interface{}{
				"content": []interface{}{
					map[string]interface{}{
						"content": []interface{}{
							map[string]interface{}{
								"text": "Description text",
							},
						},
					},
				},
			},
			"status":   map[string]string{"name": "In Progress"},
			"priority": map[string]string{"name": "High"},
			"assignee": map[string]string{"displayName": "Chris Prather"},
			"labels":   []string{"backend"},
			"created":  "2026-03-01T10:00:00.000+0000",
			"updated":  "2026-03-15T14:30:00.000+0000",
		},
	}
}

// serverWithBadAuth returns a test server that always rejects auth.
func serverWithBadAuth() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Unauthorized"})
	}))
}

func TestGetIssue(t *testing.T) {
	srv := httptest.NewServer(jiraHandler(t))
	defer srv.Close()

	c := client.NewClient(srv.URL, "user@example.com", "token123")
	issue, err := c.GetIssue("LOPS-142")
	if err != nil {
		t.Fatalf("GetIssue returned error: %v", err)
	}

	if issue.Key != "LOPS-142" {
		t.Errorf("Key = %q, want %q", issue.Key, "LOPS-142")
	}
	if issue.Summary != "Extract config service" {
		t.Errorf("Summary = %q, want %q", issue.Summary, "Extract config service")
	}
	if issue.Description != "Description text" {
		t.Errorf("Description = %q, want %q", issue.Description, "Description text")
	}
	if issue.Status != "In Progress" {
		t.Errorf("Status = %q, want %q", issue.Status, "In Progress")
	}
	if issue.Priority != "High" {
		t.Errorf("Priority = %q, want %q", issue.Priority, "High")
	}
	if issue.Assignee != "Chris Prather" {
		t.Errorf("Assignee = %q, want %q", issue.Assignee, "Chris Prather")
	}
	if len(issue.Labels) != 1 || issue.Labels[0] != "backend" {
		t.Errorf("Labels = %v, want [backend]", issue.Labels)
	}

	wantCreated := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	if !issue.Created.Equal(wantCreated) {
		t.Errorf("Created = %v, want %v", issue.Created, wantCreated)
	}
	wantUpdated := time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC)
	if !issue.Updated.Equal(wantUpdated) {
		t.Errorf("Updated = %v, want %v", issue.Updated, wantUpdated)
	}
}

func TestGetIssueNotFound(t *testing.T) {
	srv := httptest.NewServer(jiraHandler(t))
	defer srv.Close()

	c := client.NewClient(srv.URL, "user@example.com", "token123")
	_, err := c.GetIssue("NOTFOUND-1")
	if err == nil {
		t.Fatal("expected error for missing issue, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q should contain 'not found'", err.Error())
	}
}

func TestGetIssueAuthError(t *testing.T) {
	srv := serverWithBadAuth()
	defer srv.Close()

	c := client.NewClient(srv.URL, "bad@example.com", "badtoken")
	_, err := c.GetIssue("LOPS-1")
	if err == nil {
		t.Fatal("expected auth error, got nil")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error %q should mention 401", err.Error())
	}
}

func TestSearchIssuesPagination(t *testing.T) {
	srv := httptest.NewServer(jiraHandler(t))
	defer srv.Close()

	// maxResults=2 so we exercise the two-page path (3 total issues).
	c := client.NewClient(srv.URL, "user@example.com", "token123")
	issues, err := c.SearchIssues("project = PROJ ORDER BY created DESC", 2)
	if err != nil {
		t.Fatalf("SearchIssues returned error: %v", err)
	}
	if len(issues) != 3 {
		t.Errorf("got %d issues, want 3", len(issues))
	}
	keys := make([]string, len(issues))
	for i, iss := range issues {
		keys[i] = iss.Key
	}
	want := []string{"PROJ-1", "PROJ-2", "PROJ-3"}
	for i, k := range want {
		if keys[i] != k {
			t.Errorf("issue[%d].Key = %q, want %q", i, keys[i], k)
		}
	}
}

func TestGetTransitions(t *testing.T) {
	srv := httptest.NewServer(jiraHandler(t))
	defer srv.Close()

	c := client.NewClient(srv.URL, "user@example.com", "token123")
	transitions, err := c.GetTransitions("LOPS-142")
	if err != nil {
		t.Fatalf("GetTransitions returned error: %v", err)
	}
	if len(transitions) != 3 {
		t.Fatalf("got %d transitions, want 3", len(transitions))
	}
	if transitions[2].ID != "31" || transitions[2].Name != "Done" {
		t.Errorf("transitions[2] = {%s %s}, want {31 Done}", transitions[2].ID, transitions[2].Name)
	}
}

func TestDoTransition(t *testing.T) {
	srv := httptest.NewServer(jiraHandler(t))
	defer srv.Close()

	c := client.NewClient(srv.URL, "user@example.com", "token123")
	err := c.DoTransition("LOPS-142", "31")
	if err != nil {
		t.Fatalf("DoTransition returned error: %v", err)
	}
}

func TestUpdateIssue(t *testing.T) {
	srv := httptest.NewServer(jiraHandler(t))
	defer srv.Close()

	c := client.NewClient(srv.URL, "user@example.com", "token123")
	err := c.UpdateIssue("LOPS-142", map[string]interface{}{
		"summary": map[string]string{"set": "New summary"},
	})
	if err != nil {
		t.Fatalf("UpdateIssue returned error: %v", err)
	}
}
