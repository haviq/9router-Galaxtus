path = r'c:\Users\hanif\Desktop\galaxtus\9router\9router-goGALAXTUS\web\src\components\keys\ApiKeyPolicyModal.svelte'
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

target1 = "  let tokenQuota = $state(apiKey.tokenQuota ?? 0)"
repl1 = "  let tokenQuota = $state(apiKey.tokenQuota ?? 0)\n  let balance = $state(apiKey.balance ?? -1)"

target2 = "      await api.updateApiKeyPolicy(apiKey.id, policy)\n      await api.setApiKeyModels(apiKey.id, models)"
repl2 = """      await api.updateApiKeyPolicy(apiKey.id, policy)
      await api.setApiKeyModels(apiKey.id, models)
      if (balance !== (apiKey.balance ?? -1)) {
        await fetch('/api/billing/topup', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ apiKeyId: apiKey.id, balance: Number(balance) }),
        }).catch(() => null)
      }"""

target3 = """            <div class="space-y-1">
              <label for="quota-input" class="text-[11px] text-text-muted">Lifetime Token Quota (0 = unlimited)</label>"""

repl3 = """            <div class="space-y-1 sm:col-span-2">
              <label for="balance-input" class="text-[11px] font-semibold text-emerald-400">Prepaid Saldo (Rp / IDR, -1 = unlimited)</label>
              <input
                id="balance-input"
                type="number"
                bind:value={balance}
                placeholder="-1"
                class="w-full bg-surface border border-emerald-500/40 rounded-lg px-3 py-1.5 text-xs text-text-main font-mono focus:outline-none focus:border-emerald-500"
              />
            </div>

            <div class="space-y-1">
              <label for="quota-input" class="text-[11px] text-text-muted">Lifetime Token Quota (0 = unlimited)</label>"""

for t, r in [(target1, repl1), (target2, repl2), (target3, repl3)]:
    if t not in content:
        print(f"FAILED TO MATCH: {t[:30]}...")
        exit(1)
    content = content.replace(t, r, 1)

with open(path, 'w', encoding='utf-8') as f:
    f.write(content)
print("POLICY_MODAL_PATCH_SUCCESS")

