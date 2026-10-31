<script lang="ts">
  import { onMount } from 'svelte'
  import { Sliders, X, Check, Trash2, Sparkles, RotateCcw } from 'lucide-svelte'
  import { api, type ModelParameters } from '../api/client'

  interface Props {
    isOpen: boolean
    target: string
    targetType?: 'combo' | 'model'
    initialParams?: ModelParameters | null
    onClose: () => void
    onSaved?: (params: ModelParameters | null) => void
  }

  let {
    isOpen = $bindable(false),
    target = '',
    targetType = 'combo',
    initialParams = null,
    onClose,
    onSaved
  }: Props = $props()

  // Form State
  let temperature = $state<number | null>(1.0)
  let topP = $state<number | null>(1.0)
  let topK = $state<number | null>(0)
  let repetitionPenalty = $state<number | null>(1.0)
  let frequencyPenalty = $state<number | null>(0.0)
  let presencePenalty = $state<number | null>(0.0)
  let maxTokens = $state<number | null>(8192)
  let contextLength = $state<number | null>(131072)
  let stream = $state<'on' | 'off' | 'default'>('on')

  let saving = $state(false)
  let deleting = $state(false)
  let errorMessage = $state('')
  let hasExistingOverride = $state(false)

  // Watch for opening or target change
  $effect(() => {
    if (isOpen && target) {
      loadParams()
    }
  })

  async function loadParams() {
    errorMessage = ''
    if (initialParams) {
      applyToForm(initialParams)
      hasExistingOverride = true
      return
    }

    try {
      const data = await api.getModelParameters(target)
      if (data && Object.keys(data).length > 0) {
        applyToForm(data)
        hasExistingOverride = true
      } else {
        resetToDefaults()
        hasExistingOverride = false
      }
    } catch {
      resetToDefaults()
      hasExistingOverride = false
    }
  }

  function applyToForm(p: ModelParameters) {
    temperature = p.temperature !== undefined ? p.temperature : 1.0
    topP = p.topP !== undefined ? p.topP : 1.0
    topK = p.topK !== undefined ? p.topK : 0
    repetitionPenalty = p.repetitionPenalty !== undefined ? p.repetitionPenalty : 1.0
    frequencyPenalty = p.frequencyPenalty !== undefined ? p.frequencyPenalty : 0.0
    presencePenalty = p.presencePenalty !== undefined ? p.presencePenalty : 0.0
    maxTokens = p.maxTokens !== undefined ? p.maxTokens : 8192
    contextLength = p.contextLength !== undefined ? p.contextLength : 131072
    stream = p.stream === 'off' ? 'off' : p.stream === 'default' ? 'default' : 'on'
  }

  function resetToDefaults() {
    temperature = 1.0
    topP = 1.0
    topK = 0
    repetitionPenalty = 1.0
    frequencyPenalty = 0.0
    presencePenalty = 0.0
    maxTokens = 8192
    contextLength = 131072
    stream = 'on'
  }

  function clearAllFields() {
    temperature = null
    topP = null
    topK = null
    repetitionPenalty = null
    frequencyPenalty = null
    presencePenalty = null
    maxTokens = null
    contextLength = null
    stream = 'default'
  }

  async function handleSave() {
    if (!target) return
    saving = true
    errorMessage = ''
    try {
      const payload: ModelParameters = {
        temperature: temperature !== null ? Number(temperature) : null,
        topP: topP !== null ? Number(topP) : null,
        topK: topK !== null ? parseInt(String(topK), 10) : null,
        repetitionPenalty: repetitionPenalty !== null ? Number(repetitionPenalty) : null,
        frequencyPenalty: frequencyPenalty !== null ? Number(frequencyPenalty) : null,
        presencePenalty: presencePenalty !== null ? Number(presencePenalty) : null,
        maxTokens: maxTokens !== null ? parseInt(String(maxTokens), 10) : null,
        contextLength: contextLength !== null ? parseInt(String(contextLength), 10) : null,
        stream: stream !== 'default' ? stream : null,
      }
      await api.setModelParameters(target, payload)
      onSaved?.(payload)
      onClose()
    } catch (err: any) {
      errorMessage = err?.message || 'Failed to save parameters'
    } finally {
      saving = false
    }
  }

  async function handleDelete() {
    if (!target) return
    deleting = true
    errorMessage = ''
    try {
      await api.deleteModelParameters(target)
      onSaved?.(null)
      onClose()
    } catch (err: any) {
      errorMessage = err?.message || 'Failed to delete override'
    } finally {
      deleting = false
    }
  }
</script>

{#if isOpen}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm animate-fade-in">
    <div
      class="w-full max-w-xl rounded-2xl bg-surface-1 border border-border shadow-2xl overflow-hidden flex flex-col max-h-[90vh]"
      role="dialog"
      aria-modal="true"
    >
      <!-- Header -->
      <div class="flex items-center justify-between px-6 py-4 border-b border-border bg-surface-2/50">
        <div class="flex items-center gap-3">
          <div class="size-9 rounded-xl bg-brand-500/10 flex items-center justify-center text-brand-500">
            <Sliders class="w-5 h-5" />
          </div>
          <div>
            <h3 class="text-base font-semibold text-text-main flex items-center gap-2">
              Parameters Override
              <span class="text-xs font-mono font-normal px-2 py-0.5 rounded-full {targetType === 'combo' ? 'bg-purple-500/10 text-purple-400 border border-purple-500/20' : 'bg-blue-500/10 text-blue-400 border border-blue-500/20'}">
                {targetType === 'combo' ? 'Combo' : 'Model'}
              </span>
            </h3>
            <p class="text-xs text-text-muted font-mono truncate max-w-md" title={target}>
              {target}
            </p>
          </div>
        </div>
        <button
          type="button"
          onclick={onClose}
          class="rounded-lg p-1.5 text-text-muted hover:text-text-main hover:bg-black/5 dark:hover:bg-white/5 transition-colors"
        >
          <X class="w-5 h-5" />
        </button>
      </div>

      <!-- Presets & Quick Actions -->
      <div class="flex items-center justify-between px-6 py-2.5 bg-surface-2/30 border-b border-border/50 text-xs">
        <div class="flex items-center gap-2">
          <span class="text-text-muted">Presets:</span>
          <button
            type="button"
            onclick={resetToDefaults}
            class="px-2 py-1 rounded bg-brand-500/10 text-brand-500 hover:bg-brand-500/20 font-medium transition-colors"
          >
            Default (1.0 / 8k / 128k)
          </button>
          <button
            type="button"
            onclick={() => {
              temperature = 0.2
              topP = 0.95
              topK = 0
              repetitionPenalty = 1.0
              frequencyPenalty = 0.0
              presencePenalty = 0.0
              maxTokens = 8192
              contextLength = 131072
              stream = 'on'
            }}
            class="px-2 py-1 rounded bg-black/5 dark:bg-white/5 text-text-muted hover:text-text-main transition-colors"
          >
            Coding (0.2)
          </button>
        </div>
        <button
          type="button"
          onclick={clearAllFields}
          class="flex items-center gap-1 text-text-muted hover:text-text-main transition-colors"
        >
          <RotateCcw class="w-3 h-3" />
          Clear All
        </button>
      </div>

      <!-- Body / Form Fields -->
      <div class="flex-1 overflow-y-auto px-6 py-4 space-y-4 font-sans text-xs">
        {#if errorMessage}
          <div class="p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-red-400 text-xs">
            {errorMessage}
          </div>
        {/if}

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <!-- Temperature -->
          <div class="space-y-1.5 p-3 rounded-xl bg-surface-2/40 border border-border/60">
            <div class="flex items-center justify-between">
              <label for="p-temp" class="font-medium text-text-main">Temperature</label>
              <span class="font-mono text-brand-500 font-semibold">{temperature ?? 'None'}</span>
            </div>
            <div class="flex items-center gap-2">
              <input
                id="p-temp"
                type="range"
                min="0.0"
                max="2.0"
                step="0.05"
                value={temperature ?? 1.0}
                oninput={(e) => temperature = parseFloat(e.currentTarget.value)}
                class="flex-1 accent-brand-500 cursor-pointer"
              />
              <input
                type="number"
                min="0.0"
                max="2.0"
                step="0.05"
                bind:value={temperature}
                class="w-16 px-2 py-1 rounded bg-surface-1 border border-border text-center font-mono"
              />
            </div>
          </div>

          <!-- Top P -->
          <div class="space-y-1.5 p-3 rounded-xl bg-surface-2/40 border border-border/60">
            <div class="flex items-center justify-between">
              <label for="p-topp" class="font-medium text-text-main">Top P</label>
              <span class="font-mono text-brand-500 font-semibold">{topP ?? 'None'}</span>
            </div>
            <div class="flex items-center gap-2">
              <input
                id="p-topp"
                type="range"
                min="0.0"
                max="1.0"
                step="0.01"
                value={topP ?? 1.0}
                oninput={(e) => topP = parseFloat(e.currentTarget.value)}
                class="flex-1 accent-brand-500 cursor-pointer"
              />
              <input
                type="number"
                min="0.0"
                max="1.0"
                step="0.01"
                bind:value={topP}
                class="w-16 px-2 py-1 rounded bg-surface-1 border border-border text-center font-mono"
              />
            </div>
          </div>

          <!-- Top K -->
          <div class="space-y-1.5 p-3 rounded-xl bg-surface-2/40 border border-border/60">
            <div class="flex items-center justify-between">
              <label for="p-topk" class="font-medium text-text-main">Top K</label>
              <span class="font-mono text-text-muted">{topK ?? 'None'}</span>
            </div>
            <input
              id="p-topk"
              type="number"
              min="0"
              max="200"
              placeholder="0"
              bind:value={topK}
              class="w-full px-2.5 py-1.5 rounded-lg bg-surface-1 border border-border font-mono"
            />
          </div>

          <!-- Repetition Penalty -->
          <div class="space-y-1.5 p-3 rounded-xl bg-surface-2/40 border border-border/60">
            <div class="flex items-center justify-between">
              <label for="p-reppen" class="font-medium text-text-main">Repetition Penalty</label>
              <span class="font-mono text-text-muted">{repetitionPenalty ?? 'None'}</span>
            </div>
            <input
              id="p-reppen"
              type="number"
              min="0.0"
              max="2.0"
              step="0.05"
              placeholder="1.0"
              bind:value={repetitionPenalty}
              class="w-full px-2.5 py-1.5 rounded-lg bg-surface-1 border border-border font-mono"
            />
          </div>

          <!-- Frequency Penalty -->
          <div class="space-y-1.5 p-3 rounded-xl bg-surface-2/40 border border-border/60">
            <div class="flex items-center justify-between">
              <label for="p-freqpen" class="font-medium text-text-main">Frequency Penalty</label>
              <span class="font-mono text-text-muted">{frequencyPenalty ?? 'None'}</span>
            </div>
            <input
              id="p-freqpen"
              type="number"
              min="-2.0"
              max="2.0"
              step="0.1"
              placeholder="0"
              bind:value={frequencyPenalty}
              class="w-full px-2.5 py-1.5 rounded-lg bg-surface-1 border border-border font-mono"
            />
          </div>

          <!-- Presence Penalty -->
          <div class="space-y-1.5 p-3 rounded-xl bg-surface-2/40 border border-border/60">
            <div class="flex items-center justify-between">
              <label for="p-prespen" class="font-medium text-text-main">Presence Penalty</label>
              <span class="font-mono text-text-muted">{presencePenalty ?? 'None'}</span>
            </div>
            <input
              id="p-prespen"
              type="number"
              min="-2.0"
              max="2.0"
              step="0.1"
              placeholder="0"
              bind:value={presencePenalty}
              class="w-full px-2.5 py-1.5 rounded-lg bg-surface-1 border border-border font-mono"
            />
          </div>

          <!-- Max Tokens -->
          <div class="space-y-1.5 p-3 rounded-xl bg-surface-2/40 border border-border/60">
            <div class="flex items-center justify-between">
              <label for="p-maxtok" class="font-medium text-text-main">Max Tokens</label>
              <span class="font-mono text-brand-500 font-semibold">{maxTokens ?? 'None'}</span>
            </div>
            <input
              id="p-maxtok"
              type="number"
              min="1"
              max="262144"
              placeholder="8192"
              bind:value={maxTokens}
              class="w-full px-2.5 py-1.5 rounded-lg bg-surface-1 border border-border font-mono"
            />
            <div class="flex items-center gap-1.5 pt-1 text-[10px]">
              <button type="button" onclick={() => maxTokens = 4096} class="px-1.5 py-0.5 rounded bg-surface-1 border border-border hover:border-brand-500">4k</button>
              <button type="button" onclick={() => maxTokens = 8192} class="px-1.5 py-0.5 rounded bg-surface-1 border border-border hover:border-brand-500 font-semibold text-brand-500">8k</button>
              <button type="button" onclick={() => maxTokens = 16384} class="px-1.5 py-0.5 rounded bg-surface-1 border border-border hover:border-brand-500">16k</button>
              <button type="button" onclick={() => maxTokens = 65536} class="px-1.5 py-0.5 rounded bg-surface-1 border border-border hover:border-brand-500">64k</button>
            </div>
          </div>

          <!-- Context Length -->
          <div class="space-y-1.5 p-3 rounded-xl bg-surface-2/40 border border-border/60">
            <div class="flex items-center justify-between">
              <label for="p-ctxlen" class="font-medium text-text-main">Context Length</label>
              <span class="font-mono text-brand-500 font-semibold">
                {contextLength ? (contextLength >= 1024 ? `${Math.round(contextLength / 1024)}k` : contextLength) : 'None'}
              </span>
            </div>
            <input
              id="p-ctxlen"
              type="number"
              min="1024"
              max="2097152"
              placeholder="131072"
              bind:value={contextLength}
              class="w-full px-2.5 py-1.5 rounded-lg bg-surface-1 border border-border font-mono"
            />
            <div class="flex items-center gap-1.5 pt-1 text-[10px]">
              <button type="button" onclick={() => contextLength = 32768} class="px-1.5 py-0.5 rounded bg-surface-1 border border-border hover:border-brand-500">32k</button>
              <button type="button" onclick={() => contextLength = 65536} class="px-1.5 py-0.5 rounded bg-surface-1 border border-border hover:border-brand-500">64k</button>
              <button type="button" onclick={() => contextLength = 131072} class="px-1.5 py-0.5 rounded bg-surface-1 border border-border hover:border-brand-500 font-semibold text-brand-500">128k</button>
            </div>
          </div>
        </div>

        <!-- Stream Mode -->
        <div class="p-3 rounded-xl bg-surface-2/40 border border-border/60 flex items-center justify-between">
          <div>
            <div class="font-medium text-text-main">Stream Output</div>
            <div class="text-[11px] text-text-muted">Enforce SSE token-by-token streaming</div>
          </div>
          <div class="flex items-center gap-1 bg-surface-1 p-1 rounded-lg border border-border">
            <button
              type="button"
              onclick={() => stream = 'on'}
              class="px-2.5 py-1 rounded text-xs font-medium transition-colors {stream === 'on' ? 'bg-brand-500 text-white' : 'text-text-muted hover:text-text-main'}"
            >
              On
            </button>
            <button
              type="button"
              onclick={() => stream = 'off'}
              class="px-2.5 py-1 rounded text-xs font-medium transition-colors {stream === 'off' ? 'bg-red-500 text-white' : 'text-text-muted hover:text-text-main'}"
            >
              Off
            </button>
            <button
              type="button"
              onclick={() => stream = 'default'}
              class="px-2.5 py-1 rounded text-xs font-medium transition-colors {stream === 'default' ? 'bg-surface-3 text-text-main' : 'text-text-muted hover:text-text-main'}"
            >
              Default
            </button>
          </div>
        </div>
      </div>

      <!-- Footer -->
      <div class="flex items-center justify-between px-6 py-4 border-t border-border bg-surface-2/50">
        <div>
          {#if hasExistingOverride}
            <button
              type="button"
              onclick={handleDelete}
              disabled={deleting}
              class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium text-red-400 hover:bg-red-500/10 border border-red-500/20 transition-colors"
            >
              <Trash2 class="w-3.5 h-3.5" />
              {deleting ? 'Removing...' : 'Delete Override'}
            </button>
          {/if}
        </div>
        <div class="flex items-center gap-2">
          <button
            type="button"
            onclick={onClose}
            class="px-4 py-1.5 rounded-lg text-xs font-medium text-text-muted hover:text-text-main hover:bg-black/5 dark:hover:bg-white/5 transition-colors"
          >
            Cancel
          </button>
          <button
            type="button"
            onclick={handleSave}
            disabled={saving}
            class="flex items-center gap-1.5 px-4 py-1.5 rounded-lg text-xs font-medium bg-brand-500 text-white hover:bg-brand-600 transition-colors shadow-sm disabled:opacity-50"
          >
            <Check class="w-3.5 h-3.5" />
            {saving ? 'Saving...' : 'Save Parameters'}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}

