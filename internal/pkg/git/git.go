// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package git inspects the current repository and parses git remote metadata.
package git

import (
	"fmt"
	"net/url"
	"os/exec"
	"path"
	"strings"
)

// RepoContext describes the current git repository and selected remote.
type RepoContext struct {
	// Root is the repository root directory.
	Root string
	// RemoteName is the git remote name that was inspected.
	RemoteName string
	// RemoteURL is the git remote URL.
	RemoteURL string
	// RepoOwner is the remote repository owner or namespace.
	RepoOwner string
	// RepoName is the remote repository name.
	RepoName string
	// RepoSlug is the owner and repository name joined as owner/repo.
	RepoSlug string
	// RemoteHost is the remote repository host.
	RemoteHost string
	// CurrentBranch is the currently checked out branch name.
	CurrentBranch string
}

// Inspect gathers repository details for the current working tree and remote.
func Inspect(remote string) (*RepoContext, error) {
	if remote == "" {
		remote = "origin"
	}

	root, err := runGit("rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not a git repository: %w", err)
	}

	remoteURL, err := runGit("remote", "get-url", remote)
	if err != nil {
		return nil, fmt.Errorf("read git remote %q: %w", remote, err)
	}

	host, owner, repo, err := ParseRemoteURL(remoteURL)
	if err != nil {
		return nil, err
	}

	branch, err := runGit("branch", "--show-current")
	if err != nil {
		branch = ""
	}

	return &RepoContext{
		Root:          root,
		RemoteName:    remote,
		RemoteURL:     remoteURL,
		RepoOwner:     owner,
		RepoName:      repo,
		RepoSlug:      owner + "/" + repo,
		RemoteHost:    host,
		CurrentBranch: branch,
	}, nil
}

// ParseRemoteURL parses a git remote URL into host, owner, and repository name.
func ParseRemoteURL(raw string) (host, owner, repo string, err error) {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "git@") {
		parts := strings.SplitN(strings.TrimPrefix(trimmed, "git@"), ":", 2)
		if len(parts) != 2 {
			return "", "", "", fmt.Errorf("unsupported git remote %q", raw)
		}
		host = parts[0]
		owner, repo, err = splitPath(parts[1])
		return host, owner, repo, err
	}

	if strings.HasPrefix(trimmed, "ssh://") || strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "http://") {
		u, parseErr := url.Parse(trimmed)
		if parseErr != nil {
			return "", "", "", parseErr
		}
		host = u.Hostname()
		owner, repo, err = splitPath(strings.TrimPrefix(u.Path, "/"))
		return host, owner, repo, err
	}

	return "", "", "", fmt.Errorf("unsupported git remote %q", raw)
}

func splitPath(p string) (string, string, error) {
	p = strings.TrimSuffix(p, ".git")
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("unsupported repository path %q", p)
	}
	repo := parts[len(parts)-1]
	owner := path.Join(parts[:len(parts)-1]...)
	return owner, repo, nil
}

func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
