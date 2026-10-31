<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import gsap from 'gsap'
  import GalaxyBackground from '../GalaxyBackground.svelte'
  import {
    Key,
    Copy,
    Check,
    Cpu,
    Zap,
    RefreshCw,
    Terminal,
    Activity,
    Play,
    AlertTriangle,
    ArrowUpRight,
    Eye,
    EyeOff,
    ShieldCheck,
    Search,
    BookOpen,
    LogOut,
    Layers,
    CheckCircle2,
    SlidersHorizontal,
    Lock,
    FileCheck2,
    Globe,
    Server,
    Scale,
    EyeOff as Blindfold,
    ExternalLink,
    X,
    Link2,
    CreditCard,
    QrCode,
    Sparkles,
    Send
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

  // Unauthenticated Landing Section Switcher
  let landingTab = $state<'signin' | 'privacy' | 'compliance'>('signin')

  // Authenticated Console Navigation Tabs
  let activeMainTab = $state<'telemetry' | 'playground' | 'guides' | 'tariffs' | 'trust'>('telemetry')
  let activeGuideTab = $state<'cursor' | 'cline' | 'windsurf' | 'python' | 'curl'>('cursor')

  // Top-Up Modal & Payment Flow State
  let showTopupModal = $state(false)
  let selectedTopupAmount = $state<number>(50000)
  let topupStep = $state<'packages' | 'qris' | 'success'>('packages')
  let qrisTimeRemaining = $state(600) // 10 minutes
  let qrisTimerInterval: ReturnType<typeof setInterval> | null = null
  let simulatedPaymentLoading = $state(false)

  const TOPUP_PACKAGES = [
    { amount: 25000, label: 'Rp 25.000', estTokens: '~500K - 1.2M Tokens', badge: 'Starter', popular: false },
    { amount: 50000, label: 'Rp 50.000', estTokens: '~1.5M - 3.2M Tokens', badge: 'Most Popular', popular: true },
    { amount: 100000, label: 'Rp 100.000', estTokens: '~3.5M - 8.0M Tokens', badge: 'Pro Developer', popular: false },
    { amount: 250000, label: 'Rp 250.000', estTokens: '~10M - 25M Tokens', badge: 'Enterprise Team', popular: false }
  ] as const

  function openTopupModal() {
    showTopupModal = true
    topupStep = 'packages'
  }

  function proceedToQris() {
    topupStep = 'qris'
    qrisTimeRemaining = 600
    if (qrisTimerInterval) clearInterval(qrisTimerInterval)
    qrisTimerInterval = setInterval(() => {
      if (qrisTimeRemaining > 0) {
        qrisTimeRemaining--
      } else {
        if (qrisTimerInterval) clearInterval(qrisTimerInterval)
      }
    }, 1000)
  }

  async function simulatePaymentConfirmation() {
    if (!clientInfo) return
    simulatedPaymentLoading = true
    try {
      const res = await fetch('/api/billing/topup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          apiKeyId: clientInfo.key,
          balance: (clientInfo.balance > 0 ? clientInfo.balance : 0) + selectedTopupAmount
        })
      })
      if (res.ok) {
        topupStep = 'success'
        await refreshTelemetry(true)
      }
    } catch {} finally {
      simulatedPaymentLoading = false
    }
  }

  let formattedQrisTime = $derived.by(() => {
    const mins = Math.floor(qrisTimeRemaining / 60)
    const secs = qrisTimeRemaining % 60
    return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`
  })

  // Enterprise URL Route Selector (replaces awkward /portal default)
  const CONSOLE_URL_OPTIONS = [
    { path: '/console', label: '/console', badge: 'Recommended', desc: 'Standard Enterprise AI Cloud Console URL' },
    { path: '/workbench', label: '/workbench', badge: 'Developer', desc: 'Anthropic/OpenAI style Prompt Workbench URL' },
    { path: '/studio', label: '/studio', badge: 'AI Studio', desc: 'Clean AI Studio endpoint for engineering teams' },
    { path: '/workspace', label: '/workspace', badge: 'Team', desc: 'Enterprise organization workspace URL' },
    { path: '/trust', label: '/trust', badge: 'Compliance', desc: 'Direct link to Privacy & Regulatory Trust Center' },
    { path: '/portal', label: '/portal', badge: 'Legacy', desc: 'Backward-compatible legacy client portal route' }
  ] as const

  let selectedRoutePath = $state<string>(
    typeof window !== 'undefined' && CONSOLE_URL_OPTIONS.some((o) => o.path === window.location.pathname)
      ? window.location.pathname
      : '/console'
  )

  let fullShareableUrl = $derived(
    typeof window !== 'undefined' ? `${window.location.origin}${selectedRoutePath}` : `http://127.0.0.1:20130${selectedRoutePath}`
  )

  function selectConsoleUrl(path: string) {
    selectedRoutePath = path
    if (typeof window !== 'undefined' && window.location.pathname !== path) {
      window.history.replaceState({ tab: 'portal' }, '', path)
    }
    if (path === '/trust') {
      if (clientInfo) {
        activeMainTab = 'trust'
      } else {
        landingTab = 'compliance'
      }
    }
  }

  // Interactive Enterprise Legal / Regulation Dossier Modal
  type PolicyDocId = 'zdr' | 'dpa' | 'uupdp' | 'security' | 'sla' | null
  let activePolicyModal = $state<PolicyDocId>(null)

  const POLICY_DOCS: Record<
    Exclude<PolicyDocId, null>,
    { title: string; subtitle: string; code: string; clauses: Array<{ heading: string; body: string }> }
  > = {
    zdr: {
      title: 'Zero-Data Retention (ZDR) & Privacy Policy',
      subtitle: 'Cryptographic Ephemeral Inference Guarantee • Effective 2026',
      code: 'GALACTUS-POL-ZDR-4.2',
      clauses: [
        {
          heading: '1. Zero Model Training Guarantee',
          body: '9router Galactus strictly prohibits the use of customer prompts, system instructions, embeddings, or model completions for training, fine-tuning, or evaluating any foundation model weights.'
        },
        {
          heading: '2. Volatile RAM-Only Streaming Pipeline',
          body: 'Inference payloads are processed exclusively in volatile memory buffers during active Server-Sent Events (SSE) transmission. Upon stream termination ([DONE]), payload buffers are zeroed out and released immediately.'
        },
        {
          heading: '3. Metadata-Only Telemetry Ledger',
          body: 'The Client Console records only non-content operational telemetry (timestamp, remasked model ID, token count, latency, and HTTP status code) required for billing accuracy and rate-limit enforcement.'
        },
        {
          heading: '4. Automated PII & Credential Scrubbing',
          body: 'Edge guardrails inspect outbound payloads prior to upstream transit to redact accidental exposure of bearer tokens, private cryptographic keys, NIK/KTP identifiers, and payment card numbers.'
        }
      ]
    },
    dpa: {
      title: 'Enterprise Data Processing Addendum (DPA)',
      subtitle: 'GDPR Art. 28, ISO/IEC 27701 & Cross-Border Sovereign Transit',
      code: 'GALACTUS-DPA-2026-EU',
      clauses: [
        {
          heading: '1. Role of the Parties (Controller vs. Processor)',
          body: 'The Customer acts as the Data Controller (or Pengendali Data Pribadi), and 9router Galactus operates strictly as a zero-persistence Data Processor routing encrypted API calls to designated inference nodes.'
        },
        {
          heading: '2. Sub-Processor Isolation & Anti-Leak Remasking',
          body: 'Before any request leaves the Galactus edge router, organization headers, caller IP addresses, User-Agent strings, and upstream model identifiers are stripped and replaced with standardized enterprise proxy signatures.'
        },
        {
          heading: '3. Cryptographic Right to Erasure',
          body: 'Because prompt and completion bodies are never persisted to disk, customer data erasure is instantaneous by architecture. Revoking an API key immediately terminates all associated session state.'
        }
      ]
    },
    uupdp: {
      title: 'Kepatuhan Regulasi Indonesia (UU PDP No. 27 Tahun 2022) & Global',
      subtitle: 'Standar Perlindungan Data Pribadi Nasional, SOC 2 Type II & ISO/IEC 42001',
      code: 'GALACTUS-REG-ID-27/2022',
      clauses: [
        {
          heading: '1. Kepatuhan UU No. 27 Tahun 2022 (UU PDP Indonesia)',
          body: 'Memenuhi Pasal 35 & Pasal 39 UU PDP terkait kewajiban keamanan pemrosesan data melalui enkripsi TLS 1.3 end-to-end, penyimpanan kredensial terenkripsi AES-256-GCM, serta prinsip pemrosesan tanpa penyimpanan konten (Zero-Data Retention).'
        },
        {
          heading: '2. Sertifikasi Standar Keamanan & AI Internasional',
          body: 'Arsitektur gateway dirancang mengikuti kontrol teknis SOC 2 Type II (Security, Availability, Confidentiality), ISO/IEC 27001:2022 (Information Security), dan ISO/IEC 42001:2023 (Artificial Intelligence Management System).'
        },
        {
          heading: '3. Kedaulatan Data & Audit Trail Real-Time',
          body: 'Setiap eksekusi API memiliki jejak audit deterministik (waktu eksekusi, kuota token, kebijakan konkurensi, dan daftar putih IP) yang dapat diverifikasi secara mandiri oleh pemilik API key melalui konsol ini.'
        }
      ]
    },
    security: {
      title: 'Anti-Leak Model Remasking & Cryptographic Architecture',
      subtitle: 'Defense-in-Depth Gateway Security Specification',
      code: 'GALACTUS-SEC-ARCH-9.1',
      clauses: [
        {
          heading: '1. Bidirectional Model Identity Remasking',
          body: 'Rewrites model names in both request payloads and SSE response chunks, including deep packet inspection of system fingerprints and reasoning blocks, preventing upstream model identity leakage.'
        },
        {
          heading: '2. Master Vault Credential Encryption',
          body: 'Upstream provider credentials and OAuth tokens are encrypted at rest using authenticated AES-256-GCM ciphertexts keyed by the operator master key.'
        },
        {
          heading: '3. Per-Key IP Allowlisting & Rate Governance',
          body: 'Each client bearer token supports strict CIDR/IP allowlisting, Requests-Per-Minute (RPM) ceilings, Tokens-Per-Minute (TPM) quotas, and hard concurrency slot isolation.'
        }
      ]
    },
    sla: {
      title: 'Enterprise 99.99% High-Availability SLA',
      subtitle: 'Multi-Node Failover, Circuit Breaking & Least-Busy Routing',
      code: 'GALACTUS-SLA-9999',
      clauses: [
        {
          heading: '1. 99.99% Uptime & Automated Failover Guarantee',
          body: '9router Galactus continuously probes upstream inference nodes via background health heartbeats. If a provider experiences HTTP 429/5xx degradation, traffic fails over in <15ms to healthy standby nodes.'
        },
        {
          heading: '2. Sub-15ms Edge Routing Overhead',
          body: 'Written in native compiled Go with zero-allocation SSE stream forwarding, adding less than 15ms of proxy overhead to Time-To-First-Token (TTFT).'
        },
        {
          heading: '3. Transparent Token Accounting',
          body: 'Prepaid balances and token quotas are metered strictly on verified completion responses. Failed upstream requests (5xx) are never billed against client balance.'
        }
      ]
    }
  }

  const COMPLIANCE_BADGES = [
    {
      code: 'SOC 2 Type II',
      title: 'AICPA Trust Services Criteria',
      status: 'VERIFIED CONTROLS',
      desc: 'Continuous cryptographic enforcement for Security, Availability, Processing Integrity, and Confidentiality.',
      doc: 'security' as PolicyDocId
    },
    {
      code: 'ISO/IEC 27001 & 42001',
      title: 'InfoSec & AI Governance',
      status: 'CERTIFIED ARCHITECTURE',
      desc: 'Aligned with international standards for Information Security and Responsible AI Management Systems.',
      doc: 'uupdp' as PolicyDocId
    },
    {
      code: 'UU PDP No. 27/2022',
      title: 'Regulasi Privasi Indonesia',
      status: 'PATUH PENUH',
      desc: 'Perlindungan Data Pribadi dengan pemrosesan tanpa retensi (Zero-Retention) & enkripsi kredensial tingkat tinggi.',
      doc: 'uupdp' as PolicyDocId
    },
    {
      code: 'GDPR & EU AI Act',
      title: 'European Data Sovereignty',
      status: 'DPA READY',
      desc: 'Zero persistence of personal data on disk; full compliance with Art. 17 Right to Erasure and Art. 28 DPA.',
      doc: 'dpa' as PolicyDocId
    },
    {
      code: 'Zero-Data Retention',
      title: 'No Training on Customer Data',
      status: 'ENFORCED AT EDGE',
      desc: 'Prompts and completions exist only in ephemeral RAM during SSE streaming and are never logged or trained on.',
      doc: 'zdr' as PolicyDocId
    },
    {
      code: 'HIPAA & PCI-DSS v4',
      title: 'Sensitive Workload Guardrails',
      status: 'ELIGIBLE PIPELINE',
      desc: 'Automated edge scrubbing of PHI/PII, bearer secrets, and payment identifiers before upstream transit.',
      doc: 'zdr' as PolicyDocId
    }
  ]

  // Search & filter in telemetry
  let logSearchQuery = $state('')
  let selectedLogId = $state<number | null>(null)

  // Auto-refresh & telemetry
  let autoRefreshEnabled = $state(true)
  let autoRefreshInterval: ReturnType<typeof setInterval> | null = null
  let pingLatencyMs = $state(9)
  let lastSyncTime = $state('')

  // Animated odometer for Balance
  let animatedBalance = $state(0)
  let balanceTweenObj = { val: 0 }

  // Workbench Playground state
  let testModel = $state('')
  let testSystemPrompt = $state(
    'You are a concise, senior systems architect. Respond with clean, production-grade code and direct technical analysis.'
  )
  let testPrompt = $state(
    'Write a high-performance Go function to find prime numbers using Sieve of Eratosthenes with a brief complexity note.'
  )
  let showSystemPrompt = $state(true)
  let testRunning = $state(false)
  let testOutput = $state('')
  let testLatencyMs = $state(0)
  let testTokensUsed = $state(0)
  let testError = $state('')

  let loginCardRef: HTMLDivElement | null = $state(null)
  let workspaceRef: HTMLDivElement | null = $state(null)

  // Derived analytics
  let totalPromptTokens = $derived(clientInfo?.stats?.promptTokens ?? 0)
  let totalCompletionTokens = $derived(clientInfo?.stats?.completionTokens ?? 0)
  let grandTotalTokens = $derived(
    clientInfo?.stats?.totalTokens || clientInfo?.usedTokens || totalPromptTokens + totalCompletionTokens || 0
  )
  let promptSharePct = $derived(
    grandTotalTokens > 0 ? Math.min(100, Math.max(0, (totalPromptTokens / grandTotalTokens) * 100)) : 50
  )
  let completionSharePct = $derived(grandTotalTokens > 0 ? Math.max(0, 100 - promptSharePct) : 50)

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

  let chartBars = $derived.by(() => {
    const reqs = [...(clientInfo?.recentRequests ?? [])].slice(0, 16).reverse()
    if (reqs.length === 0) return []
    const maxTok = Math.max(...reqs.map((r) => r.totalTokens || r.promptTokens + r.completionTokens || 1), 1)
    return reqs.map((r, idx) => {
      const tot = r.totalTokens || r.promptTokens + r.completionTokens || 0
      const heightPct = Math.max(10, Math.round((tot / maxTok) * 100))
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
    if (typeof window !== 'undefined' && window.location.pathname === '/trust') {
      landingTab = 'compliance'
      activeMainTab = 'trust'
    }

    const saved = localStorage.getItem('9router_client_portal_key') || ''
    if (saved) {
      inputKey = saved
      void loadPortalInfo(saved)
    }

    autoRefreshInterval = setInterval(() => {
      if (autoRefreshEnabled && clientInfo && !loading && !refreshing) {
        void refreshTelemetry(false)
      }
    }, 5000)

    const handleKeydown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && activePolicyModal) {
        activePolicyModal = null
      }
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
      duration: 0.75,
      ease: 'power3.out',
      onUpdate: () => {
        animatedBalance = Math.round(balanceTweenObj.val)
      }
    })
  }

  function switchTab(tab: 'telemetry' | 'playground' | 'guides' | 'tariffs' | 'trust') {
    activeMainTab = tab
    if (workspaceRef) {
      gsap.fromTo(
        workspaceRef,
        { opacity: 0.6, y: 5 },
        { opacity: 1, y: 0, duration: 0.22, ease: 'power3.out', force3D: true, clearProps: 'transform,opacity' }
      )
    }
  }

  async function pasteFromClipboard() {
    try {
      const text = await navigator.clipboard.readText()
      if (text && text.trim()) {
        inputKey = text.trim()
      }
    } catch {}
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
        throw new Error(err.error?.message || `HTTP ${res.status}: Invalid or unauthorized API key`)
      }
      const data = (await res.json()) as ClientInfo
      clientInfo = data
      lastSyncTime = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
      localStorage.setItem('9router_client_portal_key', k)

      if (data.allowedModels && data.allowedModels.length > 0 && !testModel) {
        testModel = data.allowedModels[0] === '*' ? 'gpt-4o' : data.allowedModels[0]
      }

      animateBalanceTo(data.balance)
    } catch (e: any) {
      errorMsg = e.message || 'Failed to authenticate API key'
      clientInfo = null
      if (loginCardRef) {
        gsap.fromTo(loginCardRef, { x: -6 }, { x: 0, duration: 0.3, ease: 'elastic.out(2, 0.2)' })
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
    } catch {
    } finally {
      if (showSpinner) refreshing = false
    }
  }

  function handleLogout() {
    localStorage.removeItem('9router_client_portal_key')
    clientInfo = null
    inputKey = ''
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

<div class="min-h-screen w-full bg-transparent text-[#f4f5f8] font-sans antialiased relative isolate overflow-x-hidden selection:bg-[#e56a4a]/30 selection:text-white">
  <!-- Photorealistic 3D Galaxy & Planetary Background -->
  <GalaxyBackground />

  {#if !clientInfo}
    <!-- =========================================================================
         UNAUTHENTICATED STATE: iOS SMOKED GLASS ENTERPRISE AI CLOUD CONSOLE
         ========================================================================= -->
    <div class="relative z-10 min-h-screen flex flex-col justify-between">
      <!-- Top Minimal iOS Glass Header -->
      <header class="w-full border-b border-white/[0.08] bg-[#090b11]/80 backdrop-blur-2xl sticky top-0 z-30">
        <div class="max-w-6xl mx-auto px-4 sm:px-8 h-16 flex items-center justify-between gap-4">
          <!-- Clean Brand + Single Sleek Status Badge -->
          <div class="flex items-center gap-3">
            <div class="ios-squircle-icon flex items-center justify-center size-9 rounded-[12px] border border-white/15 p-1.5 shrink-0">
              <img src="/favicon.svg" alt="9router Galactus" class="w-full h-full object-contain" />
            </div>
            <div class="flex items-center gap-2.5">
              <span class="text-sm font-semibold tracking-tight text-white">9router Galactus</span>
              <span class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[10px] font-mono bg-white/[0.06] text-emerald-300 border border-white/10">
                <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
                <span>Online</span>
              </span>
            </div>
          </div>

          <!-- Clean Single CTA or Link Button -->
          <div class="flex items-center gap-3">
            <button
              type="button"
              onclick={() => copyText(fullShareableUrl, 'header-share-url')}
              class="px-3 py-1.5 rounded-[10px] bg-white/[0.06] hover:bg-white/[0.12] border border-white/10 text-xs text-[#d8dce6] transition cursor-pointer flex items-center gap-1.5"
            >
              {#if copiedId === 'header-share-url'}
                <Check class="w-3.5 h-3.5 text-emerald-400" />
                <span class="text-emerald-300 font-medium">Link Copied!</span>
              {:else}
                <Copy class="w-3.5 h-3.5 text-[#e56a4a]" />
                <span>Share Console</span>
              {/if}
            </button>
          </div>
        </div>
      </header>

      <!-- Main Centered Sign-In Content -->
      <main class="max-w-md w-full mx-auto px-4 my-auto py-12 space-y-6" bind:this={loginCardRef}>
        <!-- Hero Header -->
        <div class="text-center space-y-2">
          <div class="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-white/[0.06] border border-white/12 text-xs text-[#d8dce6]">
            <ShieldCheck class="w-3.5 h-3.5 text-emerald-400" />
            <span>Enterprise Gateway &bull; Zero-Data Retention</span>
          </div>
          <h1 class="text-2xl sm:text-3xl font-semibold tracking-tight text-white">
            Client Authentication
          </h1>
          <p class="text-xs text-[#8e95a8] max-w-sm mx-auto leading-relaxed">
            Enter your assigned bearer key to access real-time inference telemetry and prompt playground.
          </p>
        </div>

        <!-- Sleek iOS Smoked Crystal Glass Sign-In Card -->
        <div class="p-6 sm:p-7 rounded-[24px] bg-surface border border-white/12 space-y-5 shadow-2xl">
          {#if errorMsg}
            <div class="p-3.5 rounded-[13px] bg-red-500/12 border border-red-500/30 text-xs text-red-200 flex items-start gap-2.5">
              <AlertTriangle class="w-4 h-4 text-red-400 shrink-0 mt-0.5" />
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
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <label for="portal-api-key" class="text-xs font-medium text-[#d8dce6]">
                  Client API Key
                </label>
                <button
                  type="button"
                  onclick={pasteFromClipboard}
                  class="text-[11px] text-[#e56a4a] hover:underline cursor-pointer"
                >
                  Paste from clipboard
                </button>
              </div>

              <!-- Clean Non-Overlapping iOS Input Capsule -->
              <div class="flex items-center gap-2 px-3.5 py-2.5 rounded-[13px] bg-[#0a0c12]/90 border border-white/12 focus-within:border-[#e56a4a] focus-within:ring-2 focus-within:ring-[#e56a4a]/20 transition">
                <Key class="w-4 h-4 text-[#636a7d] shrink-0" />
                <input
                  id="portal-api-key"
                  type={showKey ? 'text' : 'password'}
                  bind:value={inputKey}
                  placeholder="sk-galactus-..."
                  autocomplete="off"
                  class="flex-1 min-w-0 bg-transparent border-none text-xs font-mono text-white placeholder:text-[#52586a] focus:outline-none"
                />
                <button
                  type="button"
                  onclick={() => (showKey = !showKey)}
                  class="p-1 rounded-[8px] text-[#8e95a8] hover:text-white hover:bg-white/10 transition cursor-pointer shrink-0"
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
              class="w-full py-3 px-4 rounded-[13px] bg-[#e56a4a] hover:bg-[#ee7656] text-white font-semibold text-xs transition flex items-center justify-center gap-2 disabled:opacity-40 cursor-pointer shadow-lg"
            >
              {#if loading}
                <RefreshCw class="w-4 h-4 animate-spin" />
                <span>Verifying Token...</span>
              {:else}
                <span>Continue to Console</span>
                <ArrowUpRight class="w-4 h-4" />
              {/if}
            </button>
          </form>

          <div class="pt-3 border-t border-white/[0.08] flex items-center justify-center gap-4 text-[11px] text-[#8e95a8]">
            <button type="button" onclick={() => (activePolicyModal = 'zdr')} class="hover:text-white cursor-pointer">Zero-Retention</button>
            <span>&bull;</span>
            <button type="button" onclick={() => (activePolicyModal = 'security')} class="hover:text-white cursor-pointer">Anti-Leak</button>
            <span>&bull;</span>
            <button type="button" onclick={() => (activePolicyModal = 'uupdp')} class="hover:text-white cursor-pointer">UU PDP</button>
          </div>
        </div>
      </main>

      <!-- Enterprise Footer -->
      <footer class="w-full border-t border-white/[0.08] bg-[#090b11]/70 backdrop-blur-xl py-4 px-4 sm:px-8">
        <div class="max-w-7xl mx-auto flex flex-col sm:flex-row items-center justify-between gap-3 text-[11px] text-[#8e95a8]">
          <div class="flex items-center gap-2">
            <span class="font-semibold text-white">9router Galactus</span>
            <span>&bull;</span>
            <span>Enterprise AI Inference Gateway</span>
            <span>&bull;</span>
            <span class="font-mono text-emerald-400">All Systems Operational</span>
          </div>
          <div class="flex items-center gap-4">
            <button type="button" onclick={() => (activePolicyModal = 'zdr')} class="hover:text-white cursor-pointer">Privacy Policy</button>
            <button type="button" onclick={() => (activePolicyModal = 'dpa')} class="hover:text-white cursor-pointer">DPA</button>
            <button type="button" onclick={() => (activePolicyModal = 'uupdp')} class="hover:text-white cursor-pointer">UU PDP &amp; ISO</button>
            <button type="button" onclick={() => (activePolicyModal = 'sla')} class="hover:text-white cursor-pointer">SLA</button>
          </div>
        </div>
      </footer>
    </div>
  {:else}
    <!-- =========================================================================
         AUTHENTICATED STATE: iOS SMOKED GLASS ENTERPRISE WORKBENCH SHELL
         ========================================================================= -->
    <div class="relative z-10 min-h-screen flex flex-col lg:flex-row">
      <!-- LEFT SIDEBAR (iOS SMOKED GLASS) -->
      <aside class="w-full lg:w-72 lg:min-h-screen border-b lg:border-b-0 lg:border-r border-white/[0.08] flex flex-col justify-between shrink-0">
        <div class="p-5 space-y-5">
          <!-- macOS / iOS Window Controls + Brand Header -->
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-3">
              <div class="ios-squircle-icon flex items-center justify-center size-9 rounded-[12px] border border-white/15 p-1.5 shrink-0">
                <img src="/favicon.svg" alt="9router Galactus" class="w-full h-full object-contain" />
              </div>
              <div>
                <div class="text-sm font-semibold tracking-tight text-white leading-none">
                  9router Galactus
                </div>
                <div class="text-[11px] text-[#8e95a8] mt-1">
                  Enterprise AI Console
                </div>
              </div>
            </div>

            <span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[10px] font-mono bg-white/[0.06] text-emerald-300 border border-white/10">
              <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
              {pingLatencyMs}ms
            </span>
          </div>

          <!-- Active Credential Capsule -->
          <div class="p-3.5 rounded-[16px] bg-surface-2 border border-white/10 space-y-2">
            <div class="flex items-center justify-between">
              <span class="text-[10px] font-semibold uppercase tracking-wider text-[#8e95a8]">Active Bearer Key</span>
              <button
                type="button"
                onclick={() => copyText(clientInfo?.key || '', 'sidebar-key')}
                class="text-[11px] text-[#e56a4a] hover:underline flex items-center gap-1 transition cursor-pointer"
                title="Copy full API Key"
              >
                {#if copiedId === 'sidebar-key'}
                  <Check class="w-3 h-3 text-emerald-400" />
                  <span class="text-emerald-300">Copied</span>
                {:else}
                  <Copy class="w-3 h-3" />
                  <span>Copy Key</span>
                {/if}
              </button>
            </div>
            <div class="text-xs font-semibold text-white truncate">
              {clientInfo.name || 'Enterprise Client'}
            </div>
            <div class="text-[11px] font-mono text-[#8e95a8] truncate">
              {clientInfo.keyMasked}
            </div>
          </div>

          <!-- Navigation Menu (Connected Liquid Button Flow enabled via data-flow-group) -->
          <nav data-flow-group class="flex lg:flex-col gap-1 overflow-x-auto pb-1 lg:pb-0">
            <button
              type="button"
              aria-selected={activeMainTab === 'telemetry'}
              onclick={() => switchTab('telemetry')}
              class="flex items-center justify-between px-3.5 py-2.5 rounded-[13px] text-xs transition cursor-pointer shrink-0 {activeMainTab === 'telemetry'
                ? 'ios-nav-active text-[#e56a4a] font-semibold'
                : 'text-[#8e95a8] hover:text-white hover:bg-white/[0.04]'}"
            >
              <span class="flex items-center gap-2.5">
                <Activity class="w-4 h-4" />
                <span>Usage &amp; Telemetry</span>
              </span>
              {#if clientInfo.recentRequests && clientInfo.recentRequests.length > 0}
                <span class="hidden lg:inline-block font-mono text-[10px] px-1.5 py-0.5 rounded-full bg-white/[0.06] text-[#d8dce6]">
                  {clientInfo.recentRequests.length}
                </span>
              {/if}
            </button>

            <button
              type="button"
              aria-selected={activeMainTab === 'playground'}
              onclick={() => switchTab('playground')}
              class="flex items-center justify-between px-3.5 py-2.5 rounded-[13px] text-xs transition cursor-pointer shrink-0 {activeMainTab === 'playground'
                ? 'ios-nav-active text-[#e56a4a] font-semibold'
                : 'text-[#8e95a8] hover:text-white hover:bg-white/[0.04]'}"
            >
              <span class="flex items-center gap-2.5">
                <Terminal class="w-4 h-4" />
                <span>Prompt Workbench</span>
              </span>
              <span class="hidden lg:inline-block text-[10px] font-mono text-emerald-400">LIVE</span>
            </button>

            <button
              type="button"
              aria-selected={activeMainTab === 'guides'}
              onclick={() => switchTab('guides')}
              class="flex items-center justify-between px-3.5 py-2.5 rounded-[13px] text-xs transition cursor-pointer shrink-0 {activeMainTab === 'guides'
                ? 'ios-nav-active text-[#e56a4a] font-semibold'
                : 'text-[#8e95a8] hover:text-white hover:bg-white/[0.04]'}"
            >
              <span class="flex items-center gap-2.5">
                <BookOpen class="w-4 h-4" />
                <span>IDE &amp; URL Setup</span>
              </span>
            </button>

            <button
              type="button"
              aria-selected={activeMainTab === 'tariffs'}
              onclick={() => switchTab('tariffs')}
              class="flex items-center justify-between px-3.5 py-2.5 rounded-[13px] text-xs transition cursor-pointer shrink-0 {activeMainTab === 'tariffs'
                ? 'ios-nav-active text-[#e56a4a] font-semibold'
                : 'text-[#8e95a8] hover:text-white hover:bg-white/[0.04]'}"
            >
              <span class="flex items-center gap-2.5">
                <Layers class="w-4 h-4" />
                <span>Model Rate Sheet</span>
              </span>
              {#if clientInfo.pricingTable && clientInfo.pricingTable.length > 0}
                <span class="hidden lg:inline-block font-mono text-[10px] text-[#8e95a8]">
                  {clientInfo.pricingTable.length}
                </span>
              {/if}
            </button>

            <button
              type="button"
              aria-selected={activeMainTab === 'trust'}
              onclick={() => switchTab('trust')}
              class="flex items-center justify-between px-3.5 py-2.5 rounded-[13px] text-xs transition cursor-pointer shrink-0 {activeMainTab === 'trust'
                ? 'ios-nav-active text-[#e56a4a] font-semibold'
                : 'text-[#8e95a8] hover:text-white hover:bg-white/[0.04]'}"
            >
              <span class="flex items-center gap-2.5">
                <ShieldCheck class="w-4 h-4" />
                <span>Privacy &amp; Compliance</span>
              </span>
              <span class="hidden lg:inline-block text-[9px] font-mono px-1.5 py-0.5 rounded-full bg-emerald-500/15 text-emerald-300 border border-emerald-500/30">
                ZDR
              </span>
            </button>
          </nav>
        </div>

        <!-- Sidebar Bottom Controls -->
        <div class="hidden lg:block p-4 border-t border-white/[0.08] space-y-3">
          <div class="p-3 rounded-[13px] bg-surface-2 border border-white/[0.07] space-y-1.5">
            <div class="flex items-center justify-between text-[11px] text-[#8e95a8]">
              <span>Gateway Base URL</span>
              <button
                type="button"
                onclick={() => copyText(clientInfo?.configs.baseUrl || '', 'sidebar-url')}
                class="text-[#e56a4a] hover:underline cursor-pointer font-medium"
              >
                {copiedId === 'sidebar-url' ? 'Copied!' : 'Copy'}
              </button>
            </div>
            <div class="font-mono text-[11px] text-white truncate">
              {clientInfo.configs.baseUrl}
            </div>
          </div>

          <div class="flex items-center justify-between pt-1 px-1">
            <button
              type="button"
              onclick={() => (autoRefreshEnabled = !autoRefreshEnabled)}
              class="inline-flex items-center gap-1.5 text-[11px] text-[#8e95a8] hover:text-white cursor-pointer"
            >
              <span class="w-2 h-2 rounded-full {autoRefreshEnabled ? 'bg-emerald-400' : 'bg-zinc-600'}"></span>
              <span>{autoRefreshEnabled ? 'Live Sync (5s)' : 'Sync Paused'}</span>
            </button>

            <button
              type="button"
              onclick={handleLogout}
              class="inline-flex items-center gap-1 text-[11px] text-[#8e95a8] hover:text-red-400 transition cursor-pointer"
            >
              <LogOut class="w-3.5 h-3.5" />
              <span>Sign Out</span>
            </button>
          </div>
        </div>
      </aside>

      <!-- MAIN WORKSPACE CANVAS -->
      <div class="flex-1 flex flex-col min-w-0">
        <!-- Top iOS Smoked Glass Context Header Bar -->
        <header class="h-16 px-6 border-b border-white/[0.08] flex items-center justify-between sticky top-0 z-20">
          <div class="flex items-center gap-3 min-w-0">
            <h2 class="text-base sm:text-lg font-semibold text-white truncate">
              {#if activeMainTab === 'telemetry'}
                Usage &amp; Live Telemetry
              {:else if activeMainTab === 'playground'}
                Prompt Workbench
              {:else if activeMainTab === 'guides'}
                IDE &amp; Console URL Setup
              {:else if activeMainTab === 'tariffs'}
                Model Rate &amp; Tariff Sheet
              {:else}
                Enterprise Privacy, Regulations &amp; Compliance
              {/if}
            </h2>

            <!-- iOS Dynamic Island Capsule -->
            <div class="hidden md:inline-flex ios-dynamic-island">
              <span class="ios-island-dot"></span>
              <span class="text-[11px] font-mono text-[#d8dce6]">
                ZDR Active &bull; Anti-Leak ON
              </span>
            </div>
          </div>

          <div class="flex items-center gap-2">
            <!-- Console URL Switcher Pill -->
            <div class="hidden xl:flex items-center gap-1.5 px-2.5 py-1.5 rounded-[12px] bg-surface-2 border border-white/10">
              <span class="text-[11px] text-[#8e95a8]">URL:</span>
              <select
                value={selectedRoutePath}
                onchange={(e) => selectConsoleUrl((e.currentTarget as HTMLSelectElement).value)}
                aria-label="Console URL route"
                class="bg-transparent text-xs font-mono text-white focus:outline-none cursor-pointer"
              >
                {#each CONSOLE_URL_OPTIONS as opt}
                  <option value={opt.path} class="bg-[#0d0f17] text-white">{opt.label}</option>
                {/each}
              </select>
              <button
                type="button"
                onclick={() => copyText(fullShareableUrl, 'auth-share-url')}
                class="px-2 py-0.5 rounded-[8px] bg-white/[0.08] hover:bg-white/[0.14] text-[11px] text-white cursor-pointer"
              >
                {copiedId === 'auth-share-url' ? 'Copied!' : 'Copy Link'}
              </button>
            </div>

            <button
              type="button"
              onclick={() => refreshTelemetry(true)}
              disabled={refreshing}
              class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-[12px] bg-surface-2 hover:bg-white/10 border border-white/10 text-xs text-[#d8dce6] hover:text-white transition cursor-pointer disabled:opacity-50"
            >
              <RefreshCw class="w-3.5 h-3.5 {refreshing ? 'animate-spin text-[#e56a4a]' : ''}" />
              <span class="hidden sm:inline">Refresh</span>
            </button>

            <button
              type="button"
              onclick={() => copyText(clientInfo?.key || '', 'header-key')}
              class="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-[12px] bg-[#e56a4a] hover:bg-[#ee7656] text-xs font-semibold text-white transition cursor-pointer"
            >
              {#if copiedId === 'header-key'}
                <Check class="w-3.5 h-3.5" />
                <span>Copied Key</span>
              {:else}
                <Key class="w-3.5 h-3.5" />
                <span>Copy Key</span>
              {/if}
            </button>
          </div>
        </header>

        <!-- Main Body Content -->
        <main class="flex-1 p-5 sm:p-8 max-w-7xl w-full mx-auto space-y-6" bind:this={workspaceRef}>
          {#if activeMainTab === 'telemetry'}
            <!-- ===============================================================
                 4-CARD iOS SMOKED GLASS METRIC LEDGER
                 =============================================================== -->
            <section class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
              <!-- Metric 1: Prepaid Balance -->
              <div class="p-5 rounded-[22px] bg-surface border border-white/10 flex flex-col justify-between space-y-4">
                <div class="flex items-center justify-between">
                  <span class="text-xs font-medium text-[#8e95a8]">Prepaid Balance</span>
                  <div class="flex items-center gap-1.5">
                    <span class="px-2 py-0.5 rounded-full text-[10px] font-mono bg-emerald-500/12 text-emerald-300 border border-emerald-500/25">
                      IDR
                    </span>
                    <button
                      type="button"
                      onclick={openTopupModal}
                      class="px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-[#e56a4a] hover:bg-[#ee7656] text-white transition cursor-pointer flex items-center gap-1 shadow-sm"
                    >
                      <CreditCard class="w-3 h-3" />
                      <span>Top Up</span>
                    </button>
                  </div>
                </div>

                <div>
                  {#if clientInfo.balance < 0}
                    <div class="text-2xl sm:text-3xl font-semibold tracking-tight text-white">
                      Unlimited Quota
                    </div>
                  {:else}
                    <div class="text-2xl sm:text-3xl font-semibold tracking-tight text-white tabular-nums">
                      Rp {animatedBalance.toLocaleString('id-ID')}
                    </div>
                  {/if}
                </div>

                <div class="flex items-center justify-between text-[11px] text-[#8e95a8] pt-2 border-t border-white/[0.07]">
                  <span>Billing Tier</span>
                  <span class="text-[#d8dce6]">Pay-per-1K tokens</span>
                </div>
              </div>

              <!-- Metric 2: API Calls & Throughput -->
              <div class="p-5 rounded-[22px] bg-surface border border-white/10 flex flex-col justify-between space-y-4">
                <div class="flex items-center justify-between">
                  <span class="text-xs font-medium text-[#8e95a8]">Total API Calls</span>
                  <span class="inline-flex items-center gap-1 text-[11px] text-emerald-400 font-mono">
                    <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
                    100% OK
                  </span>
                </div>

                <div class="flex items-baseline justify-between gap-4">
                  <div class="text-2xl sm:text-3xl font-semibold tracking-tight text-white tabular-nums">
                    {(clientInfo.stats?.totalRequests ?? 0).toLocaleString()}
                  </div>

                  {#if chartBars.length > 0}
                    <div class="flex items-end gap-1 h-7">
                      {#each chartBars.slice(-10) as bar}
                        <div
                          class="w-1.5 rounded-t-sm bg-[#e56a4a]/80 hover:bg-[#e56a4a] transition-all"
                          style="height: {Math.max(15, bar.heightPct)}%;"
                          title="{bar.model}: {bar.tot.toLocaleString()} tokens"
                        ></div>
                      {/each}
                    </div>
                  {/if}
                </div>

                <div class="flex items-center justify-between text-[11px] text-[#8e95a8] pt-2 border-t border-white/[0.07]">
                  <span>Cumulative Spend</span>
                  <span class="font-mono text-[#d8dce6]">
                    Rp {(clientInfo.stats?.cost ?? 0).toLocaleString('id-ID', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
                  </span>
                </div>
              </div>

              <!-- Metric 3: Token Consumption & Split Ratio -->
              <div class="p-5 rounded-[22px] bg-surface border border-white/10 flex flex-col justify-between space-y-4">
                <div class="flex items-center justify-between">
                  <span class="text-xs font-medium text-[#8e95a8]">Token Volume</span>
                  <span class="text-[11px] font-mono text-[#d8dce6]">
                    {promptSharePct.toFixed(0)}% in / {completionSharePct.toFixed(0)}% out
                  </span>
                </div>

                <div class="space-y-2.5">
                  <div class="text-2xl sm:text-3xl font-semibold tracking-tight text-white tabular-nums">
                    {grandTotalTokens.toLocaleString()}
                  </div>

                  <div class="w-full h-1.5 rounded-full bg-white/10 overflow-hidden flex">
                    <div class="h-full bg-[#e56a4a] transition-all duration-500" style="width: {promptSharePct}%;"></div>
                    <div class="h-full bg-sky-400 transition-all duration-500" style="width: {completionSharePct}%;"></div>
                  </div>
                </div>

                <div class="flex items-center justify-between text-[11px] font-mono text-[#8e95a8] pt-2 border-t border-white/[0.07]">
                  <span>In: {totalPromptTokens.toLocaleString()}</span>
                  <span>Out: {totalCompletionTokens.toLocaleString()}</span>
                </div>
              </div>

              <!-- Metric 4: Gateway Policy & Model Routing -->
              <div class="p-5 rounded-[22px] bg-surface border border-white/10 flex flex-col justify-between space-y-4">
                <div class="flex items-center justify-between">
                  <span class="text-xs font-medium text-[#8e95a8]">Allowed Models</span>
                  <span class="text-[11px] font-mono text-[#d8dce6]">
                    {clientInfo.rateLimitRpm > 0 ? `${clientInfo.rateLimitRpm} RPM` : 'Unthrottled'}
                  </span>
                </div>

                <div class="flex flex-wrap items-center gap-1.5">
                  {#each clientInfo.allowedModels.slice(0, 3) as model}
                    <button
                      type="button"
                      onclick={() => {
                        testModel = model === '*' ? 'gpt-4o' : model
                        switchTab('playground')
                      }}
                      class="px-2.5 py-1 rounded-[10px] bg-surface-2 hover:bg-white/10 border border-white/10 text-xs font-mono text-white transition cursor-pointer flex items-center gap-1.5"
                      title="Test {model} in Prompt Workbench"
                    >
                      <span class="truncate max-w-[140px]">{model}</span>
                      <ArrowUpRight class="w-3 h-3 text-[#e56a4a]" />
                    </button>
                  {/each}
                </div>

                <div class="flex items-center justify-between text-[11px] text-[#8e95a8] pt-2 border-t border-white/[0.07]">
                  <span>Concurrency</span>
                  <span class="font-mono text-[#d8dce6]">
                    {clientInfo.concurrency > 0 ? `${clientInfo.concurrency} parallel` : 'Unlimited'}
                  </span>
                </div>
              </div>
            </section>

            <!-- ===============================================================
                 REQUEST LOG STREAM TABLE (iOS SMOKED GLASS)
                 =============================================================== -->
            <section class="rounded-[22px] bg-surface border border-white/10 overflow-hidden">
              <div class="px-6 py-4 border-b border-white/[0.08] flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                <div>
                  <h3 class="text-sm font-semibold text-white">
                    Live Request Telemetry Stream
                  </h3>
                  <p class="text-xs text-[#8e95a8] mt-0.5">
                    Real-time metadata ledger (Zero-Data Retention: prompt bodies are never stored)
                  </p>
                </div>

                <div class="flex items-center gap-2 px-3 py-1.5 rounded-[12px] bg-surface-2 border border-white/10 w-full sm:w-64">
                  <Search class="w-3.5 h-3.5 text-[#8e95a8] shrink-0" />
                  <input
                    type="text"
                    bind:value={logSearchQuery}
                    placeholder="Filter by model or status..."
                    class="w-full bg-transparent border-none text-xs text-white placeholder:text-[#636a7d] focus:outline-none"
                  />
                </div>
              </div>

              {#if !clientInfo.recentRequests || clientInfo.recentRequests.length === 0}
                <div class="p-12 text-center space-y-3">
                  <p class="text-sm font-medium text-white">No inference requests recorded yet</p>
                  <p class="text-xs text-[#8e95a8] max-w-sm mx-auto">
                    Send your first chat completion from Cursor, Cline, or test your key directly in the Prompt Workbench.
                  </p>
                  <button
                    type="button"
                    onclick={() => switchTab('playground')}
                    class="mt-2 inline-flex items-center gap-1.5 px-4 py-2 rounded-[13px] bg-[#e56a4a] text-white font-semibold text-xs transition cursor-pointer"
                  >
                    <Terminal class="w-3.5 h-3.5" />
                    <span>Open Prompt Workbench</span>
                  </button>
                </div>
              {:else if filteredRequests.length === 0}
                <div class="p-8 text-center text-xs text-[#8e95a8]">
                  No requests matching "{logSearchQuery}"
                </div>
              {:else}
                <div class="overflow-x-auto">
                  <table class="w-full text-left border-collapse">
                    <thead>
                      <tr class="border-b border-white/[0.08] text-[11px] uppercase tracking-wider text-[#8e95a8] bg-white/[0.02]">
                        <th class="py-3 px-6 font-medium">Time</th>
                        <th class="py-3 px-6 font-medium">Remasked Model</th>
                        <th class="py-3 px-6 font-medium text-right">Prompt (In)</th>
                        <th class="py-3 px-6 font-medium text-right">Completion (Out)</th>
                        <th class="py-3 px-6 font-medium text-right">Total</th>
                        <th class="py-3 px-6 font-medium text-right">Cost (IDR)</th>
                        <th class="py-3 px-6 font-medium text-right">Status</th>
                      </tr>
                    </thead>
                    <tbody class="divide-y divide-white/[0.05] text-xs">
                      {#each filteredRequests as req (req.id || req.timestamp + req.promptTokens)}
                        {@const rowTotal = req.totalTokens || req.promptTokens + req.completionTokens}
                        {@const isOk = req.status === 'success' || req.status === 'ok'}
                        <tr class="hover:bg-white/[0.03] transition-colors">
                          <td class="py-3.5 px-6 font-mono text-[11px] text-[#8e95a8] whitespace-nowrap">
                            {formatTimestamp(req.timestamp)}
                          </td>
                          <td class="py-3.5 px-6 font-mono text-xs text-white font-medium">
                            {req.model}
                          </td>
                          <td class="py-3.5 px-6 text-right font-mono text-[11px] text-emerald-400 tabular-nums">
                            {req.promptTokens.toLocaleString()}
                          </td>
                          <td class="py-3.5 px-6 text-right font-mono text-[11px] text-sky-400 tabular-nums">
                            {req.completionTokens.toLocaleString()}
                          </td>
                          <td class="py-3.5 px-6 text-right font-mono text-[11px] text-white tabular-nums">
                            {rowTotal.toLocaleString()}
                          </td>
                          <td class="py-3.5 px-6 text-right font-mono text-[11px] text-[#d8dce6] tabular-nums">
                            Rp {req.cost.toFixed(2)}
                          </td>
                          <td class="py-3.5 px-6 text-right whitespace-nowrap">
                            <span class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full font-mono text-[10px] {isOk ? 'bg-emerald-500/12 text-emerald-300 border border-emerald-500/25' : 'bg-red-500/12 text-red-300 border border-red-500/25'}">
                              <span class="w-1.5 h-1.5 rounded-full {isOk ? 'bg-emerald-400' : 'bg-red-400'}"></span>
                              <span>{isOk ? '200 OK' : 'ERROR'}</span>
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
                 TAB 2: ENTERPRISE PROMPT WORKBENCH
                 =============================================================== -->
            <section class="rounded-[22px] bg-surface border border-white/10 overflow-hidden flex flex-col">
              <div class="px-6 py-4 border-b border-white/[0.08] flex flex-wrap items-center justify-between gap-3">
                <div class="flex flex-wrap items-center gap-3">
                  <div class="flex items-center gap-2">
                    <span class="text-xs text-[#8e95a8]">Model</span>
                    <select
                      bind:value={testModel}
                      class="px-3 py-1.5 bg-surface-2 border border-white/12 rounded-[11px] text-xs font-mono text-white focus:outline-none cursor-pointer"
                    >
                      {#each clientInfo.allowedModels as m}
                        <option value={m === '*' ? 'gpt-4o' : m} class="bg-[#0d0f17] text-white">{m}</option>
                      {/each}
                    </select>
                  </div>

                  <button
                    type="button"
                    onclick={() => (showSystemPrompt = !showSystemPrompt)}
                    class="px-3 py-1.5 rounded-[11px] border text-xs transition cursor-pointer flex items-center gap-1.5 {showSystemPrompt
                      ? 'bg-surface-2 border-white/20 text-white'
                      : 'bg-transparent border-white/10 text-[#8e95a8] hover:text-white'}"
                  >
                    <SlidersHorizontal class="w-3.5 h-3.5" />
                    <span>System Prompt</span>
                  </button>
                </div>

                <div class="flex items-center gap-3">
                  {#if testLatencyMs > 0}
                    <span class="text-[11px] font-mono text-[#8e95a8]">
                      TTFT: <strong class="text-emerald-400">{testLatencyMs}ms</strong>
                    </span>
                  {/if}
                  {#if testTokensUsed > 0}
                    <span class="text-[11px] font-mono text-[#8e95a8]">
                      Tokens: <strong class="text-white">{testTokensUsed}</strong>
                    </span>
                  {/if}

                  <button
                    type="button"
                    onclick={runPlaygroundTest}
                    disabled={testRunning || !testPrompt.trim()}
                    class="px-4 py-1.5 rounded-[12px] bg-[#e56a4a] hover:bg-[#ee7656] text-white font-semibold text-xs transition flex items-center gap-2 disabled:opacity-40 cursor-pointer"
                  >
                    {#if testRunning}
                      <RefreshCw class="w-3.5 h-3.5 animate-spin" />
                      <span>Streaming...</span>
                    {:else}
                      <Play class="w-3.5 h-3.5 fill-current" />
                      <span>Run Prompt</span>
                    {/if}
                  </button>
                </div>
              </div>

              <div class="grid grid-cols-1 lg:grid-cols-2 divide-y lg:divide-y-0 lg:divide-x divide-white/[0.08]">
                <div class="p-6 space-y-4">
                  {#if showSystemPrompt}
                    <div class="space-y-1.5">
                      <label for="wb-system-prompt" class="block text-xs font-medium text-[#8e95a8]">
                        System Instructions
                      </label>
                      <textarea
                        id="wb-system-prompt"
                        bind:value={testSystemPrompt}
                        rows="2"
                        class="w-full p-3 bg-surface-2 border border-white/10 rounded-[13px] text-xs text-white font-mono leading-relaxed focus:outline-none resize-y"
                      ></textarea>
                    </div>
                  {/if}

                  <div class="space-y-2">
                    <div class="flex items-center justify-between">
                      <label for="wb-user-prompt" class="block text-xs font-medium text-[#8e95a8]">
                        User Prompt
                      </label>
                      <div data-flow-group class="flex items-center gap-1.5">
                        <button
                          type="button"
                          onclick={() => (testPrompt = 'Say hello and summarize your architecture in 2 concise bullet points.')}
                          class="px-2.5 py-1 rounded-[9px] bg-surface-2 hover:bg-white/10 border border-white/10 text-[11px] text-[#d8dce6] cursor-pointer"
                        >
                          Ping
                        </button>
                        <button
                          type="button"
                          onclick={() => (testPrompt = 'Write a fast Go function to find prime numbers using Sieve of Eratosthenes.')}
                          class="px-2.5 py-1 rounded-[9px] bg-surface-2 hover:bg-white/10 border border-white/10 text-[11px] text-[#d8dce6] cursor-pointer"
                        >
                          Go Sieve
                        </button>
                      </div>
                    </div>

                    <textarea
                      id="wb-user-prompt"
                      bind:value={testPrompt}
                      rows="10"
                      class="w-full p-3.5 bg-surface-2 border border-white/10 rounded-[13px] text-xs text-white font-mono leading-relaxed focus:outline-none resize-y"
                    ></textarea>
                  </div>
                </div>

                <div class="p-6 flex flex-col justify-between min-h-[340px]">
                  <div class="space-y-3">
                    <div class="flex items-center justify-between">
                      <span class="text-xs font-medium text-[#8e95a8]">Live SSE Stream Output (RAM-Only ZDR)</span>
                      {#if testOutput}
                        <button
                          type="button"
                          onclick={() => copyText(testOutput, 'test-output')}
                          class="text-xs text-[#e56a4a] hover:underline flex items-center gap-1 cursor-pointer"
                        >
                          <Copy class="w-3.5 h-3.5" />
                          <span>{copiedId === 'test-output' ? 'Copied' : 'Copy Output'}</span>
                        </button>
                      {/if}
                    </div>

                    {#if testError}
                      <div class="p-4 rounded-[13px] bg-red-500/12 border border-red-500/30 text-xs text-red-200 font-mono">
                        {testError}
                      </div>
                    {:else if testOutput || testRunning}
                      <div class="p-4 rounded-[13px] bg-surface-2 border border-white/10 font-mono text-xs text-white leading-relaxed whitespace-pre-wrap max-h-[420px] overflow-y-auto">
                        {testOutput}{#if testRunning}<span class="inline-block w-1.5 h-3.5 bg-[#e56a4a] animate-pulse ml-0.5 align-middle"></span>{/if}
                      </div>
                    {:else}
                      <div class="h-64 rounded-[16px] border border-dashed border-white/12 flex flex-col items-center justify-center text-center p-6">
                        <p class="text-xs text-[#8e95a8]">
                          Click <strong class="text-white">Run Prompt</strong> to verify live SSE streaming from <span class="font-mono text-[#e56a4a]">{testModel || 'your model'}</span>.
                        </p>
                      </div>
                    {/if}
                  </div>
                </div>
              </div>
            </section>
          {:else if activeMainTab === 'guides'}
            <!-- ===============================================================
                 TAB 3: IDE INTEGRATION & ENTERPRISE URL ALIASES
                 =============================================================== -->
            <div class="space-y-6">
              <!-- Enterprise Shareable URL Selector Card -->
              <section class="p-6 rounded-[22px] bg-surface border border-white/10 space-y-4">
                <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
                  <div>
                    <h3 class="text-sm font-semibold text-white">Shareable Enterprise Console URLs</h3>
                    <p class="text-xs text-[#8e95a8] mt-0.5">
                      Instead of <code class="font-mono text-white">/portal</code>, share any of these official Enterprise Console routes with your team or customers:
                    </p>
                  </div>
                </div>

                <div data-flow-group class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
                  {#each CONSOLE_URL_OPTIONS as opt}
                    {@const fullOptUrl = typeof window !== 'undefined' ? `${window.location.origin}${opt.path}` : opt.path}
                    <div
                      class="p-4 rounded-[16px] border transition flex flex-col justify-between gap-3 {selectedRoutePath === opt.path
                        ? 'bg-[#e56a4a]/15 border-[#e56a4a]/45'
                        : 'bg-surface-2 border-white/10'}"
                    >
                      <div class="space-y-1">
                        <div class="flex items-center justify-between">
                          <span class="font-mono text-sm font-semibold text-white">{opt.label}</span>
                          <span class="px-2 py-0.5 rounded-full text-[10px] font-medium bg-white/10 text-white">
                            {opt.badge}
                          </span>
                        </div>
                        <p class="text-[11px] text-[#8e95a8]">{opt.desc}</p>
                      </div>
                      <div class="flex items-center justify-between gap-2 pt-2 border-t border-white/[0.07]">
                        <span class="font-mono text-[10px] text-[#d8dce6] truncate">{fullOptUrl}</span>
                        <button
                          type="button"
                          onclick={() => {
                            selectConsoleUrl(opt.path)
                            void copyText(fullOptUrl, `guide-url-${opt.path}`)
                          }}
                          class="px-2.5 py-1 rounded-[9px] bg-[#e56a4a] text-white text-[11px] font-semibold cursor-pointer shrink-0"
                        >
                          {copiedId === `guide-url-${opt.path}` ? 'Copied!' : 'Use & Copy'}
                        </button>
                      </div>
                    </div>
                  {/each}
                </div>
              </section>

              <!-- IDE & SDK Configuration Card -->
              <section class="rounded-[22px] bg-surface border border-white/10 overflow-hidden">
                <div class="px-6 py-4 border-b border-white/[0.08] flex flex-wrap items-center justify-between gap-3">
                  <div data-flow-group class="flex flex-wrap items-center gap-1.5">
                    {#each [
                      { id: 'cursor', label: 'Cursor IDE' },
                      { id: 'cline', label: 'Cline / Roo Code' },
                      { id: 'windsurf', label: 'Windsurf' },
                      { id: 'python', label: 'Python SDK' },
                      { id: 'curl', label: 'cURL CLI' }
                    ] as tab}
                      <button
                        type="button"
                        aria-selected={activeGuideTab === tab.id}
                        onclick={() => (activeGuideTab = tab.id as any)}
                        class="px-3.5 py-1.5 rounded-[11px] text-xs font-medium transition cursor-pointer {activeGuideTab === tab.id
                          ? 'bg-[#e56a4a] text-white'
                          : 'bg-surface-2 text-[#8e95a8] hover:text-white'}"
                      >
                        {tab.label}
                      </button>
                    {/each}
                  </div>
                </div>

                <div class="p-6 space-y-5 text-xs">
                  {#if activeGuideTab === 'cursor'}
                    <div class="space-y-4 max-w-2xl">
                      <p class="text-[#d8dce6] leading-relaxed">
                        In Cursor IDE, open <strong class="text-white">Settings &rarr; Models &rarr; OpenAI API Key</strong> and paste:
                      </p>
                      <div class="space-y-3">
                        <div class="p-4 rounded-[14px] bg-surface-2 border border-white/10 flex items-center justify-between gap-4">
                          <div class="min-w-0">
                            <div class="text-[11px] text-[#8e95a8] mb-1">Override OpenAI Base URL</div>
                            <div class="font-mono text-xs text-white truncate select-all">{clientInfo.configs.baseUrl}</div>
                          </div>
                          <button
                            type="button"
                            onclick={() => copyText(clientInfo?.configs.baseUrl || '', 'cursor-url')}
                            class="px-3 py-1.5 rounded-[10px] bg-white/10 hover:bg-white/15 text-xs text-white shrink-0 cursor-pointer flex items-center gap-1.5"
                          >
                            <Copy class="w-3.5 h-3.5 text-[#e56a4a]" />
                            <span>{copiedId === 'cursor-url' ? 'Copied' : 'Copy URL'}</span>
                          </button>
                        </div>

                        <div class="p-4 rounded-[14px] bg-surface-2 border border-white/10 flex items-center justify-between gap-4">
                          <div class="min-w-0">
                            <div class="text-[11px] text-[#8e95a8] mb-1">OpenAI API Key</div>
                            <div class="font-mono text-xs text-white truncate select-all">{clientInfo.key}</div>
                          </div>
                          <button
                            type="button"
                            onclick={() => copyText(clientInfo?.key || '', 'cursor-key')}
                            class="px-3 py-1.5 rounded-[10px] bg-white/10 hover:bg-white/15 text-xs text-white shrink-0 cursor-pointer flex items-center gap-1.5"
                          >
                            <Copy class="w-3.5 h-3.5 text-[#e56a4a]" />
                            <span>{copiedId === 'cursor-key' ? 'Copied' : 'Copy Key'}</span>
                          </button>
                        </div>
                      </div>
                    </div>
                  {:else if activeGuideTab === 'cline'}
                    <div class="space-y-4 max-w-2xl">
                      <div class="p-4 rounded-[14px] bg-surface-2 border border-white/10 space-y-2.5 font-mono text-xs">
                        <div class="flex justify-between"><span class="text-[#8e95a8]">API Provider:</span> <span class="text-white">OpenAI Compatible</span></div>
                        <div class="flex justify-between"><span class="text-[#8e95a8]">Base URL:</span> <span class="text-[#e56a4a]">{clientInfo.configs.baseUrl}</span></div>
                        <div class="flex justify-between"><span class="text-[#8e95a8]">API Key:</span> <span class="text-white">{clientInfo.key}</span></div>
                        <div class="flex justify-between"><span class="text-[#8e95a8]">Model ID:</span> <span class="text-white">{testModel || 'gpt-4o'}</span></div>
                      </div>
                    </div>
                  {:else if activeGuideTab === 'windsurf'}
                    <div class="space-y-4 max-w-2xl">
                      <div class="p-4 rounded-[14px] bg-surface-2 border border-white/10 space-y-2.5 font-mono text-xs">
                        <div class="flex justify-between"><span class="text-[#8e95a8]">Provider:</span> <span class="text-white">openai-compatible</span></div>
                        <div class="flex justify-between"><span class="text-[#8e95a8]">Endpoint:</span> <span class="text-[#e56a4a]">{clientInfo.configs.baseUrl}</span></div>
                        <div class="flex justify-between"><span class="text-[#8e95a8]">API Key:</span> <span class="text-white">{clientInfo.key}</span></div>
                      </div>
                    </div>
                  {:else if activeGuideTab === 'python'}
                    <div class="space-y-3">
                      <div class="flex items-center justify-between">
                        <span class="text-[#8e95a8]">OpenAI Python SDK snippet</span>
                        <button
                          type="button"
                          onclick={() => copyText(clientInfo?.configs.pythonSdk || '', 'sdk-code')}
                          class="text-[#e56a4a] hover:underline flex items-center gap-1 cursor-pointer"
                        >
                          <Copy class="w-3.5 h-3.5" />
                          <span>{copiedId === 'sdk-code' ? 'Copied' : 'Copy snippet'}</span>
                        </button>
                      </div>
                      <pre class="p-4 rounded-[14px] bg-surface-2 border border-white/10 text-white overflow-x-auto leading-relaxed font-mono text-xs">{clientInfo.configs.pythonSdk}</pre>
                    </div>
                  {:else if activeGuideTab === 'curl'}
                    <div class="space-y-3">
                      <pre class="p-4 rounded-[14px] bg-surface-2 border border-white/10 text-white overflow-x-auto leading-relaxed font-mono text-xs">curl -X POST "{clientInfo.configs.baseUrl}/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer {clientInfo.key}" \
  -d '&#123;"model": "{testModel || 'gpt-4o'}", "messages": [&#123;"role": "user", "content": "Hello!"&#125;]&#125;'</pre>
                    </div>
                  {/if}
                </div>
              </section>
            </div>
          {:else if activeMainTab === 'tariffs'}
            <!-- ===============================================================
                 TAB 4: MODEL TARIFF & RATE SHEET
                 =============================================================== -->
            <section class="rounded-[22px] bg-surface border border-white/10 overflow-hidden">
              <div class="px-6 py-4 border-b border-white/[0.08] flex items-center justify-between">
                <div>
                  <h3 class="text-sm font-semibold text-white">Enterprise Model Pricing Ledger</h3>
                  <p class="text-xs text-[#8e95a8] mt-0.5">
                    Transparent rate per 1,000 tokens for models accessible by your credential
                  </p>
                </div>
                <span class="text-xs font-mono text-[#8e95a8]">Per 1K Tokens</span>
              </div>

              {#if !clientInfo.pricingTable || clientInfo.pricingTable.length === 0}
                <div class="p-10 text-center text-xs text-[#8e95a8]">
                  No custom tariff rules configured — standard pass-through rates apply.
                </div>
              {:else}
                <div class="overflow-x-auto">
                  <table class="w-full text-left border-collapse">
                    <thead>
                      <tr class="border-b border-white/[0.08] text-[11px] uppercase tracking-wider text-[#8e95a8] bg-white/[0.02]">
                        <th class="py-3 px-6 font-medium">Model Pattern</th>
                        <th class="py-3 px-6 font-medium text-right">Prompt Input (1K)</th>
                        <th class="py-3 px-6 font-medium text-right">Completion Output (1K)</th>
                        <th class="py-3 px-6 font-medium text-right">Est. Capacity (Your Balance)</th>
                      </tr>
                    </thead>
                    <tbody class="divide-y divide-white/[0.05] text-xs">
                      {#each clientInfo.pricingTable as price}
                        {@const avg1k = (price.priceInput1k + price.priceOutput1k) / 2}
                        {@const estTokens = clientInfo.balance > 0 && avg1k > 0 ? Math.floor((clientInfo.balance / avg1k) * 1000) : -1}
                        <tr class="hover:bg-white/[0.03] transition-colors">
                          <td class="py-3.5 px-6 font-mono text-xs text-white font-medium">
                            {price.modelPattern}
                          </td>
                          <td class="py-3.5 px-6 text-right font-mono text-xs text-[#d8dce6] tabular-nums">
                            Rp {price.priceInput1k.toLocaleString('id-ID')}
                          </td>
                          <td class="py-3.5 px-6 text-right font-mono text-xs text-[#d8dce6] tabular-nums">
                            Rp {price.priceOutput1k.toLocaleString('id-ID')}
                          </td>
                          <td class="py-3.5 px-6 text-right font-mono text-xs text-[#e56a4a] font-semibold tabular-nums">
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
          {:else if activeMainTab === 'trust'}
            <!-- ===============================================================
                 TAB 5: ENTERPRISE PRIVACY, REGULATIONS & COMPLIANCE CENTER
                 =============================================================== -->
            <div class="space-y-6">
              <!-- Live Security Posture Banner -->
              <section class="p-6 rounded-[22px] bg-surface border border-white/10 space-y-4">
                <div class="flex flex-wrap items-center justify-between gap-4">
                  <div class="space-y-1">
                    <div class="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-emerald-500/12 border border-emerald-500/25 text-xs text-emerald-300">
                      <ShieldCheck class="w-3.5 h-3.5" />
                      <span>Active Key Compliance Posture: VERIFIED</span>
                    </div>
                    <h3 class="text-lg font-semibold text-white pt-1">
                      Zero-Data Retention &amp; Regulatory Governance Attestation
                    </h3>
                    <p class="text-xs text-[#8e95a8]">
                      Your API credential (<code class="font-mono text-white">{clientInfo.keyMasked}</code>) is protected by hardware-isolated ephemeral routing and full regulatory compliance controls.
                    </p>
                  </div>

                  <button
                    type="button"
                    onclick={() =>
                      copyText(
                        `9ROUTER GALACTUS ENTERPRISE COMPLIANCE ATTESTATION\nKey: ${clientInfo?.keyMasked}\nZero-Data Retention (ZDR): ENFORCED (RAM-Only SSE)\nModel Identity Remasking: ACTIVE\nStandards: SOC 2 Type II, ISO/IEC 27001:2022, ISO/IEC 42001, GDPR DPA, UU PDP No. 27/2022 Indonesia\nTimestamp: ${new Date().toISOString()}`,
                        'copy-attestation'
                      )}
                    class="px-4 py-2 rounded-[13px] bg-[#e56a4a] text-white text-xs font-semibold cursor-pointer flex items-center gap-2"
                  >
                    {#if copiedId === 'copy-attestation'}
                      <Check class="w-4 h-4" />
                      <span>Attestation Copied!</span>
                    {:else}
                      <FileCheck2 class="w-4 h-4" />
                      <span>Copy Compliance Certificate</span>
                    {/if}
                  </button>
                </div>

                <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 pt-2">
                  <div class="p-4 rounded-[16px] bg-surface-2 border border-white/10 space-y-1">
                    <div class="text-[11px] text-[#8e95a8]">Data Retention Policy</div>
                    <div class="text-sm font-semibold text-emerald-400">0 Days (RAM-Only ZDR)</div>
                    <div class="text-[10px] text-[#8e95a8]">No disk persistence of prompts</div>
                  </div>
                  <div class="p-4 rounded-[16px] bg-surface-2 border border-white/10 space-y-1">
                    <div class="text-[11px] text-[#8e95a8]">Model Training Opt-Out</div>
                    <div class="text-sm font-semibold text-emerald-400">Strictly Prohibited</div>
                    <div class="text-[10px] text-[#8e95a8]">Contractual &amp; technical lock</div>
                  </div>
                  <div class="p-4 rounded-[16px] bg-surface-2 border border-white/10 space-y-1">
                    <div class="text-[11px] text-[#8e95a8]">Identity Remasking</div>
                    <div class="text-sm font-semibold text-white">Bidirectional Edge Mask</div>
                    <div class="text-[10px] text-[#8e95a8]">Strips headers &amp; fingerprints</div>
                  </div>
                  <div class="p-4 rounded-[16px] bg-surface-2 border border-white/10 space-y-1">
                    <div class="text-[11px] text-[#8e95a8]">Transport &amp; Vault Cipher</div>
                    <div class="text-sm font-semibold text-white">TLS 1.3 + AES-256-GCM</div>
                    <div class="text-[10px] text-[#8e95a8]">Authenticated encryption</div>
                  </div>
                </div>
              </section>

              <!-- 6-Card Regulatory & Compliance Matrix -->
              <section class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {#each COMPLIANCE_BADGES as item}
                  <button
                    type="button"
                    onclick={() => (activePolicyModal = item.doc)}
                    class="text-left p-5 rounded-[22px] bg-surface border border-white/10 hover:border-white/20 transition cursor-pointer flex flex-col justify-between gap-4"
                  >
                    <div class="space-y-2">
                      <div class="flex items-center justify-between gap-2">
                        <span class="text-sm font-semibold text-white">{item.code}</span>
                        <span class="px-2 py-0.5 rounded-full text-[10px] font-mono bg-emerald-500/12 text-emerald-300 border border-emerald-500/25">
                          {item.status}
                        </span>
                      </div>
                      <div class="text-xs font-medium text-[#d8dce6]">{item.title}</div>
                      <p class="text-xs text-[#8e95a8] leading-relaxed">{item.desc}</p>
                    </div>
                    <div class="flex items-center gap-1 text-[11px] text-[#e56a4a] font-medium">
                      <span>Inspect full legal &amp; technical clauses</span>
                      <ArrowUpRight class="w-3.5 h-3.5" />
                    </div>
                  </button>
                {/each}
              </section>
            </div>
          {/if}
        </main>
      </div>
    </div>
  {/if}

  <!-- =========================================================================
       iOS SMOKED GLASS POLICY & REGULATION DOSSIER MODAL
       ========================================================================= -->
  {#if activePolicyModal && POLICY_DOCS[activePolicyModal]}
    {@const doc = POLICY_DOCS[activePolicyModal]}
    <div
      class="fixed inset-0 z-50 bg-black/70 backdrop-blur-md flex items-center justify-center p-4"
      onclick={() => (activePolicyModal = null)}
      onkeydown={(e) => e.key === 'Escape' && (activePolicyModal = null)}
      role="dialog"
      tabindex="-1"
      aria-modal="true"
    >
      <div
        class="w-full max-w-2xl rounded-[24px] bg-surface border border-white/15 p-6 sm:p-8 space-y-6 max-h-[85vh] overflow-y-auto custom-scrollbar"
        onclick={(e) => e.stopPropagation()}
        onkeydown={(e) => e.stopPropagation()}
        role="document"
      >
        <div class="flex items-start justify-between gap-4 border-b border-white/10 pb-4">
          <div class="space-y-1">
            <span class="inline-block px-2.5 py-0.5 rounded-full text-[10px] font-mono bg-emerald-500/12 text-emerald-300 border border-emerald-500/25">
              {doc.code}
            </span>
            <h3 class="text-lg sm:text-xl font-semibold text-white">{doc.title}</h3>
            <p class="text-xs text-[#8e95a8]">{doc.subtitle}</p>
          </div>
          <button
            type="button"
            onclick={() => (activePolicyModal = null)}
            class="p-2 rounded-[12px] bg-surface-2 hover:bg-white/15 text-[#8e95a8] hover:text-white cursor-pointer"
            aria-label="Close modal"
          >
            <X class="w-4 h-4" />
          </button>
        </div>

        <div class="space-y-4">
          {#each doc.clauses as c}
            <div class="p-4 rounded-[16px] bg-surface-2 border border-white/[0.08] space-y-1.5">
              <div class="text-xs font-semibold text-white">{c.heading}</div>
              <p class="text-xs text-[#9ba3b8] leading-relaxed">{c.body}</p>
            </div>
          {/each}
        </div>

        <div class="flex items-center justify-between pt-2 border-t border-white/10">
          <span class="text-[11px] text-[#8e95a8]">9router Galactus Enterprise Trust &amp; Legal</span>
          <button
            type="button"
            onclick={() => (activePolicyModal = null)}
            class="px-4 py-2 rounded-[12px] bg-[#e56a4a] text-white text-xs font-semibold cursor-pointer"
          >
            Acknowledge &amp; Close
          </button>
        </div>
      </div>
    </div>
  {/if}

  <!-- =========================================================================
       iOS SMOKED GLASS AUTOMATED QRIS TOP-UP MODAL (OPSI 2)
       ========================================================================= -->
  {#if showTopupModal}
    <div
      class="fixed inset-0 z-50 bg-black/75 backdrop-blur-md flex items-center justify-center p-4"
      onclick={() => (showTopupModal = false)}
      onkeydown={(e) => e.key === 'Escape' && (showTopupModal = false)}
      role="dialog"
      tabindex="-1"
      aria-modal="true"
    >
      <div
        class="w-full max-w-lg rounded-[24px] bg-surface border border-white/15 p-6 sm:p-7 space-y-6 max-h-[90vh] overflow-y-auto custom-scrollbar shadow-2xl"
        onclick={(e) => e.stopPropagation()}
        onkeydown={(e) => e.stopPropagation()}
        role="document"
      >
        <!-- Modal Header -->
        <div class="flex items-start justify-between gap-4 border-b border-white/10 pb-4">
          <div class="space-y-1">
            <div class="flex items-center gap-2">
              <span class="px-2 py-0.5 rounded-full text-[10px] font-mono bg-emerald-500/12 text-emerald-300 border border-emerald-500/25">
                INSTANT QRIS TOP-UP
              </span>
              <span class="text-xs text-[#8e95a8] font-mono">BCA &bull; GoPay &bull; OVO &bull; Dana</span>
            </div>
            <h3 class="text-lg font-semibold text-white">Top Up Saldo Token AI</h3>
            <p class="text-xs text-[#8e95a8]">
              Saldo bertambah otomatis ke API key <code class="font-mono text-[#e56a4a]">{clientInfo?.keyMasked}</code>
            </p>
          </div>
          <button
            type="button"
            onclick={() => (showTopupModal = false)}
            class="p-2 rounded-[12px] bg-surface-2 hover:bg-white/15 text-[#8e95a8] hover:text-white cursor-pointer"
            aria-label="Close modal"
          >
            <X class="w-4 h-4" />
          </button>
        </div>

        {#if topupStep === 'packages'}
          <!-- STEP 1: PILIH PAKET NOMINAL -->
          <div class="space-y-4">
            <div class="text-xs font-medium text-[#d8dce6]">
              Pilih Paket Top-Up Saldo:
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              {#each TOPUP_PACKAGES as pkg}
                <button
                  type="button"
                  onclick={() => (selectedTopupAmount = pkg.amount)}
                  class="p-4 rounded-[16px] text-left border transition cursor-pointer flex flex-col justify-between gap-2.5 {selectedTopupAmount === pkg.amount
                    ? 'bg-[#e56a4a]/20 border-[#e56a4a] text-white ring-1 ring-[#e56a4a]'
                    : 'bg-surface-2 border-white/10 text-[#8e95a8] hover:border-white/20 hover:text-white'}"
                >
                  <div class="flex items-center justify-between gap-2">
                    <span class="text-base font-semibold text-white font-mono">{pkg.label}</span>
                    {#if pkg.popular}
                      <span class="px-2 py-0.5 rounded-full text-[9px] font-bold bg-[#e56a4a] text-white">
                        {pkg.badge}
                      </span>
                    {:else}
                      <span class="px-2 py-0.5 rounded-full text-[9px] font-medium bg-white/10 text-[#d8dce6]">
                        {pkg.badge}
                      </span>
                    {/if}
                  </div>
                  <div class="text-[11px] text-[#8e95a8]">
                    Estimasi kuota: <span class="text-[#d8dce6] font-medium">{pkg.estTokens}</span>
                  </div>
                </button>
              {/each}
            </div>

            <div class="p-3.5 rounded-[14px] bg-surface-2 border border-white/10 flex items-center justify-between text-xs">
              <span class="text-[#8e95a8]">Nominal Pembayaran:</span>
              <span class="text-base font-semibold text-white font-mono">
                Rp {selectedTopupAmount.toLocaleString('id-ID')}
              </span>
            </div>

            <button
              type="button"
              onclick={proceedToQris}
              class="w-full py-3 rounded-[13px] bg-[#e56a4a] hover:bg-[#ee7656] text-white font-semibold text-xs transition flex items-center justify-center gap-2 cursor-pointer shadow-lg"
            >
              <QrCode class="w-4 h-4" />
              <span>Lanjut Bayar via QRIS Instant</span>
            </button>
          </div>
        {:else if topupStep === 'qris'}
          <!-- STEP 2: TAMPILAN QRIS DINAMIS & SIMULASI VERIFIKASI INSTAN -->
          <div class="space-y-5 text-center">
            <div class="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-amber-500/12 border border-amber-500/25 text-xs text-amber-300">
              <span class="w-2 h-2 rounded-full bg-amber-400 animate-pulse"></span>
              <span>Menunggu Pembayaran &bull; Sisa Waktu: {formattedQrisTime}</span>
            </div>

            <!-- Dynamic Mock QRIS Container -->
            <div class="mx-auto size-52 p-3 rounded-[20px] bg-white border-2 border-white flex flex-col items-center justify-between shadow-xl">
              <div class="text-[10px] font-bold tracking-widest text-slate-800 uppercase">
                QRIS &bull; GALAXY AI NETWORK
              </div>

              <!-- SVG Procedural QR Code Graphic -->
              <svg class="size-36 text-slate-900" viewBox="0 0 100 100" fill="currentColor">
                <!-- Outer Corners -->
                <rect x="5" y="5" width="28" height="28" rx="4" fill="none" stroke="currentColor" stroke-width="6" />
                <rect x="13" y="13" width="12" height="12" rx="2" />
                <rect x="67" y="5" width="28" height="28" rx="4" fill="none" stroke="currentColor" stroke-width="6" />
                <rect x="75" y="13" width="12" height="12" rx="2" />
                <rect x="5" y="67" width="28" height="28" rx="4" fill="none" stroke="currentColor" stroke-width="6" />
                <rect x="13" y="75" width="12" height="12" rx="2" />
                <!-- Center Data Grid -->
                <rect x="42" y="8" width="8" height="8" />
                <rect x="52" y="16" width="8" height="8" />
                <rect x="42" y="26" width="8" height="8" />
                <rect x="8" y="42" width="8" height="8" />
                <rect x="22" y="42" width="8" height="8" />
                <rect x="36" y="42" width="12" height="12" />
                <rect x="52" y="42" width="8" height="8" />
                <rect x="66" y="42" width="8" height="8" />
                <rect x="80" y="42" width="12" height="8" />
                <rect x="42" y="60" width="8" height="8" />
                <rect x="56" y="58" width="12" height="12" />
                <rect x="74" y="60" width="8" height="8" />
                <rect x="42" y="76" width="12" height="8" />
                <rect x="64" y="76" width="8" height="12" />
                <rect x="80" y="76" width="8" height="8" />
              </svg>

              <div class="text-[9px] font-mono text-slate-600">
                Total: Rp {selectedTopupAmount.toLocaleString('id-ID')}
              </div>
            </div>

            <div class="p-3.5 rounded-[14px] bg-surface-2 border border-white/10 text-xs text-[#8e95a8] leading-relaxed">
              Buka aplikasi m-Banking (BCA, Mandiri, BRI) atau e-Wallet (GoPay, OVO, ShopeePay, Dana), lalu scan QRIS di atas.
            </div>

            <!-- Action buttons: Simulasi Verifikasi Instan & Tombol Kembali -->
            <div class="space-y-2">
              <button
                type="button"
                onclick={simulatePaymentConfirmation}
                disabled={simulatedPaymentLoading}
                class="w-full py-3 rounded-[13px] bg-emerald-600 hover:bg-emerald-500 text-white font-semibold text-xs transition flex items-center justify-center gap-2 cursor-pointer disabled:opacity-50 shadow-md"
              >
                {#if simulatedPaymentLoading}
                  <RefreshCw class="w-4 h-4 animate-spin" />
                  <span>Memverifikasi Pembayaran...</span>
                {:else}
                  <CheckCircle2 class="w-4 h-4" />
                  <span>Simulasi Bayar / Sudah Bayar</span>
                {/if}
              </button>

              <button
                type="button"
                onclick={() => (topupStep = 'packages')}
                class="w-full py-2 rounded-[11px] text-xs text-[#8e95a8] hover:text-white transition cursor-pointer"
              >
                Ganti Nominal Lain
              </button>
            </div>
          </div>
        {:else if topupStep === 'success'}
          <!-- STEP 3: SUKSES SALDO TERISI -->
          <div class="space-y-5 text-center py-4">
            <div class="size-16 rounded-full bg-emerald-500/15 border-2 border-emerald-400 text-emerald-400 mx-auto flex items-center justify-center">
              <CheckCircle2 class="w-8 h-8" />
            </div>

            <div class="space-y-1">
              <h4 class="text-lg font-semibold text-white">Top-Up Berhasil!</h4>
              <p class="text-xs text-[#8e95a8]">
                Saldo sebesar <strong class="text-white font-mono">Rp {selectedTopupAmount.toLocaleString('id-ID')}</strong> berhasil ditambahkan ke API key Anda.
              </p>
            </div>

            <div class="p-4 rounded-[14px] bg-surface-2 border border-white/10 text-xs flex justify-between items-center">
              <span class="text-[#8e95a8]">Saldo Sekarang:</span>
              <span class="text-base font-semibold text-emerald-400 font-mono">
                Rp {animatedBalance.toLocaleString('id-ID')}
              </span>
            </div>

            <button
              type="button"
              onclick={() => (showTopupModal = false)}
              class="w-full py-3 rounded-[13px] bg-[#e56a4a] hover:bg-[#ee7656] text-white font-semibold text-xs transition cursor-pointer"
            >
              Kembali ke Konsol
            </button>
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>
