// The API-key row menu and its batch bar collapse seven per-row controls into
// one trigger, so the rules the table depends on are stated once here rather
// than re-derived in the markup.
//
// Batch actions run one request per key on purpose: there is no bulk endpoint, and
// a single failing key must not roll back the keys that already succeeded. The
// outcome is therefore a tally, not a yes/no.

export interface KeyOutcome {
  ok: number
  failed: number
  /** Names or ids of the keys the server rejected, for the message. */
  failedIds: string[]
}

/** Whether a run of batch actions should stay disabled. */
export function isBatchBusy(busy: boolean, keys: string[]): boolean {
  return busy || keys.length === 0
}

/**
 * Runs `action` for each key, counting outcomes instead of stopping at the first
 * failure — a batch that aborts halfway leaves the operator unable to tell which
 * half landed.
 */
export async function runBatch(
  ids: string[],
  action: (id: string) => Promise<unknown>,
  label: (id: string) => string = (id) => id,
): Promise<KeyOutcome> {
  const outcome: KeyOutcome = { ok: 0, failed: 0, failedIds: [] }
  for (const id of ids) {
    try {
      await action(id)
      outcome.ok += 1
    } catch {
      outcome.failed += 1
      outcome.failedIds.push(label(id))
    }
  }
  return outcome
}

/** The message shown once a batch action finishes. */
export function batchSummary(verb: string, outcome: KeyOutcome): string {
  if (outcome.failed === 0) {
    return outcome.ok === 1
      ? `1 key ${verb}.`
      : `${outcome.ok} keys ${verb}.`
  }
  const base =
    outcome.ok === 0 ? `No key was ${verb}.` : `${outcome.ok} ${outcome.ok === 1 ? 'key was' : 'keys were'} ${verb}.`
  return `${base} ${outcome.failed} failed: ${outcome.failedIds.join(', ')}.`
}

/**
 * Whether the checkbox for one row is ticked. A key not in the list is not
 * selected; an empty list means nothing is selected rather than everything,
 * which is what `every()` on an empty array would otherwise report.
 */
export function isSelected(selectedIds: string[], id: string): boolean {
  return selectedIds.includes(id)
}

/** Whether the header checkbox should render as ticked. */
export function isAllSelected(selectedIds: string[], allIds: string[]): boolean {
  return allIds.length > 0 && selectedIds.length === allIds.length
}

/** The header checkbox state: ticked when all, indeterminate when some. */
export function selectAllState(
  selectedIds: string[],
  allIds: string[],
): 'none' | 'some' | 'all' {
  if (allIds.length === 0 || selectedIds.length === 0) return 'none'
  return isAllSelected(selectedIds, allIds) ? 'all' : 'some'
}

/**
 * Toggling the header checkbox selects everything, or clears the selection when
 * everything is already selected.
 */
export function toggleAll(selectedIds: string[], allIds: string[]): string[] {
  return isAllSelected(selectedIds, allIds) ? [] : [...allIds]
}

/** Toggling one row adds or removes it, leaving the rest untouched. */
export function toggleOne(selectedIds: string[], id: string): string[] {
  return selectedIds.includes(id)
    ? selectedIds.filter((current) => current !== id)
    : [...selectedIds, id]
}

// --- Bulk policy ------------------------------------------------------------
//
// A bulk edit over many keys is not "set these values on all": leaving a field
// alone must be expressible, or the dialog silently rewrites a limit the
// operator never looked at. Every numeric column is therefore tri-state.

/** How a bulk-policy column is treated: keep the stored value, set it, clear it. */
export type PolicyMode = 'keep' | 'set' | 'clear'

/** The numeric policy columns the bulk dialog can drive. */
export type NumericPolicyKey =
  | 'rateLimitRpm'
  | 'rateLimitTpm'
  | 'rateLimitConcurrency'
  | 'tokenQuota'
  | 'queueTimeoutMs'

/** A partial key policy as the API accepts it: omitted fields are left alone. */
export interface BulkPolicy {
  rateLimitRpm?: number
  rateLimitTpm?: number
  rateLimitConcurrency?: number
  tokenQuota?: number
  queueTimeoutMs?: number
  expiresAt?: string
}

/**
 * The values one bulk edit writes to every selected key.
 *
 * `keep` omits the column so the server retains the stored value — the only way
 * to edit one axis without resetting the others. `clear` writes the unrestricted
 * default of 0 (or an empty expiry). A non-finite or negative set value is
 * floored to 0 rather than refused, matching the per-key modal.
 */
export function buildBulkPolicy(input: {
  modes: Record<NumericPolicyKey, PolicyMode>
  values: Record<NumericPolicyKey, number>
  expiresMode: PolicyMode
  expiresAt: string
}): BulkPolicy {
  const policy: BulkPolicy = {}
  const keys: NumericPolicyKey[] = [
    'rateLimitRpm',
    'rateLimitTpm',
    'rateLimitConcurrency',
    'tokenQuota',
    'queueTimeoutMs',
  ]
  for (const key of keys) {
    const mode = input.modes[key]
    if (mode === 'keep') continue
    const raw = input.values[key]
    policy[key] = mode === 'clear' || !Number.isFinite(raw) || raw <= 0 ? 0 : Math.floor(raw)
  }
  if (input.expiresMode === 'set') policy.expiresAt = input.expiresAt
  else if (input.expiresMode === 'clear') policy.expiresAt = ''
  return policy
}

/**
 * Whether a key cannot serve a request it otherwise would: paused, expired, or
 * out of budget. The triage view over `GET /api/keys` is built on this.
 */
export function needsAttention(key: {
  isActive: number
  expiresAt?: string | null
  tokenQuota?: number | null
  usedTokens?: number | null
}): boolean {
  const expiry = key.expiresAt ?? ''
  const expired = expiry !== '' && new Date(expiry).getTime() < Date.now()
  const quota = key.tokenQuota ?? 0
  const spent = quota > 0 && (key.usedTokens ?? 0) >= quota
  return key.isActive !== 1 || expired || spent
}

/**
 * Whether a key carries no policy at all: no limits, no budget, no expiry.
 * The "did I forget to configure this?" view.
 */
export function isUnrestricted(key: {
  rateLimitRpm?: number | null
  rateLimitTpm?: number | null
  rateLimitConcurrency?: number | null
  tokenQuota?: number | null
  expiresAt?: string | null
}): boolean {
  return (
    (key.rateLimitRpm ?? 0) === 0 &&
    (key.rateLimitTpm ?? 0) === 0 &&
    (key.rateLimitConcurrency ?? 0) === 0 &&
    (key.tokenQuota ?? 0) === 0 &&
    (key.expiresAt ?? '') === ''
  )
}