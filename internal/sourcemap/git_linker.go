// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package sourcemap

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitLinker resolves source file paths relative to a repository root and
// constructs GitHub blob URLs. It supports nested Cargo workspaces and
// monorepo layouts where a contract lives in a subdirectory.
type GitLinker struct {
	// repoRoot is the absolute path to the .git directory's parent.
	repoRoot string
	// remoteURL is the parsed GitHub remote origin URL.
	remoteURL string
	// defaultBranch is the branch used when constructing blob URLs.
	defaultBranch string
}

// NewGitLinker creates a GitLinker by discovering the repository root upward
// from startPath. startPath may be a file or directory inside the repo.
func NewGitLinker(startPath string) (*GitLinker, error) {
	root, err := findRepoRoot(startPath)
	if err != nil {
		return nil, fmt.Errorf("repository root not found from %q: %w", startPath, err)
	}

	remote, err := gitRemoteOrigin(root)
	if err != nil {
		return nil, fmt.Errorf("could not determine remote origin for repo at %q: %w", root, err)
	}

	branch, err := gitDefaultBranch(root)
	if err != nil {
		branch = "main" // safe fallback
	}

	return &GitLinker{
		repoRoot:      root,
		remoteURL:     remote,
		defaultBranch: branch,
	}, nil
}

// GitHubURL returns the GitHub blob URL for the given absolute source file path.
// It normalises the path relative to the repository root so that contracts in
// nested workspaces (e.g. contracts/token/src/lib.rs) resolve correctly.
func (g *GitLinker) GitHubURL(absFilePath string) (string, error) {
	owner, repo, err := parseGitHubURL(g.remoteURL)
	if err != nil {
		return "", fmt.Errorf("cannot parse remote URL %q: %w", g.remoteURL, err)
	}

	rel, err := filepath.Rel(g.repoRoot, absFilePath)
	if err != nil {
		return "", fmt.Errorf("cannot make %q relative to repo root %q: %w", absFilePath, g.repoRoot, err)
	}

	// Normalise to forward slashes for GitHub URLs.
	rel = filepath.ToSlash(rel)
	if strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("file %q is outside the repository root %q", absFilePath, g.repoRoot)
	}

	return fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s",
		owner, repo, g.defaultBranch, rel), nil
}

// RepoRoot returns the discovered repository root path.
func (g *GitLinker) RepoRoot() string { return g.repoRoot }

// findRepoRoot walks upward from startPath searching for a .git directory,
// Cargo.toml workspace manifest, or other repository root indicators.
// It returns the first directory that contains a .git entry.
func findRepoRoot(startPath string) (string, error) {
	abs, err := filepath.Abs(startPath)
	if err != nil {
		return "", err
	}

	// If startPath is a file, begin from its directory.
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	dir := abs
	if !info.IsDir() {
		dir = filepath.Dir(abs)
	}

	for {
		if isRepoRoot(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached filesystem root
		}
		dir = parent
	}

	return "", fmt.Errorf("no .git directory found above %q", startPath)
}

// isRepoRoot returns true when dir contains a .git entry (directory or file
// for git worktrees).
func isRepoRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// gitRemoteOrigin returns the remote origin URL for the repository at repoRoot.
func gitRemoteOrigin(repoRoot string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git remote get-url origin: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// gitDefaultBranch returns the default branch name for the repository.
func gitDefaultBranch(repoRoot string) (string, error) {
	// Try symbolic-ref HEAD first (works for local checkouts).
	cmd := exec.Command("git", "symbolic-ref", "--short", "HEAD")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err == nil {
		branch := strings.TrimSpace(string(out))
		if branch != "" {
			return branch, nil
		}
	}

	// Fallback: ask the remote for its HEAD.
	cmd2 := exec.Command("git", "remote", "show", "origin")
	cmd2.Dir = repoRoot
	out2, err := cmd2.Output()
	if err != nil {
		return "main", nil
	}
	for _, line := range strings.Split(string(out2), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "HEAD branch:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "HEAD branch:")), nil
		}
	}
	return "main", nil
}

// NormalizeSourcePath resolves a potentially relative source path to an
// absolute path anchored at repoRoot, then returns the GitHub URL.
// This handles DWARF debug info that may embed relative paths.
func (g *GitLinker) NormalizeSourcePath(rawPath string) (string, error) {
	if filepath.IsAbs(rawPath) {
		return g.GitHubURL(rawPath)
	}
	// Treat relative paths as relative to the repo root.
	abs := filepath.Join(g.repoRoot, rawPath)
	return g.GitHubURL(abs)
}
