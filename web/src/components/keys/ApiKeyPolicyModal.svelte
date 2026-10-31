<script lang="ts">
  import {
    api,
    type APIKey,
    type APIKeyPolicy,
    type Plan,
    type Combo,
    type ProviderConnection,
    type ProviderNode,
    type APIKeyClient,
    type ModelRemaskRule,
  } from '../../api/client'
  import {
    Loader2,
    Save,
    X,
    Layers,
    Shield,
    Activity,
    Users,
    Key,
    RefreshCw,
    Plus,
    Check,
    Clock,
    Copy,
  } from 'lucide-svelte'
  import { isNavEntryHidden } from '../../lib/navVisibility.svelte'
  import { copyToClipboard } from '../../lib/clipboard'
  import ModelPickerModal from '../combos/ModelPickerModal.svelte'
  import { resolveModelPickerGroups } from '../combos/pickerData'
  import {
    EMPTY_PICKER_EXTRAS,
    loadPickerExtras,
    type ResolvedPickerExtras,
  } from '../combos/pickerExtras'

  interface Props {
    apiKey: APIKey
    onClose: () => void
    onSaved: () => void
  }

  let { apiKey, onClose, onSaved }: Props = $props()

  // Governance & limits
  let rpm = $state(apiKey.rateLimitRpm ?? 0)
  let tpm = $state(apiKey.rateLimitTpm ?? 0)
  let concurrency = $state(apiKey.rateLimitConcurrency ?? 0)
  let tokenQuota = $state(apiKey.tokenQuota ?? 0)
  let balance = $state(apiKey.balance ?? -1)
  let queueTimeoutMs = $state(apiKey.queueTimeoutMs ?? 0)
  let expiresLocal = $state(toLocalInput(apiKey.expiresAt))
  let metadata = $state(apiKey.metadata ?? '')
  let name = $state(apiKey.name ?? '')
  let planId = $state(apiKey.planId ?? '')

  // Creator & IP Management
  let createdBy = $state(apiKey.createdBy || 'admin')
  let allowedIps = $state(apiKey.allowedIps || '')

  // Tab State
  type TabType = 'overview' | 'clients' | 'security' | 'models' | 'governance'
  let activeTab = $state<TabType>('overview')

  // Live client telemetries
  let clientsData = $state<{
    apiKeyId: string
    activeRequests: number
    activeIps: string[]
    lastUsedIp: string
    lastUsedAt: string
    clients: APIKeyClient[]
  } | null>(null)
  let isLoadingClients = $state(false)
  let clientsError = $state<string | null>(null)
  let copiedIp = $state<string | null>(null)
  let isResettingQuota = $state(false)

  // Model Policy state
  let models = $state<string[]>([])
  let modelDraft = $state('')
  let isLoadingModels = $state(false)
  let isSaving = $state(false)
  let error = $state<string | null>(null)
  let hasLoadedModels = $state(false)
  let plans = $state<Plan[]>([])
  let isLoadingPlans = $state(false)
  let plansHidden = $derived(isNavEntryHidden('plans'))

  let connections = $state<ProviderConnection[]>([])
  let combos = $state<Combo[]>([])
  let providerNodes = $state<ProviderNode[]>([])
  let pickerExtras = $state<ResolvedPickerExtras>(EMPTY_PICKER_EXTRAS)
  let remaskRules = $state<ModelRemaskRule[]>([])
  let showPickerModal = $state(false)
  let isDropdownOpen = $state(false)
  let focusedIndex = $state(-1)
  let inputContainerEl = $state<HTMLDivElement | null>(null)

  let pickerGroups = $derived(
    resolveModelPickerGroups(connections, providerNodes, pickerExtras)
  )

  interface SuggestionItem {
    value: string
    label: string
    providerName: string
    isWildcard: boolean
  }

  let quickAddShortcuts = $derived.by(() => {
    const list: { pattern: string; label: string }[] = []
    if (!models.includes('*')) {
      list.push({ pattern: '*', label: '* (all)' })
    }
    // Quick add active remask virtual aliases
    for (const r of remaskRules) {
      if (r.enabled && r.alias && !models.includes(r.alias)) {
        list.push({ pattern: r.alias, label: `+${r.alias}` })
      }
    }
    const seen = new Set<string>()
    for (const group of pickerGroups) {
      if (!group.id) continue
      if (group.id.startsWith('openai-compatible') || group.id.length > 18) continue
      const pattern = `${group.id}/*`
      if (models.includes(pattern) || seen.has(pattern)) continue
      seen.add(pattern)
      list.push({ pattern, label: `+${group.id}/*` })
    }
    return list.slice(0, 8)
  })

  let allSuggestions = $derived.by(() => {
    const list: SuggestionItem[] = []
    list.push({
      value: '*',
      label: 'All models (*)',
      providerName: 'Global Wildcard',
      isWildcard: true,
    })
    for (const r of remaskRules) {
      if (r.alias) {
        list.push({
          value: r.alias,
          label: `${r.alias} (Remask Clone → ${r.target})`,
          providerName: 'Remask',
          isWildcard: false,
        })
      }
    }
    for (const group of pickerGroups) {
      if (group.id && !group.id.startsWith('openai-compatible-') && group.id.length <= 20) {
        list.push({
          value: `${group.id}/*`,
          label: `All ${group.name || group.id} models`,
          providerName: group.name || group.id,
          isWildcard: true,
        })
      }
      for (const m of group.models) {
        if (m.value && !m.value.includes('__placeholder__')) {
          list.push({
            value: m.value,
            label: m.name && m.name !== m.id ? `${m.name} (${m.id})` : m.id,
            providerName: group.name || group.id,
            isWildcard: false,
          })
        }
      }
    }
    for (const c of combos) {
      if (c.name) {
        list.push({
          value: c.name,
          label: c.name,
          providerName: 'Combo',
          isWildcard: false,
        })
      }
    }
    return list
  })

  let filteredSuggestions = $derived.by(() => {
    const q = modelDraft.trim().toLowerCase()
    const available = allSuggestions.filter((item) => !models.includes(item.value))
    if (!q) {
      return available.slice(0, 20)
    }
    return available
      .filter(
        (item) =>
          item.value.toLowerCase().includes(q) ||
          item.label.toLowerCase().includes(q) ||
          item.providerName.toLowerCase().includes(q)
      )
      .slice(0, 30)
  })

  async function loadAvailableSources() {
    try {
      const [connsRes, nodesRes, combosRes, extrasRes, remaskRes] = await Promise.all([
        api.getConnections().catch(() => []),
        api.getProviderNodes().catch(() => []),
        api.getCombos().catch(() => []),
        loadPickerExtras(api).catch(() => EMPTY_PICKER_EXTRAS),
        api.getModelRemaskRules().catch(() => [] as ModelRemaskRule[]),
      ])
      connections = connsRes
      providerNodes = nodesRes
      combos = combosRes
      pickerExtras = extrasRes
      remaskRules = remaskRes || []
    } catch {}
  }

  function selectSuggestion(val: string) {
    const clean = val.trim()
    if (!clean || models.includes(clean)) return
    models = [...models, clean]
    modelDraft = ''
    isDropdownOpen = false
    focusedIndex = -1
  }

  function handleDraftKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      if (!isDropdownOpen) {
        isDropdownOpen = true
        focusedIndex = 0
      } else {
        focusedIndex = (focusedIndex + 1) % Math.max(1, filteredSuggestions.length)
      }
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      if (!isDropdownOpen) {
        isDropdownOpen = true
        focusedIndex = filteredSuggestions.length - 1
      } else {
        focusedIndex = (focusedIndex - 1 + filteredSuggestions.length) % Math.max(1, filteredSuggestions.length)
      }
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (isDropdownOpen && focusedIndex >= 0 && filteredSuggestions[focusedIndex]) {
        selectSuggestion(filteredSuggestions[focusedIndex].value)
      } else {
        addModel()
      }
    } else if (e.key === 'Escape') {
      if (isDropdownOpen) {
        e.stopPropagation()
        isDropdownOpen = false
      }
    }
  }

  function toLocalInput(rfc3339?: string): string {
    if (!rfc3339) return ''
    const parsed = new Date(rfc3339)
    if (Number.isNaN(parsed.getTime())) return ''
    const pad = (n: number) => String(n).padStart(2, '0')
    return `${parsed.getFullYear()}-${pad(parsed.getMonth() + 1)}-${pad(parsed.getDate())}T${pad(parsed.getHours())}:${pad(parsed.getMinutes())}`
  }

  function toRFC3339(local: string): string {
    if (!local) return ''
    const parsed = new Date(local)
    if (Number.isNaN(parsed.getTime())) return ''
    return parsed.toISOString()
  }

  function nonNegative(value: number): number {
    return Number.isFinite(value) && value > 0 ? Math.floor(value) : 0
  }

  async function loadModels() {
    isLoadingModels = true
    try {
      const res = await api.getApiKeyModels(apiKey.id)
      models = res.models ?? []
      hasLoadedModels = true
      error = null
    } catch (err) {
      error = `Could not load model allowlist: ${err instanceof Error ? err.message : String(err)}`
    } finally {
      isLoadingModels = false
    }
  }

  async function loadPlans() {
    isLoadingPlans = true
    try {
      const res = await api.getPlans()
      plans = res.plans ?? []
    } catch {
      plans = []
    } finally {
      isLoadingPlans = false
    }
  }

  async function loadClients() {
    isLoadingClients = true
    clientsError = null
    try {
      const res = await api.getApiKeyClients(apiKey.id)
      clientsData = res
    } catch (err) {
      clientsError = err instanceof Error ? err.message : String(err)
    } finally {
      isLoadingClients = false
    }
  }

  async function handleResetQuota() {
    if (isResettingQuota) return
    isResettingQuota = true
    try {
      await api.resetApiKeyQuota(apiKey.id)
      apiKey.usedTokens = 0
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      isResettingQuota = false
    }
  }

  function addIpToWhitelist(ipToAdd: string) {
    const clean = ipToAdd.trim()
    if (!clean) return
    const current = allowedIps
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
    if (!current.includes(clean)) {
      current.push(clean)
      allowedIps = current.join(', ')
    }
    activeTab = 'security'
  }

  async function handleCopyIp(ipStr: string) {
    await copyToClipboard(ipStr)
    copiedIp = ipStr
    setTimeout(() => {
      if (copiedIp === ipStr) copiedIp = null
    }, 2000)
  }

  function parseClientTool(ua: string): string {
    if (!ua) return 'Direct / Unknown'
    const lower = ua.toLowerCase()
    if (lower.includes('cursor')) return 'Cursor'
    if (lower.includes('claude-code') || lower.includes('claudecode')) return 'Claude Code'
    if (lower.includes('cline')) return 'Cline'
    if (lower.includes('continue')) return 'Continue'
    if (lower.includes('aider')) return 'Aider'
    if (lower.includes('python-requests') || lower.includes('python')) return 'Python Script'
    if (lower.includes('curl')) return 'cURL CLI'
    if (lower.includes('mozilla') || lower.includes('chrome')) return 'Web Browser'
    return ua.slice(0, 30)
  }

  $effect(() => {
    if (!hasLoadedModels) loadModels()
    if (!plansHidden) void loadPlans()
    void loadAvailableSources()
    void loadClients()
  })

  async function save(e: SubmitEvent) {
    e.preventDefault()
    error = null

    const metadataValue = metadata.trim()
    if (metadataValue) {
      try {
        const parsed = JSON.parse(metadataValue)
        if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
          error = 'Metadata must be a JSON object, e.g. {"customerId":"acme"}'
          return
        }
      } catch {
        error = 'Metadata is not valid JSON'
        return
      }
    }

    if (!hasLoadedModels) {
      error = isLoadingModels
        ? 'Still loading the model allowlist — try Save again in a moment.'
        : 'The model allowlist could not be read. Press Reload, then Save.'
      return
    }

    try {
      isSaving = true
      const policy: APIKeyPolicy = {
        rateLimitRpm: nonNegative(rpm),
        rateLimitTpm: nonNegative(tpm),
        rateLimitConcurrency: nonNegative(concurrency),
        tokenQuota: nonNegative(tokenQuota),
        queueTimeoutMs: nonNegative(queueTimeoutMs),
        expiresAt: toRFC3339(expiresLocal),
        metadata: metadataValue,
        name: name.trim(),
        planId,
        createdBy: createdBy.trim() || 'admin',
        allowedIps: allowedIps.trim(),
      }

      await api.updateApiKeyPolicy(apiKey.id, policy)
      await api.setApiKeyModels(apiKey.id, models)
      if (balance !== (apiKey.balance ?? -1)) {
        await fetch('/api/billing/topup', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ apiKeyId: apiKey.id, balance: Number(balance) }),
        }).catch(() => null)
      }

      onSaved()
      onClose()
    } catch (err) {
      error = err instanceof Error ? err.message : String(err)
    } finally {
      isSaving = false
    }
  }

  function addModel() {
    const value = modelDraft.trim()
    if (!value || models.includes(value)) return
    models = [...models, value]
    modelDraft = ''
  }

  function removeModel(pattern: string) {
    models = models.filter((p) => p !== pattern)
  }

  function handleSelectFromPicker(val: string) {
    const clean = val.trim()
    if (clean && !models.includes(clean)) {
      models = [...models, clean]
    }
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }

  const activeReqs = $derived(clientsData?.activeRequests ?? apiKey.activeRequests ?? 0)
  const activeIPs = $derived(clientsData?.activeIps ?? apiKey.activeIps ?? [])
  const quotaPercent = $derived(
    tokenQuota > 0 ? Math.min(100, Math.round(((apiKey.usedTokens ?? 0) / tokenQuota) * 100)) : 0
  )
</script>

<svelte:window
  onkeydown={onKeydown}
  onclick={(e) => {
    const target = e.target as HTMLElement | null
    if (inputContainerEl && !inputContainerEl.contains(target)) {
      isDropdownOpen = false
    }
  }}
/>

<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-md p-4">
  <div class="w-full max-w-3xl max-h-[92vh] flex flex-col rounded-2xl bg-surface-2 border border-border shadow-2xl overflow-hidden">
    <!-- Modal Header -->
    <div class="bg-surface-2 border-b border-border px-6 py-4 flex items-center justify-between shrink-0">
      <div class="min-w-0">
        <div class="flex items-center gap-2">
          <div class="p-1.5 rounded-lg bg-brand-500/10 text-brand-500">
            <Key class="w-4 h-4" />
          </div>
          <h2 class="font-headline text-sm font-bold text-text-main truncate">
            API Key Manager — {name || apiKey.keyDisplay || apiKey.key}
          </h2>
          {#if activeReqs > 0}
            <span class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[10px] font-bold bg-emerald-500/20 text-emerald-400 border border-emerald-500/30 animate-pulse">
              <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
              {activeReqs} in-flight
            </span>
          {/if}
        </div>
        <p class="font-code text-[10px] text-text-subtle truncate mt-0.5 ml-7">{apiKey.id}</p>
      </div>

      <button
        type="button"
        onclick={onClose}
        aria-label="Close dialog"
        class="p-1.5 rounded-lg text-text-subtle hover:text-text-main hover:bg-surface-3 transition cursor-pointer"
      >
        <X class="w-4 h-4" />
      </button>
    </div>

    <!-- Navigation Tabs -->
    <div class="flex items-center gap-1 px-6 border-b border-border bg-surface-2/40 overflow-x-auto text-xs shrink-0 select-none">
      <button
        type="button"
        onclick={() => (activeTab = 'overview')}
        class="px-3.5 py-2.5 border-b-2 font-medium flex items-center gap-1.5 transition-colors cursor-pointer {activeTab === 'overview' ? 'border-brand-500 text-brand-500 font-semibold' : 'border-transparent text-text-muted hover:text-text-main'}"
      >
        <span class="material-symbols-outlined text-[16px]">monitoring</span>
        Live Activity
        {#if activeReqs > 0}
          <span class="px-1.5 py-0.2 rounded-full text-[9px] bg-emerald-500/20 text-emerald-400 font-bold">{activeReqs}</span>
        {/if}
      </button>

      <button
        type="button"
        onclick={() => (activeTab = 'clients')}
        class="px-3.5 py-2.5 border-b-2 font-medium flex items-center gap-1.5 transition-colors cursor-pointer {activeTab === 'clients' ? 'border-brand-500 text-brand-500 font-semibold' : 'border-transparent text-text-muted hover:text-text-main'}"
      >
        <span class="material-symbols-outlined text-[16px]">devices</span>
        Connected Clients &amp; IPs
        {#if clientsData?.clients?.length}
          <span class="px-1.5 py-0.2 rounded-full text-[9px] bg-surface-3 text-text-muted font-mono">{clientsData.clients.length}</span>
        {/if}
      </button>

      <button
        type="button"
        onclick={() => (activeTab = 'security')}
        class="px-3.5 py-2.5 border-b-2 font-medium flex items-center gap-1.5 transition-colors cursor-pointer {activeTab === 'security' ? 'border-brand-500 text-brand-500 font-semibold' : 'border-transparent text-text-muted hover:text-text-main'}"
      >
        <span class="material-symbols-outlined text-[16px]">security</span>
        IP Security
        {#if allowedIps.trim()}
          <span class="px-1.5 py-0.2 rounded-full text-[9px] bg-amber-500/20 text-amber-400 font-mono">Restricted</span>
        {/if}
      </button>

      <button
        type="button"
        onclick={() => (activeTab = 'models')}
        class="px-3.5 py-2.5 border-b-2 font-medium flex items-center gap-1.5 transition-colors cursor-pointer {activeTab === 'models' ? 'border-brand-500 text-brand-500 font-semibold' : 'border-transparent text-text-muted hover:text-text-main'}"
      >
        <span class="material-symbols-outlined text-[16px]">smart_toy</span>
        Model Access
        {#if models.length > 0}
          <span class="px-1.5 py-0.2 rounded-full text-[9px] bg-surface-3 text-text-muted font-mono">{models.length}</span>
        {/if}
      </button>

      <button
        type="button"
        onclick={() => (activeTab = 'governance')}
        class="px-3.5 py-2.5 border-b-2 font-medium flex items-center gap-1.5 transition-colors cursor-pointer {activeTab === 'governance' ? 'border-brand-500 text-brand-500 font-semibold' : 'border-transparent text-text-muted hover:text-text-main'}"
      >
        <span class="material-symbols-outlined text-[16px]">speed</span>
        Rate Limits &amp; Quotas
      </button>
    </div>

    <!-- Form Content Area -->
    <form onsubmit={save} class="flex-1 overflow-y-auto p-6 space-y-6 text-xs font-body">
      {#if error}
        <div class="p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-red-500 text-xs">
          {error}
        </div>
      {/if}

      <!-- TAB 1: OVERVIEW & LIVE ACTIVITY -->
      {#if activeTab === 'overview'}
        <div class="space-y-5">
          <!-- Live Status Card -->
          <div class="p-4 rounded-xl border {activeReqs > 0 ? 'bg-emerald-500/10 border-emerald-500/30' : 'bg-surface border-border'} space-y-3">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2">
                {#if activeReqs > 0}
                  <span class="relative flex h-3 w-3">
                    <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                    <span class="relative inline-flex rounded-full h-3 w-3 bg-emerald-500"></span>
                  </span>
                  <span class="text-sm font-bold text-emerald-400">
                    Currently In-Use: {activeReqs} Active Request{activeReqs > 1 ? 's' : ''}
                  </span>
                {:else}
                  <span class="w-2.5 h-2.5 rounded-full bg-slate-400"></span>
                  <span class="text-sm font-semibold text-text-main">
                    Idle — Key Ready for Inbound Calls
                  </span>
                {/if}
              </div>
              <span class="text-[11px] font-mono text-text-subtle">
                Concurrency: {concurrency > 0 ? `${activeReqs} / ${concurrency} limit` : `${activeReqs} (unlimited)`}
              </span>
            </div>

            {#if activeIPs.length > 0}
              <div class="pt-2 border-t border-border/50 text-[11px]">
                <p class="text-text-muted font-medium mb-1">Active Caller IPs right now:</p>
                <div class="flex flex-wrap gap-1.5">
                  {#each activeIPs as ip}
                    <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-300 font-mono text-[10px]">
                      <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
                      {ip}
                    </span>
                  {/each}
                </div>
              </div>
            {/if}
          </div>

          <!-- Token Quota & Spend Overview -->
          {#if (tokenQuota > 0 || (apiKey.usedTokens ?? 0) > 0)}
            <div class="p-4 rounded-xl bg-surface border border-border space-y-2">
              <div class="flex items-center justify-between text-xs">
                <span class="font-semibold text-text-main">Lifetime Token Budget</span>
                <button
                  type="button"
                  disabled={isResettingQuota}
                  onclick={handleResetQuota}
                  class="text-[11px] text-brand-500 hover:underline flex items-center gap-1 cursor-pointer disabled:opacity-50"
                >
                  <RefreshCw class="w-3 h-3 {isResettingQuota ? 'animate-spin' : ''}" />
                  Reset Quota Spend
                </button>
              </div>
              <div class="w-full h-2 rounded-full bg-surface-3 overflow-hidden">
                <div
                  class="h-full transition-all {quotaPercent >= 90 ? 'bg-danger' : quotaPercent >= 70 ? 'bg-amber-500' : 'bg-brand-500'}"
                  style="width: {quotaPercent}%"
                ></div>
              </div>
              <div class="flex justify-between text-[11px] text-text-subtle">
                <span>Spent: {(apiKey.usedTokens ?? 0).toLocaleString()} tokens</span>
                <span>Quota: {tokenQuota > 0 ? `${tokenQuota.toLocaleString()} tokens` : 'Unlimited'}</span>
              </div>
            </div>
          {/if}

          <!-- Identity & Metadata -->
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div class="space-y-1.5">
              <label for="key-name-field" class="block font-semibold text-text-muted">Key Name</label>
              <input
                id="key-name-field"
                type="text"
                bind:value={name}
                maxlength="200"
                placeholder="Client Token"
                class="w-full bg-surface border border-border rounded-lg px-3 py-2 text-xs text-text-main focus:outline-none focus:border-brand-500"
              />
            </div>

            <div class="space-y-1.5">
              <label for="key-creator-field" class="block font-semibold text-text-muted">Created By / Owner</label>
              <input
                id="key-creator-field"
                type="text"
                bind:value={createdBy}
                placeholder="admin, devops, developer"
                class="w-full bg-surface border border-border rounded-lg px-3 py-2 text-xs text-text-main focus:outline-none focus:border-brand-500"
              />
            </div>
          </div>

          <!-- Quick Telemetry Stats -->
          <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 pt-2">
            <div class="p-3 rounded-lg bg-surface border border-border/60">
              <span class="text-[10px] text-text-subtle uppercase tracking-wider block">Total Requests</span>
              <span class="text-sm font-bold text-text-main mt-0.5 block font-mono">
                {(apiKey.usedCount ?? 0).toLocaleString()}
              </span>
            </div>
            <div class="p-3 rounded-lg bg-surface border border-border/60">
              <span class="text-[10px] text-text-subtle uppercase tracking-wider block">Last Used IP</span>
              <span class="text-xs font-semibold text-text-main mt-0.5 block font-mono truncate" title={clientsData?.lastUsedIp || apiKey.lastUsedIp || 'Never'}>
                {clientsData?.lastUsedIp || apiKey.lastUsedIp || 'Never'}
              </span>
            </div>
            <div class="p-3 rounded-lg bg-surface border border-border/60">
              <span class="text-[10px] text-text-subtle uppercase tracking-wider block">Created</span>
              <span class="text-xs font-medium text-text-main mt-0.5 block truncate">
                {apiKey.createdAt ? new Date(apiKey.createdAt).toLocaleDateString() : '—'}
              </span>
            </div>
            <div class="p-3 rounded-lg bg-surface border border-border/60">
              <span class="text-[10px] text-text-subtle uppercase tracking-wider block">Last Seen</span>
              <span class="text-xs font-medium text-text-main mt-0.5 block truncate">
                {apiKey.lastUsedAt ? new Date(apiKey.lastUsedAt).toLocaleTimeString() : 'Never'}
              </span>
            </div>
          </div>
        </div>
      {/if}

      <!-- TAB 2: CONNECTED CLIENTS & IP AUDIT -->
      {#if activeTab === 'clients'}
        <div class="space-y-4">
          <div class="flex items-center justify-between">
            <div>
              <h3 class="font-headline text-xs font-bold text-text-main">Caller IP History &amp; Diagnostics</h3>
              <p class="text-text-subtle mt-0.5">
                All client machines and IPs that have made API requests with this token.
              </p>
            </div>
            <button
              type="button"
              onclick={loadClients}
              disabled={isLoadingClients}
              class="flex items-center gap-1.5 px-2.5 py-1.5 rounded-lg border border-border bg-surface hover:bg-surface-3 text-text-main text-xs transition cursor-pointer disabled:opacity-50"
            >
              <RefreshCw class="w-3.5 h-3.5 {isLoadingClients ? 'animate-spin' : ''}" />
              Refresh
            </button>
          </div>

          {#if isLoadingClients && !clientsData}
            <div class="text-center py-8 text-text-subtle flex items-center justify-center gap-2">
              <Loader2 class="w-4 h-4 animate-spin text-brand-500" />
              <span>Fetching connected clients...</span>
            </div>
          {:else if clientsData?.clients && clientsData.clients.length > 0}
            <div class="border border-border rounded-xl overflow-hidden bg-surface">
              <table class="w-full text-left text-xs">
                <thead>
                  <tr class="border-b border-border bg-surface-2 text-text-subtle font-code text-[10px] uppercase">
                    <th class="py-2.5 px-3">Status</th>
                    <th class="py-2.5 px-3">IP Address</th>
                    <th class="py-2.5 px-3">Client Tool</th>
                    <th class="py-2.5 px-3 text-right">Requests</th>
                    <th class="py-2.5 px-3">Last Seen</th>
                    <th class="py-2.5 px-3 text-right">Action</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-border/50">
                  {#each clientsData.clients as client (client.ip)}
                    <tr class="hover:bg-surface-2/40 transition">
                      <td class="py-2.5 px-3">
                        {#if client.isLive}
                          <span class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded-full text-[9px] font-bold bg-emerald-500/20 text-emerald-400 border border-emerald-500/30 animate-pulse">
                            <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
                            Live
                          </span>
                        {:else}
                          <span class="text-text-subtle text-[10px] font-mono">Idle</span>
                        {/if}
                      </td>

                      <td class="py-2.5 px-3">
                        <div class="flex items-center gap-1.5">
                          <code class="font-mono text-xs text-text-main font-semibold">{client.ip}</code>
                          <button
                            type="button"
                            onclick={() => handleCopyIp(client.ip)}
                            class="p-1 rounded text-text-subtle hover:text-text-main hover:bg-surface-2 transition cursor-pointer"
                            title="Copy IP"
                          >
                            {#if copiedIp === client.ip}
                              <Check class="w-3 h-3 text-emerald-400" />
                            {:else}
                              <Copy class="w-3 h-3" />
                            {/if}
                          </button>
                        </div>
                      </td>

                      <td class="py-2.5 px-3">
                        <span class="px-2 py-0.5 rounded bg-surface-2 border border-border text-[10px] text-text-muted">
                          {parseClientTool(client.userAgent)}
                        </span>
                      </td>

                      <td class="py-2.5 px-3 text-right font-mono text-xs text-text-main">
                        {client.requestCount.toLocaleString()}
                      </td>

                      <td class="py-2.5 px-3 text-text-subtle text-[11px]">
                        {client.lastSeenAt ? new Date(client.lastSeenAt).toLocaleString() : '—'}
                      </td>

                      <td class="py-2.5 px-3 text-right">
                        <button
                          type="button"
                          onclick={() => addIpToWhitelist(client.ip)}
                          class="px-2 py-1 rounded bg-brand-500/10 hover:bg-brand-500/20 text-brand-500 text-[10px] font-medium border border-brand-500/20 transition cursor-pointer"
                        >
                          + Whitelist
                        </button>
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {:else}
            <div class="p-8 text-center bg-surface border border-border rounded-xl space-y-2">
              <span class="material-symbols-outlined text-3xl text-text-subtle">radar</span>
              <p class="text-text-main font-semibold text-xs">No client connection history yet</p>
              <p class="text-[11px] text-text-subtle max-w-sm mx-auto">
                When tools like Cursor, Claude Code, or curl send requests with this API key, their IP addresses and client signatures will be recorded here automatically.
              </p>
            </div>
          {/if}
        </div>
      {/if}

      <!-- TAB 3: IP SECURITY & WHITELIST -->
      {#if activeTab === 'security'}
        <div class="space-y-5">
          <div>
            <h3 class="font-headline text-xs font-bold text-text-main">IP Access Restriction (Allowlist)</h3>
            <p class="text-text-subtle mt-0.5">
              Only incoming requests originating from these IP addresses will be authenticated. Requests from unlisted IPs will be rejected with HTTP 403 Forbidden.
            </p>
          </div>

          <div class="space-y-2">
            <label for="allowed-ips-input" class="block font-semibold text-text-muted">Allowed IPs or CIDR Subnets</label>
            <textarea
              id="allowed-ips-input"
              bind:value={allowedIps}
              rows="3"
              placeholder="e.g. 192.168.1.50, 10.0.0.0/16, 203.0.113.10"
              class="w-full bg-surface border border-border rounded-lg px-3 py-2 text-xs font-mono text-text-main focus:outline-none focus:border-brand-500"
            ></textarea>
            <p class="text-[11px] text-text-subtle">
              Separate multiple entries with commas. Leave blank to allow connections from any IP address.
            </p>
          </div>

          <!-- Quick Preset Buttons -->
          <div class="p-3.5 rounded-xl bg-surface border border-border space-y-2">
            <span class="text-[11px] font-semibold text-text-main block">Quick Security Presets</span>
            <div class="flex flex-wrap gap-2">
              <button
                type="button"
                onclick={() => (allowedIps = '127.0.0.1, ::1')}
                class="px-2.5 py-1 rounded bg-surface-2 hover:bg-surface-3 border border-border text-[11px] text-text-muted hover:text-text-main transition cursor-pointer"
              >
                Localhost Only (127.0.0.1)
              </button>
              <button
                type="button"
                onclick={() => (allowedIps = '10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16')}
                class="px-2.5 py-1 rounded bg-surface-2 hover:bg-surface-3 border border-border text-[11px] text-text-muted hover:text-text-main transition cursor-pointer"
              >
                Private Subnets (RFC 1918)
              </button>
              <button
                type="button"
                onclick={() => (allowedIps = '')}
                class="px-2.5 py-1 rounded bg-red-500/10 hover:bg-red-500/20 border border-red-500/20 text-[11px] text-red-400 transition cursor-pointer"
              >
                Clear Restrictions (Allow All)
              </button>
            </div>
          </div>
        </div>
      {/if}

      <!-- TAB 4: MODEL ACCESS -->
      {#if activeTab === 'models'}
        <div class="space-y-4">
          <div class="flex items-center justify-between">
            <div>
              <h3 class="font-headline text-xs font-bold text-text-main">Model Access Control</h3>
              <p class="text-text-subtle mt-0.5">
                Allowed patterns for this key. Empty allowlist means every model is permitted.
              </p>
            </div>
            <button
              type="button"
              onclick={() => (showPickerModal = true)}
              class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-brand-500/10 hover:bg-brand-500/20 border border-brand-500/30 text-brand-500 text-xs font-semibold transition cursor-pointer"
            >
              <Layers class="w-3.5 h-3.5" />
              <span>Browse Catalog</span>
            </button>
          </div>

          <!-- Quick Add Bar -->
          {#if quickAddShortcuts.length > 0}
            <div class="flex flex-wrap items-center gap-1.5">
              <span class="text-[11px] text-text-subtle">Quick add:</span>
              {#each quickAddShortcuts as item}
                <button
                  type="button"
                  onclick={() => selectSuggestion(item.pattern)}
                  class="px-2 py-0.5 rounded bg-surface border border-border hover:border-brand-500 text-[10px] text-text-muted hover:text-brand-500 transition cursor-pointer font-mono"
                >
                  {item.label}
                </button>
              {/each}
            </div>
          {/if}

          <!-- Search & Autocomplete Input -->
          <div class="relative" bind:this={inputContainerEl}>
            <div class="flex gap-2">
              <input
                type="text"
                bind:value={modelDraft}
                onfocus={() => (isDropdownOpen = true)}
                onkeydown={handleDraftKeydown}
                placeholder="Type prefix or model (e.g. github/*, gpt-4o, claude-3)..."
                class="flex-1 bg-surface border border-border rounded-lg px-3 py-2 text-xs font-mono text-text-main focus:outline-none focus:border-brand-500"
              />
              <button
                type="button"
                onclick={addModel}
                disabled={!modelDraft.trim()}
                class="px-3.5 py-2 rounded-lg bg-surface border border-border hover:bg-surface-3 text-text-main text-xs font-semibold transition cursor-pointer disabled:opacity-40"
              >
                Add
              </button>
            </div>

            {#if isDropdownOpen && filteredSuggestions.length > 0}
              <div class="absolute left-0 right-0 top-full mt-1 max-h-56 overflow-y-auto rounded-xl bg-surface border border-border shadow-xl z-20 divide-y divide-border/40">
                {#each filteredSuggestions as suggestion, idx}
                  <button
                    type="button"
                    onclick={() => selectSuggestion(suggestion.value)}
                    class="w-full text-left px-3 py-2 text-xs flex items-center justify-between hover:bg-surface-2 transition cursor-pointer {idx === focusedIndex ? 'bg-surface-2' : ''}"
                  >
                    <div class="min-w-0">
                      <span class="font-mono text-text-main font-semibold truncate block">{suggestion.value}</span>
                      <span class="text-[10px] text-text-subtle truncate block">{suggestion.label}</span>
                    </div>
                    <span class="text-[9px] px-1.5 py-0.5 rounded bg-surface-3 text-text-muted font-mono shrink-0 ml-2">
                      {suggestion.providerName}
                    </span>
                  </button>
                {/each}
              </div>
            {/if}
          </div>

          <!-- Allowed Models List -->
          <div class="space-y-2 pt-2">
            {#if models.length === 0}
              <div class="p-4 rounded-xl bg-surface/50 border border-border/60 text-center text-text-subtle text-xs">
                No allowlist configured — every registered model is permitted.
              </div>
            {:else}
              <div class="flex flex-wrap gap-2">
                {#each models as pattern}
                  <span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-surface border border-border text-text-main font-mono text-xs">
                    <span>{pattern}</span>
                    <button
                      type="button"
                      onclick={() => removeModel(pattern)}
                      class="text-text-subtle hover:text-red-400 p-0.5 rounded cursor-pointer"
                    >
                      <X class="w-3 h-3" />
                    </button>
                  </span>
                {/each}
              </div>
            {/if}
          </div>
        </div>
      {/if}

      <!-- TAB 5: RATE LIMITS & GOVERNANCE -->
      {#if activeTab === 'governance'}
        <div class="space-y-5">
          {#if !plansHidden}
            <div class="space-y-1.5">
              <label for="key-plan-select" class="block font-semibold text-text-muted">Inherit from Plan</label>
              <select
                id="key-plan-select"
                bind:value={planId}
                class="w-full bg-surface border border-border rounded-lg px-3 py-2 text-xs text-text-main focus:outline-none focus:border-brand-500"
              >
                <option value="">No plan — use custom key policy</option>
                {#each plans as plan}
                  <option value={plan.id}>{plan.name}</option>
                {/each}
              </select>
            </div>
          {/if}

          <!-- Rates -->
          <div>
            <h4 class="font-headline text-xs font-bold text-text-main mb-2">Window Rate Limits</h4>
            <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <div class="space-y-1">
                <label for="rpm-input" class="text-[11px] text-text-muted">Requests / min</label>
                <input
                  id="rpm-input"
                  type="number"
                  min="0"
                  bind:value={rpm}
                  class="w-full bg-surface border border-border rounded-lg px-3 py-1.5 text-xs text-text-main font-mono focus:outline-none focus:border-brand-500"
                />
              </div>

              <div class="space-y-1">
                <label for="tpm-input" class="text-[11px] text-text-muted">Tokens / min</label>
                <input
                  id="tpm-input"
                  type="number"
                  min="0"
                  bind:value={tpm}
                  class="w-full bg-surface border border-border rounded-lg px-3 py-1.5 text-xs text-text-main font-mono focus:outline-none focus:border-brand-500"
                />
              </div>

              <div class="space-y-1">
                <label for="concurrency-input" class="text-[11px] text-text-muted">Max Concurrency</label>
                <input
                  id="concurrency-input"
                  type="number"
                  min="0"
                  bind:value={concurrency}
                  class="w-full bg-surface border border-border rounded-lg px-3 py-1.5 text-xs text-text-main font-mono focus:outline-none focus:border-brand-500"
                />
              </div>
            </div>
          </div>

          <!-- Lifetime Quota & Timeout -->
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div class="space-y-1 sm:col-span-2">
              <label for="balance-input" class="text-[11px] font-semibold text-emerald-400">Prepaid Saldo (Rp / IDR, -1 = unlimited)</label>
              <input
                id="balance-input"
                type="number"
                bind:value={balance}
                placeholder="-1"
                class="w-full bg-surface border border-emerald-500/40 rounded-lg px-3 py-1.5 text-xs text-text-main font-mono focus:outline-none focus:border-emerald-500"
              />
            </div>

            <div class="space-y-1">
              <label for="quota-input" class="text-[11px] text-text-muted">Lifetime Token Quota (0 = unlimited)</label>
              <input
                id="quota-input"
                type="number"
                min="0"
                bind:value={tokenQuota}
                class="w-full bg-surface border border-border rounded-lg px-3 py-1.5 text-xs text-text-main font-mono focus:outline-none focus:border-brand-500"
              />
            </div>

            <div class="space-y-1">
              <label for="timeout-input" class="text-[11px] text-text-muted">Queue Timeout (ms)</label>
              <input
                id="timeout-input"
                type="number"
                min="0"
                bind:value={queueTimeoutMs}
                class="w-full bg-surface border border-border rounded-lg px-3 py-1.5 text-xs text-text-main font-mono focus:outline-none focus:border-brand-500"
              />
            </div>
          </div>

          <!-- Expiry Date -->
          <div class="space-y-1">
            <label for="expiry-input" class="text-[11px] text-text-muted">Contract Expiry (leave empty for never)</label>
            <input
              id="expiry-input"
              type="datetime-local"
              bind:value={expiresLocal}
              class="w-full bg-surface border border-border rounded-lg px-3 py-1.5 text-xs text-text-main focus:outline-none focus:border-brand-500"
            />
          </div>

          <!-- Resale Metadata -->
          <div class="space-y-1">
            <label for="metadata-input" class="text-[11px] text-text-muted">Resale Metadata (JSON)</label>
            <textarea
              id="metadata-input"
              bind:value={metadata}
              rows="3"
              placeholder={'{\n  "customerId": "acme",\n  "priceCents": 5000\n}'}
              class="w-full bg-surface border border-border rounded-lg px-3 py-2 text-xs font-mono text-text-main focus:outline-none focus:border-brand-500"
            ></textarea>
          </div>
        </div>
      {/if}

      <!-- Bottom Actions -->
      <div class="pt-4 border-t border-border flex items-center justify-between">
        <div class="text-[11px] text-text-subtle">
          Changes will apply immediately across all worker nodes.
        </div>

        <div class="flex items-center gap-2">
          <button
            type="button"
            onclick={onClose}
            class="px-4 py-2 rounded-lg bg-surface hover:bg-surface-3 text-text-main font-semibold text-xs transition cursor-pointer border border-border"
          >
            Cancel
          </button>

          <button
            type="submit"
            disabled={isSaving}
            class="flex items-center gap-1.5 px-5 py-2 rounded-lg bg-brand-500 hover:bg-brand-600 text-white font-semibold text-xs transition cursor-pointer shadow-md shadow-brand-500/20 disabled:opacity-50"
          >
            {#if isSaving}
              <Loader2 class="w-3.5 h-3.5 animate-spin" />
              <span>Saving...</span>
            {:else}
              <Save class="w-3.5 h-3.5" />
              <span>Save Policy</span>
            {/if}
          </button>
        </div>
      </div>
    </form>
  </div>
</div>

{#if showPickerModal}
  <ModelPickerModal
    isOpen={showPickerModal}
    target="router"
    {connections}
    {combos}
    {providerNodes}
    {remaskRules}
    addedModelValues={models}
    onSelect={(m) => handleSelectFromPicker(m)}
    onDeselect={(m) => (models = models.filter((x) => x !== m))}
    onClose={() => (showPickerModal = false)}
  />
{/if}
