// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package trace

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func makeTrace(txHash string, states []ExecutionState) *ExecutionTrace {
	return &ExecutionTrace{
		TransactionHash: txHash,
		StartTime:       time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		EndTime:         time.Date(2026, 1, 15, 10, 0, 1, 0, time.UTC),
		States:          states,
	}
}

func defaultOpts() FormatOptions {
	return FormatOptions{LineWidth: 80, IndentWidth: 2}
}

// ─── FormatTrace ─────────────────────────────────────────────────────────────

func TestFormatTrace_NilTrace(t *testing.T) {
	out := FormatTrace(nil, defaultOpts())
	assert.Empty(t, out)
}

func TestFormatTrace_EmptyStates(t *testing.T) {
	t.Parallel()
	tr := makeTrace("TXABC", []ExecutionState{})
	out := FormatTrace(tr, defaultOpts())
	assert.Contains(t, out, "TXABC")
	assert.Contains(t, out, "Steps       : 0")
}

func TestFormatTrace_HeaderContainsHash(t *testing.T) {
	t.Parallel()
	tr := makeTrace("DEADBEEF123", []ExecutionState{})
	out := FormatTrace(tr, defaultOpts())
	assert.Contains(t, out, "DEADBEEF123")
}

func TestFormatTrace_HeaderContainsTimes(t *testing.T) {
	t.Parallel()
	tr := makeTrace("TX1", []ExecutionState{})
	out := FormatTrace(tr, defaultOpts())
	assert.Contains(t, out, "2026-01-15")
}

func TestFormatTrace_StepNumberPresent(t *testing.T) {
	t.Parallel()
	tr := makeTrace("TX1", []ExecutionState{
		{Step: 1, Operation: "contract_call"},
	})
	out := FormatTrace(tr, defaultOpts())
	assert.Contains(t, out, "[1]")
	assert.Contains(t, out, "contract_call")
}

func TestFormatTrace_SourcePreserved(t *testing.T) {
	t.Parallel()
	tr := makeTrace("TX2", []ExecutionState{
		{Step: 1, Operation: "call", SourceFile: "token.rs", SourceLine: 42},
	})
	out := FormatTrace(tr, defaultOpts())
	assert.Contains(t, out, "token.rs:42", "source reference must appear in output")
}

func TestFormatTrace_GitHubLinkPreserved(t *testing.T) {
	t.Parallel()
	tr := makeTrace("TX3", []ExecutionState{
		{Step: 1, Operation: "call", GitHubLink: "https://github.com/owner/repo/blob/main/src/token.rs#L42"},
	})
	out := FormatTrace(tr, defaultOpts())
	assert.Contains(t, out, "https://github.com/owner/repo", "GitHub link must appear in output")
}

func TestFormatTrace_ErrorPreserved(t *testing.T) {
	t.Parallel()
	tr := makeTrace("TX4", []ExecutionState{
		{Step: 1, Operation: "trap", Error: "integer overflow"},
	})
	out := FormatTrace(tr, defaultOpts())
	assert.Contains(t, out, "integer overflow")
}

func TestFormatTrace_LongLineWrapped(t *testing.T) {
	t.Parallel()
	longArgs := strings.Repeat("argument_value_", 20)
	tr := makeTrace("TX5", []ExecutionState{
		{Step: 1, Operation: "call", Arguments: []interface{}{longArgs}},
	})
	opts := FormatOptions{LineWidth: 60, IndentWidth: 2}
	out := FormatTrace(tr, opts)

	// Each line must fit within a reasonable bound. We use rune count (not
	// byte count) because the ruler uses multi-byte Unicode box-drawing chars.
	// Allow some slack beyond LineWidth=60 for the tree prefix and labels.
	for _, line := range strings.Split(out, "\n") {
		runeLen := utf8.RuneCountInString(line)
		assert.LessOrEqual(t, runeLen, 100,
			"no line should be excessively long when wrapping is active: %q", line)
	}
}

func TestFormatTrace_MultipleSteps_AllPresent(t *testing.T) {
	t.Parallel()
	tr := makeTrace("TX6", []ExecutionState{
		{Step: 1, Operation: "init"},
		{Step: 2, Operation: "transfer", Function: "transfer"},
		{Step: 3, Operation: "trap", Error: "out of gas"},
	})
	out := FormatTrace(tr, defaultOpts())
	assert.Contains(t, out, "[1]")
	assert.Contains(t, out, "[2]")
	assert.Contains(t, out, "[3]")
	assert.Contains(t, out, "out of gas")
}

func TestFormatTrace_ContractIDAbbreviated(t *testing.T) {
	t.Parallel()
	longID := "CCCCCCCCAAAABBBBDDDDEEEEFFFFGGGGHHHHIIIIJJJJKKKKLLLLMMMM"
	tr := makeTrace("TX7", []ExecutionState{
		{Step: 1, Operation: "call", ContractID: longID, Function: "approve"},
	})
	out := FormatTrace(tr, defaultOpts())
	// The full ID should not appear — it should be abbreviated.
	assert.NotContains(t, out, longID, "long contract ID should be abbreviated")
	assert.Contains(t, out, "CCCCCCCC", "prefix of ID should be present")
}

// ─── FormatTraceNode ─────────────────────────────────────────────────────────

func TestFormatTraceNode_NilRoot(t *testing.T) {
	out := FormatTraceNode(nil, defaultOpts())
	assert.Empty(t, out)
}

func TestFormatTraceNode_SingleNode(t *testing.T) {
	t.Parallel()
	n := NewTraceNode("n1", "contract_call")
	n.Function = "mint"
	out := FormatTraceNode(n, defaultOpts())
	assert.Contains(t, out, "contract_call")
	assert.Contains(t, out, "mint")
}

func TestFormatTraceNode_NestedChildren(t *testing.T) {
	t.Parallel()
	root := NewTraceNode("root", "contract_call")
	root.Function = "swap"
	child := NewTraceNode("c1", "host_function")
	child.Function = "call_contract"
	root.AddChild(child)
	grandchild := NewTraceNode("g1", "auth")
	grandchild.Function = "check_auth"
	child.AddChild(grandchild)

	opts := FormatOptions{LineWidth: 100, IndentWidth: 2}
	out := FormatTraceNode(root, opts)

	assert.Contains(t, out, "swap")
	assert.Contains(t, out, "call_contract")
	assert.Contains(t, out, "check_auth")
	assert.Contains(t, out, "└──", "last child should use └── connector")
}

func TestFormatTraceNode_SourceRefPreserved(t *testing.T) {
	t.Parallel()
	n := NewTraceNode("n1", "contract_call")
	n.SourceRef = &SourceRef{File: "pool.rs", Line: 99, Column: 7}
	out := FormatTraceNode(n, defaultOpts())
	assert.Contains(t, out, "pool.rs:99:7", "source ref with column should appear in output")
}

func TestFormatTraceNode_ErrorNode(t *testing.T) {
	t.Parallel()
	n := NewTraceNode("n1", "error")
	n.Error = "overflow in checked_add"
	out := FormatTraceNode(n, defaultOpts())
	assert.Contains(t, out, "overflow in checked_add")
}

// ─── buildTreePrefix ─────────────────────────────────────────────────────────

func TestBuildTreePrefix_Depth0(t *testing.T) {
	assert.Equal(t, "", buildTreePrefix(0, 2, true))
	assert.Equal(t, "", buildTreePrefix(0, 2, false))
}

func TestBuildTreePrefix_Depth1_Last(t *testing.T) {
	assert.Equal(t, "└── ", buildTreePrefix(1, 2, true))
}

func TestBuildTreePrefix_Depth1_NotLast(t *testing.T) {
	assert.Equal(t, "├── ", buildTreePrefix(1, 2, false))
}

func TestBuildTreePrefix_Depth2(t *testing.T) {
	prefix := buildTreePrefix(2, 2, false)
	assert.Equal(t, "  ├── ", prefix, "depth 2 adds one indent level before the connector")
}

// ─── writeMetaLine ───────────────────────────────────────────────────────────

func TestWriteMetaLine_Empty(t *testing.T) {
	var b strings.Builder
	writeMetaLine(&b, "  ", "source", "", 60)
	assert.Empty(t, b.String(), "empty value should produce no output")
}

func TestWriteMetaLine_Short(t *testing.T) {
	var b strings.Builder
	writeMetaLine(&b, "  ", "source", "token.rs:10", 60)
	out := b.String()
	assert.Contains(t, out, "source: token.rs:10")
}

func TestWriteMetaLine_LongValueWrapped(t *testing.T) {
	var b strings.Builder
	longVal := strings.Repeat("x", 200)
	writeMetaLine(&b, "", "args", longVal, 40)
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	require.Greater(t, len(lines), 1, "long value should produce multiple lines")
}

// ─── abbreviateID ────────────────────────────────────────────────────────────

func TestAbbreviateID_Short(t *testing.T) {
	assert.Equal(t, "ABC", abbreviateID("ABC"))
}

func TestAbbreviateID_Long(t *testing.T) {
	id := "AAAAAAAABBBBBBBBCCCCDDDD"
	result := abbreviateID(id)
	assert.Contains(t, result, "AAAAAAAA")
	assert.Contains(t, result, "DDDD")
	assert.Contains(t, result, "…")
	assert.NotEqual(t, id, result, "long ID should be abbreviated")
}

// ─── FormatOptions defaults ───────────────────────────────────────────────────

func TestFormatOptions_Defaults(t *testing.T) {
	opts := FormatOptions{}
	assert.Equal(t, defaultLineWidth, opts.lineWidth())
	assert.Equal(t, defaultIndentWidth, opts.indentWidth())
}

func TestFormatOptions_Custom(t *testing.T) {
	opts := FormatOptions{LineWidth: 60, IndentWidth: 4}
	assert.Equal(t, 60, opts.lineWidth())
	assert.Equal(t, 4, opts.indentWidth())
}
