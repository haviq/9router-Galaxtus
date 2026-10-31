<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import gsap from 'gsap'
  import {
    Key,
    Copy,
    Check,
    CreditCard,
    Cpu,
    Zap,
    RefreshCw,
    Terminal,
    Sliders,
    Activity,
    Send,
    Play,
    Clock,
    AlertTriangle,
    ArrowUpRight,
    ChevronRight,
    Eye,
    EyeOff,
    ShieldCheck,
    Search,
    Sparkles,
    BookOpen,
    LogOut,
    Layers,
    CheckCircle2,
    SlidersHorizontal
  } from 'lucide-svelte'

  interface RecentRequest {
    id: number
    timestamp: string
    model: string
    endpoint: string
    promptTokens: number
    completionTokens: number
    totalTokens: number
    cost: number
    status: string
  }

  interface TokenStats {
    totalRequests: number
    promptTokens: number
    completionTokens: number
    totalTokens: number
    cost: number
  }

  interface PricingItem {
    id: string
    modelPattern: string
    priceInput1k: number
    priceOutput1k: number
    currency: string
  }

  interface ClientInfo {
    key: string
    keyMasked: string
    name: string
    balance: number
    currency: string
    tokenQuota: number
    usedTokens: number
    rateLimitRpm: number
    rateLimitTpm: number
    concurrency: number
    allowedModels: string[]
    expiresAt?: string
    isActive: boolean
    recentRequests?: RecentRequest[]
    stats?: TokenStats
    pricingTable?: PricingItem[]
    systemStatus?: {
      status: string
      version: string
      loadBalance: string
      promptCache: string
      antiLeak: string
    }
    configs: {
      baseUrl: string
      cursor: {
        openaiBaseUrl: string
        apiKey: string
      }
      cline: {
        provider: string
        baseUrl: string
        apiKey: string
      }
      windsurf: {
        modelProvider: string
        endpoint: string
        apiKey: string
      }
      pythonSdk: string
    }
  }

  let inputKey = $state('')
  let showKey = $state(false)
  let clientInfo = $state<ClientInfo | null>(null)
  let loading = $state(false)
  let refreshing = $state(false)
  let errorMsg = $state('')
  let copiedId = $state<string | null>(null)

  // Navigation tabs
  let activeMainTab = $state<'telemetry' | 'playground' | 'guides' | 'tariffs'>('telemetry')
  let activeGuideTab = $state<'cursor' | 'cline' | 'windsurf' | 'python' | 'curl'>('cursor')

  // Search & filter in telemetry
  let logSearchQuery = $state('')
  let selectedLogId = $state<number | null>(null)

  // Auto-refresh & telemetry
  let autoRefreshEnabled = $state(true)
  let autoRefreshInterval: ReturnType<typeof setInterval> | null = null
  let pingLatencyMs = $state(12)
  let lastSyncTime = $state('')

  // Animated odometer for Balance
  let animatedBalance = $state(0)
  let balanceTweenObj = { val: 0 }

  // Workbench Playground state
  let testModel = $state('')
  let testSystemPrompt = $state('You are a concise, senior systems architect. Respond with clean, production-grade code and direct technical analysis.')
  let testPrompt = $state('Write a high-performance Go function to find prime numbers using Sieve of Eratosthenes with a brief complexity note.')
  let showSystemPrompt = $state(true)
  let testRunning = $state(false)
  let testOutput = $state('')
  let testLatencyMs = $state(0)
  let testTokensUsed = $state(0)
  let testError = $state('')

  // DOM node references for GSAP
  let loginCardRef: HTMLDivElement | null = $state(null)
  let workspaceRef: HTMLDivElement | null = $state(null)

  // Derived analytics
  let totalPromptTokens = $derived(clientInfo?.stats?.promptTokens ?? 0)
  let totalCompletionTokens = $derived(clientInfo?.stats?.completionTokens ?? 0)
  let grandTotalTokens = $derived(
    (clientInfo?.stats?.totalTokens || clientInfo?.usedTokens || (totalPromptTokens + totalCompletionTokens)) || 0
  )
  let promptSharePct = $derived(
    grandTotalTokens > 0 ? Math.min(100, Math.max(0, (totalPromptTokens / grandTotalTokens) * 100)) : 50
  )
  let completionSharePct = $derived(
    grandTotalTokens > 0 ? Math.max(0, 100 - promptSharePct) : 50
  )

  let filteredRequests = $derived.by(() => {
    const list = clientInfo?.recentRequests ?? []
    const q = logSearchQuery.trim().toLowerCase()
    if (!q) return list
    return list.filter(
      (r) =>
        r.model.toLowerCase().includes(q) ||
        r.status.toLowerCase().includes(q) ||
        (r.endpoint && r.endpoint.toLowerCase().includes(q))
    )
  })

  // Chart bars from recent requests (oldest to newest, up to 16)
  let chartBars = $derived.by(() => {
    const reqs = [...(clientInfo?.recentRequests ?? [])].slice(0, 16).reverse()
    if (reqs.length === 0) return []
    const maxTok = Math.max(...reqs.map((r) => r.totalTokens || (r.promptTokens + r.completionTokens) || 1), 1)
    return reqs.map((r, idx) => {
      const tot = r.totalTokens || (r.promptTokens + r.completionTokens) || 0
      const heightPct = Math.max(8, Math.round((tot / maxTok) * 100))
      const promptPct = tot > 0 ? Math.round((r.promptTokens / tot) * 100) : 70
      return {
        ...r,
        idx,
        tot,
        heightPct,
        promptPct
      }
    })
  })

  onMount(() => {
    // Bust stale service workers that might cache old portal bundles
    if ('serviceWorker' in navigator) {
      navigator.serviceWorker.getRegistrations().then((regs) => {
        for (const reg of regs) {
          reg.update().catch(() => {})
        }
      }).catch(() => {})
    }

    const saved = localStorage.getItem('9router_client_portal_key') || ''
    if (saved) {
      inputKey = saved
      void loadPortalInfo(saved)
    } else if (loginCardRef) {
      gsap.from(loginCardRef, {
        opacity: 0,
        y: 16,
        duration: 0.45,
        ease: 'power3.out'
      })
    }

    autoRefreshInterval = setInterval(() => {
      if (autoRefreshEnabled && clientInfo && !loading && !refreshing) {
        void refreshTelemetry(false)
      }
    }, 5000)

    const handleKeydown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter' && activeMainTab === 'playground') {
        e.preventDefault()
        void runPlaygroundTest()
      }
    }
    window.addEventListener('keydown', handleKeydown)

    return () => {
      if (autoRefreshInterval) clearInterval(autoRefreshInterval)
      window.removeEventListener('keydown', handleKeydown)
    }
  })

  onDestroy(() => {
    if (autoRefreshInterval) clearInterval(autoRefreshInterval)
  })

  function animateBalanceTo(target: number) {
    if (target < 0) {
      animatedBalance = -1
      return
    }
    gsap.to(balanceTweenObj, {
      val: target,
      duration: 0.85,
      ease: 'power3.out',
      onUpdate: () => {
        animatedBalance = Math.round(balanceTweenObj.val)
      }
    })
  }

  function switchTab(tab: 'telemetry' | 'playground' | 'guides' | 'tariffs') {
    activeMainTab = tab
    setTimeout(() => {
      if (workspaceRef) {
        gsap.fromTo(
          workspaceRef.querySelectorAll('.gsap-panel'),
          { opacity: 0, y: 8 },
          { opacity: 1, y: 0, stagger: 0.04, duration: 0.32, ease: 'power3.out', clearProps: 'all' }
        )
      }
    }, 10)
  }

  async function loadPortalInfo(keyToFetch?: string) {
    const k = (keyToFetch || inputKey).trim()
    if (!k) return

    loading = true
    errorMsg = ''
    const t0 = performance.now()
    try {
      const res = await fetch('/api/client/portal-info', {
        headers: { Authorization: `Bearer ${k}` }
      })
      pingLatencyMs = Math.max(4, Math.round(performance.now() - t0))
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: { message: `HTTP ${res.status}` } }))
        throw new Error(err.error?.message || `HTTP ${res.status}: Access Denied`)
      }
      const data = (await res.json()) as ClientInfo
      clientInfo = data
      lastSyncTime = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
      localStorage.setItem('9router_client_portal_key', k)

      if (data.allowedModels && data.allowedModels.length > 0 && !testModel) {
        testModel = data.allowedModels[0] === '*' ? 'gpt-4o' : data.allowedModels[0]
      }

      animateBalanceTo(data.balance)

      setTimeout(() => {
        if (workspaceRef) {
          gsap.fromTo(
            workspaceRef.querySelectorAll('.gsap-panel'),
            { opacity: 0, y: 10 },
            { opacity: 1, y: 0, stagger: 0.04, duration: 0.4, ease: 'power3.out', clearProps: 'all' }
          )
        }
      }, 30)
    } catch (e: any) {
      errorMsg = e.message || 'Failed to authenticate API key'
      clientInfo = null
      if (loginCardRef) {
        gsap.fromTo(
          loginCardRef,
          { x: -6 },
          { x: 0, duration: 0.3, ease: 'elastic.out(2, 0.2)' }
        )
      }
    } finally {
      loading = false
    }
  }

  async function refreshTelemetry(showSpinner = true) {
    if (!clientInfo) return
    if (showSpinner) refreshing = true
    const t0 = performance.now()
    try {
      const res = await fetch('/api/client/portal-info', {
        headers: { Authorization: `Bearer ${clientInfo.key}` }
      })
      pingLatencyMs = Math.max(4, Math.round(performance.now() - t0))
      if (res.ok) {
        const data = (await res.json()) as ClientInfo
        clientInfo = data
        lastSyncTime = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
        animateBalanceTo(data.balance)
      }
    } catch {} finally {
      if (showSpinner) refreshing = false
    }
  }

  function handleLogout() {
    localStorage.removeItem('9router_client_portal_key')
    clientInfo = null
    inputKey = ''
    setTimeout(() => {
      if (loginCardRef) {
        gsap.from(loginCardRef, {
          opacity: 0,
          y: 12,
          duration: 0.35,
          ease: 'power3.out'
        })
      }
    }, 30)
  }

  async function copyText(text: string, id: string) {
    try {
      await navigator.clipboard.writeText(text)
      copiedId = id
      setTimeout(() => {
        if (copiedId === id) copiedId = null
      }, 2000)
    } catch {}
  }

  async function runPlaygroundTest() {
    if (!clientInfo || !testModel || !testPrompt.trim() || testRunning) return

    testRunning = true
    testOutput = ''
    testError = ''
    testLatencyMs = 0
    testTokensUsed = 0

    const startTime = performance.now()
    try {
      const messages: Array<{ role: string; content: string }> = []
      if (showSystemPrompt && testSystemPrompt.trim()) {
        messages.push({ role: 'system', content: testSystemPrompt.trim() })
      }
      messages.push({ role: 'user', content: testPrompt.trim() })

      const res = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${clientInfo.key}`
        },
        body: JSON.stringify({
          model: testModel,
          messages,
          stream: true
        })
      })

      testLatencyMs = Math.round(performance.now() - startTime)

      if (!res.ok) {
        const errJson = await res.json().catch(() => ({ error: { message: `HTTP ${res.status}` } }))
        throw new Error(errJson.error?.message || `HTTP ${res.status} Error`)
      }

      const reader = res.body?.getReader()
      const decoder = new TextDecoder()
      if (!reader) throw new Error('Readable stream not supported')

      let buffer = ''
      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() || ''

        for (const line of lines) {
          const trimmed = line.trim()
          if (!trimmed || !trimmed.startsWith('data: ')) continue
          const dataStr = trimmed.slice(6)
          if (dataStr === '[DONE]') continue

          try {
            const parsed = JSON.parse(dataStr)
            const delta = parsed.choices?.[0]?.delta?.content || ''
            testOutput += delta
            if (parsed.usage) {
              testTokensUsed = parsed.usage.total_tokens || 0
            }
          } catch {}
        }
      }

      setTimeout(() => {
        void refreshTelemetry(false)
      }, 400)
    } catch (e: any) {
      testError = e.message || 'Request failed'
    } finally {
      testRunning = false
    }
  }

  function formatTimestamp(ts: string) {
    if (!ts) return '—'
    try {
      const d = new Date(ts)
      return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
    } catch {
      return ts
    }
  }
</script>

<svelte:head>
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous" />
  <link
    href="https://fonts.googleapis.com/css2?family=Instrument+Serif:ital@0;1&display=swap"
    rel="stylesheet"
  />
</svelte:head>

<div class="min-h-screen bg-[#141413] text-[#f0efea] font-sans antialiased selection:bg-[#d97757]/25 selection:text-[#f0efea]">
  {#if !clientInfo}
    <!-- =========================================================================
         UNAUTHENTICATED STATE: ANTHROPIC CLAUDE EDITORIAL SIGN-IN
         ========================================================================= -->
    <div class="min-h-screen flex flex-col justify-between p-6 sm:p-10">
      <!-- Top Minimal Brand Bar -->
      <header class="w-full max-w-6xl mx-auto flex items-center justify-between">
        <div class="flex items-center gap-3">
          <div class="w-7 h-7 rounded-lg bg-[#d97757]/15 border border-[#d97757]/30 flex items-center justify-center text-[#d97757]">
            <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 2v20M2 12h20M4.93 4.93l14.14 14.14M19.07 4.93L4.93 19.07" />
            </svg>
          </div>
          <span class="text-sm font-medium tracking-tight text-[#f0efea]">9Router</span>
          <span class="text-xs text-[#6e6c65]">/</span>
          <span class="text-xs text-[#a3a198]">Developer Workbench</span>
        </div>
        <div class="text-xs text-[#6e6c65] font-mono">
          v1.9 &bull; TLS Direct
        </div>
      </header>

      <!-- Centered Editorial Login Card -->
      <main class="w-full max-w-[400px] mx-auto my-12" bind:this={loginCardRef}>
        <div class="mb-8 text-center">
          <h1
            class="text-3xl sm:text-4xl font-normal tracking-tight text-[#f0efea] mb-2.5"
            style="font-family: 'Instrument Serif', Georgia, serif;"
          >
            Welcome to the Workbench
          </h1>
          <p class="text-[13px] text-[#a3a198] leading-relaxed">
            Enter your client bearer key to inspect live token telemetry, test models in the workbench, and configure your IDE.
          </p>
        </div>

        <div class="p-6 rounded-xl bg-[#1c1b1a] border border-[#2a2926] shadow-[0_12px_32px_rgba(0,0,0,0.35)]">
          {#if errorMsg}
            <div class="mb-4 p-3 rounded-lg bg-[#d16b6b]/10 border border-[#d16b6b]/25 text-xs text-[#e59898] flex items-start gap-2.5">
              <AlertTriangle class="w-4 h-4 text-[#d16b6b] shrink-0 mt-0.5" />
              <span>{errorMsg}</span>
            </div>
          {/if}

          <form
            onsubmit={(e) => {
              e.preventDefault()
              void loadPortalInfo()
            }}
            class="space-y-4"
          >
            <div>
              <label for="portal-api-key" class="block text-xs font-medium text-[#a3a198] mb-1.5">
                API Key
              </label>
              <div class="relative">
                <input
                  id="portal-api-key"
                  type={showKey ? 'text' : 'password'}
                  bind:value={inputKey}
                  placeholder="sk-..."
                  autocomplete="off"
                  class="w-full pl-3.5 pr-10 py-2.5 bg-[#141413] border border-[#2e2d2a] focus:border-[#d97757] rounded-lg text-xs font-mono text-[#f0efea] focus:outline-none transition placeholder:text-[#57554f]"
                />
                <button
                  type="button"
                  onclick={() => (showKey = !showKey)}
                  class="absolute right-3 top-2.5 text-[#6e6c65] hover:text-[#f0efea] transition cursor-pointer"
                  aria-label="Toggle key visibility"
                >
                  {#if showKey}
                    <EyeOff class="w-4 h-4" />
                  {:else}
                    <Eye class="w-4 h-4" />
                  {/if}
                </button>
              </div>
            </div>

            <button
              type="submit"
              disabled={loading || !inputKey.trim()}
              class="w-full py-2.5 px-4 rounded-lg bg-[#d97757] hover:bg-[#e28769] text-[#141413] font-medium text-xs transition flex items-center justify-center gap-2 disabled:opacity-40 cursor-pointer"
            >
              {#if loading}
                <RefreshCw class="w-3.5 h-3.5 animate-spin" />
                <span>Authenticating...</span>
              {:else}
                <span>Continue to Workbench</span>
                <ArrowUpRight class="w-3.5 h-3.5" />
              {/if}
            </button>
          </form>
        </div>

        <p class="mt-6 text-center text-[11px] text-[#6e6c65]">
          Protected by anti-leak model remasking &bull; Zero-retention gateway
        </p>
      </main>

      <footer class="w-full max-w-6xl mx-auto flex items-center justify-between text-[11px] text-[#6e6c65] border-t border-[#22211e] pt-4">
        <span>9Router Client Telemetry Portal</span>
        <span class="font-mono">OpenAI &amp; Anthropic Compatible</span>
      </footer>
    </div>
  {:else}
    <!-- =========================================================================
         AUTHENTICATED STATE: ANTHROPIC CONSOLE WORKBENCH SHELL
         ========================================================================= -->
    <div class="min-h-screen flex flex-col lg:flex-row">
      <!-- LEFT SIDEBAR (CLAUDE CONSOLE / LINEAR STYLE) -->
      <aside class="w-full lg:w-64 lg:min-h-screen bg-[#191817] border-b lg:border-b-0 lg:border-r border-[#2a2926] flex flex-col justify-between shrink-0">
        <div class="p-4 sm:p-5 space-y-6">
          <!-- Brand Header -->
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2.5">
              <div class="w-6 h-6 rounded-md bg-[#d97757]/15 border border-[#d97757]/35 flex items-center justify-center text-[#d97757]">
                <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
                  <path d="M12 2v20M2 12h20M4.93 4.93l14.14 14.14M19.07 4.93L4.93 19.07" />
                </svg>
              </div>
              <div>
                <div class="text-[13px] font-semibold tracking-tight text-[#f0efea] leading-none">
                  9Router
                </div>
                <div class="text-[11px] text-[#7c7a72] mt-0.5">
                  Console Workbench
                </div>
              </div>
            </div>

            <span class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-[10px] font-mono bg-[#232220] text-[#a3a198] border border-[#2e2d2a]">
              <span class="w-1.5 h-1.5 rounded-full bg-[#78b88c]"></span>
              {pingLatencyMs}ms
            </span>
          </div>

          <!-- Active Credential Switcher Card -->
          <div class="p-3 rounded-lg bg-[#141413] border border-[#2a2926] space-y-2">
            <div class="flex items-center justify-between">
              <span class="text-[11px] text-[#7c7a72]">Active Key</span>
              <button
                type="button"
                onclick={() => copyText(clientInfo?.key || '', 'sidebar-key')}
                class="text-[11px] text-[#d97757] hover:text-[#e28769] flex items-center gap-1 transition cursor-pointer"
                title="Copy full API Key"
              >
                {#if copiedId === 'sidebar-key'}
                  <Check class="w-3 h-3" />
                  <span>Copied</span>
                {:else}
                  <Copy class="w-3 h-3" />
                  <span>Copy</span>
                {/if}
              </button>
            </div>
            <div class="text-xs font-medium text-[#f0efea] truncate">
              {clientInfo.name || 'Default Client'}
            </div>
            <div class="text-[11px] font-mono text-[#7c7a72] truncate">
              {clientInfo.keyMasked}
            </div>
          </div>

          <!-- Navigation Menu -->
          <nav class="flex lg:flex-col gap-1 overflow-x-auto pb-1 lg:pb-0">
            <button
              type="button"
              onclick={() => switchTab('telemetry')}
              class="flex items-center justify-between px-3 py-2 rounded-lg text-xs transition cursor-pointer shrink-0 {activeMainTab === 'telemetry' ? 'bg-[#262522] text-[#f0efea] font-medium' : 'text-[#9c9a92] hover:text-[#f0efea] hover:bg-[#1f1e1c]'}"
            >
              <span class="flex items-center gap-2.5">
                <Activity class="w-3.5 h-3.5 {activeMainTab === 'telemetry' ? 'text-[#d97757]' : 'text-[#7c7a72]'}" />
                <span>Usage &amp; Telemetry</span>
              </span>
              {#if clientInfo.recentRequests && clientInfo.recentRequests.length > 0}
                <span class="hidden lg:inline-block font-mono text-[10px] text-[#7c7a72]">
                  {clientInfo.recentRequests.length}
                </span>
              {/if}
            </button>

            <button
              type="button"
              onclick={() => switchTab('playground')}
              class="flex items-center justify-between px-3 py-2 rounded-lg text-xs transition cursor-pointer shrink-0 {activeMainTab === 'playground' ? 'bg-[#262522] text-[#f0efea] font-medium' : 'text-[#9c9a92] hover:text-[#f0efea] hover:bg-[#1f1e1c]'}"
            >
              <span class="flex items-center gap-2.5">
                <Terminal class="w-3.5 h-3.5 {activeMainTab === 'playground' ? 'text-[#d97757]' : 'text-[#7c7a72]'}" />
                <span>Prompt Workbench</span>
              </span>
              <span class="hidden lg:inline-block w-1.5 h-1.5 rounded-full bg-[#d97757]"></span>
            </button>

            <button
              type="button"
              onclick={() => switchTab('guides')}
              class="flex items-center justify-between px-3 py-2 rounded-lg text-xs transition cursor-pointer shrink-0 {activeMainTab === 'guides' ? 'bg-[#262522] text-[#f0efea] font-medium' : 'text-[#9c9a92] hover:text-[#f0efea] hover:bg-[#1f1e1c]'}"
            >
              <span class="flex items-center gap-2.5">
                <BookOpen class="w-3.5 h-3.5 {activeMainTab === 'guides' ? 'text-[#d97757]' : 'text-[#7c7a72]'}" />
                <span>IDE &amp; SDK Setup</span>
              </span>
            </button>

            <button
              type="button"
              onclick={() => switchTab('tariffs')}
              class="flex items-center justify-between px-3 py-2 rounded-lg text-xs transition cursor-pointer shrink-0 {activeMainTab === 'tariffs' ? 'bg-[#262522] text-[#f0efea] font-medium' : 'text-[#9c9a92] hover:text-[#f0efea] hover:bg-[#1f1e1c]'}"
            >
              <span class="flex items-center gap-2.5">
                <Layers class="w-3.5 h-3.5 {activeMainTab === 'tariffs' ? 'text-[#d97757]' : 'text-[#7c7a72]'}" />
                <span>Model Rate Sheet</span>
              </span>
              {#if clientInfo.pricingTable && clientInfo.pricingTable.length > 0}
                <span class="hidden lg:inline-block font-mono text-[10px] text-[#7c7a72]">
                  {clientInfo.pricingTable.length}
                </span>
              {/if}
            </button>
          </nav>
        </div>

        <!-- Sidebar Bottom Controls -->
        <div class="hidden lg:block p-4 border-t border-[#2a2926] space-y-3">
          <div class="flex items-center justify-between text-[11px] text-[#7c7a72]">
            <span>Endpoint</span>
            <button
              type="button"
              onclick={() => copyText(clientInfo?.configs.baseUrl || '', 'sidebar-url')}
              class="font-mono text-[#a3a198] hover:text-[#f0efea] truncate max-w-[140px] cursor-pointer"
              title="Click to copy Base URL"
            >
              {copiedId === 'sidebar-url' ? 'Copied URL' : clientInfo.configs.baseUrl.replace(/^https?:\/\//, '')}
            </button>
          </div>

          <div class="flex items-center justify-between pt-1">
            <button
              type="button"
              onclick={() => (autoRefreshEnabled = !autoRefreshEnabled)}
              class="inline-flex items-center gap-1.5 text-[11px] text-[#9c9a92] hover:text-[#f0efea] cursor-pointer"
            >
              <span class="w-1.5 h-1.5 rounded-full {autoRefreshEnabled ? 'bg-[#78b88c]' : 'bg-[#6e6c65]'}"></span>
              <span>{autoRefreshEnabled ? 'Auto-sync 5s' : 'Sync paused'}</span>
            </button>

            <button
              type="button"
              onclick={handleLogout}
              class="inline-flex items-center gap-1 text-[11px] text-[#7c7a72] hover:text-[#d16b6b] transition cursor-pointer"
            >
              <LogOut class="w-3 h-3" />
              <span>Sign out</span>
            </button>
          </div>
        </div>
      </aside>

      <!-- MAIN WORKSPACE CANVAS -->
      <div class="flex-1 flex flex-col min-w-0" bind:this={workspaceRef}>
        <!-- Top Context Header Bar -->
        <header class="h-14 px-6 border-b border-[#2a2926] flex items-center justify-between bg-[#141413]/90 backdrop-blur-sm sticky top-0 z-20">
          <div class="flex items-center gap-3 min-w-0">
            <h2
              class="text-xl font-normal text-[#f0efea] truncate"
              style="font-family: 'Instrument Serif', Georgia, serif;"
            >
              {#if activeMainTab === 'telemetry'}
                Usage &amp; Live Telemetry
              {:else if activeMainTab === 'playground'}
                Prompt Workbench
              {:else if activeMainTab === 'guides'}
                Client &amp; IDE Integration
              {:else}
                Model Rate &amp; Tariff Sheet
              {/if}
            </h2>
            {#if lastSyncTime}
              <span class="hidden sm:inline-block text-[11px] text-[#6e6c65] font-mono">
                Updated {lastSyncTime}
              </span>
            {/if}
          </div>

          <div class="flex items-center gap-2">
            <button
              type="button"
              onclick={() => refreshTelemetry(true)}
              disabled={refreshing}
              class="inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-md bg-[#1c1b1a] hover:bg-[#262522] border border-[#2a2926] text-xs text-[#a3a198] hover:text-[#f0efea] transition cursor-pointer disabled:opacity-50"
              title="Refresh telemetry"
            >
              <RefreshCw class="w-3 h-3 {refreshing ? 'animate-spin text-[#d97757]' : ''}" />
              <span class="hidden sm:inline">Refresh</span>
            </button>

            <button
              type="button"
              onclick={() => copyText(clientInfo?.key || '', 'header-key')}
              class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md bg-[#1c1b1a] hover:bg-[#262522] border border-[#2a2926] text-xs text-[#f0efea] transition cursor-pointer"
            >
              {#if copiedId === 'header-key'}
                <Check class="w-3 h-3 text-[#78b88c]" />
                <span>Copied Key</span>
              {:else}
                <Key class="w-3 h-3 text-[#d97757]" />
                <span>Copy Key</span>
              {/if}
            </button>
          </div>
        </header>

        <!-- Main Body Content -->
        <main class="flex-1 p-6 sm:p-8 max-w-7xl w-full mx-auto space-y-8">
          {#if activeMainTab === 'telemetry'}
            <!-- ===============================================================
                 UNIFIED ARCHITECTURAL METRIC LEDGER (1PX HAIRLINE GRID)
                 =============================================================== -->
            <section class="gsap-panel rounded-xl bg-[#191817] border border-[#2a2926] overflow-hidden">
              <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 divide-y sm:divide-y-0 sm:divide-x divide-[#2a2926]">
                <!-- Metric 1: Prepaid Balance -->
                <div class="p-5 sm:p-6 flex flex-col justify-between space-y-4">
                  <div class="flex items-center justify-between">
                    <span class="text-xs text-[#9c9a92]">Available balance</span>
                    <span class="text-[11px] font-mono text-[#6e6c65]">IDR</span>
                  </div>

                  <div>
                    {#if clientInfo.balance < 0}
                      <div
                        class="text-3xl font-normal tracking-tight text-[#f0efea]"
                        style="font-family: 'Instrument Serif', Georgia, serif;"
                      >
                        Unlimited Quota
                      </div>
                    {:else}
                      <div
                        class="text-3xl font-normal tracking-tight text-[#f0efea] tabular-nums"
                        style="font-family: 'Instrument Serif', Georgia, serif;"
                      >
                        Rp {animatedBalance.toLocaleString('id-ID')}
                      </div>
                    {/if}
                  </div>

                  <div class="flex items-center justify-between text-[11px] text-[#7c7a72] pt-1 border-t border-[#22211e]">
                    <span>Billing mode</span>
                    <span class="text-[#a3a198]">Pay-per-1k tokens</span>
                  </div>
                </div>

                <!-- Metric 2: API Calls & Throughput -->
                <div class="p-5 sm:p-6 flex flex-col justify-between space-y-4">
                  <div class="flex items-center justify-between">
                    <span class="text-xs text-[#9c9a92]">Total requests</span>
                    <span class="inline-flex items-center gap-1 text-[11px] text-[#78b88c]">
                      <span class="w-1.5 h-1.5 rounded-full bg-[#78b88c]"></span>
                      100% OK
                    </span>
                  </div>

                  <div class="flex items-baseline justify-between gap-4">
                    <div
                      class="text-3xl font-normal tracking-tight text-[#f0efea] tabular-nums"
                      style="font-family: 'Instrument Serif', Georgia, serif;"
                    >
                      {(clientInfo.stats?.totalRequests ?? 0).toLocaleString()}
                    </div>

                    <!-- Mini inline sparkbars -->
                    {#if chartBars.length > 0}
                      <div class="flex items-end gap-1 h-7">
                        {#each chartBars.slice(-10) as bar}
                          <div
                            class="w-1.5 rounded-t-sm bg-[#d97757]/75 hover:bg-[#d97757] transition-all"
                            style="height: {Math.max(15, bar.heightPct)}%;"
                            title="{bar.model}: {bar.tot.toLocaleString()} tokens"
                          ></div>
                        {/each}
                      </div>
                    {/if}
                  </div>

                  <div class="flex items-center justify-between text-[11px] text-[#7c7a72] pt-1 border-t border-[#22211e]">
                    <span>Cumulative spend</span>
                    <span class="font-mono text-[#a3a198]">
                      Rp {(clientInfo.stats?.cost ?? 0).toLocaleString('id-ID', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
                    </span>
                  </div>
                </div>

                <!-- Metric 3: Token Consumption & Split Ratio -->
                <div class="p-5 sm:p-6 flex flex-col justify-between space-y-4">
                  <div class="flex items-center justify-between">
                    <span class="text-xs text-[#9c9a92]">Token volume</span>
                    <span class="text-[11px] font-mono text-[#a3a198]">
                      {promptSharePct.toFixed(0)}% in / {completionSharePct.toFixed(0)}% out
                    </span>
                  </div>

                  <div class="space-y-2.5">
                    <div
                      class="text-3xl font-normal tracking-tight text-[#f0efea] tabular-nums"
                      style="font-family: 'Instrument Serif', Georgia, serif;"
                    >
                      {grandTotalTokens.toLocaleString()}
                    </div>

                    <!-- Proportional Split Bar (Terracotta vs Stone) -->
                    <div class="w-full h-1.5 rounded-full bg-[#262522] overflow-hidden flex">
                      <div
                        class="h-full bg-[#d97757] transition-all duration-500"
                        style="width: {promptSharePct}%;"
                      ></div>
                      <div
                        class="h-full bg-[#7c7a72] transition-all duration-500"
                        style="width: {completionSharePct}%;"
                      ></div>
                    </div>
                  </div>

                  <div class="flex items-center justify-between text-[11px] font-mono text-[#7c7a72] pt-1 border-t border-[#22211e]">
                    <span class="flex items-center gap-1.5">
                      <span class="w-1.5 h-1.5 rounded-xs bg-[#d97757]"></span>
                      In: {totalPromptTokens.toLocaleString()}
                    </span>
                    <span class="flex items-center gap-1.5">
                      <span class="w-1.5 h-1.5 rounded-xs bg-[#7c7a72]"></span>
                      Out: {totalCompletionTokens.toLocaleString()}
                    </span>
                  </div>
                </div>

                <!-- Metric 4: Gateway Policy & Model Routing -->
                <div class="p-5 sm:p-6 flex flex-col justify-between space-y-4">
                  <div class="flex items-center justify-between">
                    <span class="text-xs text-[#9c9a92]">Gateway policy</span>
                    <span class="text-[11px] font-mono text-[#a3a198]">
                      {clientInfo.rateLimitRpm > 0 ? `${clientInfo.rateLimitRpm} RPM` : 'Unthrottled'}
                    </span>
                  </div>

                  <div class="flex flex-wrap items-center gap-1.5">
                    {#each clientInfo.allowedModels.slice(0, 4) as model}
                      <button
                        type="button"
                        onclick={() => {
                          testModel = model === '*' ? 'gpt-4o' : model
                          switchTab('playground')
                        }}
                        class="px-2.5 py-1 rounded-md bg-[#141413] hover:bg-[#262522] border border-[#2e2d2a] hover:border-[#d97757]/50 text-xs font-mono text-[#f0efea] transition cursor-pointer flex items-center gap-1.5"
                        title="Open {model} in Prompt Workbench"
                      >
                        <span class="truncate max-w-[150px]">{model}</span>
                        <ArrowUpRight class="w-3 h-3 text-[#d97757]" />
                      </button>
                    {/each}
                  </div>

                  <div class="flex items-center justify-between text-[11px] text-[#7c7a72] pt-1 border-t border-[#22211e]">
                    <span>Concurrency slots</span>
                    <span class="font-mono text-[#a3a198]">
                      {clientInfo.concurrency > 0 ? `${clientInfo.concurrency} parallel` : 'Unlimited'}
                    </span>
                  </div>
                </div>
              </div>
            </section>

            <!-- ===============================================================
                 TOKEN DISTRIBUTION TIMELINE (VISUAL TELEMETRY BAR CHART)
                 =============================================================== -->
            {#if chartBars.length > 1}
              <section class="gsap-panel p-5 sm:p-6 rounded-xl bg-[#191817] border border-[#2a2926] space-y-4">
                <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
                  <div>
                    <h3 class="text-xs font-medium text-[#f0efea]">
                      Recent Inference Token Profile
                    </h3>
                    <p class="text-[11px] text-[#7c7a72] mt-0.5">
                      Token distribution across your last {chartBars.length} requests (chronological left-to-right)
                    </p>
                  </div>
                  <div class="flex items-center gap-4 text-[11px] text-[#9c9a92]">
                    <span class="flex items-center gap-1.5">
                      <span class="w-2 h-2 rounded-xs bg-[#d97757]"></span>
                      Prompt context
                    </span>
                    <span class="flex items-center gap-1.5">
                      <span class="w-2 h-2 rounded-xs bg-[#5c5a53]"></span>
                      Completion output
                    </span>
                  </div>
                </div>

                <div class="h-28 pt-4 flex items-end gap-2 border-b border-[#2a2926] pb-2">
                  {#each chartBars as bar}
                    <button
                      type="button"
                      onclick={() => (selectedLogId = selectedLogId === bar.id ? null : bar.id)}
                      class="flex-1 h-full flex flex-col justify-end group cursor-pointer focus:outline-none"
                      title="{formatTimestamp(bar.timestamp)} — {bar.model} ({bar.tot.toLocaleString()} tokens)"
                    >
                      <div
                        class="w-full rounded-t transition-all duration-300 flex flex-col justify-end overflow-hidden border border-transparent group-hover:border-[#f0efea]/30"
                        style="height: {bar.heightPct}%;"
                      >
                        <div
                          class="w-full bg-[#5c5a53] group-hover:bg-[#7c7a72] transition-colors"
                          style="height: {100 - bar.promptPct}%;"
                        ></div>
                        <div
                          class="w-full bg-[#d97757]/85 group-hover:bg-[#d97757] transition-colors"
                          style="height: {bar.promptPct}%;"
                        ></div>
                      </div>
                    </button>
                  {/each}
                </div>
              </section>
            {/if}

            <!-- ===============================================================
                 REQUEST LOG STREAM TABLE (CLEAN LINEAR / ANTHROPIC LOGS)
                 =============================================================== -->
            <section class="gsap-panel rounded-xl bg-[#191817] border border-[#2a2926] overflow-hidden">
              <div class="px-5 py-4 border-b border-[#2a2926] flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                <div>
                  <h3 class="text-xs font-medium text-[#f0efea]">
                    Request Log Stream
                  </h3>
                  <p class="text-[11px] text-[#7c7a72] mt-0.5">
                    Real-time ledger of masked model calls routed through your key
                  </p>
                </div>

                <!-- Search Filter -->
                <div class="relative w-full sm:w-64">
                  <Search class="w-3.5 h-3.5 text-[#6e6c65] absolute left-3 top-2.5" />
                  <input
                    type="text"
                    bind:value={logSearchQuery}
                    placeholder="Filter by model or status..."
                    class="w-full pl-8 pr-3 py-1.5 bg-[#141413] border border-[#2a2926] focus:border-[#d97757] rounded-lg text-xs text-[#f0efea] placeholder:text-[#5c5a53] focus:outline-none transition"
                  />
                </div>
              </div>

              {#if !clientInfo.recentRequests || clientInfo.recentRequests.length === 0}
                <div class="p-12 text-center space-y-3">
                  <p class="text-sm text-[#a3a198]">No inference requests recorded yet</p>
                  <p class="text-xs text-[#6e6c65] max-w-sm mx-auto">
                    Send your first chat completion from Cursor, Cline, or test your key directly in the Prompt Workbench.
                  </p>
                  <button
                    type="button"
                    onclick={() => switchTab('playground')}
                    class="mt-2 inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg bg-[#d97757] hover:bg-[#e28769] text-[#141413] font-medium text-xs transition cursor-pointer"
                  >
                    <Terminal class="w-3.5 h-3.5" />
                    <span>Open Prompt Workbench</span>
                  </button>
                </div>
              {:else if filteredRequests.length === 0}
                <div class="p-8 text-center text-xs text-[#7c7a72]">
                  No requests matching "{logSearchQuery}"
                </div>
              {:else}
                <div class="overflow-x-auto">
                  <table class="w-full text-left border-collapse">
                    <thead>
                      <tr class="border-b border-[#2a2926] text-[11px] text-[#7c7a72] bg-[#161514]">
                        <th class="py-2.5 px-5 font-normal">Timestamp</th>
                        <th class="py-2.5 px-5 font-normal">Routed Model</th>
                        <th class="py-2.5 px-5 font-normal text-right">Prompt</th>
                        <th class="py-2.5 px-5 font-normal text-right">Completion</th>
                        <th class="py-2.5 px-5 font-normal text-right">Total</th>
                        <th class="py-2.5 px-5 font-normal text-right">Cost</th>
                        <th class="py-2.5 px-5 font-normal text-right">Status</th>
                      </tr>
                    </thead>
                    <tbody class="divide-y divide-[#22211e] text-xs">
                      {#each filteredRequests as req (req.id || req.timestamp + req.promptTokens)}
                        {@const rowTotal = req.totalTokens || (req.promptTokens + req.completionTokens)}
                        {@const isOk = req.status === 'success' || req.status === 'ok'}
                        <tr
                          onclick={() => (selectedLogId = selectedLogId === req.id ? null : req.id)}
                          class="hover:bg-[#1f1e1c] transition-colors cursor-pointer {selectedLogId === req.id ? 'bg-[#232220]' : ''}"
                        >
                          <td class="py-3 px-5 font-mono text-[11px] text-[#7c7a72] whitespace-nowrap">
                            {formatTimestamp(req.timestamp)}
                          </td>
                          <td class="py-3 px-5">
                            <div class="flex items-center gap-2">
                              <span class="font-mono text-xs text-[#f0efea]">
                                {req.model}
                              </span>
                            </div>
                          </td>
                          <td class="py-3 px-5 text-right font-mono text-[11px] text-[#a3a198] tabular-nums">
                            {req.promptTokens.toLocaleString()}
                          </td>
                          <td class="py-3 px-5 text-right font-mono text-[11px] text-[#a3a198] tabular-nums">
                            {req.completionTokens.toLocaleString()}
                          </td>
                          <td class="py-3 px-5 text-right font-mono text-[11px] text-[#f0efea] tabular-nums">
                            {rowTotal.toLocaleString()}
                          </td>
                          <td class="py-3 px-5 text-right font-mono text-[11px] text-[#a3a198] tabular-nums">
                            Rp {req.cost.toFixed(2)}
                          </td>
                          <td class="py-3 px-5 text-right whitespace-nowrap">
                            <span class="inline-flex items-center justify-end gap-1.5 font-mono text-[11px] {isOk ? 'text-[#a3a198]' : 'text-[#d16b6b]'}">
                              <span class="w-1.5 h-1.5 rounded-full {isOk ? 'bg-[#78b88c]' : 'bg-[#d16b6b]'}"></span>
                              <span>{isOk ? '200' : 'ERR'}</span>
                            </span>
                          </td>
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                </div>
              {/if}
            </section>
          {:else if activeMainTab === 'playground'}
            <!-- ===============================================================
                 TAB 2: ANTHROPIC CLAUDE WORKBENCH PLAYGROUND
                 =============================================================== -->
            <section class="gsap-panel rounded-xl bg-[#191817] border border-[#2a2926] overflow-hidden flex flex-col">
              <!-- Workbench Top Control Bar -->
              <div class="px-5 py-3.5 border-b border-[#2a2926] bg-[#161514] flex flex-wrap items-center justify-between gap-3">
                <div class="flex flex-wrap items-center gap-3">
                  <div class="flex items-center gap-2">
                    <span class="text-xs text-[#7c7a72]">Model</span>
                    <select
                      bind:value={testModel}
                      class="px-3 py-1.5 bg-[#141413] border border-[#2e2d2a] focus:border-[#d97757] rounded-lg text-xs font-mono text-[#f0efea] focus:outline-none cursor-pointer"
                    >
                      {#each clientInfo.allowedModels as m}
                        <option value={m === '*' ? 'gpt-4o' : m}>{m}</option>
                      {/each}
                    </select>
                  </div>

                  <button
                    type="button"
                    onclick={() => (showSystemPrompt = !showSystemPrompt)}
                    class="px-2.5 py-1.5 rounded-lg border text-xs transition cursor-pointer flex items-center gap-1.5 {showSystemPrompt ? 'bg-[#262522] border-[#3a3935] text-[#f0efea]' : 'bg-[#141413] border-[#2a2926] text-[#7c7a72] hover:text-[#a3a198]'}"
                  >
                    <SlidersHorizontal class="w-3 h-3" />
                    <span>System Prompt</span>
                  </button>
                </div>

                <div class="flex items-center gap-3">
                  {#if testLatencyMs > 0}
                    <span class="text-[11px] font-mono text-[#9c9a92]">
                      TTFT: <strong class="text-[#f0efea]">{testLatencyMs}ms</strong>
                    </span>
                  {/if}
                  {#if testTokensUsed > 0}
                    <span class="text-[11px] font-mono text-[#9c9a92]">
                      Tokens: <strong class="text-[#f0efea]">{testTokensUsed}</strong>
                    </span>
                  {/if}

                  <button
                    type="button"
                    onclick={runPlaygroundTest}
                    disabled={testRunning || !testPrompt.trim()}
                    class="px-4 py-1.5 rounded-lg bg-[#d97757] hover:bg-[#e28769] text-[#141413] font-medium text-xs transition flex items-center gap-2 disabled:opacity-40 cursor-pointer"
                  >
                    {#if testRunning}
                      <RefreshCw class="w-3.5 h-3.5 animate-spin" />
                      <span>Running...</span>
                    {:else}
                      <Play class="w-3.5 h-3.5 fill-current" />
                      <span>Run</span>
                      <span class="text-[10px] opacity-75 font-mono hidden sm:inline">Ctrl+↵</span>
                    {/if}
                  </button>
                </div>
              </div>

              <!-- Split Workbench Area -->
              <div class="grid grid-cols-1 lg:grid-cols-2 divide-y lg:divide-y-0 lg:divide-x divide-[#2a2926]">
                <!-- Left Pane: Prompt Composer -->
                <div class="p-5 space-y-4">
                  {#if showSystemPrompt}
                    <div class="space-y-1.5">
                      <label for="wb-system-prompt" class="block text-[11px] font-medium text-[#7c7a72]">
                        System Prompt
                      </label>
                      <textarea
                        id="wb-system-prompt"
                        bind:value={testSystemPrompt}
                        rows="2"
                        placeholder="Optional system instructions..."
                        class="w-full p-3 bg-[#141413] border border-[#2a2926] focus:border-[#d97757] rounded-lg text-xs text-[#d6d4ce] font-mono leading-relaxed focus:outline-none resize-y"
                      ></textarea>
                    </div>
                  {/if}

                  <div class="space-y-1.5">
                    <div class="flex items-center justify-between">
                      <label for="wb-user-prompt" class="block text-[11px] font-medium text-[#7c7a72]">
                        User Message
                      </label>
                      <div class="flex items-center gap-1.5">
                        <button
                          type="button"
                          onclick={() => (testPrompt = 'Say hello and summarize your architecture in 2 concise bullet points.')}
                          class="px-2 py-0.5 rounded bg-[#1f1e1c] hover:bg-[#262522] border border-[#2a2926] text-[11px] text-[#9c9a92] hover:text-[#f0efea] transition cursor-pointer"
                        >
                          Ping
                        </button>
                        <button
                          type="button"
                          onclick={() => (testPrompt = 'Write a fast Go function to find prime numbers using Sieve of Eratosthenes.')}
                          class="px-2 py-0.5 rounded bg-[#1f1e1c] hover:bg-[#262522] border border-[#2a2926] text-[11px] text-[#9c9a92] hover:text-[#f0efea] transition cursor-pointer"
                        >
                          Go Sieve
                        </button>
                        <button
                          type="button"
                          onclick={() => (testPrompt = 'Give me 3 critical kernel hardening sysctl rules for a production Linux server.')}
                          class="px-2 py-0.5 rounded bg-[#1f1e1c] hover:bg-[#262522] border border-[#2a2926] text-[11px] text-[#9c9a92] hover:text-[#f0efea] transition cursor-pointer"
                        >
                          Linux Sysctl
                        </button>
                      </div>
                    </div>

                    <textarea
                      id="wb-user-prompt"
                      bind:value={testPrompt}
                      rows="10"
                      placeholder="Enter a user message to test streaming response..."
                      class="w-full p-3.5 bg-[#141413] border border-[#2a2926] focus:border-[#d97757] rounded-lg text-xs text-[#f0efea] font-mono leading-relaxed focus:outline-none resize-y"
                    ></textarea>
                  </div>
                </div>

                <!-- Right Pane: Editorial Output Canvas -->
                <div class="p-5 flex flex-col justify-between bg-[#161514]/50 min-h-[320px]">
                  <div class="space-y-3">
                    <div class="flex items-center justify-between">
                      <span class="text-[11px] font-medium text-[#7c7a72]">Assistant Output</span>
                      {#if testOutput}
                        <button
                          type="button"
                          onclick={() => copyText(testOutput, 'test-output')}
                          class="text-[11px] text-[#9c9a92] hover:text-[#f0efea] flex items-center gap-1 transition cursor-pointer"
                        >
                          {#if copiedId === 'test-output'}
                            <Check class="w-3 h-3 text-[#78b88c]" />
                            <span>Copied</span>
                          {:else}
                            <Copy class="w-3 h-3" />
                            <span>Copy</span>
                          {/if}
                        </button>
                      {/if}
                    </div>

                    {#if testError}
                      <div class="p-3.5 rounded-lg bg-[#d16b6b]/10 border border-[#d16b6b]/25 text-xs text-[#e59898] font-mono">
                        {testError}
                      </div>
                    {:else if testOutput || testRunning}
                      <div class="p-4 rounded-lg bg-[#141413] border border-[#2a2926] font-mono text-xs text-[#f0efea] leading-relaxed whitespace-pre-wrap max-h-[420px] overflow-y-auto">
                        {testOutput}{#if testRunning}<span class="inline-block w-1.5 h-3.5 bg-[#d97757] animate-pulse ml-0.5 align-middle"></span>{/if}
                      </div>
                    {:else}
                      <div class="h-64 rounded-lg border border-dashed border-[#2a2926] flex flex-col items-center justify-center text-center p-6">
                        <p class="text-xs text-[#7c7a72]">
                          Click <strong class="text-[#a3a198]">Run</strong> to stream a live response from <span class="font-mono text-[#d97757]">{testModel || 'your model'}</span>.
                        </p>
                      </div>
                    {/if}
                  </div>
                </div>
              </div>
            </section>
          {:else if activeMainTab === 'guides'}
            <!-- ===============================================================
                 TAB 3: IDE & SDK INTEGRATION GUIDES
                 =============================================================== -->
            <section class="gsap-panel rounded-xl bg-[#191817] border border-[#2a2926] overflow-hidden">
              <div class="px-5 py-3.5 border-b border-[#2a2926] bg-[#161514] flex flex-wrap items-center justify-between gap-3">
                <div class="flex flex-wrap items-center gap-1">
                  {#each [
                    { id: 'cursor', label: 'Cursor IDE' },
                    { id: 'cline', label: 'Cline / Roo Code' },
                    { id: 'windsurf', label: 'Windsurf' },
                    { id: 'python', label: 'Python SDK' },
                    { id: 'curl', label: 'cURL CLI' }
                  ] as tab}
                    <button
                      type="button"
                      onclick={() => (activeGuideTab = tab.id as any)}
                      class="px-3 py-1.5 rounded-md text-xs transition cursor-pointer {activeGuideTab === tab.id ? 'bg-[#262522] text-[#f0efea] font-medium' : 'text-[#7c7a72] hover:text-[#a3a198]'}"
                    >
                      {tab.label}
                    </button>
                  {/each}
                </div>
              </div>

              <div class="p-6 space-y-5 text-xs">
                {#if activeGuideTab === 'cursor'}
                  <div class="space-y-4 max-w-2xl">
                    <p class="text-[#a3a198] leading-relaxed">
                      In Cursor IDE, open <strong class="text-[#f0efea]">Settings &rarr; Models &rarr; OpenAI API Key</strong> and paste the following endpoints:
                    </p>

                    <div class="space-y-3">
                      <div class="p-4 rounded-lg bg-[#141413] border border-[#2a2926] flex items-center justify-between gap-4">
                        <div class="min-w-0">
                          <div class="text-[11px] text-[#7c7a72] mb-1">Override OpenAI Base URL</div>
                          <div class="font-mono text-xs text-[#f0efea] truncate select-all">{clientInfo.configs.baseUrl}</div>
                        </div>
                        <button
                          type="button"
                          onclick={() => copyText(clientInfo?.configs.baseUrl || '', 'cursor-url')}
                          class="px-3 py-1.5 rounded-md bg-[#1f1e1c] hover:bg-[#262522] border border-[#2e2d2a] text-xs text-[#f0efea] shrink-0 cursor-pointer flex items-center gap-1.5"
                        >
                          <Copy class="w-3 h-3 text-[#d97757]" />
                          <span>{copiedId === 'cursor-url' ? 'Copied' : 'Copy URL'}</span>
                        </button>
                      </div>

                      <div class="p-4 rounded-lg bg-[#141413] border border-[#2a2926] flex items-center justify-between gap-4">
                        <div class="min-w-0">
                          <div class="text-[11px] text-[#7c7a72] mb-1">OpenAI API Key</div>
                          <div class="font-mono text-xs text-[#f0efea] truncate select-all">{clientInfo.key}</div>
                        </div>
                        <button
                          type="button"
                          onclick={() => copyText(clientInfo?.key || '', 'cursor-key')}
                          class="px-3 py-1.5 rounded-md bg-[#1f1e1c] hover:bg-[#262522] border border-[#2e2d2a] text-xs text-[#f0efea] shrink-0 cursor-pointer flex items-center gap-1.5"
                        >
                          <Copy class="w-3 h-3 text-[#d97757]" />
                          <span>{copiedId === 'cursor-key' ? 'Copied' : 'Copy Key'}</span>
                        </button>
                      </div>
                    </div>
                  </div>
                {:else if activeGuideTab === 'cline'}
                  <div class="space-y-4 max-w-2xl">
                    <p class="text-[#a3a198] leading-relaxed">
                      In VS Code <strong class="text-[#f0efea]">Cline / Roo Code</strong> settings, choose <strong class="text-[#f0efea]">OpenAI Compatible</strong>:
                    </p>
                    <div class="p-4 rounded-lg bg-[#141413] border border-[#2a2926] space-y-2.5 font-mono text-xs">
                      <div class="flex justify-between"><span class="text-[#7c7a72]">API Provider:</span> <span class="text-[#f0efea]">OpenAI Compatible</span></div>
                      <div class="flex justify-between"><span class="text-[#7c7a72]">Base URL:</span> <span class="text-[#d97757]">{clientInfo.configs.baseUrl}</span></div>
                      <div class="flex justify-between"><span class="text-[#7c7a72]">API Key:</span> <span class="text-[#f0efea]">{clientInfo.key}</span></div>
                      <div class="flex justify-between"><span class="text-[#7c7a72]">Model ID:</span> <span class="text-[#f0efea]">{testModel || 'gpt-4o'}</span></div>
                    </div>
                  </div>
                {:else if activeGuideTab === 'windsurf'}
                  <div class="space-y-4 max-w-2xl">
                    <p class="text-[#a3a198] leading-relaxed">
                      In <strong class="text-[#f0efea]">Windsurf Cascade</strong> custom provider configuration:
                    </p>
                    <div class="p-4 rounded-lg bg-[#141413] border border-[#2a2926] space-y-2.5 font-mono text-xs">
                      <div class="flex justify-between"><span class="text-[#7c7a72]">Provider:</span> <span class="text-[#f0efea]">openai-compatible</span></div>
                      <div class="flex justify-between"><span class="text-[#7c7a72]">Endpoint:</span> <span class="text-[#d97757]">{clientInfo.configs.baseUrl}</span></div>
                      <div class="flex justify-between"><span class="text-[#7c7a72]">API Key:</span> <span class="text-[#f0efea]">{clientInfo.key}</span></div>
                    </div>
                  </div>
                {:else if activeGuideTab === 'python'}
                  <div class="space-y-3">
                    <div class="flex items-center justify-between">
                      <span class="text-[#9c9a92]">OpenAI Python SDK snippet</span>
                      <button
                        type="button"
                        onclick={() => copyText(clientInfo?.configs.pythonSdk || '', 'sdk-code')}
                        class="text-[#d97757] hover:underline flex items-center gap-1 cursor-pointer"
                      >
                        <Copy class="w-3 h-3" />
                        <span>{copiedId === 'sdk-code' ? 'Copied' : 'Copy snippet'}</span>
                      </button>
                    </div>
                    <pre class="p-4 rounded-lg bg-[#141413] border border-[#2a2926] text-[#f0efea] overflow-x-auto leading-relaxed font-mono text-xs">{clientInfo.configs.pythonSdk}</pre>
                  </div>
                {:else if activeGuideTab === 'curl'}
                  <div class="space-y-3">
                    <div class="flex items-center justify-between">
                      <span class="text-[#9c9a92]">Terminal cURL request</span>
                      <button
                        type="button"
                        onclick={() =>
                          copyText(
                            `curl -X POST "${clientInfo?.configs.baseUrl}/chat/completions" \\\n  -H "Content-Type: application/json" \\\n  -H "Authorization: Bearer ${clientInfo?.key}" \\\n  -d '{"model": "${testModel || 'gpt-4o'}", "messages": [{"role": "user", "content": "Hello!"}]}'`,
                            'curl-code'
                          )}
                        class="text-[#d97757] hover:underline flex items-center gap-1 cursor-pointer"
                      >
                        <Copy class="w-3 h-3" />
                        <span>{copiedId === 'curl-code' ? 'Copied' : 'Copy cURL'}</span>
                      </button>
                    </div>
                    <pre class="p-4 rounded-lg bg-[#141413] border border-[#2a2926] text-[#f0efea] overflow-x-auto leading-relaxed font-mono text-xs">curl -X POST "{clientInfo.configs.baseUrl}/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer {clientInfo.key}" \
  -d '&#123;"model": "{testModel || 'gpt-4o'}", "messages": [&#123;"role": "user", "content": "Hello!"&#125;]&#125;'</pre>
                  </div>
                {/if}
              </div>
            </section>
          {:else if activeMainTab === 'tariffs'}
            <!-- ===============================================================
                 TAB 4: MODEL TARIFF & RATE SHEET
                 =============================================================== -->
            <section class="gsap-panel rounded-xl bg-[#191817] border border-[#2a2926] overflow-hidden">
              <div class="px-5 py-4 border-b border-[#2a2926] flex items-center justify-between">
                <div>
                  <h3 class="text-xs font-medium text-[#f0efea]">Model Pricing Ledger</h3>
                  <p class="text-[11px] text-[#7c7a72] mt-0.5">
                    Standardized rate per 1,000 tokens for models accessible by your key
                  </p>
                </div>
                <span class="text-[11px] font-mono text-[#7c7a72]">Per 1K Tokens</span>
              </div>

              {#if !clientInfo.pricingTable || clientInfo.pricingTable.length === 0}
                <div class="p-10 text-center text-xs text-[#7c7a72]">
                  No custom tariff rules configured — standard pass-through rates apply.
                </div>
              {:else}
                <div class="overflow-x-auto">
                  <table class="w-full text-left border-collapse">
                    <thead>
                      <tr class="border-b border-[#2a2926] text-[11px] text-[#7c7a72] bg-[#161514]">
                        <th class="py-2.5 px-5 font-normal">Model Pattern</th>
                        <th class="py-2.5 px-5 font-normal text-right">Prompt Input (1K)</th>
                        <th class="py-2.5 px-5 font-normal text-right">Completion Output (1K)</th>
                        <th class="py-2.5 px-5 font-normal text-right">Est. Capacity (Your Balance)</th>
                      </tr>
                    </thead>
                    <tbody class="divide-y divide-[#22211e] text-xs">
                      {#each clientInfo.pricingTable as price}
                        {@const avg1k = (price.priceInput1k + price.priceOutput1k) / 2}
                        {@const estTokens = clientInfo.balance > 0 && avg1k > 0 ? Math.floor((clientInfo.balance / avg1k) * 1000) : -1}
                        <tr class="hover:bg-[#1f1e1c] transition-colors">
                          <td class="py-3 px-5 font-mono text-xs text-[#f0efea]">
                            {price.modelPattern}
                          </td>
                          <td class="py-3 px-5 text-right font-mono text-[11px] text-[#a3a198] tabular-nums">
                            Rp {price.priceInput1k.toLocaleString('id-ID')}
                          </td>
                          <td class="py-3 px-5 text-right font-mono text-[11px] text-[#a3a198] tabular-nums">
                            Rp {price.priceOutput1k.toLocaleString('id-ID')}
                          </td>
                          <td class="py-3 px-5 text-right font-mono text-[11px] text-[#d97757] tabular-nums">
                            {#if clientInfo.balance < 0 || avg1k === 0}
                              Unlimited
                            {:else if estTokens > 0}
                              ~{(estTokens / 1000).toFixed(0)}K tok
                            {:else}
                              —
                            {/if}
                          </td>
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                </div>
              {/if}
            </section>
          {/if}
        </main>
      </div>
    </div>
  {/if}
</div>
