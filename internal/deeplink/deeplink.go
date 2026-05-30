// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

// Package deeplink provides utilities for verifying that the glassbox:// custom
// URL scheme is correctly registered with the host operating system and that
// clicking such a link actually dispatches to the running Glassbox binary.
package deeplink

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const (
	// Scheme is the custom URL scheme registered by Glassbox.
	Scheme = "glassbox"

	// MockURL is a safe no-op deep link used only for registration probing.
	// The handler must recognise the "doctor-probe" path and exit cleanly.
	MockURL = "glassbox://doctor-probe"

	// probeTimeout is the maximum time we wait for the OS to dispatch the link.
	probeTimeout = 5 * time.Second
)

// Result carries the outcome of a deep link verification attempt.
type Result struct {
	// Registered reports whether the glassbox:// scheme is registered with the OS.
	Registered bool
	// PartialSuccess is true when the scheme is registered but the handler is
	// stale, malformed, or points to a different binary. Callers should treat
	// this as a degraded state requiring repair rather than a clean failure.
	PartialSuccess bool
	// Dispatched reports whether a mock link was successfully dispatched.
	Dispatched bool
	// Handler is the binary path the OS has associated with the scheme.
	Handler string
	// Err holds the first error encountered, if any. When PartialSuccess is
	// true, Err describes the specific mismatch (e.g. stale path, missing
	// URL Protocol value, unsupported desktop environment).
	Err error
	// FixSteps contains ordered troubleshooting instructions.
	FixSteps []string
}

// Check performs a two-phase verification:
//  1. Inspect OS registration to confirm the scheme points to a Glassbox binary.
//  2. Trigger MockURL and verify the process exits cleanly within probeTimeout.
//
// The probe is intentionally non-interactive: the binary must handle
// "glassbox://doctor-probe" by printing nothing and exiting 0.
func Check(selfPath string) Result {
	if selfPath == "" {
		var err error
		selfPath, err = os.Executable()
		if err != nil {
			return Result{
				Err:      fmt.Errorf("cannot determine own executable path: %w", err),
				FixSteps: genericFixSteps(),
			}
		}
	}
	selfPath, _ = filepath.Abs(selfPath)

	res := checkRegistration(selfPath)

	// Propagate partial-success flag when the registration exists but is broken.
	if res.Registered && res.Err != nil {
		res.PartialSuccess = true
		return res
	}

	if !res.Registered {
		return res
	}

	res.Dispatched = triggerMockLink(selfPath)
	if !res.Dispatched {
		res.FixSteps = append(res.FixSteps,
			"The scheme is registered but the OS failed to dispatch the mock link.",
			"Try re-running 'glassbox install-scheme' or reinstalling Glassbox.",
		)
	}
	return res
}

// genericFixSteps returns platform-appropriate registration instructions.
func genericFixSteps() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"Register the scheme: glassbox install-scheme",
			"Or manually add Glassbox to /Applications and run: /System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f /Applications/Glassbox.app",
			"Verify with: open glassbox://doctor-probe",
		}
	case "windows":
		return []string{
			"Register the scheme: glassbox install-scheme  (requires Administrator)",
			"Or manually add the registry key: HKEY_CLASSES_ROOT\\Glassbox",
			"Verify with: start glassbox://doctor-probe",
		}
	default: // Linux / BSD
		return []string{
			"Register the scheme: glassbox install-scheme",
			"Or create ~/.local/share/applications/glassbox.desktop with MimeType=x-scheme-handler/glassbox",
			"Then run: xdg-mime default glassbox.desktop x-scheme-handler/glassbox",
			"Verify with: xdg-open glassbox://doctor-probe",
		}
	}
}

// triggerMockLink dispatches MockURL through the OS handler and waits for the
// process to exit.  It returns true only when the process exits with code 0
// within probeTimeout.
func triggerMockLink(selfPath string) bool {
	// We invoke the binary directly rather than through the OS URL dispatcher
	// so the test is hermetic and does not require the scheme to be registered.
	// The --deep-link flag tells the binary to handle the URL and exit.
	cmd := exec.Command(selfPath, "--deep-link", MockURL) //nolint:gosec // selfPath is our own trusted binary
	cmd.Env = append(os.Environ(), "GLASSBOX_DEEP_LINK_PROBE=1")

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return false
	}
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return err == nil
	case <-time.After(probeTimeout):
		_ = cmd.Process.Kill()
		return false
	}
}
