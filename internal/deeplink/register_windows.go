// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package deeplink

import (
	"fmt"
	"os/exec"
	"strings"
)

const windowsRegistryKey = `HKEY_CLASSES_ROOT\Glassbox`

// checkRegistration queries the Windows registry for the glassbox:// URL handler
// and validates the registration state explicitly, reporting partial success and
// failure causes rather than silently returning false.
func checkRegistration(selfPath string) Result {
	res := Result{
		FixSteps: genericFixSteps(),
	}

	// Detect whether the current process can write to HKEY_CLASSES_ROOT.
	// A read-only probe distinguishes "not registered" from "permission denied".
	if !hasRegistryWriteAccess() {
		res.Err = fmt.Errorf("insufficient privileges to read/write HKEY_CLASSES_ROOT; run as Administrator")
		res.FixSteps = append([]string{
			"Registry writes to HKEY_CLASSES_ROOT require Administrator privileges.",
			"Right-click the terminal and choose 'Run as administrator', then retry.",
		}, res.FixSteps...)
		return res
	}

	// Query the registry key.
	out, err := exec.Command("reg", "query", windowsRegistryKey, "/ve").Output()
	if err != nil {
		// Key does not exist — explicit not-registered failure.
		res.Err = fmt.Errorf("registry key %s not found: glassbox:// scheme is not registered", windowsRegistryKey)
		return res
	}

	value := string(out)

	// Validate the URL Protocol marker is present.
	if !strings.Contains(strings.ToLower(value), "url protocol") &&
		!strings.Contains(strings.ToLower(value), "url:glassbox") {
		res.Registered = true // key exists but is malformed
		res.Err = fmt.Errorf("registry key %s exists but is missing the 'URL Protocol' value (partial/corrupt registration)", windowsRegistryKey)
		res.FixSteps = []string{
			"Re-register to repair the handler: glassbox install-scheme (requires Administrator)",
			fmt.Sprintf("Or manually add: reg add %s /v \"URL Protocol\" /d \"\" /f", windowsRegistryKey),
		}
		return res
	}

	// Query the shell open command to validate the handler path.
	cmdOut, err := exec.Command(
		"reg", "query", windowsRegistryKey+`\shell\open\command`, "/ve",
	).Output()
	if err != nil {
		res.Registered = true // URL Protocol present but no open command
		res.Err = fmt.Errorf("registry key %s\\shell\\open\\command is missing (partial registration)", windowsRegistryKey)
		res.FixSteps = []string{
			"Re-register to repair the handler: glassbox install-scheme (requires Administrator)",
		}
		return res
	}

	cmdValue := strings.TrimSpace(string(cmdOut))

	// Partial-success check: registered handler must reference selfPath.
	if selfPath != "" && !strings.Contains(cmdValue, selfPath) {
		res.Registered = true // scheme exists but points elsewhere
		res.Handler = cmdValue
		res.Err = fmt.Errorf("glassbox:// is registered but handler does not point to %s (stale registration)", selfPath)
		res.FixSteps = []string{
			"Re-register to update the handler: glassbox install-scheme (requires Administrator)",
		}
		return res
	}

	res.Registered = true
	res.Handler = cmdValue
	res.FixSteps = nil
	return res
}

// hasRegistryWriteAccess probes whether the current process can query
// HKEY_CLASSES_ROOT. A failure here indicates insufficient privileges.
func hasRegistryWriteAccess() bool {
	err := exec.Command("reg", "query", `HKEY_CLASSES_ROOT`).Run()
	return err == nil
}
