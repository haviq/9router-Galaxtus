path = r'c:\Users\hanif\Desktop\galaxtus\9router\9router-goGALAXTUS\internal\handlers\chat\access.go'
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

target = '''func (h *ChatHandler) enforceModelAccess(w http.ResponseWriter, r *http.Request, modelID string) bool {
\tif modelID == "" {
\t\treturn true
\t}
\terr := h.checkModelAccess(requestKey(r), modelID)'''

repl = '''func (h *ChatHandler) enforceModelAccess(w http.ResponseWriter, r *http.Request, modelID string) bool {
\tif modelID == "" {
\t\treturn true
\t}
\tkey := requestKey(r)
\tif key != nil && key.ID != "" && key.ID != "dashboard-session" && key.ID != "dashboard" {
\t\tif key.Balance != nil && *key.Balance == 0 {
\t\t\thandlerutil.WriteJSON(w, http.StatusPaymentRequired, map[string]any{
\t\t\t\t"error": map[string]string{
\t\t\t\t\t"message": "Insufficient balance (saldo habis). Please top up your balance.",
\t\t\t\t\t"type":    "insufficient_balance",
\t\t\t\t\t"code":    "balance_exhausted",
\t\t\t\t},
\t\t\t})
\t\t\treturn false
\t\t}
\t}
\terr := h.checkModelAccess(key, modelID)'''

if target in content:
    content = content.replace(target, repl, 1)
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)
    print("ACCESS_BALANCE_PATCH_SUCCESS")
else:
    print("FAILED TO MATCH TARGET IN ACCESS.GO")

