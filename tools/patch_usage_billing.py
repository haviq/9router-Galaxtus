path = r'c:\Users\hanif\Desktop\galaxtus\9router\9router-goGALAXTUS\internal\handlers\chat\usage.go'
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

target = '''\tclientKey := ""
\tif caller := middleware.AuthenticatedApiKeyFromContext(ctx); caller != nil {
\t\tclientKey = maskAPIKey(caller.Key)
\t}'''

repl = '''\tclientKey := ""
\tif caller := middleware.AuthenticatedApiKeyFromContext(ctx); caller != nil {
\t\tclientKey = maskAPIKey(caller.Key)
\t\t// Feature 1: Deduct prepaid saldo/balance if model pricing is configured
\t\tif caller.ID != "" && caller.ID != "dashboard-session" && caller.ID != "dashboard" {
\t\t\tif pricing, err := h.Repo.GetModelPricing(info.Model); err == nil && pricing != nil {
\t\t\t\tinputCost := (float64(usage.PromptTokens) / 1000.0) * pricing.PriceInput1k
\t\t\t\toutputCost := (float64(usage.CompletionTokens) / 1000.0) * pricing.PriceOutput1k
\t\t\t\ttotalCharge := inputCost + outputCost
\t\t\t\tif totalCharge > 0 {
\t\t\t\t\trem, err := h.Repo.DeductApiKeyBalance(caller.ID, totalCharge)
\t\t\t\t\tif err == nil {
\t\t\t\t\t\tlog.Info("billing", "deducted saldo", "apiKeyId", caller.ID, "charge", totalCharge, "remaining", rem, "currency", pricing.Currency)
\t\t\t\t\t}
\t\t\t\t}
\t\t\t}
\t\t}
\t}'''

if target in content:
    content = content.replace(target, repl, 1)
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)
    print("USAGE_BILLING_PATCH_SUCCESS")
else:
    print("USAGE_TARGET_NOT_FOUND")

