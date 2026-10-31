path = r'c:\Users\hanif\Desktop\galaxtus\9router\9router-goGALAXTUS\internal\handlers\router.go'
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

target1 = '''\t// Compression Analytics
\tr.Get("/api/analytics/compression", dashH.HandleGetCompressionAnalytics)'''

repl1 = '''\t// Compression Analytics
\tr.Get("/api/analytics/compression", dashH.HandleGetCompressionAnalytics)

\t// Feature 1: Billing & Pricing Engine
\tr.Get("/api/billing/pricing", dashH.HandleListModelPricing)
\tr.Post("/api/billing/pricing", dashH.HandleSetModelPricing)
\tr.Delete("/api/billing/pricing/{id}", dashH.HandleDeleteModelPricing)
\tr.Post("/api/billing/topup", dashH.HandleTopupApiKey)

\t// Feature 2: Client Self-Service Portal
\tr.Get("/api/client/portal-info", dashH.HandleGetClientPortalInfo)
\tr.Options("/api/client/portal-info", func(w http.ResponseWriter, r *http.Request) {
\t\tw.Header().Set("Access-Control-Allow-Origin", "*")
\t\tw.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
\t\tw.Header().Set("Access-Control-Allow-Headers", "*")
\t\tw.WriteHeader(http.StatusOK)
\t})'''

target2 = '''\tr.Get("/login", webH.ServeHTTP)'''
repl2 = '''\tr.Get("/login", webH.ServeHTTP)
\tr.Get("/portal", webH.ServeHTTP)'''

if target1 in content and target2 in content:
    content = content.replace(target1, repl1, 1)
    content = content.replace(target2, repl2, 1)
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)
    print("ROUTER_BILLING_PATCH_SUCCESS")
else:
    print("ROUTER_TARGET_NOT_FOUND")

