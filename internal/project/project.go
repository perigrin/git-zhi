// ABOUTME: Cross-repo project aggregation: loads multiple git repos, computes
// ABOUTME: cross-repo critical chain, CCPM buffers, and per-worker next-issue recommendation.
package project

import (
	"fmt"
	"math"
	"os"

	git "github.com/go-git/go-git/v5"
	"github.com/goccy/go-yaml"
	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/graph"
	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
	"github.com/perigrin/git-zhi/internal/storage"
	"github.com/perigrin/git-zhi/internal/telemetry"
)

// ProjectDef defines a cross-repo project with repos, optional feeding
// buffer relationships, and a set of workers with their repo assignments.
type ProjectDef struct {
	Name    string      `yaml:"name"`
	Repos   []RepoDef   `yaml:"repos"`
	Workers []WorkerDef `yaml:"workers"`
}

// RepoDef names a repo path, its active milestone, and optional feeding
// buffers from other repos that must complete before this repo can proceed.
type RepoDef struct {
	Path      string    `yaml:"path"`
	Milestone string    `yaml:"milestone"`
	Feeds     []FeedDef `yaml:"feeds,omitempty"`
}

// FeedDef declares that another repo feeds into this repo with an explicit
// buffer duration (e.g. "3d" = 3 days).
type FeedDef struct {
	Repo   string `yaml:"repo"`
	Buffer string `yaml:"buffer"` // e.g. "3d"
}

// WorkerDef describes a worker and the repos they work across.
type WorkerDef struct {
	Name  string   `yaml:"name"`
	Repos []string `yaml:"repos"`
}

// ProjectStatus is the aggregated status of a cross-repo project.
type ProjectStatus struct {
	Name            string          `json:"name"`
	Repos           []RepoStatus    `json:"repos"`
	CriticalChain   []string        `json:"critical_chain"`    // repo names in critical path order
	ProjectBuffer   BufferStatus    `json:"project_buffer"`
	FeedingBuffers  []FeedingBuffer `json:"feeding_buffers"`
	ResourceBuffers []ResourceBuffer `json:"resource_buffers"`
}

// RepoStatus is the status of a single repo within the project.
type RepoStatus struct {
	Name        string  `json:"name"`
	Path        string  `json:"path"`
	Milestone   string  `json:"milestone"`
	Progress    float64 `json:"progress"`    // 0-1
	FeverChart  string  `json:"fever_chart"` // GREEN/YELLOW/RED
	Speed       float64 `json:"speed"`
	IssuesDone  int     `json:"issues_done"`
	IssuesTotal int     `json:"issues_total"`
}

// BufferStatus describes the current consumption of a CCPM buffer.
type BufferStatus struct {
	SizeDays    float64 `json:"size_days"`
	ConsumedDays float64 `json:"consumed_days"`
	ConsumedPct  float64 `json:"consumed_pct"`
	Status      string  `json:"status"` // GREEN/YELLOW/RED
}

// FeedingBuffer records the buffer at a cross-repo feeding junction.
type FeedingBuffer struct {
	FromRepo string       `json:"from_repo"`
	ToRepo   string       `json:"to_repo"`
	Buffer   BufferStatus `json:"buffer"`
}

// ResourceBuffer is the binary signal at a shared-worker handoff point.
type ResourceBuffer struct {
	Worker string `json:"worker"`
	Status string `json:"status"` // "on track" / "at risk"
	Detail string `json:"detail"`
}

// LoadProject parses a YAML project definition file and returns a ProjectDef.
func LoadProject(path string) (*ProjectDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read project file %q: %w", path, err)
	}
	var def ProjectDef
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse project file %q: %w", path, err)
	}
	return &def, nil
}

// repoState holds the loaded state for a single repo within the project.
type repoState struct {
	def    RepoDef
	issues []*issue.Issue
	ms     *milestone.Milestone
	stats  *telemetry.Stats
}

// openRepoState opens a repo at the given path and loads its issue/milestone state.
func openRepoState(rd RepoDef) (*repoState, error) {
	repo, err := git.PlainOpenWithOptions(rd.Path, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open repo at %q: %w", rd.Path, err)
	}
	store, err := storage.NewStore(repo)
	if err != nil {
		return nil, fmt.Errorf("create store for %q: %w", rd.Path, err)
	}
	allIssues, err := issue.LoadAllIssues(store)
	if err != nil {
		return nil, fmt.Errorf("load issues for %q: %w", rd.Path, err)
	}
	// Filter to the milestone.
	var msIssues []*issue.Issue
	for _, iss := range allIssues {
		if iss.Milestone == rd.Milestone {
			msIssues = append(msIssues, iss)
		}
	}

	ms, err := milestone.LoadMilestone(store, rd.Milestone)
	if err != nil {
		return nil, fmt.Errorf("load milestone %q in %q: %w", rd.Milestone, rd.Path, err)
	}

	stats := telemetry.Compute(allIssues, ms)

	return &repoState{
		def:    rd,
		issues: msIssues,
		ms:     ms,
		stats:  stats,
	}, nil
}

// BufferStatusFromValues computes the CCPM buffer status string from consumed
// and size values. GREEN < 1/3 consumed, YELLOW 1/3-2/3, RED > 2/3.
// Exported for use in tests and the cmd layer.
func BufferStatusFromValues(consumed, size float64) string {
	if size <= 0 {
		return "GREEN"
	}
	pct := consumed / size
	switch {
	case pct < 1.0/3.0:
		return "GREEN"
	case pct <= 2.0/3.0:
		return "YELLOW"
	default:
		return "RED"
	}
}

// criticalChainLength returns the length of the critical chain for a set of
// issues (number of issues on the longest sequential dependency path).
func criticalChainLength(issues []*issue.Issue) int {
	g := graph.New(issues)
	chain := g.CriticalChain()
	return len(chain)
}

// ComputeProjectStatus loads all repos defined in the project and computes
// the aggregated project status including per-repo telemetry, CCPM project
// buffer, feeding buffers, and resource buffers.
func ComputeProjectStatus(def *ProjectDef) (*ProjectStatus, error) {
	// Open and load all repos.
	states := make([]*repoState, 0, len(def.Repos))
	for _, rd := range def.Repos {
		rs, err := openRepoState(rd)
		if err != nil {
			return nil, err
		}
		states = append(states, rs)
	}

	// Build per-repo status entries.
	repoStatuses := make([]RepoStatus, 0, len(states))
	for _, rs := range states {
		total := len(rs.issues)
		done := 0
		for _, iss := range rs.issues {
			if iss.State == issue.StateDone {
				done++
			}
		}
		progress := 0.0
		if total > 0 {
			progress = float64(done) / float64(total)
		}

		feverChart := string(rs.stats.FeverStatus)

		repoStatuses = append(repoStatuses, RepoStatus{
			Name:        rs.def.Path, // use path as name when no display name field exists
			Path:        rs.def.Path,
			Milestone:   rs.def.Milestone,
			Progress:    progress,
			FeverChart:  feverChart,
			Speed:       rs.stats.Speed,
			IssuesDone:  done,
			IssuesTotal: total,
		})
	}

	// Compute cross-repo critical chain: find the repo with the longest
	// critical chain (most sequential work remaining).
	criticalChainOrder := computeCriticalChainOrder(states)

	// Compute project buffer: 50% of the largest critical chain length across
	// repos. This is a unit-less count of issues; callers treat it as "days"
	// at 1 issue/day for display purposes.
	maxChainLen := 0
	for _, rs := range states {
		if l := criticalChainLength(rs.issues); l > maxChainLen {
			maxChainLen = l
		}
	}
	projectBufferSize := math.Round(float64(maxChainLen) * 0.5)
	if projectBufferSize < 1 && maxChainLen > 0 {
		projectBufferSize = 1
	}
	// ConsumedDays: sum of buffer burned across all repos' telemetry, scaled.
	totalBurned := 0.0
	for _, rs := range states {
		totalBurned += rs.stats.BufferBurned
	}
	projectBufferConsumedPct := 0.0
	if projectBufferSize > 0 {
		projectBufferConsumedPct = totalBurned / projectBufferSize
	}
	projectBuffer := BufferStatus{
		SizeDays:    projectBufferSize,
		ConsumedDays: totalBurned,
		ConsumedPct:  projectBufferConsumedPct,
		Status:      BufferStatusFromValues(totalBurned, projectBufferSize),
	}

	// Compute feeding buffers.
	feedingBuffers := computeFeedingBuffers(def, states)

	// Compute resource buffers.
	resourceBuffers := computeResourceBuffers(def, states)

	return &ProjectStatus{
		Name:            def.Name,
		Repos:           repoStatuses,
		CriticalChain:   criticalChainOrder,
		ProjectBuffer:   projectBuffer,
		FeedingBuffers:  feedingBuffers,
		ResourceBuffers: resourceBuffers,
	}, nil
}

// computeCriticalChainOrder returns repo paths ordered by critical chain
// length (longest chain first — that repo is on the critical path).
func computeCriticalChainOrder(states []*repoState) []string {
	type entry struct {
		path  string
		chain int
	}
	entries := make([]entry, 0, len(states))
	for _, rs := range states {
		entries = append(entries, entry{
			path:  rs.def.Path,
			chain: criticalChainLength(rs.issues),
		})
	}
	// Sort descending by chain length.
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].chain > entries[i].chain {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
	result := make([]string, len(entries))
	for i, e := range entries {
		result[i] = e.path
	}
	return result
}

// computeFeedingBuffers builds feeding buffer entries for each cross-repo
// dependency declared in the project definition.
func computeFeedingBuffers(def *ProjectDef, states []*repoState) []FeedingBuffer {
	// Build a map of path -> repoState for lookup.
	stateByPath := make(map[string]*repoState, len(states))
	for _, rs := range states {
		stateByPath[rs.def.Path] = rs
	}

	var buffers []FeedingBuffer
	for _, rd := range def.Repos {
		for _, feed := range rd.Feeds {
			feedingState, ok := stateByPath[feed.Repo]
			if !ok {
				// Feeding repo not found in loaded states — skip silently.
				continue
			}
			// Buffer size is 50% of the feeding repo's critical chain.
			chainLen := criticalChainLength(feedingState.issues)
			bufferSize := math.Round(float64(chainLen) * 0.5)
			if bufferSize < 1 && chainLen > 0 {
				bufferSize = 1
			}
			consumed := feedingState.stats.BufferBurned
			consumedPct := 0.0
			if bufferSize > 0 {
				consumedPct = consumed / bufferSize
			}
			buffers = append(buffers, FeedingBuffer{
				FromRepo: feed.Repo,
				ToRepo:   rd.Path,
				Buffer: BufferStatus{
					SizeDays:    bufferSize,
					ConsumedDays: consumed,
					ConsumedPct:  consumedPct,
					Status:      BufferStatusFromValues(consumed, bufferSize),
				},
			})
		}
	}
	return buffers
}

// computeResourceBuffers determines whether shared workers are at risk by
// checking if they have an in-progress issue in any repo (signaling potential
// contention for their next repo assignment).
func computeResourceBuffers(def *ProjectDef, states []*repoState) []ResourceBuffer {
	// Build a map of path -> repoState for lookup.
	stateByPath := make(map[string]*repoState, len(states))
	for _, rs := range states {
		stateByPath[rs.def.Path] = rs
	}

	var buffers []ResourceBuffer
	for _, wd := range def.Workers {
		// A worker is "at risk" if they have an in-progress issue in any repo
		// that is not their last repo — meaning they are currently busy and may
		// not be available for subsequent repos in their assignment list.
		var busyRepo string
		for _, repoPath := range wd.Repos {
			rs, ok := stateByPath[repoPath]
			if !ok {
				continue
			}
			for _, iss := range rs.issues {
				if iss.State == issue.StateInProgress && iss.Assigned == wd.Name {
					busyRepo = repoPath
					break
				}
			}
			if busyRepo != "" {
				break
			}
		}

		if busyRepo != "" && len(wd.Repos) > 1 {
			buffers = append(buffers, ResourceBuffer{
				Worker: wd.Name,
				Status: "at risk",
				Detail: fmt.Sprintf("worker %q has in-progress work in %q", wd.Name, busyRepo),
			})
		} else {
			buffers = append(buffers, ResourceBuffer{
				Worker: wd.Name,
				Status: "on track",
				Detail: "",
			})
		}
	}
	return buffers
}

// NextForActor returns the repo path and issue ID of the most actionable issue
// for the named actor across the repos they are assigned to in the project.
// Returns an error if the actor is not found in the workers list or if no
// actionable issue exists.
func NextForActor(def *ProjectDef, actor string) (repoPath string, issueID uuid.UUID, err error) {
	// Find the worker definition for this actor.
	var workerRepos []string
	found := false
	for _, wd := range def.Workers {
		if wd.Name == actor {
			workerRepos = wd.Repos
			found = true
			break
		}
	}
	if !found {
		return "", uuid.UUID{}, fmt.Errorf("actor %q not found in project workers", actor)
	}

	// Try each repo in the worker's assignment list in order.
	for _, repoPath := range workerRepos {
		// Find the RepoDef for this path.
		var rd *RepoDef
		for i := range def.Repos {
			if def.Repos[i].Path == repoPath {
				rd = &def.Repos[i]
				break
			}
		}
		if rd == nil {
			continue
		}

		rs, err := openRepoState(*rd)
		if err != nil {
			continue
		}

		g := graph.New(rs.issues)
		iss, err := g.Head(actor)
		if err != nil {
			// No actionable issue in this repo — try the next.
			continue
		}
		return repoPath, iss.ID, nil
	}

	return "", uuid.UUID{}, fmt.Errorf("no actionable issues for actor %q across assigned repos", actor)
}
