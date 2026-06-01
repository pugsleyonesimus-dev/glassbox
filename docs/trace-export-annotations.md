# Trace Export Annotations

Trace exports can include collaborator comments and session metadata:

```bash
glassbox trace execution.json \
  --export trace.html \
  --comment "Reviewed with Alice" \
  --meta session=payroll-bug \
  --meta network=testnet
```

Supported fields:

- `comments`: free-form notes, repeatable with `--comment`
- `session_metadata`: key/value metadata supplied with `--meta key=value`
- `generated_at`: timestamp added when annotations are merged into exports

Annotations are included in HTML and Markdown trace artifacts and preserved in
JSON trace exports under the `annotations` object.
