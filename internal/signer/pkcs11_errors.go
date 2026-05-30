// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package signer

import "fmt"

// PKCS#11 return value constants from the Cryptoki specification (PKCS#11 v2.40).
// These are the raw CK_RV values returned by HSM modules.
const (
	CKR_OK                          uint64 = 0x00000000
	CKR_CANCEL                      uint64 = 0x00000001
	CKR_HOST_MEMORY                 uint64 = 0x00000002
	CKR_SLOT_ID_INVALID             uint64 = 0x00000003
	CKR_GENERAL_ERROR               uint64 = 0x00000005
	CKR_FUNCTION_FAILED             uint64 = 0x00000006
	CKR_ARGUMENTS_BAD               uint64 = 0x00000007
	CKR_NO_EVENT                    uint64 = 0x00000008
	CKR_NEED_TO_CREATE_THREADS      uint64 = 0x00000009
	CKR_CANT_LOCK                   uint64 = 0x0000000A
	CKR_ATTRIBUTE_READ_ONLY         uint64 = 0x00000010
	CKR_ATTRIBUTE_SENSITIVE         uint64 = 0x00000011
	CKR_ATTRIBUTE_TYPE_INVALID      uint64 = 0x00000012
	CKR_ATTRIBUTE_VALUE_INVALID     uint64 = 0x00000013
	CKR_DATA_INVALID                uint64 = 0x00000020
	CKR_DATA_LEN_RANGE              uint64 = 0x00000021
	CKR_DEVICE_ERROR                uint64 = 0x00000030
	CKR_DEVICE_MEMORY               uint64 = 0x00000031
	CKR_DEVICE_REMOVED              uint64 = 0x00000032
	CKR_ENCRYPTED_DATA_INVALID      uint64 = 0x00000040
	CKR_ENCRYPTED_DATA_LEN_RANGE    uint64 = 0x00000041
	CKR_FUNCTION_CANCELED           uint64 = 0x00000050
	CKR_FUNCTION_NOT_PARALLEL       uint64 = 0x00000051
	CKR_FUNCTION_NOT_SUPPORTED      uint64 = 0x00000054
	CKR_KEY_HANDLE_INVALID          uint64 = 0x00000060
	CKR_KEY_SIZE_RANGE              uint64 = 0x00000062
	CKR_KEY_TYPE_INCONSISTENT       uint64 = 0x00000063
	CKR_KEY_NOT_NEEDED              uint64 = 0x00000064
	CKR_KEY_CHANGED                 uint64 = 0x00000065
	CKR_KEY_NEEDED                  uint64 = 0x00000066
	CKR_KEY_INDIGESTIBLE            uint64 = 0x00000067
	CKR_KEY_FUNCTION_NOT_PERMITTED  uint64 = 0x00000068
	CKR_KEY_NOT_WRAPPABLE           uint64 = 0x00000069
	CKR_KEY_UNEXTRACTABLE           uint64 = 0x0000006A
	CKR_MECHANISM_INVALID           uint64 = 0x00000070
	CKR_MECHANISM_PARAM_INVALID     uint64 = 0x00000071
	CKR_OBJECT_HANDLE_INVALID       uint64 = 0x00000082
	CKR_OPERATION_ACTIVE            uint64 = 0x00000090
	CKR_OPERATION_NOT_INITIALIZED   uint64 = 0x00000091
	CKR_PIN_INCORRECT               uint64 = 0x000000A0
	CKR_PIN_INVALID                 uint64 = 0x000000A1
	CKR_PIN_LEN_RANGE               uint64 = 0x000000A2
	CKR_PIN_EXPIRED                 uint64 = 0x000000A3
	CKR_PIN_LOCKED                  uint64 = 0x000000A4
	CKR_SESSION_CLOSED              uint64 = 0x000000B0
	CKR_SESSION_COUNT               uint64 = 0x000000B1
	CKR_SESSION_HANDLE_INVALID      uint64 = 0x000000B3
	CKR_SESSION_PARALLEL_NOT_SUP    uint64 = 0x000000B4
	CKR_SESSION_READ_ONLY           uint64 = 0x000000B5
	CKR_SESSION_EXISTS              uint64 = 0x000000B6
	CKR_SESSION_READ_ONLY_EXISTS    uint64 = 0x000000B7
	CKR_SESSION_READ_WRITE_SO_EXISTS uint64 = 0x000000B8
	CKR_SIGNATURE_INVALID           uint64 = 0x000000C0
	CKR_SIGNATURE_LEN_RANGE         uint64 = 0x000000C1
	CKR_TEMPLATE_INCOMPLETE         uint64 = 0x000000D0
	CKR_TEMPLATE_INCONSISTENT       uint64 = 0x000000D1
	CKR_TOKEN_NOT_PRESENT           uint64 = 0x000000E0
	CKR_TOKEN_NOT_RECOGNIZED        uint64 = 0x000000E1
	CKR_TOKEN_WRITE_PROTECTED       uint64 = 0x000000E2
	CKR_UNWRAPPING_KEY_HANDLE_INVALID uint64 = 0x000000F0
	CKR_UNWRAPPING_KEY_SIZE_RANGE   uint64 = 0x000000F1
	CKR_UNWRAPPING_KEY_TYPE_INCONSISTENT uint64 = 0x000000F2
	CKR_USER_ALREADY_LOGGED_IN      uint64 = 0x00000100
	CKR_USER_NOT_LOGGED_IN          uint64 = 0x00000101
	CKR_USER_PIN_NOT_INITIALIZED    uint64 = 0x00000102
	CKR_USER_TYPE_INVALID           uint64 = 0x00000103
	CKR_USER_ANOTHER_ALREADY_LOGGED_IN uint64 = 0x00000104
	CKR_USER_TOO_MANY_TYPES         uint64 = 0x00000105
	CKR_WRAPPED_KEY_INVALID         uint64 = 0x00000110
	CKR_WRAPPED_KEY_LEN_RANGE       uint64 = 0x00000112
	CKR_WRAPPING_KEY_HANDLE_INVALID uint64 = 0x00000113
	CKR_WRAPPING_KEY_SIZE_RANGE     uint64 = 0x00000114
	CKR_WRAPPING_KEY_TYPE_INCONSISTENT uint64 = 0x00000115
	CKR_RANDOM_SEED_NOT_SUPPORTED   uint64 = 0x00000120
	CKR_RANDOM_NO_RNG               uint64 = 0x00000121
	CKR_DOMAIN_PARAMS_INVALID       uint64 = 0x00000130
	CKR_BUFFER_TOO_SMALL            uint64 = 0x00000150
	CKR_SAVED_STATE_INVALID         uint64 = 0x00000160
	CKR_INFORMATION_SENSITIVE       uint64 = 0x00000170
	CKR_STATE_UNSAVEABLE            uint64 = 0x00000180
	CKR_CRYPTOKI_NOT_INITIALIZED    uint64 = 0x00000190
	CKR_CRYPTOKI_ALREADY_INITIALIZED uint64 = 0x00000191
	CKR_MUTEX_BAD                   uint64 = 0x000001A0
	CKR_MUTEX_NOT_LOCKED            uint64 = 0x000001A1
	CKR_VENDOR_DEFINED              uint64 = 0x80000000
)

// pkcs11ErrorEntry describes a PKCS#11 error with a human-readable message
// and an actionable remediation hint.
type pkcs11ErrorEntry struct {
	Message     string
	Remediation string
}

// pkcs11ErrorTable maps CK_RV codes to user-friendly descriptions and
// remediation hints. Entries cover the most common failure modes encountered
// in HSM integration workflows.
var pkcs11ErrorTable = map[uint64]pkcs11ErrorEntry{
	CKR_SLOT_ID_INVALID: {
		Message:     "the specified slot ID does not exist on this module",
		Remediation: "check GLASSBOX_PKCS11_SLOT; run 'pkcs11-tool --list-slots' to see available slots",
	},
	CKR_GENERAL_ERROR: {
		Message:     "the HSM module reported a general error",
		Remediation: "check HSM device connectivity and module logs; try reinitializing the token",
	},
	CKR_FUNCTION_FAILED: {
		Message:     "the requested PKCS#11 function failed",
		Remediation: "verify the key type and mechanism are supported by this module; check HSM firmware version",
	},
	CKR_ARGUMENTS_BAD: {
		Message:     "invalid arguments passed to the PKCS#11 module",
		Remediation: "verify key label, key ID, and slot configuration are correct",
	},
	CKR_DEVICE_ERROR: {
		Message:     "the HSM device reported an internal error",
		Remediation: "check physical device connection; try unplugging and reinserting the token",
	},
	CKR_DEVICE_MEMORY: {
		Message:     "the HSM device is out of memory",
		Remediation: "delete unused keys or objects from the token to free space",
	},
	CKR_DEVICE_REMOVED: {
		Message:     "the HSM device was removed during the operation",
		Remediation: "reinsert the token and retry; ensure the device is firmly connected",
	},
	CKR_FUNCTION_NOT_SUPPORTED: {
		Message:     "the requested function is not supported by this PKCS#11 module",
		Remediation: "verify the module supports Ed25519 (CKM_EDDSA); check module documentation for supported mechanisms",
	},
	CKR_KEY_HANDLE_INVALID: {
		Message:     "the key handle is no longer valid",
		Remediation: "the session may have been closed; reinitialize the signer",
	},
	CKR_KEY_TYPE_INCONSISTENT: {
		Message:     "the key type is inconsistent with the requested mechanism",
		Remediation: "ensure the key at GLASSBOX_PKCS11_KEY_LABEL is an Ed25519 key; RSA and ECDSA keys are not supported",
	},
	CKR_KEY_FUNCTION_NOT_PERMITTED: {
		Message:     "the key does not have the sign permission",
		Remediation: "the key was created without CKA_SIGN=true; recreate the key with signing permissions enabled",
	},
	CKR_MECHANISM_INVALID: {
		Message:     "the CKM_EDDSA mechanism is not supported by this module",
		Remediation: "verify the module supports Ed25519 signing; SoftHSM2 requires version 2.5.0+; YubiKey requires firmware 5.2.3+",
	},
	CKR_MECHANISM_PARAM_INVALID: {
		Message:     "invalid mechanism parameters for the signing operation",
		Remediation: "Ed25519 signing requires no mechanism parameters; check for module-specific quirks",
	},
	CKR_OBJECT_HANDLE_INVALID: {
		Message:     "the key object handle is invalid",
		Remediation: "the key may have been deleted or the session expired; verify GLASSBOX_PKCS11_KEY_LABEL exists on the token",
	},
	CKR_PIN_INCORRECT: {
		Message:     "the PIN is incorrect",
		Remediation: "verify GLASSBOX_PKCS11_PIN is correct; note that repeated failures may lock the token",
	},
	CKR_PIN_INVALID: {
		Message:     "the PIN contains invalid characters",
		Remediation: "check GLASSBOX_PKCS11_PIN for non-printable characters or encoding issues",
	},
	CKR_PIN_LEN_RANGE: {
		Message:     "the PIN length is outside the allowed range for this token",
		Remediation: "check the token's minimum and maximum PIN length requirements",
	},
	CKR_PIN_EXPIRED: {
		Message:     "the PIN has expired and must be changed",
		Remediation: "use 'pkcs11-tool --change-pin' or the HSM management utility to set a new PIN",
	},
	CKR_PIN_LOCKED: {
		Message:     "the token PIN is locked due to too many incorrect attempts",
		Remediation: "use the SO (Security Officer) PIN to unlock the token; for YubiKey use 'ykman piv access change-puk'",
	},
	CKR_SESSION_CLOSED: {
		Message:     "the PKCS#11 session was closed unexpectedly",
		Remediation: "reinitialize the signer; check for concurrent access or token removal",
	},
	CKR_SESSION_HANDLE_INVALID: {
		Message:     "the session handle is no longer valid",
		Remediation: "the token may have been removed or reset; reinitialize the signer",
	},
	CKR_TOKEN_NOT_PRESENT: {
		Message:     "no token is present in the slot",
		Remediation: "insert the HSM token; for SoftHSM2 verify the token was initialized with 'softhsm2-util --init-token'",
	},
	CKR_TOKEN_NOT_RECOGNIZED: {
		Message:     "the token is not recognized by this module",
		Remediation: "verify the token was initialized with this module; check for firmware compatibility",
	},
	CKR_TOKEN_WRITE_PROTECTED: {
		Message:     "the token is write-protected",
		Remediation: "the token is in read-only mode; check hardware write-protect switch or token flags",
	},
	CKR_USER_ALREADY_LOGGED_IN: {
		Message:     "a user is already logged in to this session",
		Remediation: "close existing sessions before opening a new one; check for concurrent signer instances",
	},
	CKR_USER_NOT_LOGGED_IN: {
		Message:     "the user is not logged in",
		Remediation: "ensure C_Login is called before signing; verify GLASSBOX_PKCS11_PIN is set",
	},
	CKR_USER_PIN_NOT_INITIALIZED: {
		Message:     "the user PIN has not been initialized on this token",
		Remediation: "initialize the token with 'pkcs11-tool --init-token --init-pin' or 'softhsm2-util --init-token'",
	},
	CKR_CRYPTOKI_NOT_INITIALIZED: {
		Message:     "the PKCS#11 library has not been initialized",
		Remediation: "C_Initialize must be called before any other PKCS#11 function; this is a library integration bug",
	},
	CKR_CRYPTOKI_ALREADY_INITIALIZED: {
		Message:     "the PKCS#11 library is already initialized",
		Remediation: "only one initialization per process is allowed; check for duplicate signer instances",
	},
	CKR_BUFFER_TOO_SMALL: {
		Message:     "the output buffer is too small for the result",
		Remediation: "this is an internal error; please report it with the module name and version",
	},
}

// Pkcs11Error wraps a raw PKCS#11 return value with a human-readable
// message and an actionable remediation hint.
type Pkcs11Error struct {
	// Op is the PKCS#11 function that failed (e.g. "C_Login", "C_Sign").
	Op string
	// RV is the raw CK_RV return value from the module.
	RV uint64
	// Message is the human-readable description of the error.
	Message string
	// Remediation is an actionable hint for resolving the error.
	Remediation string
}

func (e *Pkcs11Error) Error() string {
	return fmt.Sprintf("pkcs11 %s failed (0x%08X): %s — %s", e.Op, e.RV, e.Message, e.Remediation)
}

// MapPkcs11Error converts a raw CK_RV return value into a Pkcs11Error with
// a human-readable message and remediation hint. If the code is not in the
// table, a generic vendor-error message is returned.
func MapPkcs11Error(op string, rv uint64) *Pkcs11Error {
	if rv == CKR_OK {
		return nil
	}

	if entry, ok := pkcs11ErrorTable[rv]; ok {
		return &Pkcs11Error{
			Op:          op,
			RV:          rv,
			Message:     entry.Message,
			Remediation: entry.Remediation,
		}
	}

	// Vendor-defined or unknown error code.
	msg := "unknown PKCS#11 error"
	remediation := "consult the HSM vendor documentation for error code 0x" + fmt.Sprintf("%08X", rv)
	if rv >= CKR_VENDOR_DEFINED {
		msg = "vendor-defined error"
		remediation = "consult the HSM vendor documentation for vendor error code 0x" + fmt.Sprintf("%08X", rv)
	}

	return &Pkcs11Error{
		Op:          op,
		RV:          rv,
		Message:     msg,
		Remediation: remediation,
	}
}
