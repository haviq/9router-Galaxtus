import sys

path = 'internal/proxy/executor/deepseek_web_endpoints.go'
with open(path, 'r', encoding='utf-8') as f:
    c = f.read()

target = """	if base, ok := strings.CutSuffix(completion, suffix); ok {
		root = base
	}
	return deepSeekEndpoints{"""

replacement = """	if base, ok := strings.CutSuffix(completion, suffix); ok {
		root = base
	}
	if !strings.Contains(root, "/api/v0") {
		root = strings.TrimRight(root, "/") + "/api/v0"
		completion = root + suffix
	}
	return deepSeekEndpoints{"""

if target in c:
    c = c.replace(target, replacement, 1)
    with open(path, 'w', encoding='utf-8', newline='\n') as f:
        f.write(c)
    print("SUCCESS: deepseek_web_endpoints.go patched")
else:
    print("FAILED: target not found")

