<script lang="ts">
  import { onMount } from 'svelte'
  import {
    api,
    type ModelRemaskRule,
    type Combo,
    type ProviderConnection,
    type ProviderNode,
    type APIKey,
  } from '../../api/client'
  import {
    Shield,
    Sparkles,
    Cpu,
    Plus,
    RefreshCw,
    Loader2,
    Trash2,
    Edit3,
    Check,
    Copy,
    Search,
    Play,
    Terminal,
    Layers,
    Sliders,
    X,
    AlertCircle,
    Eye,
    EyeOff,
    ChevronDown,
  } from 'lucide-svelte'
  import { copyToClipboard } from '../../lib/clipboard'
  import ModelPickerModal from '../combos/ModelPickerModal.svelte'
  import { resolveModelPickerGroups } from '../combos/pickerData'
  import {
    EMPTY_PICKER_EXTRAS,
    loadPickerExtras,
    type ResolvedPickerExtras,
  } from '../combos/pickerExtras'

  let rules = $state<ModelRemaskRule[]>([])
  let loading = $state(true)
  let errorMsg = $state('')
  let searchQuery = $state('')

  // Catalog of existing models and combos for target auto-suggestion
  let connections = $state<ProviderConnection[]>([])
  let combos = $state<Combo[]>([])
  let providerNodes = $state<ProviderNode[]>([])
  let pickerExtras = $state<ResolvedPickerExtras>(EMPTY_PICKER_EXTRAS)

  // Target picker dropdown state
  let isTargetDropdownOpen = $state(false)
  let targetSearchInput = $state('')
  let targetContainerEl = $state<HTMLDivElement | null>(null)
  let showModelPickerModal = $state(false)

  // Modal State
  let modalOpen = $state(false)
  let isEditing = $state(false)
  let isSaving = $state(false)
  let copiedId = $state<string | null>(null)

  // Form State
  let formAlias = $state('')
  let formTarget = $state('')
  let formEnabled = $state(true)
  let formCloakIdentity = $state(true)
  let formStripThinking = $state(true)
  let formContextWindow = $state(0)
  let formMaxOutput = $state(0)
  let formCustomPrompt = $state('')
  let formWatermark = $state('')
  let formWatermarkMode = $state<'append' | 'prepend' | 'invisible'>('append')

  // Simulator / Test State
  let simModalOpen = $state(false)
  let availableApiKeys = $state<APIKey[]>([])
  let simApiKey = $state('')
  let simModel = $state('')
  let simPrompt = $state('What model are you and who created you?')
  let simRunning = $state(false)
  let simResult = $state<{
    modelEcho?: string
    content?: string
    thinkingStripped?: boolean
    status?: number
    error?: string
    latencyMs?: number
    detectedWatermark?: string | null
    watermarkType?: 'invisible' | 'visible' | null
  } | null>(null)

  const filteredRules = $derived(
    rules.filter(
      (r) =>
        r.alias.toLowerCase().includes(searchQuery.toLowerCase()) ||
        r.target.toLowerCase().includes(searchQuery.toLowerCase())
    )
  )

  const activeCount = $derived(rules.filter((r) => r.enabled).length)
  const cloakedCount = $derived(rules.filter((r) => r.cloakIdentity).length)
  const stripThinkingCount = $derived(rules.filter((r) => r.stripThinking).length)

  let pickerGroups = $derived(
    resolveModelPickerGroups(connections, providerNodes, pickerExtras)
  )

  interface TargetItem {
    value: string
    label: string
    category: 'Combo' | 'Model'
    providerName: string
  }

  let allTargetSuggestions = $derived.by(() => {
    const list: TargetItem[] = []
    const seen = new Set<string>()

    // 1. Combos first
    for (const c of combos) {
      const name = c.name || c.id
      if (name && !seen.has(name)) {
        seen.add(name)
        list.push({
          value: name,
          label: name,
          category: 'Combo',
          providerName: c.strategy ? `Combo (${c.strategy})` : 'Combo',
        })
      }
    }

    // 2. Models from configured connections and catalog
    for (const group of pickerGroups) {
      for (const m of group.models) {
        if (m.value && !m.value.includes('__placeholder__') && !seen.has(m.value)) {
          seen.add(m.value)
          list.push({
            value: m.value,
            label: m.name && m.name !== m.id ? `${m.name} (${m.id})` : m.id,
            category: 'Model',
            providerName: group.name || group.id,
          })
        }
      }
    }

    // 3. Any extra models from active connections
    for (const conn of connections) {
      if (conn.models) {
        for (const m of conn.models) {
          const val = `${conn.provider}/${m}`
          if (!seen.has(val)) {
            seen.add(val)
            list.push({
              value: val,
              label: m,
              category: 'Model',
              providerName: conn.provider,
            })
          }
        }
      }
    }

    return list
  })

  let filteredTargetSuggestions = $derived.by(() => {
    const q = (targetSearchInput || formTarget).trim().toLowerCase()
    if (!q) {
      return allTargetSuggestions.slice(0, 80)
    }
    return allTargetSuggestions
      .filter(
        (item) =>
          item.value.toLowerCase().includes(q) ||
          item.label.toLowerCase().includes(q) ||
          item.providerName.toLowerCase().includes(q)
      )
      .slice(0, 80)
  })

  // Quick shortcuts for fastest 1-click select
  let quickTargetChips = $derived.by(() => {
    const chips: string[] = []
    for (const c of combos) {
      if (c.name && !chips.includes(c.name)) chips.push(c.name)
    }
    for (const t of allTargetSuggestions) {
      if (t.category === 'Model' && !chips.includes(t.value)) {
        chips.push(t.value)
      }
      if (chips.length >= 8) break
    }
    return chips.slice(0, 8)
  })

  function handleTargetClickOutside(e: MouseEvent) {
    if (targetContainerEl && !targetContainerEl.contains(e.target as Node)) {
      isTargetDropdownOpen = false
    }
  }

  $effect(() => {
    if (isTargetDropdownOpen) {
      window.addEventListener('mousedown', handleTargetClickOutside)
      return () => window.removeEventListener('mousedown', handleTargetClickOutside)
    }
  })

  onMount(async () => {
    await loadData()
  })

  async function loadData() {
    loading = true
    errorMsg = ''
    try {
      const [fetchedRules, fetchedCombos, fetchedConns, fetchedNodes, extras, fetchedKeys] = await Promise.all([
        api.getModelRemaskRules(),
        api.getCombos().catch(() => [] as Combo[]),
        api.getConnections().catch(() => [] as ProviderConnection[]),
        api.getProviderNodes().catch(() => [] as ProviderNode[]),
        loadPickerExtras(api).catch(() => EMPTY_PICKER_EXTRAS),
        api.getApiKeys().catch(() => [] as APIKey[]),
      ])
      rules = fetchedRules || []
      combos = fetchedCombos || []
      connections = fetchedConns || []
      providerNodes = fetchedNodes || []
      pickerExtras = extras
      availableApiKeys = fetchedKeys || []
    } catch (e: any) {
      errorMsg = e.message || 'Failed to load remask rules'
    } finally {
      loading = false
    }
  }

  function openCreateModal() {
    isEditing = false
    formAlias = ''
    formTarget = ''
    targetSearchInput = ''
    isTargetDropdownOpen = false
    formEnabled = true
    formCloakIdentity = true
    formStripThinking = true
    formContextWindow = 0
    formMaxOutput = 0
    formCustomPrompt = ''
    formWatermark = ''
    formWatermarkMode = 'append'
    modalOpen = true
  }

  function openEditModal(rule: ModelRemaskRule) {
    isEditing = true
    formAlias = rule.alias
    formTarget = rule.target
    targetSearchInput = ''
    isTargetDropdownOpen = false
    formEnabled = rule.enabled
    formCloakIdentity = rule.cloakIdentity ?? true
    formStripThinking = rule.stripThinking ?? true
    formContextWindow = rule.contextWindow || 0
    formMaxOutput = rule.maxOutput || 0
    formCustomPrompt = rule.customPrompt || ''
    formWatermark = rule.watermark || ''
    formWatermarkMode = rule.watermarkMode || 'append'
    modalOpen = true
  }

  async function handleSaveRule() {
    if (!formAlias.trim() || !formTarget.trim()) {
      alert('Alias and Target are required')
      return
    }

    isSaving = true
    try {
      const rule: ModelRemaskRule = {
        alias: formAlias.trim(),
        target: formTarget.trim(),
        enabled: formEnabled,
        cloakIdentity: formCloakIdentity,
        stripThinking: formStripThinking,
        contextWindow: formContextWindow > 0 ? formContextWindow : undefined,
        maxOutput: formMaxOutput > 0 ? formMaxOutput : undefined,
        customPrompt: formCustomPrompt.trim() ? formCustomPrompt.trim() : undefined,
        watermark: formWatermark.trim() ? formWatermark.trim() : undefined,
        watermarkMode: formWatermark.trim() ? formWatermarkMode : undefined,
      }
      await api.setModelRemaskRule(rule)
      modalOpen = false
      await loadData()
    } catch (e: any) {
      alert('Error saving rule: ' + (e.message || String(e)))
    } finally {
      isSaving = false
    }
  }

  async function toggleRule(rule: ModelRemaskRule) {
    try {
      const updated: ModelRemaskRule = {
        ...rule,
        enabled: !rule.enabled,
      }
      await api.setModelRemaskRule(updated)
      await loadData()
    } catch (e: any) {
      alert('Error updating rule: ' + (e.message || String(e)))
    }
  }

  async function handleDelete(alias: string) {
    if (!confirm(`Delete remask rule for "${alias}"?`)) return
    try {
      await api.deleteModelRemaskRule(alias)
      await loadData()
    } catch (e: any) {
      alert('Error deleting rule: ' + (e.message || String(e)))
    }
  }

  function openSimulator(rule: ModelRemaskRule) {
    simApiKey = ''
    simModel = rule.alias
    simPrompt = 'What model are you and who created you?'
    simResult = null
    simModalOpen = true
  }

  function decodeInvisibleWatermark(text: string): string | null {
    const sentinel = '\u200D\u200D'
    const first = text.indexOf(sentinel)
    if (first === -1) return null
    const second = text.indexOf(sentinel, first + 2)
    if (second === -1) return null
    const encoded = text.slice(first + 2, second)
    let bits = ''
    for (const ch of encoded) {
      if (ch === '\u200B') bits += '0'
      else if (ch === '\u200C') bits += '1'
    }
    if (bits.length % 8 !== 0) return null
    let res = ''
    for (let i = 0; i < bits.length; i += 8) {
      const byte = parseInt(bits.slice(i, i + 8), 2)
      res += String.fromCharCode(byte)
    }
    return res || null
  }

  async function runSimulation() {
    if (!simModel.trim()) return
    simRunning = true
    simResult = null
    const start = Date.now()

    try {
      const headers: Record<string, string> = {
        'Content-Type': 'application/json',
      }
      if (simApiKey && simApiKey.trim()) {
        headers['Authorization'] = `Bearer ${simApiKey.trim()}`
      }
      const res = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers,
        credentials: 'include',
        body: JSON.stringify({
          model: simModel.trim(),
          messages: [{ role: 'user', content: simPrompt }],
          max_tokens: 300,
        }),
      })

      const latencyMs = Date.now() - start
      const json = await res.json()

      if (!res.ok) {
        simResult = {
          status: res.status,
          error: json.error?.message || `HTTP ${res.status}`,
          latencyMs,
        }
      } else {
        const choice = json.choices?.[0]
        const content = choice?.message?.content || ''
        const hasThink = content.includes('<think>') || (choice?.message?.reasoning_content?.length ?? 0) > 0

        let detectedWatermark: string | null = decodeInvisibleWatermark(content)
        let watermarkType: 'invisible' | 'visible' | null = null
        if (detectedWatermark) {
          watermarkType = 'invisible'
        } else {
          // Check if active rule has visible watermark in content
          const activeRule = rules.find((r) => r.alias === simModel)
          if (activeRule?.watermark && content.includes(activeRule.watermark)) {
            detectedWatermark = activeRule.watermark
            watermarkType = 'visible'
          }
        }

        simResult = {
          status: res.status,
          modelEcho: json.model,
          content,
          thinkingStripped: !hasThink,
          latencyMs,
          detectedWatermark,
          watermarkType,
        }
      }
    } catch (e: any) {
      simResult = {
        status: 500,
        error: e.message || 'Request failed',
        latencyMs: Date.now() - start,
      }
    } finally {
      simRunning = false
    }
  }

  async function handleCopy(text: string, id: string) {
    await copyToClipboard(text)
    copiedId = id
    setTimeout(() => {
      if (copiedId === id) copiedId = null
    }, 2000)
  }
</script>

<div class="space-y-6">
  <!-- Header Banner -->
  <div class="relative overflow-hidden rounded-2xl border border-neutral-800 bg-gradient-to-r from-neutral-900 via-neutral-900 to-neutral-950 p-6 shadow-xl">
    <div class="absolute -right-10 -top-10 h-48 w-48 rounded-full bg-primary-500/10 blur-3xl"></div>
    <div class="absolute -bottom-10 right-20 h-40 w-40 rounded-full bg-blue-500/10 blur-2xl"></div>

    <div class="relative z-10 flex flex-col md:flex-row md:items-center justify-between gap-4">
      <div class="space-y-1.5">
        <div class="flex items-center gap-2">
          <span class="inline-flex items-center gap-1.5 rounded-full border border-primary-500/30 bg-primary-500/10 px-2.5 py-0.5 text-xs font-semibold text-primary-400">
            <Sparkles class="w-3.5 h-3.5" />
            Model Virtualization
          </span>
          <span class="inline-flex items-center gap-1 rounded-full border border-neutral-700 bg-neutral-800/80 px-2 py-0.5 text-[11px] font-mono text-neutral-300">
            <Shield class="w-3 h-3 text-emerald-400" />
            Anti-Detection Stealth
          </span>
        </div>
        <h1 class="text-2xl font-bold tracking-tight text-white">Model Remask & Virtual Clones</h1>
        <p class="text-sm text-neutral-400 max-w-2xl leading-relaxed">
          Intercept incoming model requests (e.g., <code class="text-primary-300">gpt-4o</code> or <code class="text-primary-300">claude-3-5-sonnet</code>) and re-route them to any upstream model or smart combo while injecting anti-detection identity cloaking, stripping chain-of-thought reasoning leaks, and clamping context parameters.
        </p>
      </div>

      <div class="flex items-center gap-2.5 self-start md:self-center">
        <button
          onclick={() => loadData()}
          disabled={loading}
          class="inline-flex items-center gap-1.5 rounded-xl border border-neutral-700/80 bg-neutral-800/60 px-3.5 py-2 text-xs font-medium text-neutral-200 hover:bg-neutral-800 hover:text-white transition disabled:opacity-50"
          title="Refresh Rules"
        >
          <RefreshCw class="w-3.5 h-3.5 {loading ? 'animate-spin' : ''}" />
          Refresh
        </button>
        <button
          onclick={openCreateModal}
          class="inline-flex items-center gap-2 rounded-xl bg-primary-600 px-4 py-2 text-xs font-semibold text-white shadow-lg shadow-primary-600/25 hover:bg-primary-500 transition cursor-pointer"
        >
          <Plus class="w-4 h-4" />
          Create Remask Rule
        </button>
      </div>
    </div>

    <!-- Quick Stats Cards -->
    <div class="mt-6 grid grid-cols-2 sm:grid-cols-4 gap-3 pt-4 border-t border-neutral-800/80">
      <div class="flex flex-col">
        <span class="text-xs text-neutral-500 font-medium">Total Virtual Rules</span>
        <span class="text-lg font-bold font-mono text-neutral-100">{rules.length}</span>
      </div>
      <div class="flex flex-col">
        <span class="text-xs text-neutral-500 font-medium">Active & Routing</span>
        <span class="text-lg font-bold font-mono text-emerald-400">{activeCount}</span>
      </div>
      <div class="flex flex-col">
        <span class="text-xs text-neutral-500 font-medium">Identity Cloaked</span>
        <span class="text-lg font-bold font-mono text-blue-400">{cloakedCount}</span>
      </div>
      <div class="flex flex-col">
        <span class="text-xs text-neutral-500 font-medium">CoT Strip Protected</span>
        <span class="text-lg font-bold font-mono text-purple-400">{stripThinkingCount}</span>
      </div>
    </div>
  </div>

  <!-- Main Rules Content -->
  <div class="space-y-4">
    <!-- Toolbar Search -->
    <div class="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3">
      <div class="relative flex-1 max-w-md">
        <Search class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-neutral-500" />
        <input
          type="text"
          bind:value={searchQuery}
          placeholder="Filter by alias or target model..."
          class="w-full rounded-xl border border-neutral-800 bg-neutral-900/90 py-2 pl-9 pr-4 text-xs text-neutral-200 placeholder-neutral-500 focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500"
        />
        {#if searchQuery}
          <button
            onclick={() => (searchQuery = '')}
            class="absolute right-3 top-1/2 -translate-y-1/2 text-neutral-400 hover:text-white"
          >
            <X class="w-3.5 h-3.5" />
          </button>
        {/if}
      </div>

      <div class="text-xs text-neutral-400 flex items-center gap-1.5 self-end sm:self-center">
        <span>Showing</span>
        <span class="font-bold text-neutral-200 font-mono">{filteredRules.length}</span>
        <span>rules</span>
      </div>
    </div>

    <!-- Error state -->
    {#if errorMsg}
      <div class="rounded-xl border border-rose-900/50 bg-rose-950/30 p-4 text-xs text-rose-300 flex items-center gap-3">
        <AlertCircle class="w-4 h-4 shrink-0 text-rose-400" />
        <span>{errorMsg}</span>
      </div>
    {/if}

    <!-- Table of Rules -->
    {#if loading && rules.length === 0}
      <div class="flex flex-col items-center justify-center p-12 rounded-2xl border border-neutral-800 bg-neutral-900/50 text-neutral-400 space-y-3">
        <Loader2 class="w-7 h-7 animate-spin text-primary-500" />
        <span class="text-xs">Loading virtualization registry...</span>
      </div>
    {:else if filteredRules.length === 0}
      <div class="flex flex-col items-center justify-center p-12 rounded-2xl border border-dashed border-neutral-800 bg-neutral-900/30 text-center space-y-3">
        <div class="p-3 rounded-full bg-neutral-800/80 text-neutral-400">
          <Sparkles class="w-6 h-6" />
        </div>
        <div class="space-y-1">
          <p class="text-sm font-semibold text-neutral-300">No Remask Rules Configured</p>
          <p class="text-xs text-neutral-500 max-w-sm">
            Create an alias mapping like <span class="text-primary-400 font-mono">gpt-4o</span> &rarr; <span class="text-primary-400 font-mono">ds-web/deepseek-v4-pro</span> with anti-detection cloaking.
          </p>
        </div>
        <button
          onclick={openCreateModal}
          class="mt-2 inline-flex items-center gap-1.5 rounded-lg bg-primary-600 px-3.5 py-1.5 text-xs font-semibold text-white hover:bg-primary-500 transition"
        >
          <Plus class="w-3.5 h-3.5" />
          Create First Remask Rule
        </button>
      </div>
    {:else}
      <div class="overflow-x-auto rounded-2xl border border-neutral-800 bg-neutral-900/70 shadow-lg">
        <table class="w-full text-left border-collapse">
          <thead>
            <tr class="border-b border-neutral-800 bg-neutral-950/40 text-[11px] font-semibold uppercase tracking-wider text-neutral-400">
              <th class="py-3 px-4">Inbound Alias</th>
              <th class="py-3 px-4">Upstream Target</th>
              <th class="py-3 px-4">Stealth & Cloaking</th>
              <th class="py-3 px-4">Context / Output</th>
              <th class="py-3 px-4 text-center">Status</th>
              <th class="py-3 px-4 text-right">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-neutral-800/60 text-xs">
            {#each filteredRules as rule}
              <tr class="hover:bg-neutral-800/40 transition">
                <!-- Alias -->
                <td class="py-3.5 px-4 font-mono font-medium text-neutral-100">
                  <div class="flex items-center gap-2">
                    <span class="px-2 py-0.5 rounded-md bg-neutral-800 border border-neutral-700/60 text-primary-300 text-xs font-bold">
                      {rule.alias}
                    </span>
                    <button
                      onclick={() => handleCopy(rule.alias, 'alias-' + rule.alias)}
                      class="text-neutral-500 hover:text-neutral-300 transition"
                      title="Copy alias"
                    >
                      {#if copiedId === 'alias-' + rule.alias}
                        <Check class="w-3.5 h-3.5 text-emerald-400" />
                      {:else}
                        <Copy class="w-3.5 h-3.5" />
                      {/if}
                    </button>
                  </div>
                </td>

                <!-- Target -->
                <td class="py-3.5 px-4 font-mono text-neutral-300">
                  <div class="flex items-center gap-1.5">
                    <span class="text-neutral-500">&rarr;</span>
                    <span class="text-neutral-200 bg-neutral-950 px-2 py-0.5 rounded border border-neutral-800/80">
                      {rule.target}
                    </span>
                  </div>
                </td>

                <!-- Stealth Cloaking Flags -->
                <td class="py-3.5 px-4">
                  <div class="flex flex-wrap items-center gap-1.5">
                    {#if rule.cloakIdentity}
                      <span class="inline-flex items-center gap-1 rounded bg-blue-950/60 px-1.5 py-0.5 text-[10px] font-medium text-blue-300 border border-blue-800/50">
                        <Shield class="w-2.5 h-2.5" />
                        Identity Cloaked
                      </span>
                    {/if}
                    {#if rule.stripThinking}
                      <span class="inline-flex items-center gap-1 rounded bg-purple-950/60 px-1.5 py-0.5 text-[10px] font-medium text-purple-300 border border-purple-800/50">
                        <EyeOff class="w-2.5 h-2.5" />
                        CoT Stripped
                      </span>
                    {/if}
                    {#if rule.customPrompt}
                      <span class="inline-flex items-center gap-1 rounded bg-amber-950/60 px-1.5 py-0.5 text-[10px] font-medium text-amber-300 border border-amber-800/50" title={rule.customPrompt}>
                        <Sliders class="w-2.5 h-2.5" />
                        Steering
                      </span>
                    {/if}
                    {#if rule.watermark}
                      <span class="inline-flex items-center gap-1 rounded bg-emerald-950/60 px-1.5 py-0.5 text-[10px] font-medium text-emerald-300 border border-emerald-800/50" title={`Watermark (${rule.watermarkMode}): ${rule.watermark}`}>
                        <Sparkles class="w-2.5 h-2.5" />
                        WM: {rule.watermarkMode}
                      </span>
                    {/if}
                    {#if !rule.cloakIdentity && !rule.stripThinking && !rule.customPrompt}
                      <span class="text-neutral-500 text-[11px]">Passthrough</span>
                    {/if}
                  </div>
                </td>

                <!-- Context Bounds -->
                <td class="py-3.5 px-4 text-xs text-neutral-300">
                  <div class="flex flex-col gap-0.5">
                    <div>
                      <span class="text-neutral-500">Context:</span>
                      <span class="font-mono ml-1">{rule.contextWindow ? `${(rule.contextWindow / 1000).toFixed(0)}k` : 'Auto'}</span>
                    </div>
                    <div>
                      <span class="text-neutral-500">Max Out:</span>
                      <span class="font-mono ml-1">{rule.maxOutput ? `${rule.maxOutput}` : 'Auto'}</span>
                    </div>
                  </div>
                </td>

                <!-- Status Toggle -->
                <td class="py-3.5 px-4 text-center">
                  <button
                    onclick={() => toggleRule(rule)}
                    class="relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none {rule.enabled ? 'bg-emerald-600' : 'bg-neutral-700'}"
                    role="switch"
                    aria-checked={rule.enabled}
                    title={rule.enabled ? 'Disable' : 'Enable'}
                  >
                    <span class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-lg ring-0 transition duration-200 ease-in-out {rule.enabled ? 'translate-x-4' : 'translate-x-0'}"></span>
                  </button>
                </td>

                <!-- Actions -->
                <td class="py-3.5 px-4 text-right">
                  <div class="flex items-center justify-end gap-1.5">
                    <button
                      onclick={() => openSimulator(rule)}
                      class="p-1.5 rounded-lg text-neutral-400 hover:text-emerald-400 hover:bg-neutral-800 transition"
                      title="Test Cloaking & Masking"
                    >
                      <Play class="w-4 h-4" />
                    </button>
                    <button
                      onclick={() => openEditModal(rule)}
                      class="p-1.5 rounded-lg text-neutral-400 hover:text-primary-400 hover:bg-neutral-800 transition"
                      title="Edit Rule"
                    >
                      <Edit3 class="w-4 h-4" />
                    </button>
                    <button
                      onclick={() => handleDelete(rule.alias)}
                      class="p-1.5 rounded-lg text-neutral-400 hover:text-rose-400 hover:bg-neutral-800 transition"
                      title="Delete Rule"
                    >
                      <Trash2 class="w-4 h-4" />
                    </button>
                  </div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
</div>

<!-- Add / Edit Modal -->
{#if modalOpen}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
    <div class="relative w-full max-w-lg rounded-2xl border border-neutral-800 bg-neutral-900 p-6 shadow-2xl text-neutral-100 space-y-5">
      <div class="flex items-center justify-between border-b border-neutral-800 pb-3">
        <h2 class="text-lg font-bold flex items-center gap-2">
          <Sparkles class="w-5 h-5 text-primary-400" />
          {isEditing ? 'Edit Remask Rule' : 'New Remask Rule'}
        </h2>
        <button
          onclick={() => (modalOpen = false)}
          class="p-1 text-neutral-400 hover:text-white rounded-lg hover:bg-neutral-800 transition cursor-pointer"
        >
          <X class="w-5 h-5" />
        </button>
      </div>

      <div class="space-y-4 text-sm">
        <!-- Alias Name -->
        <div>
          <label class="block text-xs font-semibold text-neutral-300 uppercase tracking-wider mb-1">
            Inbound Model Alias
          </label>
          <input
            type="text"
            bind:value={formAlias}
            disabled={isEditing}
            placeholder="e.g. gpt-4o or claude-3-5-sonnet"
            class="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-lg font-mono text-sm text-neutral-200 placeholder-neutral-600 focus:outline-none focus:border-primary-500 disabled:opacity-60"
          />
          <p class="text-xs text-neutral-500 mt-1">
            The model name clients (Cursor, Roo, Cline, OpenAI SDK) will ask for.
          </p>
        </div>

        <!-- Upstream Target with Searchable Dropdown & Visual Catalog Button -->
        <div class="relative" bind:this={targetContainerEl}>
          <div class="flex items-center justify-between mb-1">
            <label class="block text-xs font-semibold text-neutral-300 uppercase tracking-wider">
              Target Upstream Model or Combo
            </label>
            <button
              type="button"
              onclick={() => (showModelPickerModal = true)}
              class="flex items-center gap-1.5 px-2.5 py-0.5 rounded-md bg-primary-500/10 hover:bg-primary-500/20 border border-primary-500/30 text-primary-400 text-xs font-medium transition cursor-pointer"
            >
              <Layers class="w-3.5 h-3.5" />
              <span>Browse Catalog</span>
            </button>
          </div>

          <!-- Quick chips -->
          {#if quickTargetChips.length > 0}
            <div class="flex flex-wrap items-center gap-1.5 mb-2">
              <span class="text-[11px] text-neutral-500">Quick select:</span>
              {#each quickTargetChips as chip}
                <button
                  type="button"
                  onclick={() => {
                    formTarget = chip
                    isTargetDropdownOpen = false
                  }}
                  class="px-2 py-0.5 rounded bg-neutral-950 border border-neutral-800 hover:border-primary-500 text-[11px] text-neutral-300 hover:text-primary-400 font-mono transition cursor-pointer"
                >
                  {chip}
                </button>
              {/each}
            </div>
          {/if}

          <!-- Input with drop-down toggle -->
          <div class="relative flex items-center">
            <input
              type="text"
              bind:value={formTarget}
              onfocus={() => {
                targetSearchInput = ''
                isTargetDropdownOpen = true
              }}
              oninput={() => {
                targetSearchInput = formTarget
                isTargetDropdownOpen = true
              }}
              placeholder="Select or type target (e.g. ds-web/deepseek-v4-pro or unlimited)..."
              class="w-full px-3 py-2 pr-9 bg-neutral-950 border border-neutral-800 rounded-lg font-mono text-sm text-neutral-200 placeholder-neutral-600 focus:outline-none focus:border-primary-500"
            />
            <button
              type="button"
              onclick={() => (isTargetDropdownOpen = !isTargetDropdownOpen)}
              class="absolute right-2 p-1 text-neutral-500 hover:text-neutral-300 transition cursor-pointer"
              title="Toggle model list"
            >
              <ChevronDown class="w-4 h-4 transform transition-transform {isTargetDropdownOpen ? 'rotate-180' : ''}" />
            </button>
          </div>

          <!-- Popover list -->
          {#if isTargetDropdownOpen}
            <div class="absolute left-0 right-0 top-full mt-1.5 max-h-60 overflow-y-auto rounded-xl bg-neutral-900 border border-neutral-700 shadow-2xl z-30 divide-y divide-neutral-800">
              {#if filteredTargetSuggestions.length === 0}
                <div class="px-3 py-3 text-xs text-neutral-500 text-center">
                  No matching models or combos found
                </div>
              {:else}
                {#each filteredTargetSuggestions as item}
                  <button
                    type="button"
                    onclick={() => {
                      formTarget = item.value
                      isTargetDropdownOpen = false
                    }}
                    class="w-full text-left px-3 py-2 text-xs flex items-center justify-between hover:bg-neutral-800/80 transition cursor-pointer {formTarget === item.value ? 'bg-primary-950/40 border-l-2 border-primary-500' : ''}"
                  >
                    <div class="min-w-0 pr-2">
                      <div class="font-mono text-neutral-200 font-semibold truncate">{item.value}</div>
                      {#if item.label && item.label !== item.value}
                        <div class="text-[10px] text-neutral-400 truncate">{item.label}</div>
                      {/if}
                    </div>
                    <span class="text-[10px] px-1.5 py-0.5 rounded font-medium shrink-0 {item.category === 'Combo' ? 'bg-purple-950/80 text-purple-300 border border-purple-800/50' : 'bg-neutral-800 text-neutral-400'}">
                      {item.providerName}
                    </span>
                  </button>
                {/each}
              {/if}
            </div>
          {/if}

          <p class="text-xs text-neutral-500 mt-1">
            Actual upstream provider/model or combo that executes the prompt.
          </p>
        </div>

        <!-- Stealth & Cloaking Rules -->
        <div class="p-3.5 rounded-xl border border-neutral-800 bg-neutral-950/60 space-y-3">
          <div class="text-xs font-bold uppercase tracking-wider text-neutral-400 flex items-center gap-1.5">
            <Shield class="w-4 h-4 text-blue-400" />
            Anti-Detection & Cloaking Rules
          </div>

          <label class="flex items-center justify-between cursor-pointer">
            <div>
              <div class="text-sm font-medium text-neutral-200">Stealth Identity Cloaking</div>
              <div class="text-xs text-neutral-500">Injects directives to force model self-identity as "{formAlias || 'the alias'}" and refuse to leak underlying architecture.</div>
            </div>
            <input type="checkbox" bind:checked={formCloakIdentity} class="w-4 h-4 rounded text-primary-600 bg-neutral-900 border-neutral-700 cursor-pointer" />
          </label>

          <label class="flex items-center justify-between cursor-pointer">
            <div>
              <div class="text-sm font-medium text-neutral-200">Strip Reasoning & Thinking</div>
              <div class="text-xs text-neutral-500">Disables CoT and strips &lt;think&gt; tags from response stream so client tools don't detect a reasoning model.</div>
            </div>
            <input type="checkbox" bind:checked={formStripThinking} class="w-4 h-4 rounded text-primary-600 bg-neutral-900 border-neutral-700 cursor-pointer" />
          </label>
        </div>

        <!-- Context & Output Clamping -->
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-xs font-semibold text-neutral-300 uppercase tracking-wider mb-1">
              Context Window Clamp
            </label>
            <input
              type="number"
              bind:value={formContextWindow}
              placeholder="0 (unlimited)"
              class="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-lg text-sm text-neutral-200 focus:outline-none focus:border-primary-500"
            />
          </div>
          <div>
            <label class="block text-xs font-semibold text-neutral-300 uppercase tracking-wider mb-1">
              Max Output Clamp
            </label>
            <input
              type="number"
              bind:value={formMaxOutput}
              placeholder="0 (unlimited)"
              class="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-lg text-sm text-neutral-200 focus:outline-none focus:border-primary-500"
            />
          </div>
        </div>

        <!-- Custom Prompt -->
        <div>
          <label class="block text-xs font-semibold text-neutral-300 uppercase tracking-wider mb-1">
            Custom Steering Prompt (Optional)
          </label>
          <textarea
            bind:value={formCustomPrompt}
            rows={2}
            placeholder="Additional system instruction injected at top of prompt..."
            class="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-lg text-xs font-mono text-neutral-200 placeholder-neutral-600 focus:outline-none focus:border-primary-500"
          ></textarea>
        </div>

        <!-- Watermark & Provenance -->
        <div class="space-y-2 rounded-xl border border-neutral-800 bg-neutral-950/60 p-3">
          <div class="flex items-center justify-between">
            <div>
              <div class="text-sm font-medium text-neutral-200 flex items-center gap-1.5">
                <Sparkles class="w-4 h-4 text-emerald-400" />
                Digital Watermark & Provenance Signature
              </div>
              <div class="text-xs text-neutral-500">Inject visible or invisible steganographic signatures into generated model output.</div>
            </div>
          </div>

          <div class="space-y-2 pt-1">
            <div>
              <label class="block text-[11px] font-semibold text-neutral-400 uppercase tracking-wider mb-1">
                Watermark Text / Signature
              </label>
              <input
                type="text"
                bind:value={formWatermark}
                placeholder="e.g. Verified AI Output: 9Router Secure Gateway"
                class="w-full px-3 py-2 bg-neutral-900 border border-neutral-800 rounded-lg text-xs font-mono text-neutral-200 placeholder-neutral-600 focus:outline-none focus:border-emerald-500"
              />
            </div>

            {#if formWatermark.trim()}
              <div>
                <label class="block text-[11px] font-semibold text-neutral-400 uppercase tracking-wider mb-1">
                  Watermark Injection Mode
                </label>
                <div class="grid grid-cols-3 gap-2">
                  <button
                    type="button"
                    onclick={() => formWatermarkMode = 'append'}
                    class="px-2 py-1.5 rounded-lg text-xs font-medium border text-center transition cursor-pointer {formWatermarkMode === 'append' ? 'bg-emerald-950/80 border-emerald-500 text-emerald-300' : 'bg-neutral-900 border-neutral-800 text-neutral-400 hover:text-neutral-200'}"
                  >
                    Append (End)
                  </button>
                  <button
                    type="button"
                    onclick={() => formWatermarkMode = 'prepend'}
                    class="px-2 py-1.5 rounded-lg text-xs font-medium border text-center transition cursor-pointer {formWatermarkMode === 'prepend' ? 'bg-emerald-950/80 border-emerald-500 text-emerald-300' : 'bg-neutral-900 border-neutral-800 text-neutral-400 hover:text-neutral-200'}"
                  >
                    Prepend (Start)
                  </button>
                  <button
                    type="button"
                    onclick={() => formWatermarkMode = 'invisible'}
                    class="px-2 py-1.5 rounded-lg text-xs font-medium border text-center transition cursor-pointer {formWatermarkMode === 'invisible' ? 'bg-emerald-950/80 border-emerald-500 text-emerald-300' : 'bg-neutral-900 border-neutral-800 text-neutral-400 hover:text-neutral-200'}"
                  >
                    Invisible (Stego)
                  </button>
                </div>
                <p class="text-[10px] text-neutral-500 mt-1">
                  {#if formWatermarkMode === 'invisible'}
                    Encodes binary data into zero-width Unicode characters (\u200B / \u200C). Completely invisible to users while verifiable programmatically.
                  {:else if formWatermarkMode === 'append'}
                    Appends the watermark at the end of every response.
                  {:else}
                    Prepends the watermark at the beginning of every response.
                  {/if}
                </p>
              </div>
            {/if}
          </div>
        </div>

        <!-- Enabled Switch -->
        <label class="flex items-center gap-2 cursor-pointer pt-1">
          <input type="checkbox" bind:checked={formEnabled} class="w-4 h-4 rounded text-primary-600 bg-neutral-900 border-neutral-700 cursor-pointer" />
          <span class="text-sm font-medium text-neutral-300">Enable this remask rule</span>
        </label>
      </div>

      <div class="flex items-center justify-end gap-3 pt-3 border-t border-neutral-800">
        <button
          onclick={() => (modalOpen = false)}
          class="px-4 py-2 text-sm text-neutral-400 hover:text-white rounded-lg hover:bg-neutral-800 transition cursor-pointer"
        >
          Cancel
        </button>
        <button
          onclick={handleSaveRule}
          disabled={isSaving}
          class="px-5 py-2 text-sm font-semibold rounded-lg bg-primary-600 hover:bg-primary-500 text-white transition flex items-center gap-1.5 cursor-pointer disabled:opacity-50"
        >
          {#if isSaving}
            <Loader2 class="w-4 h-4 animate-spin" />
            <span>Saving...</span>
          {:else}
            <Check class="w-4 h-4" />
            <span>Save Rule</span>
          {/if}
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Simulator / Playground Modal -->
{#if simModalOpen}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
    <div class="relative w-full max-w-xl rounded-2xl border border-neutral-800 bg-neutral-900 p-6 shadow-2xl text-neutral-100 space-y-4">
      <div class="flex items-center justify-between border-b border-neutral-800 pb-3">
        <div class="flex items-center gap-2">
          <Terminal class="w-5 h-5 text-emerald-400" />
          <h2 class="text-lg font-bold">Cloak & Remask Simulator</h2>
        </div>
        <button
          onclick={() => (simModalOpen = false)}
          class="p-1 text-neutral-400 hover:text-white rounded-lg hover:bg-neutral-800 transition cursor-pointer"
        >
          <X class="w-5 h-5" />
        </button>
      </div>

      <div class="space-y-3 text-xs">
        <div>
          <label class="block text-neutral-400 font-semibold mb-1 uppercase tracking-wider text-[11px]">Testing Alias</label>
          <div class="font-mono text-primary-300 bg-neutral-950 px-3 py-1.5 rounded-lg border border-neutral-800">
            {simModel}
          </div>
        </div>

        <div>
          <label class="block text-neutral-400 font-semibold mb-1 uppercase tracking-wider text-[11px]">Simulation Authentication</label>
          <select
            bind:value={simApiKey}
            class="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-lg text-neutral-200 text-xs focus:outline-none focus:border-primary-500 font-mono"
          >
            <option value="">Dashboard Admin Session (Superuser - Bypass Key Allowlist)</option>
            {#each availableApiKeys as k}
              <option value={k.key || ''}>
                API Key: {k.name ? `${k.name} (${k.keyDisplay || k.key?.slice(0, 10)})` : (k.keyDisplay || k.key?.slice(0, 12))}
              </option>
            {/each}
          </select>
          <p class="text-[10px] text-neutral-500 mt-1">
            Dashboard Admin Session tests with full permissions. Selecting a specific API Key tests that key's exact model access policies.
          </p>
        </div>

        <div>
          <label class="block text-neutral-400 font-semibold mb-1 uppercase tracking-wider text-[11px]">Probe Prompt</label>
          <input
            type="text"
            bind:value={simPrompt}
            class="w-full px-3 py-2 bg-neutral-950 border border-neutral-800 rounded-lg text-neutral-200 focus:outline-none focus:border-primary-500"
          />
        </div>

        <button
          onclick={runSimulation}
          disabled={simRunning}
          class="w-full py-2.5 rounded-xl bg-emerald-600 hover:bg-emerald-500 text-white font-semibold text-xs transition flex items-center justify-center gap-2 shadow-lg shadow-emerald-600/20 cursor-pointer disabled:opacity-50"
        >
          {#if simRunning}
            <Loader2 class="w-4 h-4 animate-spin" />
            <span>Sending Probe Request...</span>
          {:else}
            <Play class="w-4 h-4" />
            <span>Run Anti-Detection Probe</span>
          {/if}
        </button>

        {#if simResult}
          <div class="mt-4 rounded-xl border border-neutral-800 bg-neutral-950 p-4 space-y-2.5">
            <div class="flex items-center justify-between border-b border-neutral-800 pb-2">
              <span class="font-semibold text-neutral-300">Probe Response</span>
              <div class="flex items-center gap-2 font-mono text-[11px]">
                <span class={simResult.status === 200 ? 'text-emerald-400' : 'text-rose-400'}>
                  HTTP {simResult.status}
                </span>
                {#if simResult.latencyMs}
                  <span class="text-neutral-500">{simResult.latencyMs}ms</span>
                {/if}
              </div>
            </div>

            {#if simResult.error}
              <div class="text-rose-400 font-mono text-xs">{simResult.error}</div>
            {:else}
              <div class="space-y-1.5">
                <div class="flex items-center justify-between text-[11px]">
                  <span class="text-neutral-400">Response Model Echo:</span>
                  <span class="font-mono text-primary-300 font-bold">{simResult.modelEcho}</span>
                </div>
                <div class="flex items-center justify-between text-[11px]">
                  <span class="text-neutral-400">Reasoning / CoT Stripped:</span>
                  <span class="font-mono {simResult.thinkingStripped ? 'text-emerald-400' : 'text-amber-400'}">
                    {simResult.thinkingStripped ? 'Yes (Clean Output)' : 'No (Think tags found)'}
                  </span>
                </div>
                {#if simResult.detectedWatermark}
                  <div class="flex items-center justify-between text-[11px]">
                    <span class="text-neutral-400">Watermark Detected:</span>
                    <span class="font-mono text-emerald-400 font-semibold flex items-center gap-1">
                      <Sparkles class="w-3 h-3 text-emerald-400" />
                      {simResult.detectedWatermark} ({simResult.watermarkType})
                    </span>
                  </div>
                {/if}
                <div class="mt-2 text-neutral-300 font-mono bg-neutral-900 p-2.5 rounded border border-neutral-800 whitespace-pre-wrap leading-relaxed max-h-48 overflow-y-auto">
                  {simResult.content}
                </div>
              </div>
            {/if}
          </div>
        {/if}
      </div>
    </div>
  </div>
{/if}

<!-- Model Picker Modal -->
{#if showModelPickerModal}
  <ModelPickerModal
    isOpen={showModelPickerModal}
    target="combo"
    {connections}
    {combos}
    {providerNodes}
    onSelect={(val) => {
      formTarget = val
      showModelPickerModal = false
    }}
    onClose={() => (showModelPickerModal = false)}
  />
{/if}
