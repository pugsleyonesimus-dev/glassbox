# Session Bookmarking

Saved debug sessions can be bookmarked with a human-readable name:

```bash
glassbox session save --name payroll-bug
glassbox session list
glassbox session load payroll-bug
```

`session load` is an alias for `session resume`. Lookups accept exact session
IDs, bookmark names, unique ID prefixes, and transaction hashes.

Bookmark names are stored with the saved session snapshot metadata and must be
unique.
