// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/dotandev/glassbox/internal/errors"
	"github.com/dotandev/glassbox/internal/signer"
	"github.com/spf13/cobra"
)

var (
	auditSignPayload      string
	auditSignPayloadFile  string
	auditSignSoftwareKey  string
	auditSignHSMProvider  string
	auditSignValidateOnly bool
)

// SignedAuditLog is the JSON output of the audit:sign command.
type SignedAuditLog struct {
	Version   string          `json:"version"`
	Timestamp time.Time       `json:"timestamp"`
	TraceHash string          `json:"trace_hash"`
	Signature string          `json:"signature"`
	PublicKey string          `json:"public_key"`
	Payload   json.RawMessage `json:"payload"`
}

var auditSignCmd = &cobra.Command{
	Use:     "audit:sign",
	GroupID: "utility",
	Short:   "Generate a deterministic signed audit log from a JSON payload",
	Long: `Generate a deterministic signed audit log from a JSON payload.

The payload can be supplied as a string via --payload, as a file via --payload-file,
or piped on stdin. Use --software-private-key for PEM-based Ed25519 signing or
--hsm-provider pkcs11 for PKCS#11 signing.

Providers:
  software   Ed25519 key in memory. Requires --software-private-key (PKCS#8 PEM)
             or GLASSBOX_AUDIT_PRIVATE_KEY_PEM.
  pkcs11     PKCS#11 hardware security module. Requires:
               GLASSBOX_PKCS11_MODULE   path to the .so/.dylib module
               GLASSBOX_PKCS11_PIN      user PIN
               GLASSBOX_PKCS11_KEY_LABEL or GLASSBOX_PKCS11_KEY_ID (hex)
             Optional:
               GLASSBOX_PKCS11_TOKEN_LABEL  select token by label
               GLASSBOX_PKCS11_SLOT         select slot by index (default 0)

Use --validate-only with --hsm-provider pkcs11 to run a preflight check of the
PKCS#11 configuration without signing any payload. This checks module loading,
slot enumeration, PIN authentication, and key lookup.

Examples:
  # Software signing
  glassbox audit:sign \
    --payload '{"input":{},"state":{},"events":[],"timestamp":"2026-01-01T00:00:00.000Z"}' \
    --software-private-key "$(cat ./ed25519-private-key.pem)"

  # PKCS#11 signing
  glassbox audit:sign --payload-file payload.json --hsm-provider pkcs11

  # Validate PKCS#11 configuration without signing
  glassbox audit:sign --hsm-provider pkcs11 --validate-only`,
	Args: cobra.NoArgs,
	RunE: runAuditSign,
}

func init() {
	auditSignCmd.Flags().StringVar(&auditSignPayload, "payload", "", "JSON payload to sign")
	auditSignCmd.Flags().StringVar(&auditSignPayloadFile, "payload-file", "", "Path to JSON payload file")
	auditSignCmd.Flags().StringVar(&auditSignSoftwareKey, "software-private-key", "", "PKCS#8 PEM Ed25519 private key for software signing")
	auditSignCmd.Flags().StringVar(&auditSignHSMProvider, "hsm-provider", "", "HSM provider to use for signing (pkcs11)")
	auditSignCmd.Flags().BoolVar(&auditSignValidateOnly, "validate-only", false, "Run PKCS#11 preflight checks without signing (requires --hsm-provider pkcs11)")

	rootCmd.AddCommand(auditSignCmd)
}

func runAuditSign(cmd *cobra.Command, args []string) error {
	// --validate-only: run PKCS#11 preflight checks and exit without signing.
	if auditSignValidateOnly {
		return runPkcs11Preflight(cmd)
	}

	if auditSignPayload != "" && auditSignPayloadFile != "" {
		return errors.WrapValidationError("only one of --payload or --payload-file may be provided")
	}

	payloadBytes, err := readAuditPayload(auditSignPayload, auditSignPayloadFile)
	if err != nil {
		return err
	}

	if len(strings.TrimSpace(string(payloadBytes))) == 0 {
		return errors.WrapValidationError("payload is required")
	}

	var payload interface{}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return errors.WrapValidationError(fmt.Sprintf("invalid JSON payload: %v", err))
	}

	canonicalPayload, err := marshalCanonical(payload)
	if err != nil {
		return errors.WrapMarshalFailed(err)
	}

	signerImpl, err := resolveAuditSigner()
	if err != nil {
		return err
	}
	defer func() {
		if closer, ok := signerImpl.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}()

	hash := sha256.Sum256(canonicalPayload)
	signature, err := signerImpl.Sign(hash[:])
	if err != nil {
		return errors.WrapValidationError(fmt.Sprintf("signing failed: %v", err))
	}

	publicKey, err := signerImpl.PublicKey()
	if err != nil {
		return errors.WrapValidationError(fmt.Sprintf("failed to retrieve public key: %v", err))
	}

	auditLog := SignedAuditLog{
		Version:   "1.0.0",
		Timestamp: time.Now().UTC(),
		TraceHash: hex.EncodeToString(hash[:]),
		Signature: hex.EncodeToString(signature),
		PublicKey: hex.EncodeToString(publicKey),
		Payload:   json.RawMessage(payloadBytes),
	}

	output, err := json.MarshalIndent(auditLog, "", "  ")
	if err != nil {
		return errors.WrapMarshalFailed(err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), string(output))
	return nil
}

func readAuditPayload(payload, payloadFile string) ([]byte, error) {
	if payloadFile != "" {
		data, err := os.ReadFile(payloadFile)
		if err != nil {
			return nil, errors.WrapValidationError(fmt.Sprintf("failed to read payload file: %v", err))
		}
		return data, nil
	}

	if payload != "" {
		return []byte(payload), nil
	}

	stat, err := os.Stdin.Stat()
	if err != nil {
		return nil, errors.WrapValidationError(fmt.Sprintf("failed to inspect stdin: %v", err))
	}

	if stat.Mode()&os.ModeCharDevice == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, errors.WrapValidationError(fmt.Sprintf("failed to read payload from stdin: %v", err))
		}
		return data, nil
	}

	return nil, nil
}

func resolveAuditSigner() (signer.Signer, error) {
	if strings.EqualFold(auditSignHSMProvider, "pkcs11") {
		cfg, err := signer.Pkcs11ConfigFromEnv()
		if err != nil {
			return nil, err
		}
		return signer.NewPkcs11Signer(*cfg)
	}

	if auditSignHSMProvider != "" {
		return nil, errors.WrapValidationError(fmt.Sprintf("unsupported hsm provider: %s", auditSignHSMProvider))
	}

	keyPEM := auditSignSoftwareKey
	if keyPEM == "" {
		keyPEM = os.Getenv("GLASSBOX_AUDIT_PRIVATE_KEY_PEM")
	}

	if keyPEM == "" {
		if strings.EqualFold(os.Getenv("GLASSBOX_SIGNER_TYPE"), "pkcs11") {
			return signer.NewFromEnv()
		}
		return nil, errors.WrapCliArgumentRequired("software-private-key or GLASSBOX_AUDIT_PRIVATE_KEY_PEM")
	}

	if !strings.Contains(keyPEM, "-----BEGIN") {
		if fileBytes, err := os.ReadFile(keyPEM); err == nil {
			keyPEM = string(fileBytes)
		}
	}

	return signer.NewInMemorySignerFromPEM(keyPEM)
}

// runPkcs11Preflight executes the PKCS#11 preflight validator and prints a
// human-readable report. It exits with a non-zero status if any check fails.
func runPkcs11Preflight(cmd *cobra.Command) error {
	if !strings.EqualFold(auditSignHSMProvider, "pkcs11") {
		return errors.WrapValidationError("--validate-only requires --hsm-provider pkcs11")
	}

	cfg, err := signer.Pkcs11ConfigFromEnv()
	if err != nil {
		return err
	}

	vcfg := signer.DefaultValidatorConfig()
	validator := signer.NewPkcs11Validator(*cfg, vcfg, &signer.OsPkcs11Provider{})

	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Running PKCS#11 preflight checks...")
	fmt.Fprintln(out)

	report := validator.Validate(context.Background())

	for _, r := range report.Results {
		if r.OK {
			fmt.Fprintf(out, "  [PASS] %-14s %s\n", r.Step, r.Message)
		} else {
			fmt.Fprintf(out, "  [FAIL] %-14s %s\n", r.Step, r.Message)
			fmt.Fprintf(out, "         %-14s Remediation: %s\n", "", r.Remediation)
		}
	}

	fmt.Fprintln(out)
	if report.Ready {
		fmt.Fprintln(out, "Result: PKCS#11 configuration is valid and ready for signing.")
		return nil
	}
	return errors.WrapValidationError("PKCS#11 preflight checks failed; review the output above for remediation steps")
}

