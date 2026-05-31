// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const checkpointFilename = "active_session.json"

// Checkpoint records the active debug session for crash-recovery purposes.
// It is written when a session starts and removed on clean exit. If the file
// still exists on the next invocation and the originating process is gone,
// the session was interrupted and can be recovered.
type Checkpoint struct {
	// SessionID is the ID of the session in the Store.
	SessionID string `json:"session_id"`
	// TxHash is the transaction being debugged.
	TxHash string `json:"tx_hash"`
	// Network is the Stellar network the session ran against.
	Network string `json:"network"`
	// StartedAt is the moment the session became active.
	StartedAt time.Time `json:"started_at"`
	// PID is the OS process ID of the Glassbox process that owns the session.
	PID int `json:"pid"`
}

// checkpointPath returns the path to the on-disk checkpoint file.
func checkpointPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory unavailable: %w", err)
	}
	return filepath.Join(home, ".Glassbox", checkpointFilename), nil
}

// WriteCheckpoint persists an active-session checkpoint for crash recovery.
// Call this when a debug session starts and ClearCheckpoint when it ends cleanly.
func WriteCheckpoint(cp *Checkpoint) error {
	path, err := checkpointPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create checkpoint directory: %w", err)
	}
	cp.PID = os.Getpid()
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}

// ClearCheckpoint removes the crash-recovery checkpoint after a clean session exit.
// A missing checkpoint file is not treated as an error.
func ClearCheckpoint() error {
	path, err := checkpointPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove checkpoint: %w", err)
	}
	return nil
}

// LoadCheckpoint reads the last crash-recovery checkpoint if one exists.
// Returns (nil, nil) when no checkpoint file is present.
func LoadCheckpoint() (*Checkpoint, error) {
	path, err := checkpointPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint: %w", err)
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("malformed checkpoint file: %w", err)
	}
	return &cp, nil
}

// IsOrphaned returns true when the checkpoint's originating process is no
// longer running, indicating an unclean termination that left the session open.
// It uses signal 0 (existence probe) which never delivers a signal to the target.
func (c *Checkpoint) IsOrphaned() bool {
	if c.PID <= 0 {
		return true
	}
	// syscall.Kill(pid, 0) probes process existence on POSIX systems:
	//   nil   → process exists and is accessible
	//   ESRCH → no such process (orphaned)
	//   EPERM → process exists but we lack permission (not orphaned)
	err := syscall.Kill(c.PID, 0)
	return err == syscall.ESRCH
}
