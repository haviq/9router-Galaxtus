with open('web/src/App.svelte', 'r', encoding='utf-8') as f:
    c = f.read()

target = """            {:else if activeTab === 'combos'}
              <CombosView {combos} {connections} {providerNodes} onRefresh={loadData} bind:isCreatingOpen={isCreateComboOpen} />"""

replacement = """            {:else if activeTab === 'combos'}
              <CombosView {combos} {connections} {providerNodes} onRefresh={loadData} bind:isCreatingOpen={isCreateComboOpen} />
            {:else if activeTab === 'remask'}
              <ModelRemaskView />"""

if target in c:
    c = c.replace(target, replacement, 1)
    with open('web/src/App.svelte', 'w', encoding='utf-8', newline='\n') as f:
        f.write(c)
    print("SUCCESS: ModelRemaskView rendered in App.svelte")
else:
    print("FAILED: target not found")

