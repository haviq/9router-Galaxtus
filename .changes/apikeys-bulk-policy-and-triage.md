### API keys: one bulk-policy surface, and a triage view for stalled keys

Per-key governance — usage limits, allowed models and an active window — already
lived in the per-key dialog (`Edit key & policy`). Applying the same shape to a
handful of keys meant opening a handful of dialogs, and a key that had quietly
stalled was only visible by scanning the table.

**Bulk policy** (`ApiKeysBulkPolicyModal.svelte`) collapses the three governance
axes into one form over the current selection:

- **Usage limits** — RPM, TPM, concurrency, lifetime token budget, queue wait.
- **Models allowed** — an allowlist written to every selected key.
- **Active window** — an `expiresAt` stamped on every selected key.

Every field is **tri-state**, because a bulk edit is not the same as "set these
values on all": leaving a field *unchanged* has to be expressible, or the dialog
silently rewrites a limit the operator never looked at. Each row carries
**Leave unchanged** (the column is omitted, the server keeps it), **Set** (the
typed value is written to every key) and, where clearing has a meaning,
**Clear** (reset to the unrestricted default). The allowlist has no "unchanged"
middle state in the API — `PUT /api/keys/{id}/models` replaces the whole list —
so it is an explicit Set-or-Skip switch, and a write with an empty list is
refused rather than silently emptying every selected key. A separate checkbox
resets each key's *spent* budget when a new ceiling is applied, so raising a
limit never clears usage by accident.

The batch reuses the existing per-key batch contract: one request per key, no
assumed bulk endpoint, and a key that rejects never rolls back the keys that
landed — the result is a tally, not a yes/no.

**Triage view.** The table gained status filters — **All**, **Needs attention**,
**Unrestricted**, **Active**, **Paused** — with live counts. *Needs attention* is
the set of keys that cannot serve a request they otherwise would: paused,
expired, or out of budget. *Unrestricted* is the opposite failure — a key
carrying no policy at all, the "did I forget to configure this?" case. Both
predicates live in `tableActions.ts` next to the batch helpers, so the table and
its tests state the rule once.

The pure policy builder and the two posture predicates are covered in
`web/src/components/keys/tableActions.test.ts` (11 new cases: keep/set/clear per
column, fractional and negative values floored, expiry set/clear, and the
attention/unrestricted rules).

Verified with `bun run build`, `bun run ratchet:svelte` (0 unresolved
identifiers, count held at the 77 baseline), `bunx oxlint --deny-warnings`,
`bun test src scripts` (670 pass, up from 659), and `go test ./web/...
./internal/handlers/dashboard/`.
