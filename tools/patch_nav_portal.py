path = r'c:\Users\hanif\Desktop\galaxtus\9router\9router-goGALAXTUS\web\src\lib\nav.ts'
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

target = "      { kind: 'link', id: 'remask', label: 'Model Remask', icon: 'theater_comedy' },"
repl = "      { kind: 'link', id: 'remask', label: 'Model Remask', icon: 'theater_comedy' },\n      { kind: 'link', id: 'portal', label: 'Client Portal', icon: 'storefront' },"

if target in content:
    content = content.replace(target, repl, 1)
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)
    print("NAV_PATCH_SUCCESS")
else:
    print("NAV_TARGET_NOT_FOUND")

