<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type JailbreakModelStatus, type ProviderNode } from '../api/client'
  import { PROVIDER_CATALOG } from '../lib/providers'
  import Badge from '../lib/ui/Badge.svelte'
  import Button from '../lib/ui/Button.svelte'
  import Card from '../lib/ui/Card.svelte'
  import Pagination from './Pagination.svelte'
  import ModelParametersModal from './ModelParametersModal.svelte'

  // Models & status list
  let models = $state<JailbreakModelStatus[]>([])
  let counts = $state<Record<string, number>>({})
  let providerNodes = $state<ProviderNode[]>([])
  let loading = $state(false)
  let error = $state('')
  let search = $state('')
  let filter = $state('all')
  let page = $state(1)
  let pageSize = $state(20)

  // Interactive Inspection & Detail Modals state
  let hoveredModel = $state<JailbreakModelStatus | null>(null)
  let selectedModel = $state<JailbreakModelStatus | null>(null)
  let detailModel = $state<JailbreakModelStatus | null>(null)
  let showDetailModal = $state(false)
  let showFullResultsModal = $state(false)
  let paramsModalTarget = $state('')
  let showParamsModal = $state(false)

  function openParamsModal(modelId: string) {
    paramsModalTarget = modelId
    showParamsModal = true
  }

  // Filters inside Full Results Modal
  let fullResultsSearch = $state('')
  let fullResultsStatus = $state<'all' | 'fail' | 'slow' | 'ok'>('all')
  let fullResultsProvider = $state<string>('all')

  let activeModel = $derived<JailbreakModelStatus | null>(
    hoveredModel || selectedModel || (filtered.length > 0 ? filtered[0] : null)
  )

  // Upstream reachability probe results
  type Probe = {
    state: 'idle' | 'running' | 'ok' | 'fail'
    latencyMs?: number
    error?: string
    status?: number
    note?: string
    testedAt?: number
  }
  let probes = $state<Record<string, Probe>>({})
  let probing = $state(false)
  let probeError = $state('')
  let providerProbing = $state<Record<string, boolean>>({})

  // Bounded concurrency & limits
  let probeConcurrency = $state(4)
  let probeLimit = $state(20)
  let probeSort = $state<'default' | 'fastest' | 'status'>('default')

  // View mode persistence
  let view = $state<'grid' | 'wall' | 'list'>(
    (() => {
      if (typeof localStorage === 'undefined') return 'grid'
      const saved = localStorage.getItem('9router_model_status_view')
      if (saved === 'wall' || saved === 'list') return saved
      return 'grid'
    })()
  )

  function setView(v: 'grid' | 'wall' | 'list') {
    view = v
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem('9router_model_status_view', v)
    }
  }

  // Favorite providers management (persisted in localStorage)
  let favorites = $state<string[]>(
    (() => {
      if (typeof localStorage === 'undefined') return []
      try {
        const stored = localStorage.getItem('9router_favorite_providers')
        return stored ? JSON.parse(stored) : []
      } catch {
        return []
      }
    })()
  )

  function toggleFavorite(providerKey: string) {
    if (favorites.includes(providerKey)) {
      favorites = favorites.filter((k) => k !== providerKey)
    } else {
      favorites = [...favorites, providerKey]
    }
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem('9router_favorite_providers', JSON.stringify(favorites))
    }
  }

  let copiedId = $state<string | null>(null)
  function copyModelId(id: string) {
    if (navigator?.clipboard?.writeText) {
      navigator.clipboard.writeText(id)
      copiedId = id
      setTimeout(() => {
        if (copiedId === id) copiedId = null
      }, 2000)
    }
  }

  // Open single model detail
  function openModelDetail(m: JailbreakModelStatus) {
    detailModel = m
    selectedModel = m
    showDetailModal = true
  }

  // Open full test results report
  function openFullResults(providerKey?: string, statusFilter?: 'all' | 'fail' | 'slow' | 'ok') {
    if (providerKey) {
      fullResultsProvider = providerKey
    } else {
      fullResultsProvider = 'all'
    }
    if (statusFilter) {
      fullResultsStatus = statusFilter
    } else {
      fullResultsStatus = 'all'
    }
    fullResultsSearch = ''
    showFullResultsModal = true
  }

  function formatTimestamp(ts?: number): string {
    if (!ts) return 'Belum pernah'
    const d = new Date(ts)
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  }

  const FILTERS = [
    ['all', 'All'],
    ['injected', 'On'],
    ['excluded', 'Excluded'],
    ['off', 'Off'],
  ] as const

  const TONES: Record<string, 'primary' | 'error' | 'default'> = {
    injected: 'primary',
    excluded: 'error',
    off: 'default',
  }

  const LABELS: Record<string, string> = {
    injected: 'On',
    excluded: 'Excluded',
    off: 'Off',
  }

  onMount(load)

  async function load() {
    loading = true
    error = ''
    try {
      const [res, nodes] = await Promise.all([
        api.getJailbreakModelStatus(),
        api.getProviderNodes().catch(() => [] as ProviderNode[])
      ])
      models = Array.isArray(res?.models) ? res.models : []
      counts = res?.counts || {}
      providerNodes = Array.isArray(nodes) ? nodes : []
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      loading = false
    }
  }

  // Ping a single model and record testedAt timestamp
  async function pingOne(id: string) {
    probes = { ...probes, [id]: { state: 'running', testedAt: Date.now() } }
    try {
      const r = await api.testModel(id)
      probes = {
        ...probes,
        [id]: {
          state: r.ok ? 'ok' : 'fail',
          latencyMs: r.latencyMs,
          error: r.ok ? undefined : r.error || 'probe failed',
          status: r.status,
          note: r.note,
          testedAt: Date.now(),
        },
      }
    } catch (e) {
      probes = {
        ...probes,
        [id]: {
          state: 'fail',
          error: e instanceof Error ? e.message : String(e),
          testedAt: Date.now(),
        },
      }
    }
  }

  // Ping all models for a specific provider
  async function pingProvider(groupKey: string, groupItems: JailbreakModelStatus[]) {
    if (providerProbing[groupKey]) return
    providerProbing = { ...providerProbing, [groupKey]: true }
    try {
      const targets = groupItems.map((m) => m.id)
      let cursor = 0
      async function worker() {
        while (cursor < targets.length) {
          const id = targets[cursor++]
          await pingOne(id)
        }
      }
      await Promise.all(
        Array.from({ length: Math.min(3, targets.length) }, worker)
      )
    } finally {
      providerProbing = { ...providerProbing, [groupKey]: false }
    }
  }

  // Ping filtered rows
  async function pingVisible() {
    probing = true
    probeError = ''
    const targets = filtered.slice(0, probeLimit).map((m) => m.id)
    let cursor = 0
    async function worker() {
      while (cursor < targets.length) {
        const id = targets[cursor++]
        await pingOne(id)
      }
    }
    try {
      await Promise.all(
        Array.from({ length: Math.min(probeConcurrency, targets.length) }, worker)
      )
    } catch (e) {
      probeError = e instanceof Error ? e.message : String(e)
    } finally {
      probing = false
    }
  }

  let probeStats = $derived.by(() => {
    const values = Object.values(probes)
    const ok = values.filter((p) => p.state === 'ok' && (p.latencyMs ?? 0) > 0)
    const lat = ok.map((p) => p.latencyMs as number).sort((a, b) => a - b)
    const median = lat.length ? lat[Math.floor(lat.length / 2)] : 0
    return {
      tested: values.filter((p) => p.state === 'ok' || p.state === 'fail').length,
      up: values.filter((p) => p.state === 'ok').length,
      down: values.filter((p) => p.state === 'fail').length,
      slow: values.filter((p) => p.state === 'ok' && (p.latencyMs ?? 0) >= 2500).length,
      median,
      min: lat.length ? lat[0] : 0,
      max: lat.length ? lat[lat.length - 1] : 0,
    }
  })

  // Full tested models flat list for the comprehensive report
  let testedModelsList = $derived.by(() => {
    return models
      .filter((m) => probes[m.id] && probes[m.id].state !== 'idle')
      .map((m) => {
        const p = probes[m.id]!
        const parsed = cleanModelName(m.id)
        const prov = providerOf(m.id)
        const provMeta = getProviderMeta(parsed.providerPrefix || prov)
        const ms = p.latencyMs ?? 0
        const isSlow = p.state === 'ok' && ms >= 2500
        const isOk = p.state === 'ok' && ms < 2500
        const isFail = p.state === 'fail'
        const isRunning = p.state === 'running'
        return {
          model: m,
          probe: p,
          parsed,
          providerMeta: provMeta,
          isSlow,
          isOk,
          isFail,
          isRunning,
        }
      })
  })

  // Filtered results for the Full List Modal
  let filteredFullResults = $derived.by(() => {
    const q = fullResultsSearch.trim().toLowerCase()
    return testedModelsList
      .filter((item) => {
        // Provider filter
        if (fullResultsProvider !== 'all' && item.providerMeta.key !== fullResultsProvider) {
          return false
        }
        // Status filter
        if (fullResultsStatus === 'fail' && !item.isFail) return false
        if (fullResultsStatus === 'slow' && !item.isSlow) return false
        if (fullResultsStatus === 'ok' && !item.isOk) return false

        // Search query
        if (q) {
          const matchesId = item.model.id.toLowerCase().includes(q)
          const matchesName = item.parsed.title.toLowerCase().includes(q)
          const matchesProv = item.providerMeta.displayName.toLowerCase().includes(q)
          const matchesErr = (item.probe.error || '').toLowerCase().includes(q)
          const matchesNote = (item.probe.note || '').toLowerCase().includes(q)
          if (!matchesId && !matchesName && !matchesProv && !matchesErr && !matchesNote) {
            return false
          }
        }
        return true
      })
      .sort((a, b) => {
        // Down first, then slow, then ok
        const rank = (x: typeof a) => (x.isFail ? 0 : x.isSlow ? 1 : x.isRunning ? 2 : 3)
        const ra = rank(a)
        const rb = rank(b)
        if (ra !== rb) return ra - rb
        return (b.probe.testedAt ?? 0) - (a.probe.testedAt ?? 0)
      })
  })

  function copyFailedModelIds() {
    const failedIds = testedModelsList.filter((item) => item.isFail).map((item) => item.model.id)
    if (failedIds.length === 0) return
    if (navigator?.clipboard?.writeText) {
      navigator.clipboard.writeText(failedIds.join('\n'))
      copiedId = 'FAILED_LIST'
      setTimeout(() => {
        if (copiedId === 'FAILED_LIST') copiedId = null
      }, 2000)
    }
  }

  async function retestFailed() {
    const failedIds = testedModelsList.filter((item) => item.isFail).map((item) => item.model.id)
    if (failedIds.length === 0) return
    let cursor = 0
    async function worker() {
      while (cursor < failedIds.length) {
        const id = failedIds[cursor++]
        await pingOne(id)
      }
    }
    await Promise.all(
      Array.from({ length: Math.min(probeConcurrency, failedIds.length) }, worker)
    )
  }

  function latencyLabel(p: Probe | undefined): string {
    if (!p || p.state === 'idle') return '—'
    if (p.state === 'running') return '…'
    if (p.state === 'fail') return 'down'
    return (p.latencyMs ?? 0) > 0 ? `${p.latencyMs} ms` : 'ok'
  }

  function latencyTone(p: Probe | undefined): 'primary' | 'error' | 'default' {
    if (!p || p.state === 'idle') return 'default'
    if (p.state === 'fail') return 'error'
    if (p.state === 'running') return 'default'
    return 'primary'
  }

  function providerOf(id: string): string {
    const slash = id.indexOf('/')
    return slash > 0 ? id.slice(0, slash) : '(combos & bare ids)'
  }

  export interface ProviderMeta {
    key: string
    displayName: string
    shortName: string
    prefix?: string
    icon?: string
    isCustom?: boolean
  }

  function getProviderMeta(rawProvider: string): ProviderMeta {
    if (rawProvider === '(combos & bare ids)' || rawProvider === 'combos') {
      return {
        key: rawProvider,
        displayName: 'Combos & Direct Models',
        shortName: 'Combos',
        icon: 'alt_route',
      }
    }

    const customMatch = providerNodes.find((n) => {
      if (n.prefix && n.prefix.toLowerCase() === rawProvider.toLowerCase()) return true
      if (n.id && n.id.toLowerCase() === rawProvider.toLowerCase()) return true
      if (n.name && n.name.toLowerCase() === rawProvider.toLowerCase()) return true
      return false
    })

    if (customMatch) {
      const pfx = customMatch.prefix ? customMatch.prefix : ''
      return {
        key: rawProvider,
        displayName: pfx ? `${customMatch.name} (${pfx})` : customMatch.name,
        shortName: customMatch.name,
        prefix: pfx,
        icon: 'hub',
        isCustom: true,
      }
    }

    const catalogMatch = PROVIDER_CATALOG.find((p) => {
      if (p.id.toLowerCase() === rawProvider.toLowerCase()) return true
      if (p.alias && p.alias.toLowerCase() === rawProvider.toLowerCase()) return true
      return false
    })

    if (catalogMatch) {
      return {
        key: rawProvider,
        displayName: catalogMatch.name,
        shortName: catalogMatch.name,
        icon: catalogMatch.icon || 'smart_toy',
      }
    }

    let clean = rawProvider
    if (clean.startsWith('openai-compatible-chat-')) {
      clean = 'Custom (' + clean.slice(23, 31) + ')'
    } else if (clean.startsWith('anthropic-compatible-')) {
      clean = 'Custom (' + clean.slice(21, 29) + ')'
    } else if (clean.length > 0) {
      clean = clean.charAt(0).toUpperCase() + clean.slice(1)
    }

    return {
      key: rawProvider,
      displayName: clean,
      shortName: clean,
      icon: 'smart_toy',
    }
  }

  function cleanModelName(id: string): { title: string; providerPrefix: string } {
    const slash = id.indexOf('/')
    if (slash > 0) {
      return {
        providerPrefix: id.slice(0, slash),
        title: id.slice(slash + 1),
      }
    }
    return {
      providerPrefix: '',
      title: id,
    }
  }

  let filtered = $derived.by(() => {
    const q = search.trim().toLowerCase()
    const rows = models.filter((m) => {
      if (filter !== 'all' && m.status !== filter) return false
      if (!q) return true
      const p = providerOf(m.id)
      const meta = getProviderMeta(p)
      return (
        m.id.toLowerCase().includes(q) ||
        meta.displayName.toLowerCase().includes(q) ||
        meta.shortName.toLowerCase().includes(q) ||
        (m.presetName || '').toLowerCase().includes(q) ||
        (m.reason || '').toLowerCase().includes(q)
      )
    })
    return sortProbed(rows)
  })

  function sortProbed(rows: JailbreakModelStatus[]): JailbreakModelStatus[] {
    if (probeSort === 'default') return rows
    const rank = (p: Probe | undefined): number => {
      if (!p) return Number.MAX_SAFE_INTEGER
      if (p.state === 'ok') return 0
      if (p.state === 'running') return 1
      if (p.state === 'fail') return 2
      return Number.MAX_SAFE_INTEGER
    }
    const lat = (p: Probe | undefined): number => {
      const ms = p?.latencyMs ?? 0
      return ms > 0 ? ms : Number.MAX_SAFE_INTEGER
    }
    return [...rows].sort((a, b) => {
      const pa = probes[a.id]
      const pb = probes[b.id]
      if (probeSort === 'status') {
        const order = (p: Probe | undefined): number =>
          p?.state === 'fail' ? 0 : p?.state === 'ok' ? 1 : p?.state === 'running' ? 2 : 3
        const oa = order(pa)
        const ob = order(pb)
        if (oa !== ob) return oa - ob
        return lat(pa) - lat(pb)
      }
      const ra = rank(pa)
      const rb = rank(pb)
      if (ra !== rb) return ra - rb
      return lat(pa) - lat(pb)
    })
  }

  let pageCount = $derived(Math.max(1, Math.ceil(filtered.length / pageSize)))
  let paged = $derived.by(() => filtered.slice((page - 1) * pageSize, page * pageSize))
  $effect(() => {
    if (page > pageCount) page = pageCount
  })

  // Group by provider with smart sorting:
  // 1. Favorited providers first
  // 2. Currently probing providers
  // 3. Providers with tested models (most recently tested at the top!)
  // 4. Untested providers
  let providerGroups = $derived.by(() => {
    const byProvider = new Map<string, { meta: ProviderMeta; items: JailbreakModelStatus[] }>()
    for (const m of filtered) {
      const p = providerOf(m.id)
      const existing = byProvider.get(p)
      if (existing) {
        existing.items.push(m)
      } else {
        byProvider.set(p, {
          meta: getProviderMeta(p),
          items: [m],
        })
      }
    }

    const groups = [...byProvider.values()].map((g) => ({
      ...g,
      items: g.items.slice().sort((a, b) => {
        const pa = probes[a.id]
        const pb = probes[b.id]
        if (probeSort === 'status') {
          const order = (p?: Probe) => (p?.state === 'fail' ? 0 : p?.state === 'ok' ? 1 : p?.state === 'running' ? 2 : 3)
          const diff = order(pa) - order(pb)
          if (diff !== 0) return diff
        }
        return a.id.localeCompare(b.id)
      }),
    }))

    return groups.sort((a, b) => {
      // 1. Favorites at the very top!
      const aFav = favorites.includes(a.meta.key) ? 1 : 0
      const bFav = favorites.includes(b.meta.key) ? 1 : 0
      if (aFav !== bFav) return bFav - aFav

      // 2. Currently probing
      const aProbing = providerProbing[a.meta.key] ? 1 : 0
      const bProbing = providerProbing[b.meta.key] ? 1 : 0
      if (aProbing !== bProbing) return bProbing - aProbing

      // 3. Recently tested provider moves to the top!
      const aTestedLatest = Math.max(0, ...a.items.map((m) => probes[m.id]?.testedAt ?? 0))
      const bTestedLatest = Math.max(0, ...b.items.map((m) => probes[m.id]?.testedAt ?? 0))

      const aHasTested = aTestedLatest > 0 ? 1 : 0
      const bHasTested = bTestedLatest > 0 ? 1 : 0
      if (aHasTested !== bHasTested) return bHasTested - aHasTested

      if (aTestedLatest !== bTestedLatest) {
        return bTestedLatest - aTestedLatest // Most recently tested first!
      }

      // 4. Alphabetical fallback
      return a.meta.displayName.localeCompare(b.meta.displayName)
    })
  })

  // Color rules requested by user:
  // - SUKSES (OK) = HIJAU (Green)
  // - LAMBAT (High Latency >= 2500ms) = KUNING (Yellow/Amber)
  // - DOWN (Error/Fail) = MERAH (Red)
  // - UNTESTED = Abu-abu (Dark slate)
  function getModelHealthClass(m: JailbreakModelStatus): { bg: string; border: string; glow?: string } {
    const p = probes[m.id]
    if (p?.state === 'running') {
      return {
        bg: 'bg-amber-400 dark:bg-amber-500 animate-pulse',
        border: 'border-amber-500/60 dark:border-amber-400/60',
      }
    }
    if (p?.state === 'fail') {
      return {
        bg: 'bg-rose-500 dark:bg-rose-600',
        border: 'border-rose-700/60 dark:border-rose-400/50',
        glow: 'shadow-[0_0_6px_rgba(244,63,94,0.4)]',
      }
    }
    if (p?.state === 'ok') {
      const ms = p.latencyMs ?? 0
      // LAMBAT: >= 2500ms = KUNING (Yellow)
      if (ms >= 2500) {
        return {
          bg: 'bg-amber-500 dark:bg-[#f59e0b]',
          border: 'border-amber-600/60 dark:border-amber-400/60',
          glow: 'shadow-[0_0_6px_rgba(245,158,11,0.35)]',
        }
      }
      // SUKSES CEPAT: < 600ms = HIJAU Terang (Bright Emerald)
      if (ms > 0 && ms < 600) {
        return {
          bg: 'bg-emerald-400 dark:bg-[#39d353]',
          border: 'border-emerald-600/50 dark:border-emerald-300/50',
          glow: 'shadow-[0_0_6px_rgba(57,211,83,0.35)]',
        }
      }
      // SUKSES NORMAL: < 2500ms = HIJAU (Green)
      return {
        bg: 'bg-emerald-500 dark:bg-[#22c55e]',
        border: 'border-emerald-600/50 dark:border-emerald-400/50',
      }
    }
    if (m.status === 'injected') {
      return {
        bg: 'bg-indigo-500 dark:bg-indigo-600',
        border: 'border-indigo-600/50 dark:border-indigo-400/40',
      }
    }
    if (m.status === 'excluded') {
      return {
        bg: 'bg-surface-3 opacity-60',
        border: 'border-border-subtle',
      }
    }
    // UNTESTED: Kotak abu-abu netral
    return {
      bg: 'bg-[#ebedf0] dark:bg-[#21262d] hover:bg-[#d0d7de] dark:hover:bg-[#30363d]',
      border: 'border-[#d0d7de] dark:border-[#30363d]/80',
    }
  }

  function cellTitle(m: JailbreakModelStatus): string {
    const p = probes[m.id]
    const parsed = cleanModelName(m.id)
    const provMeta = getProviderMeta(parsed.providerPrefix || providerOf(m.id))
    const bits = [
      `Model: ${parsed.title}`,
      `Provider: ${provMeta.displayName}`,
      `Routing ID: ${m.id}`,
      `Injection: ${LABELS[m.status] || m.status}`,
    ]
    if (m.presetName) bits.push(`Preset: ${m.presetName}`)
    if (p?.state === 'ok') {
      const ms = p.latencyMs ?? 0
      bits.push(`Status: ${ms >= 2500 ? 'Lambat' : 'Sukses'} (${ms} ms)`)
    } else if (p?.state === 'fail') {
      bits.push(`Status: Down (${p.error || 'probe failed'})`)
    } else if (p?.state === 'running') {
      bits.push('Status: Probing…')
    } else {
      bits.push('Status: Belum Dites')
    }
    bits.push('(Klik untuk melihat detail lengkap)')
    return bits.join('\n')
  }
</script>

<div class="space-y-4">
  <Card padding="md" class="overflow-hidden">
    <!-- Header bar with Title, Summary, and Controls -->
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex items-center gap-2">
        <h2 class="text-sm font-bold text-text-main flex items-center gap-2">
          <span class="material-symbols-outlined text-brand-500 text-[20px]">dataset</span>
          Model Status
          <span class="text-text-subtle font-normal text-xs">({filtered.length} model dari {providerGroups.length} provider)</span>
        </h2>
      </div>

      <div class="flex items-center gap-2">
        <!-- View Mode Selector -->
        <div class="flex rounded-[8px] border border-border-subtle overflow-hidden bg-surface-2/60">
          <button
            type="button"
            onclick={() => setView('grid')}
            class="px-2.5 py-1 text-xs flex items-center gap-1.5 transition-colors {view === 'grid'
              ? 'bg-brand-500/15 text-brand-500 font-semibold'
              : 'text-text-muted hover:bg-surface-3'}"
            title="Grid: Compact GitHub contribution cards by provider"
          >
            <span class="material-symbols-outlined text-[15px]">grid_view</span>
            Grid
          </button>
          <button
            type="button"
            onclick={() => setView('wall')}
            class="px-2.5 py-1 text-xs border-l border-border-subtle flex items-center gap-1.5 transition-colors {view === 'wall'
              ? 'bg-brand-500/15 text-brand-500 font-semibold'
              : 'text-text-muted hover:bg-surface-3'}"
            title="Wall: High-density contribution matrix without scrolling"
          >
            <span class="material-symbols-outlined text-[15px]">density_small</span>
            Wall
          </button>
          <button
            type="button"
            onclick={() => setView('list')}
            class="px-2.5 py-1 text-xs border-l border-border-subtle flex items-center gap-1.5 transition-colors {view === 'list'
              ? 'bg-brand-500/15 text-brand-500 font-semibold'
              : 'text-text-muted hover:bg-surface-3'}"
            title="List: Full table view with pagination"
          >
            <span class="material-symbols-outlined text-[15px]">format_list_bulleted</span>
            List
          </button>
        </div>

        <Button
          variant="secondary"
          size="sm"
          icon="network_check"
          disabled={probing || filtered.length === 0}
          onclick={pingVisible}
        >
          {probing ? 'Pinging…' : `Ping ${Math.min(probeLimit, filtered.length)}`}
        </Button>
        <Button variant="secondary" size="sm" icon="refresh" disabled={loading} onclick={load}>
          {loading ? 'Resolving…' : 'Refresh'}
        </Button>
      </div>
    </div>

    <!-- Color Legend & Full Report Trigger -->
    <div class="flex flex-wrap items-center justify-between gap-3 pt-2 text-xs">
      <div class="flex flex-wrap items-center gap-2.5 text-[11px] text-text-subtle font-mono">
        <span class="flex items-center gap-1.5">
          <span class="w-3 h-3 rounded-[2.5px] bg-emerald-500 dark:bg-[#22c55e] border border-emerald-600/50 inline-block shadow-[0_0_4px_rgba(34,197,94,0.3)]"></span>
          <span class="text-text-main font-semibold">Sukses (Hijau)</span>
        </span>
        <span class="text-border-subtle">·</span>
        <span class="flex items-center gap-1.5">
          <span class="w-3 h-3 rounded-[2.5px] bg-amber-500 dark:bg-[#f59e0b] border border-amber-600/50 inline-block shadow-[0_0_4px_rgba(245,158,11,0.3)]"></span>
          <span class="text-text-main font-semibold">Lambat &ge;2.5s (Kuning)</span>
        </span>
        <span class="text-border-subtle">·</span>
        <span class="flex items-center gap-1.5">
          <span class="w-3 h-3 rounded-[2.5px] bg-rose-500 dark:bg-rose-600 border border-rose-700/50 inline-block shadow-[0_0_4px_rgba(244,63,94,0.3)]"></span>
          <span class="text-text-main font-semibold">Down (Merah)</span>
        </span>
        <span class="text-border-subtle">·</span>
        <span class="flex items-center gap-1.5">
          <span class="w-3 h-3 rounded-[2.5px] bg-[#ebedf0] dark:bg-[#21262d] border border-[#d0d7de] dark:border-[#30363d]/80 inline-block"></span>
          <span class="text-text-subtle">Belum Dites</span>
        </span>
        <span class="text-border-subtle">·</span>
        <span class="flex items-center gap-1 text-amber-400">
          <span class="material-symbols-outlined text-[15px]">star</span>
          <span class="text-text-muted font-sans font-medium">Favorite (Paling Atas)</span>
        </span>
      </div>

      {#if probeStats.tested > 0}
        <button
          type="button"
          onclick={() => openFullResults()}
          class="text-xs font-semibold text-brand-500 hover:text-brand-400 flex items-center gap-1 transition-colors"
        >
          <span class="material-symbols-outlined text-[16px]">list_alt</span>
          Buka Laporan Full List ({probeStats.tested})
        </button>
      {/if}
    </div>

    {#if error}
      <p class="text-xs text-error pt-2">Status sweep failed: {error}</p>
    {/if}
    {#if probeError}
      <p class="text-xs text-error pt-2">Probe failed: {probeError}</p>
    {/if}

    <!-- Probe Stats Bar with Full Report Button -->
    {#if probeStats.tested > 0}
      <div class="flex flex-wrap items-center justify-between gap-2 pt-2 text-[11px] text-text-muted bg-surface-2/40 p-2.5 rounded-lg border border-border-subtle/60 mt-1">
        <div class="flex flex-wrap items-center gap-x-4 gap-y-1">
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
            <strong class="text-emerald-500 font-bold">{probeStats.up - probeStats.slow}</strong> sukses
          </span>
          {#if probeStats.slow > 0}
            <span class="flex items-center gap-1.5">
              <span class="w-2 h-2 rounded-full bg-amber-500"></span>
              <strong class="text-amber-500 font-bold">{probeStats.slow}</strong> lambat
            </span>
          {/if}
          <span class="flex items-center gap-1.5">
            <span class="w-2 h-2 rounded-full bg-rose-500"></span>
            <strong class="text-rose-500 font-bold">{probeStats.down}</strong> down
          </span>
          {#if probeStats.median > 0}
            <span>median <strong class="text-text-main">{probeStats.median} ms</strong></span>
            <span>min {probeStats.min} ms · max {probeStats.max} ms</span>
          {/if}
          <span class="text-text-subtle font-mono">{probeStats.tested} total dites</span>
        </div>

        <button
          type="button"
          onclick={() => openFullResults()}
          class="px-2.5 py-1 rounded-[6px] bg-brand-500/10 hover:bg-brand-500/20 text-brand-500 font-semibold text-xs flex items-center gap-1.5 transition-colors border border-brand-500/30"
          title="Buka daftar lengkap seluruh hasil ping"
        >
          <span class="material-symbols-outlined text-[15px]">list_alt</span>
          Lihat Full List Hasil ({probeStats.tested})
        </button>
      </div>
    {/if}

    <!-- Search, Filters & Controls -->
    <div class="flex flex-wrap items-center gap-2 pt-3 border-t border-border-subtle">
      {#each FILTERS as [key, label] (key)}
        <button
          type="button"
          onclick={() => {
            filter = key
            page = 1
          }}
          class="px-2.5 py-1 rounded-[8px] border text-xs transition-colors {filter === key
            ? 'border-brand-500/40 bg-brand-500/10 text-brand-500 font-medium'
            : 'border-border-subtle text-text-muted hover:bg-surface-2'}"
        >
          {label}
          <span class="text-text-subtle ml-0.5">{key === 'all' ? models.length : counts[key] || 0}</span>
        </button>
      {/each}

      <div class="relative flex-1 min-w-[200px]">
        <span class="material-symbols-outlined absolute left-2.5 top-1/2 -translate-y-1/2 text-[16px] text-text-subtle">search</span>
        <input
          type="text"
          bind:value={search}
          placeholder="Filter model, provider (atria, klien, river, deepseek)…"
          aria-label="Filter models"
          class="w-full pl-8 pr-3 py-1.5 rounded-[10px] bg-surface-2 border border-border-subtle text-text-main text-xs placeholder:text-text-subtle outline-none focus:border-brand-500/50"
        />
      </div>

      <select
        bind:value={probeSort}
        aria-label="Sort order"
        class="px-2 py-1.5 rounded-[10px] bg-surface-2 border border-border-subtle text-text-main text-xs outline-none"
      >
        <option value="default">Urutan default</option>
        <option value="fastest">Tercepat pertama</option>
        <option value="status">Down pertama</option>
      </select>

      <select
        bind:value={probeLimit}
        aria-label="Probe count"
        class="px-2 py-1.5 rounded-[10px] bg-surface-2 border border-border-subtle text-text-main text-xs outline-none"
      >
        <option value={10}>Ping 10</option>
        <option value={20}>Ping 20</option>
        <option value={50}>Ping 50</option>
        <option value={100}>Ping 100</option>
      </select>

      {#if view === 'list'}
        <select
          bind:value={pageSize}
          aria-label="Rows per page"
          class="px-2 py-1.5 rounded-[10px] bg-surface-2 border border-border-subtle text-text-main text-xs outline-none"
        >
          <option value={20}>20 / page</option>
          <option value={50}>50 / page</option>
          <option value={100}>100 / page</option>
        </select>
      {/if}
    </div>

    <!-- Active Hover/Selection Inspector Dock -->
    <div class="pt-3">
      {#if activeModel}
        {@const ap = probes[activeModel.id]}
        {@const aParsed = cleanModelName(activeModel.id)}
        {@const aMeta = getProviderMeta(aParsed.providerPrefix || providerOf(activeModel.id))}
        <div class="p-3 rounded-[10px] bg-surface-2/70 border border-brand-500/30 flex flex-wrap items-center justify-between gap-3 shadow-sm transition-all">
          <div class="flex items-center gap-3 min-w-0">
            <div class="w-8 h-8 rounded-lg bg-brand-500/10 border border-brand-500/20 flex items-center justify-center shrink-0">
              <span class="material-symbols-outlined text-[18px] text-brand-500">{aMeta.icon || 'hub'}</span>
            </div>
            <div class="min-w-0">
              <div class="flex items-center gap-2 flex-wrap">
                <span class="text-sm font-bold text-text-main font-code">{aParsed.title}</span>
                <span class="px-2 py-0.5 rounded-[5px] bg-surface-3 border border-border-subtle text-[11px] font-medium text-text-muted">
                  {aMeta.displayName}
                </span>
                {#if aMeta.isCustom}
                  <span class="px-1.5 py-0.5 rounded-[4px] bg-brand-500/10 text-brand-500 text-[10px] font-semibold">Custom</span>
                {/if}
                <Badge tone={TONES[activeModel.status] || 'default'} size="sm">
                  {LABELS[activeModel.status] || activeModel.status}
                </Badge>
                {#if ap?.state === 'ok'}
                  {@const isSlow = (ap.latencyMs ?? 0) >= 2500}
                  <span class="px-2 py-0.5 rounded-[5px] text-[11px] font-bold font-code flex items-center gap-1 {isSlow
                    ? 'bg-amber-500/10 border border-amber-500/30 text-amber-500'
                    : 'bg-emerald-500/10 border border-emerald-500/30 text-emerald-500'}">
                    ⚡ {ap.latencyMs} ms {isSlow ? '(Lambat)' : '(Sukses)'}
                  </span>
                {:else if ap?.state === 'fail'}
                  <span class="px-2 py-0.5 rounded-[5px] bg-rose-500/10 border border-rose-500/30 text-rose-500 text-[11px] font-bold font-code flex items-center gap-1">
                    ❌ Down ({ap.error || 'error'})
                  </span>
                {:else if ap?.state === 'running'}
                  <span class="px-2 py-0.5 rounded-[5px] bg-amber-500/10 border border-amber-500/30 text-amber-500 text-[11px] font-bold font-code animate-pulse flex items-center gap-1">
                    ⏳ Probing…
                  </span>
                {:else}
                  <span class="px-2 py-0.5 rounded-[5px] bg-surface-3 border border-border-subtle text-text-subtle text-[11px] font-code">
                    ○ Belum Dites
                  </span>
                {/if}
              </div>
              <div class="flex items-center gap-2 text-xs text-text-subtle mt-0.5">
                <span class="font-code select-all text-text-muted">{activeModel.id}</span>
                <button
                  type="button"
                  onclick={() => copyModelId(activeModel.id)}
                  class="hover:text-text-main text-text-subtle transition-colors flex items-center gap-1"
                  title="Salin routing ID"
                >
                  <span class="material-symbols-outlined text-[13px]">
                    {copiedId === activeModel.id ? 'check' : 'content_copy'}
                  </span>
                  {#if copiedId === activeModel.id}
                    <span class="text-[10px] text-emerald-500 font-semibold">Tersalin!</span>
                  {/if}
                </button>
                {#if activeModel.presetName}
                  <span>·</span>
                  <span class="text-text-muted">Preset: <strong class="text-text-main">{activeModel.presetName}</strong></span>
                {/if}
              </div>
            </div>
          </div>

          <div class="flex items-center gap-2 shrink-0">
            <Button
              variant="secondary"
              size="sm"
              icon="open_in_new"
              onclick={() => openModelDetail(activeModel)}
            >
              Detail Model
            </Button>
            <Button
              variant="primary"
              size="sm"
              icon="bolt"
              disabled={ap?.state === 'running'}
              onclick={() => pingOne(activeModel.id)}
            >
              {ap?.state === 'running' ? 'Testing…' : 'Ping Model'}
            </Button>
          </div>
        </div>
      {/if}
    </div>

    <!-- Content Modes -->
    {#if loading && models.length === 0}
      <p class="text-xs text-text-subtle pt-6 pb-4 text-center">Memuat seluruh model dari provider…</p>
    {:else if filtered.length === 0}
      <p class="text-xs text-text-subtle pt-6 pb-4 text-center">Tidak ada model yang cocok dengan filter pencarian ini.</p>

    <!-- 1. GRID VIEW: Multi-Column GitHub Cards with Favorites & Test Priority -->
    {:else if view === 'grid'}
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 gap-3 pt-3">
        {#each providerGroups as group (group.meta.key)}
          {@const isFav = favorites.includes(group.meta.key)}
          {@const groupProbing = providerProbing[group.meta.key]}
          {@const groupFast = group.items.filter((m) => probes[m.id]?.state === 'ok' && (probes[m.id]?.latencyMs ?? 0) < 2500).length}
          {@const groupSlow = group.items.filter((m) => probes[m.id]?.state === 'ok' && (probes[m.id]?.latencyMs ?? 0) >= 2500).length}
          {@const groupFailed = group.items.filter((m) => probes[m.id]?.state === 'fail').length}
          {@const groupTestedTotal = groupFast + groupSlow + groupFailed}
          <div class="p-3 rounded-[10px] border transition-all flex flex-col justify-between {isFav
            ? 'border-amber-500/50 bg-amber-500/[0.04] shadow-[0_0_10px_rgba(245,158,11,0.06)]'
            : groupTestedTotal > 0
              ? 'border-brand-500/40 bg-surface-2/70'
              : 'border-border-subtle bg-surface-2/40'} hover:border-brand-500/60">
            <div>
              <!-- Provider Card Header -->
              <div class="flex items-center justify-between gap-1.5 mb-2">
                <div class="flex items-center gap-1.5 min-w-0">
                  <button
                    type="button"
                    onclick={() => toggleFavorite(group.meta.key)}
                    title={isFav ? 'Hapus dari favorit' : 'Pin provider ini ke paling atas'}
                    class="p-0.5 rounded transition-colors {isFav
                      ? 'text-amber-400 hover:text-amber-300'
                      : 'text-text-subtle hover:text-amber-400'}"
                  >
                    <span class="material-symbols-outlined text-[17px] leading-none">
                      {isFav ? 'star' : 'star_outline'}
                    </span>
                  </button>

                  <span class="material-symbols-outlined text-[17px] text-brand-500 shrink-0">{group.meta.icon || 'hub'}</span>
                  <span class="text-xs font-bold text-text-main truncate" title={group.meta.displayName}>
                    {group.meta.displayName}
                  </span>
                  {#if group.meta.isCustom}
                    <span class="px-1 py-0.2 rounded bg-brand-500/10 text-brand-500 text-[9px] font-bold">Custom</span>
                  {/if}
                </div>

                <div class="flex items-center gap-1">
                  {#if groupTestedTotal > 0}
                    <button
                      type="button"
                      onclick={() => openFullResults(group.meta.key)}
                      title="Buka full list hasil untuk {group.meta.displayName}"
                      class="p-1 rounded-[6px] text-text-subtle hover:text-brand-500 hover:bg-surface-3 transition-colors"
                    >
                      <span class="material-symbols-outlined text-[15px]">list_alt</span>
                    </button>
                  {/if}
                  <button
                    type="button"
                    disabled={groupProbing}
                    onclick={() => pingProvider(group.meta.key, group.items)}
                    title="Ping semua model di {group.meta.displayName}"
                    class="p-1 rounded-[6px] text-text-subtle hover:text-brand-500 hover:bg-surface-3 transition-colors shrink-0 disabled:opacity-40"
                  >
                    <span class="material-symbols-outlined text-[15px] {groupProbing ? 'animate-spin' : ''}">
                      {groupProbing ? 'progress_activity' : 'bolt'}
                    </span>
                  </button>
                </div>
              </div>

              <!-- Provider Card Subheader / Counter -->
              <div class="flex items-center justify-between text-[11px] text-text-subtle mb-2.5 pb-2 border-b border-border-subtle/50">
                <span>{group.items.length} model</span>
                <div class="flex items-center gap-1.5 text-[10px] font-mono">
                  {#if groupFast > 0}
                    <button
                      type="button"
                      onclick={() => openFullResults(group.meta.key, 'ok')}
                      class="text-emerald-500 font-semibold hover:underline"
                      title="Lihat model sukses"
                    >{groupFast} ok</button>
                  {/if}
                  {#if groupSlow > 0}
                    <button
                      type="button"
                      onclick={() => openFullResults(group.meta.key, 'slow')}
                      class="text-amber-500 font-semibold hover:underline"
                      title="Lihat model lambat"
                    >{groupSlow} slow</button>
                  {/if}
                  {#if groupFailed > 0}
                    <button
                      type="button"
                      onclick={() => openFullResults(group.meta.key, 'fail')}
                      class="text-rose-500 font-semibold hover:underline flex items-center gap-0.5"
                      title="Lihat model down"
                    >
                      <span class="material-symbols-outlined text-[11px]">warning</span>
                      {groupFailed} down
                    </button>
                  {/if}
                  {#if groupTestedTotal === 0}
                    <span class="text-text-subtle">untested</span>
                  {/if}
                </div>
              </div>

              <!-- GitHub Contribution Square Matrix -->
              <div class="flex flex-wrap gap-[3.5px] items-center p-2 rounded-lg bg-surface-1/60 border border-border-subtle/40">
                {#each group.items as m (m.id)}
                  {@const h = getModelHealthClass(m)}
                  {@const isSelected = activeModel?.id === m.id}
                  <button
                    type="button"
                    onclick={() => openModelDetail(m)}
                    onmouseenter={() => (hoveredModel = m)}
                    onmouseleave={() => (hoveredModel = null)}
                    title={cellTitle(m)}
                    aria-label={m.id}
                    class="w-3.5 h-3.5 rounded-[2.5px] border {h.border} {h.bg} {h.glow || ''} {isSelected
                      ? 'ring-2 ring-brand-500 ring-offset-1 scale-125 z-10'
                      : 'hover:scale-125 hover:z-10'} transition-all cursor-pointer"
                  ></button>
                {/each}
              </div>
            </div>
          </div>
        {/each}
      </div>

    <!-- 2. WALL VIEW: Ultra-Dense Single Heatmap Matrix -->
    {:else if view === 'wall'}
      <div class="pt-3 space-y-2">
        {#each providerGroups as group (group.meta.key)}
          {@const isFav = favorites.includes(group.meta.key)}
          {@const groupProbing = providerProbing[group.meta.key]}
          {@const groupFast = group.items.filter((m) => probes[m.id]?.state === 'ok' && (probes[m.id]?.latencyMs ?? 0) < 2500).length}
          {@const groupSlow = group.items.filter((m) => probes[m.id]?.state === 'ok' && (probes[m.id]?.latencyMs ?? 0) >= 2500).length}
          {@const groupFailed = group.items.filter((m) => probes[m.id]?.state === 'fail').length}
          {@const groupTestedTotal = groupFast + groupSlow + groupFailed}
          <div class="flex items-center gap-3 p-2 rounded-[8px] border transition-colors {isFav
            ? 'border-amber-500/50 bg-amber-500/[0.04]'
            : 'border-border-subtle/60 bg-surface-2/40 hover:bg-surface-2/70'}">
            <!-- Left: Provider identity -->
            <div class="w-52 shrink-0 flex items-center justify-between gap-1 pr-2 border-r border-border-subtle/60">
              <div class="flex items-center gap-1.5 min-w-0">
                <button
                  type="button"
                  onclick={() => toggleFavorite(group.meta.key)}
                  title={isFav ? 'Hapus dari favorit' : 'Pin favorit'}
                  class="p-0.5 rounded {isFav ? 'text-amber-400' : 'text-text-subtle hover:text-amber-400'}"
                >
                  <span class="material-symbols-outlined text-[15px]">
                    {isFav ? 'star' : 'star_outline'}
                  </span>
                </button>
                <span class="material-symbols-outlined text-[15px] text-brand-500 shrink-0">{group.meta.icon || 'hub'}</span>
                <span class="text-xs font-semibold text-text-main truncate" title={group.meta.displayName}>
                  {group.meta.displayName}
                </span>
                {#if group.meta.isCustom}
                  <span class="px-1 py-0.2 rounded bg-brand-500/10 text-brand-500 text-[9px] font-bold shrink-0">Custom</span>
                {/if}
              </div>
              <span class="text-[10px] text-text-subtle font-mono shrink-0">
                {group.items.length}
              </span>
            </div>

            <!-- Middle: GitHub Squares in tight rows -->
            <div class="flex-1 flex flex-wrap gap-[3px] items-center min-w-0">
              {#each group.items as m (m.id)}
                {@const h = getModelHealthClass(m)}
                {@const isSelected = activeModel?.id === m.id}
                <button
                  type="button"
                  onclick={() => openModelDetail(m)}
                  onmouseenter={() => (hoveredModel = m)}
                  onmouseleave={() => (hoveredModel = null)}
                  title={cellTitle(m)}
                  aria-label={m.id}
                  class="w-3 h-3 rounded-[2px] border {h.border} {h.bg} {h.glow || ''} {isSelected
                    ? 'ring-2 ring-brand-500 scale-125 z-10'
                    : 'hover:scale-125 hover:z-10'} transition-all cursor-pointer"
                ></button>
              {/each}
            </div>

            <!-- Right: Status summary & ping button -->
            <div class="shrink-0 flex items-center gap-2 pl-2 border-l border-border-subtle/60">
              <div class="text-[10px] font-mono text-text-subtle flex items-center gap-1.5">
                {#if groupFast > 0}
                  <button
                    type="button"
                    onclick={() => openFullResults(group.meta.key, 'ok')}
                    class="text-emerald-500 font-semibold hover:underline"
                  >{groupFast} ok</button>
                {/if}
                {#if groupSlow > 0}
                  <button
                    type="button"
                    onclick={() => openFullResults(group.meta.key, 'slow')}
                    class="text-amber-500 font-semibold hover:underline"
                  >{groupSlow} slow</button>
                {/if}
                {#if groupFailed > 0}
                  <button
                    type="button"
                    onclick={() => openFullResults(group.meta.key, 'fail')}
                    class="text-rose-500 font-semibold hover:underline"
                  >{groupFailed} down</button>
                {/if}
              </div>
              {#if groupTestedTotal > 0}
                <button
                  type="button"
                  onclick={() => openFullResults(group.meta.key)}
                  title="Buka full list hasil untuk {group.meta.displayName}"
                  class="p-1 rounded-[5px] text-text-subtle hover:text-brand-500 hover:bg-surface-3 transition-colors"
                >
                  <span class="material-symbols-outlined text-[14px]">list_alt</span>
                </button>
              {/if}
              <button
                type="button"
                disabled={groupProbing}
                onclick={() => pingProvider(group.meta.key, group.items)}
                title="Ping all models in {group.meta.displayName}"
                class="p-1 rounded-[5px] text-text-subtle hover:text-brand-500 hover:bg-surface-3 transition-colors disabled:opacity-40"
              >
                <span class="material-symbols-outlined text-[14px] {groupProbing ? 'animate-spin' : ''}">
                  {groupProbing ? 'progress_activity' : 'bolt'}
                </span>
              </button>
            </div>
          </div>
        {/each}
      </div>

    <!-- 3. LIST VIEW: Full Table View with Pagination -->
    {:else}
      <div class="-mx-5 mt-4 border-t border-border-subtle">
        <ul class="divide-y border-border-subtle divide-border-subtle">
          {#each paged as m (m.id)}
            {@const p = probes[m.id]}
            {@const parsed = cleanModelName(m.id)}
            {@const provMeta = getProviderMeta(parsed.providerPrefix || providerOf(m.id))}
            <li class="flex items-center gap-3 px-5 py-3 hover:bg-surface-2/30 transition-colors">
              <Badge
                tone={TONES[m.status] || 'default'}
                size="sm"
                class="shrink-0 w-[74px] justify-center"
              >
                {LABELS[m.status] || m.status}
              </Badge>
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2 flex-wrap">
                  <button
                    type="button"
                    onclick={() => openModelDetail(m)}
                    class="text-sm font-semibold text-text-main font-code hover:text-brand-500 transition-colors text-left"
                  >
                    {parsed.title}
                  </button>
                  <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-[6px] bg-surface-3 border border-border-subtle text-[11px] font-medium text-text-muted">
                    <span class="material-symbols-outlined text-[13px] text-text-subtle">{provMeta.icon || 'hub'}</span>
                    {provMeta.displayName}
                  </span>
                  {#if provMeta.isCustom}
                    <span class="px-1.5 py-0.5 rounded-[4px] bg-brand-500/10 text-brand-500 text-[10px] font-semibold">Custom</span>
                  {/if}
                </div>
                <div class="flex items-center gap-2 mt-1 text-[11px] text-text-subtle truncate">
                  <span class="font-code select-all text-text-subtle">{m.id}</span>
                  <button
                    type="button"
                    onclick={() => copyModelId(m.id)}
                    class="hover:text-text-main text-text-subtle transition-colors"
                    title="Copy model ID"
                  >
                    <span class="material-symbols-outlined text-[13px] leading-none align-middle">
                      {copiedId === m.id ? 'check' : 'content_copy'}
                    </span>
                  </button>
                  {#if p?.error || m.detail || m.reason}
                    <span>·</span>
                    <span class="truncate {p?.state === 'fail' ? 'text-error' : ''}">
                      {p?.error || m.detail || m.reason}
                    </span>
                  {/if}
                </div>
              </div>

              {#if m.presetKind}
                <span
                  class="shrink-0 px-1.5 py-0.5 rounded-[6px] bg-surface-3 border border-border-subtle text-[10px] font-semibold uppercase text-text-main"
                  title="Instruction-file kind">{m.presetKind}</span
                >
              {/if}
              {#if m.injectMode}
                <span
                  class="shrink-0 px-1.5 py-0.5 rounded-[6px] border border-border-subtle text-[10px] text-text-muted"
                  title="System injection mode">{m.injectMode}</span
                >
              {/if}
              {#if m.presetName || m.presetId}
                <span
                  class="shrink-0 text-[11px] text-text-muted truncate max-w-[160px]"
                  title={m.presetName || m.presetId}>{m.presetName || m.presetId}</span
                >
              {/if}
              <span
                class="shrink-0"
                title={p?.state === 'fail'
                  ? p?.error || 'probe failed'
                  : p?.note || 'measured upstream latency'}
              >
                <Badge tone={latencyTone(p)} size="sm" class="w-[64px] justify-center font-code">
                  {latencyLabel(p)}
                </Badge>
              </span>
              <button
                type="button"
                onclick={() => openModelDetail(m)}
                title="Buka detail lengkap model ini"
                class="shrink-0 p-1.5 rounded-[6px] text-text-muted hover:text-brand-500 hover:bg-surface-2 transition-colors"
              >
                <span class="material-symbols-outlined text-[16px] leading-none align-middle">info</span>
              </button>
              <button
                type="button"
                onclick={() => pingOne(m.id)}
                disabled={p?.state === 'running'}
                title="Ping this model through the provider"
                aria-label="Ping {m.id}"
                class="shrink-0 px-2 py-1 rounded-[6px] border border-border-subtle text-text-muted hover:text-text-main hover:bg-surface-2 disabled:opacity-50 transition-colors"
              >
                <span class="material-symbols-outlined text-[15px] leading-none align-middle">bolt</span>
              </button>
            </li>
          {/each}
        </ul>
      </div>
      <Pagination
        page={page}
        pageCount={pageCount}
        total={filtered.length}
        noun="model"
        onPage={(p) => (page = p)}
      />
    {/if}
  </Card>
</div>

<!-- FLOATING SWEEP STATUS PILL (WHILE PROBING) -->
{#if probing}
  <div class="fixed bottom-5 right-5 z-40 p-3 rounded-xl bg-surface-1 border border-brand-500/50 shadow-2xl flex items-center gap-3 animate-fade-in">
    <div class="w-8 h-8 rounded-lg bg-brand-500/10 flex items-center justify-center">
      <span class="material-symbols-outlined text-[18px] text-brand-500 animate-spin">progress_activity</span>
    </div>
    <div class="text-xs">
      <p class="font-bold text-text-main">Sedang Menguji Model Upstream…</p>
      <p class="text-[11px] text-text-subtle">{probeStats.tested} diuji ({probeStats.up - probeStats.slow} ok · {probeStats.down} down)</p>
    </div>
    <Button variant="secondary" size="sm" onclick={() => openFullResults()}>
      Lihat Full List
    </Button>
  </div>
{/if}

<!-- 1. FULL TEST RESULTS MODAL ("ada yang full list juga") -->
{#if showFullResultsModal}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-5">
    <!-- Backdrop -->
    <div
      class="fixed inset-0 bg-black/80 backdrop-blur-sm transition-opacity"
      onclick={() => (showFullResultsModal = false)}
      onkeydown={(e) => e.key === 'Escape' && (showFullResultsModal = false)}
      role="button"
      tabindex="-1"
    ></div>

    <!-- Modal Card -->
    <div class="relative w-full max-w-5xl rounded-xl bg-surface-1 border border-border-subtle shadow-2xl overflow-hidden z-10 flex flex-col max-h-[92vh]">
      <!-- Header -->
      <div class="px-5 py-3.5 border-b border-border-subtle flex items-center justify-between bg-surface-2/60">
        <div class="flex items-center gap-2.5">
          <span class="material-symbols-outlined text-brand-500 text-[24px]">fact_check</span>
          <div>
            <h3 class="text-sm font-bold text-text-main flex items-center gap-2">
              Laporan Hasil Pengujian Model (Full List)
              <span class="text-text-subtle font-normal text-xs">({filteredFullResults.length} dari {testedModelsList.length} model diuji)</span>
            </h3>
            <p class="text-[11px] text-text-subtle">
              Daftar komprehensif seluruh model yang telah diuji beserta latensi wire dan diagnostik status.
            </p>
          </div>
        </div>

        <button
          type="button"
          onclick={() => (showFullResultsModal = false)}
          class="p-1 rounded-lg text-text-muted hover:text-text-main hover:bg-surface-3 transition-colors"
          title="Tutup dialog"
        >
          <span class="material-symbols-outlined text-[20px]">close</span>
        </button>
      </div>

      <!-- Filters & Actions Bar -->
      <div class="p-3 border-b border-border-subtle bg-surface-2/30 space-y-2.5">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <!-- Status Tabs -->
          <div class="flex rounded-lg border border-border-subtle overflow-hidden bg-surface-2/60">
            <button
              type="button"
              onclick={() => (fullResultsStatus = 'all')}
              class="px-3 py-1 text-xs transition-colors {fullResultsStatus === 'all'
                ? 'bg-brand-500/15 text-brand-500 font-semibold'
                : 'text-text-muted hover:bg-surface-3'}"
            >
              Semua ({testedModelsList.length})
            </button>
            <button
              type="button"
              onclick={() => (fullResultsStatus = 'fail')}
              class="px-3 py-1 text-xs border-l border-border-subtle transition-colors flex items-center gap-1.5 {fullResultsStatus === 'fail'
                ? 'bg-rose-500/15 text-rose-500 font-semibold'
                : 'text-text-muted hover:bg-surface-3'}"
            >
              <span class="w-2 h-2 rounded-full bg-rose-500"></span>
              Down ({probeStats.down})
            </button>
            <button
              type="button"
              onclick={() => (fullResultsStatus = 'slow')}
              class="px-3 py-1 text-xs border-l border-border-subtle transition-colors flex items-center gap-1.5 {fullResultsStatus === 'slow'
                ? 'bg-amber-500/15 text-amber-500 font-semibold'
                : 'text-text-muted hover:bg-surface-3'}"
            >
              <span class="w-2 h-2 rounded-full bg-amber-500"></span>
              Lambat ({probeStats.slow})
            </button>
            <button
              type="button"
              onclick={() => (fullResultsStatus = 'ok')}
              class="px-3 py-1 text-xs border-l border-border-subtle transition-colors flex items-center gap-1.5 {fullResultsStatus === 'ok'
                ? 'bg-emerald-500/15 text-emerald-500 font-semibold'
                : 'text-text-muted hover:bg-surface-3'}"
            >
              <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
              Sukses ({probeStats.up - probeStats.slow})
            </button>
          </div>

          <!-- Quick Actions -->
          <div class="flex items-center gap-2">
            {#if probeStats.down > 0}
              <Button
                variant="secondary"
                size="sm"
                icon="refresh"
                disabled={probing}
                onclick={retestFailed}
              >
                Retest Model Down ({probeStats.down})
              </Button>
              <Button
                variant="secondary"
                size="sm"
                icon="content_copy"
                onclick={copyFailedModelIds}
              >
                {copiedId === 'FAILED_LIST' ? 'Tersalin!' : 'Salin ID Down'}
              </Button>
            {/if}
          </div>
        </div>

        <!-- Filter Provider & Search -->
        <div class="flex flex-wrap items-center gap-2">
          <select
            bind:value={fullResultsProvider}
            aria-label="Filter berdasarkan provider"
            class="px-2.5 py-1.5 rounded-lg bg-surface-2 border border-border-subtle text-xs text-text-main outline-none"
          >
            <option value="all">Semua Provider ({providerGroups.length})</option>
            {#each providerGroups as g}
              {@const gTested = g.items.filter(m => probes[m.id]?.state === 'ok' || probes[m.id]?.state === 'fail').length}
              <option value={g.meta.key}>
                {g.meta.displayName} ({gTested} dites)
              </option>
            {/each}
          </select>

          <div class="relative flex-1 min-w-[200px]">
            <span class="material-symbols-outlined absolute left-2.5 top-1/2 -translate-y-1/2 text-[15px] text-text-subtle">search</span>
            <input
              type="text"
              bind:value={fullResultsSearch}
              placeholder="Cari dalam hasil tes (nama model, error trace, provider)..."
              class="w-full pl-8 pr-3 py-1.5 rounded-lg bg-surface-2 border border-border-subtle text-xs text-text-main placeholder:text-text-subtle outline-none focus:border-brand-500/50"
            />
          </div>
        </div>
      </div>

      <!-- Results Table / List -->
      <div class="flex-1 overflow-y-auto min-h-[320px]">
        {#if filteredFullResults.length === 0}
          <div class="py-16 text-center text-text-subtle text-xs">
            <span class="material-symbols-outlined text-[36px] text-text-subtle mb-1 block">search_off</span>
            Tidak ada hasil pengujian yang cocok dengan filter yang dipilih.
          </div>
        {:else}
          <table class="w-full text-left text-xs border-collapse">
            <thead class="sticky top-0 bg-surface-2 border-b border-border-subtle text-[11px] text-text-muted font-semibold">
              <tr>
                <th class="py-2.5 px-4 w-[110px]">Status</th>
                <th class="py-2.5 px-3">Model & Routing ID</th>
                <th class="py-2.5 px-3 w-[150px]">Provider</th>
                <th class="py-2.5 px-3 w-[110px]">Latency</th>
                <th class="py-2.5 px-3 w-[90px]">HTTP Code</th>
                <th class="py-2.5 px-3">Diagnostik / Error</th>
                <th class="py-2.5 px-4 w-[90px] text-right">Aksi</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border-subtle/50">
              {#each filteredFullResults as item (item.model.id)}
                <tr class="hover:bg-surface-2/40 transition-colors">
                  <!-- Status Badge -->
                  <td class="py-2.5 px-4">
                    {#if item.isOk}
                      <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-[5px] bg-emerald-500/10 border border-emerald-500/30 text-emerald-500 font-semibold text-[11px]">
                        <span class="w-1.5 h-1.5 rounded-full bg-emerald-500"></span> Sukses
                      </span>
                    {:else if item.isSlow}
                      <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-[5px] bg-amber-500/10 border border-amber-500/30 text-amber-500 font-semibold text-[11px]">
                        <span class="w-1.5 h-1.5 rounded-full bg-amber-500"></span> Lambat
                      </span>
                    {:else if item.isFail}
                      <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-[5px] bg-rose-500/10 border border-rose-500/30 text-rose-500 font-semibold text-[11px]">
                        <span class="w-1.5 h-1.5 rounded-full bg-rose-500"></span> Down
                      </span>
                    {:else}
                      <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded-[5px] bg-brand-500/10 border border-brand-500/30 text-brand-500 font-semibold text-[11px] animate-pulse">
                        <span class="w-1.5 h-1.5 rounded-full bg-brand-500"></span> Probing
                      </span>
                    {/if}
                  </td>

                  <!-- Model & Routing ID -->
                  <td class="py-2.5 px-3 min-w-0">
                    <button
                      type="button"
                      onclick={() => openModelDetail(item.model)}
                      class="font-code font-bold text-text-main hover:text-brand-500 transition-colors text-left block truncate max-w-[280px]"
                    >
                      {item.parsed.title}
                    </button>
                    <div class="flex items-center gap-1.5 text-[11px] text-text-subtle font-code mt-0.5">
                      <span class="truncate max-w-[240px] select-all">{item.model.id}</span>
                      <button
                        type="button"
                        onclick={() => copyModelId(item.model.id)}
                        class="hover:text-text-main text-text-subtle transition-colors shrink-0"
                        title="Salin routing ID"
                      >
                        <span class="material-symbols-outlined text-[12px]">content_copy</span>
                      </button>
                    </div>
                  </td>

                  <!-- Provider -->
                  <td class="py-2.5 px-3">
                    <span class="inline-flex items-center gap-1 text-[11px] font-medium text-text-muted">
                      <span class="material-symbols-outlined text-[13px] text-brand-500">{item.providerMeta.icon || 'hub'}</span>
                      <span class="truncate max-w-[130px]" title={item.providerMeta.displayName}>{item.providerMeta.displayName}</span>
                    </span>
                  </td>

                  <!-- Latency -->
                  <td class="py-2.5 px-3 font-code font-semibold">
                    {#if item.probe.state === 'ok'}
                      <span class="{item.isSlow ? 'text-amber-500' : 'text-emerald-500'}">
                        {item.probe.latencyMs} ms
                      </span>
                    {:else if item.probe.state === 'fail'}
                      <span class="text-rose-500">down</span>
                    {:else}
                      <span class="text-text-subtle">…</span>
                    {/if}
                  </td>

                  <!-- HTTP Code -->
                  <td class="py-2.5 px-3 font-code">
                    {#if item.probe.status}
                      <span class="{item.probe.status >= 400 ? 'text-rose-400 font-bold' : 'text-text-main'}">
                        {item.probe.status}
                      </span>
                    {:else if item.probe.state === 'ok'}
                      <span class="text-text-main">200</span>
                    {:else}
                      <span class="text-text-subtle">—</span>
                    {/if}
                  </td>

                  <!-- Diagnostic / Error Trace -->
                  <td class="py-2.5 px-3 text-[11px] max-w-[260px]">
                    {#if item.probe.error}
                      <span class="text-rose-400 font-code truncate block select-all" title={item.probe.error}>
                        {item.probe.error}
                      </span>
                    {:else if item.probe.note}
                      <span class="text-text-muted truncate block" title={item.probe.note}>
                        {item.probe.note}
                      </span>
                    {:else if item.probe.state === 'ok'}
                      <span class="text-emerald-500/80">Reachable & verified</span>
                    {:else}
                      <span class="text-text-subtle">—</span>
                    {/if}
                  </td>

                  <!-- Actions -->
                  <td class="py-2.5 px-4 text-right space-x-1 shrink-0">
                    <button
                      type="button"
                      onclick={() => pingOne(item.model.id)}
                      disabled={item.probe.state === 'running'}
                      title="Ping ulang model ini"
                      class="p-1 rounded text-text-subtle hover:text-brand-500 hover:bg-surface-3 transition-colors disabled:opacity-40"
                    >
                      <span class="material-symbols-outlined text-[15px] {item.probe.state === 'running' ? 'animate-spin' : ''}">
                        bolt
                      </span>
                    </button>
                    <button
                      type="button"
                      onclick={() => openModelDetail(item.model)}
                      title="Buka detail diagnostik lengkap model ini"
                      class="p-1 rounded text-text-subtle hover:text-brand-500 hover:bg-surface-3 transition-colors"
                    >
                      <span class="material-symbols-outlined text-[15px]">info</span>
                    </button>
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
      </div>

      <!-- Footer -->
      <div class="px-5 py-3 border-t border-border-subtle flex items-center justify-between bg-surface-2/40 text-xs">
        <span class="text-text-subtle">
          Klik baris atau ikon <span class="material-symbols-outlined text-[13px] align-middle">info</span> untuk melihat diagnosa per model.
        </span>
        <Button
          variant="secondary"
          size="sm"
          onclick={() => (showFullResultsModal = false)}
        >
          Tutup Laporan
        </Button>
      </div>
    </div>
  </div>
{/if}

<!-- 2. PER-MODEL DIAGNOSTIC DETAIL MODAL ("ada yang per model gitu") -->
{#if showDetailModal && detailModel}
  {@const dp = probes[detailModel.id]}
  {@const dParsed = cleanModelName(detailModel.id)}
  {@const dMeta = getProviderMeta(dParsed.providerPrefix || providerOf(detailModel.id))}
  {@const isSlow = (dp?.latencyMs ?? 0) >= 2500}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <!-- Backdrop -->
    <div
      class="fixed inset-0 bg-black/75 backdrop-blur-sm transition-opacity"
      onclick={() => (showDetailModal = false)}
      onkeydown={(e) => e.key === 'Escape' && (showDetailModal = false)}
      role="button"
      tabindex="-1"
    ></div>

    <!-- Modal Card -->
    <div class="relative w-full max-w-xl rounded-xl bg-surface-1 border border-border-subtle shadow-2xl overflow-hidden z-10 flex flex-col max-h-[90vh]">
      <!-- Modal Header -->
      <div class="px-5 py-4 border-b border-border-subtle flex items-center justify-between bg-surface-2/40">
        <div class="flex items-center gap-3 min-w-0">
          <div class="w-9 h-9 rounded-lg bg-brand-500/10 border border-brand-500/20 flex items-center justify-center shrink-0">
            <span class="material-symbols-outlined text-[20px] text-brand-500">{dMeta.icon || 'smart_toy'}</span>
          </div>
          <div class="min-w-0">
            <h3 class="text-sm font-bold text-text-main font-code truncate">{dParsed.title}</h3>
            <p class="text-xs text-text-subtle flex items-center gap-1.5">
              <span>{dMeta.displayName}</span>
              {#if dMeta.isCustom}
                <span class="px-1.5 py-0.2 rounded bg-brand-500/10 text-brand-500 text-[10px] font-bold">Custom</span>
              {/if}
            </p>
          </div>
        </div>
        <button
          type="button"
          onclick={() => (showDetailModal = false)}
          class="p-1 rounded-lg text-text-muted hover:text-text-main hover:bg-surface-3 transition-colors"
          title="Tutup dialog"
        >
          <span class="material-symbols-outlined text-[20px]">close</span>
        </button>
      </div>

      <!-- Modal Body -->
      <div class="p-5 overflow-y-auto space-y-4">
        <!-- Diagnostic Verdict Banner -->
        {#if dp?.state === 'ok'}
          <div class="p-3.5 rounded-lg border {isSlow
            ? 'bg-amber-500/10 border-amber-500/40 text-amber-500'
            : 'bg-emerald-500/10 border-emerald-500/40 text-emerald-500'} flex items-start gap-3">
            <span class="material-symbols-outlined text-[24px] shrink-0">
              {isSlow ? 'warning' : 'check_circle'}
            </span>
            <div class="min-w-0 flex-1">
              <p class="text-xs font-bold uppercase tracking-wider">
                {isSlow ? 'STATUS: TERHUBUNG TAPI LAMBAT' : 'STATUS: SUKSES / NORMAL'}
              </p>
              <p class="text-xs text-text-main mt-0.5">
                Upstream model merespons dengan waktu <strong class="font-code">{dp.latencyMs} ms</strong>. Model siap melayani routing request.
              </p>
            </div>
          </div>
        {:else if dp?.state === 'fail'}
          <div class="p-3.5 rounded-lg border bg-rose-500/10 border-rose-500/40 text-rose-500 flex items-start gap-3">
            <span class="material-symbols-outlined text-[24px] shrink-0">cancel</span>
            <div class="min-w-0 flex-1">
              <p class="text-xs font-bold uppercase tracking-wider">STATUS: GAGAL / DOWN</p>
              <p class="text-xs text-text-main mt-0.5">
                Upstream gagal merespons atau mengembalikan error code <strong class="font-code">{dp.status || 500}</strong>.
              </p>
            </div>
          </div>
        {:else if dp?.state === 'running'}
          <div class="p-3.5 rounded-lg border bg-brand-500/10 border-brand-500/40 text-brand-500 flex items-start gap-3 animate-pulse">
            <span class="material-symbols-outlined text-[24px] shrink-0">progress_activity</span>
            <div class="min-w-0 flex-1">
              <p class="text-xs font-bold uppercase tracking-wider">STATUS: PING SEDANG BERJALAN…</p>
              <p class="text-xs text-text-main mt-0.5">
                Sedang mengirim request probe ke upstream provider untuk mengukur latensi wire.
              </p>
            </div>
          </div>
        {:else}
          <div class="p-3.5 rounded-lg border bg-surface-2 border-border-subtle text-text-subtle flex items-start gap-3">
            <span class="material-symbols-outlined text-[24px] shrink-0">help_outline</span>
            <div class="min-w-0 flex-1">
              <p class="text-xs font-bold uppercase tracking-wider text-text-main">STATUS: BELUM DITES</p>
              <p class="text-xs text-text-subtle mt-0.5">
                Model ini belum pernah diuji pada sesi ini. Klik tombol <strong>Ping Model Sekarang</strong> di bawah untuk menguji konektivitas.
              </p>
            </div>
          </div>
        {/if}

        <!-- Diagnostic Metrics Grid -->
        <div class="grid grid-cols-3 gap-3">
          <div class="p-3 rounded-lg bg-surface-2/60 border border-border-subtle">
            <span class="text-[10px] text-text-subtle uppercase tracking-wider block font-semibold">Latency</span>
            <span class="text-sm font-bold font-code mt-1 block {dp?.state === 'ok'
              ? isSlow ? 'text-amber-500' : 'text-emerald-500'
              : 'text-text-main'}">
              {dp?.latencyMs ? `${dp.latencyMs} ms` : '—'}
            </span>
            <span class="text-[10px] text-text-subtle">
              {dp?.state === 'ok' ? (isSlow ? 'Lambat (≥2.5s)' : 'Normal (<2.5s)') : 'Belum diukur'}
            </span>
          </div>

          <div class="p-3 rounded-lg bg-surface-2/60 border border-border-subtle">
            <span class="text-[10px] text-text-subtle uppercase tracking-wider block font-semibold">HTTP Code</span>
            <span class="text-sm font-bold font-code mt-1 block {dp?.status && dp.status >= 400 ? 'text-rose-500' : 'text-text-main'}">
              {dp?.status ? dp.status : (dp?.state === 'ok' ? '200 OK' : '—')}
            </span>
            <span class="text-[10px] text-text-subtle">
              {dp?.state === 'ok' ? 'Success response' : (dp?.state === 'fail' ? 'Upstream error' : 'Idle')}
            </span>
          </div>

          <div class="p-3 rounded-lg bg-surface-2/60 border border-border-subtle">
            <span class="text-[10px] text-text-subtle uppercase tracking-wider block font-semibold">Terakhir Dites</span>
            <span class="text-sm font-bold font-code text-text-main mt-1 block truncate">
              {formatTimestamp(dp?.testedAt)}
            </span>
            <span class="text-[10px] text-text-subtle">Waktu uji lokal</span>
          </div>
        </div>

        <!-- Failure or Note Trace (if any) -->
        {#if dp?.error}
          <div class="space-y-1.5">
            <div class="flex items-center justify-between text-xs">
              <span class="font-semibold text-rose-400 flex items-center gap-1">
                <span class="material-symbols-outlined text-[15px]">bug_report</span>
                Pesan Error / Traceback
              </span>
              <button
                type="button"
                onclick={() => copyModelId(dp.error || '')}
                class="text-[11px] text-text-subtle hover:text-text-main transition-colors"
              >
                Salin Pesan Error
              </button>
            </div>
            <pre class="p-3 rounded-lg bg-black/60 border border-rose-500/30 text-rose-300 text-xs font-code whitespace-pre-wrap break-all select-all">{dp.error}</pre>
          </div>
        {/if}

        {#if dp?.note}
          <div class="p-3 rounded-lg bg-surface-2/80 border border-border-subtle text-xs text-text-muted">
            <strong class="text-text-main">Catatan Upstream:</strong> {dp.note}
          </div>
        {/if}

        <!-- Routing & Gateway Configuration Details -->
        <div class="p-3.5 rounded-lg bg-surface-2/40 border border-border-subtle space-y-2">
          <p class="text-xs font-semibold text-text-main uppercase tracking-wider">Informasi Routing Gateway</p>
          <div class="grid grid-cols-2 gap-2 text-xs">
            <div>
              <span class="text-text-subtle block">Full Routing ID:</span>
              <div class="flex items-center gap-1.5 mt-0.5">
                <span class="font-code font-bold text-text-main select-all">{detailModel.id}</span>
                <button
                  type="button"
                  onclick={() => copyModelId(detailModel.id)}
                  class="text-text-subtle hover:text-text-main"
                  title="Salin routing ID"
                >
                  <span class="material-symbols-outlined text-[13px]">
                    {copiedId === detailModel.id ? 'check' : 'content_copy'}
                  </span>
                </button>
              </div>
            </div>

            <div>
              <span class="text-text-subtle block">Injection Verdict:</span>
              <span class="font-semibold text-text-main mt-0.5 inline-block">
                {LABELS[detailModel.status] || detailModel.status}
              </span>
            </div>

            {#if detailModel.presetName}
              <div>
                <span class="text-text-subtle block">Preset Name:</span>
                <span class="text-text-main font-code">{detailModel.presetName}</span>
              </div>
            {/if}

            {#if detailModel.presetKind}
              <div>
                <span class="text-text-subtle block">Preset Kind:</span>
                <span class="text-text-main font-code uppercase">{detailModel.presetKind}</span>
              </div>
            {/if}

            {#if detailModel.reason}
              <div class="col-span-2">
                <span class="text-text-subtle block">Resolution Reason:</span>
                <span class="text-text-muted">{detailModel.reason}</span>
              </div>
            {/if}
          </div>
        </div>
      </div>

      <!-- Modal Footer Actions -->
      <div class="px-5 py-3.5 border-t border-border-subtle flex items-center justify-between bg-surface-2/40">
        <Button
          variant="secondary"
          size="sm"
          icon="content_copy"
          onclick={() => copyModelId(detailModel.id)}
        >
          {copiedId === detailModel.id ? 'Tersalin!' : 'Salin Model ID'}
        </Button>

        <div class="flex items-center gap-2">
          <Button
            variant="secondary"
            size="sm"
            icon="tune"
            onclick={() => openParamsModal(detailModel.id)}
          >
            Setting Parameter
          </Button>
          <Button
            variant="secondary"
            size="sm"
            onclick={() => (showDetailModal = false)}
          >
            Tutup
          </Button>
          <Button
            variant="primary"
            size="sm"
            icon="bolt"
            disabled={dp?.state === 'running'}
            onclick={() => pingOne(detailModel.id)}
          >
            {dp?.state === 'running' ? 'Pinging…' : 'Ping Model Sekarang'}
          </Button>
        </div>
      </div>
    </div>
  </div>
{/if}

<!-- Model Parameters Override Modal -->
<ModelParametersModal
  bind:isOpen={showParamsModal}
  target={paramsModalTarget}
  targetType="model"
  onClose={() => { showParamsModal = false; paramsModalTarget = '' }}
/>
