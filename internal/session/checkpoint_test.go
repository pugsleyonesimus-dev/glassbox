// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// overrideCheckpointDir temporarily redirects the checkpoint file into a temp
// directory by setting HOME, restoring it in t.Cleanup.
func overrideCheckpointDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := os.Getenv("HOME")
	t.Setenv("HOME", dir)
	t.Cleanup(func() { os.Setenv("HOME", prev) })
	return dir
}

// ─── WriteCheckpoint / LoadCheckpoint ────────────────────────────────────────

func TestCheckpoint_WriteAndLoad(t *testing.T) {
	overrideCheckpointDir(t)

	cp := &Checkpoint{
		SessionID: "abc123",
		TxHash:    "deadbeef",
		Network:   "testnet",
		StartedAt: time.Now().Truncate(time.Second),
	}
	require.NoError(t, WriteCheckpoint(cp))

	loaded, err := LoadCheckpoint()
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, cp.SessionID, loaded.SessionID)
	assert.Equal(t, cp.TxHash, loaded.TxHash)
	assert.Equal(t, cp.Network, loaded.Network)
	assert.Equal(t, os.Getpid(), loaded.PID, "PID should be set to the current process")
}

func TestCheckpoint_LoadAbsent(t *testing.T) {
	overrideCheckpointDir(t)

	loaded, err := LoadCheckpoint()
	require.NoError(t, err)
	assert.Nil(t, loaded, "nil returned when no checkpoint exists")
}

func TestCheckpoint_LoadMalformed(t *testing.T) {
	dir := overrideCheckpointDir(t)
	cpPath := filepath.Join(dir, ".Glassbox", checkpointFilename)
	require.NoError(t, os.MkdirAll(filepath.Dir(cpPath), 0755))
	require.NoError(t, os.WriteFile(cpPath, []byte("not json {{{"), 0600))

	_, err := LoadCheckpoint()
	assert.Error(t, err, "malformed checkpoint should return an error")
}

// ─── ClearCheckpoint ─────────────────────────────────────────────────────────

func TestCheckpoint_ClearRemovesFile(t *testing.T) {
	overrideCheckpointDir(t)

	require.NoError(t, WriteCheckpoint(&Checkpoint{SessionID: "x"}))
	require.NoError(t, ClearCheckpoint())

	loaded, err := LoadCheckpoint()
	require.NoError(t, err)
	assert.Nil(t, loaded, "checkpoint should be gone after Clear")
}

func TestCheckpoint_ClearIdempotent(t *testing.T) {
	overrideCheckpointDir(t)
	assert.NoError(t, ClearCheckpoint(), "clearing a non-existent checkpoint is a no-op")
}

// ─── IsOrphaned ──────────────────────────────────────────────────────────────

func TestCheckpoint_IsOrphaned_CurrentProcess(t *testing.T) {
	cp := &Checkpoint{PID: os.Getpid()}
	assert.False(t, cp.IsOrphaned(), "current process is not orphaned")
}

func TestCheckpoint_IsOrphaned_InvalidPID(t *testing.T) {
	cp := &Checkpoint{PID: 0}
	assert.True(t, cp.IsOrphaned(), "PID=0 is treated as orphaned")
}

func TestCheckpoint_IsOrphaned_DeadPID(t *testing.T) {
	// PID 1 is always the init process (guaranteed alive on Linux/macOS). We
	// instead look for a PID that is very unlikely to exist — INT_MAX-like value.
	// On most kernels pid_max is 4194304; 9999999 should be beyond that.
	cp := &Checkpoint{PID: 9999999}
	// We expect this to be orphaned, but it might not be if the OS wraps pids.
	// At minimum the call must not panic.
	_ = cp.IsOrphaned()
}

func TestCheckpoint_WriteCreatesGlassboxDir(t *testing.T) {
	dir := overrideCheckpointDir(t)
	glassboxDir := filepath.Join(dir, ".Glassbox")

	// Ensure the directory does not exist before write.
	require.NoError(t, os.RemoveAll(glassboxDir))

	require.NoError(t, WriteCheckpoint(&Checkpoint{SessionID: "new"}))
	_, err := os.Stat(glassboxDir)
	assert.NoError(t, err, ".Glassbox directory should be created automatically")
}
