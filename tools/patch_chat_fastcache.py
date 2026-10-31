path = r'c:\Users\hanif\Desktop\galaxtus\9router\9router-goGALAXTUS\internal\handlers\chat\chat.go'
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

target = '''\t// Check prompt/response cache for non-streaming requests before model'''
repl = '''\t// Feature 4: Fast Exact Prompt Cache (<2ms TTFT, zero upstream cost)
\tfastHash := semanticcache.GlobalFastCache.ComputeHash(reqBody.Model, body)
\tif cachedItem, hit := semanticcache.GlobalFastCache.Get(fastHash); hit {
\t\tsemanticcache.GlobalFastCache.ServeCachedResponse(w, cachedItem, reqBody.Stream)
\t\tlog.Info("chat", "fast prompt cache hit", "model", reqBody.Model, "stream", reqBody.Stream)
\t\treturn
\t}
\tif !reqBody.Stream {
\t\tfastCapture := &cacheCaptureWriter{ResponseWriter: w}
\t\tw = fastCapture
\t\tdefer func() {
\t\t\tif fastCapture.statusCode == http.StatusOK && fastCapture.body.Len() > 0 {
\t\t\t\tsemanticcache.GlobalFastCache.Put(fastHash, reqBody.Model, "", fastCapture.body.Bytes(), 0, 0)
\t\t\t}
\t\t}()
\t}

\t// Check prompt/response cache for non-streaming requests before model'''

if target in content:
    content = content.replace(target, repl, 1)
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)
    print("CHAT_FASTCACHE_PATCH_SUCCESS")
else:
    print("CHAT_TARGET_NOT_FOUND")

