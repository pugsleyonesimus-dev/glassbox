# Trace Cost Breakdowns

Glassbox can annotate contract-call trace states with resource cost information.

Cost annotations may come from observed simulator budget usage or from a gas
model JSON file:

```bash
glassbox trace execution.json --gas-model ./gas-model.json --export trace.md --export-format markdown
```

Each annotated trace node includes:

- `source`: `observed`, `gas_model_estimate`, or `node_delta`
- `cpu`: CPU instructions attributed to the call
- `memory_bytes`: memory attributed to the call
- `operations`: host operation count when available
- `estimated_fee`: gas-model fee estimate in stroops when available
- `breakdown`: component-level resource buckets

HTML and Markdown trace exports render both a compact cost summary and the
component breakdown for each annotated contract call.
