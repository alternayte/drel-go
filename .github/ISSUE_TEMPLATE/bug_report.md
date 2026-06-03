---
name: Bug report
about: Report incorrect behavior
title: ""
labels: bug
assignees: ""
---

**What happened**
A clear description of the bug.

**Reproduction**
Minimal model definition and the query / SaveChanges that misbehaves. The
smaller, the better.

```go
// model + call that reproduces it
```

**Expected behavior**

**Environment**
- drel version (or commit):
- Dialect: Postgres / SQLite / LibSQL
- Go version:
- OS:

**Generated SQL / logs (if relevant)**
Enable `drel.WithQueryLog(true)` and paste the offending SQL.
