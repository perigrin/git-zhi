// ABOUTME: Identity mapping between git-zhi actor names and Jira email addresses.
// ABOUTME: Reads from a flat map derived from git config zhi.sync.jira.actor.<name>.
package identity

// MapActorToJira looks up the Jira email address for the given git-zhi actor
// name in cfg. cfg is a flat map of actorName → jiraEmail derived from git
// config keys under zhi.sync.jira.actor.<name>.
// Returns empty string if the actor is not configured.
func MapActorToJira(cfg map[string]string, actorName string) string {
	return cfg[actorName]
}

// MapJiraToActor performs the reverse lookup: given a Jira email (or display
// name), it returns the git-zhi actor name whose configured Jira identity
// matches jiraDisplayName. Returns empty string if no match is found.
func MapJiraToActor(cfg map[string]string, jiraDisplayName string) string {
	for actor, email := range cfg {
		if email == jiraDisplayName {
			return actor
		}
	}
	return ""
}
