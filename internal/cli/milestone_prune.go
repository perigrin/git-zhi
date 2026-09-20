// ABOUTME: Implementation of the milestone prune command: removes machine-made
// ABOUTME: empty milestones while sparing anything a human authored or tagged.
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/perigrin/git-zhi/internal/issue"
	"github.com/perigrin/git-zhi/internal/milestone"
	"github.com/perigrin/git-zhi/internal/storage"
)

// tagRefPrefix is the ref namespace holding named tags. A tag's content is
// the ref path it points at (e.g. a milestone ref), not a commit SHA.
const tagRefPrefix = "refs/zhi/_/tags/"

func newMilestonePruneCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove empty, unauthored milestones",
		Args:  cobra.NoArgs,
		RunE:  runMilestonePrune,
	}
	cmd.Flags().Bool("dry-run", false, "report what would be pruned without removing anything")
	return cmd
}

// isPrunable reports whether a milestone matches the narrow "machine-made and
// empty" predicate: zero issues, no body, no resolution, no postmortem, no
// due date, not completed, and not tagged. This is exactly what
// EnsureInitialized used to manufacture and nothing a human typed — a single
// authored field, or a tag, is enough to spare it.
func isPrunable(ms *milestone.Milestone, issueCount int, tagged bool) bool {
	return issueCount == 0 &&
		ms.Body == "" &&
		ms.Resolution == "" &&
		ms.Postmortem == "" &&
		ms.Due == nil &&
		ms.State != "completed" &&
		!tagged
}

// loadTagTargets reads every tag ref and returns a map from the ref path it
// points at (e.g. a milestone ref) to the tag's own name.
func loadTagTargets(store *storage.Store) (map[string]string, error) {
	tagRefs, err := store.ListRefs(tagRefPrefix)
	if err != nil {
		return nil, fmt.Errorf("list tag refs: %w", err)
	}
	targets := make(map[string]string, len(tagRefs))
	for _, tagRef := range tagRefs {
		data, err := store.ReadEntity(tagRef, "tag.txt")
		if err != nil {
			continue // unreadable tag ref; nothing to key on
		}
		targets[string(data)] = strings.TrimPrefix(tagRef, tagRefPrefix)
	}
	return targets, nil
}

func runMilestonePrune(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd.Context())
	if app == nil {
		return fmt.Errorf("no git repository found")
	}

	if err := app.EnsureInitialized(); err != nil {
		return fmt.Errorf("initialize chain: %w", err)
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")

	milestones, err := milestone.LoadAllMilestones(app.Store)
	if err != nil {
		return fmt.Errorf("load milestones: %w", err)
	}

	issues, err := issue.LoadAllIssues(app.Store)
	if err != nil {
		return fmt.Errorf("load issues: %w", err)
	}
	issueCounts := make(map[string]int, len(milestones))
	for _, iss := range issues {
		issueCounts[iss.Milestone]++
	}

	tagTargets, err := loadTagTargets(app.Store)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	count := 0
	for _, ms := range milestones {
		refPath := milestone.RefPrefix + ms.Name
		tagName, tagged := tagTargets[refPath]

		if !isPrunable(ms, issueCounts[ms.Name], false) {
			continue
		}
		if tagged {
			fmt.Fprintf(out, "%s: skipped, tagged %q\n", ms.Name, tagName)
			continue
		}

		if dryRun {
			fmt.Fprintf(out, "[dry-run] would prune %s\n", ms.Name)
			count++
			continue
		}

		if err := app.Store.DeleteRef(refPath); err != nil {
			return fmt.Errorf("delete milestone %s: %w", ms.Name, err)
		}
		fmt.Fprintf(out, "pruned %s\n", ms.Name)
		count++
	}

	if dryRun {
		fmt.Fprintf(out, "examined %d milestones, would prune %d\n", len(milestones), count)
	} else {
		fmt.Fprintf(out, "examined %d milestones, pruned %d\n", len(milestones), count)
	}
	return nil
}
