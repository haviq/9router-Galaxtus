<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import {
    api,
    getAuthHeaders,
    normalizeLastError,
    type ProviderConnection,
    type ProviderNode,
  } from '../api/client'
  import Card from '../lib/ui/Card.svelte'
  import Button from '../lib/ui/Button.svelte'
  import Badge from '../lib/ui/Badge.svelte'
  import Modal from '../lib/ui/Modal.svelte'
  import { copyToClipboard } from '../lib/clipboard'
  import {
    fmt,
    timeAgo,
    formatDuration,
    formatLocalTimestamp,
    providerDisplayName,
    type Period,
    type RequestDetailItem,
    type RecentRequestItem,
    type ActiveRequestItem,
  } from './analytics/types'

  interface Props {
    connections?: ProviderConnection[]
    providerNodes?: ProviderNode[]
  }

  let { connections = [], providerNodes = [] }: Props = $props()

  // Layout mode: split (both stream + table), table, or stream
  type ViewMode = 'split' | 'table' | 'stream'
  let viewMode = $state<ViewMode>('split')

  // Filter mode for table and stream: success (default as requested), all, or error
  type FilterStatus = 'success' | 'all' | 'error'
  let filterStatus = $state<FilterStatus>('success')

  // Table state
  let period = $state<Period>('today')
  let searchQuery = $state('')
  let details = $state<RequestDetailItem[]>([])
  let detailsTotal = $state(0)
  let detailsPage = $state(1)
  let detailsLoading = $state(false)
  let detailsError = $state('')
  const limit = 20

  // Real-time Stream State
  interface LiveLogEntry {
    id: string
    timestamp: string
    timeStr: string
    status: 'success' | 'error'
    statusCode: number
    model: string
    provider: string
    latencyMs?: number
    promptTokens?: number
    completionTokens?: number
    cachedTokens?: number
    errorMessage?: string
  }

  let liveLogs = $state<LiveLogEntry[]>([])
  let activeRequests = $state<ActiveRequestItem[]>([])
  let streamConnected = $state(false)
  let streamPaused = $state(false)
  let autoScroll = $state(true)
  let terminalBoxEl: HTMLDivElement | null = $state(null)
  let autoRefresh = $state(true)

  // Payload modal state
  let selectedDetail = $state<RequestDetailItem | null>(null)
  let detailLoading = $state(false)
  let detailError = $state('')
  let modalTab = $state<'messages' | 'response' | 'raw'>('messages')
  let copiedJson = $state(false)

  // SSE Controller
  let streamController: AbortController | null = null
  let refreshTimer: ReturnType<typeof setInterval> | null = null

  // Counters
  let successCount = $derived(
    details.filter((d) => d.status === 'success' || d.status === 'ok').length
  )
  let errorCount = $derived(
    details.filter((d) => d.status === 'error' || (typeof d.status === 'string' && d.status.startsWith('5'))).length
  )

  // Filtered details
  let filteredDetails = $derived.by(() => {
    let list = details
    if (filterStatus === 'success') {
      list = list.filter((d) => d.status === 'success' || d.status === 'ok')
    } else if (filterStatus === 'error') {
      list = list.filter((d) => d.status !== 'success' && d.status !== 'ok')
    }

    if (!searchQuery.trim()) return list
    const q = searchQuery.toLowerCase().trim()
    return list.filter((d) => {
      const model = (d.requestedModel || d.model || '').toLowerCase()
      const prov = (d.provider || '').toLowerCase()
      const acc = (d.account || d.connectionId || '').toLowerCase()
      const id = (d.id || '').toLowerCase()
      return model.includes(q) || prov.includes(q) || acc.includes(q) || id.includes(q)
    })
  })

  // Filtered live stream logs
  let filteredLiveLogs = $derived.by(() => {
    if (filterStatus === 'success') {
      return liveLogs.filter((l) => l.status === 'success')
    }
    if (filterStatus === 'error') {
      return liveLogs.filter((l) => l.status === 'error')
    }
    return liveLogs
  })

  async function loadDetails(page = 1) {
    detailsLoading = true
    detailsError = ''
    try {
      const offset = (page - 1) * limit
      const res = await api.getRequestDetails(limit, offset, period)
      if (res && Array.isArray(res.details)) {
        details = res.details
        detailsTotal = res.total || 0
        detailsPage = page
      }
    } catch (err) {
      detailsError = normalizeLastError(err) || 'Failed to load request connection logs.'
    } finally {
      detailsLoading = false
    }
  }

  async function openInspect(item: RequestDetailItem) {
    selectedDetail = item
    modalTab = 'messages'
    detailError = ''
    copiedJson = false
    if (!item.id) return

    detailLoading = true
    try {
      const res = await api.getRequestDetail(item.id)
      if (selectedDetail?.id === item.id && res?.detail) {
        selectedDetail = { ...selectedDetail, ...(res.detail as RequestDetailItem) }
      }
    } catch (err) {
      detailError = normalizeLastError(err) || 'Failed to load full request payload.'
    } finally {
      detailLoading = false
    }
  }

  function handleCopyJson() {
    if (!selectedDetail) return
    copyToClipboard(JSON.stringify(selectedDetail, null, 2))
    copiedJson = true
    setTimeout(() => {
      copiedJson = false
    }, 2000)
  }

  function connectSseStream() {
    if (streamPaused) return
    streamController?.abort()
    streamController = new AbortController()

    const token = typeof localStorage !== 'undefined' ? localStorage.getItem('9router_key') || '' : ''
    const streamUrl = token ? `/api/usage/stream?key=${encodeURIComponent(token)}` : '/api/usage/stream'

    fetch(streamUrl, {
      headers: getAuthHeaders(),
      signal: streamController.signal,
    })
      .then(async (res) => {
        if (!res.ok) {
          streamConnected = false
          return
        }
        streamConnected = true
        const reader = res.body?.getReader()
        if (!reader) return
        const decoder = new TextDecoder()
        let buffer = ''

        while (true) {
          const { done, value } = await reader.read()
          if (done) break
          buffer += decoder.decode(value, { stream: true })
          const lines = buffer.split('\n\n')
          buffer = lines.pop() || ''

          for (const line of lines) {
            if (!line.startsWith('data: ')) continue
            try {
              const rawData = JSON.parse(line.slice(6))
              handleStreamUpdate(rawData)
            } catch {
              // ignore malformed frame
            }
          }
        }
      })
      .catch((err) => {
        if (err.name !== 'AbortError') {
          streamConnected = false
        }
      })
  }

  function handleStreamUpdate(data: {
    activeRequests?: ActiveRequestItem[]
    recentRequests?: RecentRequestItem[]
  }) {
    if (Array.isArray(data.activeRequests)) {
      activeRequests = data.activeRequests
    }
    if (Array.isArray(data.recentRequests) && data.recentRequests.length > 0) {
      for (const req of data.recentRequests) {
        const isSuccess = req.status === 'ok' || req.status === 'success' || !req.status
        const d = req.timestamp ? new Date(req.timestamp) : new Date()
        const timeStr = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
        const logId = `${req.timestamp || Date.now()}-${req.model}-${req.provider}`

        if (!liveLogs.some((l) => l.id === logId)) {
          const entry: LiveLogEntry = {
            id: logId,
            timestamp: req.timestamp || new Date().toISOString(),
            timeStr,
            status: isSuccess ? 'success' : 'error',
            statusCode: isSuccess ? 200 : 500,
            model: req.requestedModel || req.model || 'unknown-model',
            provider: req.provider || 'unknown',
            latencyMs: req.latencyMs,
            promptTokens: req.promptTokens,
            completionTokens: req.completionTokens,
            cachedTokens: req.cachedTokens,
          }
          liveLogs = [entry, ...liveLogs.slice(0, 99)]
        }
      }

      if (autoScroll && terminalBoxEl) {
        terminalBoxEl.scrollTop = 0
      }
    }
  }

  function togglePauseStream() {
    streamPaused = !streamPaused
    if (streamPaused) {
      streamController?.abort()
      streamConnected = false
    } else {
      connectSseStream()
    }
  }

  function clearLiveFeed() {
    liveLogs = []
  }

  onMount(() => {
    loadDetails(1)
    connectSseStream()
    refreshTimer = setInterval(() => {
      if (autoRefresh) {
        loadDetails(detailsPage)
      }
    }, 5000)
  })

  onDestroy(() => {
    streamController?.abort()
    if (refreshTimer) clearInterval(refreshTimer)
  })
</script>

<div class="space-y-6">
  <!-- Header Bar -->
  <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
    <div>
      <h1 class="text-xl font-bold flex items-center gap-2 text-text-main">
        <span class="material-symbols-outlined text-primary text-2xl">receipt_long</span>
        Connection Logs
      </h1>
      <p class="text-xs text-text-muted mt-0.5">
        Real-time connection telemetry, live traffic feed, and detailed payload inspector.
      </p>
    </div>

    <!-- Top Action Toolbar -->
    <div class="flex flex-wrap items-center gap-2">
      <!-- Layout Switcher -->
      <div class="inline-flex rounded-lg bg-surface-2 p-1 border border-border">
        <button
          type="button"
          onclick={() => (viewMode = 'split')}
          class="px-2.5 py-1 rounded text-xs font-medium transition-colors cursor-pointer flex items-center gap-1 {viewMode === 'split' ? 'bg-primary text-white shadow-sm' : 'text-text-muted hover:text-text-main'}"
          title="Split View (Live Feed + Table)"
        >
          <span class="material-symbols-outlined text-sm">view_agenda</span>
          <span class="hidden sm:inline">Split</span>
        </button>
        <button
          type="button"
          onclick={() => (viewMode = 'table')}
          class="px-2.5 py-1 rounded text-xs font-medium transition-colors cursor-pointer flex items-center gap-1 {viewMode === 'table' ? 'bg-primary text-white shadow-sm' : 'text-text-muted hover:text-text-main'}"
          title="Table View Only"
        >
          <span class="material-symbols-outlined text-sm">table_rows</span>
          <span class="hidden sm:inline">Table</span>
        </button>
        <button
          type="button"
          onclick={() => (viewMode = 'stream')}
          class="px-2.5 py-1 rounded text-xs font-medium transition-colors cursor-pointer flex items-center gap-1 {viewMode === 'stream' ? 'bg-primary text-white shadow-sm' : 'text-text-muted hover:text-text-main'}"
          title="Live Terminal Feed Only"
        >
          <span class="material-symbols-outlined text-sm">terminal</span>
          <span class="hidden sm:inline">Live Feed</span>
        </button>
      </div>

      <!-- Auto Refresh Button -->
      <Button
        variant="ghost"
        size="sm"
        onclick={() => loadDetails(detailsPage)}
        disabled={detailsLoading}
        title="Refresh logs"
      >
        <span class="material-symbols-outlined text-sm {detailsLoading ? 'animate-spin' : ''}">refresh</span>
        <span class="hidden sm:inline">Refresh</span>
      </Button>
    </div>
  </div>

  <!-- KPI / Status Counter Cards -->
  <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
    <!-- Successful Connections (Highlight) -->
    <div class="p-3.5 rounded-xl bg-surface-1 border border-success/30 shadow-sm relative overflow-hidden">
      <div class="flex items-center justify-between">
        <span class="text-xs font-medium text-text-muted">Success (200 OK)</span>
        <span class="material-symbols-outlined text-success text-base">check_circle</span>
      </div>
      <div class="text-2xl font-bold font-mono text-success mt-1">
        {successCount}
      </div>
      <div class="text-[10px] text-text-subtle mt-0.5">Verified active completions</div>
    </div>

    <!-- Error Connections -->
    <div class="p-3.5 rounded-xl bg-surface-1 border border-border shadow-sm">
      <div class="flex items-center justify-between">
        <span class="text-xs font-medium text-text-muted">Failed / Errors</span>
        <span class="material-symbols-outlined text-red-400 text-base">error</span>
      </div>
      <div class="text-2xl font-bold font-mono text-red-400 mt-1">
        {errorCount}
      </div>
      <div class="text-[10px] text-text-subtle mt-0.5">Upstream 4xx/5xx failures</div>
    </div>

    <!-- In-Flight Active Requests -->
    <div class="p-3.5 rounded-xl bg-surface-1 border border-border shadow-sm">
      <div class="flex items-center justify-between">
        <span class="text-xs font-medium text-text-muted">Active In-Flight</span>
        <span class="relative flex h-2 w-2">
          <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-cyan-400 opacity-75"></span>
          <span class="relative inline-flex rounded-full h-2 w-2 bg-cyan-500"></span>
        </span>
      </div>
      <div class="text-2xl font-bold font-mono text-cyan-400 mt-1">
        {activeRequests.reduce((acc, curr) => acc + (curr.count || 1), 0)}
      </div>
      <div class="text-[10px] text-text-subtle mt-0.5">Current concurrent tunnels</div>
    </div>

    <!-- Live Stream Heartbeat -->
    <div class="p-3.5 rounded-xl bg-surface-1 border border-border shadow-sm">
      <div class="flex items-center justify-between">
        <span class="text-xs font-medium text-text-muted">Live Stream</span>
        <span class="material-symbols-outlined text-base {streamConnected ? 'text-emerald-400' : 'text-amber-400'}">
          {streamConnected ? 'sensors' : 'sensors_off'}
        </span>
      </div>
      <div class="text-xs font-semibold mt-2.5 flex items-center gap-1.5 {streamConnected ? 'text-emerald-400' : 'text-amber-400'}">
        <span class="w-2 h-2 rounded-full {streamConnected ? 'bg-emerald-400 animate-pulse' : 'bg-amber-400'}"></span>
        {streamConnected ? 'SSE LISTENING' : streamPaused ? 'PAUSED' : 'CONNECTING...'}
      </div>
      <div class="text-[10px] text-text-subtle mt-1 font-mono">/api/usage/stream</div>
    </div>
  </div>

  <!-- ═════════ SECTION 1: LIVE CONNECTION STREAM (TERMINAL FEED) ═════════ -->
  {#if viewMode === 'split' || viewMode === 'stream'}
    <Card padding="md" class="border-border/80 shadow-md">
      <div class="flex flex-wrap items-center justify-between gap-2 pb-3 border-b border-border">
        <div class="flex items-center gap-2">
          <span class="material-symbols-outlined text-emerald-400 text-lg">terminal</span>
          <h2 class="text-sm font-bold text-text-main flex items-center gap-2">
            Live Connection Feed
            <span class="px-1.5 py-0.2 rounded text-[10px] font-mono bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
              REAL-TIME
            </span>
          </h2>
        </div>

        <!-- Terminal Controls -->
        <div class="flex items-center gap-2">
          <label class="text-[11px] text-text-muted flex items-center gap-1 cursor-pointer select-none">
            <input type="checkbox" bind:checked={autoScroll} class="rounded accent-primary h-3.5 w-3.5" />
            <span>Auto-scroll</span>
          </label>

          <button
            type="button"
            onclick={togglePauseStream}
            class="px-2 py-1 rounded text-[11px] border border-border bg-surface-2 hover:bg-surface-3 text-text-muted hover:text-text-main transition-colors cursor-pointer flex items-center gap-1"
          >
            <span class="material-symbols-outlined text-xs">{streamPaused ? 'play_arrow' : 'pause'}</span>
            <span>{streamPaused ? 'Resume' : 'Pause'}</span>
          </button>

          <button
            type="button"
            onclick={clearLiveFeed}
            class="px-2 py-1 rounded text-[11px] border border-border bg-surface-2 hover:bg-surface-3 text-text-muted hover:text-text-main transition-colors cursor-pointer flex items-center gap-1"
          >
            <span class="material-symbols-outlined text-xs">delete</span>
            <span>Clear</span>
          </button>
        </div>
      </div>

      <!-- Terminal Output Window -->
      <div
        bind:this={terminalBoxEl}
        class="mt-3 p-3.5 rounded-xl bg-black/85 border border-border/60 font-mono text-xs max-h-56 overflow-y-auto space-y-1.5 custom-scrollbar"
      >
        {#if filteredLiveLogs.length === 0}
          <div class="text-text-muted/60 text-xs py-4 text-center">
            [Waiting for new client connections... Send a request to see live logs here]
          </div>
        {:else}
          {#each filteredLiveLogs as log (log.id)}
            <div class="flex items-start gap-2.5 leading-relaxed hover:bg-white/5 px-1 py-0.5 rounded transition-colors">
              <span class="text-text-muted text-[11px] shrink-0">[{log.timeStr}]</span>

              <!-- Status Pill -->
              {#if log.status === 'success'}
                <span class="px-1.5 py-0.2 rounded text-[10px] font-bold bg-emerald-500/20 text-emerald-400 border border-emerald-500/30 shrink-0">
                  200 OK
                </span>
              {:else}
                <span class="px-1.5 py-0.2 rounded text-[10px] font-bold bg-red-500/20 text-red-400 border border-red-500/30 shrink-0">
                  {log.statusCode || 'ERR'}
                </span>
              {/if}

              <!-- Content Payload -->
              <div class="flex-1 min-w-0 flex flex-wrap items-center gap-x-2 text-[11px]">
                <span class="text-cyan-300 font-semibold">{log.model}</span>
                <span class="text-text-muted text-[10px]">via</span>
                <span class="text-purple-300">{providerDisplayName(log.provider, providerNodes)}</span>

                {#if log.latencyMs != null}
                  <span class="text-amber-300 font-medium">({log.latencyMs}ms)</span>
                {/if}

                {#if log.promptTokens || log.completionTokens}
                  <span class="text-text-muted text-[10px]">
                    <span class="text-blue-400">{log.promptTokens || 0}↑</span>
                    <span class="text-emerald-400">{log.completionTokens || 0}↓</span>
                    {#if log.cachedTokens}
                      <span class="text-cyan-400 font-bold">({log.cachedTokens} cached)</span>
                    {/if}
                  </span>
                {/if}
              </div>
            </div>
          {/each}
        {/if}
      </div>
    </Card>
  {/if}

  <!-- ═════════ SECTION 2: STRUCTURED CONNECTION LOGS TABLE ═════════ -->
  {#if viewMode === 'split' || viewMode === 'table'}
    <Card padding="md" class="border-border/80 shadow-md space-y-4">
      <!-- Table Filters & Search Bar -->
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <!-- Status Filter Tabs -->
        <div class="inline-flex rounded-lg bg-surface-2 p-1 border border-border">
          <button
            type="button"
            onclick={() => (filterStatus = 'success')}
            class="px-3 py-1 rounded text-xs font-medium transition-colors cursor-pointer flex items-center gap-1.5 {filterStatus === 'success' ? 'bg-success text-white shadow-sm' : 'text-text-muted hover:text-text-main'}"
          >
            <span class="material-symbols-outlined text-sm">check_circle</span>
            <span>Success (200 OK)</span>
          </button>
          <button
            type="button"
            onclick={() => (filterStatus = 'all')}
            class="px-3 py-1 rounded text-xs font-medium transition-colors cursor-pointer flex items-center gap-1.5 {filterStatus === 'all' ? 'bg-primary text-white shadow-sm' : 'text-text-muted hover:text-text-main'}"
          >
            <span class="material-symbols-outlined text-sm">list</span>
            <span>All Requests</span>
          </button>
          <button
            type="button"
            onclick={() => (filterStatus = 'error')}
            class="px-3 py-1 rounded text-xs font-medium transition-colors cursor-pointer flex items-center gap-1.5 {filterStatus === 'error' ? 'bg-red-500 text-white shadow-sm' : 'text-text-muted hover:text-text-main'}"
          >
            <span class="material-symbols-outlined text-sm">error</span>
            <span>Errors</span>
          </button>
        </div>

        <!-- Search Box & Period Selector -->
        <div class="flex items-center gap-2 flex-wrap">
          <div class="relative min-w-[200px]">
            <span class="material-symbols-outlined text-sm text-text-muted absolute left-2.5 top-1/2 -translate-y-1/2">
              search
            </span>
            <input
              type="text"
              bind:value={searchQuery}
              placeholder="Search model, provider..."
              class="w-full pl-8 pr-3 py-1.5 text-xs rounded-lg bg-surface-2 border border-border text-text-main outline-none focus:border-primary transition-all"
            />
          </div>

          <select
            bind:value={period}
            onchange={() => loadDetails(1)}
            class="px-2.5 py-1.5 text-xs rounded-lg bg-surface-2 border border-border text-text-main outline-none cursor-pointer"
          >
            <option value="today">Today</option>
            <option value="24h">Last 24 Hours</option>
            <option value="7d">Last 7 Days</option>
            <option value="all">All Time</option>
          </select>
        </div>
      </div>

      <!-- Table Container -->
      <div class="overflow-x-auto rounded-xl border border-border/80 bg-surface-1">
        <table class="w-full text-left border-collapse text-xs">
          <thead>
            <tr class="border-b border-border bg-surface-2/60 text-text-muted font-medium">
              <th class="py-2.5 px-3">Status</th>
              <th class="py-2.5 px-3">Time</th>
              <th class="py-2.5 px-3">Model</th>
              <th class="py-2.5 px-3">Provider</th>
              <th class="py-2.5 px-3">Latency</th>
              <th class="py-2.5 px-3">Tokens (In / Out)</th>
              <th class="py-2.5 px-3">Account / Key</th>
              <th class="py-2.5 px-3 text-right">Action</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border/50">
            {#if detailsLoading && filteredDetails.length === 0}
              <tr>
                <td colspan="8" class="py-8 text-center text-text-muted">
                  <span class="material-symbols-outlined text-lg animate-spin align-middle mr-1">progress_activity</span>
                  Loading connection logs...
                </td>
              </tr>
            {:else if filteredDetails.length === 0}
              <tr>
                <td colspan="8" class="py-8 text-center text-text-muted">
                  No connection records matching your filter.
                </td>
              </tr>
            {:else}
              {#each filteredDetails as item (item.id || item.timestamp)}
                {@const isSuccess = item.status === 'success' || item.status === 'ok'}
                <tr class="hover:bg-surface-2/40 transition-colors">
                  <!-- Status -->
                  <td class="py-2.5 px-3 whitespace-nowrap">
                    {#if isSuccess}
                      <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[11px] font-semibold bg-emerald-500/15 text-emerald-400 border border-emerald-500/30">
                        <span class="material-symbols-outlined text-xs">check_circle</span>
                        200 OK
                      </span>
                    {:else}
                      <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[11px] font-semibold bg-red-500/15 text-red-400 border border-red-500/30">
                        <span class="material-symbols-outlined text-xs">error</span>
                        {item.status || 'Error'}
                      </span>
                    {/if}
                  </td>

                  <!-- Time -->
                  <td class="py-2.5 px-3 whitespace-nowrap text-text-muted font-mono text-[11px]">
                    <span title={formatLocalTimestamp(item.timestamp)}>
                      {timeAgo(item.timestamp)}
                    </span>
                  </td>

                  <!-- Model -->
                  <td class="py-2.5 px-3 max-w-[180px] truncate font-medium text-text-main" title={item.requestedModel || item.model}>
                    {item.requestedModel || item.model}
                  </td>

                  <!-- Provider -->
                  <td class="py-2.5 px-3 whitespace-nowrap">
                    <span class="px-2 py-0.5 rounded-md bg-surface-2 border border-border text-[11px] text-text-muted">
                      {providerDisplayName(item.provider, providerNodes)}
                    </span>
                  </td>

                  <!-- Latency -->
                  <td class="py-2.5 px-3 whitespace-nowrap font-mono text-[11px]">
                    <span class="text-text-main font-semibold">
                      {formatDuration(item.latency?.total)}
                    </span>
                    {#if item.latency?.ttft}
                      <span class="text-text-muted text-[10px]"> (ttft {item.latency.ttft}ms)</span>
                    {/if}
                  </td>

                  <!-- Tokens -->
                  <td class="py-2.5 px-3 whitespace-nowrap font-mono text-[11px]">
                    <span class="text-primary">{fmt(item.tokens?.prompt_tokens)}↑</span>
                    <span class="text-success ml-1">{fmt(item.tokens?.completion_tokens)}↓</span>
                    {#if item.tokens?.cached_tokens}
                      <span class="text-cyan-400 font-bold ml-1">({fmt(item.tokens.cached_tokens)}c)</span>
                    {/if}
                  </td>

                  <!-- Account -->
                  <td class="py-2.5 px-3 max-w-[130px] truncate text-text-muted text-[11px]" title={item.account || item.connectionId}>
                    {item.account || item.connectionId || '—'}
                  </td>

                  <!-- Action -->
                  <td class="py-2.5 px-3 text-right whitespace-nowrap">
                    <button
                      type="button"
                      onclick={() => openInspect(item)}
                      class="px-2.5 py-1 rounded text-xs font-medium bg-surface-2 hover:bg-surface-3 border border-border text-text-main transition-colors cursor-pointer inline-flex items-center gap-1"
                    >
                      <span class="material-symbols-outlined text-xs">visibility</span>
                      Inspect
                    </button>
                  </td>
                </tr>
              {/each}
            {/if}
          </tbody>
        </table>
      </div>

      <!-- Pagination Toolbar -->
      <div class="flex items-center justify-between pt-2 text-xs text-text-muted">
        <span>Page {detailsPage} of {Math.max(1, Math.ceil(detailsTotal / limit))} ({detailsTotal} total records)</span>

        <div class="flex items-center gap-1.5">
          <Button
            size="sm"
            variant="ghost"
            disabled={detailsPage <= 1 || detailsLoading}
            onclick={() => loadDetails(detailsPage - 1)}
          >
            Previous
          </Button>
          <Button
            size="sm"
            variant="ghost"
            disabled={detailsPage * limit >= detailsTotal || detailsLoading}
            onclick={() => loadDetails(detailsPage + 1)}
          >
            Next
          </Button>
        </div>
      </div>
    </Card>
  {/if}
</div>

<!-- ═════════ MODAL: PAYLOAD & CONNECTION INSPECTOR ═════════ -->
{#if selectedDetail}
  <Modal
    isOpen={!!selectedDetail}
    onClose={() => (selectedDetail = null)}
    title="Connection Request Inspector"
    size="xl"
    icon="terminal"
  >
    <div class="space-y-4">
      <!-- Overview Banner -->
      <div class="p-3.5 rounded-xl bg-surface-2/60 border border-border grid grid-cols-2 sm:grid-cols-4 gap-3 text-xs">
        <div>
          <span class="text-text-muted text-[10px] uppercase font-bold">Status</span>
          <div class="mt-0.5 font-bold {selectedDetail.status === 'success' || selectedDetail.status === 'ok' ? 'text-success' : 'text-red-400'}">
            {selectedDetail.status === 'success' || selectedDetail.status === 'ok' ? '200 OK' : selectedDetail.status}
          </div>
        </div>
        <div>
          <span class="text-text-muted text-[10px] uppercase font-bold">Model</span>
          <div class="mt-0.5 font-medium text-text-main truncate" title={selectedDetail.model}>
            {selectedDetail.model}
          </div>
        </div>
        <div>
          <span class="text-text-muted text-[10px] uppercase font-bold">Total Duration</span>
          <div class="mt-0.5 font-mono text-text-main">
            {formatDuration(selectedDetail.latency?.total)}
          </div>
        </div>
        <div>
          <span class="text-text-muted text-[10px] uppercase font-bold">Tokens</span>
          <div class="mt-0.5 font-mono text-text-main">
            {fmt(selectedDetail.tokens?.prompt_tokens)} in / {fmt(selectedDetail.tokens?.completion_tokens)} out
          </div>
        </div>
      </div>

      <!-- Modal Sub-Tabs -->
      <div class="flex items-center justify-between border-b border-border pb-1">
        <div class="inline-flex gap-2 text-xs">
          <button
            type="button"
            onclick={() => (modalTab = 'messages')}
            class="px-3 py-1 font-medium transition-colors border-b-2 cursor-pointer {modalTab === 'messages' ? 'border-primary text-primary font-bold' : 'border-transparent text-text-muted hover:text-text-main'}"
          >
            Prompt Messages
          </button>
          <button
            type="button"
            onclick={() => (modalTab = 'response')}
            class="px-3 py-1 font-medium transition-colors border-b-2 cursor-pointer {modalTab === 'response' ? 'border-primary text-primary font-bold' : 'border-transparent text-text-muted hover:text-text-main'}"
          >
            Response Output
          </button>
          <button
            type="button"
            onclick={() => (modalTab = 'raw')}
            class="px-3 py-1 font-medium transition-colors border-b-2 cursor-pointer {modalTab === 'raw' ? 'border-primary text-primary font-bold' : 'border-transparent text-text-muted hover:text-text-main'}"
          >
            Raw JSON
          </button>
        </div>

        <button
          type="button"
          onclick={handleCopyJson}
          class="px-2.5 py-1 rounded text-xs border border-border bg-surface-2 hover:bg-surface-3 text-text-muted hover:text-text-main transition-colors cursor-pointer flex items-center gap-1"
        >
          <span class="material-symbols-outlined text-xs">content_copy</span>
          <span>{copiedJson ? 'Copied!' : 'Copy JSON'}</span>
        </button>
      </div>

      <!-- Tab 1: Messages List -->
      {#if modalTab === 'messages'}
        <div class="max-h-96 overflow-y-auto space-y-2.5 custom-scrollbar pr-1">
          {#if detailLoading}
            <div class="py-8 text-center text-text-muted text-xs">
              <span class="material-symbols-outlined text-sm animate-spin align-middle mr-1">progress_activity</span>
              Loading full prompt messages...
            </div>
          {:else if selectedDetail.request?.messages && Array.isArray(selectedDetail.request.messages)}
            {#each selectedDetail.request.messages as msg, idx}
              <div class="p-3 rounded-xl bg-surface-2 border border-border space-y-1">
                <div class="flex items-center justify-between text-[11px] font-bold text-text-muted uppercase">
                  <span class="px-2 py-0.5 rounded font-mono {msg.role === 'system' ? 'bg-amber-500/20 text-amber-300' : msg.role === 'user' ? 'bg-blue-500/20 text-blue-300' : 'bg-emerald-500/20 text-emerald-300'}">
                    {msg.role || 'message'}
                  </span>
                  <span class="text-[10px] text-text-subtle font-mono">#{idx + 1}</span>
                </div>
                <pre class="text-xs font-mono whitespace-pre-wrap break-words text-text-main pt-1">{typeof msg.content === 'string' ? msg.content : JSON.stringify(msg.content, null, 2)}</pre>
              </div>
            {/each}
          {:else}
            <div class="py-6 text-center text-text-muted text-xs">
              No message contents stored for this row.
            </div>
          {/if}
        </div>

      <!-- Tab 2: Response Output -->
      {:else if modalTab === 'response'}
        <div class="max-h-96 overflow-y-auto custom-scrollbar p-3.5 rounded-xl bg-black/60 border border-border font-mono text-xs">
          {#if selectedDetail.response?.content}
            <pre class="text-text-main whitespace-pre-wrap break-words">{selectedDetail.response.content}</pre>
          {:else if selectedDetail.response?.error}
            <pre class="text-red-400 whitespace-pre-wrap break-words">{selectedDetail.response.error}</pre>
          {:else}
            <span class="text-text-muted">No response output body stored.</span>
          {/if}
        </div>

      <!-- Tab 3: Raw JSON -->
      {:else if modalTab === 'raw'}
        <div class="max-h-96 overflow-y-auto custom-scrollbar p-3.5 rounded-xl bg-black/60 border border-border font-mono text-xs">
          <pre class="text-text-main whitespace-pre-wrap break-words">{JSON.stringify(selectedDetail, null, 2)}</pre>
        </div>
      {/if}
    </div>
  </Modal>
{/if}

