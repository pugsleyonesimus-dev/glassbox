// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package signer

import (
	"fmt"
	"os"
)

// OsPkcs11Provider is the default Pkcs11Provider used by the CLI. It
// performs the file-system checks that do not require a loaded module
// (module_path step) and delegates the remaining steps to the Go plugin
// loader. Because Go's plugin package cannot load C shared objects
// directly, the actual PKCS#11 FFI calls are not implemented here; they
// require a cgo binding or a pure-Go wrapper such as github.com/miekg/pkcs11.
//
// The provider is intentionally kept as a thin shim so that the validator
// logic and its tests remain fully exercisable via the mock provider. When
// a real PKCS#11 binding is wired in, replace the method bodies below with
// the appropriate FFI calls.
type OsPkcs11Provider struct {
	// modulePath is set by LoadModule and used by subsequent calls.
	modulePath string
}

// LoadModule verifies the module file exists and is readable. It does not
// actually dlopen the library because Go's plugin package only supports
// Go plugins, not C shared objects.
func (p *OsPkcs11Provider) LoadModule(path string) error {
	p.modulePath = path
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Error{Op: "pkcs11", Msg: fmt.Sprintf("module file not found: %s", path)}
		}
		return &Error{Op: "pkcs11", Msg: fmt.Sprintf("cannot access module file: %v", err)}
	}
	if info.IsDir() {
		return &Error{Op: "pkcs11", Msg: fmt.Sprintf("%s is a directory, not a shared library", path)}
	}
	return nil
}

// Initialize is a no-op in the OS provider because the actual C_Initialize
// call requires a cgo binding. Returns a descriptive error so the validator
// surfaces a clear message rather than a nil-pointer panic.
func (p *OsPkcs11Provider) Initialize() error {
	return &Error{
		Op:  "pkcs11",
		Msg: "C_Initialize is not available: the OS PKCS#11 provider requires a cgo binding (e.g. github.com/miekg/pkcs11); use a mock provider for testing",
	}
}

// GetSlotList is not implemented in the OS provider.
func (p *OsPkcs11Provider) GetSlotList(_ bool) ([]uint64, error) {
	return nil, &Error{Op: "pkcs11", Msg: "GetSlotList not implemented in OS provider"}
}

// GetTokenInfo is not implemented in the OS provider.
func (p *OsPkcs11Provider) GetTokenInfo(_ uint64) (string, error) {
	return "", &Error{Op: "pkcs11", Msg: "GetTokenInfo not implemented in OS provider"}
}

// OpenSession is not implemented in the OS provider.
func (p *OsPkcs11Provider) OpenSession(_ uint64) (uint64, error) {
	return 0, &Error{Op: "pkcs11", Msg: "OpenSession not implemented in OS provider"}
}

// Login is not implemented in the OS provider.
func (p *OsPkcs11Provider) Login(_ uint64, _ string) error {
	return &Error{Op: "pkcs11", Msg: "Login not implemented in OS provider"}
}

// FindKey is not implemented in the OS provider.
func (p *OsPkcs11Provider) FindKey(_ uint64, _, _ string) (uint64, error) {
	return 0, &Error{Op: "pkcs11", Msg: "FindKey not implemented in OS provider"}
}

// SignTest is not implemented in the OS provider.
func (p *OsPkcs11Provider) SignTest(_ uint64, _ uint64, _ []byte) error {
	return &Error{Op: "pkcs11", Msg: "SignTest not implemented in OS provider"}
}

// CloseSession is a no-op in the OS provider.
func (p *OsPkcs11Provider) CloseSession(_ uint64) error { return nil }

// Finalize is a no-op in the OS provider.
func (p *OsPkcs11Provider) Finalize() error { return nil }
