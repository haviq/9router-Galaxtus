path = r'c:\Users\hanif\Desktop\galaxtus\9router\9router-goGALAXTUS\web\src\components\keys\ApiKeysTable.svelte'
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

target = "    if ((k.expiresAt ?? '') !== '') {"
repl = """    if (k.balance !== undefined && k.balance !== null) {
      if (k.balance < 0) {
        badges.push({ label: 'Saldo: ∞', tone: 'info' })
      } else {
        badges.push({
          label: `Rp ${k.balance.toLocaleString('id-ID')}`,
          tone: k.balance <= 0 ? 'danger' : 'info',
        })
      }
    }
    if ((k.expiresAt ?? '') !== '') {"""

if target in content:
    content = content.replace(target, repl, 1)
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)
    print("APIKEYS_TABLE_PATCH_SUCCESS")
else:
    print("APIKEYS_TABLE_TARGET_NOT_FOUND")

