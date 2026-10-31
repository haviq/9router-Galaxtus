<script lang="ts">
  import { api, type SystemVersionInfo } from '../api/client'
  import gsap from 'gsap'
  import { copyToClipboard } from '../lib/clipboard'
  import { TAB_ROUTES, type ActiveTab } from '../lib/router'
  import { isNavLinkActive, orderedNavGroups } from '../lib/nav'

  export type { ActiveTab }
  let {
    activeTab = $bindable('endpoint'),
    navigate = (tab: ActiveTab) => {
      activeTab = tab
    },
    activeConnections = 0,
    totalConnections = 0,
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
  <!-- Window control / traffic lights -->
  <div class="flex items-center gap-2 px-6 pt-5 pb-2 shrink-0">
    <div class="w-3 h-3 rounded-full bg-[#FF5F56]"></div>
    <div class="w-3 h-3 rounded-full bg-[#FFBD2E]"></div>
    <div class="w-3 h-3 rounded-full bg-[#27C93F]"></div>
  </div>

  <!-- Brand header: 9router Galactus with official favicon.svg logo -->
  <div class="px-6 py-4 flex flex-col gap-2 shrink-0">
    <a
      href={TAB_ROUTES.endpoint}
      onclick={(e) => handleNav('endpoint', e)}
      class="flex items-center gap-3 cursor-pointer group"
    >
      <div
        class="ios-squircle-icon flex items-center justify-center size-10 rounded-[13px] bg-surface-2 border border-border-subtle shadow-[var(--shadow-warm)] flex-shrink-0 group-hover:scale-105 transition-transform overflow-hidden p-1.5"
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

  <!-- Bottom connection summary -->
  <div class="p-4 border-t border-border-subtle shrink-0">
    <div
      class="p-2.5 rounded-[10px] bg-surface border border-border-subtle flex items-center justify-between"
    >
      <div class="min-w-0">
        <p class="text-[11px] text-text-muted uppercase tracking-wide">Providers</p>
        <p class="text-xs font-semibold text-text-main mt-0.5 truncate">
          {activeConnections} <span class="text-text-muted font-normal">/ {totalConnections} active</span>
        </p>
      </div>
      <span class="material-symbols-outlined text-primary text-[18px]">radio_button_checked</span>
    </div>
  </div>
</aside>
