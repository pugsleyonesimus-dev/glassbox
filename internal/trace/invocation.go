// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package trace

import (
	"fmt"
	"strings"
)

// BoundaryKind indicates whether a boundary marker is the entry or exit of an
// invocation.
type BoundaryKind string

const (
	BoundaryEnter BoundaryKind = "enter"
	BoundaryExit  BoundaryKind = "exit"
)

// InvocationBoundary annotates the start or end of a Soroban contract function
// invocation. Each contract_call event in a trace produces one Enter and one
// Exit boundary so callers can bracket the range of steps belonging to that
// call.
type InvocationBoundary struct {
	// Kind is BoundaryEnter or BoundaryExit.
	Kind BoundaryKind
	// FunctionName is the name of the contract function being invoked.
	FunctionName string
	// ContractID identifies the contract (may be abbreviated).
	ContractID string
	// OperationIdx is the zero-based index of the operation within the
	// transaction (-1 when not available).
	OperationIdx int
	// StepIndex is the index into ExecutionTrace.States for this boundary.
	StepIndex int
	// ArgSummary is a short human-readable summary of the invocation arguments
	// (empty when no arguments are present).
	ArgSummary string
}

// AnnotateInvocationBoundaries scans t and returns an ordered list of
// InvocationBoundary markers. Every contract_call state generates one Enter
// boundary (at its own step index) and one Exit boundary (at the step
// immediately following the call, or at the same step when no subsequent step
// exists).
//
// The returned slice is ordered by StepIndex, with Enter always appearing
// before Exit for the same step.
func AnnotateInvocationBoundaries(t *ExecutionTrace) []InvocationBoundary {
	if t == nil {
		return nil
	}

	var out []InvocationBoundary

	for i, s := range t.States {
		if ClassifyEventType(&s) != EventTypeContractCall {
			continue
		}

		argSummary := summariseArgs(s)

		out = append(out, InvocationBoundary{
			Kind:         BoundaryEnter,
			FunctionName: s.Function,
			ContractID:   s.ContractID,
			OperationIdx: operationIndex(s),
			StepIndex:    i,
			ArgSummary:   argSummary,
		})

		// Exit boundary sits at the next step, or at the same step when this
		// is the last state in the trace.
		exitStep := i + 1
		if exitStep >= len(t.States) {
			exitStep = i
		}
		out = append(out, InvocationBoundary{
			Kind:         BoundaryExit,
			FunctionName: s.Function,
			ContractID:   s.ContractID,
			OperationIdx: operationIndex(s),
			StepIndex:    exitStep,
			ArgSummary:   argSummary,
		})
	}

	return out
}

// FormatInvocationBoundary renders a single InvocationBoundary as a one-line
// human-readable string suitable for trace output or log messages.
//
// Examples:
//
//	>>> ENTER  transfer  [CA…1234]  op=0  (args: amount, to)
//	<<< EXIT   transfer  [CA…1234]  op=0
func FormatInvocationBoundary(b InvocationBoundary) string {
	var sb strings.Builder

	if b.Kind == BoundaryEnter {
		sb.WriteString(">>> ENTER  ")
	} else {
		sb.WriteString("<<< EXIT   ")
	}

	name := b.FunctionName
	if name == "" {
		name = "<unknown>"
	}
	sb.WriteString(name)

	if b.ContractID != "" {
		fmt.Fprintf(&sb, "  [%s]", abbreviateID(b.ContractID))
	}

	if b.OperationIdx >= 0 {
		fmt.Fprintf(&sb, "  op=%d", b.OperationIdx)
	}

	if b.Kind == BoundaryEnter && b.ArgSummary != "" {
		fmt.Fprintf(&sb, "  (%s)", b.ArgSummary)
	}

	return sb.String()
}

// RenderInvocationBoundaries appends boundary markers inline with the
// formatted trace lines. It returns a slice of strings – one per boundary –
// using the same format as FormatInvocationBoundary.
func RenderInvocationBoundaries(t *ExecutionTrace) []string {
	boundaries := AnnotateInvocationBoundaries(t)
	out := make([]string, 0, len(boundaries))
	for _, b := range boundaries {
		out = append(out, FormatInvocationBoundary(b))
	}
	return out
}

// ── helpers ───────────────────────────────────────────────────────────────────

func operationIndex(s ExecutionState) int {
	// ExecutionState carries an Operation string, not a numeric index.
	// Return -1 to signal "not available" unless the operation string encodes
	// an index (e.g., "op:2").
	op := strings.TrimSpace(s.Operation)
	if strings.HasPrefix(op, "op:") {
		var idx int
		if _, err := fmt.Sscanf(op[3:], "%d", &idx); err == nil {
			return idx
		}
	}
	return -1
}

func summariseArgs(s ExecutionState) string {
	if len(s.Arguments) > 0 {
		parts := make([]string, 0, len(s.Arguments))
		for _, a := range s.Arguments {
			parts = append(parts, fmt.Sprintf("%v", a))
		}
		summary := strings.Join(parts, ", ")
		if len(summary) > 80 {
			summary = summary[:77] + "…"
		}
		return "args: " + summary
	}
	if len(s.RawArguments) > 0 {
		summary := strings.Join(s.RawArguments, ", ")
		if len(summary) > 80 {
			summary = summary[:77] + "…"
		}
		return "args: " + summary
	}
	return ""
}
