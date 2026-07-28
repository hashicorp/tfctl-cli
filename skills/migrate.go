package skills

import (
	"context"
	"errors"

	"github.com/hashicorp/tfctl-cli/internal/pkg/logging"
)

var migrationResult chan *MigrationResult
var migrationBegun bool

// MigrationResult is the result of attempting to migrate an installed skill, including any
// potential failure reason, and the previous version if successful.
type MigrationResult struct {
	SkillPath       string
	FailedReason    error
	PreviousVersion string
}

func init() {
	migrationResult = make(chan *MigrationResult)
}

// MigrateInstalled will attempt to migrate all installed skills for all agents to the
// latest known version. Call this function only once per execution. Call this function
// with a goroutine, then retrieve results with WaitForMigration().
func MigrateInstalled(ctx context.Context) {
	if migrationBegun {
		return
	}
	migrationBegun = true

	logger := logging.FromContext(ctx)
	seenPaths := make(map[string]struct{})

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
					migrationResult <- &MigrationResult{
						SkillPath:    existing.Path(),
						FailedReason: err,
					}
					continue
				}

				if match, ok := existing.MatchesKnownVersion(); ok {
					if match.Hash != EmbeddedSkillHash() {
						// This is a match for a previous version and can be migrated
						err := agent.installSkillToPath(installedLocation)
						if err != nil {
							migrationResult <- &MigrationResult{
								SkillPath:    installedLocation,
								FailedReason: err,
							}
						} else {
							migrationResult <- &MigrationResult{
								SkillPath:       installedLocation,
								FailedReason:    nil,
								PreviousVersion: match.Version,
							}
						}
					} else {
						// Matches the embedded hash. Skill is already up to date. Development code will not hit
						// this code path.
						logger.Debug("Installed skill already up to date", "path", existing.Path())
					}
				} else {
					// This is not a match for any known skill and should not be migrated, but
					// may be outdated.
					migrationResult <- &MigrationResult{
						SkillPath:    installedLocation,
						FailedReason: errors.New("does not match any known skill version"),
					}
				}
			}
		}
	}
	close(migrationResult)
}

// WaitForMigration waits for the migration process to complete and returns all MigrationResults.
func WaitForMigration() []MigrationResult {
	if !migrationBegun {
		return nil
	}

	var results []MigrationResult
	for res := range migrationResult {
		results = append(results, *res)
	}
	return results
}
