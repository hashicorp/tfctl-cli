package skills

import (
	"context"
	"slices"

	"github.com/hashicorp/tfctl-cli/internal/pkg/logging"
	"github.com/hashicorp/tfctl-cli/version"
)

// MigrationResult is the result of attempting to migrate an installed skill, including any
// potential failure reason, and the previous version if successful.
type MigrationResult struct {
	SkillPath       string
	FailedReason    error
	PreviousVersion string
}

// Migration represents an ongoing migration process, including a channel to signal completion
// and a slice to store results.
type Migration struct {
	done    chan struct{}
	results []MigrationResult
}

// StartMigration will attempt to migrate all installed skills for all agents to the
// latest known version. Call this function only once per execution.
func StartMigration(ctx context.Context) *Migration {
	m := &Migration{done: make(chan struct{})}
	go func() {
		defer close(m.done)
		m.results = migrateInstalled(ctx)
	}()
	return m
}

func migrateInstalled(ctx context.Context) []MigrationResult {
	logger := logging.FromContext(ctx)
	seenPaths := make(map[string]struct{})

	results := make([]MigrationResult, 0)
	for _, name := range AgentNames {
		if agent, ok := GetAgent(name); ok {
			for existing := range agent.InstalledSkills() {
				// Many agents install skills to the same path. We only need to check these once.
				if _, seen := seenPaths[existing.Path()]; seen {
					continue
				}
				seenPaths[existing.Path()] = struct{}{}

				// Some skill files are actually symlinks, so we need to resolve those first before
				// overwriting the contents.
				installedLocation, err := existing.ResolvePath()
				if err != nil {
					results = append(results, MigrationResult{
						SkillPath:    existing.Path(),
						FailedReason: err,
					})
					continue
				}

				if match, ok := matchesKnownVersionAtPath(installedLocation); ok {
					if match.Hash != EmbeddedSkillHash() {
						// This is a match for a previous version and can be migrated
						err := agent.installSkillToPath(installedLocation)
						if err != nil {
							results = append(results, MigrationResult{
								SkillPath:    installedLocation,
								FailedReason: err,
							})
						} else {
							results = append(results, MigrationResult{
								SkillPath:       installedLocation,
								FailedReason:    nil,
								PreviousVersion: match.Version,
							})
						}
					} else {
						// Matches the embedded hash. Skill is already up to date.
						logger.Debug("Installed skill already up to date", "path", existing.Path())
					}
				} else {
					// This is not a match for any known skill and should not be migrated.
					if !version.IsDev() {
						logger.Debug("Installed skill is unrecognized, likely contains user edits", "path", existing.Path())
					}
				}
			}
		}
	}
	return results
}

// Wait waits for the migration process to complete and returns all MigrationResults or an error
// if the context is canceled.
func (m *Migration) Wait(ctx context.Context) ([]MigrationResult, error) {
	select {
	case <-m.done:
		return slices.Clone(m.results), nil
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}
