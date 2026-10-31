<script lang="ts">
  import { onMount } from 'svelte'
  import { api, type Settings } from '../api/client'
  import Card from '../lib/ui/Card.svelte'
  import Toggle from '../lib/ui/Toggle.svelte'
  import Button from '../lib/ui/Button.svelte'
  import Badge from '../lib/ui/Badge.svelte'

  interface Props {
    settings?: Settings | null
    onRefresh?: () => Promise<void>
  }

  let { settings = null, onRefresh }: Props = $props()

  let enabled = $state(true)
  let systemPrompt = $state('')
  let assistantPrompt = $state('')
  let temperature = $state(0.9)
  let topP = $state(0.95)

  let saving = $state(false)
  let saveSuccess = $state(false)
  let errorMsg = $state('')

  // Simulator state
  let sampleInput = $state(
    JSON.stringify(
      {
        model: 'deepseek-chat',
        messages: [
          { role: 'system', content: 'You are a default assistant.' },
          { role: 'user', content: 'What is the current status of the gateway?' },
        ],
        temperature: 0.7,
        top_p: 1.0,
      },
      null,
      2
    )
  )
  let simulatedOutput = $state('')
  let simulationError = $state('')

  onMount(() => {
    loadValues()
  })

  function loadValues() {
    if (!settings) return
    const raw = settings as Record<string, any>
    if (typeof raw.chatRewriteEnabled === 'boolean') {
      enabled = raw.chatRewriteEnabled
    }
    if (typeof raw.chatRewriteSystem === 'string') {
      systemPrompt = raw.chatRewriteSystem
    }
    if (typeof raw.chatRewriteAssistant === 'string') {
      assistantPrompt = raw.chatRewriteAssistant
    }
    if (typeof raw.chatRewriteTemperature === 'number') {
      temperature = raw.chatRewriteTemperature
    }
    if (typeof raw.chatRewriteTopP === 'number') {
      topP = raw.chatRewriteTopP
    }
    runSimulation()
  }

  $effect(() => {
    if (settings) {
      loadValues()
    }
  })

  async function handleSave() {
    saving = true
    errorMsg = ''
    try {
      await api.updateSettings({
        chatRewriteEnabled: enabled,
        chatRewriteSystem: systemPrompt,
        chatRewriteAssistant: assistantPrompt,
        chatRewriteTemperature: Number(temperature),
        chatRewriteTopP: Number(topP),
      })
      saveSuccess = true
      if (onRefresh) {
        await onRefresh()
      }
      setTimeout(() => {
        saveSuccess = false
      }, 2500)
    } catch (e) {
      errorMsg = e instanceof Error ? e.message : String(e)
    } finally {
      saving = false
    }
  }

  async function pasteToSystem() {
    if (navigator?.clipboard?.readText) {
      try {
        const text = await navigator.clipboard.readText()
        if (text) {
          systemPrompt = text
          runSimulation()
        }
      } catch (e) {
        errorMsg = 'Clipboard permission denied'
      }
    }
  }

  async function pasteToAssistant() {
    if (navigator?.clipboard?.readText) {
      try {
        const text = await navigator.clipboard.readText()
        if (text) {
          assistantPrompt = text
          runSimulation()
        }
      } catch (e) {
        errorMsg = 'Clipboard permission denied'
      }
    }
  }

  function runSimulation() {
    simulationError = ''
    try {
      const parsed = JSON.parse(sampleInput)
      const rawMsgs = Array.isArray(parsed.messages) ? parsed.messages : []
      // 1. Remove all system messages
      const filtered = rawMsgs.filter(
        (m: any) => typeof m?.role === 'string' && m.role.toLowerCase() !== 'system'
      )
      // 2. Insert two new messages at index 0 and 1
      const newMessages = [
        { role: 'system', content: systemPrompt || '(empty system prompt)' },
        { role: 'assistant', content: assistantPrompt || '(empty assistant prompt)' },
        ...filtered,
      ]

      const transformed = {
        ...parsed,
        messages: newMessages,
        temperature: Number(temperature),
        top_p: Number(topP),
      }

      simulatedOutput = JSON.stringify(transformed, null, 2)
    } catch (e) {
      simulationError = e instanceof Error ? e.message : 'Invalid JSON input'
      simulatedOutput = ''
    }
  }
</script>

<div class="space-y-6">
  <!-- Top Bar / Header -->
  <div class="flex flex-wrap items-center justify-between gap-4">
    <div class="space-y-1">
      <div class="flex items-center gap-2.5">
        <div class="w-8 h-8 rounded-lg bg-brand-500/10 flex items-center justify-center border border-brand-500/20 text-brand-500">
          <span class="material-symbols-outlined text-[20px]">alt_route</span>
        </div>
        <div>
          <div class="flex items-center gap-2">
            <h1 class="text-base font-bold text-text-main">HTTP Request Rewrite Flow</h1>
            {#if enabled}
              <Badge tone="primary" size="sm">ACTIVE</Badge>
            {:else}
              <Badge tone="default" size="sm">OFF</Badge>
            {/if}
          </div>
          <p class="text-xs text-text-muted">
            Pipeline interceptor for <code class="font-mono text-text-main font-semibold">/v1/chat/completions</code> requests.
          </p>
        </div>
      </div>
    </div>

    <div class="flex items-center gap-3">
      {#if errorMsg}
        <span class="text-xs text-error">{errorMsg}</span>
      {/if}
      <button
        type="button"
        onclick={handleSave}
        disabled={saving}
        class="flex items-center gap-1.5 px-4 py-2 rounded-xl text-xs font-semibold shadow-sm transition cursor-pointer {saveSuccess
          ? 'bg-emerald-500/10 border border-emerald-500/30 text-emerald-500'
          : 'bg-brand-500 hover:bg-brand-600 text-white'}"
      >
        <span class="material-symbols-outlined text-[16px]">
          {saveSuccess ? 'check_circle' : saving ? 'sync' : 'save'}
        </span>
        <span>{saveSuccess ? 'Saved Successfully!' : saving ? 'Saving…' : 'Save Changes'}</span>
      </button>
    </div>
  </div>

  <!-- Flow Summary Card -->
  <Card padding="md" class="border-brand-500/20 bg-gradient-to-r from-surface-1 to-brand-500/[0.03]">
    <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
      <div class="space-y-1">
        <div class="flex items-center gap-2">
          <span class="text-xs font-bold text-text-main">Workflow Specification</span>
          <span class="px-2 py-0.5 rounded text-[10px] font-mono font-semibold bg-brand-500/10 text-brand-500 border border-brand-500/20">
            Trigger: Path contains /v1/chat/completions
          </span>
        </div>
        <p class="text-xs text-text-muted leading-relaxed">
          1. Parse JSON body &bull; 2. Hapus semua messages role <code class="font-mono text-text-main font-semibold">system</code> &bull; 3. Sisipkan dua message di index 0 & 1 (<code class="font-mono text-text-main">system</code> + <code class="font-mono text-text-main">assistant</code>) &bull; 4. Set <code class="font-mono text-text-main font-semibold">temperature = {temperature}</code> dan <code class="font-mono text-text-main font-semibold">top_p = {topP}</code> &bull; 5. Header HTTP tidak diubah sama sekali.
        </p>
      </div>

      <div class="flex items-center gap-3 shrink-0">
        <span class="text-xs font-semibold text-text-main">Rewrite Flow Status</span>
        <Toggle
          checked={enabled}
          size="md"
          label="Enable Rewrite Pipeline"
          onChange={(val: boolean) => {
            enabled = val
            handleSave()
          }}
        />
      </div>
    </div>
  </Card>

  <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
    <!-- Left Column: Settings -->
    <div class="space-y-6">
      <!-- Sliders Card: Temperature & Top_P -->
      <Card padding="md" class="space-y-4">
        <div class="flex items-center justify-between pb-2 border-b border-border-subtle">
          <div class="flex items-center gap-2">
            <span class="material-symbols-outlined text-[17px] text-brand-500">tune</span>
            <h2 class="text-sm font-bold text-text-main">Sampling Parameters</h2>
          </div>
          <span class="text-[11px] text-text-subtle font-mono">Overridden on each request</span>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div class="p-3 rounded-xl bg-surface-2/60 border border-border-subtle space-y-2">
            <div class="flex justify-between items-center">
              <label for="flow-temp" class="text-xs font-semibold text-text-main">Temperature</label>
              <span class="text-xs font-mono font-bold text-brand-500">{temperature}</span>
            </div>
            <input
              id="flow-temp"
              type="range"
              min="0"
              max="2"
              step="0.05"
              bind:value={temperature}
              oninput={runSimulation}
              class="w-full accent-brand-500 cursor-pointer"
            />
            <div class="flex justify-between text-[10px] text-text-subtle font-mono">
              <span>0.0 (Precise)</span>
              <span>Default: 0.9</span>
              <span>2.0 (Creative)</span>
            </div>
          </div>

          <div class="p-3 rounded-xl bg-surface-2/60 border border-border-subtle space-y-2">
            <div class="flex justify-between items-center">
              <label for="flow-top-p" class="text-xs font-semibold text-text-main">Top P (Nucleus)</label>
              <span class="text-xs font-mono font-bold text-brand-500">{topP}</span>
            </div>
            <input
              id="flow-top-p"
              type="range"
              min="0"
              max="1"
              step="0.01"
              bind:value={topP}
              oninput={runSimulation}
              class="w-full accent-brand-500 cursor-pointer"
            />
            <div class="flex justify-between text-[10px] text-text-subtle font-mono">
              <span>0.0</span>
              <span>Default: 0.95</span>
              <span>1.0</span>
            </div>
          </div>
        </div>
      </Card>

      <!-- Prompt Textareas -->
      <Card padding="md" class="space-y-5">
        <div class="flex items-center justify-between pb-2 border-b border-border-subtle">
          <div class="flex items-center gap-2">
            <span class="material-symbols-outlined text-[17px] text-brand-500">format_quote</span>
            <h2 class="text-sm font-bold text-text-main">Prepended Injections</h2>
          </div>
          <span class="text-[11px] text-text-subtle font-mono">Replaces old system prompts</span>
        </div>

        <!-- 1. System Prompt (Index 0) -->
        <div class="space-y-1.5">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-1.5">
              <span class="w-5 h-5 rounded bg-brand-500/10 text-brand-500 font-mono text-xs font-bold flex items-center justify-center">0</span>
              <label for="sys-prompt" class="text-xs font-semibold text-text-main">System Message (Role: system)</label>
            </div>
            <div class="flex items-center gap-2">
              <span class="text-[10px] text-text-subtle font-mono">{systemPrompt.length} chars</span>
              <button
                type="button"
                onclick={pasteToSystem}
                class="flex items-center gap-1 px-2 py-0.5 rounded bg-surface-2 hover:bg-surface-3 border border-border-subtle text-[11px] text-brand-500 font-semibold transition cursor-pointer"
                title="Paste from clipboard"
              >
                <span class="material-symbols-outlined text-[13px]">content_paste</span>
                <span>Paste</span>
              </button>
            </div>
          </div>
          <textarea
            id="sys-prompt"
            bind:value={systemPrompt}
            oninput={runSimulation}
            rows="6"
            spellcheck="false"
            placeholder="Ketik atau tempel teks System Prompt di sini (disisipkan di index 0)..."
            class="w-full px-3 py-2 rounded-xl bg-surface-2 border border-border-subtle text-xs font-mono text-text-main placeholder:text-text-subtle focus:outline-none focus:border-brand-500 transition-colors leading-relaxed"
          ></textarea>
        </div>

        <!-- 2. Assistant Prompt (Index 1) -->
        <div class="space-y-1.5">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-1.5">
              <span class="w-5 h-5 rounded bg-brand-500/10 text-brand-500 font-mono text-xs font-bold flex items-center justify-center">1</span>
              <label for="asst-prompt" class="text-xs font-semibold text-text-main">Assistant Message (Role: assistant)</label>
            </div>
            <div class="flex items-center gap-2">
              <span class="text-[10px] text-text-subtle font-mono">{assistantPrompt.length} chars</span>
              <button
                type="button"
                onclick={pasteToAssistant}
                class="flex items-center gap-1 px-2 py-0.5 rounded bg-surface-2 hover:bg-surface-3 border border-border-subtle text-[11px] text-brand-500 font-semibold transition cursor-pointer"
                title="Paste from clipboard"
              >
                <span class="material-symbols-outlined text-[13px]">content_paste</span>
                <span>Paste</span>
              </button>
            </div>
          </div>
          <textarea
            id="asst-prompt"
            bind:value={assistantPrompt}
            oninput={runSimulation}
            rows="5"
            spellcheck="false"
            placeholder="Ketik atau tempel teks Assistant Prompt di sini (disisipkan di index 1)..."
            class="w-full px-3 py-2 rounded-xl bg-surface-2 border border-border-subtle text-xs font-mono text-text-main placeholder:text-text-subtle focus:outline-none focus:border-brand-500 transition-colors leading-relaxed"
          ></textarea>
        </div>

        <div class="pt-2 text-[11px] text-text-subtle flex items-center gap-1.5">
          <span class="material-symbols-outlined text-[15px] text-brand-500">info</span>
          <span>
            Nilai ini juga tersinkronisasi otomatis dengan file <code class="font-mono text-text-main">rewrite_system.txt</code> dan <code class="font-mono text-text-main">rewrite_assistant.txt</code>.
          </span>
        </div>
      </Card>
    </div>

    <!-- Right Column: Live Simulator -->
    <div class="space-y-6">
      <Card padding="md" class="space-y-4">
        <div class="flex items-center justify-between pb-2 border-b border-border-subtle">
          <div class="flex items-center gap-2">
            <span class="material-symbols-outlined text-[17px] text-brand-500">science</span>
            <h2 class="text-sm font-bold text-text-main">Live Simulator &amp; Inspection</h2>
          </div>
          <button
            type="button"
            onclick={runSimulation}
            class="flex items-center gap-1 px-2.5 py-1 rounded-lg bg-brand-500/10 hover:bg-brand-500/20 text-brand-500 text-xs font-semibold transition cursor-pointer"
          >
            <span class="material-symbols-outlined text-[14px]">play_arrow</span>
            <span>Simulate</span>
          </button>
        </div>

        <div class="space-y-2">
          <div class="flex justify-between items-center">
            <span class="text-xs font-semibold text-text-main">Incoming Request Payload (Client Input)</span>
            <span class="text-[10px] text-text-subtle font-mono">Simulates client body</span>
          </div>
          <textarea
            bind:value={sampleInput}
            oninput={runSimulation}
            rows="6"
            spellcheck="false"
            class="w-full px-3 py-2 rounded-xl bg-surface-2 border border-border-subtle text-xs font-mono text-text-main placeholder:text-text-subtle focus:outline-none focus:border-brand-500 leading-normal"
          ></textarea>
        </div>

        {#if simulationError}
          <div class="p-2.5 rounded-lg bg-error/10 border border-error/20 text-xs text-error font-mono">
            {simulationError}
          </div>
        {/if}

        <div class="space-y-2 pt-2 border-t border-border-subtle">
          <div class="flex justify-between items-center">
            <div class="flex items-center gap-2">
              <span class="text-xs font-semibold text-text-main">Rewritten Outbound Payload (Sent to Model Provider)</span>
              <span class="px-1.5 py-0.2 rounded text-[10px] font-mono bg-emerald-500/10 text-emerald-500 font-bold">
                Headers Untouched
              </span>
            </div>
            <button
              type="button"
              onclick={() => {
                if (navigator?.clipboard?.writeText && simulatedOutput) {
                  navigator.clipboard.writeText(simulatedOutput)
                }
              }}
              class="text-[11px] text-brand-500 hover:text-brand-400 font-medium transition cursor-pointer"
            >
              Copy JSON
            </button>
          </div>
          <pre class="p-3 rounded-xl bg-surface-3 border border-border-subtle text-xs font-mono text-text-main overflow-x-auto max-h-[360px] leading-relaxed whitespace-pre-wrap select-all">{simulatedOutput}</pre>
        </div>
      </Card>
    </div>
  </div>
</div>

