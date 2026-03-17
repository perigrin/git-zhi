// ABOUTME: Tests for VADER sentiment analysis on commit messages and issue trajectories.
// ABOUTME: Covers AnalyzeCommitSentiment, IssueTrajectory, DetectAnomalies, MilestoneSentiment, and Compute.
package sentiment_test

import (
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/gofrs/uuid/v5"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/sanbao/sentiment"
)

// makeIssue builds a minimal in-memory issue for tests.
func makeIssue(sessions []issue.Session) *issue.Issue {
	gen := uuid.NewGen()
	id, _ := gen.NewV7()
	return &issue.Issue{
		ID:       id,
		Title:    "test issue",
		State:    issue.StateDone,
		Milestone: "v0.2",
		Created:  time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC),
		Updated:  time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC),
		Sessions: sessions,
	}
}

// initRepoWithCommits creates a real on-disk git repo in a temp dir,
// makes commits with the given messages, and returns both the repo and
// the list of commit SHAs (oldest first).
func initRepoWithCommits(t *testing.T, messages []string) (*git.Repository, []string) {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	cfg, err := repo.Config()
	if err != nil {
		t.Fatalf("repo.Config: %v", err)
	}
	cfg.User.Name = "Test User"
	cfg.User.Email = "test@example.com"
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	var shas []string
	ts := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)

	for i, msg := range messages {
		sig := &object.Signature{Name: "Test User", Email: "test@example.com", When: ts.Add(time.Duration(i) * time.Minute)}
		h, err := wt.Commit(msg, &git.CommitOptions{
			Author:            sig,
			AllowEmptyCommits: true,
		})
		if err != nil {
			t.Fatalf("Commit[%d]: %v", i, err)
		}
		shas = append(shas, h.String())
	}
	return repo, shas
}

// TestAnalyzeCommitSentiment verifies that known messages produce expected polarity.
func TestAnalyzeCommitSentiment(t *testing.T) {
	t.Run("positive message scores above zero", func(t *testing.T) {
		score := sentiment.AnalyzeCommitSentiment("clean refactor of parser, working great now")
		if score <= 0 {
			t.Errorf("expected positive score, got %f", score)
		}
	})

	t.Run("negative message scores below zero", func(t *testing.T) {
		score := sentiment.AnalyzeCommitSentiment("ugh horrible hack broken again")
		if score >= 0 {
			t.Errorf("expected negative score, got %f", score)
		}
	})

	t.Run("neutral message scores near zero", func(t *testing.T) {
		score := sentiment.AnalyzeCommitSentiment("add test for lexer")
		// neutral messages should be in [-0.3, 0.3] range
		if score < -0.3 || score > 0.3 {
			t.Errorf("expected near-zero score for neutral message, got %f", score)
		}
	})

	t.Run("score is bounded to [-1.0, 1.0]", func(t *testing.T) {
		score := sentiment.AnalyzeCommitSentiment("excellent perfect wonderful great clean beautiful awesome")
		if score < -1.0 || score > 1.0 {
			t.Errorf("score %f is outside [-1.0, 1.0]", score)
		}
	})
}

// TestIssueTrajectory verifies commit message scoring across session SHA ranges.
func TestIssueTrajectory(t *testing.T) {
	t.Run("single session with three commits returns three scores", func(t *testing.T) {
		repo, shas := initRepoWithCommits(t, []string{
			"initial commit",
			"clean refactor of parser",
			"add test for lexer",
		})
		// Session covers commits 1 and 2 (index 1 and 2 inclusive, starting after 0).
		iss := makeIssue([]issue.Session{
			{StartSHA: shas[0], EndSHA: shas[2]},
		})
		traj := sentiment.IssueTrajectory(repo, iss)
		// Should have 2 commits in range (shas[1] and shas[2]).
		if len(traj) != 2 {
			t.Errorf("expected 2 scores in trajectory, got %d", len(traj))
		}
		for _, score := range traj {
			if score < -1.0 || score > 1.0 {
				t.Errorf("trajectory score %f out of bounds", score)
			}
		}
	})

	t.Run("empty sessions returns empty trajectory", func(t *testing.T) {
		repo, _ := initRepoWithCommits(t, []string{"initial"})
		iss := makeIssue([]issue.Session{})
		traj := sentiment.IssueTrajectory(repo, iss)
		if len(traj) != 0 {
			t.Errorf("expected empty trajectory, got %d scores", len(traj))
		}
	})

	t.Run("session with no EndSHA returns empty trajectory", func(t *testing.T) {
		repo, shas := initRepoWithCommits(t, []string{"initial", "second"})
		iss := makeIssue([]issue.Session{
			{StartSHA: shas[0], EndSHA: ""},
		})
		traj := sentiment.IssueTrajectory(repo, iss)
		if len(traj) != 0 {
			t.Errorf("expected empty trajectory for open session, got %d scores", len(traj))
		}
	})

	t.Run("equal StartSHA and EndSHA returns empty trajectory", func(t *testing.T) {
		repo, shas := initRepoWithCommits(t, []string{"initial"})
		iss := makeIssue([]issue.Session{
			{StartSHA: shas[0], EndSHA: shas[0]},
		})
		traj := sentiment.IssueTrajectory(repo, iss)
		if len(traj) != 0 {
			t.Errorf("expected empty trajectory for equal SHA session, got %d scores", len(traj))
		}
	})
}

// TestDetectAnomalies verifies anomaly detection for sustained negative sentiment.
func TestDetectAnomalies(t *testing.T) {
	t.Run("three consecutive negative commits flags anomaly", func(t *testing.T) {
		traj := []float64{-0.5, -0.3, -0.2}
		if !sentiment.DetectAnomalies(traj) {
			t.Error("expected anomaly detected for 3+ consecutive negatives")
		}
	})

	t.Run("exactly at threshold -0.1 counts as negative", func(t *testing.T) {
		traj := []float64{-0.1, -0.1, -0.1}
		if !sentiment.DetectAnomalies(traj) {
			t.Error("expected anomaly for exactly -0.1 threshold values")
		}
	})

	t.Run("four consecutive negative commits flags anomaly", func(t *testing.T) {
		traj := []float64{0.2, -0.4, -0.3, -0.5, -0.2, 0.1}
		if !sentiment.DetectAnomalies(traj) {
			t.Error("expected anomaly for 4 consecutive negatives in sequence")
		}
	})

	t.Run("two consecutive negative commits does not flag anomaly", func(t *testing.T) {
		traj := []float64{-0.5, -0.3, 0.1}
		if sentiment.DetectAnomalies(traj) {
			t.Error("expected no anomaly for only 2 consecutive negatives")
		}
	})

	t.Run("all positive trajectory returns false", func(t *testing.T) {
		traj := []float64{0.4, 0.3, 0.5, 0.2}
		if sentiment.DetectAnomalies(traj) {
			t.Error("expected no anomaly for positive trajectory")
		}
	})

	t.Run("empty trajectory returns false", func(t *testing.T) {
		if sentiment.DetectAnomalies([]float64{}) {
			t.Error("expected no anomaly for empty trajectory")
		}
	})

	t.Run("score just above -0.1 does not count as negative", func(t *testing.T) {
		// -0.05 is above -0.1, so not negative
		traj := []float64{-0.05, -0.05, -0.05}
		if sentiment.DetectAnomalies(traj) {
			t.Error("expected no anomaly when scores are above -0.1 threshold")
		}
	})
}

// TestMilestoneSentiment verifies aggregate sentiment across all issues' sessions.
func TestMilestoneSentiment(t *testing.T) {
	t.Run("aggregate across multiple issues and sessions", func(t *testing.T) {
		repo, shas := initRepoWithCommits(t, []string{
			"initial setup",
			"clean working implementation",
			"add tests passing",
		})

		iss1 := makeIssue([]issue.Session{{StartSHA: shas[0], EndSHA: shas[1]}})
		iss2 := makeIssue([]issue.Session{{StartSHA: shas[1], EndSHA: shas[2]}})

		score := sentiment.MilestoneSentiment(repo, []*issue.Issue{iss1, iss2})
		// Result should be a valid float in [-1.0, 1.0]
		if score < -1.0 || score > 1.0 {
			t.Errorf("MilestoneSentiment %f out of [-1.0, 1.0]", score)
		}
	})

	t.Run("no issues returns zero", func(t *testing.T) {
		repo, _ := initRepoWithCommits(t, []string{"initial"})
		score := sentiment.MilestoneSentiment(repo, []*issue.Issue{})
		if score != 0.0 {
			t.Errorf("expected 0 for empty issues, got %f", score)
		}
	})

	t.Run("issues with no sessions returns zero", func(t *testing.T) {
		repo, _ := initRepoWithCommits(t, []string{"initial"})
		iss := makeIssue([]issue.Session{})
		score := sentiment.MilestoneSentiment(repo, []*issue.Issue{iss})
		if score != 0.0 {
			t.Errorf("expected 0 for issue with no sessions, got %f", score)
		}
	})
}

// TestCompute verifies the SentimentMetrics aggregation struct.
func TestCompute(t *testing.T) {
	t.Run("anomalous issue ID appears in anomalies list", func(t *testing.T) {
		repo, shas := initRepoWithCommits(t, []string{
			"initial commit",
			"ugh broken horrible mess",
			"hack hack terrible workaround",
			"still broken argh",
		})

		iss := makeIssue([]issue.Session{{StartSHA: shas[0], EndSHA: shas[3]}})

		metrics := sentiment.Compute(repo, []*issue.Issue{iss})

		// Mean sentiment should be in valid range
		if metrics.MeanSentiment < -1.0 || metrics.MeanSentiment > 1.0 {
			t.Errorf("MeanSentiment %f out of range", metrics.MeanSentiment)
		}

		// The issue with sustained negative commits should appear in anomalies
		issID := iss.ID.String()
		found := false
		for _, id := range metrics.Anomalies {
			if id == issID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected issue %s in anomalies, got %v", issID, metrics.Anomalies)
		}
	})

	t.Run("clean positive issue has no anomalies", func(t *testing.T) {
		repo, shas := initRepoWithCommits(t, []string{
			"initial commit",
			"clean working implementation",
			"tests passing great",
		})

		iss := makeIssue([]issue.Session{{StartSHA: shas[0], EndSHA: shas[2]}})

		metrics := sentiment.Compute(repo, []*issue.Issue{iss})

		if len(metrics.Anomalies) != 0 {
			t.Errorf("expected no anomalies for positive issue, got %v", metrics.Anomalies)
		}
	})

	t.Run("empty issues returns zero mean and empty anomalies", func(t *testing.T) {
		repo, _ := initRepoWithCommits(t, []string{"initial"})
		metrics := sentiment.Compute(repo, []*issue.Issue{})
		if metrics.MeanSentiment != 0.0 {
			t.Errorf("expected 0 mean for empty input, got %f", metrics.MeanSentiment)
		}
		if len(metrics.Anomalies) != 0 {
			t.Errorf("expected empty anomalies for empty input, got %v", metrics.Anomalies)
		}
	})
}
