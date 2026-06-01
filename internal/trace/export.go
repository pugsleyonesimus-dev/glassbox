// Copyright 2026 Glassbox Users
// SPDX-License-Identifier: Apache-2.0

package trace

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dotandev/glassbox/internal/abi"
)

type exportState struct {
	Step              int
	Summary           string
	Operation         string
	EventType         string
	Contract          string
	Function          string
	ContractMetadata  *abi.ContractMetadata
	Args              string
	Return            string
	Error             string
	SourceFile        string
	SourceLine        int
	GitHubLink        string
	CostSummary       string
	CostBreakdown     []string
	Details           []string
}

type exportData struct {
	TransactionHash string
	StartTime       string
	EndTime         string
	TotalSteps      int
	Annotations     TraceAnnotations
	States          []exportState
}

type ExportOptions struct {
	Comments        []string
	SessionMetadata map[string]string
}

const traceHTMLTemplate = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Glassbox Trace Export</title>
  <style>
    body { font-family: system-ui, sans-serif; margin: 0; padding: 1rem; background: #0b1220; color: #e8eef9; }
    .header { border-bottom: 1px solid #334155; padding-bottom: 1rem; margin-bottom: 1rem; }
    .header h1 { margin: 0 0 .25rem; font-size: 1.6rem; }
    .header p { margin: .25rem 0; color: #94a3b8; }
    .controls { margin: .75rem 0; }
    .controls button { margin-right: .5rem; padding: .5rem .75rem; border: none; border-radius: .375rem; cursor: pointer; background: #334155; color: #f8fafc; }
    details { margin-bottom: .75rem; border: 1px solid #334155; border-radius: .5rem; background: #111827; padding: .75rem; }
    summary { font-size: 1rem; font-weight: 700; cursor: pointer; }
    .state-meta { margin-top: .5rem; color: #cbd5e1; }
    .state-meta span { display: inline-block; margin-right: 1rem; }
    .field { margin: .35rem 0; }
    .field strong { color: #e2e8f0; }
    a { color: #60a5fa; }
    code { display: inline-block; background: #1e293b; padding: .15rem .3rem; border-radius: .25rem; }
  </style>
</head>
<body>
  <div class="header">
    <h1>Glassbox Trace Export</h1>
    <p>Transaction: {{ .TransactionHash }}</p>
    <p>Steps: {{ .TotalSteps }} · Started: {{ .StartTime }} · Ended: {{ .EndTime }}</p>
    {{ if .Annotations.Comments }}
    <div class="field"><strong>Comments:</strong><ul>{{ range .Annotations.Comments }}<li>{{ . }}</li>{{ end }}</ul></div>
    {{ end }}
    {{ if .Annotations.SessionMetadata }}
    <div class="field"><strong>Session metadata:</strong><ul>{{ range $k, $v := .Annotations.SessionMetadata }}<li><code>{{ $k }}</code>: {{ $v }}</li>{{ end }}</ul></div>
    {{ end }}
    <div class="controls">
      <button onclick="setAll(true)">Expand all</button>
      <button onclick="setAll(false)">Collapse all</button>
    </div>
  </div>
  {{ range .States }}
  <details open>
    <summary>#{{ .Step }} · {{ .Summary }}</summary>
    <div class="state-meta">
      <span><strong>Operation:</strong> {{ .Operation }}</span>
      {{ if .EventType }}<span><strong>Event:</strong> {{ .EventType }}</span>{{ end }}
      {{ if .Contract }}<span><strong>Contract:</strong> {{ .Contract }}</span>{{ end }}
      {{ if .Function }}<span><strong>Function:</strong> {{ .Function }}</span>{{ end }}
      {{ if .SourceFile }}<span><strong>Source:</strong> {{ .SourceFile }}:{{ .SourceLine }}</span>{{ end }}
      {{ if .GitHubLink }}<span><strong>Link:</strong> <a href="{{ .GitHubLink }}" target="_blank">View on GitHub</a></span>{{ end }}
    </div>
    <div class="field"><strong>Arguments:</strong> <code>{{ .Args }}</code></div>
    {{ if .Return }}<div class="field"><strong>Return:</strong> <code>{{ .Return }}</code></div>{{ end }}
    {{ if .Error }}<div class="field"><strong>Error:</strong> <code>{{ .Error }}</code></div>{{ end }}
    {{ if .CostSummary }}<div class="field"><strong>Cost:</strong> <code>{{ .CostSummary }}</code></div>{{ end }}
    {{ if .CostBreakdown }}
    <div class="field"><strong>Cost breakdown:</strong>
      <ul>
      {{ range .CostBreakdown }}<li>{{ . }}</li>{{ end }}
      </ul>
    </div>
    {{ end }}
    {{ if .Details }}
    <div class="field"><strong>Details:</strong>
      <ul>
      {{ range .Details }}<li>{{ . }}</li>{{ end }}
      </ul>
    </div>
    {{ end }}
  </details>
  {{ end }}
  <script>
    function setAll(open) {
      document.querySelectorAll('details').forEach(function(element) { element.open = open; });
    }
  </script>
</body>
</html>`

const traceMarkdownTemplate = `# Glassbox Trace Export

**Transaction:** {{ .TransactionHash }}

**Steps:** {{ .TotalSteps }}

**Started:** {{ .StartTime }}

**Ended:** {{ .EndTime }}

{{ if .Annotations.Comments }}## Comments
{{ range .Annotations.Comments }}- {{ . }}
{{ end }}
{{ end }}{{ if .Annotations.SessionMetadata }}## Session Metadata
{{ range $k, $v := .Annotations.SessionMetadata }}- **{{ $k }}:** {{ $v }}
{{ end }}
{{ end }}
{{ range .States }}
## Step {{ .Step }}: {{ .Summary }}

- **Operation:** {{ .Operation }}
{{ if .EventType }}- **Event:** {{ .EventType }}
{{ end }}{{ if .Contract }}- **Contract:** {{ .Contract }}
{{ end }}{{ if .Function }}- **Function:** {{ .Function }}
{{ end }}{{ if .SourceFile }}- **Source:** {{ .SourceFile }}:{{ .SourceLine }}
{{ end }}{{ if .GitHubLink }}- **GitHub:** [View on GitHub]({{ .GitHubLink }})
{{ end }}- **Arguments:** {{ .Args }}
{{ if .Return }}- **Return:** {{ .Return }}
{{ end }}{{ if .Error }}- **Error:** {{ .Error }}
{{ end }}{{ if .CostSummary }}- **Cost:** {{ .CostSummary }}
{{ end }}{{ if .CostBreakdown }}- **Cost breakdown:**
  {{ range .CostBreakdown }}
  - {{ . }}
  {{ end }}
{{ end }}{{ if .Details }}- **Details:**
  {{ range .Details }}
  - {{ . }}
  {{ end }}
{{ end }}

{{ end }}`

func ExportExecutionTrace(trace *ExecutionTrace, format string, outputPath string) error {
	return ExportExecutionTraceWithOptions(trace, format, outputPath, ExportOptions{})
}

func ExportExecutionTraceWithOptions(trace *ExecutionTrace, format string, outputPath string, opts ExportOptions) error {
	if trace == nil {
		return fmt.Errorf("trace is nil")
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "html"
	}

	var content string
	var err error
	switch format {
	case "html":
		content, err = GenerateTraceHTMLWithOptions(trace, opts)
	case "markdown", "md":
		content, err = GenerateTraceMarkdownWithOptions(trace, opts)
	default:
		return fmt.Errorf("unsupported trace export format: %s", format)
	}
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("failed to create trace export directory: %w", err)
	}
	return os.WriteFile(outputPath, []byte(content), 0o644)
}

func GenerateTraceHTML(trace *ExecutionTrace) (string, error) {
	return GenerateTraceHTMLWithOptions(trace, ExportOptions{})
}

func GenerateTraceHTMLWithOptions(trace *ExecutionTrace, opts ExportOptions) (string, error) {
	if trace == nil {
		return "", fmt.Errorf("trace is nil")
	}
	annotations := mergeTraceAnnotations(trace.Annotations, opts)

	data := exportData{
		TransactionHash: trace.TransactionHash,
		StartTime:       trace.StartTime.Format(time.RFC3339),
		EndTime:         trace.EndTime.Format(time.RFC3339),
		TotalSteps:      len(trace.States),
		Annotations:     annotations,
		States:          buildExportStates(trace),
	}

	tmpl, err := template.New("trace-html").Parse(traceHTMLTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse trace export template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render trace HTML: %w", err)
	}
	return buf.String(), nil
}

func GenerateTraceMarkdown(trace *ExecutionTrace) (string, error) {
	return GenerateTraceMarkdownWithOptions(trace, ExportOptions{})
}

func GenerateTraceMarkdownWithOptions(trace *ExecutionTrace, opts ExportOptions) (string, error) {
	if trace == nil {
		return "", fmt.Errorf("trace is nil")
	}
	annotations := mergeTraceAnnotations(trace.Annotations, opts)

	data := exportData{
		TransactionHash: trace.TransactionHash,
		StartTime:       trace.StartTime.Format(time.RFC3339),
		EndTime:         trace.EndTime.Format(time.RFC3339),
		TotalSteps:      len(trace.States),
		Annotations:     annotations,
		States:          buildExportStates(trace),
	}

	tmpl, err := template.New("trace-md").Parse(traceMarkdownTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse trace export template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render trace markdown: %w", err)
	}
	return buf.String(), nil
}

func buildExportStates(trace *ExecutionTrace) []exportState {
	states := make([]exportState, 0, len(trace.States))
	for _, s := range trace.States {
		details := make([]string, 0)
		if s.Error != "" {
			details = append(details, fmt.Sprintf("error: %s", s.Error))
		}
		if s.Operation != "" {
			details = append(details, fmt.Sprintf("operation: %s", s.Operation))
		}
		if s.ContractID != "" {
			details = append(details, fmt.Sprintf("contract: %s", s.ContractID))
		}
		if s.Function != "" {
			details = append(details, fmt.Sprintf("function: %s", s.Function))
		}
		if s.WasmInstruction != "" {
			details = append(details, fmt.Sprintf("wasm instruction: %s", s.WasmInstruction))
		}
		if len(s.Arguments) > 0 {
			details = append(details, fmt.Sprintf("arguments: %v", s.Arguments))
		}
		if s.RawArguments != nil && len(s.RawArguments) > 0 {
			details = append(details, fmt.Sprintf("raw arguments: %v", s.RawArguments))
		}
		if s.HostState != nil {
			details = append(details, fmt.Sprintf("host state entries: %d", len(s.HostState)))
		}
		if s.Memory != nil {
			details = append(details, fmt.Sprintf("memory entries: %d", len(s.Memory)))
		}
		if s.Cost != nil {
			details = append(details, fmt.Sprintf("cost: %s", FormatCostAnnotation(s.Cost)))
		}

		summary := s.Operation
		if summary == "" {
			summary = s.EventType
		}
		if summary == "" && s.ContractID != "" {
			summary = s.ContractID
		}
		if summary == "" {
			summary = fmt.Sprintf("step %d", s.Step)
		}

		states = append(states, exportState{
			Step:              s.Step,
			Summary:           summary,
			Operation:         s.Operation,
			EventType:         s.EventType,
			Contract:          s.ContractID,
			Function:          s.Function,
			ContractMetadata:  s.ContractMetadata,
			Args:              fmt.Sprintf("%v", s.Arguments),
			Return:            fmt.Sprintf("%v", s.ReturnValue),
			Error:             s.Error,
			SourceFile:        s.SourceFile,
			SourceLine:        s.SourceLine,
			GitHubLink:        s.GitHubLink,
			CostSummary:       FormatCostAnnotation(s.Cost),
			CostBreakdown:     FormatCostBreakdown(s.Cost),
			Details:           details,
		})
	}
	return states
}

func mergeTraceAnnotations(base TraceAnnotations, opts ExportOptions) TraceAnnotations {
	out := base
	if len(opts.Comments) > 0 {
		out.Comments = append(append([]string(nil), base.Comments...), opts.Comments...)
	}
	if len(opts.SessionMetadata) > 0 {
		merged := make(map[string]string, len(base.SessionMetadata)+len(opts.SessionMetadata))
		for k, v := range base.SessionMetadata {
			merged[k] = v
		}
		for k, v := range opts.SessionMetadata {
			merged[k] = v
		}
		out.SessionMetadata = merged
	}
	if out.GeneratedAt.IsZero() && (len(out.Comments) > 0 || len(out.SessionMetadata) > 0) {
		out.GeneratedAt = time.Now()
	}
	return out
}
 
