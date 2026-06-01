// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package trace

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dotandev/glassbox/internal/gasmodel"
	"github.com/dotandev/glassbox/internal/simulator"
)

// CostComponent describes one resource bucket that contributes to a trace node's
// estimated or observed execution cost.
type CostComponent struct {
	Name        string  `json:"name"`
	Category    string  `json:"category,omitempty"`
	Units       uint64  `json:"units,omitempty"`
	UnitCost    uint64  `json:"unit_cost,omitempty"`
	Total       uint64  `json:"total,omitempty"`
	Percent     float64 `json:"percent,omitempty"`
	Description string  `json:"description,omitempty"`
}

// CostAnnotation is attached to trace states or tree nodes when budget usage or
// gas-model estimates are available.
type CostAnnotation struct {
	ModelVersion string          `json:"model_version,omitempty"`
	Source       string          `json:"source"` // observed, gas_model_estimate, or node_delta
	CPU          uint64          `json:"cpu,omitempty"`
	MemoryBytes  uint64          `json:"memory_bytes,omitempty"`
	Operations   int             `json:"operations,omitempty"`
	EstimatedFee uint64          `json:"estimated_fee,omitempty"`
	LimitCPU     uint64          `json:"limit_cpu,omitempty"`
	LimitMemory  uint64          `json:"limit_memory,omitempty"`
	Breakdown    []CostComponent `json:"breakdown,omitempty"`
}

// TraceAnnotations carries collaboration metadata that should travel with
// exported trace artifacts.
type TraceAnnotations struct {
	Comments        []string          `json:"comments,omitempty"`
	SessionMetadata map[string]string `json:"session_metadata,omitempty"`
	GeneratedAt     time.Time         `json:"generated_at,omitempty"`
}

// AnnotateExecutionCosts attaches cost annotations to contract execution states.
func AnnotateExecutionCosts(t *ExecutionTrace, budget *simulator.BudgetUsage, model *gasmodel.GasModel) {
	if t == nil {
		return
	}

	contractIndexes := make([]int, 0)
	for i := range t.States {
		if ClassifyEventType(&t.States[i]) == EventTypeContractCall {
			contractIndexes = append(contractIndexes, i)
		}
	}
	if len(contractIndexes) == 0 {
		return
	}

	base := costFromBudget(budget)
	if base == nil {
		base = costFromGasModel(model)
	}
	if base == nil {
		return
	}

	for _, idx := range contractIndexes {
		ann := splitCostAnnotation(base, len(contractIndexes))
		if model != nil {
			ann.ModelVersion = model.Version
		}
		t.States[idx].Cost = ann
	}
}

// AnnotateNodeCost attaches a cost annotation to a tree node from its existing
// CPU and memory deltas.
func AnnotateNodeCost(n *TraceNode, model *gasmodel.GasModel) {
	if n == nil {
		return
	}
	if n.CPUDelta != nil || n.MemoryDelta != nil {
		ann := &CostAnnotation{Source: "node_delta"}
		if n.CPUDelta != nil {
			ann.CPU = *n.CPUDelta
			ann.Breakdown = append(ann.Breakdown, CostComponent{Name: "cpu_instructions", Category: "cpu", Units: ann.CPU, Total: ann.CPU})
		}
		if n.MemoryDelta != nil {
			ann.MemoryBytes = *n.MemoryDelta
			ann.Breakdown = append(ann.Breakdown, CostComponent{Name: "memory_bytes", Category: "memory", Units: ann.MemoryBytes, Total: ann.MemoryBytes})
		}
		if model != nil {
			ann.ModelVersion = model.Version
			if cost := model.GetCostByName(n.Function); cost != nil {
				ann.EstimatedFee = cost.Const + cost.Linear
				ann.Breakdown = append(ann.Breakdown, CostComponent{Name: cost.Name, Category: "gas_model", UnitCost: cost.Linear, Total: ann.EstimatedFee, Description: cost.Description})
			}
		}
		n.Cost = ann
	}
	for _, child := range n.Children {
		AnnotateNodeCost(child, model)
	}
}

func FormatCostAnnotation(c *CostAnnotation) string {
	if c == nil {
		return ""
	}
	parts := []string{fmt.Sprintf("source=%s", c.Source)}
	if c.CPU > 0 {
		parts = append(parts, fmt.Sprintf("cpu=%d", c.CPU))
	}
	if c.MemoryBytes > 0 {
		parts = append(parts, fmt.Sprintf("memory=%d bytes", c.MemoryBytes))
	}
	if c.Operations > 0 {
		parts = append(parts, fmt.Sprintf("operations=%d", c.Operations))
	}
	if c.EstimatedFee > 0 {
		parts = append(parts, fmt.Sprintf("estimated_fee=%d stroops", c.EstimatedFee))
	}
	return strings.Join(parts, ", ")
}

func FormatCostBreakdown(c *CostAnnotation) []string {
	if c == nil {
		return nil
	}
	out := make([]string, 0, len(c.Breakdown))
	for _, b := range c.Breakdown {
		line := b.Name
		if b.Category != "" {
			line += " [" + b.Category + "]"
		}
		if b.Units > 0 {
			line += fmt.Sprintf(": units=%d", b.Units)
		}
		if b.UnitCost > 0 {
			line += fmt.Sprintf(" unit_cost=%d", b.UnitCost)
		}
		if b.Total > 0 {
			line += fmt.Sprintf(" total=%d", b.Total)
		}
		if b.Percent > 0 {
			line += fmt.Sprintf(" %.2f%%", b.Percent)
		}
		out = append(out, line)
	}
	sort.Strings(out)
	return out
}

func costFromBudget(b *simulator.BudgetUsage) *CostAnnotation {
	if b == nil {
		return nil
	}
	ann := &CostAnnotation{
		Source:      "observed",
		CPU:         b.CPUInstructions,
		MemoryBytes: b.MemoryBytes,
		Operations:  b.OperationsCount,
		LimitCPU:    b.CPULimit,
		LimitMemory: b.MemoryLimit,
	}
	if b.CPUInstructions > 0 {
		ann.Breakdown = append(ann.Breakdown, CostComponent{Name: "cpu_instructions", Category: "cpu", Units: b.CPUInstructions, Total: b.CPUInstructions, Percent: b.CPUUsagePercent})
	}
	if b.MemoryBytes > 0 {
		ann.Breakdown = append(ann.Breakdown, CostComponent{Name: "memory_bytes", Category: "memory", Units: b.MemoryBytes, Total: b.MemoryBytes, Percent: b.MemoryUsagePercent})
	}
	if b.OperationsCount > 0 {
		ann.Breakdown = append(ann.Breakdown, CostComponent{Name: "operations", Category: "host", Units: uint64(b.OperationsCount), Total: uint64(b.OperationsCount)})
	}
	return ann
}

func costFromGasModel(model *gasmodel.GasModel) *CostAnnotation {
	if model == nil {
		return nil
	}
	ann := &CostAnnotation{Source: "gas_model_estimate", ModelVersion: model.Version}
	for _, c := range model.AllCosts() {
		total := c.Const + c.Linear
		ann.EstimatedFee += total
		ann.Breakdown = append(ann.Breakdown, CostComponent{Name: c.Name, Category: costCategory(model, c.Name), UnitCost: c.Linear, Total: total, Description: c.Description})
	}
	if ann.EstimatedFee == 0 && len(ann.Breakdown) == 0 {
		return nil
	}
	return ann
}

func costCategory(model *gasmodel.GasModel, name string) string {
	for _, c := range model.CPUCosts {
		if c.Name == name {
			return "cpu"
		}
	}
	for _, c := range model.HostCosts {
		if c.Name == name {
			return "host"
		}
	}
	for _, c := range model.LedgerCosts {
		if c.Name == name {
			return "ledger"
		}
	}
	return "gas_model"
}

func splitCostAnnotation(base *CostAnnotation, count int) *CostAnnotation {
	if count <= 1 {
		cp := *base
		cp.Breakdown = append([]CostComponent(nil), base.Breakdown...)
		return &cp
	}
	cp := *base
	cp.CPU = base.CPU / uint64(count)
	cp.MemoryBytes = base.MemoryBytes / uint64(count)
	cp.Operations = base.Operations / count
	cp.EstimatedFee = base.EstimatedFee / uint64(count)
	cp.Breakdown = make([]CostComponent, 0, len(base.Breakdown))
	for _, b := range base.Breakdown {
		nb := b
		nb.Units = b.Units / uint64(count)
		nb.Total = b.Total / uint64(count)
		cp.Breakdown = append(cp.Breakdown, nb)
	}
	return &cp
}
