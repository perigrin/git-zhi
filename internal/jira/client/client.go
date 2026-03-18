// ABOUTME: Jira Cloud REST API v3 client for reading and updating issues.
// ABOUTME: Supports GetIssue, SearchIssues (paginated), GetTransitions, DoTransition, UpdateIssue.
package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client holds the connection details for a single Jira Cloud instance.
type Client struct {
	BaseURL    string
	Token      string
	Email      string
	HTTPClient *http.Client
}

// Issue represents a single Jira issue with the fields relevant to git-zhi.
type Issue struct {
	Key         string    // e.g., "LOPS-142"
	Summary     string    // issue title
	Description string    // body text (plain text extracted from Atlassian Document Format)
	Status      string    // e.g., "In Progress", "Done"
	Priority    string    // e.g., "High", "Medium"
	Assignee    string    // display name of the assignee
	Labels      []string  // label strings
	Created     time.Time // creation timestamp
	Updated     time.Time // last-updated timestamp
}

// TransitionOption describes an available workflow transition for a Jira issue.
type TransitionOption struct {
	ID     string
	Name   string
	ToName string // target status name (from "to.name" in the Jira response)
}

// jiraTimeLayout is the timestamp format Jira Cloud returns for created/updated.
const jiraTimeLayout = "2006-01-02T15:04:05.000-0700"

// NewClient creates a Client configured to talk to baseURL with Basic auth using
// the supplied email and API token.
func NewClient(baseURL, email, token string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Email:      email,
		Token:      token,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// authHeader returns the Basic auth header value for this client.
func (c *Client) authHeader() string {
	raw := c.Email + ":" + c.Token
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}

// doRequest executes an HTTP request, attaching auth, and returns the response
// body. The caller is responsible for closing the body.
func (c *Client) doRequest(method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", c.authHeader())
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http %s %s: %w", method, url, err)
	}
	return resp, nil
}

// checkStatus reads a response and returns an error for non-2xx status codes.
func checkStatus(resp *http.Response, issueKey string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("jira auth failed (401): %s", string(body))
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("jira issue not found: %s (404)", issueKey)
	}
	return fmt.Errorf("jira request failed (%d): %s", resp.StatusCode, string(body))
}

// GetIssue fetches a single issue by its key (e.g., "LOPS-142").
func (c *Client) GetIssue(key string) (*Issue, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s", c.BaseURL, url.PathEscape(key))
	resp, err := c.doRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkStatus(resp, key); err != nil {
		return nil, err
	}

	var raw jiraIssueResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode issue %s: %w", key, err)
	}
	return raw.toIssue(), nil
}

// SearchIssues executes a JQL query and returns all matching issues, fetching
// additional pages automatically until the result set is exhausted.
// maxResults controls the page size for each individual request.
func (c *Client) SearchIssues(jql string, maxResults int) ([]Issue, error) {
	url := fmt.Sprintf("%s/rest/api/3/search", c.BaseURL)
	var all []Issue
	startAt := 0

	for {
		payload := map[string]interface{}{
			"jql":        jql,
			"startAt":    startAt,
			"maxResults": maxResults,
		}
		buf, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal search request: %w", err)
		}

		resp, err := c.doRequest(http.MethodPost, url, bytes.NewReader(buf))
		if err != nil {
			return nil, err
		}

		if err := checkStatus(resp, ""); err != nil {
			resp.Body.Close()
			return nil, err
		}

		var page jiraSearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode search response: %w", err)
		}
		resp.Body.Close()

		for _, raw := range page.Issues {
			all = append(all, *raw.toIssue())
		}

		startAt += len(page.Issues)
		if startAt >= page.Total || len(page.Issues) == 0 {
			break
		}
	}

	return all, nil
}

// GetTransitions returns the workflow transitions available for the given issue key.
func (c *Client) GetTransitions(key string) ([]TransitionOption, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s/transitions", c.BaseURL, url.PathEscape(key))
	resp, err := c.doRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := checkStatus(resp, key); err != nil {
		return nil, err
	}

	var raw struct {
		Transitions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			To   struct {
				Name string `json:"name"`
			} `json:"to"`
		} `json:"transitions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode transitions for %s: %w", key, err)
	}

	opts := make([]TransitionOption, len(raw.Transitions))
	for i, t := range raw.Transitions {
		opts[i] = TransitionOption{ID: t.ID, Name: t.Name, ToName: t.To.Name}
	}
	return opts, nil
}

// DoTransition triggers the named workflow transition on an issue.
func (c *Client) DoTransition(key string, transitionID string) error {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s/transitions", c.BaseURL, url.PathEscape(key))
	payload := map[string]interface{}{
		"transition": map[string]string{"id": transitionID},
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal transition payload: %w", err)
	}

	resp, err := c.doRequest(http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp, key)
}

// UpdateIssue sends a partial update for the given issue using the Jira
// update-fields syntax (e.g., {"summary": {"set": "new title"}}).
func (c *Client) UpdateIssue(key string, fields map[string]interface{}) error {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s", c.BaseURL, url.PathEscape(key))
	payload := map[string]interface{}{"update": fields}
	buf, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal update payload: %w", err)
	}

	resp, err := c.doRequest(http.MethodPut, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return checkStatus(resp, key)
}

// ---------------------------------------------------------------------------
// Internal JSON shapes — Jira API response types.
// ---------------------------------------------------------------------------

// jiraIssueResponse is the top-level shape returned by GET /rest/api/3/issue/{key}.
type jiraIssueResponse struct {
	Key    string      `json:"key"`
	Fields jiraFields  `json:"fields"`
}

// jiraFields holds the "fields" object from a Jira issue response.
type jiraFields struct {
	Summary     string              `json:"summary"`
	Description *jiraADF           `json:"description"`
	Status      jiraNamedObject     `json:"status"`
	Priority    jiraNamedObject     `json:"priority"`
	Assignee    *jiraNamedObject    `json:"assignee"`
	Labels      []string            `json:"labels"`
	Created     string              `json:"created"`
	Updated     string              `json:"updated"`
}

// jiraNamedObject is the common {"name": "..."} or {"displayName": "..."} wrapper.
type jiraNamedObject struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

// jiraADF is the Atlassian Document Format node used for rich-text fields.
// We only extract the first text leaf at doc→content[0]→content[0]→text.
type jiraADF struct {
	Content []jiraADFBlock `json:"content"`
}

// jiraADFBlock is a paragraph block inside an ADF document.
type jiraADFBlock struct {
	Content []jiraADFInline `json:"content"`
}

// jiraADFInline is a text run inside a paragraph block.
type jiraADFInline struct {
	Text string `json:"text"`
}

// jiraSearchResponse is the envelope returned by POST /rest/api/3/search.
type jiraSearchResponse struct {
	StartAt    int                  `json:"startAt"`
	MaxResults int                  `json:"maxResults"`
	Total      int                  `json:"total"`
	Issues     []jiraIssueResponse  `json:"issues"`
}

// toIssue converts the raw Jira API shape into a domain Issue.
func (r *jiraIssueResponse) toIssue() *Issue {
	iss := &Issue{
		Key:      r.Key,
		Summary:  r.Fields.Summary,
		Status:   r.Fields.Status.Name,
		Priority: r.Fields.Priority.Name,
		Labels:   r.Fields.Labels,
	}

	if r.Fields.Assignee != nil {
		name := r.Fields.Assignee.DisplayName
		if name == "" {
			name = r.Fields.Assignee.Name
		}
		iss.Assignee = name
	}

	if r.Fields.Description != nil {
		iss.Description = extractADFText(r.Fields.Description)
	}

	if t, err := time.Parse(jiraTimeLayout, r.Fields.Created); err != nil {
		log.Printf("warning: cannot parse Jira created timestamp %q for %s: %v", r.Fields.Created, r.Key, err)
	} else {
		iss.Created = t
	}
	if t, err := time.Parse(jiraTimeLayout, r.Fields.Updated); err != nil {
		log.Printf("warning: cannot parse Jira updated timestamp %q for %s: %v", r.Fields.Updated, r.Key, err)
	} else {
		iss.Updated = t
	}

	return iss
}

// extractADFText walks the top-level paragraph content of an ADF document and
// concatenates all text runs with newlines between paragraphs.
func extractADFText(adf *jiraADF) string {
	var parts []string
	for _, block := range adf.Content {
		var blockParts []string
		for _, inline := range block.Content {
			if inline.Text != "" {
				blockParts = append(blockParts, inline.Text)
			}
		}
		if len(blockParts) > 0 {
			parts = append(parts, strings.Join(blockParts, ""))
		}
	}
	return strings.Join(parts, "\n")
}
