<script lang="ts">
  import { onMount } from 'svelte'
  import Badge from '../../lib/ui/Badge.svelte'
  import Card from '../../lib/ui/Card.svelte'
  import ProviderArtwork from '../providers/ProviderArtwork.svelte'
  import {
    fmt,
    timeAgo,
    providerDisplayName,
    providerRoutingPrefix,
    TABLE_OPTIONS,
    type StatsData,
    type TableView,
    type ViewMode,
    type UsageItem
  } from './types'
  import { costIn } from '../../lib/currency.svelte'
  import { api, type ProviderConnection } from '../../api/client'
  import { getQuotaCache, setQuotaCache, parseQuotaData, type QuotaEntry } from '../quota/types'
  import { copyToClipboard } from '../../lib/clipboard'

  let { stats = {} }: { stats?: StatsData } = $props()

  let tableView = $state<TableView>('model')
  let viewMode = $state<ViewMode>('costs')
  let connections = $state<ProviderConnection[]>([])
  let quotaCache = $state<Record<string, QuotaEntry>>({})
  let balanceLoading = $state<Record<string, boolean>>({})
  let copiedPrefix = $state<string | null>(null)

  function copyPrefix(prefix: string) {
    copyToClipboard(prefix)
    copiedPrefix = prefix
    setTimeout(() => {
      if (copiedPrefix === prefix) copiedPrefix = null
    }, 1500)
  }

  onMount(async () => {
    quotaCache = getQuotaCache()
    try {
      const res = await api.getProvidersClient()
      if (res && Array.isArray(res.connections)) {
        connections = res.connections
      }
    } catch {
      // quiet fallback
    }
  })

  async function refreshProviderBalance(connId: string) {
    if (balanceLoading[connId]) return
    balanceLoading = { ...balanceLoading, [connId]: true }
    try {
      const res = await api.getConnectionUsage(connId, true)
      const conn = connections.find((c) => c.id === connId)
      const prov = conn?.provider || ''
      const entry: QuotaEntry = {
        quotas: parseQuotaData(prov, res),
        plan: res?.plan || null,
        message: res?.message || null,
        raw: res,
      }
      setQuotaCache(connId, entry)
      quotaCache = getQuotaCache()
    } catch (e) {
      console.error('Failed to refresh provider balance:', e)
    } finally {
      balanceLoading = { ...balanceLoading, [connId]: false }
    }
  }

  function getProviderBalance(providerId: string) {
    const prov = (providerId || '').toLowerCase().trim()
    const conn = connections.find((c) => (c.provider || '').toLowerCase() === prov)
    if (!conn) {
      return { hasData: false, isCreditBalance: false, balanceText: '', detailText: '', color: '', connectionId: undefined, loading: false }
    }
    const connId = conn.id
    const isLoading = !!balanceLoading[connId]
    const cached = quotaCache[connId]
    if (!cached || !cached.quotas || cached.quotas.length === 0) {
      return { hasData: false, isCreditBalance: false, balanceText: '', detailText: '', color: '', connectionId: connId, loading: isLoading }
    }

    const creditQ = cached.quotas.find((q) => q.isCreditBalance || (q.name && q.name.toLowerCase().includes('balance')))
    if (creditQ) {
      const cur = creditQ.currency || 'USD'
      const tot = typeof creditQ.total === 'number' ? creditQ.total : 0
      const isPositive = tot > 0
      return {
        hasData: true,
        isCreditBalance: true,
        balanceText: `$${tot.toFixed(2)} ${cur}`,
        detailText: 'Sisa Saldo',
        color: isPositive ? 'text-blue-400 font-bold' : 'text-red-400 font-bold',
        connectionId: connId,
        loading: isLoading,
      }
    }

    const firstQ = cached.quotas[0]
    if (firstQ.unlimited) {
      return {
        hasData: true,
        isCreditBalance: false,
        balanceText: 'Unlimited',
        detailText: `${firstQ.used ?? 0} used`,
        color: 'text-green-400 font-bold',
        connectionId: connId,
        loading: isLoading,
      }
    }

    const pct = firstQ.remainingPercentage ?? (firstQ.total > 0 ? Math.round(((firstQ.total - firstQ.used) / firstQ.total) * 100) : null)
    if (pct != null) {
      return {
        hasData: true,
        isCreditBalance: false,
        balanceText: `${pct}%`,
        detailText: `${firstQ.used ?? 0} / ${firstQ.total ?? '∞'} used`,
        color: pct > 30 ? 'text-green-400 font-bold' : 'text-amber-400 font-bold',
        connectionId: connId,
        loading: isLoading,
      }
    }

    return {
      hasData: true,
      isCreditBalance: false,
      balanceText: firstQ.name || 'Active',
      detailText: '',
      color: 'text-text-main',
      connectionId: connId,
      loading: isLoading,
    }
  }

  // Every column of the breakdown table is sortable: click a header for that
  // column, click it again to reverse. The default (requests, descending) is
  // what the table showed before the headers became clickable, so nothing about
  // the initial view changed.
  type SortKey =
    | 'name'
    | 'provider'
    | 'requests'
    | 'cost'
    | 'promptTokens'
    | 'completionTokens'
    | 'cachedTokens'
    | 'lastUsed'

  let sortKey = $state<SortKey>('requests')
  let sortDir = $state<'asc' | 'desc'>('desc')

  function toggleSort(key: SortKey) {
    if (sortKey === key) {
      sortDir = sortDir === 'asc' ? 'desc' : 'asc'
      return
    }
    sortKey = key
    // Text columns read best A→Z first; every numeric/timestamp column is more
    // useful biggest/newest first, which is also the unsorted default.
    sortDir = key === 'name' || key === 'provider' ? 'asc' : 'desc'
  }

  interface ProcessedUsageRow extends UsageItem {
    key: string
    /** The name this row is rendered under, and what the Name column sorts on. */
    label: string
    totalTokens: number
  }

  function rowLabel(tableView: TableView, key: string, item: UsageItem): string {
    switch (tableView) {
      case 'account':
        return item.accountName || key
      case 'apiKey':
        return item.keyName || key
      case 'endpoint':
        return item.endpoint || key
      case 'provider':
        // byProvider is keyed by the provider id; the dashboard shows its
        // display name, so the column and the sort agree with what is drawn.
        return providerDisplayName(key)
      default:
        return item.rawModel || key
    }
  }

  let tableData = $derived((): ProcessedUsageRow[] => {
    if (!stats) return []
    let sourceMap: Record<string, UsageItem> = {}
    if (tableView === 'model') sourceMap = stats.byModel || {}
    else if (tableView === 'provider') sourceMap = stats.byProvider || {}
    else if (tableView === 'account') sourceMap = stats.byAccount || {}
    else if (tableView === 'apiKey') sourceMap = stats.byApiKey || {}
    else if (tableView === 'endpoint') sourceMap = stats.byEndpoint || {}

    const rows = Object.entries(sourceMap).map(([key, item]) => {
      const totalTokens = (item.promptTokens || 0) + (item.completionTokens || 0)
      const prov = tableView === 'provider' ? item.provider || key : item.provider
      return {
        key,
        ...item,
        // The provider breakdown carries no provider field of its own (the key
        // is the provider), so fill it in: the artwork and badge read it.
        provider: prov,
        prefix: item.prefix || providerRoutingPrefix(prov),
        label: rowLabel(tableView, key, item),
        totalTokens,
      }
    })

    const dir = sortDir === 'asc' ? 1 : -1
    return rows.sort((a, b) => {
      switch (sortKey) {
        case 'name':
          return dir * a.label.localeCompare(b.label)
        case 'provider':
          return dir * (a.provider || '').localeCompare(b.provider || '')
        case 'cost':
          return dir * ((a.cost || 0) - (b.cost || 0))
        case 'promptTokens':
          return dir * ((a.promptTokens || 0) - (b.promptTokens || 0))
        case 'completionTokens':
          return dir * ((a.completionTokens || 0) - (b.completionTokens || 0))
        case 'cachedTokens':
          return dir * ((a.cachedTokens || 0) - (b.cachedTokens || 0))
        case 'lastUsed':
          // Timestamps are stored as ISO strings, so a lexical compare is a
          // chronological one. Rows without one (daily windows) sort last
          // ascending and first descending, never in the middle.
          return dir * (a.lastUsed || '').localeCompare(b.lastUsed || '')
        default:
          return dir * ((a.requests || 0) - (b.requests || 0))
      }
    })
  })

  const nameHeader = $derived(
    tableView === 'model'
      ? 'Model'
      : tableView === 'provider'
        ? 'Provider'
        : tableView === 'account'
          ? 'Account'
          : tableView === 'apiKey'
            ? 'Key Name'
            : 'Endpoint'
  )

  // A header is a button: the arrow marks the active column and its direction,
  // and an inactive column shows a dimmed pair so the affordance is visible.
  function arrow(key: SortKey): string {
    if (sortKey !== key) return '↕'
    return sortDir === 'asc' ? '↑' : '↓'
  }

  function headerClass(key: SortKey, align: 'left' | 'right' = 'left'): string {
    const base =
      'inline-flex w-full items-center gap-1 text-[10px] font-semibold uppercase tracking-wider cursor-pointer select-none hover:text-text-main transition-colors'
    return `${base} ${align === 'right' ? 'justify-end' : 'justify-start'} ${
      sortKey === key ? 'text-text-main' : 'text-text-muted'
    }`
  }
</script>

<div class="flex flex-col gap-3 pt-2">
  <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
    <!-- View selector dropdown -->
    <select
      bind:value={tableView}
      class="w-full sm:w-auto rounded-lg border border-border bg-surface px-3 py-1.5 text-sm font-semibold text-text-main focus:outline-none focus:ring-2 focus:ring-brand-500/50 cursor-pointer"
    >
      {#each TABLE_OPTIONS as opt}
        <option value={opt.value}>{opt.label}</option>
      {/each}
    </select>

    <!-- Toggle: Costs | Tokens -->
    <div class="inline-flex rounded-xl bg-surface border border-border p-1 shadow-sm self-start sm:self-auto">
      <button
        type="button"
        onclick={() => (viewMode = 'costs')}
        class="rounded-lg px-3 py-1 text-xs font-semibold transition-colors cursor-pointer {viewMode === 'costs'
          ? 'bg-brand-500 text-white shadow-sm'
          : 'text-text-muted hover:text-text-main'}"
      >
        Costs
      </button>
      <button
        type="button"
        onclick={() => (viewMode = 'tokens')}
        class="rounded-lg px-3 py-1 text-xs font-semibold transition-colors cursor-pointer {viewMode === 'tokens'
          ? 'bg-brand-500 text-white shadow-sm'
          : 'text-text-muted hover:text-text-main'}"
      >
        Tokens
      </button>
    </div>
  </div>

  <!-- Breakdown Table Card -->
  <Card padding="none" class="overflow-hidden border border-border">
    {#if tableData().length === 0}
      <div class="p-8 text-center text-text-muted text-sm font-body">
        No usage recorded for this period.
      </div>
    {:else}
      <div class="overflow-x-auto">
        <table class="w-full text-left border-collapse text-xs font-body">
          <thead class="bg-surface-2 border-b border-border uppercase font-semibold">
            <tr>
              <th class="py-3 px-4" aria-sort={sortKey === 'name' ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}>
                <button type="button" class={headerClass('name')} onclick={() => toggleSort('name')}>
                  {nameHeader}
                  <span class="text-[9px]">{arrow('name')}</span>
                </button>
              </th>
              {#if tableView === 'provider'}
                <th class="py-3 px-4 text-left">
                  <span class="inline-flex items-center gap-1 text-[10px] font-semibold uppercase tracking-wider text-text-muted">
                    Sisa Saldo / Quota
                  </span>
                </th>
              {/if}
              {#if tableView !== 'provider'}
                <th class="py-3 px-4" aria-sort={sortKey === 'provider' ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}>
                  <button type="button" class={headerClass('provider')} onclick={() => toggleSort('provider')}>
                    Provider
                    <span class="text-[9px]">{arrow('provider')}</span>
                  </button>
                </th>
              {/if}
              <th class="py-3 px-4" aria-sort={sortKey === 'requests' ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}>
                <button type="button" class={headerClass('requests', 'right')} onclick={() => toggleSort('requests')}>
                  Requests
                  <span class="text-[9px]">{arrow('requests')}</span>
                </button>
              </th>
              {#if viewMode === 'costs'}
                <th class="py-3 px-4" aria-sort={sortKey === 'cost' ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}>
                  <button type="button" class={headerClass('cost', 'right')} onclick={() => toggleSort('cost')}>
                    Total Cost
                    <span class="text-[9px]">{arrow('cost')}</span>
                  </button>
                </th>
              {:else}
                <th class="py-3 px-4" aria-sort={sortKey === 'promptTokens' ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}>
                  <button type="button" class={headerClass('promptTokens', 'right')} onclick={() => toggleSort('promptTokens')}>
                    In Tokens
                    <span class="text-[9px]">{arrow('promptTokens')}</span>
                  </button>
                </th>
                <th class="py-3 px-4" aria-sort={sortKey === 'completionTokens' ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}>
                  <button type="button" class={headerClass('completionTokens', 'right')} onclick={() => toggleSort('completionTokens')}>
                    Out Tokens
                    <span class="text-[9px]">{arrow('completionTokens')}</span>
                  </button>
                </th>
                <th class="py-3 px-4" aria-sort={sortKey === 'cachedTokens' ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}>
                  <button type="button" class={headerClass('cachedTokens', 'right')} onclick={() => toggleSort('cachedTokens')}>
                    Cached Tokens
                    <span class="text-[9px]">{arrow('cachedTokens')}</span>
                  </button>
                </th>
              {/if}
              <th class="py-3 px-4" aria-sort={sortKey === 'lastUsed' ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}>
                <button type="button" class={headerClass('lastUsed', 'right')} onclick={() => toggleSort('lastUsed')}>
                  Last Used
                  <span class="text-[9px]">{arrow('lastUsed')}</span>
                </button>
              </th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border/60">
            {#each tableData() as row}
              <tr class="hover:bg-surface-2/60 transition-colors">
                <td class="py-3 px-4 font-mono font-medium text-text-main text-xs">
                  <div class="flex items-center gap-2">
                    {#if row.provider}
                      <ProviderArtwork
                        id={row.provider}
                        alt={row.provider}
                        class="w-4 h-4 object-contain rounded shrink-0 bg-surface-2 p-0.5 border border-border/40 text-[9px] leading-none font-semibold"
                      />
                    {/if}
                    <span class="truncate font-semibold">{row.label}</span>
                    {#if tableView === 'provider'}
                      {@const prefix = row.prefix || providerRoutingPrefix(row.provider || row.key)}
                      {#if prefix}
                        <button
                          type="button"
                          class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-mono font-bold bg-brand-500/15 text-brand-400 border border-brand-500/30 hover:bg-brand-500/25 transition-colors cursor-pointer shadow-xs"
                          title="Routing prefix {prefix}<model> (Click to copy)"
                          onclick={(e) => {
                            e.stopPropagation()
                            copyPrefix(prefix)
                          }}
                        >
                          <span>{prefix}</span>
                          {#if copiedPrefix === prefix}
                            <span class="text-[9px] text-green-400 font-sans font-normal">copied!</span>
                          {/if}
                        </button>
                      {/if}
                    {/if}
                  </div>
                </td>
                {#if tableView === 'provider'}
                  {@const bal = getProviderBalance(row.provider || row.key)}
                  <td class="py-3 px-4 text-left">
                    {#if bal.loading}
                      <span class="inline-flex items-center gap-1.5 text-[11px] text-text-muted">
                        <span class="w-1.5 h-1.5 rounded-full bg-brand-500 animate-ping"></span>
                        Checking...
                      </span>
                    {:else if bal.hasData}
                      <div class="inline-flex items-center gap-2">
                        <div class="flex flex-col min-w-0">
                          <span class="font-mono text-xs font-bold {bal.color}">
                            {bal.balanceText}
                          </span>
                          {#if bal.detailText}
                            <span class="text-[10px] text-text-muted truncate">
                              {bal.detailText}
                            </span>
                          {/if}
                        </div>
                        {#if bal.connectionId}
                          <button
                            type="button"
                            class="text-[10px] text-text-muted hover:text-brand-400 transition-colors p-1 rounded hover:bg-surface-2 cursor-pointer"
                            title="Refresh live saldo"
                            onclick={() => refreshProviderBalance(bal.connectionId!)}
                          >
                            ↻
                          </button>
                        {/if}
                      </div>
                    {:else if bal.connectionId}
                      <button
                        type="button"
                        class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[11px] font-medium bg-surface-2 text-text-muted hover:text-text-main border border-border/60 hover:border-brand-500/40 transition-colors cursor-pointer"
                        onclick={() => refreshProviderBalance(bal.connectionId!)}
                      >
                        <span>Check Saldo</span>
                        <span class="text-[10px]">↻</span>
                      </button>
                    {:else}
                      <span class="text-text-muted text-xs">—</span>
                    {/if}
                  </td>
                {/if}
                {#if tableView !== 'provider'}
                  <td class="py-3 px-4">
                    <div class="flex items-center gap-1.5">
                      {#if row.provider}
                        <ProviderArtwork
                          id={row.provider}
                          alt={row.provider}
                          class="w-3.5 h-3.5 object-contain rounded shrink-0 text-[9px] leading-none font-semibold"
                        />
                      {/if}
                      <Badge variant="neutral" size="sm">{row.provider || 'unknown'}</Badge>
                    </div>
                  </td>
                {/if}
                <td class="py-3 px-4 text-right font-mono font-semibold text-text-main">
                  {fmt(row.requests)}
                </td>
                {#if viewMode === 'costs'}
                  <td class="py-3 px-4 text-right font-mono font-bold text-warning">
                    {costIn(row.cost ?? 0)}
                  </td>
                {:else}
                  <td class="py-3 px-4 text-right font-mono text-brand-500">
                    {fmt(row.promptTokens)}
                  </td>
                  <td class="py-3 px-4 text-right font-mono text-success">
                    {fmt(row.completionTokens)}
                  </td>
                  <td class="py-3 px-4 text-right font-mono text-info">
                    {fmt(row.cachedTokens)}
                  </td>
                {/if}
                <td class="py-3 px-4 text-right text-text-muted whitespace-nowrap text-[11px]">
                  {timeAgo(row.lastUsed)}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </Card>
</div>