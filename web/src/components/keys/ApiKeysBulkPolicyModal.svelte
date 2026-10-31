<!--
  ApiKeysBulkPolicyModal — one control surface for many keys at once.

  A per-key policy modal edits one key's usage limits, allowed models and expiry.
  Applying the same shape to twenty keys meant opening twenty dialogs, so this
  collapses the three governance axes into a single form over the current
  selection:

    • Usage limits    — RPM, TPM, concurrency, lifetime token quota, queue wait
    • Models allowed  — an allowlist written to every selected key
    • Active window   — an expiresAt stamped on every selected key

  Every field is tri-state, because a bulk edit is not the same as "set these
  values on all": leaving a field *unchanged* must be expressible, or the dialog
  silently rewrites a limit the operator never looked at. Each row carries its
  own switch:

    • "Leave unchanged" (default) — the column is omitted, the server keeps it
    • "Set"                        — the typed value is written to every key
    • "Clear" (where clearing has a meaning) — the column is reset to its
      unrestricted default (0, or an empty allowlist / expiry)

  The allowlist has no "unchanged" middle state in the API — setApiKeyModels
  replaces the whole list — so it is an explicit Set-or-Skip switch, and writing
  it is refused unless a list was actually typed.
-->
<script lang="ts">
  import { api, type APIKey, type APIKeyPolicy } from '../../api/client'
  import { notifications } from '../../lib/notifications'
  import {
    buildBulkPolicy,
    type PolicyMode,
    type NumericPolicyKey,
  } from './tableActions'
  import { Loader2, X } from 'lucide-svelte'

  interface Props {
    keys: APIKey[]
    onClose: () => void
    onSaved: () => void
  }

  let { keys, onClose, onSaved }: Props = $props()

  type Mode = PolicyMode

  // Usage limits. `keep` omits the column entirely, which is the only way to
  // edit one axis without resetting the others.
  let rpmMode = $state<Mode>('keep')
  let rpm = $state(0)
  let tpmMode = $state<Mode>('keep')
  let tpm = $state(0)
  let concMode = $state<Mode>('keep')
  let concurrency = $state(0)
  let quotaMode = $state<Mode>('keep')
  let tokenQuota = $state(0)
  let queueMode = $state<Mode>('keep')
  let queueTimeoutMs = $state(0)

  // When `set`, usage limits replace rather than add — an operator applying a
  // "100k budget" to a selection means each key gets 100k, not 100k more.
  let resetSpent = $state(false)

  // Active window.
  let expiresMode = $state<Mode>('keep')
  let expiresLocal = $state('')

  // Allowlist: set replaces every selected key's list with the one typed here.
  let modelsMode = $state<'keep' | 'set'>('keep')
  let models = $state<string[]>([])
  let modelDraft = $state('')

  let isSaving = $state(false)
  let error = $state<string | null>(null)

  function toRFC3339(local: string): string {
    if (!local) return ''
    const parsed = new Date(local)
    if (Number.isNaN(parsed.getTime())) return ''
    return parsed.toISOString()
  }

  function addModel() {
    const value = modelDraft.trim()
    if (!value || models.includes(value)) return
    models = [...models, value]
    modelDraft = ''
  }

  // Build the one policy every selected key receives. `keep` fields are left
  // out of the object so the server keeps the stored value; `clear` writes the
  // unrestricted default. The set of numeric columns is closed, so the modes and
  // values are handed to the shared builder rather than branched on here.
  function buildPolicy(): APIKeyPolicy {
    const modes: Record<NumericPolicyKey, PolicyMode> = {
      rateLimitRpm: rpmMode,
      rateLimitTpm: tpmMode,
      rateLimitConcurrency: concMode,
      tokenQuota: quotaMode,
      queueTimeoutMs: queueMode,
    }
    const values: Record<NumericPolicyKey, number> = {
      rateLimitRpm: rpm,
      rateLimitTpm: tpm,
      rateLimitConcurrency: concurrency,
      tokenQuota,
      queueTimeoutMs,
    }
    return buildBulkPolicy({
      modes,
      values,
      expiresMode,
      expiresAt: toRFC3339(expiresLocal),
    })
  }

  function touched(policy: APIKeyPolicy, modelsTouched: boolean): boolean {
    return Object.keys(policy).length > 0 || modelsTouched || resetSpent
  }

  async function save() {
    error = null
    const policy = buildPolicy()
    const writeModels = modelsMode === 'set'
    if (writeModels && models.length === 0) {
      error = 'Add at least one model pattern, or switch the allowlist to "Leave unchanged".'
      return
    }
    if (!touched(policy, writeModels)) {
      error = 'Nothing to apply — set at least one field, or use Cancel.'
      return
    }

    // One request per key, like every other batch here: no bulk endpoint, and a
    // key that rejects must not roll back the keys that already landed.
    const results = await Promise.allSettled(
      keys.map(async (k) => {
        if (Object.keys(policy).length > 0) await api.updateApiKeyPolicy(k.id, policy)
        if (writeModels) await api.setApiKeyModels(k.id, models)
        // Resetting the spent budget is a separate verb so a policy save that
        // only raises a ceiling never silently clears usage.
        if (resetSpent && (policy.tokenQuota ?? 0) > 0) await api.resetApiKeyQuota(k.id)
      }),
    )
    const failed = results.filter((r) => r.status === 'rejected')
    if (failed.length === 0) {
      notifications.success(`Policy applied to ${keys.length} key(s).`)
    } else {
      const first = failed[0] as PromiseRejectedResult
      notifications.error(
        `Applied to ${keys.length - failed.length} of ${keys.length}; ${failed.length} failed: ${
          first.reason instanceof Error ? first.reason.message : String(first.reason)
        }`,
      )
    }
    onSaved()
    onClose()
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }
</script>

<svelte:window onkeydown={onKeydown} />

<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-md p-4">
  <div
    class="w-full max-w-2xl max-h-[90vh] overflow-y-auto rounded-2xl bg-surface-2 border border-border shadow-2xl"
  >
    <div
      class="sticky top-0 z-10 flex items-center justify-between border-b border-border bg-surface-2 px-6 py-4"
    >
      <div class="min-w-0">
        <h2 class="font-headline text-sm font-bold text-text-main truncate">
          Bulk policy — {keys.length} key{keys.length === 1 ? '' : 's'}
        </h2>
        <p class="font-code text-[10px] text-text-subtle truncate">
          {keys.map((k) => k.name || k.keyDisplay || k.id).join(', ')}
        </p>
      </div>
      <button
        type="button"
        onclick={onClose}
        class="rounded-lg p-1.5 text-text-muted transition-colors hover:bg-surface-3 hover:text-text-main"
        aria-label="Close"
      >
        <X class="w-4 h-4" />
      </button>
    </div>

    <form class="space-y-5 px-6 py-5" onsubmit={(e) => (e.preventDefault(), save())}>
      <p class="text-xs text-text-subtle">
        Each field is independent. <strong>Leave unchanged</strong> keeps what a key already has;
        <strong>Set</strong> writes the value you type to every selected key; <strong>Clear</strong>
        resets it to unrestricted. Only the fields you move are written.
      </p>

      {#snippet modeSelect(value: Mode, onPick: (m: Mode) => void)}
        <select
          value={value}
          onchange={(e) => onPick((e.currentTarget as HTMLSelectElement).value as Mode)}
          class="rounded-lg border border-border bg-surface px-2 py-1.5 text-xs text-text-main outline-none focus:border-brand-500"
        >
          <option value="keep">Leave unchanged</option>
          <option value="set">Set</option>
          <option value="clear">Clear</option>
        </select>
      {/snippet}

      {#snippet numberField(
        label: string,
        mode: Mode,
        onPick: (m: Mode) => void,
        value: number,
        onValue: (n: number) => void,
        placeholder: string,
      )}
        <div class="flex items-center gap-2">
          <span class="w-40 shrink-0 text-xs text-text-muted">{label}</span>
          {@render modeSelect(mode, onPick)}
          <input
            type="number"
            min="0"
            value={value}
            oninput={(e) => onValue(Number((e.currentTarget as HTMLInputElement).value))}
            disabled={mode !== 'set'}
            placeholder={placeholder}
            class="w-28 rounded-lg border border-border bg-surface px-2 py-1.5 text-xs text-text-main outline-none focus:border-brand-500 disabled:opacity-50"
          />
        </div>
      {/snippet}

      <fieldset class="space-y-3 rounded-xl border border-border-subtle p-4">
        <legend class="px-1 text-xs font-semibold text-text-main">Usage limits</legend>
        {@render numberField('Requests / min (RPM)', rpmMode, (m) => (rpmMode = m), rpm, (n) => (rpm = n), 'unlimited')}
        {@render numberField('Tokens / min (TPM)', tpmMode, (m) => (tpmMode = m), tpm, (n) => (tpm = n), 'unlimited')}
        {@render numberField('Concurrency', concMode, (m) => (concMode = m), concurrency, (n) => (concurrency = n), 'unlimited')}
        {@render numberField('Lifetime token budget', quotaMode, (m) => (quotaMode = m), tokenQuota, (n) => (tokenQuota = n), 'unlimited')}
        {@render numberField('Queue wait (ms)', queueMode, (m) => (queueMode = m), queueTimeoutMs, (n) => (queueTimeoutMs = n), 'server default')}
        {#if quotaMode === 'set'}
          <label class="flex items-center gap-2 pl-40 text-xs text-text-muted">
            <input
              type="checkbox"
              bind:checked={resetSpent}
              class="size-3.5 rounded border-border/60 accent-[var(--primary)]"
            />
            Also reset each key's spent budget
          </label>
        {/if}
      </fieldset>

      <fieldset class="space-y-2 rounded-xl border border-border-subtle p-4">
        <legend class="px-1 text-xs font-semibold text-text-main">Models allowed</legend>
        <div class="flex items-center gap-2">
          <span class="w-40 shrink-0 text-xs text-text-muted">Allowlist</span>
          <select
            bind:value={modelsMode}
            class="rounded-lg border border-border bg-surface px-2 py-1.5 text-xs text-text-main outline-none focus:border-brand-500"
          >
            <option value="keep">Leave unchanged</option>
            <option value="set">Set (replace list)</option>
          </select>
        </div>
        {#if modelsMode === 'set'}
          <div class="flex gap-2 pl-40">
            <input
              type="text"
              bind:value={modelDraft}
              placeholder="provider/model or prefix*"
              onkeydown={(e) => (e.key === 'Enter' ? (e.preventDefault(), addModel()) : null)}
              class="flex-1 rounded-lg border border-border bg-surface px-2 py-1.5 font-code text-xs text-text-main outline-none focus:border-brand-500"
            />
            <button
              type="button"
              onclick={addModel}
              class="rounded-lg border border-border px-2 py-1.5 text-xs text-text-muted hover:text-text-main"
            >
              Add
            </button>
          </div>
          {#if models.length > 0}
            <div class="flex flex-wrap gap-1.5 pl-40">
              {#each models as pattern, i (pattern)}
                <span
                  class="flex items-center gap-1 rounded border border-border-subtle bg-surface px-1.5 py-0.5 font-code text-[10px] text-text-main"
                >
                  {pattern}
                  <button
                    type="button"
                    onclick={() => (models = models.filter((_, idx) => idx !== i))}
                    class="text-text-subtle hover:text-danger"
                    aria-label={`Remove ${pattern}`}
                  >
                    <X class="w-3 h-3" />
                  </button>
                </span>
              {/each}
            </div>
            <p class="pl-40 text-[11px] text-text-subtle">
              Every selected key is restricted to exactly these patterns.
            </p>
          {/if}
        {/if}
      </fieldset>

      <fieldset class="space-y-2 rounded-xl border border-border-subtle p-4">
        <legend class="px-1 text-xs font-semibold text-text-main">Active window</legend>
        <div class="flex items-center gap-2">
          <span class="w-40 shrink-0 text-xs text-text-muted">Expires at</span>
          {@render modeSelect(expiresMode, (m) => (expiresMode = m))}
          <input
            type="datetime-local"
            bind:value={expiresLocal}
            disabled={expiresMode !== 'set'}
            class="rounded-lg border border-border bg-surface px-2 py-1.5 text-xs text-text-main outline-none focus:border-brand-500 disabled:opacity-50"
          />
        </div>
      </fieldset>

      {#if error}
        <p class="text-xs text-danger">{error}</p>
      {/if}

      <div class="flex items-center justify-end gap-2 border-t border-border pt-4">
        <button
          type="button"
          onclick={onClose}
          class="rounded-lg border border-border px-3 py-1.5 text-xs text-text-muted transition-colors hover:bg-surface-3 hover:text-text-main"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={isSaving}
          class="flex items-center gap-1.5 rounded-lg bg-brand-500 px-3.5 py-1.5 text-xs font-semibold text-white transition hover:bg-brand-600 disabled:opacity-50"
        >
          {#if isSaving}
            <Loader2 class="w-3.5 h-3.5 animate-spin" />
            Applying…
          {:else}
            Apply to {keys.length} key{keys.length === 1 ? '' : 's'}
          {/if}
        </button>
      </div>
    </form>
  </div>
</div>
