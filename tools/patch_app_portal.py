path = r'c:\Users\hanif\Desktop\galaxtus\9router\9router-goGALAXTUS\web\src\App.svelte'
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

target1 = "  import ModelRemaskView from './components/remask/ModelRemaskView.svelte'\n"
repl1 = "  import ModelRemaskView from './components/remask/ModelRemaskView.svelte'\n  import ClientPortalView from './components/portal/ClientPortalView.svelte'\n"

target2 = "{:else if (requireLogin && !isAuthenticatedState) || activeTab === 'login'}\n  <LoginView"
repl2 = "{:else if activeTab === 'portal'}\n  <ClientPortalView />\n{:else if (requireLogin && !isAuthenticatedState) || activeTab === 'login'}\n  <LoginView"

if target1 in content and target2 in content:
    content = content.replace(target1, repl1, 1)
    content = content.replace(target2, repl2, 1)
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)
    print("APP_PORTAL_PATCH_SUCCESS")
else:
    print("APP_PORTAL_TARGET_NOT_FOUND")

