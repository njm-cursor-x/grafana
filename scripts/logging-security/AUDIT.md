# Logging-security audit notes

How to refresh (also `make logging-security-audit`):

```bash
# Backend — scoped to the logger package this lane touches
govulncheck ./pkg/infra/log/...

# Frontend production tree (Yarn 4)
yarn npm audit --recursive --environment production
```

Results from the branch that added emit-time redaction are recorded below after the first test/audit run.
