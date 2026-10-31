with open('web/src/components/TopBar.svelte', 'r', encoding='utf-8') as f:
    c = f.read()

target = """    combos: {
      title: 'Combos',
      description: 'Model combos with fallback',
      icon: 'layers',
    },"""

replacement = """    combos: {
      title: 'Combos',
      description: 'Model combos with fallback',
      icon: 'layers',
    },
    remask: {
      title: 'Model Remask',
      description: 'Model remapping and stealth anti-detection cloaking',
      icon: 'theater_comedy',
    },"""

if target in c and 'remask:' not in c:
    c = c.replace(target, replacement, 1)
    with open('web/src/components/TopBar.svelte', 'w', encoding='utf-8', newline='\n') as f:
        f.write(c)
    print("SUCCESS: TopBar.svelte patched")
else:
    print("FAILED or ALREADY PATCHED")

