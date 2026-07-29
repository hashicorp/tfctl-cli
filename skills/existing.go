package skills

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/tfctl-cli/version"
)

// InstalledSkill represents a skill that is already installed on the system.
type InstalledSkill struct {
	path      string
	global    bool
	agentName string
}

// Path returns the original path of the installed skill file.
func (e *InstalledSkill) Path() string {
	return e.path
}

// ResolvePath follows any symlinks and returns the absolute path of the ultimate target file.
func (e *InstalledSkill) ResolvePath() (string, error) {
	if e.path == "" {
		return "", fmt.Errorf("path is empty")
	}

	evaled, err := filepath.EvalSymlinks(e.path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(evaled)
}

// ReinstallCommand returns the command to reinstall the existing skill.
func (e *InstalledSkill) ReinstallCommand() string {
	if e.global {
		return fmt.Sprintf("%s harness install --global %s", version.Name, e.agentName)
	}
	return fmt.Sprintf("%s harness install %s", version.Name, e.agentName)
}

// sha256AtPath calculates and returns the SHA256 hash of the file at path.
// Returns an empty string if the file cannot be read or the hash cannot be calculated.
func sha256AtPath(path string) string {
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		if hash, err := hashSHA256Hex(f); err == nil {
			return hash
		}
	}
	return ""
}

// KnownSkillMatch contains information about a known version of an installed skill.
type KnownSkillMatch struct {
	Version string
	Hash    string
}

// MatchesKnownVersion checks if the existing skill is from a known version.
func (e *InstalledSkill) MatchesKnownVersion() (*KnownSkillMatch, bool) {
	return matchesKnownVersionAtPath(e.Path())
}

func matchesKnownVersionAtPath(path string) (*KnownSkillMatch, bool) {
	hash := sha256AtPath(path)
	if hash == "" {
		return nil, false
	}

	hashes, err := FS.Open(TFCTLKnownHashesPath)
	if err != nil {
		return nil, false
	}
	defer hashes.Close()

	scanner := bufio.NewScanner(hashes)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Split(line, " ")

		if len(fields) != 2 {
			continue
		}

		if hash == fields[0] {
			return &KnownSkillMatch{
				Version: fields[1],
				Hash:    fields[0],
			}, true
		}
	}
	// Ignore
	_ = scanner.Err()

	return nil, false
}

func hashSHA256Hex(r io.Reader) (string, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
