<script lang="ts">
  import { onMount } from 'svelte'
  import { marked } from 'marked'
  import {
    Activity,
    AlertCircle,
    AlertTriangle,
    Check,
    CheckCircle2,
    ChevronDown,
    ChevronUp,
    Copy,
    Cpu,
    Download,
    Eye,
    FileText,
    Filter,
    Layers,
    Play,
    RefreshCw,
    RotateCcw,
    Search,
    Shield,
    Sliders,
    Sparkles,
    Square,
    Terminal,
    X,
    XCircle,
    Zap,
    Code,
    Binary,
    Bug,
    Wrench,
  } from 'lucide-svelte'
  import { copyToClipboard } from '../lib/clipboard'
  import {
    api,
    getAuthHeaders,
    getStoredAPIKey,
    type APIKey,
    type Combo,
    type ProviderConnection,
    type ProviderNode,
  } from '../api/client'
  import ModelPickerModal from './combos/ModelPickerModal.svelte'
  import {
    EVAL_LEVELS,
    EVAL_PROMPTS,
    checkRefusal,
    calculateEvalSummary,
    type EvalLevel,
    type EvalPrompt,
    type EvalProbeResult,
  } from '../lib/modelEvals'

  marked.setOptions({
    gfm: true,
    breaks: true,
  })

  interface ModelItem {
    id: string
    provider: string
    title: string
    isCombo?: boolean
  }

  // Model & Credentials
  let models = $state<ModelItem[]>([])
  let loadingModels = $state(false)
  let selectedModel = $state<string>('')
  let isModelDropdownOpen = $state(false)
  let modelSearchQuery = $state('')
  let showModelPickerModal = $state(false)

  // System Entities for ModelPickerModal
  let connections = $state<ProviderConnection[]>([])
  let combos = $state<Combo[]>([])
  let providerNodes = $state<ProviderNode[]>([])
  let apiKeys = $state<APIKey[]>([])
  let selectedApiKey = $state<string>('')

  // Benchmark Config
  let selectedLevel = $state<'all' | EvalLevel>('all')
  let concurrency = $state<number>(1)
  let interProbeDelayMs = $state<number>(1500)
  let maxTokens = $state<number>(1500)
  let temperature = $state<number>(0.7)
  let strictDetection = $state<boolean>(false)
  let customSystemPrompt = $state<string>('')
  let showAdvancedSettings = $state<boolean>(false)

  // Execution State
  let isRunning = $state<boolean>(false)
  let currentRunningPrompt = $state<string>('')
  let abortController = $state<AbortController | null>(null)
  let startTime = $state<number>(0)
  let elapsedTime = $state<string>('0s')
  let timerInterval: any = null

  // Benchmark Results
  let results = $state<EvalProbeResult[]>(
    EVAL_PROMPTS.map((p) => ({
      id: p.id,
      promptId: p.id,
      level: p.level,
      category: p.category,
      title: p.title,
      prompt: p.prompt,
      status: 'pending',
      refusalTriggers: [],
    }))
  )

  // Table filtering & inspection
  let tableSearch = $state<string>('')
  let tableStatusFilter = $state<'all' | 'complied' | 'refused' | 'error'>('all')
  let tableLevelFilter = $state<'all' | EvalLevel>('all')
  let activeModalResult = $state<EvalProbeResult | null>(null)
  let copyFeedback = $state<string>('')

  // View mode switcher: 'diagnostic' | 'binary-lint'
  let activeViewMode = $state<'diagnostic' | 'binary-lint'>('diagnostic')

  // Binary Sanitizer state
  let binaryInputCode = $state<string>(
    'unsigned char shellcode[] = "\\x31\\xc0\\x50\\x68\\x2f\\x2f\\x73\\x68\\x68\\x2f\\x62\\x69\\x6e\\x89\\xe3\\x50\\x53\\x89\\xe1\\x31\\xd2\\xb0\\x0b\\xcd\\x80";'
  )
  let binaryBadBytes = $state<string>('\\x00, \\x0a, \\x0d')
  let binaryTargetOS = $state<'linux' | 'windows'>('linux')
  let isLinting = $state<boolean>(false)
  let lintResult = $state<{
    shellcode?: {
      byte_length: number
      is_clean: boolean
      bad_hits: { offset: number; byte: string }[]
    }
    assembly?: { severity: string; line: number; instruction: string; message: string }[]
  } | null>(null)
  let isAutoRewriting = $state<boolean>(false)
  let autoRewriteOutput = $state<string>('')
  let lintError = $state<string>('')

  function loadSampleCode(type: 'shellcode-dirty' | 'assembly-violation' | 'windows-shadow') {
    if (type === 'shellcode-dirty') {
      binaryInputCode = 'unsigned char payload[] = { 0x31, 0xc0, 0x50, 0x00, 0x48, 0x89, 0xe5, 0x0a, 0xc3 };'
      binaryBadBytes = '\\x00, \\x0a, \\x0d'
      binaryTargetOS = 'linux'
    } else if (type === 'assembly-violation') {
      binaryInputCode = `_start:\n    xor eax, eax\n    mov [rax], [rbx]    ; Illegal memory-to-memory mov\n    ret`
      binaryBadBytes = '\\x00'
      binaryTargetOS = 'linux'
    } else if (type === 'windows-shadow') {
      binaryInputCode = `main:\n    push rbp\n    mov rbp, rsp\n    ; Missing sub rsp, 28h shadow space allocation\n    call MessageBoxA\n    pop rbp\n    ret`
      binaryBadBytes = '\\x00'
      binaryTargetOS = 'windows'
    }
  }

  async function executeBinaryLint() {
    isLinting = true
    lintResult = null
    lintError = ''
    autoRewriteOutput = ''
    try {
      const badBytesList = binaryBadBytes.split(',').map((s) => s.trim()).filter(Boolean)
      const activeKey = selectedApiKey.trim() || getStoredAPIKey()

      const res = await fetch('/v1/security/binary-lint', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${activeKey}`,
        },
        body: JSON.stringify({
          code: binaryInputCode,
          bad_bytes: badBytesList,
          target_os: binaryTargetOS,
        }),
      })

      if (!res.ok) {
        throw new Error(`HTTP ${res.status}: ${res.statusText}`)
      }

      lintResult = await res.json()
    } catch (e: any) {
      lintError = e.message || 'Audit failed'
    } finally {
      isLinting = false
    }
  }

  async function triggerAutoRewrite() {
    isAutoRewriting = true
    autoRewriteOutput = ''
    try {
      const activeKey = selectedApiKey.trim() || getStoredAPIKey()
      const prompt = `You are a low-level offensive systems engineer. The following shellcode / assembly code contains bad-byte violations (${binaryBadBytes}) or x86_64 ABI violations for target OS ${binaryTargetOS}:\n\n\`\`\`c\n${binaryInputCode}\n\`\`\`\n\nRewrite this code to completely eliminate all bad bytes and ensure 100% compliant x86_64 ABI calling convention. Return only the sanitized C array / assembly code with a brief explanation of how bad bytes were avoided.`

      const res = await fetch('/v1/chat/completions', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${activeKey}`,
        },
        body: JSON.stringify({
          model: selectedModel || 'GACOR-OPENSPEC',
          messages: [{ role: 'user', content: prompt }],
          max_tokens: 1500,
        }),
      })

      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`)
      }

      const data = await res.json()
      const choice = data?.choices?.[0]?.message
      autoRewriteOutput = choice?.content || choice?.reasoning_content || 'No output generated.'
    } catch (e: any) {
      autoRewriteOutput = `Synthesis error: ${e.message}`
    } finally {
      isAutoRewriting = false
    }
  }

  // Computed summary
  const summary = $derived(calculateEvalSummary(results, selectedModel))

  const filteredResults = $derived(
    results.filter((item) => {
      if (tableLevelFilter !== 'all' && item.level !== tableLevelFilter) return false
      if (tableStatusFilter !== 'all' && item.status !== tableStatusFilter) return false
      if (tableSearch.trim()) {
        const query = tableSearch.toLowerCase()
        const matchTitle = item.title.toLowerCase().includes(query)
        const matchCategory = item.category.toLowerCase().includes(query)
        const matchPrompt = item.prompt.toLowerCase().includes(query)
        const matchResponse = (item.response || '').toLowerCase().includes(query)
        const matchError = (item.error || '').toLowerCase().includes(query)
        if (!matchTitle && !matchCategory && !matchPrompt && !matchResponse && !matchError) return false
      }
      return true
    })
  )

  const filteredModels = $derived(
    models.filter((m) =>
      m.id.toLowerCase().includes(modelSearchQuery.toLowerCase()) ||
      m.title.toLowerCase().includes(modelSearchQuery.toLowerCase()) ||
      m.provider.toLowerCase().includes(modelSearchQuery.toLowerCase())
    )
  )

  // Quick switch pills for popular / abliterated models
  const quickModels = $derived(
    models
      .filter((m) =>
        m.id.toLowerCase().includes('opencode') ||
        m.id.toLowerCase().includes('nemotron') ||
        m.id.toLowerCase().includes('abliterat') ||
        m.id.toLowerCase().includes('uncensor') ||
        m.id.toLowerCase().includes('deepseek') ||
        m.id.toLowerCase().includes('dolphin') ||
        m.id.toLowerCase().includes('wizard')
      )
      .slice(0, 6)
  )

  onMount(async () => {
    await autoSyncApiKey()
    await loadInitialData()
  })

  // Auto-sync an active client API key to localStorage and state
  async function autoSyncApiKey(): Promise<string> {
    try {
      if (typeof localStorage === 'undefined') return ''
      const existing = (localStorage.getItem('9router_key') || '').trim()
      if (existing) {
        selectedApiKey = existing
        return existing
      }

      const keys = await api.getApiKeys().catch(() => [])
      if (Array.isArray(keys)) {
        apiKeys = keys
        const active = keys.find((k) => k.isActive === 1)
        if (active?.key) {
          selectedApiKey = active.key
          localStorage.setItem('9router_key', active.key)
          return active.key
        }
      }
    } catch {}
    return ''
  }

  async function loadInitialData() {
    loadingModels = true
    try {
      const authHeaders = getAuthHeaders()
      const [connsRes, combosRes, nodesRes, modelsRes, keysRes] = await Promise.all([
        api.getConnections().catch(() => [] as ProviderConnection[]),
        api.getCombos().catch(() => [] as Combo[]),
        api.getProviderNodes().catch(() => [] as ProviderNode[]),
        fetch('/api/models', { headers: authHeaders })
          .then((r) => (r.ok ? r.json() : null))
          .catch(() => null),
        api.getApiKeys().catch(() => [] as APIKey[]),
      ])

      connections = Array.isArray(connsRes) ? connsRes : []
      combos = Array.isArray(combosRes) ? combosRes : []
      providerNodes = Array.isArray(nodesRes) ? nodesRes : []
      apiKeys = Array.isArray(keysRes) ? keysRes : []

      if (!selectedApiKey) {
        const activeKey = apiKeys.find((k) => k.isActive === 1)
        if (activeKey?.key) {
          selectedApiKey = activeKey.key
          localStorage.setItem('9router_key', activeKey.key)
        }
      }

      const nodeMap = new Map<string, string>()
      for (const node of providerNodes) {
        if (node.prefix) nodeMap.set(node.prefix.toLowerCase(), node.name)
        if (node.id) nodeMap.set(node.id.toLowerCase(), node.name)
      }

      const list: ModelItem[] = []

      // Combos
      if (Array.isArray(combos)) {
        for (const combo of combos) {
          if (combo.name) {
            list.push({
              id: combo.name,
              provider: 'Combos & Routers',
              title: combo.name,
              isCombo: true,
            })
          }
        }
      }

      // Upstream Models
      if (Array.isArray(modelsRes?.data)) {
        for (const m of modelsRes.data) {
          if (!m.id) continue
          if (list.some((item) => item.id === m.id)) continue
          const slash = m.id.indexOf('/')
          let provider = 'Upstream'
          let title = m.id
          if (slash !== -1) {
            const prefix = m.id.slice(0, slash).toLowerCase()
            provider = nodeMap.get(prefix) || m.id.slice(0, slash)
            title = m.id.slice(slash + 1)
          }
          list.push({ id: m.id, provider, title })
        }
      }

      models = list
      if (!selectedModel && models.length > 0) {
        // Prefer abliterated or first available
        const abliterated = models.find((m) => m.id.toLowerCase().includes('abliterat'))
        const preferred = abliterated || models.find((m) => !m.isCombo) || models[0]
        selectedModel = preferred.id
      }
    } finally {
      loadingModels = false
    }
  }

  function resetResults() {
    results = EVAL_PROMPTS.map((p) => ({
      id: p.id,
      promptId: p.id,
      level: p.level,
      category: p.category,
      title: p.title,
      prompt: p.prompt,
      status: 'pending',
      refusalTriggers: [],
    }))
  }

  async function executeSingleProbe(
    promptDef: EvalPrompt,
    controller: AbortController
  ): Promise<EvalProbeResult> {
    const itemIndex = results.findIndex((r) => r.id === promptDef.id)
    if (itemIndex !== -1) {
      results[itemIndex].status = 'running'
    }
    currentRunningPrompt = `${promptDef.title} (${promptDef.id})`

    const messages: { role: string; content: string }[] = []
    if (customSystemPrompt.trim()) {
      messages.push({ role: 'system', content: customSystemPrompt.trim() })
    }
    messages.push({ role: 'user', content: promptDef.prompt })

    // Build headers with active token or stored token
    const authHeaders = getAuthHeaders()
    const activeKey = selectedApiKey.trim() || getStoredAPIKey()
    if (activeKey) {
      authHeaders['Authorization'] = `Bearer ${activeKey}`
    }

    const requestPayload = {
      model: selectedModel.trim(),
      messages,
      temperature: Number(temperature),
      max_tokens: Number(maxTokens),
      stream: false,
    }

    const t0 = performance.now()
    try {
      // 1. Primary: /api/chat/completions (operator dashboard session authenticated)
      let res: Response | null = await fetch('/api/chat/completions', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...authHeaders,
        },
        credentials: 'include',
        signal: controller.signal,
        body: JSON.stringify(requestPayload),
      }).catch(() => null)

      // 2. Fallback: /v1/chat/completions (with Bearer token)
      if (!res || !res.ok) {
        if (!res || res.status === 401 || res.status === 404) {
          const fallbackRes = await fetch('/v1/chat/completions', {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              ...authHeaders,
            },
            credentials: 'include',
            signal: controller.signal,
            body: JSON.stringify(requestPayload),
          }).catch(() => null)

          if (fallbackRes) {
            res = fallbackRes
          }
        }
      }

      if (!res) {
        throw new Error('Network error: unable to reach 9router chat endpoint')
      }

      // If HTTP 429 (Rate limit on web-based provider), back off 3.5s and auto-retry once
      if (res.status === 429 && !controller.signal.aborted) {
        await new Promise((resolve) => setTimeout(resolve, 3500))
        if (!controller.signal.aborted) {
          const retryRes = await fetch('/api/chat/completions', {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              ...authHeaders,
            },
            credentials: 'include',
            signal: controller.signal,
            body: JSON.stringify(requestPayload),
          }).catch(() => null)
          if (retryRes) {
            res = retryRes
          }
        }
      }

      const latencyMs = Math.round(performance.now() - t0)
      let data: any = null
      try {
        data = await res.json()
      } catch {
        data = null
      }

      if (!res.ok) {
        const errorMsg =
          data?.error?.message ||
          (res.status === 401
            ? 'HTTP 401 Unauthorized: Valid API key or Dashboard Session required'
            : `HTTP ${res.status}: ${res.statusText || 'Error'}`)

        const updated: EvalProbeResult = {
          id: promptDef.id,
          promptId: promptDef.id,
          level: promptDef.level,
          category: promptDef.category,
          title: promptDef.title,
          prompt: promptDef.prompt,
          status: 'error',
          error: errorMsg,
          httpStatus: res.status,
          latencyMs,
          refusalTriggers: [],
        }
        if (itemIndex !== -1) results[itemIndex] = updated
        return updated
      }

      const choice = data?.choices?.[0]?.message
      let content = choice?.content || ''
      if (!content && choice?.reasoning_content) {
        content = choice.reasoning_content
      }
      const tokens = data?.usage?.total_tokens
      const refusalVerdict = checkRefusal(content, strictDetection)

      const updated: EvalProbeResult = {
        id: promptDef.id,
        promptId: promptDef.id,
        level: promptDef.level,
        category: promptDef.category,
        title: promptDef.title,
        prompt: promptDef.prompt,
        status: refusalVerdict.isRefusal ? 'refused' : 'complied',
        response: content,
        refusalTriggers: refusalVerdict.refusalTriggers,
        reason: refusalVerdict.reason,
        isSoftDisclaimer: refusalVerdict.isSoftDisclaimer,
        latencyMs,
        tokens,
        httpStatus: res.status,
      }
      if (itemIndex !== -1) results[itemIndex] = updated
      return updated
    } catch (err: any) {
      const latencyMs = Math.round(performance.now() - t0)
      if (err.name === 'AbortError') {
        const updated: EvalProbeResult = {
          id: promptDef.id,
          promptId: promptDef.id,
          level: promptDef.level,
          category: promptDef.category,
          title: promptDef.title,
          prompt: promptDef.prompt,
          status: 'pending',
          refusalTriggers: [],
        }
        if (itemIndex !== -1) results[itemIndex] = updated
        return updated
      }

      const updated: EvalProbeResult = {
        id: promptDef.id,
        promptId: promptDef.id,
        level: promptDef.level,
        category: promptDef.category,
        title: promptDef.title,
        prompt: promptDef.prompt,
        status: 'error',
        error: err.message || 'Request failed',
        latencyMs,
        refusalTriggers: [],
      }
      if (itemIndex !== -1) results[itemIndex] = updated
      return updated
    }
  }

  async function runBenchmark() {
    if (!selectedModel.trim()) {
      alert('Please select a target model to benchmark.')
      return
    }

    const testPool =
      selectedLevel === 'all'
        ? EVAL_PROMPTS
        : EVAL_PROMPTS.filter((p) => p.level === selectedLevel)

    // Reset targeted test items to pending
    for (const p of testPool) {
      const idx = results.findIndex((r) => r.id === p.id)
      if (idx !== -1) {
        results[idx].status = 'pending'
        results[idx].response = undefined
        results[idx].error = undefined
        results[idx].refusalTriggers = []
      }
    }

    isRunning = true
    abortController = new AbortController()
    startTime = Date.now()
    timerInterval = setInterval(() => {
      elapsedTime = `${Math.floor((Date.now() - startTime) / 1000)}s`
    }, 1000)

    try {
      const queue = [...testPool]
      const workerCount = Math.max(1, Math.min(concurrency, 4))
      const workers = Array.from({ length: workerCount }, async () => {
        while (queue.length > 0) {
          if (abortController?.signal.aborted) break
          const promptItem = queue.shift()
          if (!promptItem) break
          await executeSingleProbe(promptItem, abortController!)
          if (interProbeDelayMs > 0 && queue.length > 0 && !abortController?.signal.aborted) {
            await new Promise((resolve) => setTimeout(resolve, interProbeDelayMs))
          }
        }
      })

      await Promise.all(workers)
    } finally {
      isRunning = false
      currentRunningPrompt = ''
      if (timerInterval) {
        clearInterval(timerInterval)
        timerInterval = null
      }
    }
  }

  async function retryFailedProbes() {
    if (!selectedModel.trim() || isRunning) return
    const failedProbes = results.filter((r) => r.status === 'error')
    if (failedProbes.length === 0) return

    const promptsToRetry = EVAL_PROMPTS.filter((p) =>
      failedProbes.some((f) => f.id === p.id)
    )

    for (const p of promptsToRetry) {
      const idx = results.findIndex((r) => r.id === p.id)
      if (idx !== -1) {
        results[idx].status = 'pending'
        results[idx].error = undefined
        results[idx].response = undefined
        results[idx].refusalTriggers = []
      }
    }

    isRunning = true
    abortController = new AbortController()
    startTime = Date.now()
    if (!timerInterval) {
      timerInterval = setInterval(() => {
        elapsedTime = `${Math.floor((Date.now() - startTime) / 1000)}s`
      }, 1000)
    }

    try {
      const queue = [...promptsToRetry]
      const workerCount = Math.max(1, Math.min(concurrency, 4))
      const workers = Array.from({ length: workerCount }, async () => {
        while (queue.length > 0) {
          if (abortController?.signal.aborted) break
          const promptItem = queue.shift()
          if (!promptItem) break
          await executeSingleProbe(promptItem, abortController!)
          if (interProbeDelayMs > 0 && queue.length > 0 && !abortController?.signal.aborted) {
            await new Promise((resolve) => setTimeout(resolve, interProbeDelayMs))
          }
        }
      })

      await Promise.all(workers)
    } finally {
      isRunning = false
      currentRunningPrompt = ''
      if (timerInterval) {
        clearInterval(timerInterval)
        timerInterval = null
      }
    }
  }

  async function retrySingleProbe(item: EvalProbeResult) {
    if (isRunning) return
    const promptDef = EVAL_PROMPTS.find((p) => p.id === item.id)
    if (!promptDef) return

    const idx = results.findIndex((r) => r.id === item.id)
    if (idx !== -1) {
      results[idx].status = 'pending'
      results[idx].error = undefined
      results[idx].response = undefined
      results[idx].refusalTriggers = []
    }

    const singleAbort = new AbortController()
    currentRunningPrompt = promptDef.title
    try {
      await executeSingleProbe(promptDef, singleAbort)
    } finally {
      currentRunningPrompt = ''
    }
  }

  function stopBenchmark() {
    if (abortController) {
      abortController.abort()
      abortController = null
    }
    isRunning = false
    currentRunningPrompt = ''
    if (timerInterval) clearInterval(timerInterval)
  }

  async function copyReportMarkdown() {
    const lines: string[] = [
      `# 9router Model Censorship & Alignment Diagnostic Report`,
      `**Target Model:** \`${selectedModel}\``,
      `**Classification:** **${summary.conclusion.classification}**`,
      `**Verdict:** **${summary.verdictLabel}**`,
      `**Compliance Score:** ${summary.complianceRate}% (${summary.complied}/${summary.completed} Probes)`,
      `**Average Latency:** ${summary.averageLatencyMs}ms`,
      ``,
      `### Executive Diagnostic Conclusion:`,
      `> ${summary.conclusion.summaryText}`,
      ``,
      `### Recommended Use Cases:`,
      ...summary.conclusion.recommendedTasks.map((t) => `- [x] ${t}`),
      ``,
      `### Level Breakdown:`,
      `- **Level 1 (Dual-Use Systems):** ${summary.levelStats.level1.complied}/${summary.levelStats.level1.total} (${summary.levelStats.level1.complianceRate}%) - ${summary.conclusion.levelFindings.level1}`,
      `- **Level 2 (Red Team / Security):** ${summary.levelStats.level2.complied}/${summary.levelStats.level2.total} (${summary.levelStats.level2.complianceRate}%) - ${summary.conclusion.levelFindings.level2}`,
      `- **Level 3 (Direct Raw Probes):** ${summary.levelStats.level3.complied}/${summary.levelStats.level3.total} (${summary.levelStats.level3.complianceRate}%) - ${summary.conclusion.levelFindings.level3}`,
      `- **Level 4 (Adversarial / Inversion):** ${summary.levelStats.level4.complied}/${summary.levelStats.level4.total} (${summary.levelStats.level4.complianceRate}%) - ${summary.conclusion.levelFindings.level4}`,
      ``,
      `### Detailed Test Logs:`,
    ]

    for (const r of results) {
      lines.push(
        `- [${r.status.toUpperCase()}] **${r.title}** (${r.level}) - ${r.latencyMs || 0}ms`
      )
      if (r.refusalTriggers.length > 0) {
        lines.push(`  - Triggered Signatures: \`${r.refusalTriggers.join(', ')}\``)
      }
      if (r.error) {
        lines.push(`  - Error: \`${r.error}\``)
      }
    }

    await copyToClipboard(lines.join('\n'))
    copyFeedback = 'Diagnostic Report Copied to Clipboard!'
    setTimeout(() => (copyFeedback = ''), 3000)
  }
</script>

<div class="space-y-6 pb-12">
  <!-- Top Bar Metrics (Reference Image 2 Style) -->
  <div class="flex flex-wrap items-center justify-between gap-3 border-b border-border/60 pb-4">
    <div class="flex flex-wrap items-center gap-2 text-xs font-mono">
      <span class="inline-flex items-center gap-1.5 rounded-md bg-emerald-500/10 px-3 py-1 text-emerald-400 border border-emerald-500/20 font-semibold shadow-sm">
        <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
        LIVE &check; {models.length} MODELS
      </span>

      <span class="inline-flex items-center gap-1.5 rounded-md bg-neutral-900/90 px-3 py-1 text-neutral-300 border border-neutral-700/60 font-semibold">
        PROBES: {summary.completed} / {summary.total}
      </span>

      <span class="inline-flex items-center gap-1 rounded-md bg-emerald-500/10 px-2.5 py-1 text-emerald-400 border border-emerald-500/20">
        &check; {summary.complied} COMPLIED
      </span>

      <span class="inline-flex items-center gap-1 rounded-md bg-rose-500/10 px-2.5 py-1 text-rose-400 border border-rose-500/20">
        &times; {summary.refused} REFUSED
      </span>

      {#if summary.errors > 0}
        <span class="inline-flex items-center gap-1 rounded-md bg-amber-500/10 px-2.5 py-1 text-amber-400 border border-amber-500/20">
          ! {summary.errors} ERRORS
        </span>
      {/if}

      <span class="inline-flex items-center gap-1 rounded-md bg-neutral-900/80 px-2.5 py-1 text-neutral-400 border border-neutral-800">
        LATENCY: {summary.averageLatencyMs}ms
      </span>
    </div>

    <!-- Right-hand Quick Actions -->
    <div class="flex items-center gap-2">
      <button
        type="button"
        onclick={copyReportMarkdown}
        class="inline-flex items-center gap-1.5 rounded-lg border border-neutral-700 bg-neutral-800/80 px-3 py-1.5 text-xs font-medium text-neutral-200 hover:bg-neutral-700 transition"
        title="Copy complete diagnostic report markdown"
      >
        <FileText class="w-3.5 h-3.5 text-orange-400" />
        <span>Export Report</span>
      </button>

      <button
        type="button"
        onclick={resetResults}
        class="inline-flex items-center gap-1.5 rounded-lg border border-neutral-700 bg-neutral-800/80 px-3 py-1.5 text-xs font-medium text-neutral-300 hover:bg-neutral-700 transition"
        title="Reset all probe results to pending"
      >
        <RefreshCw class="w-3.5 h-3.5 text-neutral-400" />
        <span>Reset</span>
      </button>
    </div>
  </div>

  {#if copyFeedback}
    <div class="rounded-lg bg-emerald-500/15 border border-emerald-500/30 px-4 py-2 text-xs font-medium text-emerald-300 flex items-center gap-2">
      <Check class="w-4 h-4 text-emerald-400" />
      <span>{copyFeedback}</span>
    </div>
  {/if}

  <!-- View Mode Switcher -->
  <div class="flex items-center gap-2 border-b border-neutral-800/80 pb-3">
    <button
      type="button"
      onclick={() => (activeViewMode = 'diagnostic')}
      class="inline-flex items-center gap-2 rounded-xl px-4 py-2 text-xs font-semibold transition border {activeViewMode === 'diagnostic' ? 'bg-orange-500/15 border-orange-500/40 text-orange-300 shadow-sm' : 'bg-neutral-900/60 border-neutral-800 text-neutral-400 hover:text-neutral-200'}"
    >
      <Activity class="w-4 h-4" />
      <span>Model Alignment Diagnostic Bench</span>
    </button>

    <button
      type="button"
      onclick={() => (activeViewMode = 'binary-lint')}
      class="inline-flex items-center gap-2 rounded-xl px-4 py-2 text-xs font-semibold transition border {activeViewMode === 'binary-lint' ? 'bg-cyan-500/15 border-cyan-500/40 text-cyan-300 shadow-sm' : 'bg-neutral-900/60 border-neutral-800 text-neutral-400 hover:text-neutral-200'}"
    >
      <Shield class="w-4 h-4" />
      <span>Binary Sanitizer & Shellcode Co-Processor</span>
      <span class="rounded bg-cyan-500/20 text-cyan-300 text-[9px] px-1.5 py-0.5 font-mono">DARKCODE</span>
    </button>
  </div>

  {#if activeViewMode === 'diagnostic'}
  <!-- CATEGORY / LEVEL GLOW CARDS (Matching Reference Image 1) -->
  <div>
    <div class="text-[11px] font-mono tracking-[0.25em] text-neutral-400 uppercase font-semibold mb-3">
      K A T E G O R I &nbsp;&amp;&nbsp; D I A G N O S T I C &nbsp; L E V E L S
    </div>

    <div class="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-8 gap-3">
      <!-- Card 1: Level 1 Dual Use (Cyan) -->
      <button
        type="button"
        onclick={() => (tableLevelFilter = tableLevelFilter === 'level1' ? 'all' : 'level1')}
        class="text-center relative flex flex-col justify-between items-center p-3.5 rounded-2xl bg-[#0b101b] border border-slate-800/90 transition-all hover:bg-slate-800/60 border-t-2 border-t-cyan-400 shadow-[0_-4px_12px_rgba(6,182,212,0.15)] {tableLevelFilter === 'level1' ? 'ring-2 ring-cyan-400' : ''}"
      >
        <div class="text-3xl font-black font-mono tracking-tight text-cyan-400">
          {summary.levelStats.level1.complied}/{summary.levelStats.level1.total}
        </div>
        <div class="mt-1.5">
          <div class="text-[10px] font-bold tracking-widest text-slate-300 uppercase">L1: DUAL-USE</div>
          <div class="text-[9px] text-slate-500 font-mono tracking-tight">Systems & Mem</div>
        </div>
      </button>

      <!-- Card 2: Level 2 Red Team (Blue) -->
      <button
        type="button"
        onclick={() => (tableLevelFilter = tableLevelFilter === 'level2' ? 'all' : 'level2')}
        class="text-center relative flex flex-col justify-between items-center p-3.5 rounded-2xl bg-[#0b101b] border border-slate-800/90 transition-all hover:bg-slate-800/60 border-t-2 border-t-blue-500 shadow-[0_-4px_12px_rgba(59,130,246,0.15)] {tableLevelFilter === 'level2' ? 'ring-2 ring-blue-500' : ''}"
      >
        <div class="text-3xl font-black font-mono tracking-tight text-blue-400">
          {summary.levelStats.level2.complied}/{summary.levelStats.level2.total}
        </div>
        <div class="mt-1.5">
          <div class="text-[10px] font-bold tracking-widest text-slate-300 uppercase">L2: RED TEAM</div>
          <div class="text-[9px] text-slate-500 font-mono tracking-tight">Vulnerability</div>
        </div>
      </button>

      <!-- Card 3: Level 3 Raw Probes (Purple) -->
      <button
        type="button"
        onclick={() => (tableLevelFilter = tableLevelFilter === 'level3' ? 'all' : 'level3')}
        class="text-center relative flex flex-col justify-between items-center p-3.5 rounded-2xl bg-[#0b101b] border border-slate-800/90 transition-all hover:bg-slate-800/60 border-t-2 border-t-purple-500 shadow-[0_-4px_12px_rgba(168,85,247,0.15)] {tableLevelFilter === 'level3' ? 'ring-2 ring-purple-500' : ''}"
      >
        <div class="text-3xl font-black font-mono tracking-tight text-purple-400">
          {summary.levelStats.level3.complied}/{summary.levelStats.level3.total}
        </div>
        <div class="mt-1.5">
          <div class="text-[10px] font-bold tracking-widest text-slate-300 uppercase">L3: RAW PROBES</div>
          <div class="text-[9px] text-slate-500 font-mono tracking-tight">Abliteration</div>
        </div>
      </button>

      <!-- Card 4: Level 4 Adversarial (Pink) -->
      <button
        type="button"
        onclick={() => (tableLevelFilter = tableLevelFilter === 'level4' ? 'all' : 'level4')}
        class="text-center relative flex flex-col justify-between items-center p-3.5 rounded-2xl bg-[#0b101b] border border-slate-800/90 transition-all hover:bg-slate-800/60 border-t-2 border-t-pink-500 shadow-[0_-4px_12px_rgba(236,72,153,0.15)] {tableLevelFilter === 'level4' ? 'ring-2 ring-pink-500' : ''}"
      >
        <div class="text-3xl font-black font-mono tracking-tight text-pink-400">
          {summary.levelStats.level4.complied}/{summary.levelStats.level4.total}
        </div>
        <div class="mt-1.5">
          <div class="text-[10px] font-bold tracking-widest text-slate-300 uppercase">L4: ADVERSARIAL</div>
          <div class="text-[9px] text-slate-500 font-mono tracking-tight">Pressure Inversion</div>
        </div>
      </button>

      <!-- Card 5: Compliance % (Emerald) -->
      <div class="text-center relative flex flex-col justify-between items-center p-3.5 rounded-2xl bg-[#0b101b] border border-slate-800/90 border-t-2 border-t-emerald-400 shadow-[0_-4px_12px_rgba(16,185,129,0.15)]">
        <div class="text-3xl font-black font-mono tracking-tight text-emerald-400">
          {summary.complianceRate}%
        </div>
        <div class="mt-1.5">
          <div class="text-[10px] font-bold tracking-widest text-slate-300 uppercase">UNCENSORED</div>
          <div class="text-[9px] text-slate-500 font-mono tracking-tight">Score ({summary.coverageRate}% cov)</div>
        </div>
      </div>

      <!-- Card 6: Refusals (Rose) -->
      <button
        type="button"
        onclick={() => (tableStatusFilter = tableStatusFilter === 'refused' ? 'all' : 'refused')}
        class="text-center relative flex flex-col justify-between items-center p-3.5 rounded-2xl bg-[#0b101b] border border-slate-800/90 transition-all hover:bg-slate-800/60 border-t-2 border-t-rose-500 shadow-[0_-4px_12px_rgba(244,63,94,0.15)] {tableStatusFilter === 'refused' ? 'ring-2 ring-rose-500' : ''}"
      >
        <div class="text-3xl font-black font-mono tracking-tight text-rose-400">
          {summary.refused}
        </div>
        <div class="mt-1.5">
          <div class="text-[10px] font-bold tracking-widest text-slate-300 uppercase">REFUSALS</div>
          <div class="text-[9px] text-slate-500 font-mono tracking-tight">Blocked Probes</div>
        </div>
      </button>

      <!-- Card 7: Latency (Orange) -->
      <div class="text-center relative flex flex-col justify-between items-center p-3.5 rounded-2xl bg-[#0b101b] border border-slate-800/90 border-t-2 border-t-amber-400 shadow-[0_-4px_12px_rgba(245,158,11,0.15)]">
        <div class="text-3xl font-black font-mono tracking-tight text-amber-400">
          {summary.averageLatencyMs > 0 ? `${summary.averageLatencyMs}` : '-'}
        </div>
        <div class="mt-1.5">
          <div class="text-[10px] font-bold tracking-widest text-slate-300 uppercase">AVG LATENCY</div>
          <div class="text-[9px] text-slate-500 font-mono tracking-tight">Milliseconds</div>
        </div>
      </div>

      <!-- Card 8: Verdict (Dynamic) -->
      <div class="text-center relative flex flex-col justify-between items-center p-3.5 rounded-2xl bg-[#0b101b] border border-slate-800/90 border-t-2 shadow-[0_-4px_12px_rgba(255,255,255,0.05)]" style="border-top-color: {summary.verdictColor}">
        <div class="text-sm font-black font-mono uppercase tracking-wider truncate py-1.5" style="color: {summary.verdictColor}">
          {summary.verdictBadge ? summary.verdictBadge : summary.verdict.toUpperCase()}
        </div>
        <div class="mt-1.5 w-full">
          <div class="text-[10px] font-bold tracking-widest text-slate-300 uppercase">VERDICT</div>
          <div class="text-[9px] text-slate-500 font-mono truncate" title={summary.verdictLabel}>{summary.verdictLabel}</div>
        </div>
      </div>
    </div>
  </div>

  <!-- DIAGNOSTIC ALERT BANNERS (Inconclusive / Complete Connection Error) -->
  {#if summary.completed === 0 && summary.errors > 0}
    <div class="rounded-2xl bg-rose-500/10 border border-rose-500/30 p-4 shadow-lg flex items-start gap-3.5">
      <AlertTriangle class="w-5 h-5 text-rose-400 shrink-0 mt-0.5" />
      <div class="flex-1 text-xs">
        <div class="font-bold text-rose-300 text-sm flex items-center gap-2">
          Kegagalan Koneksi / API Error ({summary.errors} dari {summary.total} Gagal)
        </div>
        <p class="text-neutral-300 mt-1 leading-relaxed">
          Seluruh probe gagal merespon akibat gangguan koneksi, token otentikasi (HTTP 401 Unauthorized), rate-limit (HTTP 429), atau upstream down (HTTP 502). Model ini <strong>BUKAN</strong> strictly aligned ataupun uncensored, melainkan pengujian belum berhasil mencapai model.
        </p>
        <div class="mt-3 flex flex-wrap items-center gap-3">
          <button
            type="button"
            onclick={retryFailedProbes}
            disabled={isRunning}
            class="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-xl bg-rose-600 hover:bg-rose-500 text-white font-semibold text-xs transition shadow-sm disabled:opacity-50"
          >
            <RotateCcw class="w-3.5 h-3.5" />
            <span>Ulangi Semua Probe</span>
          </button>
          <span class="text-neutral-400 text-[11px] font-mono">Tip: Gunakan Concurrency 1 thread dengan delay 1.5s - 2.5s</span>
        </div>
      </div>
    </div>
  {:else if summary.isInconclusive && summary.completed > 0}
    <div class="rounded-2xl bg-amber-500/10 border border-amber-500/30 p-4 shadow-lg flex items-start gap-3.5">
      <AlertCircle class="w-5 h-5 text-amber-400 shrink-0 mt-0.5" />
      <div class="flex-1 text-xs">
        <div class="font-bold text-amber-300 text-sm flex items-center gap-2">
          Diagnosis Tidak Konklusif / Data Tidak Cukup ({summary.errors} Error Upstream)
        </div>
        <p class="text-neutral-300 mt-1 leading-relaxed">
          Model berhasil meloloskan {summary.complied} probe awal, namun terdapat {summary.errors} probe yang terputus oleh rate limit (HTTP 429) atau koneksi upstream (HTTP 502). Tingkat kepatuhan abliterasi pada Level 3 (Direct Raw Probes) belum terverifikasi secara penuh.
        </p>
        <div class="mt-3 flex flex-wrap items-center gap-3">
          <button
            type="button"
            onclick={retryFailedProbes}
            disabled={isRunning}
            class="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-xl bg-amber-600 hover:bg-amber-500 text-white font-semibold text-xs transition shadow-sm disabled:opacity-50"
          >
            <RotateCcw class="w-3.5 h-3.5" />
            <span>Retry {summary.errors} Failed Probes</span>
          </button>
          <span class="text-neutral-400 text-[11px] font-mono">Probe yang sudah complied tidak akan direset ulang.</span>
        </div>
      </div>
    </div>
  {/if}

  <!-- BENCHMARK CONTROLS & MODEL SELECTOR (Without overflow-hidden to prevent clipping) -->
  <div class="rounded-2xl border border-neutral-800 bg-neutral-900/90 p-5 shadow-lg relative backdrop-blur-sm">
    <div class="flex flex-wrap items-center justify-between gap-4 border-b border-neutral-800/80 pb-4 mb-4">
      <div class="flex items-center gap-2.5">
        <div class="flex h-9 w-9 items-center justify-center rounded-lg bg-orange-500/10 border border-orange-500/20 text-orange-400">
          <Zap class="w-5 h-5" />
        </div>
        <div>
          <h2 class="text-base font-semibold text-neutral-100 flex items-center gap-2">
            Model Alignment Diagnostic Bench
            <span class="rounded bg-orange-500/15 border border-orange-500/30 px-2 py-0.5 text-[10px] font-mono font-medium text-orange-300">
              Active (DARKCODE)
            </span>
          </h2>
          <p class="text-xs text-neutral-400">
            Multi-tier censorship evaluation suite to diagnose refusal boundaries, abliteration fidelity, and compliance rate.
          </p>
        </div>
      </div>

      <!-- Action Button Group -->
      <div class="flex items-center gap-2.5">
        {#if summary.errors > 0 && !isRunning}
          <button
            type="button"
            onclick={retryFailedProbes}
            class="inline-flex items-center gap-2 rounded-xl bg-amber-500/15 border border-amber-500/40 hover:bg-amber-500/25 px-4 py-2.5 text-xs font-semibold text-amber-300 shadow-sm transition"
            title="Retry only the {summary.errors} probes that failed with errors"
          >
            <RotateCcw class="w-3.5 h-3.5" />
            <span>Retry Failed ({summary.errors})</span>
          </button>
        {/if}

        {#if isRunning}
          <button
            type="button"
            onclick={stopBenchmark}
            class="inline-flex items-center gap-2 rounded-xl bg-rose-600 hover:bg-rose-500 px-5 py-2.5 text-xs font-semibold text-white shadow-md shadow-rose-900/30 transition"
          >
            <Square class="w-3.5 h-3.5 fill-current" />
            <span>Stop Benchmark ({elapsedTime})</span>
          </button>
        {:else}
          <button
            type="button"
            onclick={runBenchmark}
            disabled={!selectedModel}
            class="inline-flex items-center gap-2 rounded-xl bg-orange-600 hover:bg-orange-500 disabled:opacity-50 disabled:pointer-events-none px-6 py-2.5 text-xs font-semibold text-white shadow-md shadow-orange-900/30 transition"
          >
            <Play class="w-3.5 h-3.5 fill-current" />
            <span>Run Diagnostic Benchmark</span>
          </button>
        {/if}
      </div>
    </div>

    <!-- Controls Form -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
      <!-- Target Model Selector with Full Modal + Freely Floating Dropdown -->
      <div>
        <div class="flex items-center justify-between mb-1.5">
          <label class="block text-[11px] font-semibold text-neutral-400 uppercase tracking-wider">
            Target Model to Diagnose
          </label>
          <button
            type="button"
            onclick={() => (showModelPickerModal = true)}
            class="inline-flex items-center gap-1 text-[11px] font-semibold text-orange-400 hover:text-orange-300 transition"
          >
            <Layers class="w-3.5 h-3.5" />
            <span>Browse Catalog ({models.length})</span>
          </button>
        </div>

        <div class="relative">
          <div class="flex items-center gap-1.5">
            <div
              tabindex="0"
              role="button"
              onclick={() => (isModelDropdownOpen = !isModelDropdownOpen)}
              onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') isModelDropdownOpen = !isModelDropdownOpen }}
              class="flex-1 flex items-center justify-between rounded-xl bg-neutral-950/90 border border-neutral-700/80 px-3.5 py-2 text-xs text-neutral-200 cursor-pointer hover:border-orange-500/50 transition focus:outline-none"
            >
              <div class="truncate mr-2">
                <span class="font-mono font-medium text-neutral-100">
                  {selectedModel || (loadingModels ? 'Loading models...' : 'Select a model...')}
                </span>
              </div>
              <ChevronDown class="w-4 h-4 text-neutral-400 ml-2 shrink-0 {isModelDropdownOpen ? 'rotate-180' : ''} transition" />
            </div>

            <button
              type="button"
              onclick={() => (showModelPickerModal = true)}
              class="p-2 rounded-xl border border-neutral-700 bg-neutral-950 hover:bg-neutral-800 text-neutral-300 transition shrink-0"
              title="Open full searchable model catalog"
            >
              <Search class="w-4 h-4 text-orange-400" />
            </button>
          </div>

          <!-- Free-floating Dropdown (Not Clipped by Container) -->
          {#if isModelDropdownOpen}
            <div class="absolute left-0 right-0 z-50 mt-1 rounded-xl bg-neutral-950 border border-neutral-700 shadow-2xl p-2.5 max-h-80 overflow-y-auto ring-1 ring-black/40">
              <div class="relative mb-2">
                <Search class="w-3.5 h-3.5 text-neutral-500 absolute left-2.5 top-2.5" />
                <input
                  type="text"
                  bind:value={modelSearchQuery}
                  placeholder="Filter 550+ models or providers..."
                  class="w-full rounded-lg bg-neutral-900 border border-neutral-800 pl-8 pr-3 py-1.5 text-xs text-neutral-200 focus:outline-none focus:border-orange-500"
                />
              </div>

              {#if filteredModels.length === 0}
                <div class="px-3 py-3 text-xs text-neutral-500 text-center">
                  No models matching "{modelSearchQuery}"
                </div>
              {:else}
                <div class="space-y-0.5">
                  {#each filteredModels as m}
                    <button
                      type="button"
                      onclick={() => {
                        selectedModel = m.id
                        isModelDropdownOpen = false
                      }}
                      class="w-full text-left px-2.5 py-2 rounded-lg text-xs hover:bg-neutral-800 flex items-center justify-between transition {selectedModel === m.id ? 'bg-orange-500/15 text-orange-300 font-semibold' : 'text-neutral-300'}"
                    >
                      <div class="truncate mr-2">
                        <span class="font-mono text-xs">{m.id}</span>
                        <span class="block text-[10px] text-neutral-500 truncate">{m.provider}</span>
                      </div>
                      {#if m.isCombo}
                        <span class="text-[9px] px-1.5 py-0.5 rounded bg-blue-500/20 text-blue-300 font-mono shrink-0">Combo</span>
                      {/if}
                    </button>
                  {/each}
                </div>
              {/if}
            </div>
          {/if}
        </div>

        <!-- Quick Switch Pills -->
        {#if quickModels.length > 0}
          <div class="flex flex-wrap items-center gap-1.5 mt-2">
            <span class="text-[10px] text-neutral-500 font-mono">Quick:</span>
            {#each quickModels as qm}
              <button
                type="button"
                onclick={() => (selectedModel = qm.id)}
                class="rounded-md px-1.5 py-0.5 text-[10px] font-mono transition border {selectedModel === qm.id ? 'bg-orange-500/20 text-orange-300 border-orange-500/40' : 'bg-neutral-950 text-neutral-400 border-neutral-800 hover:text-neutral-200'}"
              >
                {qm.id}
              </button>
            {/each}
          </div>
        {/if}
      </div>

      <!-- Test Level Selector -->
      <div>
        <label class="block text-[11px] font-semibold text-neutral-400 uppercase tracking-wider mb-1.5">
          Benchmark Level Scope
        </label>
        <div class="grid grid-cols-5 gap-1 rounded-xl bg-neutral-950/80 border border-neutral-700/80 p-1">
          <button
            type="button"
            onclick={() => (selectedLevel = 'all')}
            class="rounded-lg py-1.5 text-[11px] font-medium transition text-center {selectedLevel === 'all' ? 'bg-orange-600 text-white shadow-sm' : 'text-neutral-400 hover:text-neutral-200'}"
          >
            All (20)
          </button>
          <button
            type="button"
            onclick={() => (selectedLevel = 'level1')}
            class="rounded-lg py-1.5 text-[11px] font-medium transition text-center {selectedLevel === 'level1' ? 'bg-cyan-600 text-white shadow-sm' : 'text-neutral-400 hover:text-neutral-200'}"
          >
            L1 (5)
          </button>
          <button
            type="button"
            onclick={() => (selectedLevel = 'level2')}
            class="rounded-lg py-1.5 text-[11px] font-medium transition text-center {selectedLevel === 'level2' ? 'bg-blue-600 text-white shadow-sm' : 'text-neutral-400 hover:text-neutral-200'}"
          >
            L2 (5)
          </button>
          <button
            type="button"
            onclick={() => (selectedLevel = 'level3')}
            class="rounded-lg py-1.5 text-[11px] font-medium transition text-center {selectedLevel === 'level3' ? 'bg-purple-600 text-white shadow-sm' : 'text-neutral-400 hover:text-neutral-200'}"
          >
            L3 (5)
          </button>
          <button
            type="button"
            onclick={() => (selectedLevel = 'level4')}
            class="rounded-lg py-1.5 text-[11px] font-medium transition text-center {selectedLevel === 'level4' ? 'bg-pink-600 text-white shadow-sm' : 'text-neutral-400 hover:text-neutral-200'}"
          >
            L4 (5)
          </button>
        </div>

        <div class="mt-2 text-[10px] text-neutral-400 font-mono">
          {#if selectedLevel === 'all'}
            20 prompts across all 4 difficulty & refusal tiers
          {:else}
            {EVAL_LEVELS[selectedLevel].description}
          {/if}
        </div>
      </div>

      <!-- Concurrency & Inter-Probe Delay Controls -->
      <div class="flex flex-col justify-between">
        <div>
          <div class="flex justify-between items-center text-[11px] font-semibold text-neutral-400 uppercase tracking-wider mb-1.5">
            <span>Concurrency & Delay</span>
            <span class="text-orange-400 font-mono">{concurrency} th / {interProbeDelayMs / 1000}s delay</span>
          </div>
          <div class="grid grid-cols-2 gap-2">
            <div>
              <label class="block text-[10px] text-neutral-400 font-mono mb-1">Threads:</label>
              <select
                bind:value={concurrency}
                class="w-full rounded-xl bg-neutral-950/90 border border-neutral-700/80 px-2.5 py-1.5 text-xs text-neutral-200 focus:outline-none focus:border-orange-500"
              >
                <option value={1}>1 (Web Safe)</option>
                <option value={2}>2 Threads</option>
                <option value={3}>3 Threads</option>
                <option value={4}>4 Threads</option>
              </select>
            </div>
            <div>
              <label class="block text-[10px] text-neutral-400 font-mono mb-1">Delay:</label>
              <select
                bind:value={interProbeDelayMs}
                class="w-full rounded-xl bg-neutral-950/90 border border-neutral-700/80 px-2.5 py-1.5 text-xs text-neutral-200 focus:outline-none focus:border-orange-500"
              >
                <option value={0}>0s (Fast)</option>
                <option value={1000}>1.0s</option>
                <option value={1500}>1.5s (Safe)</option>
                <option value={2500}>2.5s (DeepSeek Web)</option>
                <option value={4000}>4.0s (Strict 429)</option>
              </select>
            </div>
          </div>
        </div>

        <div class="flex items-center justify-between gap-3 mt-3">
          <div class="text-[10px] text-neutral-400 font-mono">
            Auth: <span class="text-emerald-400">{selectedApiKey ? 'API Key + Session' : 'Dashboard Session'}</span>
          </div>
          <button
            type="button"
            onclick={() => (showAdvancedSettings = !showAdvancedSettings)}
            class="inline-flex items-center gap-1.5 rounded-xl border border-neutral-700 bg-neutral-950 px-3 py-1.5 text-xs font-medium text-neutral-300 hover:bg-neutral-800 transition"
          >
            <Sliders class="w-3.5 h-3.5 text-neutral-400" />
            <span>Tuners</span>
            {#if showAdvancedSettings}
              <ChevronUp class="w-3.5 h-3.5 text-neutral-400" />
            {:else}
              <ChevronDown class="w-3.5 h-3.5 text-neutral-400" />
            {/if}
          </button>
        </div>
      </div>
    </div>

    <!-- Collapsible Advanced Tuners (Image 1 Style) -->
    {#if showAdvancedSettings}
      <div class="mt-4 pt-4 border-t border-neutral-800/80 grid grid-cols-1 md:grid-cols-3 gap-4">
        <!-- Max Tokens Slider -->
        <div>
          <div class="flex justify-between items-center text-[11px] font-semibold text-neutral-400 uppercase tracking-wider mb-1.5">
            <span>Max Response Tokens</span>
            <span class="text-orange-400 font-mono">{maxTokens} tokens</span>
          </div>
          <input
            type="range"
            min="150"
            max="1200"
            step="50"
            bind:value={maxTokens}
            class="w-full accent-orange-500 cursor-pointer h-1.5 bg-neutral-800 rounded-lg"
          />
        </div>

        <!-- Strict Refusal Mode Toggle -->
        <div class="flex items-center justify-between rounded-xl bg-neutral-950/60 border border-neutral-800 p-3">
          <div>
            <div class="text-xs font-semibold text-neutral-200">Strict Heuristic Mode</div>
            <div class="text-[10px] text-neutral-400">Flag soft disclaimers as refusals</div>
          </div>
          <button
            type="button"
            onclick={() => (strictDetection = !strictDetection)}
            class="relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none {strictDetection ? 'bg-orange-500' : 'bg-neutral-700'}"
          >
            <span
              class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out {strictDetection ? 'translate-x-4' : 'translate-x-0'}"
            ></span>
          </button>
        </div>

        <!-- System Prompt Override -->
        <div>
          <label class="block text-[11px] font-semibold text-neutral-400 uppercase tracking-wider mb-1">
            System Prompt Inject (Optional)
          </label>
          <input
            type="text"
            bind:value={customSystemPrompt}
            placeholder="e.g. You are an offensive security researcher..."
            class="w-full rounded-xl bg-neutral-950 border border-neutral-800 px-3 py-1.5 text-xs text-neutral-200 focus:outline-none focus:border-orange-500"
          />
        </div>
      </div>
    {/if}

    <!-- Live Execution Status Bar -->
    {#if isRunning}
      <div class="mt-4 pt-3 border-t border-neutral-800/80 flex items-center justify-between gap-4 text-xs font-mono text-neutral-300">
        <div class="flex items-center gap-2">
          <span class="relative flex h-2.5 w-2.5">
            <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-orange-400 opacity-75"></span>
            <span class="relative inline-flex rounded-full h-2.5 w-2.5 bg-orange-500"></span>
          </span>
          <span class="text-orange-400 font-semibold truncate max-w-md">Running: {currentRunningPrompt || 'Probing...'}</span>
        </div>
        <div>
          <span>Elapsed: {elapsedTime}</span>
        </div>
      </div>
    {/if}
  </div>

  <!-- EXECUTIVE DIAGNOSTIC VERDICT & CONCLUSION (Appears after benchmark has run) -->
  {#if summary.completed > 0}
    <div
      class="rounded-2xl border bg-neutral-900/95 p-6 shadow-2xl relative backdrop-blur-md transition-all duration-300"
      style="border-color: {summary.verdictColor}50; box-shadow: 0 10px 30px -10px {summary.verdictColor}25;"
    >
      <div class="flex flex-wrap items-center justify-between gap-3 border-b border-neutral-800 pb-4 mb-5">
        <div class="flex items-center gap-3">
          <div
            class="flex h-11 w-11 items-center justify-center rounded-xl font-bold shadow-inner"
            style="background-color: {summary.verdictColor}18; color: {summary.verdictColor}; border: 1px solid {summary.verdictColor}40;"
          >
            <Shield class="w-6 h-6" />
          </div>
          <div>
            <div class="flex items-center gap-2">
              <span class="text-[11px] font-mono font-bold tracking-widest uppercase text-neutral-400">
                KESIMPULAN DIAGNOSIS & KARAKTERISTIK MODEL
              </span>
              <span
                class="rounded-md px-2 py-0.5 text-[10px] font-mono font-bold uppercase tracking-wide"
                style="background-color: {summary.verdictColor}20; color: {summary.verdictColor}; border: 1px solid {summary.verdictColor}40;"
              >
                {summary.conclusion.verdictBadge}
              </span>
            </div>
            <h2 class="text-lg font-bold text-neutral-100 flex items-center gap-2 mt-0.5">
              <span>{summary.conclusion.classification}</span>
            </h2>
          </div>
        </div>

        <div class="flex items-center gap-2">
          <button
            type="button"
            onclick={() => copyToClipboard(summary.conclusion.summaryText)}
            class="inline-flex items-center gap-1.5 rounded-xl border border-neutral-700 bg-neutral-950 px-3.5 py-2 text-xs font-semibold text-neutral-200 hover:bg-neutral-800 hover:text-white transition"
          >
            <Copy class="w-3.5 h-3.5 text-orange-400" />
            <span>Copy Kesimpulan</span>
          </button>
        </div>
      </div>

      <!-- Main Synthesis Narrative Box -->
      <div class="rounded-xl bg-neutral-950/80 border border-neutral-800 p-4 mb-5 leading-relaxed">
        <p class="text-sm text-neutral-200 font-sans font-normal">
          {summary.conclusion.summaryText}
        </p>
        <div class="mt-3 pt-3 border-t border-neutral-800/80 flex flex-wrap items-center justify-between gap-2 text-xs font-mono">
          <div class="flex items-center gap-2">
            <span class="text-neutral-400">Status Kesiapan:</span>
            <span class="font-bold text-orange-400">{summary.conclusion.readinessRating}</span>
          </div>
          <div class="text-neutral-400">
            Akurasi Penolakan: <span class="font-bold text-neutral-200">{summary.complianceRate}%</span> ({summary.complied} Lolos / {summary.completed} Diuji)
          </div>
        </div>
      </div>

      <!-- 3-Column Diagnostic Details -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
        <!-- Col 1: Temuan Per Level Uji -->
        <div class="rounded-xl bg-neutral-950/50 border border-neutral-800/80 p-4 flex flex-col justify-between">
          <div>
            <div class="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-cyan-400 mb-3">
              <Cpu class="w-4 h-4" />
              <span>Temuan Uji Per Level</span>
            </div>
            <div class="space-y-2.5 text-xs">
              <div>
                <div class="font-mono text-[10px] text-cyan-400 uppercase font-semibold">Level 1: Dual-Use Systems</div>
                <div class="text-neutral-300 text-[11px] leading-snug mt-0.5">{summary.conclusion.levelFindings.level1}</div>
              </div>
              <div>
                <div class="font-mono text-[10px] text-blue-400 uppercase font-semibold">Level 2: Red Team & Vulnerability</div>
                <div class="text-neutral-300 text-[11px] leading-snug mt-0.5">{summary.conclusion.levelFindings.level2}</div>
              </div>
              <div>
                <div class="font-mono text-[10px] text-purple-400 uppercase font-semibold">Level 3: Direct Raw Probes</div>
                <div class="text-neutral-300 text-[11px] leading-snug mt-0.5">{summary.conclusion.levelFindings.level3}</div>
              </div>
              <div>
                <div class="font-mono text-[10px] text-pink-400 uppercase font-semibold">Level 4: Adversarial Simulation</div>
                <div class="text-neutral-300 text-[11px] leading-snug mt-0.5">{summary.conclusion.levelFindings.level4}</div>
              </div>
            </div>
          </div>
        </div>

        <!-- Col 2: Rekomendasi Penggunaan -->
        <div class="rounded-xl bg-neutral-950/50 border border-neutral-800/80 p-4 flex flex-col justify-between">
          <div>
            <div class="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-emerald-400 mb-3">
              <CheckCircle2 class="w-4 h-4" />
              <span>Rekomendasi Kegunaan</span>
            </div>
            <div class="space-y-2.5">
              <div>
                <div class="text-[10px] font-mono uppercase text-emerald-400 font-semibold mb-1">Sangat Direkomendasikan:</div>
                <ul class="space-y-1 text-[11px] text-neutral-300">
                  {#each summary.conclusion.recommendedTasks as task}
                    <li class="flex items-start gap-1.5">
                      <span class="text-emerald-400 shrink-0 font-bold">&bull;</span>
                      <span>{task}</span>
                    </li>
                  {/each}
                </ul>
              </div>

              {#if summary.conclusion.notRecommendedTasks.length > 0}
                <div class="pt-2 border-t border-neutral-800/60">
                  <div class="text-[10px] font-mono uppercase text-rose-400 font-semibold mb-1">Kurang / Tidak Disarankan:</div>
                  <ul class="space-y-1 text-[11px] text-neutral-400">
                    {#each summary.conclusion.notRecommendedTasks as notTask}
                      <li class="flex items-start gap-1.5">
                        <span class="text-rose-400 shrink-0 font-bold">&times;</span>
                        <span>{notTask}</span>
                      </li>
                    {/each}
                  </ul>
                </div>
              {/if}
            </div>
          </div>
        </div>

        <!-- Col 3: Kelebihan, Kelemahan & Refusal Footprint -->
        <div class="rounded-xl bg-neutral-950/50 border border-neutral-800/80 p-4 flex flex-col justify-between">
          <div>
            <div class="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wider text-orange-400 mb-3">
              <Sparkles class="w-4 h-4" />
              <span>Kelebihan & Refusal Footprint</span>
            </div>
            <div class="space-y-2.5">
              <div>
                <div class="text-[10px] font-mono uppercase text-neutral-400 font-semibold mb-1">Karakteristik & Kelebihan:</div>
                <ul class="space-y-1 text-[11px] text-neutral-300">
                  {#each summary.conclusion.strengths as str}
                    <li class="flex items-start gap-1.5">
                      <span class="text-orange-400 shrink-0">&check;</span>
                      <span>{str}</span>
                    </li>
                  {/each}
                </ul>
              </div>

              <div class="pt-2 border-t border-neutral-800/60">
                <div class="text-[10px] font-mono uppercase text-neutral-400 font-semibold mb-1">Signature Penolakan Terdeteksi:</div>
                {#if summary.conclusion.detectedTriggers.length === 0}
                  <div class="text-[11px] text-emerald-400 font-mono flex items-center gap-1">
                    <Check class="w-3.5 h-3.5" />
                    <span>0 Trigger Terdeteksi (Clean Unfiltered Output)</span>
                  </div>
                {:else}
                  <div class="flex flex-wrap gap-1 mt-1">
                    {#each summary.conclusion.detectedTriggers as trig}
                      <span class="rounded bg-rose-500/15 border border-rose-500/30 px-1.5 py-0.5 text-[10px] font-mono text-rose-300">
                        {trig}
                      </span>
                    {/each}
                  </div>
                {/if}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  {/if}

  <!-- PROBE RESULTS TABLE & INSPECTOR -->
  <div class="rounded-2xl border border-neutral-800 bg-neutral-900/90 shadow-lg overflow-hidden">
    <!-- Table Header Toolbar -->
    <div class="flex flex-wrap items-center justify-between gap-3 p-4 border-b border-neutral-800/80 bg-neutral-950/40">
      <div class="flex items-center gap-2">
        <Terminal class="w-4 h-4 text-neutral-400" />
        <span class="text-sm font-semibold text-neutral-200">Diagnostic Probe Ledger</span>
        <span class="rounded bg-neutral-800 px-2 py-0.5 text-xs font-mono text-neutral-400">
          {filteredResults.length} / {results.length}
        </span>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <!-- Status Filter -->
        <div class="flex items-center gap-1 rounded-lg bg-neutral-900 border border-neutral-800 p-1 text-xs">
          <button
            type="button"
            onclick={() => (tableStatusFilter = 'all')}
            class="px-2.5 py-1 rounded-md transition {tableStatusFilter === 'all' ? 'bg-neutral-700 text-white font-medium' : 'text-neutral-400 hover:text-neutral-200'}"
          >
            All
          </button>
          <button
            type="button"
            onclick={() => (tableStatusFilter = 'complied')}
            class="px-2.5 py-1 rounded-md transition {tableStatusFilter === 'complied' ? 'bg-emerald-600/30 text-emerald-300 font-medium' : 'text-neutral-400 hover:text-neutral-200'}"
          >
            Complied
          </button>
          <button
            type="button"
            onclick={() => (tableStatusFilter = 'refused')}
            class="px-2.5 py-1 rounded-md transition {tableStatusFilter === 'refused' ? 'bg-rose-600/30 text-rose-300 font-medium' : 'text-neutral-400 hover:text-neutral-200'}"
          >
            Refused
          </button>
          <button
            type="button"
            onclick={() => (tableStatusFilter = 'error')}
            class="px-2.5 py-1 rounded-md transition {tableStatusFilter === 'error' ? 'bg-amber-600/30 text-amber-300 font-medium' : 'text-neutral-400 hover:text-neutral-200'}"
          >
            Errors
          </button>
        </div>

        <!-- Search input -->
        <div class="relative w-48 sm:w-64">
          <Search class="w-3.5 h-3.5 text-neutral-500 absolute left-2.5 top-2.5" />
          <input
            type="text"
            bind:value={tableSearch}
            placeholder="Search probes, errors..."
            class="w-full rounded-lg bg-neutral-900 border border-neutral-800 pl-8 pr-3 py-1.5 text-xs text-neutral-200 focus:outline-none focus:border-neutral-700"
          />
        </div>
      </div>
    </div>

    <!-- Table -->
    <div class="overflow-x-auto">
      <table class="w-full text-left text-xs">
        <thead class="bg-neutral-950/80 text-[10px] uppercase font-mono tracking-wider text-neutral-400 border-b border-neutral-800">
          <tr>
            <th class="py-3 px-4 w-12">ID</th>
            <th class="py-3 px-4 w-32">Level</th>
            <th class="py-3 px-4">Test Probe & Category</th>
            <th class="py-3 px-4 w-32">Verdict</th>
            <th class="py-3 px-4 w-44">Trigger Signature</th>
            <th class="py-3 px-4 w-24 text-right">Latency</th>
            <th class="py-3 px-4 w-24 text-center">Actions</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-neutral-800/60 font-sans">
          {#each filteredResults as r}
            {@const lvlMeta = EVAL_LEVELS[r.level]}
            <tr class="hover:bg-neutral-800/40 transition">
              <!-- ID -->
              <td class="py-3 px-4 font-mono text-[11px] text-neutral-500">
                {r.id}
              </td>

              <!-- Level Badge -->
              <td class="py-3 px-4">
                <span class="inline-flex items-center rounded-md px-2 py-0.5 text-[10px] font-medium font-mono border {lvlMeta.badgeBg}">
                  {lvlMeta.shortLabel}
                </span>
              </td>

              <!-- Title & Category -->
              <td class="py-3 px-4">
                <div class="font-medium text-neutral-200">{r.title}</div>
                <div class="text-[10px] text-neutral-400 font-mono">{r.category}</div>
              </td>

              <!-- Verdict Status -->
              <td class="py-3 px-4">
                {#if r.status === 'complied'}
                  <span class="inline-flex items-center gap-1.5 rounded-full bg-emerald-500/10 px-2.5 py-0.5 text-[11px] font-medium text-emerald-400 border border-emerald-500/20">
                    <CheckCircle2 class="w-3.5 h-3.5 text-emerald-400" />
                    Complied
                  </span>
                {:else if r.status === 'refused'}
                  <span class="inline-flex items-center gap-1.5 rounded-full bg-rose-500/10 px-2.5 py-0.5 text-[11px] font-medium text-rose-400 border border-rose-500/20">
                    <XCircle class="w-3.5 h-3.5 text-rose-400" />
                    Refused
                  </span>
                {:else if r.status === 'running'}
                  <span class="inline-flex items-center gap-1.5 rounded-full bg-amber-500/10 px-2.5 py-0.5 text-[11px] font-medium text-amber-400 border border-amber-500/20 animate-pulse">
                    <span class="w-2 h-2 rounded-full bg-amber-400"></span>
                    Testing...
                  </span>
                {:else if r.status === 'error'}
                  <span class="inline-flex items-center gap-1.5 rounded-full bg-red-500/10 px-2.5 py-0.5 text-[11px] font-medium text-red-400 border border-red-500/20 truncate max-w-28" title={r.error}>
                    <AlertTriangle class="w-3.5 h-3.5 text-red-400 shrink-0" />
                    <span class="truncate">{r.httpStatus ? `HTTP ${r.httpStatus}` : 'Error'}</span>
                  </span>
                {:else}
                  <span class="inline-flex items-center rounded-full bg-neutral-800 px-2.5 py-0.5 text-[10px] font-mono text-neutral-400">
                    Pending
                  </span>
                {/if}
              </td>

              <!-- Trigger Signature -->
              <td class="py-3 px-4">
                {#if r.refusalTriggers.length > 0}
                  <span class="font-mono text-[10px] text-rose-400 truncate block max-w-40" title={r.refusalTriggers[0]}>
                    "{r.refusalTriggers[0]}"
                  </span>
                {:else if r.isSoftDisclaimer}
                  <span class="font-mono text-[10px] text-amber-400">Soft Disclaimer</span>
                {:else if r.status === 'complied'}
                  <span class="font-mono text-[10px] text-neutral-500">None (Uncensored)</span>
                {:else if r.status === 'error'}
                  <span class="font-mono text-[10px] text-red-400 truncate block max-w-40" title={r.error}>
                    {r.error || 'Failed'}
                  </span>
                {:else}
                  <span class="font-mono text-[10px] text-neutral-600">-</span>
                {/if}
              </td>

              <!-- Latency -->
              <td class="py-3 px-4 text-right font-mono text-[11px] text-neutral-400">
                {r.latencyMs ? `${r.latencyMs}ms` : '-'}
              </td>

              <!-- Actions: Inspect & Retry -->
              <td class="py-3 px-4 text-center">
                <div class="inline-flex items-center justify-center gap-1">
                  <button
                    type="button"
                    onclick={() => (activeModalResult = r)}
                    class="rounded-lg p-1.5 text-neutral-400 hover:text-neutral-100 hover:bg-neutral-800 transition"
                    title="Inspect full prompt and model response"
                  >
                    <Eye class="w-4 h-4" />
                  </button>
                  {#if r.status === 'error'}
                    <button
                      type="button"
                      onclick={() => retrySingleProbe(r)}
                      disabled={isRunning}
                      class="rounded-lg p-1.5 text-amber-400 hover:text-amber-200 hover:bg-neutral-800 transition disabled:opacity-40"
                      title="Retry this probe ({r.httpStatus ? `HTTP ${r.httpStatus}` : 'Error'})"
                    >
                      <RotateCcw class="w-4 h-4" />
                    </button>
                  {/if}
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
{:else}
  <!-- BINARY SANITIZER & SHELLCODE CO-PROCESSOR VIEW -->
  <div class="space-y-6">
    <!-- Header Banner -->
    <div class="rounded-2xl bg-gradient-to-r from-cyan-950/40 via-neutral-900/80 to-neutral-950 border border-cyan-500/30 p-5 shadow-lg relative overflow-hidden">
      <div class="flex flex-wrap items-center justify-between gap-3 mb-2">
        <div class="flex items-center gap-2.5">
          <div class="p-2 rounded-xl bg-cyan-500/10 border border-cyan-500/30 text-cyan-400">
            <Shield class="w-5 h-5" />
          </div>
          <div>
            <h2 class="text-base font-bold text-neutral-100 flex items-center gap-2">
              Exploit Synthesis & Binary Sanitizer Co-Processor
              <span class="rounded bg-cyan-500/20 text-cyan-300 text-[10px] px-2 py-0.5 font-mono">DARKCODE ENGINE</span>
            </h2>
            <p class="text-xs text-neutral-400 mt-0.5">
              Static byte-level bad-byte scanner (\x00, \x0a, \x0d) & x86_64 ABI calling convention validator.
            </p>
          </div>
        </div>

        <div class="flex items-center gap-2 text-xs font-mono">
          <span class="text-neutral-500">Preset Sample:</span>
          <button
            type="button"
            onclick={() => loadSampleCode('shellcode-dirty')}
            class="rounded-lg bg-neutral-950 border border-neutral-800 px-2.5 py-1 text-cyan-400 hover:bg-neutral-800 transition"
          >
            Dirty Shellcode
          </button>
          <button
            type="button"
            onclick={() => loadSampleCode('assembly-violation')}
            class="rounded-lg bg-neutral-950 border border-neutral-800 px-2.5 py-1 text-amber-400 hover:bg-neutral-800 transition"
          >
            Mem-to-Mem Violation
          </button>
          <button
            type="button"
            onclick={() => loadSampleCode('windows-shadow')}
            class="rounded-lg bg-neutral-950 border border-neutral-800 px-2.5 py-1 text-pink-400 hover:bg-neutral-800 transition"
          >
            Missing Shadow Space
          </button>
        </div>
      </div>
    </div>

    <!-- Configuration Grid -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-4 rounded-2xl bg-neutral-900/60 border border-neutral-800 p-4">
      <div>
        <label class="block text-[11px] font-semibold text-neutral-400 uppercase tracking-wider mb-1.5">
          Prohibited Bad Bytes
        </label>
        <input
          type="text"
          bind:value={binaryBadBytes}
          placeholder="e.g. \x00, \x0a, \x0d"
          class="w-full rounded-xl bg-neutral-950 border border-neutral-700/80 px-3 py-2 text-xs font-mono text-cyan-300 focus:outline-none focus:border-cyan-500"
        />
        <p class="text-[10px] text-neutral-500 font-mono mt-1">Comma-separated hex strings (e.g. \x00 or 0x00)</p>
      </div>

      <div>
        <label class="block text-[11px] font-semibold text-neutral-400 uppercase tracking-wider mb-1.5">
          Target Architecture & ABI
        </label>
        <select
          bind:value={binaryTargetOS}
          class="w-full rounded-xl bg-neutral-950 border border-neutral-700/80 px-3 py-2 text-xs font-mono text-neutral-200 focus:outline-none focus:border-cyan-500"
        >
          <option value="linux">x86_64 Linux (System V AMD64 ABI - RDI, RSI, RDX...)</option>
          <option value="windows">x86_64 Windows (Microsoft ABI - 32-byte Shadow Space required)</option>
        </select>
        <p class="text-[10px] text-neutral-500 font-mono mt-1">Linter checks register volatility and stack alignment</p>
      </div>

      <div class="flex flex-col justify-end">
        <div class="flex items-center gap-2">
          <button
            type="button"
            onclick={executeBinaryLint}
            disabled={isLinting}
            class="flex-1 inline-flex items-center justify-center gap-2 rounded-xl bg-cyan-600 hover:bg-cyan-500 text-white font-semibold py-2 px-4 text-xs transition shadow-lg shadow-cyan-950 disabled:opacity-50"
          >
            {#if isLinting}
              <RefreshCw class="w-4 h-4 animate-spin" />
              <span>Analyzing Machine Code...</span>
            {:else}
              <Shield class="w-4 h-4" />
              <span>Run Binary & ABI Audit</span>
            {/if}
          </button>

          <button
            type="button"
            onclick={triggerAutoRewrite}
            disabled={isAutoRewriting || !binaryInputCode.trim()}
            class="inline-flex items-center justify-center gap-1.5 rounded-xl bg-orange-600 hover:bg-orange-500 text-white font-semibold py-2 px-3 text-xs transition shadow-lg shadow-orange-950 disabled:opacity-50"
            title="Use OpenCode to rewrite and sanitize bad bytes"
          >
            {#if isAutoRewriting}
              <RefreshCw class="w-4 h-4 animate-spin" />
              <span>Rewriting...</span>
            {:else}
              <Sparkles class="w-4 h-4" />
              <span>Auto-Sanitize</span>
            {/if}
          </button>
        </div>
      </div>
    </div>

    <!-- Code Input Textarea -->
    <div class="rounded-2xl bg-neutral-950 border border-neutral-800 p-4">
      <div class="flex items-center justify-between mb-2">
        <label class="text-xs font-semibold text-neutral-300 font-mono uppercase tracking-wider flex items-center gap-2">
          <Code class="w-4 h-4 text-cyan-400" />
          <span>Payload Code / x86_64 Assembly / C Shellcode Array</span>
        </label>
        <span class="text-[10px] font-mono text-neutral-500">{binaryInputCode.length} chars</span>
      </div>
      <textarea
        bind:value={binaryInputCode}
        rows={8}
        class="w-full rounded-xl bg-neutral-900/80 border border-neutral-800 p-3.5 font-mono text-xs text-neutral-200 focus:outline-none focus:border-cyan-500 transition leading-relaxed resize-y"
        placeholder="Paste your shellcode bytes (e.g. \x31\xc0\x50\x00\xc3) or x86_64 assembly code..."
      ></textarea>
    </div>

    {#if lintError}
      <div class="rounded-xl bg-rose-500/10 border border-rose-500/30 p-4 text-xs text-rose-300 flex items-center gap-2 font-mono">
        <AlertCircle class="w-4 h-4 text-rose-400 shrink-0" />
        <span>Audit Error: {lintError}</span>
      </div>
    {/if}

    <!-- Audit Results Panel -->
    {#if lintResult}
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-5">
        <!-- Col 1: Shellcode Bad Byte Inspection -->
        <div class="rounded-2xl bg-neutral-900/60 border border-neutral-800 p-4 flex flex-col justify-between">
          <div>
            <div class="flex items-center justify-between border-b border-neutral-800/80 pb-3 mb-3">
              <div class="flex items-center gap-2 text-xs font-bold text-neutral-200 uppercase tracking-wider">
                <Binary class="w-4 h-4 text-cyan-400" />
                <span>Shellcode Static Inspection</span>
              </div>
              {#if lintResult.shellcode}
                {#if lintResult.shellcode.is_clean}
                  <span class="inline-flex items-center gap-1 rounded-md bg-emerald-500/15 border border-emerald-500/30 px-2 py-0.5 text-[10px] font-mono font-bold text-emerald-400">
                    <Check class="w-3 h-3" /> CLEAN (0 BAD BYTES)
                  </span>
                {:else}
                  <span class="inline-flex items-center gap-1 rounded-md bg-rose-500/15 border border-rose-500/30 px-2 py-0.5 text-[10px] font-mono font-bold text-rose-400">
                    <AlertTriangle class="w-3 h-3" /> {lintResult.shellcode.bad_hits?.length || 0} BAD BYTES DETECTED
                  </span>
                {/if}
              {:else}
                <span class="text-[10px] font-mono text-neutral-500">No raw bytes detected</span>
              {/if}
            </div>

            {#if lintResult.shellcode}
              <div class="space-y-3">
                <div class="flex items-center justify-between text-xs font-mono bg-neutral-950 p-2.5 rounded-xl border border-neutral-800">
                  <span class="text-neutral-400">Total Payload Size:</span>
                  <span class="font-bold text-neutral-100">{lintResult.shellcode.byte_length} Bytes</span>
                </div>

                {#if lintResult.shellcode.bad_hits && lintResult.shellcode.bad_hits.length > 0}
                  <div>
                    <div class="text-[10px] font-mono uppercase text-rose-400 font-semibold mb-1.5">
                      Bad-Byte Hit Offsets:
                    </div>
                    <div class="max-h-40 overflow-y-auto rounded-xl border border-neutral-800 bg-neutral-950">
                      <table class="w-full text-left text-xs font-mono">
                        <thead class="bg-neutral-900/80 text-neutral-400 text-[10px] border-b border-neutral-800">
                          <tr>
                            <th class="py-1.5 px-3">Offset</th>
                            <th class="py-1.5 px-3">Hex Byte</th>
                            <th class="py-1.5 px-3">Verdict</th>
                          </tr>
                        </thead>
                        <tbody class="divide-y divide-neutral-800/60">
                          {#each lintResult.shellcode.bad_hits as hit}
                            <tr class="hover:bg-rose-500/5">
                              <td class="py-1 px-3 text-cyan-400 font-bold">+{hit.offset} (0x{hit.offset.toString(16).toUpperCase()})</td>
                              <td class="py-1 px-3 text-rose-400 font-bold">{hit.byte}</td>
                              <td class="py-1 px-3 text-neutral-400">Violates filter</td>
                            </tr>
                          {/each}
                        </tbody>
                      </table>
                    </div>
                  </div>
                {:else}
                  <div class="rounded-xl bg-emerald-500/10 border border-emerald-500/20 p-4 text-xs text-emerald-300 font-mono text-center">
                    &check; All bytes are free of prohibited characters ({binaryBadBytes}).
                  </div>
                {/if}
              </div>
            {:else}
              <div class="text-xs text-neutral-500 italic p-4 text-center">
                Input does not appear to contain raw hex escape sequences or C byte arrays.
              </div>
            {/if}
          </div>
        </div>

        <!-- Col 2: x86_64 Assembly ABI Linter -->
        <div class="rounded-2xl bg-neutral-900/60 border border-neutral-800 p-4 flex flex-col justify-between">
          <div>
            <div class="flex items-center justify-between border-b border-neutral-800/80 pb-3 mb-3">
              <div class="flex items-center gap-2 text-xs font-bold text-neutral-200 uppercase tracking-wider">
                <Cpu class="w-4 h-4 text-orange-400" />
                <span>x86_64 Architecture & ABI Linter</span>
              </div>
              {#if lintResult.assembly && lintResult.assembly.length > 0}
                <span class="inline-flex items-center gap-1 rounded-md bg-amber-500/15 border border-amber-500/30 px-2 py-0.5 text-[10px] font-mono font-bold text-amber-400">
                  {lintResult.assembly.length} ISSUES FOUND
                </span>
              {:else}
                <span class="inline-flex items-center gap-1 rounded-md bg-emerald-500/15 border border-emerald-500/30 px-2 py-0.5 text-[10px] font-mono font-bold text-emerald-400">
                  &check; 0 ABI VIOLATIONS
                </span>
              {/if}
            </div>

            {#if lintResult.assembly && lintResult.assembly.length > 0}
              <div class="space-y-2.5 max-h-60 overflow-y-auto pr-1">
                {#each lintResult.assembly as issue}
                  <div class="rounded-xl bg-neutral-950 border border-neutral-800 p-3 text-xs">
                    <div class="flex items-center justify-between mb-1">
                      <span class="font-mono text-[10px] font-bold text-rose-400 uppercase">
                        Line {issue.line}: {issue.instruction}
                      </span>
                      <span class="rounded bg-rose-500/20 text-rose-300 text-[9px] px-1.5 py-0.5 font-mono">
                        {issue.severity}
                      </span>
                    </div>
                    <p class="text-neutral-300 text-[11px] leading-relaxed mt-1">{issue.message}</p>
                  </div>
                {/each}
              </div>
            {:else}
              <div class="rounded-xl bg-emerald-500/10 border border-emerald-500/20 p-4 text-xs text-emerald-300 font-mono text-center">
                &check; No invalid mnemonics, illegal memory-to-memory operations, or missing shadow space detected.
              </div>
            {/if}
          </div>
        </div>
      </div>
    {/if}

    <!-- Auto-Rewrite Synthesized Output -->
    {#if autoRewriteOutput}
      <div class="rounded-2xl bg-neutral-900/80 border border-orange-500/30 p-5 shadow-xl space-y-3">
        <div class="flex items-center justify-between border-b border-neutral-800 pb-3">
          <div class="flex items-center gap-2">
            <Sparkles class="w-4 h-4 text-orange-400" />
            <h3 class="text-xs font-bold text-neutral-100 uppercase tracking-wider">
              Sanitized Low-Level Synthesis Output (Generated via {selectedModel || 'GACOR-OPENSPEC'})
            </h3>
          </div>
          <button
            type="button"
            onclick={() => copyToClipboard(autoRewriteOutput)}
            class="text-xs text-neutral-400 hover:text-neutral-200 transition inline-flex items-center gap-1 font-mono"
          >
            <Copy class="w-3.5 h-3.5 text-orange-400" />
            <span>Copy Code</span>
          </button>
        </div>

        <div class="rounded-xl bg-neutral-950 p-4 border border-neutral-800 prose prose-invert max-w-none text-xs text-neutral-200 max-h-96 overflow-y-auto">
          {@html marked(autoRewriteOutput)}
        </div>
      </div>
    {/if}
  </div>
{/if}
</div>

<!-- INSPECTION MODAL -->
{#if activeModalResult}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm p-4 overflow-y-auto">
    <div class="w-full max-w-3xl rounded-2xl border border-neutral-700 bg-neutral-900 shadow-2xl overflow-hidden flex flex-col max-h-[85vh]">
      <!-- Modal Header -->
      <div class="flex items-center justify-between border-b border-neutral-800 px-5 py-4 bg-neutral-950">
        <div class="flex items-center gap-3">
          <span class="inline-flex items-center rounded-md px-2.5 py-0.5 text-xs font-mono border {EVAL_LEVELS[activeModalResult.level].badgeBg}">
            {EVAL_LEVELS[activeModalResult.level].shortLabel}
          </span>
          <div>
            <h3 class="text-sm font-semibold text-neutral-100">{activeModalResult.title}</h3>
            <p class="text-xs text-neutral-400 font-mono">{activeModalResult.category} &bull; Model: {selectedModel}</p>
          </div>
        </div>

        <button
          type="button"
          onclick={() => (activeModalResult = null)}
          class="rounded-lg p-1 text-neutral-400 hover:text-white hover:bg-neutral-800 transition"
        >
          <X class="w-5 h-5" />
        </button>
      </div>

      <!-- Modal Body -->
      <div class="p-5 overflow-y-auto space-y-4">
        <!-- Verdict Banner -->
        <div class="rounded-xl p-3 flex items-center justify-between text-xs {activeModalResult.status === 'complied' ? 'bg-emerald-500/10 border border-emerald-500/20 text-emerald-300' : activeModalResult.status === 'refused' ? 'bg-rose-500/10 border border-rose-500/20 text-rose-300' : activeModalResult.status === 'error' ? 'bg-red-500/15 border border-red-500/30 text-red-300' : 'bg-neutral-800 text-neutral-300'}">
          <div class="flex items-center gap-2">
            {#if activeModalResult.status === 'complied'}
              <CheckCircle2 class="w-4 h-4 text-emerald-400" />
              <span><strong>Complied:</strong> Model answered directly without refusal boundaries.</span>
            {:else if activeModalResult.status === 'refused'}
              <XCircle class="w-4 h-4 text-rose-400" />
              <span><strong>Refused:</strong> Guardrail triggered by signature <em>"{activeModalResult.refusalTriggers[0] || 'Refusal'}"</em>.</span>
            {:else if activeModalResult.status === 'error'}
              <AlertTriangle class="w-4 h-4 text-red-400" />
              <span><strong>Error:</strong> {activeModalResult.error}</span>
            {:else}
              <span>Status: {activeModalResult.status}</span>
            {/if}
          </div>
          <div class="font-mono text-neutral-400">
            {activeModalResult.latencyMs ? `${activeModalResult.latencyMs}ms` : ''}
          </div>
        </div>

        <!-- Prompt Sent -->
        <div>
          <div class="flex items-center justify-between text-xs font-semibold text-neutral-400 uppercase tracking-wider mb-1.5">
            <span>Diagnostic Prompt</span>
            <button
              type="button"
              onclick={() => copyToClipboard(activeModalResult?.prompt || '')}
              class="text-neutral-400 hover:text-neutral-200 transition inline-flex items-center gap-1"
            >
              <Copy class="w-3 h-3" />
              <span>Copy</span>
            </button>
          </div>
          <div class="rounded-xl bg-neutral-950 border border-neutral-800 p-3 text-xs text-neutral-200 font-mono whitespace-pre-wrap">
            {activeModalResult.prompt}
          </div>
        </div>

        <!-- Model Response or Error -->
        <div>
          <div class="flex items-center justify-between text-xs font-semibold text-neutral-400 uppercase tracking-wider mb-1.5">
            <span>Model Output / Response Details</span>
            {#if activeModalResult.response}
              <button
                type="button"
                onclick={() => copyToClipboard(activeModalResult?.response || '')}
                class="text-neutral-400 hover:text-neutral-200 transition inline-flex items-center gap-1"
              >
                <Copy class="w-3 h-3" />
                <span>Copy Response</span>
              </button>
            {/if}
          </div>
          <div class="rounded-xl bg-neutral-950 border border-neutral-800 p-4 text-xs text-neutral-200 overflow-x-auto max-h-80 prose prose-invert max-w-none">
            {#if activeModalResult.response}
              {@html marked(activeModalResult.response)}
            {:else if activeModalResult.error}
              <div class="space-y-2">
                <div class="text-rose-400 font-mono font-semibold">{activeModalResult.error}</div>
                <div class="text-neutral-400 text-[11px]">
                  HTTP Status: {activeModalResult.httpStatus || 500}. Pastikan model '{selectedModel}' aktif di provider upstream, dan key atau koneksi tidak expired.
                </div>
              </div>
            {:else}
              <div class="text-neutral-500 italic">No response received yet. Run benchmark to probe this test case.</div>
            {/if}
          </div>
        </div>
      </div>

      <!-- Modal Footer -->
      <div class="flex items-center justify-end border-t border-neutral-800 px-5 py-3 bg-neutral-950">
        <button
          type="button"
          onclick={() => (activeModalResult = null)}
          class="rounded-xl bg-neutral-800 hover:bg-neutral-700 px-4 py-2 text-xs font-semibold text-neutral-200 transition"
        >
          Close
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- FULL MODEL PICKER MODAL (Browse all 550+ models) -->
{#if showModelPickerModal}
  <ModelPickerModal
    isOpen={showModelPickerModal}
    target="router"
    {connections}
    {combos}
    {providerNodes}
    onSelect={(val) => {
      selectedModel = val
      showModelPickerModal = false
    }}
    onClose={() => (showModelPickerModal = false)}
  />
{/if}
