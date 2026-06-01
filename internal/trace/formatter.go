// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package trace

import (
	"fmt"
	"strings"

	"github.com/dotandev/glassbox/internal/abi"
)

const (
	defaultLineWidth   = 120
	defaultIndentWidth = 2
)

// FormatOptions controls how a trace is rendered as plain text.
type FormatOptions struct {
	// LineWidth is the maximum column width before text is wrapped.
	// 0 uses the default (120).
	LineWidth int

	// IndentWidth is the number of spaces added per nesting level.
	// 0 uses the default (2).
	IndentWidth int
}

func (o FormatOptions) lineWidth() int {
	if o.LineWidth > 0 {
		return o.LineWidth
	}
	return defaultLineWidth
}

func (o FormatOptions) indentWidth() int {
	if o.IndentWidth > 0 {
		return o.IndentWidth
	}
	return defaultIndentWidth
}

// FormatTrace renders an ExecutionTrace as a plain-text tree with stable
// indentation and consistent line wrapping. Source links and metadata are
// emitted on their own lines, indented to align with the node's text body.
func FormatTrace(t *ExecutionTrace, opts FormatOptions) string {
	if t == nil {
		return ""
	}

	var b strings.Builder
	lw := opts.lineWidth()
	ruler := strings.Repeat("─", min(lw, 80))

	// ── Header ──────────────────────────────────────────────────────────────
	b.WriteString("Glassbox Trace\n")
	b.WriteString(ruler + "\n")
	fmt.Fprintf(&b, "Transaction : %s\n", t.TransactionHash)
	if !t.StartTime.IsZero() {
		fmt.Fprintf(&b, "Started     : %s\n", t.StartTime.Format("2006-01-02 15:04:05 UTC"))
	}
	if !t.EndTime.IsZero() {
		fmt.Fprintf(&b, "Ended       : %s\n", t.EndTime.Format("2006-01-02 15:04:05 UTC"))
	}
	fmt.Fprintf(&b, "Steps       : %d\n", len(t.States))
	b.WriteString(ruler + "\n\n")

	// ── Steps ────────────────────────────────────────────────────────────────
	total := len(t.States)
	iw := opts.indentWidth()

	for i, s := range t.States {
		isLast := i == total-1
		depth := inferStateDepth(s)
		prefix := buildTreePrefix(depth, iw, isLast)
		textWidth := lw - len(prefix)
		if textWidth < 20 {
			textWidth = 20
		}
		cont := strings.Repeat(" ", len(prefix))

		// Node title (step + operation + contract/function)
		title := buildStateTitle(s)
		writeWrapped(&b, prefix, cont, title, textWidth)

		// Metadata lines — each preserves source links and stays readable.
		if src := formatStateSource(s); src != "" {
			writeMetaLine(&b, cont, "source", src, textWidth)
		}
		if args := formatStateArgs(s); args != "" {
			writeMetaLine(&b, cont, "args", args, textWidth)
		}
		if s.ReturnValue != nil && fmt.Sprintf("%v", s.ReturnValue) != "<nil>" {
			writeMetaLine(&b, cont, "return", fmt.Sprintf("%v", s.ReturnValue), textWidth)
		}
		if s.Error != "" {
			writeMetaLine(&b, cont, "error", s.Error, textWidth)
		}
		if s.Cost != nil {
			writeMetaLine(&b, cont, "cost", FormatCostAnnotation(s.Cost), textWidth)
			for _, line := range FormatCostBreakdown(s.Cost) {
				writeMetaLine(&b, cont, "cost breakdown", line, textWidth)
			}
		}
		if s.GitHubLink != "" {
			writeMetaLine(&b, cont, "link", s.GitHubLink, textWidth)
		}
		if s.ContractMetadata != nil && s.ContractMetadata.HasMetadata() {
			metaStr := s.ContractMetadata.String()
			for _, line := range strings.Split(strings.TrimSpace(metaStr), "\n") {
				if line == "" {
					continue
				}
				parts := strings.SplitN(line, ": ", 2)
				if len(parts) == 2 {
					writeMetaLine(&b, cont, parts[0], parts[1], textWidth)
				} else {
					writeMetaLine(&b, cont, "meta", line, textWidth)
				}
			}
		}
	}

	return b.String()
}

// FormatTraceNode renders a TraceNode tree with stable indentation and line
// wrapping. The tree connectors (├──, └──) are aligned consistently at every
// nesting level, and source references are preserved on a continuation line.
func FormatTraceNode(root *TraceNode, opts FormatOptions) string {
	if root == nil {
		return ""
	}
	var b strings.Builder
	lw := opts.lineWidth()
	iw := opts.indentWidth()
	renderNode(&b, root, 0, lw, iw, true)
	return b.String()
}

// ─── internal helpers ─────────────────────────────────────────────────────────

func renderNode(b *strings.Builder, n *TraceNode, depth, lw, iw int, isLast bool) {
	prefix := buildTreePrefix(depth, iw, isLast)
	textWidth := lw - len(prefix)
	if textWidth < 20 {
		textWidth = 20
	}
	cont := strings.Repeat(" ", len(prefix))

	title := buildNodeTitle(n)
	writeWrapped(b, prefix, cont, title, textWidth)

	if n.SourceRef != nil {
		loc := fmt.Sprintf("%s:%d", n.SourceRef.File, n.SourceRef.Line)
		if n.SourceRef.Column > 0 {
			loc = fmt.Sprintf("%s:%d:%d", n.SourceRef.File, n.SourceRef.Line, n.SourceRef.Column)
		}
		writeMetaLine(b, cont, "source", loc, textWidth)
	}
	if n.Cost != nil {
		writeMetaLine(b, cont, "cost", FormatCostAnnotation(n.Cost), textWidth)
		for _, line := range FormatCostBreakdown(n.Cost) {
			writeMetaLine(b, cont, "cost breakdown", line, textWidth)
		}
	}

	if n.ContractMetadata != nil && n.ContractMetadata.HasMetadata() {
		metaStr := n.ContractMetadata.String()
		// Split the string into individual meta lines
		for _, line := range strings.Split(strings.TrimSpace(metaStr), "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, ": ", 2)
			if len(parts) == 2 {
				writeMetaLine(b, cont, parts[0], parts[1], textWidth)
			} else {
				writeMetaLine(b, cont, "meta", line, textWidth)
			}
		}
	}

	for i, child := range n.Children {
		renderNode(b, child, depth+1, lw, iw, i == len(n.Children)-1)
	}
}

// buildTreePrefix constructs the tree-connector prefix for a node.
//
//	depth 0 → no prefix
//	depth 1, not last → "├── "
//	depth 1, last     → "└── "
//	depth 2           → "  ├── " (indentWidth spaces per extra level)
func buildTreePrefix(depth, indentWidth int, isLast bool) string {
	if depth == 0 {
		return ""
	}
	pad := strings.Repeat(" ", (depth-1)*indentWidth)
	if isLast {
		return pad + "└── "
	}
	return pad + "├── "
}

// inferStateDepth heuristically assigns a nesting depth to an ExecutionState.
func inferStateDepth(s ExecutionState) int {
	switch s.EventType {
	case "contract_call":
		return 1
	case "host_function", "auth":
		return 2
	case "trap", "error":
		return 2
	default:
		return 0
	}
}

// buildStateTitle creates a concise one-line title for an ExecutionState.
func buildStateTitle(s ExecutionState) string {
	step := fmt.Sprintf("[%d]", s.Step)
	op := s.Operation
	if op == "" {
		op = s.EventType
	}
	if op == "" {
		op = "step"
	}
	cid := ""
	if s.ContractID != "" {
		cid = abbreviateID(s.ContractID)
	}

	switch {
	case cid != "" && s.Function != "":
		return fmt.Sprintf("%s %s  %s::%s", step, op, cid, s.Function)
	case s.Function != "":
		return fmt.Sprintf("%s %s  %s", step, op, s.Function)
	case cid != "":
		return fmt.Sprintf("%s %s  %s", step, op, cid)
	default:
		return fmt.Sprintf("%s %s", step, op)
	}
}

// buildNodeTitle creates a one-line title for a TraceNode.
func buildNodeTitle(n *TraceNode) string {
	cid := abbreviateID(n.ContractID)
	switch {
	case cid != "" && n.Function != "":
		return fmt.Sprintf("[%s] %s::%s", n.Type, cid, n.Function)
	case n.Function != "":
		return fmt.Sprintf("[%s] %s", n.Type, n.Function)
	case n.Error != "":
		return fmt.Sprintf("[%s] ERROR: %s", n.Type, n.Error)
	case n.EventData != "":
		return fmt.Sprintf("[%s] %s", n.Type, n.EventData)
	default:
		return fmt.Sprintf("[%s]", n.Type)
	}
}

// abbreviateID shortens a long contract/account ID for display.
func abbreviateID(id string) string {
	const keep = 16
	if len(id) <= keep {
		return id
	}
	return id[:8] + "…" + id[len(id)-4:]
}

func formatStateSource(s ExecutionState) string {
	if s.SourceFile == "" {
		return ""
	}
	if s.SourceLine > 0 {
		return fmt.Sprintf("%s:%d", s.SourceFile, s.SourceLine)
	}
	return s.SourceFile
}

func formatStateArgs(s ExecutionState) string {
	if len(s.Arguments) == 0 {
		return ""
	}
	return fmt.Sprintf("%v", s.Arguments)
}

// writeWrapped writes a possibly-long string starting with prefix and
// wrapping continuation lines to cont+wrapped text.
func writeWrapped(b *strings.Builder, prefix, cont, text string, textWidth int) {
	wrapped := wrapText(text, textWidth)
	for i, line := range strings.Split(wrapped, "\n") {
		if i == 0 {
			b.WriteString(prefix)
		} else {
			b.WriteString(cont)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
}

// writeMetaLine emits a "key: value" metadata line indented to prefix,
// wrapping the value when it exceeds the available text width.
func writeMetaLine(b *strings.Builder, prefix, key, value string, textWidth int) {
	if value == "" {
		return
	}
	label := key + ": "
	valueWidth := textWidth - len(label)
	if valueWidth < 10 {
		valueWidth = 10
	}
	wrapped := wrapText(value, valueWidth)
	valCont := strings.Repeat(" ", len(prefix)+len(label))

	for i, line := range strings.Split(wrapped, "\n") {
		if i == 0 {
			b.WriteString(prefix + label + line + "\n")
		} else {
			b.WriteString(valCont + line + "\n")
		}
	}
}
