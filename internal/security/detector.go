// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package security

import (
	"encoding/base64"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/dotandev/glassbox/internal/abi"
	"github.com/stellar/go-stellar-sdk/xdr"
)

// Severity levels for security findings
type Severity string

const (
	SeverityHigh   Severity = "HIGH"
	SeverityMedium Severity = "MEDIUM"
	SeverityLow    Severity = "LOW"
	SeverityInfo   Severity = "INFO"
)

// FindingType categorizes the security issue
type FindingType string

const (
	FindingVerifiedRisk  FindingType = "VERIFIED_RISK"
	FindingHeuristicWarn FindingType = "HEURISTIC_WARNING"
)

// Finding represents a security vulnerability or warning
type Finding struct {
	Type        FindingType `json:"type"`
	Severity    Severity    `json:"severity"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Evidence    string      `json:"evidence,omitempty"`
}

// Detector analyzes transactions for security vulnerabilities
type Detector struct {
	findings []Finding
}

// NewDetector creates a new security detector
func NewDetector() *Detector {
	return &Detector{
		findings: make([]Finding, 0),
	}
}

// Analyze performs security checks on transaction data
func (d *Detector) Analyze(envelopeXdr, resultMetaXdr string, events []string, logs []string) []Finding {
	d.findings = make([]Finding, 0)

	// Decode envelope
	envelope, err := decodeEnvelope(envelopeXdr)
	if err == nil {
		d.checkLargeValueTransfers(envelope)
		d.checkReentrancyPatterns(envelope, events)
	}

	// Check patterns that don't require envelope
	d.checkIntegerOverflow(events, logs)
	d.checkSuspiciousEvents(events)
	d.checkAuthorizationBypass(events, logs)

	return d.findings
}

// ScanMetadata applies heuristic vulnerability rules to ABI metadata and source
// text. It is intentionally conservative: findings are warnings unless the
// source explicitly contains a risky execution signal such as panic/unwrap.
func (d *Detector) ScanMetadata(spec *abi.ContractSpec, sourceFiles map[string]string) []Finding {
	d.findings = make([]Finding, 0)

	if spec != nil {
		for _, fn := range spec.Functions {
			name := strings.ToLower(string(fn.Name))
			if isPrivilegedName(name) && !functionMentionsAuth(sourceFiles, name) {
				d.addFinding(Finding{
					Type:        FindingHeuristicWarn,
					Severity:    SeverityHigh,
					Title:       "Privileged ABI Function Without Visible Auth",
					Description: fmt.Sprintf("Function %q looks privileged, but no require_auth/check_auth call was found in supplied source metadata.", string(fn.Name)),
					Evidence:    string(fn.Name),
				})
			}
			if strings.Contains(name, "upgrade") || strings.Contains(name, "deploy") {
				d.addFinding(Finding{
					Type:        FindingHeuristicWarn,
					Severity:    SeverityMedium,
					Title:       "Upgradeable Contract Surface",
					Description: "Deployment or upgrade functions should be reviewed for strict authorization and governance controls.",
					Evidence:    string(fn.Name),
				})
			}
		}
	}

	d.checkSourcePatterns(sourceFiles)
	return d.findings
}

// ScanSourcePath reads a source file or directory and scans common contract
// source files for vulnerability heuristics.
func (d *Detector) ScanSourcePath(path string, spec *abi.ContractSpec) ([]Finding, error) {
	files := make(map[string]string)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		files[path] = string(data)
		return d.ScanMetadata(spec, files), nil
	}
	err = filepath.WalkDir(path, func(p string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == "target" || name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if ext != ".rs" && ext != ".ts" && ext != ".js" && ext != ".json" {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[p] = string(data)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return d.ScanMetadata(spec, files), nil
}

// GetFindings returns all detected findings
func (d *Detector) GetFindings() []Finding {
	return d.findings
}

func (d *Detector) addFinding(finding Finding) {
	d.findings = append(d.findings, finding)
}

// checkLargeValueTransfers detects unusually large value transfers
func (d *Detector) checkLargeValueTransfers(envelope xdr.TransactionEnvelope) {
	const largeTransferThreshold = 1000000 * 10000000 // 1M XLM in stroops

	ops := extractOperations(envelope)
	for _, op := range ops {
		switch op.Body.Type {
		case xdr.OperationTypePayment:
			payment := op.Body.PaymentOp
			if payment.Amount > xdr.Int64(largeTransferThreshold) {
				d.addFinding(Finding{
					Type:        FindingHeuristicWarn,
					Severity:    SeverityHigh,
					Title:       "Large Value Transfer Detected",
					Description: fmt.Sprintf("Transfer of %d stroops (%.2f XLM) detected. Verify recipient address.", payment.Amount, float64(payment.Amount)/10000000.0),
					Evidence:    fmt.Sprintf("Destination: %s", payment.Destination.Address()),
				})
			}
		case xdr.OperationTypeInvokeHostFunction:
			// Check for large amounts in contract invocations
			d.checkContractValueTransfer(op)
		}
	}
}

// checkContractValueTransfer analyzes contract invocations for large transfers
func (d *Detector) checkContractValueTransfer(op xdr.Operation) {
	hostFn := op.Body.InvokeHostFunctionOp
	if hostFn == nil {
		return
	}

	if hostFn.HostFunction.Type == xdr.HostFunctionTypeHostFunctionTypeInvokeContract {
		invokeArgs := hostFn.HostFunction.InvokeContract
		if invokeArgs == nil {
			return
		}

		// Look for amount parameters (common in transfer functions)
		for _, arg := range invokeArgs.Args {
			if arg.Type == xdr.ScValTypeScvI128 || arg.Type == xdr.ScValTypeScvU128 {
				amount := extractAmount(arg)
				if amount != nil && amount.Cmp(big.NewInt(100000000000000)) > 0 { // 10M tokens (assuming 7 decimals)
					d.addFinding(Finding{
						Type:        FindingHeuristicWarn,
						Severity:    SeverityMedium,
						Title:       "Large Contract Value Transfer",
						Description: fmt.Sprintf("Contract invocation with large amount: %s", amount.String()),
						Evidence:    "Review contract address and function parameters",
					})
				}
			}
		}
	}
}

// checkReentrancyPatterns detects potential reentrancy vulnerabilities
func (d *Detector) checkReentrancyPatterns(envelope xdr.TransactionEnvelope, events []string) {
	ops := extractOperations(envelope)

	// Count contract invocations
	invocationCount := 0
	for _, op := range ops {
		if op.Body.Type == xdr.OperationTypeInvokeHostFunction {
			invocationCount++
		}
	}

	// Multiple invocations + state changes = potential reentrancy
	if invocationCount > 1 {
		hasStateChange := false
		for _, event := range events {
			if strings.Contains(event, "contract_data") || strings.Contains(event, "write") {
				hasStateChange = true
				break
			}
		}

		if hasStateChange {
			d.addFinding(Finding{
				Type:        FindingHeuristicWarn,
				Severity:    SeverityMedium,
				Title:       "Potential Reentrancy Pattern",
				Description: fmt.Sprintf("Transaction contains %d contract invocations with state changes. Verify reentrancy guards are in place.", invocationCount),
				Evidence:    "Multiple contract calls with storage modifications detected",
			})
		}
	}
}

// checkIntegerOverflow detects potential integer overflow issues
func (d *Detector) checkIntegerOverflow(events []string, logs []string) {
	overflowKeywords := []string{"overflow", "underflow"}
	arithmeticKeywords := []string{"checked_add", "checked_sub", "checked_mul", "checked_div", "arithmetic"}

	for _, log := range logs {
		logLower := strings.ToLower(log)

		// Check for explicit overflow/underflow mentions
		for _, keyword := range overflowKeywords {
			if strings.Contains(logLower, keyword) {
				d.addFinding(Finding{
					Type:        FindingVerifiedRisk,
					Severity:    SeverityHigh,
					Title:       "Integer Overflow/Underflow Detected",
					Description: "Arithmetic operation failed, indicating potential overflow or underflow",
					Evidence:    log,
				})
				return
			}
		}

		// Check for arithmetic operation failures
		for _, keyword := range arithmeticKeywords {
			if strings.Contains(logLower, keyword) && (strings.Contains(logLower, "fail") || strings.Contains(logLower, "error")) {
				d.addFinding(Finding{
					Type:        FindingVerifiedRisk,
					Severity:    SeverityHigh,
					Title:       "Integer Overflow/Underflow Detected",
					Description: "Arithmetic operation failed, indicating potential overflow or underflow",
					Evidence:    log,
				})
				return
			}
		}
	}
}

// checkSuspiciousEvents analyzes diagnostic events for suspicious patterns
func (d *Detector) checkSuspiciousEvents(events []string) {
	for _, event := range events {
		eventLower := strings.ToLower(event)

		// Check for authorization failures
		if strings.Contains(eventLower, "auth") && (strings.Contains(eventLower, "fail") || strings.Contains(eventLower, "invalid")) {
			d.addFinding(Finding{
				Type:        FindingVerifiedRisk,
				Severity:    SeverityHigh,
				Title:       "Authorization Failure",
				Description: "Contract authorization check failed",
				Evidence:    event,
			})
		}

		// Check for panic/trap events
		if strings.Contains(eventLower, "panic") || strings.Contains(eventLower, "trap") {
			d.addFinding(Finding{
				Type:        FindingVerifiedRisk,
				Severity:    SeverityHigh,
				Title:       "Contract Panic/Trap",
				Description: "Contract execution panicked or trapped",
				Evidence:    event,
			})
		}
	}
}

// checkAuthorizationBypass detects potential authorization bypass attempts
func (d *Detector) checkAuthorizationBypass(events []string, logs []string) {
	hasAuthCheck := false
	hasPrivilegedOp := false

	for _, log := range logs {
		logLower := strings.ToLower(log)
		if strings.Contains(logLower, "require_auth") || strings.Contains(logLower, "check_auth") {
			hasAuthCheck = true
		}
		if strings.Contains(logLower, "admin") || strings.Contains(logLower, "owner") || strings.Contains(logLower, "privileged") {
			hasPrivilegedOp = true
		}
	}

	// Privileged operation without auth check
	if hasPrivilegedOp && !hasAuthCheck {
		d.addFinding(Finding{
			Type:        FindingHeuristicWarn,
			Severity:    SeverityHigh,
			Title:       "Potential Authorization Bypass",
			Description: "Privileged operation detected without corresponding authorization check",
			Evidence:    "Review contract authorization logic",
		})
	}
}

func (d *Detector) checkSourcePatterns(files map[string]string) {
	for path, content := range files {
		lower := strings.ToLower(content)
		if strings.Contains(lower, "unwrap()") || strings.Contains(lower, "expect(") || strings.Contains(lower, "panic!") {
			d.addFinding(Finding{
				Type:        FindingVerifiedRisk,
				Severity:    SeverityHigh,
				Title:       "Panic-Prone Source Pattern",
				Description: "Source contains unwrap/expect/panic patterns that can trap contract execution.",
				Evidence:    path,
			})
		}
		if strings.Contains(lower, "env.storage().persistent().set") && !strings.Contains(lower, "require_auth") {
			d.addFinding(Finding{
				Type:        FindingHeuristicWarn,
				Severity:    SeverityHigh,
				Title:       "Persistent Storage Write Without Visible Auth",
				Description: "Persistent storage writes should normally be guarded by an authorization check.",
				Evidence:    path,
			})
		}
		if strings.Contains(lower, "prng") || strings.Contains(lower, "random") {
			d.addFinding(Finding{
				Type:        FindingHeuristicWarn,
				Severity:    SeverityMedium,
				Title:       "Randomness Usage",
				Description: "Randomness-dependent contract logic should be reviewed for predictability and replay assumptions.",
				Evidence:    path,
			})
		}
	}
}

func isPrivilegedName(name string) bool {
	return strings.Contains(name, "admin") || strings.Contains(name, "owner") ||
		strings.Contains(name, "upgrade") || strings.Contains(name, "pause") ||
		strings.Contains(name, "mint") || strings.Contains(name, "burn")
}

func functionMentionsAuth(files map[string]string, name string) bool {
	for _, content := range files {
		lower := strings.ToLower(content)
		if strings.Contains(lower, name) && (strings.Contains(lower, "require_auth") || strings.Contains(lower, "check_auth")) {
			return true
		}
	}
	return false
}

// Helper functions

func decodeEnvelope(envelopeXdr string) (xdr.TransactionEnvelope, error) {
	var envelope xdr.TransactionEnvelope
	decoded, err := base64.StdEncoding.DecodeString(envelopeXdr)
	if err != nil {
		return envelope, err
	}
	_, err = xdr.Unmarshal(strings.NewReader(string(decoded)), &envelope)
	return envelope, err
}

func extractOperations(envelope xdr.TransactionEnvelope) []xdr.Operation {
	switch envelope.Type {
	case xdr.EnvelopeTypeEnvelopeTypeTx:
		return envelope.V1.Tx.Operations
	case xdr.EnvelopeTypeEnvelopeTypeTxV0:
		return envelope.V0.Tx.Operations
	case xdr.EnvelopeTypeEnvelopeTypeTxFeeBump:
		return envelope.FeeBump.Tx.InnerTx.V1.Tx.Operations
	}
	return nil
}

func extractAmount(val xdr.ScVal) *big.Int {
	switch val.Type {
	case xdr.ScValTypeScvI128:
		parts := val.I128
		if parts == nil {
			return nil
		}
		amount := new(big.Int).SetInt64(int64(parts.Hi))
		amount.Lsh(amount, 64)
		amount.Add(amount, new(big.Int).SetUint64(uint64(parts.Lo)))
		return amount
	case xdr.ScValTypeScvU128:
		parts := val.U128
		if parts == nil {
			return nil
		}
		amount := new(big.Int).SetUint64(uint64(parts.Hi))
		amount.Lsh(amount, 64)
		amount.Add(amount, new(big.Int).SetUint64(uint64(parts.Lo)))
		return amount
	}
	return nil
}
