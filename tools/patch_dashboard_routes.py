path = r'c:\Users\hanif\Desktop\galaxtus\9router\9router-goGALAXTUS\internal\handlers\dashboard\routes.go'
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

target = '''\t\t// Compression Analytics
\t\tr.Get("/analytics/compression", h.HandleGetCompressionAnalytics)
\t})'''

repl = '''\t\t// Compression Analytics
\t\tr.Get("/analytics/compression", h.HandleGetCompressionAnalytics)

\t\t// Feature 1: Billing & Pricing Engine
\t\tr.Get("/billing/pricing", h.HandleListModelPricing)
\t\tr.Post("/billing/pricing", h.HandleSetModelPricing)
\t\tr.Delete("/billing/pricing/{id}", h.HandleDeleteModelPricing)
\t\tr.Post("/billing/topup", h.HandleTopupApiKey)
\t})'''

if target in content:
    content = content.replace(target, repl, 1)
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)
    print("DASHBOARD_ROUTES_PATCH_SUCCESS")
else:
    print("DASHBOARD_ROUTES_TARGET_NOT_FOUND")

