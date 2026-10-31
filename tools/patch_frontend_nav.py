# Patch client.ts
with open('web/src/api/client.ts', 'r', encoding='utf-8') as f:
    c = f.read()

model_remask_interface = '''export interface ModelRemaskRule {
  alias: string
  target: string
  enabled: boolean
  cloakIdentity?: boolean
  stripThinking?: boolean
  contextWindow?: number
  maxOutput?: number
  customPrompt?: string
}

'''

target_interface = 'export interface ModelParameters {'
if target_interface in c and 'export interface ModelRemaskRule' not in c:
    c = c.replace(target_interface, model_remask_interface + target_interface, 1)

api_methods = '''  // Model Remasking
  getModelRemaskRules: () => request<ModelRemaskRule[]>('/api/models/remask'),
  setModelRemaskRule: (rule: ModelRemaskRule) =>
    request<{ success: boolean; rule: ModelRemaskRule }>('/api/models/remask', {
      method: 'POST',
      body: JSON.stringify(rule),
    }),
  deleteModelRemaskRule: (alias: string) =>
    request<{ success: boolean }>(`/api/models/remask?alias=${encodeURIComponent(alias)}`, {
      method: 'DELETE',
    }),

'''

target_methods = '  // Settings\n  getSettings:'
if target_methods in c and 'getModelRemaskRules:' not in c:
    c = c.replace(target_methods, api_methods + target_methods, 1)

with open('web/src/api/client.ts', 'w', encoding='utf-8', newline='\n') as f:
    f.write(c)
print("SUCCESS: web/src/api/client.ts patched")

# Patch router.ts
with open('web/src/lib/router.ts', 'r', encoding='utf-8') as f:
    r = f.read()

target_active_tab = "  | 'keys' // legacy alias for the endpoint tab; see ROUTE_TO_TAB below"
replacement_active_tab = "  | 'keys' // legacy alias for the endpoint tab; see ROUTE_TO_TAB below\n  | 'remask'"

if target_active_tab in r and "'remask'" not in r:
    r = r.replace(target_active_tab, replacement_active_tab, 1)

target_tab_routes = "  keys: '/dashboard/keys',"
replacement_tab_routes = "  keys: '/dashboard/keys',\n  remask: '/dashboard/remask',"

if target_tab_routes in r and "remask: '/dashboard/remask'" not in r:
    r = r.replace(target_tab_routes, replacement_tab_routes, 1)

target_route_to_tab = "  // keys / API key management\n  '/dashboard/keys': 'keys',"
replacement_route_to_tab = """  // remask / model remasking
  '/dashboard/remask': 'remask',
  '/dashboard/model-remask': 'remask',
  '/remask': 'remask',
  '/model-remask': 'remask',

  // keys / API key management
  '/dashboard/keys': 'keys',"""

if target_route_to_tab in r and "'/dashboard/remask': 'remask'" not in r:
    r = r.replace(target_route_to_tab, replacement_route_to_tab, 1)

with open('web/src/lib/router.ts', 'w', encoding='utf-8', newline='\n') as f:
    f.write(r)
print("SUCCESS: web/src/lib/router.ts patched")

# Patch nav.ts
with open('web/src/lib/nav.ts', 'r', encoding='utf-8') as f:
    n = f.read()

target_nav = "{ kind: 'link', id: 'combos', label: 'Combo & Vision Adapter', icon: 'layers' },"
replacement_nav = """{ kind: 'link', id: 'combos', label: 'Combo & Vision Adapter', icon: 'layers' },
      { kind: 'link', id: 'remask', label: 'Model Remask', icon: 'theater_comedy' },"""

if target_nav in n and "id: 'remask'" not in n:
    n = n.replace(target_nav, replacement_nav, 1)

with open('web/src/lib/nav.ts', 'w', encoding='utf-8', newline='\n') as f:
    f.write(n)
print("SUCCESS: web/src/lib/nav.ts patched")

