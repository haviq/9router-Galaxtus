<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { marked } from 'marked'
  import { api, getAuthHeaders, type ProviderNode } from '../api/client'

  // Configure marked for clean Github-flavored markdown
  marked.setOptions({
    gfm: true,
    breaks: true,
  })

  interface ModelItem {
    id: string
    provider: string
    title: string
    isCustom?: boolean
    isCombo?: boolean
    status?: string
  }

  interface ChatMessage {
    id: string
    role: 'user' | 'assistant'
    content: string
    ttft?: number
    tokensPerSec?: string
    tokens?: number
    isGenerating?: boolean
    error?: string
  }

  // Active models state (loaded from router)
  let models = $state<ModelItem[]>([])
  let loadingModels = $state(false)
  let selectedModel = $state<string>(
    typeof localStorage !== 'undefined' ? localStorage.getItem('9router_playground_model') || '' : ''
  )
  let isModelDropdownOpen = $state(false)
  let modelSearch = $state('')

  // Parameters (matching user reference screenshot)
  let systemPrompt = $state<string>(
    typeof localStorage !== 'undefined'
      ? localStorage.getItem('9router_playground_system_prompt') || ''
      : ''
  )
  let temperature = $state<number>(1.0)
  let maxTokens = $state<number>(65536)
  let topP = $state<number>(1.0)
  let frequencyPenalty = $state<number>(0.0)
  let presencePenalty = $state<number>(0.0)
  let topK = $state<number>(0)
  let repetitionPenalty = $state<number>(1.0)
  let contextLength = $state<number>(131072)
  let streamOutput = $state<boolean>(true)
  let paramsFeedback = $state<string>('')

  async function loadModelParameters(target: string) {
    if (!target) return
    try {
      const p = await api.getModelParameters(target)
      if (p && Object.keys(p).length > 0) {
        if (p.temperature !== undefined && p.temperature !== null) temperature = p.temperature
        if (p.topP !== undefined && p.topP !== null) topP = p.topP
        if (p.topK !== undefined && p.topK !== null) topK = p.topK
        if (p.repetitionPenalty !== undefined && p.repetitionPenalty !== null) repetitionPenalty = p.repetitionPenalty
        if (p.frequencyPenalty !== undefined && p.frequencyPenalty !== null) frequencyPenalty = p.frequencyPenalty
        if (p.presencePenalty !== undefined && p.presencePenalty !== null) presencePenalty = p.presencePenalty
        if (p.maxTokens !== undefined && p.maxTokens !== null) maxTokens = p.maxTokens
        if (p.contextLength !== undefined && p.contextLength !== null) contextLength = p.contextLength
        if (p.stream === 'on') streamOutput = true
        if (p.stream === 'off') streamOutput = false
      }
    } catch {}
  }

  async function saveParametersAsModelDefault() {
    if (!selectedModel) return
    try {
      await api.setModelParameters(selectedModel, {
        temperature: Number(temperature),
        topP: Number(topP),
        topK: Number(topK),
        repetitionPenalty: Number(repetitionPenalty),
        frequencyPenalty: Number(frequencyPenalty),
        presencePenalty: Number(presencePenalty),
        maxTokens: Number(maxTokens),
        contextLength: Number(contextLength),
        stream: streamOutput ? 'on' : 'off',
      })
      paramsFeedback = `Parameter default tersimpan untuk ${selectedModel}!`
      setTimeout(() => (paramsFeedback = ''), 3500)
    } catch (err: any) {
      paramsFeedback = `Gagal menyimpan: ${err?.message || err}`
      setTimeout(() => (paramsFeedback = ''), 4000)
    }
  }

  // Collapsible accordions
  let isAdvancedOpen = $state(false)
  let isFunctionCallingOpen = $state(false)

  // Chat conversation state
  let messages = $state<ChatMessage[]>([])
  let userInput = $state('')
  let isGenerating = $state(false)
  let abortController = $state<AbortController | null>(null)
  let chatContainerEl = $state<HTMLElement | null>(null)
  let textareaEl = $state<HTMLTextAreaElement | null>(null)
  let copiedBlock = $state<string | null>(null)

  // TTFT live timer
  let timerInterval: any = null
  let elapsedSeconds = $state(0)

  // Persist settings
  $effect(() => {
    if (typeof localStorage !== 'undefined') {
      if (selectedModel) localStorage.setItem('9router_playground_model', selectedModel)
      localStorage.setItem('9router_playground_system_prompt', systemPrompt)
    }
  })

  onMount(async () => {
    await autoSyncApiKey()
    await loadActiveModels()
    if (textareaEl) textareaEl.focus()
  })

  // Auto-sync an active client API key to localStorage if not yet present
  async function autoSyncApiKey() {
    try {
      if (typeof localStorage === 'undefined') return
      const existing = localStorage.getItem('9router_key')
      if (existing && existing.trim()) return

      const keys = await api.getApiKeys().catch(() => [])
      const active = keys.find((k) => k.isActive === 1)
      if (active && active.key) {
        localStorage.setItem('9router_key', active.key)
      }
    } catch {}
  }

  // Load only active/routable models from the 9router gateway
  async function loadActiveModels() {
    loadingModels = true
    try {
      const authHeaders = getAuthHeaders()

      // 1. Fetch combos & provider nodes & models simultaneously
      const [combosRes, nodesRes, modelsRes, statusRes] = await Promise.all([
        api.getCombos().catch(() => []),
        api.getProviderNodes().catch(() => [] as ProviderNode[]),
        fetch('/api/models', { headers: authHeaders })
          .then((r) => (r.ok ? r.json() : null))
          .catch(() => null),
        api.getJailbreakModelStatus().catch(() => null),
      ])

      const nodeMap = new Map<string, string>()
      for (const node of nodesRes) {
        if (node.prefix) nodeMap.set(node.prefix.toLowerCase(), node.name)
        if (node.id) nodeMap.set(node.id.toLowerCase(), node.name)
      }

      const list: ModelItem[] = []

      // A. Add active Combos first
      if (Array.isArray(combosRes) && combosRes.length > 0) {
        for (const combo of combosRes) {
          if (combo.name) {
            list.push({
              id: combo.name,
              provider: 'Combos & Routers',
              title: combo.name,
              isCombo: true,
              status: 'ok',
            })
          }
        }
      }

      // B. Add Models returned by /api/models
      if (Array.isArray(modelsRes?.data) && modelsRes.data.length > 0) {
        for (const m of modelsRes.data) {
          if (!m.id) continue
          if (list.some((item) => item.id === m.id)) continue
          list.push(parseModelId(m.id, nodeMap))
        }
      }

      // C. Cross-reference with Jailbreak model status
      if (Array.isArray(statusRes?.models) && statusRes.models.length > 0) {
        for (const m of statusRes.models) {
          if (!m.id || m.status === 'excluded') continue
          const existing = list.find((item) => item.id === m.id)
          if (existing) {
            existing.status = m.status
          } else {
            const item = parseModelId(m.id, nodeMap)
            item.status = m.status
            list.push(item)
          }
        }
      }

      // Fallback if list is still empty: fetch /v1/models
      if (list.length === 0) {
        const v1Res = await fetch('/v1/models', { headers: authHeaders })
          .then((r) => (r.ok ? r.json() : null))
          .catch(() => null)
        if (Array.isArray(v1Res?.data)) {
          for (const m of v1Res.data) {
            if (m.id) list.push(parseModelId(m.id, nodeMap))
          }
        }
      }

      models = list

      // Set default selected model
      if (models.length > 0) {
        if (!selectedModel || !models.some((m) => m.id === selectedModel)) {
          // Prioritize jb, FREEV2, or the first combo/model
          const preferred =
            models.find((m) => m.id.toLowerCase() === 'jb') ||
            models.find((m) => m.id.toLowerCase() === 'freev2') ||
            models.find((m) => m.isCombo) ||
            models[0]
          selectedModel = preferred.id
        }
      }
    } catch (err) {
      console.error('Failed to load active models:', err)
    } finally {
      loadingModels = false
    }
  }

  function parseModelId(id: string, nodeMap?: Map<string, string>): ModelItem {
    const slash = id.indexOf('/')
    if (slash > 0) {
      const pfx = id.slice(0, slash)
      const title = id.slice(slash + 1)
      const customName = nodeMap?.get(pfx.toLowerCase())
      return {
        id,
        provider: customName ? `${customName} (${pfx})` : pfx,
        title,
        isCustom: !!customName,
      }
    }
    return {
      id,
      provider: 'Direct & Combos',
      title: id,
    }
  }

  // Filtered models for dropdown
  let filteredModels = $derived.by(() => {
    const q = modelSearch.trim().toLowerCase()
    if (!q) return models
    return models.filter(
      (m) =>
        m.id.toLowerCase().includes(q) ||
        m.title.toLowerCase().includes(q) ||
        m.provider.toLowerCase().includes(q)
    )
  })

  // Group filtered models by provider
  let groupedModels = $derived.by(() => {
    const map = new Map<string, ModelItem[]>()
    for (const m of filteredModels) {
      const group = map.get(m.provider) || []
      group.push(m)
      map.set(m.provider, group)
    }
    return Array.from(map.entries()).map(([provider, items]) => ({
      provider,
      items: items.sort((a, b) => a.title.localeCompare(b.title)),
    }))
  })

  let selectedModelItem = $derived(
    models.find((m) => m.id === selectedModel) || {
      id: selectedModel || 'No model',
      title: selectedModel || 'No model',
      provider: 'Direct',
    }
  )

  // Scroll to bottom of chat
  async function scrollToBottom() {
    await tick()
    if (chatContainerEl) {
      chatContainerEl.scrollTop = chatContainerEl.scrollHeight
    }
  }

  // Helper to reliably update assistant message in Svelte 5 state
  function updateAssistantMessage(id: string, patch: Partial<ChatMessage>) {
    const idx = messages.findIndex((m) => m.id === id)
    if (idx !== -1) {
      messages[idx] = { ...messages[idx], ...patch }
      messages = [...messages]
    }
  }

  // Send message
  async function sendMessage() {
    const text = userInput.trim()
    if (!text || isGenerating || !selectedModel) return

    const userMsg: ChatMessage = {
      id: 'msg-' + Date.now() + '-u',
      role: 'user',
      content: text,
    }

    const assistantId = 'msg-' + (Date.now() + 1) + '-a'
    const assistantMsg: ChatMessage = {
      id: assistantId,
      role: 'assistant',
      content: '',
      isGenerating: true,
    }

    messages = [...messages, userMsg, assistantMsg]
    userInput = ''
    isGenerating = true
    elapsedSeconds = 0
    scrollToBottom()

    abortController = new AbortController()
    const startTime = performance.now()
    let ttft: number | null = null
    let tokenCount = 0

    clearInterval(timerInterval)
    timerInterval = setInterval(() => {
      elapsedSeconds = Math.max(1, Math.round((performance.now() - startTime) / 1000))
    }, 250)

    try {
      // Build conversation payload
      const payloadMessages = [
        ...(systemPrompt.trim() ? [{ role: 'system', content: systemPrompt.trim() }] : []),
        ...messages
          .filter((m) => m.id !== assistantId && !m.error)
          .map((m) => ({ role: m.role, content: m.content })),
      ]

      const requestBody: Record<string, any> = {
        model: selectedModel,
        messages: payloadMessages,
        temperature: Number(temperature),
        max_tokens: Number(maxTokens),
        top_p: Number(topP),
        stream: Boolean(streamOutput),
      }

      if (topK > 0) requestBody.top_k = Number(topK)
      if (repetitionPenalty !== 1.0) requestBody.repetition_penalty = Number(repetitionPenalty)
      if (frequencyPenalty !== 0) requestBody.frequency_penalty = Number(frequencyPenalty)
      if (presencePenalty !== 0) requestBody.presence_penalty = Number(presencePenalty)
      if (contextLength > 0) requestBody.context_length = Number(contextLength)

      const authHeaders = getAuthHeaders()

      // Primary: /api/chat/completions (native dashboard session)
      // Fallback: /v1/chat/completions
      let res: Response | null = await fetch('/api/chat/completions', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...authHeaders,
        },
        body: JSON.stringify(requestBody),
        signal: abortController.signal,
      }).catch(() => null)

      if (!res || !res.ok) {
        // Fallback to /v1/chat/completions
        const fallbackRes = await fetch('/v1/chat/completions', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            ...authHeaders,
          },
          body: JSON.stringify(requestBody),
          signal: abortController.signal,
        }).catch(() => null)

        if (fallbackRes && (fallbackRes.ok || !res)) {
          res = fallbackRes
        }
      }

      if (!res || !res.ok) {
        let errorText = res ? `HTTP ${res.status}` : 'Gagal menghubungi server 9router'
        if (res) {
          try {
            const errJson = await res.json()
            if (errJson?.error?.message) {
              errorText = errJson.error.message
            } else if (errJson?.message) {
              errorText = errJson.message
            }
          } catch {
            const txt = await res.text().catch(() => '')
            if (txt) errorText += `: ${txt}`
          }
        }
        throw new Error(errorText)
      }

      // Check if response is stream
      const isStream = res.headers.get('content-type')?.includes('text/event-stream')

      if (isStream && res.body) {
        const reader = res.body.getReader()
        const decoder = new TextDecoder()
        let buffer = ''
        let accumulated = ''

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
              const delta = parsed.choices?.[0]?.delta?.content
              if (delta) {
                if (ttft === null) {
                  ttft = Math.round(performance.now() - startTime)
                  clearInterval(timerInterval)
                }
                tokenCount += Math.max(1, Math.round(delta.length / 3.8))
                accumulated += delta
                updateAssistantMessage(assistantId, {
                  content: accumulated,
                  isGenerating: true,
                })
                scrollToBottom()
              }
            } catch {
              // Partial JSON, continue
            }
          }
        }
      } else {
        // Non-streaming response
        const data = await res.json()
        const reply = data.choices?.[0]?.message?.content || ''
        ttft = Math.round(performance.now() - startTime)
        tokenCount = data.usage?.completion_tokens || Math.max(1, Math.round(reply.length / 3.8))
        updateAssistantMessage(assistantId, {
          content: reply,
          isGenerating: false,
        })
      }

      // Compute performance metrics
      const totalTimeSec = (performance.now() - startTime) / 1000
      const speed = totalTimeSec > 0 ? (tokenCount / totalTimeSec).toFixed(2) : '0'

      updateAssistantMessage(assistantId, {
        isGenerating: false,
        ttft: ttft ?? Math.round(performance.now() - startTime),
        tokensPerSec: speed,
        tokens: tokenCount,
      })
    } catch (err: any) {
      clearInterval(timerInterval)
      if (err.name === 'AbortError') {
        const cur = messages.find((m) => m.id === assistantId)?.content || ''
        updateAssistantMessage(assistantId, {
          content: cur + '\n\n*(Generasi dihentikan oleh pengguna)*',
          isGenerating: false,
        })
      } else {
        updateAssistantMessage(assistantId, {
          error: err?.message || String(err),
          isGenerating: false,
        })
      }
    } finally {
      clearInterval(timerInterval)
      isGenerating = false
      abortController = null
      scrollToBottom()
    }
  }

  function retryMessage(errorMsgId: string) {
    const idx = messages.findIndex((m) => m.id === errorMsgId)
    if (idx > 0 && messages[idx - 1].role === 'user') {
      const prevPrompt = messages[idx - 1].content
      // Remove both error message and user message, then re-send
      messages = messages.filter((m, i) => i !== idx && i !== idx - 1)
      userInput = prevPrompt
      sendMessage()
    }
  }

  function stopGeneration() {
    if (abortController) {
      abortController.abort()
      abortController = null
      isGenerating = false
      clearInterval(timerInterval)
    }
  }

  function clearChat() {
    if (isGenerating) stopGeneration()
    messages = []
    if (textareaEl) textareaEl.focus()
  }

  function copyText(str: string, id: string) {
    if (navigator?.clipboard?.writeText) {
      navigator.clipboard.writeText(str)
      copiedBlock = id
      setTimeout(() => {
        if (copiedBlock === id) copiedBlock = null
      }, 2000)
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      sendMessage()
    }
  }
</script>

<div class="h-[calc(100vh-7rem)] flex flex-col md:flex-row gap-4 overflow-hidden">
  <!-- LEFT SETTINGS SIDEBAR (Parameters like user photo) -->
  <aside class="w-full md:w-80 shrink-0 flex flex-col rounded-xl bg-surface-1/70 border border-border-subtle shadow-sm overflow-hidden">
    <div class="p-4 border-b border-border-subtle flex items-center justify-between bg-surface-2/40">
      <div class="flex items-center gap-2">
        <span class="material-symbols-outlined text-brand-500 text-[18px]">tune</span>
        <h2 class="text-xs font-bold text-text-main uppercase tracking-wider">Model Parameters</h2>
      </div>
      <span class="px-2 py-0.5 rounded-[5px] bg-emerald-500/10 border border-emerald-500/20 text-emerald-500 text-[10px] font-semibold flex items-center gap-1">
        <span class="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse"></span>
        Direct 9router
      </span>
    </div>

    <div class="flex-1 overflow-y-auto p-4 space-y-4 text-xs">
      <!-- System prompt -->
      <div class="space-y-1.5">
        <div class="flex items-center justify-between">
          <label for="playground-sys-prompt" class="font-semibold text-text-main flex items-center gap-1">
            System prompt
            <span class="material-symbols-outlined text-[13px] text-text-subtle" title="Instruksi sistem yang menetapkan peran, kepribadian, dan format respons model">info</span>
          </label>
          {#if systemPrompt}
            <button
              type="button"
              onclick={() => (systemPrompt = '')}
              class="text-[11px] text-text-subtle hover:text-rose-500 transition-colors"
            >
              Hapus
            </button>
          {/if}
        </div>
        <textarea
          id="playground-sys-prompt"
          bind:value={systemPrompt}
          placeholder="Masukkan instruksi sistem yang akan memandu respons model..."
          rows="6"
          class="w-full p-2.5 rounded-lg bg-surface-2/80 border border-border-subtle text-text-main text-xs font-sans placeholder:text-text-subtle focus:border-brand-500/50 outline-none resize-y min-h-[120px] transition-all"
        ></textarea>
        <!-- User / Token Counter below System Prompt (matching photo) -->
        <div class="flex items-center justify-between text-[11px] text-text-subtle font-mono">
          <span>Tokens: ~{Math.round(systemPrompt.length / 3.8)}</span>
          <span class="text-text-muted">User: {systemPrompt.length}</span>
        </div>
      </div>

      <!-- Temperature -->
      <div class="space-y-1.5 pt-1 border-t border-border-subtle/50">
        <div class="flex items-center justify-between">
          <span class="font-semibold text-text-main flex items-center gap-1">
            Temperature
            <span class="material-symbols-outlined text-[13px] text-text-subtle" title="Mengatur keacakan sampling (0 = deterministik/logis, 1 = seimbang, 2 = sangat kreatif)">info</span>
          </span>
          <input
            type="number"
            min="0"
            max="2"
            step="0.05"
            bind:value={temperature}
            class="w-16 px-2 py-0.5 rounded-[6px] bg-surface-2 border border-border-subtle text-right text-xs font-mono text-text-main outline-none focus:border-brand-500/50"
          />
        </div>
        <input
          type="range"
          min="0"
          max="2"
          step="0.05"
          bind:value={temperature}
          class="w-full accent-amber-500 cursor-pointer"
        />
      </div>

      <!-- Max tokens -->
      <div class="space-y-1.5 pt-1 border-t border-border-subtle/50">
        <div class="flex items-center justify-between">
          <span class="font-semibold text-text-main flex items-center gap-1">
            Max tokens
            <span class="material-symbols-outlined text-[13px] text-text-subtle" title="Batas jumlah token maksimal yang dihasilkan oleh model">info</span>
          </span>
          <input
            type="number"
            min="1"
            max="128000"
            step="128"
            bind:value={maxTokens}
            class="w-20 px-2 py-0.5 rounded-[6px] bg-surface-2 border border-border-subtle text-right text-xs font-mono text-text-main outline-none focus:border-brand-500/50"
          />
        </div>
        <input
          type="range"
          min="1"
          max="128000"
          step="128"
          bind:value={maxTokens}
          class="w-full accent-amber-500 cursor-pointer"
        />
      </div>

      <!-- Top P -->
      <div class="space-y-1.5 pt-1 border-t border-border-subtle/50">
        <div class="flex items-center justify-between">
          <span class="font-semibold text-text-main flex items-center gap-1">
            Top P
            <span class="material-symbols-outlined text-[13px] text-text-subtle" title="Sampling nucleus: mempertimbangkan kemungkinan token dengan massa probabilitas top_p">info</span>
          </span>
          <input
            type="number"
            min="0"
            max="1"
            step="0.05"
            bind:value={topP}
            class="w-16 px-2 py-0.5 rounded-[6px] bg-surface-2 border border-border-subtle text-right text-xs font-mono text-text-main outline-none focus:border-brand-500/50"
          />
        </div>
        <input
          type="range"
          min="0"
          max="1"
          step="0.05"
          bind:value={topP}
          class="w-full accent-amber-500 cursor-pointer"
        />
      </div>

      <!-- Collapsible: ADVANCED (Frequency/Presence penalty & stream toggle) -->
      <div class="pt-1 border-t border-border-subtle/50">
        <button
          type="button"
          onclick={() => (isAdvancedOpen = !isAdvancedOpen)}
          class="w-full flex items-center justify-between py-2 text-text-main font-bold uppercase tracking-wider text-[11px] hover:text-brand-500 transition-colors"
        >
          <span>Advanced</span>
          <span class="material-symbols-outlined text-[16px] transition-transform duration-200 {isAdvancedOpen ? 'rotate-180' : ''}">
            expand_more
          </span>
        </button>

        {#if isAdvancedOpen}
          <div class="space-y-3.5 pb-2 pt-1 animate-fadeIn">
            <!-- Frequency penalty -->
            <div class="space-y-1">
              <div class="flex items-center justify-between">
                <span class="text-text-subtle">Frequency penalty</span>
                <span class="font-mono text-text-main">{frequencyPenalty}</span>
              </div>
              <input
                type="range"
                min="-2"
                max="2"
                step="0.1"
                bind:value={frequencyPenalty}
                class="w-full accent-amber-500 cursor-pointer"
              />
            </div>

            <!-- Presence penalty -->
            <div class="space-y-1">
              <div class="flex items-center justify-between">
                <span class="text-text-subtle">Presence penalty</span>
                <span class="font-mono text-text-main">{presencePenalty}</span>
              </div>
              <input
                type="range"
                min="-2"
                max="2"
                step="0.1"
                bind:value={presencePenalty}
                class="w-full accent-amber-500 cursor-pointer"
              />
            </div>

            <!-- Top K -->
            <div class="space-y-1">
              <div class="flex items-center justify-between">
                <span class="text-text-subtle">Top K</span>
                <span class="font-mono text-text-main">{topK}</span>
              </div>
              <input
                type="range"
                min="0"
                max="100"
                step="1"
                bind:value={topK}
                class="w-full accent-amber-500 cursor-pointer"
              />
            </div>

            <!-- Repetition penalty -->
            <div class="space-y-1">
              <div class="flex items-center justify-between">
                <span class="text-text-subtle">Repetition penalty</span>
                <span class="font-mono text-text-main">{repetitionPenalty}</span>
              </div>
              <input
                type="range"
                min="0.5"
                max="2.0"
                step="0.05"
                bind:value={repetitionPenalty}
                class="w-full accent-amber-500 cursor-pointer"
              />
            </div>

            <!-- Context length -->
            <div class="space-y-1">
              <div class="flex items-center justify-between">
                <span class="text-text-subtle">Context length</span>
                <span class="font-mono text-text-main">{contextLength >= 1024 ? Math.round(contextLength / 1024) + 'k' : contextLength}</span>
              </div>
              <input
                type="number"
                min="1024"
                max="2097152"
                step="4096"
                bind:value={contextLength}
                class="w-full px-2 py-1 rounded bg-surface-2 border border-border-subtle font-mono text-right text-xs"
              />
            </div>

            <!-- Stream output switch -->
            <label class="flex items-center justify-between cursor-pointer pt-1">
              <span class="text-text-subtle">Stream tokens</span>
              <input
                type="checkbox"
                bind:checked={streamOutput}
                class="accent-brand-500 rounded cursor-pointer"
              />
            </label>
          </div>
        {/if}

        <!-- Save parameters as default for model/combo button -->
        {#if selectedModel}
          <div class="pt-2">
            <button
              type="button"
              onclick={saveParametersAsModelDefault}
              class="w-full py-1.5 px-3 rounded-lg border border-brand-500/30 bg-brand-500/10 hover:bg-brand-500/20 text-brand-500 text-xs font-semibold flex items-center justify-center gap-1.5 transition-colors"
            >
              <span class="material-symbols-outlined text-[15px]">save</span>
              Simpan Default Parameter
            </button>
            {#if paramsFeedback}
              <p class="text-[11px] text-emerald-400 text-center mt-1 font-medium animate-fadeIn">
                {paramsFeedback}
              </p>
            {/if}
          </div>
        {/if}
      </div>

      <!-- Collapsible: FUNCTION CALLING -->
      <div class="pt-1 border-t border-border-subtle/50">
        <div class="w-full flex items-center justify-between py-2">
          <div class="flex items-center gap-1 text-text-main font-bold uppercase tracking-wider text-[11px]">
            <span>Function Calling</span>
            <span class="material-symbols-outlined text-[13px] text-text-subtle" title="Mendukung penambahan function tool definition (schema JSON)">info</span>
          </div>
          <button
            type="button"
            onclick={() => (isFunctionCallingOpen = !isFunctionCallingOpen)}
            class="px-2 py-0.5 rounded text-[11px] border border-border-subtle hover:border-brand-500/50 text-text-subtle hover:text-text-main transition-colors flex items-center gap-0.5"
          >
            <span class="material-symbols-outlined text-[13px]">add</span>
            <span>Add</span>
          </button>
        </div>

        {#if isFunctionCallingOpen}
          <div class="p-2 rounded-lg bg-surface-2/60 border border-border-subtle text-[11px] text-text-subtle">
            <p>Fitur deklarasi tools/functions aktif secara default di router untuk model yang mendukung OpenAI tools format.</p>
          </div>
        {:else}
          <p class="text-[11px] text-text-subtle italic">No functions added.</p>
        {/if}
      </div>
    </div>
  </aside>

  <!-- RIGHT CHAT CONTAINER -->
  <main class="flex-1 flex flex-col rounded-xl bg-surface-1/70 border border-border-subtle shadow-sm overflow-hidden min-w-0">
    <!-- Top Bar: Model Selector Dropdown, Details link, Clear Chat (matching photo) -->
    <header class="p-3 border-b border-border-subtle flex items-center justify-between bg-surface-2/40 gap-3">
      <!-- Model Dropdown -->
      <div class="relative flex-1 max-w-md">
        <button
          type="button"
          onclick={() => (isModelDropdownOpen = !isModelDropdownOpen)}
          class="w-full flex items-center justify-between gap-2 px-3 py-1.5 rounded-lg bg-surface-2 border border-border-subtle hover:border-brand-500/50 transition-colors text-xs font-medium text-text-main text-left shadow-sm"
        >
          <div class="flex items-center gap-2 truncate">
            <span class="material-symbols-outlined text-[16px] text-brand-500 shrink-0">
              {selectedModelItem.isCombo ? 'alt_route' : 'smart_toy'}
            </span>
            <span class="truncate font-semibold">{selectedModelItem.title}</span>
            <span class="text-[10px] px-1.5 py-0.2 rounded bg-surface-3 text-text-subtle shrink-0">
              {selectedModelItem.provider}
            </span>
            {#if selectedModelItem.status === 'ok'}
              <span class="w-1.5 h-1.5 rounded-full bg-emerald-500 shrink-0" title="Status: OK"></span>
            {/if}
          </div>
          <span class="material-symbols-outlined text-[16px] text-text-subtle shrink-0">
            {isModelDropdownOpen ? 'expand_less' : 'expand_more'}
          </span>
        </button>

        {#if isModelDropdownOpen}
          <!-- Backdrop to close dropdown -->
          <button
            type="button"
            class="fixed inset-0 z-30 cursor-default bg-transparent"
            onclick={() => (isModelDropdownOpen = false)}
            aria-label="Tutup pilihan model"
          ></button>

          <!-- Dropdown menu -->
          <div class="absolute left-0 top-full mt-1.5 w-full md:w-96 rounded-xl bg-surface-2 border border-border-subtle shadow-2xl z-40 max-h-80 flex flex-col overflow-hidden text-xs">
            <div class="p-2 border-b border-border-subtle bg-surface-1/50 flex items-center gap-2">
              <span class="material-symbols-outlined text-[15px] text-text-subtle">search</span>
              <input
                type="text"
                placeholder="Cari model atau provider..."
                bind:value={modelSearch}
                class="w-full bg-transparent outline-none text-xs text-text-main placeholder:text-text-subtle"
                autofocus
              />
              {#if loadingModels}
                <span class="material-symbols-outlined text-[14px] text-brand-500 animate-spin">sync</span>
              {/if}
            </div>

            <div class="overflow-y-auto p-1.5 space-y-2 flex-1">
              {#if groupedModels.length === 0}
                <div class="p-4 text-center text-text-subtle text-xs">
                  Tidak ada model ditemukan.
                </div>
              {:else}
                {#each groupedModels as group}
                  <div>
                    <div class="px-2 py-1 text-[10px] font-bold text-text-subtle uppercase tracking-wider flex items-center justify-between">
                      <span>{group.provider}</span>
                      <span class="font-mono text-[9px] text-text-muted">{group.items.length}</span>
                    </div>
                    <div class="space-y-0.5">
                      {#each group.items as item}
                        <button
                          type="button"
                          onclick={() => {
                            selectedModel = item.id
                            isModelDropdownOpen = false
                            loadModelParameters(item.id)
                          }}
                          class="w-full flex items-center justify-between px-2.5 py-1.5 rounded-lg text-left transition-colors {selectedModel === item.id
                            ? 'bg-brand-500/15 text-brand-500 font-semibold'
                            : 'hover:bg-surface-3/70 text-text-main'}"
                        >
                          <div class="truncate flex items-center gap-1.5">
                            {#if item.status === 'ok'}
                              <span class="w-1.5 h-1.5 rounded-full bg-emerald-500 shrink-0"></span>
                            {:else if item.status === 'down'}
                              <span class="w-1.5 h-1.5 rounded-full bg-rose-500 shrink-0"></span>
                            {:else}
                              <span class="w-1.5 h-1.5 rounded-full bg-surface-4 shrink-0"></span>
                            {/if}
                            <span class="truncate">{item.title}</span>
                          </div>
                          {#if selectedModel === item.id}
                            <span class="material-symbols-outlined text-[15px] text-brand-500">check</span>
                          {/if}
                        </button>
                      {/each}
                    </div>
                  </div>
                {/each}
              {/if}
            </div>
          </div>
        {/if}
      </div>

      <!-- Action Buttons: Details, Clear -->
      <div class="flex items-center gap-3 text-xs">
        <a
          href="#/models"
          class="text-text-subtle hover:text-text-main transition-colors flex items-center gap-0.5 font-medium"
          title="Buka Model Catalog 9router"
        >
          <span>Details</span>
          <span class="material-symbols-outlined text-[14px]">north_east</span>
        </a>

        <button
          type="button"
          onclick={clearChat}
          class="text-text-subtle hover:text-rose-500 transition-colors flex items-center gap-1 font-medium"
          title="Bersihkan riwayat percakapan"
        >
          <span class="material-symbols-outlined text-[15px]">delete</span>
          <span>Clear</span>
        </button>
      </div>
    </header>

    <!-- Chat Conversation Stream Container -->
    <div
      bind:this={chatContainerEl}
      class="flex-1 overflow-y-auto p-4 space-y-4 select-text"
    >
      {#if messages.length === 0}
        <div class="h-full flex flex-col items-center justify-center text-center text-text-subtle p-6 space-y-3">
          <div class="w-12 h-12 rounded-2xl bg-surface-2 border border-border-subtle flex items-center justify-center text-brand-500 shadow-inner">
            <span class="material-symbols-outlined text-[26px]">chat</span>
          </div>
          <div>
            <h3 class="font-bold text-text-main text-sm">9router AI Playground</h3>
            <p class="text-xs max-w-sm mt-1 text-text-subtle leading-relaxed">
              Ketik pesan di bawah untuk menguji respons model secara langsung melalui router lokal Anda.
            </p>
          </div>
          <div class="flex flex-wrap gap-2 justify-center pt-2">
            <button
              type="button"
              onclick={() => {
                userInput = 'Jelaskan konsep dasar low-level routing di 9router secara singkat.'
                if (textareaEl) textareaEl.focus()
              }}
              class="px-3 py-1.5 rounded-lg bg-surface-2 hover:bg-surface-3 border border-border-subtle text-xs text-text-main transition-colors"
            >
              "Jelaskan routing 9router"
            </button>
            <button
              type="button"
              onclick={() => {
                userInput = 'Buat satu fungsi helper Go untuk parsing SSE stream data.'
                if (textareaEl) textareaEl.focus()
              }}
              class="px-3 py-1.5 rounded-lg bg-surface-2 hover:bg-surface-3 border border-border-subtle text-xs text-text-main transition-colors"
            >
              "Helper Go SSE parser"
            </button>
          </div>
        </div>
      {:else}
        {#each messages as msg (msg.id)}
          <div class="flex flex-col gap-1.5 {msg.role === 'user' ? 'items-end' : 'items-start'}">
            <div class="max-w-[85%] rounded-xl px-4 py-3 shadow-sm {msg.role === 'user'
              ? 'bg-brand-500 text-white font-sans text-xs'
              : 'bg-surface-2/60 border border-border-subtle text-text-main text-xs'}">

              {#if msg.role === 'user'}
                <p class="whitespace-pre-wrap leading-relaxed select-text">{msg.content}</p>
              {:else}
                <!-- Assistant Message Content with Markdown Parsing -->
                {#if msg.error}
                  <div class="p-3 rounded-lg bg-rose-500/10 border border-rose-500/40 text-rose-500 text-xs font-mono space-y-2">
                    <div class="flex items-center justify-between">
                      <p class="font-bold flex items-center gap-1 text-rose-400">
                        <span class="material-symbols-outlined text-[16px]">error</span>
                        Gagal Menghubungi Model
                      </p>
                      <button
                        type="button"
                        onclick={() => retryMessage(msg.id)}
                        class="text-[11px] px-2 py-0.5 rounded border border-rose-500/40 bg-rose-500/20 hover:bg-rose-500/30 text-rose-400 transition-colors flex items-center gap-1"
                        title="Coba kirim ulang pesan ini"
                      >
                        <span class="material-symbols-outlined text-[13px]">replay</span>
                        <span>Coba Lagi</span>
                      </button>
                    </div>
                    <p class="whitespace-pre-wrap break-all text-[11px] leading-relaxed bg-black/30 p-2 rounded border border-rose-500/20">
                      {msg.error}
                    </p>
                  </div>
                {:else if msg.content}
                  <div class="changelog-body prose-invert max-w-none text-xs leading-relaxed select-text">
                    {@html marked.parse(msg.content)}
                  </div>
                  {#if msg.isGenerating}
                    <span class="inline-block w-2 h-4 bg-brand-500 animate-pulse ml-0.5 align-middle"></span>
                  {/if}
                {:else if msg.isGenerating}
                  <div class="flex items-center justify-between gap-3 text-text-subtle py-1 px-1">
                    <div class="flex items-center gap-2">
                      <span class="material-symbols-outlined text-[16px] animate-spin text-brand-500">progress_activity</span>
                      <span>Menunggu respons pertama (TTFT)…</span>
                      {#if elapsedSeconds > 0}
                        <span class="font-mono text-[11px] text-text-muted px-1.5 py-0.5 rounded bg-surface-3">
                          {elapsedSeconds}s
                        </span>
                      {/if}
                    </div>
                    <button
                      type="button"
                      onclick={stopGeneration}
                      class="text-[11px] text-rose-400 hover:text-rose-300 flex items-center gap-1 px-2 py-0.5 rounded border border-rose-500/30 hover:bg-rose-500/10 transition-colors"
                      title="Batalkan generasi"
                    >
                      <span class="material-symbols-outlined text-[13px]">cancel</span>
                      <span>Batal</span>
                    </button>
                  </div>
                {/if}
              {/if}
            </div>

            <!-- Footer Metrics (matching user photo: "944 ms ttft • 157.72 tokens/s • 860 tokens") -->
            {#if msg.role === 'assistant' && msg.ttft !== undefined}
              <div class="flex items-center gap-1.5 text-[11px] text-text-subtle font-mono px-1">
                <span>{msg.ttft} ms ttft</span>
                <span>•</span>
                <span>{msg.tokensPerSec} tokens/s</span>
                <span>•</span>
                <span>{msg.tokens} tokens</span>
                <button
                  type="button"
                  onclick={() => copyText(msg.content, msg.id)}
                  class="ml-2 hover:text-text-main text-text-subtle transition-colors flex items-center gap-0.5"
                  title="Salin respons"
                >
                  <span class="material-symbols-outlined text-[13px]">
                    {copiedBlock === msg.id ? 'check' : 'content_copy'}
                  </span>
                  <span>{copiedBlock === msg.id ? 'Copied' : 'Copy'}</span>
                </button>
              </div>
            {/if}
          </div>
        {/each}
      {/if}
    </div>

    <!-- Bottom Input Textarea & Controls (matching photo) -->
    <div class="p-3 border-t border-border-subtle bg-surface-1">
      <div class="relative rounded-xl bg-surface-2/70 border border-border-subtle focus-within:border-brand-500/50 focus-within:ring-1 focus-within:ring-brand-500/20 transition-all p-2 flex flex-col">
        <textarea
          bind:this={textareaEl}
          bind:value={userInput}
          onkeydown={handleKeydown}
          placeholder="Type a message... (Enter to send, Shift+Enter for a new line)"
          rows="2"
          class="w-full bg-transparent text-xs text-text-main placeholder:text-text-subtle outline-none resize-none px-2 py-1 min-h-[48px] max-h-36 font-sans"
        ></textarea>

        <div class="flex items-center justify-between pt-1 px-1">
          <div class="flex items-center gap-2 text-[11px] text-text-subtle font-mono">
            <span>Model: <strong class="text-text-main font-semibold">{selectedModelItem.title}</strong></span>
          </div>

          <div class="flex items-center gap-2">
            {#if isGenerating}
              <button
                type="button"
                onclick={stopGeneration}
                class="px-3 py-1.5 rounded-lg bg-rose-500 hover:bg-rose-600 text-white text-xs font-semibold flex items-center gap-1 transition-colors shadow-sm"
              >
                <span class="material-symbols-outlined text-[15px]">stop</span>
                <span>Stop</span>
              </button>
            {:else}
              <button
                type="button"
                disabled={!userInput.trim() || !selectedModel}
                onclick={sendMessage}
                class="p-2 rounded-lg bg-brand-500 hover:bg-brand-600 text-white disabled:opacity-40 disabled:cursor-not-allowed transition-colors shadow-sm flex items-center justify-center"
                title="Kirim pesan (Enter)"
              >
                <span class="material-symbols-outlined text-[17px] leading-none">arrow_upward</span>
              </button>
            {/if}
          </div>
        </div>
      </div>
    </div>
  </main>
</div>
