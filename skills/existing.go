package skills

import (
	"bufio"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/hashicorp/tfctl-cli/version"
)

// InstalledSkill represents a skill that is already installed on the system.
type InstalledSkill struct {
	Path      string
	global    bool
	agentName string
}

// ReinstallCommand returns the command to reinstall the existing skill.
func (e *InstalledSkill) ReinstallCommand() string {
	if e.global {
		return fmt.Sprintf("%s harness install --global %s", version.Name, e.agentName)
	}
	return fmt.Sprintf("%s harness install %s", version.Name, e.agentName)
}

// crc32 calculates and returns the CRC32 hash of the existing skill file.
// Returns nil if the file cannot be read or the hash cannot be calculated.
func (e *InstalledSkill) crc32() *uint32 {
	f, err := os.Open(e.Path)
	if err == nil {
		defer f.Close()
		if hash, err := calculateCRC32(f); err == nil {
			return &hash
		}
	}
	return nil
}

// KnownVersion checks if the existing skill is from a known version.
func (e *InstalledSkill) KnownVersion() (string, bool) {
	checksums, err := FS.Open("tfctl/checksums")
	if err != nil {
		return "", false
	}
	defer checksums.Close()

	scanner := bufio.NewScanner(checksums)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Split(line, " ")

		if len(fields) < 3 {
			continue
		}

		checksum, err := strconv.ParseUint(fields[1], 10, 32)
		if err != nil {
			continue
		}

		if InstalledSkillChecksum := e.crc32(); InstalledSkillChecksum != nil && *InstalledSkillChecksum == uint32(checksum) {
			return fields[0], true
		}
	}
	// Ignore
	_ = scanner.Err()

	return "", false
}

func calculateCRC32(r io.Reader) (uint32, error) {
	hasher := crc32.NewIEEE()
	if _, err := io.Copy(hasher, r); err != nil {
		return 0, err
	}
	return hasher.Sum32(), nil
}
