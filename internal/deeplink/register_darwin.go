// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package deeplink

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const lsregisterPath = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

// checkRegistration queries Launch Services to find the handler for glassbox://
// and validates the registration state explicitly, reporting partial success and
// failure causes rather than silently returning false.
func checkRegistration(selfPath string) Result {
	res := Result{
		FixSteps: genericFixSteps(),
	}

	// Validate that lsregister is accessible (required for macOS scheme registration).
	if _, err := os.Stat(lsregisterPath); err != nil {
		res.Err = fmt.Errorf("lsregister not found at %s: macOS LaunchServices unavailable", lsregisterPath)
		res.FixSteps = append([]string{
			"lsregister is missing — this may indicate an incomplete macOS installation.",
			"Ensure Xcode Command Line Tools are installed: xcode-select --install",
		}, res.FixSteps...)
		return res
	}

	// Detect whether the current process has sufficient privileges to query LaunchServices.
	if !hasLaunchServicesAccess() {
		res.Err = fmt.Errorf("insufficient privileges to query LaunchServices; run as the owning user, not root")
		res.FixSteps = append([]string{
			"LaunchServices queries must run as the desktop user, not root.",
			"Re-run without sudo: glassbox doctor",
		}, res.FixSteps...)
		return res
	}

	// Query lsregister dump for the glassbox scheme.
	out, err := exec.Command(lsregisterPath, "-dump").Output()
	if err != nil {
		res.Err = fmt.Errorf("lsregister -dump failed: %w", err)
		return res
	}

	dump := string(out)
	if strings.Contains(strings.ToLower(dump), "glassbox") {
		// Extract the handler line for reporting.
		for _, line := range strings.Split(dump, "\n") {
			if strings.Contains(strings.ToLower(line), "glassbox") {
				res.Handler = strings.TrimSpace(line)
				break
			}
		}

		// Partial-success check: registered handler must reference selfPath.
		if selfPath != "" && !strings.Contains(dump, selfPath) {
			res.Registered = true // scheme exists but points elsewhere
			res.Err = fmt.Errorf("glassbox:// is registered but handler does not point to %s (stale registration)", selfPath)
			res.FixSteps = []string{
				"Re-register to update the handler: glassbox install-scheme",
				fmt.Sprintf("Or manually: %s -f /Applications/Glassbox.app", lsregisterPath),
			}
			return res
		}

		res.Registered = true
		res.FixSteps = nil
		return res
	}

	// Fallback: open -Ra exits 0 when a handler exists.
	if err2 := exec.Command("open", "-Ra", "glassbox://").Run(); err2 == nil {
		res.Registered = true
		res.Handler = "registered (via open -Ra)"
		res.FixSteps = nil
		return res
	}

	// No registration found — provide explicit failure cause.
	res.Err = fmt.Errorf("glassbox:// scheme is not registered with LaunchServices")
	return res
}

// hasLaunchServicesAccess returns false when running as root, which prevents
// per-user LaunchServices queries from working correctly on macOS.
func hasLaunchServicesAccess() bool {
	return os.Getuid() != 0
}
