with open('web/src/App.svelte', 'r', encoding='utf-8') as f:
    c = f.read()

import_target = "import PlaygroundView from './components/PlaygroundView.svelte'"
import_replace = "import PlaygroundView from './components/PlaygroundView.svelte'\n  import ModelRemaskView from './components/remask/ModelRemaskView.svelte'"

if import_target in c and 'ModelRemaskView' not in c:
    c = c.replace(import_target, import_replace, 1)

render_target = """            {:else if activeTab === 'combos'}
              <CombosView />"""

render_replace = """            {:else if activeTab === 'combos'}
              <CombosView />
            {:else if activeTab === 'remask'}
              <ModelRemaskView />"""

if render_target in c and "activeTab === 'remask'" not in c:
    c = c.replace(render_target, render_replace, 1)

with open('web/src/App.svelte', 'w', encoding='utf-8', newline='\n') as f:
    f.write(c)

print("SUCCESS: web/src/App.svelte patched")

