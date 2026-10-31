<script lang="ts">
  import {
    Key,
    Activity,
    Shield,
    Users,
    Plus,
    RefreshCw,
    Loader2,
    X,
    Copy,
    Check,
    Coins,
  } from 'lucide-svelte'
  import ApiKeysTable from './ApiKeysTable.svelte'
  import { api, type APIKey } from '../../api/client'
  import { copyToClipboard } from '../../lib/clipboard'

  interface Props {
    apiKeys?: APIKey[]
    onRefresh?: () => void
  }

  let { apiKeys = [], onRefresh }: Props = $props()

  let isCreateKeyOpen = $state(false)
  let newKeyName = $state('')
  let newKeyCreator = $state('admin')
  let newKeyAllowedIps = $state('')
  let isSubmittingKey = $state(false)
  let newlyCreatedKey = $state<string | null>(null)
  let copiedId = $state<string | null>(null)

  // Derived metrics for summary cards
  const totalKeys = $derived(apiKeys.length)
  const activeKeysCount = $derived(apiKeys.filter((k) => k.isActive === 1).length)
  const inFlightCount = $derived(apiKeys.reduce((acc, k) => acc + (k.activeRequests ?? 0), 0))
  const activeIPsCount = $derived(
    new Set(apiKeys.flatMap((k) => k.activeIps ?? [])).size
  )
  const totalTokensSpent = $derived(
    apiKeys.reduce((acc, k) => acc + (k.usedTokens ?? 0), 0)
  )

  function formatTokens(n: number): string {
    if (n >= 1_000_000_000) return (n / 1_000_000_000).toFixed(1) + 'B'
    if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
    if (n >= 1_000) return (n / 1_000).toFixed(1) + 'k'
    return n.toLocaleString()
  }

  async function copy(text: string, id: string) {
    await copyToClipboard(text)
    copiedId = id
    setTimeout(() => {
      if (copiedId === id) copiedId = null
    }, 2000)
  }

  async function handleCreateKey(e: SubmitEvent) {
    e.preventDefault()
    if (!newKeyName.trim()) return

    isSubmittingKey = true
    try {
      const res = await api.createApiKey({
        name: newKeyName.trim(),
        createdBy: newKeyCreator.trim() || 'admin',
        allowedIps: newKeyAllowedIps.trim(),
      })
      if (res.key) {
        newlyCreatedKey = res.key
        newKeyName = ''
        newKeyCreator = 'admin'
        newKeyAllowedIps = ''
        isCreateKeyOpen = false
        onRefresh?.()
      }
    } catch (err) {
      alert(err instanceof Error ? err.message : String(err))
    } finally {
      isSubmittingKey = false
    }
  }
</script>

<div class="space-y-6">
  <!-- Top Stat Cards -->
  <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
    <!-- Card 1: Total Keys -->
    <div class="p-4 rounded-xl bg-surface-2 border border-border flex flex-col justify-between">
      <div class="flex items-center justify-between text-text-muted mb-2">
        <span class="text-xs font-semibold">Total API Keys</span>
        <Key class="w-4 h-4 text-brand-500" />
      </div>
      <div>
        <div class="text-2xl font-bold font-headline text-text-main">{totalKeys}</div>
        <div class="text-[11px] text-text-subtle mt-0.5">{activeKeysCount} active / {totalKeys - activeKeysCount} paused</div>
      </div>
    </div>

    <!-- Card 2: Live In-Flight Requests -->
    <div class="p-4 rounded-xl bg-surface-2 border border-border flex flex-col justify-between">
      <div class="flex items-center justify-between text-text-muted mb-2">
        <span class="text-xs font-semibold">In-Flight Requests</span>
        <Activity class="w-4 h-4 {inFlightCount > 0 ? 'text-emerald-400 animate-pulse' : 'text-text-subtle'}" />
      </div>
      <div>
        <div class="flex items-center gap-2">
          <span class="text-2xl font-bold font-headline {inFlightCount > 0 ? 'text-emerald-400' : 'text-text-main'}">
            {inFlightCount}
          </span>
          {#if inFlightCount > 0}
            <span class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-bold bg-emerald-500/20 text-emerald-400 animate-pulse">
              LIVE
            </span>
          {/if}
        </div>
        <div class="text-[11px] text-text-subtle mt-0.5">Active requests processing</div>
      </div>
    </div>

    <!-- Card 3: Active Caller IPs -->
    <div class="p-4 rounded-xl bg-surface-2 border border-border flex flex-col justify-between">
      <div class="flex items-center justify-between text-text-muted mb-2">
        <span class="text-xs font-semibold">Connected Client IPs</span>
        <Users class="w-4 h-4 text-cyan-400" />
      </div>
      <div>
        <div class="text-2xl font-bold font-headline text-text-main">{activeIPsCount}</div>
        <div class="text-[11px] text-text-subtle mt-0.5">Currently sending traffic</div>
      </div>
    </div>

    <!-- Card 4: Total Spent Tokens -->
    <div class="p-4 rounded-xl bg-surface-2 border border-border flex flex-col justify-between">
      <div class="flex items-center justify-between text-text-muted mb-2">
        <span class="text-xs font-semibold">Lifetime Tokens</span>
        <Coins class="w-4 h-4 text-amber-400" />
      </div>
      <div>
        <div class="text-2xl font-bold font-headline text-text-main">{formatTokens(totalTokensSpent)}</div>
        <div class="text-[11px] text-text-subtle mt-0.5">Prompt + completion tokens</div>
      </div>
    </div>
  </div>

  <!-- Main Table Card -->
  <ApiKeysTable
    {apiKeys}
    onRefresh={onRefresh}
    onCreate={() => (isCreateKeyOpen = true)}
  />
</div>

<!-- Modal: Create API Key -->
{#if isCreateKeyOpen}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4">
    <div class="w-full max-w-md p-6 rounded-2xl bg-surface border border-border shadow-2xl space-y-4">
      <div class="flex items-center justify-between pb-3 border-b border-border">
        <div class="flex items-center gap-2">
          <div class="p-1.5 rounded-lg bg-brand-500/10 text-brand-500">
            <Key class="w-4 h-4" />
          </div>
          <h3 class="text-base font-bold text-text-main">Create API Key</h3>
        </div>
        <button
          type="button"
          onclick={() => {
            isCreateKeyOpen = false
            newKeyName = ''
          }}
          class="text-text-muted hover:text-text-main cursor-pointer"
        >
          <X class="w-4 h-4" />
        </button>
      </div>

      <form onsubmit={handleCreateKey} class="space-y-4">
        <div class="space-y-1">
          <label for="key-name-input-dedicated" class="block text-xs font-semibold text-text-muted">
            Key Name
          </label>
          <input
            id="key-name-input-dedicated"
            type="text"
            bind:value={newKeyName}
            placeholder="e.g. Production Key, Cursor-Dev"
            class="w-full px-3 py-2 rounded-lg bg-bg border border-border text-sm text-text-main focus:outline-none focus:border-brand-500"
          />
        </div>

        <div class="space-y-1">
          <label for="key-creator-input-dedicated" class="block text-xs font-semibold text-text-muted">
            Created By / Owner
          </label>
          <input
            id="key-creator-input-dedicated"
            type="text"
            bind:value={newKeyCreator}
            placeholder="admin, devops, or username"
            class="w-full px-3 py-2 rounded-lg bg-bg border border-border text-sm text-text-main focus:outline-none focus:border-brand-500"
          />
        </div>

        <div class="space-y-1">
          <label for="key-ips-input-dedicated" class="block text-xs font-semibold text-text-muted">
            Allowed Client IPs / Subnets (Optional)
          </label>
          <input
            id="key-ips-input-dedicated"
            type="text"
            bind:value={newKeyAllowedIps}
            placeholder="e.g. 192.168.1.5, 10.0.0.0/16 (leave blank for any IP)"
            class="w-full px-3 py-2 rounded-lg bg-bg border border-border text-sm text-text-main focus:outline-none focus:border-brand-500 font-mono text-xs"
          />
          <p class="text-[11px] text-text-subtle">Leave empty to allow connections from any IP address.</p>
        </div>

        <div class="flex gap-2 pt-2">
          <button
            type="submit"
            disabled={!newKeyName.trim() || isSubmittingKey}
            class="flex-1 py-2 px-4 rounded-lg bg-brand-500 hover:bg-brand-600 disabled:opacity-50 text-white font-semibold text-xs transition cursor-pointer flex items-center justify-center gap-1.5"
          >
            {#if isSubmittingKey}
              <Loader2 class="w-3.5 h-3.5 animate-spin" />
              <span>Creating...</span>
            {:else}
              <Plus class="w-3.5 h-3.5" />
              <span>Create Key</span>
            {/if}
          </button>
          <button
            type="button"
            onclick={() => {
              isCreateKeyOpen = false
              newKeyName = ''
            }}
            class="flex-1 py-2 px-4 rounded-lg bg-surface-2 hover:bg-surface-3 text-text-main font-semibold text-xs transition cursor-pointer border border-border"
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- Modal: API Key Created -->
{#if newlyCreatedKey}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4">
    <div class="w-full max-w-md p-6 rounded-2xl bg-surface border border-border shadow-2xl space-y-4">
      <div class="flex items-center justify-between pb-3 border-b border-border">
        <h3 class="text-base font-bold text-text-main">API Key Created</h3>
      </div>

      <div class="bg-amber-500/10 border border-amber-500/20 rounded-xl p-4 space-y-1 text-amber-700 dark:text-amber-300">
        <p class="text-sm font-bold">Save this key now!</p>
        <p class="text-xs leading-relaxed">
          This key can be revealed and copied again from the table below, but treat it as a
          credential: anyone who reaches this dashboard can read it.
        </p>
      </div>

      <div class="flex items-center gap-2">
        <input
          type="text"
          value={newlyCreatedKey}
          readonly
          class="flex-1 font-mono text-xs bg-bg border border-border rounded-lg px-3 py-2 text-text-main select-all"
        />
        <button
          type="button"
          onclick={() => copy(newlyCreatedKey || '', 'created_key_dedicated')}
          class="flex items-center gap-1.5 px-3 py-2 rounded-lg bg-surface-2 hover:bg-surface-3 border border-border text-xs font-semibold text-text-main cursor-pointer"
        >
          {#if copiedId === 'created_key_dedicated'}
            <Check class="w-4 h-4 text-emerald-400" />
            <span>Copied!</span>
          {:else}
            <Copy class="w-4 h-4" />
            <span>Copy</span>
          {/if}
        </button>
      </div>

      <div class="pt-2">
        <button
          type="button"
          onclick={() => (newlyCreatedKey = null)}
          class="w-full py-2 px-4 rounded-lg bg-brand-500 hover:bg-brand-600 text-white font-semibold text-xs transition cursor-pointer"
        >
          Done
        </button>
      </div>
    </div>
  </div>
{/if}
