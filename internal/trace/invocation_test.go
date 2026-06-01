// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package trace

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeContractCallState(step int, fn, contractID string, args []interface{}) ExecutionState {
	return ExecutionState{
		Step:       step,
		Timestamp:  time.Now(),
		Operation:  "contract_call",
		EventType:  EventTypeContractCall,
		Function:   fn,
		ContractID: contractID,
		Arguments:  args,
	}
}

func TestAnnotateInvocationBoundaries_Empty(t *testing.T) {
	tr := NewExecutionTrace("txhash", 0)
	out := AnnotateInvocationBoundaries(tr)
	assert.Empty(t, out)
}

func TestAnnotateInvocationBoundaries_Nil(t *testing.T) {
	out := AnnotateInvocationBoundaries(nil)
	assert.Nil(t, out)
}

func TestAnnotateInvocationBoundaries_NoContractCalls(t *testing.T) {
	tr := NewExecutionTrace("txhash", 0)
	tr.AddState(ExecutionState{Step: 0, EventType: EventTypeAuth, Function: "auth_fn"})
	tr.AddState(ExecutionState{Step: 1, EventType: EventTypeTrap, Operation: "trap"})

	out := AnnotateInvocationBoundaries(tr)
	assert.Empty(t, out, "non-contract-call events should produce no boundaries")
}

func TestAnnotateInvocationBoundaries_SingleCall(t *testing.T) {
	tr := NewExecutionTrace("txhash", 0)
	tr.AddState(makeContractCallState(0, "transfer", "CA1234", []interface{}{"100", "GABC"}))

	out := AnnotateInvocationBoundaries(tr)
	require.Len(t, out, 2)

	assert.Equal(t, BoundaryEnter, out[0].Kind)
	assert.Equal(t, "transfer", out[0].FunctionName)
	assert.Equal(t, "CA1234", out[0].ContractID)
	assert.Equal(t, 0, out[0].StepIndex)

	assert.Equal(t, BoundaryExit, out[1].Kind)
	assert.Equal(t, "transfer", out[1].FunctionName)
	assert.Equal(t, 0, out[1].StepIndex, "exit step equals enter step when trace has only one state")
}

func TestAnnotateInvocationBoundaries_MultipleCallsExitStepsForward(t *testing.T) {
	tr := NewExecutionTrace("txhash", 0)
	tr.AddState(makeContractCallState(0, "mint", "CA0001", nil))
	tr.AddState(ExecutionState{Step: 1, EventType: EventTypeHostFunction, Operation: "host_fn"})
	tr.AddState(makeContractCallState(2, "burn", "CA0002", nil))
	tr.AddState(ExecutionState{Step: 3, EventType: EventTypeAuth, Operation: "auth"})

	out := AnnotateInvocationBoundaries(tr)
	require.Len(t, out, 4)

	// mint enter/exit
	assert.Equal(t, BoundaryEnter, out[0].Kind)
	assert.Equal(t, "mint", out[0].FunctionName)
	assert.Equal(t, 0, out[0].StepIndex)

	assert.Equal(t, BoundaryExit, out[1].Kind)
	assert.Equal(t, "mint", out[1].FunctionName)
	assert.Equal(t, 1, out[1].StepIndex, "exit step should be the step after the call")

	// burn enter/exit
	assert.Equal(t, BoundaryEnter, out[2].Kind)
	assert.Equal(t, "burn", out[2].FunctionName)
	assert.Equal(t, 2, out[2].StepIndex)

	assert.Equal(t, BoundaryExit, out[3].Kind)
	assert.Equal(t, "burn", out[3].FunctionName)
	assert.Equal(t, 3, out[3].StepIndex)
}

func TestFormatInvocationBoundary_Enter(t *testing.T) {
	b := InvocationBoundary{
		Kind:         BoundaryEnter,
		FunctionName: "swap",
		ContractID:   "CABC1234",
		OperationIdx: 0,
		StepIndex:    5,
		ArgSummary:   "args: 500, GDEST",
	}
	out := FormatInvocationBoundary(b)
	assert.True(t, strings.HasPrefix(out, ">>> ENTER"), "enter boundary should start with '>>> ENTER'")
	assert.Contains(t, out, "swap")
	assert.Contains(t, out, "op=0")
	assert.Contains(t, out, "args: 500, GDEST")
}

func TestFormatInvocationBoundary_Exit(t *testing.T) {
	b := InvocationBoundary{
		Kind:         BoundaryExit,
		FunctionName: "swap",
		ContractID:   "CABC1234",
		OperationIdx: 0,
		StepIndex:    6,
	}
	out := FormatInvocationBoundary(b)
	assert.True(t, strings.HasPrefix(out, "<<< EXIT"), "exit boundary should start with '<<< EXIT'")
	assert.Contains(t, out, "swap")
	assert.NotContains(t, out, "args:", "exit boundary should not show args")
}

func TestFormatInvocationBoundary_UnknownFunction(t *testing.T) {
	b := InvocationBoundary{
		Kind:         BoundaryEnter,
		FunctionName: "",
		OperationIdx: -1,
	}
	out := FormatInvocationBoundary(b)
	assert.Contains(t, out, "<unknown>")
	assert.NotContains(t, out, "op=", "op field should be omitted when index is -1")
}

func TestRenderInvocationBoundaries(t *testing.T) {
	tr := NewExecutionTrace("txhash", 0)
	tr.AddState(makeContractCallState(0, "claim", "CAXYZ", []interface{}{"addr"}))
	tr.AddState(ExecutionState{Step: 1, EventType: EventTypeAuth})

	lines := RenderInvocationBoundaries(tr)
	require.Len(t, lines, 2)
	assert.Contains(t, lines[0], "ENTER")
	assert.Contains(t, lines[0], "claim")
	assert.Contains(t, lines[1], "EXIT")
	assert.Contains(t, lines[1], "claim")
}

func TestAnnotateInvocationBoundaries_ArgSummaryTruncated(t *testing.T) {
	longArg := strings.Repeat("X", 100)
	tr := NewExecutionTrace("txhash", 0)
	tr.AddState(makeContractCallState(0, "fn", "C1", []interface{}{longArg}))

	out := AnnotateInvocationBoundaries(tr)
	require.Len(t, out, 2)
	assert.LessOrEqual(t, len(out[0].ArgSummary), 90, "arg summary should be truncated")
}
