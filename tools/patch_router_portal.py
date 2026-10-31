path = r'c:\Users\hanif\Desktop\galaxtus\9router\9router-goGALAXTUS\web\src\lib\router.ts'
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

target1 = "  | 'remask'\n"
repl1 = "  | 'remask'\n  | 'portal'\n"

target2 = "  remask: '/dashboard/remask',\n}"
repl2 = "  remask: '/dashboard/remask',\n  portal: '/portal',\n}"

target3 = "  // remask / model remasking"
repl3 = "  // portal\n  '/portal': 'portal',\n\n  // remask / model remasking"

target4 = "  if (normalized.includes('login')) return 'login'"
repl4 = "  if (normalized.includes('portal')) return 'portal'\n  if (normalized.includes('login')) return 'login'"

for t, r in [(target1, repl1), (target2, repl2), (target3, repl3), (target4, repl4)]:
    if t not in content:
        print(f"FAILED TO MATCH: {t[:30]}...")
        exit(1)
    content = content.replace(t, r, 1)

with open(path, 'w', encoding='utf-8') as f:
    f.write(content)
print("ROUTER_PORTAL_PATCH_SUCCESS")

