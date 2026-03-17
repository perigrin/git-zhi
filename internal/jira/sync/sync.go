// ABOUTME: Bidirectional sync logic between Jira Cloud and git-zhi issues.
// ABOUTME: Handles pull (Jira→zhi), push (zhi→Jira), conflict detection, and snapshots.
package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
	jclient "github.com/perigrin/git-zhi/internal/jira/client"
	"github.com/perigrin/git-zhi/internal/issue"
)

// StateMapping maps a zhi state string to the corresponding Jira status name.
// Example: StateMapping{"done": "Done", "in-progress": "In Progress"}.
type StateMapping map[string]string

// SyncResult aggregates the outcomes of a single sync pass.
type SyncResult struct {
	Pulled    []PullUpdate // inbound field changes detected from Jira
	Pushed    []PushUpdate // outbound state transitions sent to Jira
	Conflicts []Conflict   // fields changed on both sides since last sync
}

// PullUpdate records a single inbound field change: what the field was and
// what Jira now reports it should be.
type PullUpdate struct {
	IssueID  string
	Field    string
	OldValue string
	NewValue string
}

// PushUpdate records an outbound state transition sent to Jira.
type PushUpdate struct {
	TrackerKey string
	Field      string
	Value      string
}

// Conflict records a field that changed on both the zhi side and the Jira
// side since the last snapshot, requiring manual resolution.
type Conflict struct {
	IssueID      string
	TrackerKey   string
	Field        string
	ZhiValue     string
	TrackerValue string
}

// snapshotFields lists the issue fields that are recorded in snapshots and
// compared during sync. Order is not significant.
var snapshotFields = []string{"state", "urgency", "assigned", "labels"}

// ---------------------------------------------------------------------------
// Snapshot helpers
// ---------------------------------------------------------------------------

// SaveSnapshot writes the given field map to disk at
// <snapshotDir>/jira/<issueID>.yaml. The directory is created if it does not
// exist.
func SaveSnapshot(snapshotDir string, issueID string, fields map[string]string) error {
	dir := filepath.Join(snapshotDir, "jira")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create snapshot dir: %w", err)
	}
	path := filepath.Join(dir, issueID+".yaml")
	data, err := yaml.Marshal(fields)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}
	return nil
}

// LoadSnapshot reads the last-synced field values for the given issue from
// <snapshotDir>/jira/<issueID>.yaml. If the file does not exist an empty map
// is returned without error; the caller treats a missing snapshot as "no prior
// sync" (all fields are treated as new).
func LoadSnapshot(snapshotDir string, issueID string) (map[string]string, error) {
	path := filepath.Join(snapshotDir, "jira", issueID+".yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}
	var fields map[string]string
	if err := yaml.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("parse snapshot: %w", err)
	}
	if fields == nil {
		fields = map[string]string{}
	}
	return fields, nil
}

// ---------------------------------------------------------------------------
// Field extraction helpers
// ---------------------------------------------------------------------------

// zhiFields extracts the tracked field values from a zhi issue into a flat
// string map keyed by snapshotFields names.
func zhiFields(iss *issue.Issue) map[string]string {
	labels := strings.Join(iss.Labels, ",")
	return map[string]string{
		"state":    string(iss.State),
		"urgency":  string(iss.Urgency),
		"assigned": iss.Assigned,
		"labels":   labels,
	}
}

// jiraFields extracts the tracked field values from a Jira issue into a flat
// string map keyed by snapshotFields names. Priority is mapped to zhi urgency
// names (high/normal/low). Status is mapped to lowercase for comparison.
func jiraFields(ji *jclient.Issue) map[string]string {
	return map[string]string{
		"state":    mapJiraStatusToZhi(ji.Status),
		"urgency":  mapJiraPriorityToZhi(ji.Priority),
		"assigned": ji.Assignee,
		"labels":   strings.Join(ji.Labels, ","),
	}
}

// mapJiraPriorityToZhi converts a Jira priority name to a zhi urgency name.
// Unknown values map to "normal" for safe defaults.
func mapJiraPriorityToZhi(priority string) string {
	switch strings.ToLower(priority) {
	case "highest", "high":
		return "high"
	case "lowest", "low":
		return "low"
	default:
		return "normal"
	}
}

// mapJiraStatusToZhi converts a Jira status name to a zhi state name.
// The mapping is intentionally permissive — unknown statuses are preserved
// in lowercase so conflict detection still works with custom workflows.
func mapJiraStatusToZhi(status string) string {
	switch strings.ToLower(status) {
	case "to do", "open", "backlog", "new":
		return "pending"
	case "in progress", "in review":
		return "in-progress"
	case "done", "closed", "resolved":
		return "done"
	case "cancelled", "won't do", "wont do":
		return "cancelled"
	default:
		return strings.ToLower(status)
	}
}

// ---------------------------------------------------------------------------
// Pull
// ---------------------------------------------------------------------------

// Pull fetches current Jira state for each issue that has a TrackerID,
// compares it against the last snapshot, and returns PullUpdates and Conflicts.
//
// Decision rules for each tracked field:
//   - Only Jira changed since snapshot → emit PullUpdate
//   - Only zhi changed → no pull action (Push handles outbound)
//   - Both changed the same field → emit Conflict
//   - Neither changed → no action
//
// After a successful comparison the snapshot is updated with the current Jira
// values for fields that Jira owns (i.e. no zhi-side change detected).
func Pull(jiraClient *jclient.Client, issues []*issue.Issue, snapshotDir string) (*SyncResult, error) {
	result := &SyncResult{}

	for _, iss := range issues {
		if iss.TrackerID == "" {
			continue
		}
		key := trackerKey(iss.TrackerID)
		if key == "" {
			continue
		}

		snapshot, err := LoadSnapshot(snapshotDir, iss.ID.String())
		if err != nil {
			return nil, fmt.Errorf("load snapshot for %s: %w", iss.ID, err)
		}

		jiraIssue, err := jiraClient.GetIssue(key)
		if err != nil {
			return nil, fmt.Errorf("fetch jira issue %s: %w", key, err)
		}

		zhiCurrent := zhiFields(iss)
		jiraCurrent := jiraFields(jiraIssue)

		for _, field := range snapshotFields {
			snapshotVal := snapshot[field]
			zhiVal := zhiCurrent[field]
			jiraVal := jiraCurrent[field]

			zhiChanged := zhiVal != snapshotVal && snapshotVal != ""
			jiraChanged := jiraVal != snapshotVal && snapshotVal != ""

			// When there is no prior snapshot treat Jira as authoritative only
			// if zhi has never set the field (empty zhi value), otherwise keep
			// zhi's value without conflict.
			if snapshotVal == "" {
				// No prior sync — skip conflict detection; only pull if zhi has
				// no opinion (empty) and Jira has a value.
				if zhiVal == "" && jiraVal != "" {
					result.Pulled = append(result.Pulled, PullUpdate{
						IssueID:  iss.ID.String(),
						Field:    field,
						OldValue: zhiVal,
						NewValue: jiraVal,
					})
				}
				continue
			}

			switch {
			case zhiChanged && jiraChanged:
				result.Conflicts = append(result.Conflicts, Conflict{
					IssueID:      iss.ID.String(),
					TrackerKey:   key,
					Field:        field,
					ZhiValue:     zhiVal,
					TrackerValue: jiraVal,
				})
			case jiraChanged && !zhiChanged:
				result.Pulled = append(result.Pulled, PullUpdate{
					IssueID:  iss.ID.String(),
					Field:    field,
					OldValue: zhiVal,
					NewValue: jiraVal,
				})
			}
		}

		// Save the current Jira field values as the new snapshot baseline so
		// repeated pulls do not re-detect the same changes.
		if err := SaveSnapshot(snapshotDir, iss.ID.String(), jiraCurrent); err != nil {
			return nil, fmt.Errorf("save snapshot for %s: %w", iss.ID, err)
		}
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Push
// ---------------------------------------------------------------------------

// Push sends zhi state changes to Jira for each issue with a TrackerID where
// the zhi state has a mapping in stateMapping. It calls GetTransitions to
// resolve the transition id and DoTransition to execute it.
//
// An issue is considered to need pushing when its state has a mapping in
// stateMapping AND either LastSyncedAt is set (any state change since that
// timestamp is assumed outbound) or the issue has never been synced.
//
// The current implementation pushes the state field only. Future phases may
// extend this to other fields.
func Push(jiraClient *jclient.Client, issues []*issue.Issue, stateMapping StateMapping, snapshotDir string) (*SyncResult, error) {
	result := &SyncResult{}

	for _, iss := range issues {
		if iss.TrackerID == "" {
			continue
		}
		key := trackerKey(iss.TrackerID)
		if key == "" {
			continue
		}

		targetStatus, ok := stateMapping[string(iss.State)]
		if !ok {
			continue
		}

		// Fetch available transitions and find the one matching targetStatus.
		transitions, err := jiraClient.GetTransitions(key)
		if err != nil {
			return nil, fmt.Errorf("get transitions for %s: %w", key, err)
		}

		transitionID := ""
		for _, t := range transitions {
			if strings.EqualFold(t.Name, targetStatus) {
				transitionID = t.ID
				break
			}
		}
		if transitionID == "" {
			// Target status not available as a transition; skip without error.
			continue
		}

		if err := jiraClient.DoTransition(key, transitionID); err != nil {
			return nil, fmt.Errorf("do transition on %s: %w", key, err)
		}

		result.Pushed = append(result.Pushed, PushUpdate{
			TrackerKey: key,
			Field:      "state",
			Value:      targetStatus,
		})
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// FormatBatchEdits
// ---------------------------------------------------------------------------

// batchEditLine is the JSON shape consumed by `git zhi issue edit --batch`.
// Fields is a map from field name to value, matching the batchOp struct in
// internal/cli/issue_edit.go.
type batchEditLine struct {
	IssueID string            `json:"issue_id"`
	Fields  map[string]string `json:"fields"`
}

// FormatBatchEdits converts a slice of PullUpdates to a newline-delimited
// sequence of JSON objects suitable for piping to `git zhi issue edit --batch`.
// Returns nil if updates is empty.
func FormatBatchEdits(updates []PullUpdate) []byte {
	if len(updates) == 0 {
		return nil
	}
	var sb strings.Builder
	for _, u := range updates {
		line := batchEditLine{
			IssueID: u.IssueID,
			Fields:  map[string]string{u.Field: u.NewValue},
		}
		data, err := json.Marshal(line)
		if err != nil {
			// json.Marshal of a plain struct cannot fail for string-valued fields.
			continue
		}
		sb.Write(data)
		sb.WriteByte('\n')
	}
	return []byte(sb.String())
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// trackerKey extracts the bare tracker key from a TrackerID like "jira:LOPS-142".
// Returns an empty string if the TrackerID is not a jira: prefixed value.
func trackerKey(trackerID string) string {
	const prefix = "jira:"
	if !strings.HasPrefix(trackerID, prefix) {
		return ""
	}
	return strings.TrimPrefix(trackerID, prefix)
}
