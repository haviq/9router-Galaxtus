<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type SystemVersionInfo, type ProviderConnection } from '../api/client'
  import gsap from 'gsap'
  import { copyToClipboard } from '../lib/clipboard'
  import { TAB_ROUTES, type ActiveTab } from '../lib/router'
  import { isNavLinkActive, orderedNavGroups } from '../lib/nav'
  import { getQuotaCache, setQuotaCache, parseQuotaData, type QuotaEntry } from './quota/types'
  import { providerRoutingPrefix } from './analytics/types'

  export type { ActiveTab }
  let {
    activeTab = $bindable('endpoint'),
    navigate = (tab: ActiveTab) => {
      activeTab = tab
    },
    activeConnections = 0,
    totalConnections = 0,
    connections = [] as ProviderConnection[],
    onClose,
    showUpdateModal = $bindable(false),
    updateInfo = $bindable(null),
    version = $bindable(''),
    hiddenNav,
    navOrder,
    brandName = '',
    brandLogo = '',
  }: {
    activeTab: ActiveTab
    navigate?: (tab: ActiveTab, replace?: boolean) => void
    activeConnections?: number
    totalConnections?: number
    connections?: ProviderConnection[]
    onClose?: () => void
    showUpdateModal?: boolean
    updateInfo?: SystemVersionInfo | null
    version?: string
    hiddenNav?: string[]
    navOrder?: string[]
    brandName?: string
    brandLogo?: string
  } = $props()

  const displayBrandName = $derived(brandName || '9router Galactus')
  const displayBrandLogo = $derived(brandLogo || '/favicon.svg')
  const INSTALL_CMD = '9router-go update'
  let copied = $state(false)

  // Media providers accordion (collapsed by default)
  let isMediaOpen = $state(false)

  // Live Provider Saldo & Prefix Widget State
  let liveConnections = $state<ProviderConnection[]>([])
  let balanceLoading = $state<Record<string, boolean>>({})
  let isProbingAllSidebar = $state(false)
  let copiedSidebarPrefix = $state<string | null>(null)
  let quotaCache = $state(getQuotaCache())
  let isBalancesOpen = $state(true)

  function copyPrefix(prefix: string) {
    copyToClipboard(prefix)
    copiedSidebarPrefix = prefix
    setTimeout(() => {
      if (copiedSidebarPrefix === prefix) copiedSidebarPrefix = null
    }, 1500)
  }

  async function refreshBalance(connId: string) {
    if (balanceLoading[connId]) return
    balanceLoading = { ...balanceLoading, [connId]: true }
    try {
      const res = await api.getConnectionUsage(connId, true)
      const conn = liveConnections.find((c) => c.id === connId)
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
      console.error('Failed to probe provider balance in sidebar:', e)
    } finally {
      balanceLoading = { ...balanceLoading, [connId]: false }
    }
  }

  async function probeAllSidebar() {
    if (isProbingAllSidebar || !liveConnections.length) return
    isProbingAllSidebar = true
    try {
      const tasks = liveConnections.map((c) => refreshBalance(c.id))
      await Promise.allSettled(tasks)
    } finally {
      isProbingAllSidebar = false
    }
  }

  function getConnBalance(connId: string) {
    const cached = quotaCache[connId]
    const isLoading = !!balanceLoading[connId]
    if (!cached || !cached.quotas || cached.quotas.length === 0) {
      return { text: 'Check Saldo', color: 'text-text-muted', isLoading, hasData: false }
    }
    const creditQ = cached.quotas.find((q) => q.isCreditBalance || (q.name && q.name.toLowerCase().includes('balance')) || (q.name && q.name.toLowerCase().includes('credit')))
    if (creditQ) {
      const cur = creditQ.currency || 'USD'
      const tot = typeof creditQ.total === 'number' ? creditQ.total : 0
      const isPositive = tot > 0
      return {
        text: `${cur === 'USD' ? '$' : ''}${tot.toFixed(2)} ${cur}`,
        color: isPositive ? 'text-cyan-400 font-bold' : 'text-rose-400 font-bold',
        isLoading,
        hasData: true,
      }
    }
    const firstQ = cached.quotas[0]
    if (firstQ.unlimited) {
      return { text: 'Unlimited', color: 'text-emerald-400 font-bold', isLoading, hasData: true }
    }
    if (firstQ.remainingPercentage != null) {
      const pct = firstQ.remainingPercentage
      return {
        text: `${pct}%`,
        color: pct > 30 ? 'text-emerald-400 font-bold' : 'text-amber-400 font-bold',
        isLoading,
        hasData: true,
      }
    }
    return { text: firstQ.name || 'Active', color: 'text-text-main font-semibold', isLoading, hasData: true }
  }

  $effect(() => {
    if (connections && connections.length > 0) {
      liveConnections = connections
    }
  })

  onMount(async () => {
    try {
      const res = await api.getConnections()
      if (Array.isArray(res) && res.length > 0) {
        liveConnections = res
        // Auto-probe uncached active providers in the background
        for (const conn of res.slice(0, 6)) {
          if (!quotaCache[conn.id]) {
            refreshBalance(conn.id)
          }
        }
      }
    } catch {
      // quiet fallback
    }
  })

  $effect(() => {
    api
      .getSystemVersion()
      .then((v) => {
        if (v) {
          version = v.currentVersion || ''
          if (v.hasUpdate) {
            updateInfo = v
          }
        }
      })
      .catch(() => {})

    const timer = setTimeout(() => {
      api
        .checkUpdate()
        .then((v) => {
          if (v) {
            version = v.currentVersion || version
            if (v.hasUpdate) {
              updateInfo = v
            }
          }
        })
        .catch(() => {})
    }, 2500)

    return () => clearTimeout(timer)
  })

  async function copyInstallCmd() {
    await copyToClipboard(INSTALL_CMD)
    copied = true
    setTimeout(() => {
      copied = false
    }, 2000)
  }

  // Persistent GPU-accelerated liquid sliding pill + trailing droplet
  let navEl: HTMLElement | undefined = $state()
  let pillEl: HTMLDivElement | undefined = $state()
  let trailEl: HTMLDivElement | undefined = $state()
  let pillInitialized = false
  let lastTargetEl: HTMLElement | null = null

  function animateLiquidPillTo(targetItem: HTMLElement, animateFlow = true) {
    if (!navEl || !pillEl || !trailEl) return

    const navRect = navEl.getBoundingClientRect()
    const itemRect = targetItem.getBoundingClientRect()
    const x = itemRect.left - navRect.left
    const y = itemRect.top - navRect.top + navEl.scrollTop
    const width = itemRect.width
    const height = itemRect.height

    if (!pillInitialized || !animateFlow) {
      pillInitialized = true
      lastTargetEl = targetItem
      gsap.set([pillEl, trailEl], {
        x,
        y,
        width,
        height,
        scaleX: 1,
        scaleY: 1,
        force3D: true,
      })
      gsap.set(pillEl, { opacity: 1 })
      gsap.set(trailEl, { opacity: 0 })
      return
    }

    const currentY = Number(gsap.getProperty(pillEl, 'y')) || y
    const currentX = Number(gsap.getProperty(pillEl, 'x')) || x
    const dy = y - currentY
    const absDy = Math.abs(dy)

    lastTargetEl = targetItem
    gsap.killTweensOf([pillEl, trailEl])

    // 1. Lead capsule glides with pure GPU translate3d + smooth bell-curve liquid stretch
    const stretchY = Math.min(1.26, 1 + absDy / 260)
    const squeezeX = Math.max(0.95, 1 - absDy / 1400)

    gsap.to(pillEl, {
      x,
      y,
      width,
      height,
      opacity: 1,
      duration: 0.44,
      ease: 'expo.out',
      force3D: true,
    })

    if (absDy > 4) {
      gsap.to(pillEl, {
        keyframes: [
          { scaleY: stretchY, scaleX: squeezeX, duration: 0.13, ease: 'sine.out' },
          { scaleY: 1, scaleX: 1, duration: 0.35, ease: 'elastic.out(1, 0.72)' },
        ],
        force3D: true,
      })

      // 2. Coupled liquid trailing stream that stretches across the gap and pours into the target
      const midY = currentY + dy * 0.35
      const trailStretch = Math.min(1.65, 1 + absDy / 140)
      gsap.set(trailEl, {
        x: currentX,
        y: currentY,
        width,
        height,
        scaleY: 1,
        scaleX: 0.96,
        opacity: 0.75,
        force3D: true,
      })
      gsap.to(trailEl, {
        keyframes: [
          {
            y: midY,
            scaleY: trailStretch,
            scaleX: 0.92,
            opacity: 0.65,
            duration: 0.14,
            ease: 'power2.out',
          },
          {
            x,
            y,
            scaleY: 1,
            scaleX: 1,
            opacity: 0,
            duration: 0.36,
            ease: 'expo.out',
          },
        ],
        force3D: true,
      })
    }
  }

  function syncNavPill(animateFlow = true) {
    if (!navEl || !pillEl) return
    const activeItem = navEl.querySelector<HTMLElement>('[data-nav-active="true"]')
    if (!activeItem) {
      gsap.set(pillEl, { opacity: 0 })
      if (trailEl) gsap.set(trailEl, { opacity: 0 })
      return
    }
    if (activeItem === lastTargetEl && pillInitialized) return
    animateLiquidPillTo(activeItem, animateFlow)
  }

  function handleNav(tab: ActiveTab, e?: MouseEvent) {
    if (e) {
      if (e.ctrlKey || e.metaKey || e.shiftKey || e.altKey || (e.button !== undefined && e.button !== 0)) return
      e.preventDefault()
      const clickedEl = e.currentTarget as HTMLElement | null
      if (clickedEl && navEl?.contains(clickedEl)) {
        // Start GPU liquid flow BEFORE mounting the new route view so frame 0 is 120fps butter-smooth
        animateLiquidPillTo(clickedEl, true)
      }
    }
    requestAnimationFrame(() => {
      navigate(tab)
      onClose?.()
    })
  }

  const navGroups = $derived(orderedNavGroups(hiddenNav, navOrder))

  $effect(() => {
    void activeTab
    void isMediaOpen
    void navGroups
    requestAnimationFrame(() => syncNavPill(true))
  })
</script>

<aside
  class="ios-sidebar flex h-full max-h-screen w-72 flex-col overflow-hidden border-r border-border-subtle bg-sidebar backdrop-blur-xl transition-colors duration-300 flex-shrink-0 select-none z-30"
>
  <!-- Brand header: 9router Galactus with Galaxy logo -->
  <div class="px-6 pt-6 pb-4 flex flex-col gap-2 shrink-0">
    <a
      href={TAB_ROUTES.endpoint}
      onclick={(e) => handleNav('endpoint', e)}
      class="flex items-center gap-3 cursor-pointer group"
    >
      <div
        class="ios-squircle-icon flex items-center justify-center size-10 rounded-[13px] bg-surface-2 border border-border-subtle shadow-[var(--shadow-warm)] flex-shrink-0 group-hover:scale-105 transition-transform overflow-hidden p-0.5"
      >
        <img
          src={displayBrandLogo}
          alt={displayBrandName}
          class="w-full h-full object-contain"
        />
      </div>
      <div class="flex flex-col min-w-0">
        <h1 class="text-lg font-semibold tracking-tight text-text-main truncate leading-snug">
          {displayBrandName}
        </h1>
        <span class="text-xs text-text-muted leading-tight">
          {version ? `v${version}` : 'v1.9.9'}
        </span>
      </div>
    </a>

    <!-- Update notification banner below version (matches upstream 9router) -->
    {#if updateInfo && updateInfo.hasUpdate}
      <div
        class="flex flex-col gap-1.5 rounded-lg p-2 bg-green-500/10 dark:bg-amber-500/10 border border-green-500/30 dark:border-amber-500/30 text-green-700 dark:text-amber-400 mt-0.5 animate-in fade-in duration-200"
      >
        <span class="text-[11px] font-semibold text-green-700 dark:text-amber-400 flex items-center gap-1">
          <span class="material-symbols-outlined text-[14px]">arrow_upward</span>
          <span class="truncate">New version available: v{updateInfo.latestVersion}</span>
        </span>
        <div class="flex items-center gap-1.5">
          <button
            type="button"
            onclick={() => (showUpdateModal = true)}
            class="px-2 py-0.5 rounded bg-green-600 hover:bg-green-700 dark:bg-amber-500 dark:hover:bg-amber-600 text-white text-[11px] font-semibold transition-colors cursor-pointer shrink-0"
          >
            Update now
          </button>
          <button
            type="button"
            onclick={copyInstallCmd}
            title="Copy install command"
            class="flex-1 text-left hover:opacity-80 transition-opacity cursor-pointer min-w-0"
          >
            <code class="block text-[10px] text-green-700 dark:text-amber-300 font-mono truncate bg-surface/70 px-1 py-0.5 rounded">
              {copied ? '✓ copied!' : INSTALL_CMD}
            </code>
          </button>
        </div>
      </div>
    {/if}
  </div>

  <!-- Navigation -->
  <nav
    bind:this={navEl}
    data-no-bridge
    class="relative flex-1 min-h-0 px-4 py-2 space-y-0.5 overflow-y-auto custom-scrollbar"
  >
    <div bind:this={trailEl} class="sidebar-flow-trail pointer-events-none" aria-hidden="true"></div>
    <div bind:this={pillEl} class="sidebar-flow-pill pointer-events-none" aria-hidden="true"></div>
    {#each navGroups as group (group.id)}
      <div class="{group.label ? 'pt-3 mt-2' : ''} space-y-0.5">
        {#if group.label}
          <p class="px-3 text-xs font-semibold text-text-muted/60 uppercase tracking-wider mb-2">
            {group.label}
          </p>
        {/if}

        {#each group.entries as entry (entry.id)}
          {#if entry.kind === 'accordion'}
            <button
              type="button"
              onclick={() => (isMediaOpen = !isMediaOpen)}
              class="w-full flex items-center gap-3 px-3 py-1.5 rounded-lg transition-colors group cursor-pointer {activeTab.startsWith(
                'media-'
              )
                ? 'text-primary font-semibold'
                : 'text-text-muted hover:bg-surface-2/60 hover:text-text-main'}"
            >
              <span class="material-symbols-outlined text-[18px]">{entry.icon}</span>
              <span class="text-[13px] flex-1 text-left">{entry.label}</span>
              <span
                class="material-symbols-outlined text-[14px] transition-transform duration-200"
                style:transform={isMediaOpen ? 'rotate(180deg)' : 'rotate(0deg)'}
              >
                expand_more
              </span>
            </button>

            {#if isMediaOpen}
              <div class="pl-4 space-y-0.5">
                {#each entry.children as child (child.id)}
                  {@const childActive = isNavLinkActive(child.id, activeTab)}
                  <a
                    href={TAB_ROUTES[child.id]}
                    onclick={(e) => handleNav(child.id, e)}
                    data-nav-active={childActive ? 'true' : 'false'}
                    class="relative z-10 flex items-center gap-3 px-3 py-1.5 rounded-[13px] transition-colors duration-200 group cursor-pointer {childActive
                      ? 'text-primary font-semibold'
                      : 'text-text-muted hover:bg-surface-2/50 hover:text-text-main'}"
                  >
                    <span
                      class="material-symbols-outlined text-[16px] {childActive
                        ? 'fill-1'
                        : 'group-hover:text-primary transition-colors'}"
                    >
                      {child.icon}
                    </span>
                    <span class="text-[13px]">{child.label}</span>
                  </a>
                {/each}
              </div>
            {/if}
          {:else}
            {@const active = isNavLinkActive(entry.id, activeTab)}
            <a
              href={TAB_ROUTES[entry.id]}
              onclick={(e) => handleNav(entry.id, e)}
              data-nav-active={active ? 'true' : 'false'}
              class="ios-nav-item relative z-10 flex items-center gap-3 px-3.5 py-2 rounded-[13px] transition-colors duration-200 group cursor-pointer {active
                ? 'text-primary font-semibold'
                : 'text-text-muted hover:bg-surface-2/50 hover:text-text-main'}"
            >
              <span
                class="material-symbols-outlined text-[18px] {active
                  ? 'fill-1'
                  : 'group-hover:text-primary transition-colors'}"
              >
                {entry.icon}
              </span>
              <span class="text-[13px]">{entry.label}</span>
            </a>
          {/if}
        {/each}
      </div>
    {/each}
  </nav>

  <!-- Live Provider Saldo & Routing Prefix Widget (rv/, or/, etc.) -->
  {#if liveConnections.length > 0}
    <div class="px-4 py-2 border-t border-border-subtle shrink-0">
      <div class="rounded-xl bg-surface/80 border border-border-subtle p-2.5 space-y-2">
        <div class="flex items-center justify-between">
          <button
            type="button"
            class="flex items-center gap-1.5 text-[11px] font-semibold text-text-muted hover:text-text-main transition-colors cursor-pointer"
            onclick={() => (isBalancesOpen = !isBalancesOpen)}
          >
            <span class="w-1.5 h-1.5 rounded-full bg-cyan-400 animate-pulse"></span>
            <span class="uppercase tracking-wider">Saldo & Prefix</span>
            <span class="text-[10px] text-text-subtle font-mono">({liveConnections.length})</span>
            <span
              class="material-symbols-outlined text-[14px] transition-transform duration-200"
              style:transform={isBalancesOpen ? 'rotate(180deg)' : 'rotate(0deg)'}
            >
              expand_more
            </span>
          </button>
          <div class="flex items-center gap-1">
            <button
              type="button"
              class="text-[10px] px-1.5 py-0.5 rounded bg-brand-500/15 text-brand-400 hover:bg-brand-500/25 transition-colors cursor-pointer inline-flex items-center gap-0.5 font-medium"
              title="Refresh semua saldo provider"
              onclick={probeAllSidebar}
              disabled={isProbingAllSidebar}
            >
              <span class={isProbingAllSidebar ? 'animate-spin' : ''}>↻</span>
              <span>{isProbingAllSidebar ? 'Probing...' : 'Probe'}</span>
            </button>
          </div>
        </div>

        {#if isBalancesOpen}
          <div class="space-y-1.5 max-h-40 overflow-y-auto custom-scrollbar pr-0.5">
            {#each liveConnections as conn (conn.id)}
              {@const prefix = providerRoutingPrefix(conn.provider || conn.name)}
              {@const bal = getConnBalance(conn.id)}
              <div class="flex items-center justify-between gap-1.5 py-1 px-1.5 rounded-lg bg-surface-2/40 hover:bg-surface-2/70 transition-colors text-xs">
                <div class="flex items-center gap-1.5 min-w-0">
                  {#if prefix}
                    <button
                      type="button"
                      class="px-1.5 py-0.5 rounded text-[10px] font-mono font-bold bg-brand-500/15 text-brand-400 border border-brand-500/30 hover:bg-brand-500/25 transition-colors cursor-pointer shrink-0"
                      title="Routing prefix {prefix}<model> (Click to copy)"
                      onclick={() => copyPrefix(prefix)}
                    >
                      {prefix}
                    </button>
                  {/if}
                  <span class="truncate text-[11px] font-medium text-text-main" title={conn.name || conn.provider}>
                    {conn.name || conn.provider}
                  </span>
                </div>

                <div class="flex items-center gap-1 shrink-0">
                  {#if bal.isLoading}
                    <span class="text-[10px] text-text-muted animate-pulse">...</span>
                  {:else if bal.hasData}
                    <span class="text-[11px] font-mono {bal.color}">{bal.text}</span>
                  {:else}
                    <button
                      type="button"
                      class="text-[10px] text-brand-400 hover:underline cursor-pointer"
                      onclick={() => refreshBalance(conn.id)}
                    >
                      Cek
                    </button>
                  {/if}
                  <button
                    type="button"
                    class="text-[10px] text-text-muted hover:text-brand-400 transition-colors p-0.5 rounded cursor-pointer"
                    title="Refresh saldo akun ini"
                    onclick={() => refreshBalance(conn.id)}
                  >
                    ↻
                  </button>
                </div>
              </div>
            {/each}
          </div>

          {#if copiedSidebarPrefix}
            <div class="text-[10px] text-emerald-400 text-center py-0.5 bg-emerald-500/10 rounded border border-emerald-500/20 font-mono">
              ✓ Prefix {copiedSidebarPrefix} disalin!
            </div>
          {/if}

          <div class="pt-1 flex items-center justify-between text-[10px] text-text-muted border-t border-border-subtle/50">
            <button
              type="button"
              class="hover:text-brand-400 transition-colors cursor-pointer"
              onclick={(e) => handleNav('quota', e)}
            >
              Quota Tracker &rarr;
            </button>
            <button
              type="button"
              class="hover:text-brand-400 transition-colors cursor-pointer"
              onclick={(e) => handleNav('analytics', e)}
            >
              Analytics &rarr;
            </button>
          </div>
        {/if}
      </div>
    </div>
  {/if}

  <!-- Bottom connection summary -->
  <div class="p-4 border-t border-border-subtle shrink-0">
    <a
      href={TAB_ROUTES.connections}
      onclick={(e) => handleNav('connections', e)}
      class="p-2.5 rounded-[10px] bg-surface border border-border-subtle flex items-center justify-between hover:border-brand-500/30 transition-colors cursor-pointer group"
    >
      <div class="min-w-0">
        <p class="text-[11px] text-text-muted uppercase tracking-wide group-hover:text-text-main transition-colors">Providers</p>
        <p class="text-xs font-semibold text-text-main mt-0.5 truncate">
          {activeConnections} <span class="text-text-muted font-normal">/ {totalConnections} active</span>
        </p>
      </div>
      <span class="material-symbols-outlined text-primary text-[18px]">radio_button_checked</span>
    </a>
  </div>
</aside>
