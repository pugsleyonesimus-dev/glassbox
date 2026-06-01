// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package trace

import (
	"strings"
	"testing"

	"github.com/dotandev/glassbox/internal/gasmodel"
	"github.com/dotandev/glassbox/internal/simulator"
)

func TestAnnotateExecutionCosts_FromBudgetUsage(t *testing.T) {
	tr := NewExecutionTrace("tx", 10)
	tr.AddState(ExecutionState{Operation: "contract_call", EventType: EventTypeContractCall, ContractID: "C1", Function: "transfer"})
	tr.AddState(ExecutionState{Operation: "host_function", EventType: EventTypeHostFunction, Function: "require_auth"})

	AnnotateExecutionCosts(tr, &simulator.BudgetUsage{
		CPUInstructions:    1000,
		MemoryBytes:        2048,
		OperationsCount:    4,
		CPULimit:           10000,
		MemoryLimit:        8192,
		CPUUsagePercent:    10,
		MemoryUsagePercent: 25,
	}, nil)

	if tr.States[0].Cost == nil {
		t.Fatal("expected contract call to receive cost annotation")
	}
	if tr.States[0].Cost.CPU != 1000 {
		t.Fatalf("expected cpu annotation 1000, got %d", tr.States[0].Cost.CPU)
	}
	if tr.States[1].Cost != nil {
		t.Fatal("did not expect host function state to receive contract cost annotation")
	}
}

func TestAnnotateExecutionCosts_FromGasModel(t *testing.T) {
	tr := NewExecutionTrace("tx", 10)
	tr.AddState(ExecutionState{Operation: "contract_call", EventType: EventTypeContractCall, ContractID: "C1", Function: "mint"})

	AnnotateExecutionCosts(tr, nil, &gasmodel.GasModel{
		Version:  "v1",
		CPUCosts: []gasmodel.GasCost{{Name: "invoke_contract", Linear: 2, Const: 10}},
	})

	if tr.States[0].Cost == nil {
		t.Fatal("expected gas model cost annotation")
	}
	if tr.States[0].Cost.Source != "gas_model_estimate" {
		t.Fatalf("expected gas model estimate, got %s", tr.States[0].Cost.Source)
	}
	if tr.States[0].Cost.EstimatedFee != 12 {
		t.Fatalf("expected estimated fee 12, got %d", tr.States[0].Cost.EstimatedFee)
	}
}

func TestTraceExportIncludesCommentsMetadataAndCost(t *testing.T) {
	tr := NewExecutionTrace("tx", 10)
	tr.AddState(ExecutionState{
		Operation: "contract_call",
		EventType: EventTypeContractCall,
		Function:  "transfer",
		Cost:      &CostAnnotation{Source: "observed", CPU: 42, Breakdown: []CostComponent{{Name: "cpu_instructions", Category: "cpu", Units: 42, Total: 42}}},
	})

	md, err := GenerateTraceMarkdownWithOptions(tr, ExportOptions{
		Comments:        []string{"reviewed by alice"},
		SessionMetadata: map[string]string{"snapshot": "payroll-bug"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"reviewed by alice", "snapshot", "source=observed", "cpu_instructions"} {
		if !strings.Contains(md, want) {
			t.Fatalf("export missing %q:\n%s", want, md)
		}
	}
}
