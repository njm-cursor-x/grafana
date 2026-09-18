# Logging security inventory

Regenerate the call-site / secret-identifier inventory (console/fmt.Print/Faro emit sites plus Authorization/Bearer/token/password nearby):

```bash
make logging-secret-inventory
# or
./scripts/logging-security/inventory.sh
```

The script writes `scripts/logging-security/inventory-report.txt` (generated; not committed).

Vulnerability scan notes for this lane live in `AUDIT.md`.
