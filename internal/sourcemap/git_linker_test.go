// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package sourcemap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeGitRepo creates a minimal fake git repository in dir and returns the
// root path. It writes a .git directory and a fake remote origin config so
// that gitRemoteOrigin can be tested without a real network.
func makeGitRepo(t *testing.T, remoteURL string) string {
	t.Helper()
	root := t.TempDir()

	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs"), 0o755); err != nil {
		t.Fatalf("mkdir .git/refs: %v", err)
	}

	// Write HEAD pointing to main.
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}

	// Write a minimal config with remote origin.
	config := "[remote \"origin\"]\n\turl = " + remoteURL + "\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(config), 0o644); err != nil {
		t.Fatalf("write git config: %v", err)
	}

	return root
}

// TestFindRepoRoot_FromFile verifies that findRepoRoot walks up from a file
// inside the repo and returns the root containing .git.
func TestFindRepoRoot_FromFile(t *testing.T) {
	root := makeGitRepo(t, "https://github.com/test/repo")

	// Create a nested source file.
	srcDir := filepath.Join(root, "contracts", "token", "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	srcFile := filepath.Join(srcDir, "lib.rs")
	if err := os.WriteFile(srcFile, []byte("// lib"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got, err := findRepoRoot(srcFile)
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	if got != root {
		t.Errorf("findRepoRoot = %q, want %q", got, root)
	}
}

// TestFindRepoRoot_FromDirectory verifies that findRepoRoot works when given
// a directory path rather than a file path.
func TestFindRepoRoot_FromDirectory(t *testing.T) {
	root := makeGitRepo(t, "https://github.com/test/repo")

	subDir := filepath.Join(root, "contracts", "token")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, err := findRepoRoot(subDir)
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}
	if got != root {
		t.Errorf("findRepoRoot = %q, want %q", got, root)
	}
}

// TestFindRepoRoot_NoGit verifies that findRepoRoot returns an error when
// there is no .git directory in the path hierarchy.
func TestFindRepoRoot_NoGit(t *testing.T) {
	dir := t.TempDir() // no .git
	_, err := findRepoRoot(dir)
	if err == nil {
		t.Error("expected error when no .git directory exists")
	}
}

// TestIsRepoRoot verifies the .git detection helper.
func TestIsRepoRoot(t *testing.T) {
	root := makeGitRepo(t, "https://github.com/test/repo")
	if !isRepoRoot(root) {
		t.Errorf("isRepoRoot(%q) = false, want true", root)
	}
	if isRepoRoot(t.TempDir()) {
		t.Error("isRepoRoot should return false for a directory without .git")
	}
}

// TestGitLinker_GitHubURL_NestedWorkspace verifies that a file deep inside a
// monorepo subdirectory produces the correct GitHub blob URL.
func TestGitLinker_GitHubURL_NestedWorkspace(t *testing.T) {
	root := makeGitRepo(t, "https://github.com/stellar/soroban-examples")

	// Simulate a nested workspace: contracts/token/src/lib.rs
	srcDir := filepath.Join(root, "contracts", "token", "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	srcFile := filepath.Join(srcDir, "lib.rs")
	if err := os.WriteFile(srcFile, []byte("// lib"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	linker := &GitLinker{
		repoRoot:      root,
		remoteURL:     "https://github.com/stellar/soroban-examples",
		defaultBranch: "main",
	}

	url, err := linker.GitHubURL(srcFile)
	if err != nil {
		t.Fatalf("GitHubURL: %v", err)
	}

	want := "https://github.com/stellar/soroban-examples/blob/main/contracts/token/src/lib.rs"
	if url != want {
		t.Errorf("GitHubURL = %q, want %q", url, want)
	}
}

// TestGitLinker_GitHubURL_RepoRoot verifies a file at the repository root.
func TestGitLinker_GitHubURL_RepoRoot(t *testing.T) {
	root := makeGitRepo(t, "https://github.com/stellar/soroban-examples")

	cargoToml := filepath.Join(root, "Cargo.toml")
	if err := os.WriteFile(cargoToml, []byte("[workspace]"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	linker := &GitLinker{
		repoRoot:      root,
		remoteURL:     "https://github.com/stellar/soroban-examples",
		defaultBranch: "main",
	}

	url, err := linker.GitHubURL(cargoToml)
	if err != nil {
		t.Fatalf("GitHubURL: %v", err)
	}

	want := "https://github.com/stellar/soroban-examples/blob/main/Cargo.toml"
	if url != want {
		t.Errorf("GitHubURL = %q, want %q", url, want)
	}
}

// TestGitLinker_GitHubURL_OutsideRepo verifies that a file outside the repo
// root returns an error.
func TestGitLinker_GitHubURL_OutsideRepo(t *testing.T) {
	root := makeGitRepo(t, "https://github.com/stellar/soroban-examples")

	outsideFile := filepath.Join(t.TempDir(), "outside.rs")
	if err := os.WriteFile(outsideFile, []byte(""), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	linker := &GitLinker{
		repoRoot:      root,
		remoteURL:     "https://github.com/stellar/soroban-examples",
		defaultBranch: "main",
	}

	_, err := linker.GitHubURL(outsideFile)
	if err == nil {
		t.Error("expected error for file outside repo root")
	}
}

// TestGitLinker_NormalizeSourcePath_Relative verifies that a relative path is
// resolved against the repo root.
func TestGitLinker_NormalizeSourcePath_Relative(t *testing.T) {
	root := makeGitRepo(t, "https://github.com/stellar/soroban-examples")

	srcDir := filepath.Join(root, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "lib.rs"), []byte(""), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	linker := &GitLinker{
		repoRoot:      root,
		remoteURL:     "https://github.com/stellar/soroban-examples",
		defaultBranch: "main",
	}

	url, err := linker.NormalizeSourcePath("src/lib.rs")
	if err != nil {
		t.Fatalf("NormalizeSourcePath: %v", err)
	}

	if !strings.Contains(url, "src/lib.rs") {
		t.Errorf("URL %q should contain src/lib.rs", url)
	}
	if !strings.HasPrefix(url, "https://github.com/") {
		t.Errorf("URL %q should start with https://github.com/", url)
	}
}

// TestGitLinker_NormalizeSourcePath_Absolute verifies that an absolute path
// is handled correctly.
func TestGitLinker_NormalizeSourcePath_Absolute(t *testing.T) {
	root := makeGitRepo(t, "https://github.com/stellar/soroban-examples")

	srcFile := filepath.Join(root, "contracts", "counter", "src", "lib.rs")
	if err := os.MkdirAll(filepath.Dir(srcFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(srcFile, []byte(""), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	linker := &GitLinker{
		repoRoot:      root,
		remoteURL:     "https://github.com/stellar/soroban-examples",
		defaultBranch: "main",
	}

	url, err := linker.NormalizeSourcePath(srcFile)
	if err != nil {
		t.Fatalf("NormalizeSourcePath: %v", err)
	}

	want := "https://github.com/stellar/soroban-examples/blob/main/contracts/counter/src/lib.rs"
	if url != want {
		t.Errorf("NormalizeSourcePath = %q, want %q", url, want)
	}
}

// TestGitLinker_SSHRemote verifies that SSH remote URLs are parsed correctly.
func TestGitLinker_SSHRemote(t *testing.T) {
	linker := &GitLinker{
		repoRoot:      t.TempDir(),
		remoteURL:     "git@github.com:stellar/soroban-examples.git",
		defaultBranch: "main",
	}

	root := makeGitRepo(t, "git@github.com:stellar/soroban-examples.git")
	srcFile := filepath.Join(root, "src", "lib.rs")
	if err := os.MkdirAll(filepath.Dir(srcFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(srcFile, []byte(""), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	linker.repoRoot = root

	url, err := linker.GitHubURL(srcFile)
	if err != nil {
		t.Fatalf("GitHubURL with SSH remote: %v", err)
	}
	if !strings.Contains(url, "stellar/soroban-examples") {
		t.Errorf("URL %q should contain stellar/soroban-examples", url)
	}
}
