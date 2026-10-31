with open('internal/db/aliases.go', 'r', encoding='utf-8') as f:
    c = f.read()

target = """	_, err = r.db.Exec(`
		INSERT INTO kv (scope, key, value, updatedAt)
		VALUES ('modelAliases', ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(scope, key) DO UPDATE SET
			value = excluded.value,
			updatedAt = CURRENT_TIMESTAMP
	`, rule.Alias, string(b))"""

replacement = """	_, err = r.db.Exec(`
		INSERT INTO kv (scope, key, value)
		VALUES ('modelAliases', ?, ?)
		ON CONFLICT(scope, key) DO UPDATE SET
			value = excluded.value
	`, rule.Alias, string(b))"""

if target in c:
    c = c.replace(target, replacement, 1)
    with open('internal/db/aliases.go', 'w', encoding='utf-8', newline='\n') as f:
        f.write(c)
    print("SUCCESS: internal/db/aliases.go patched")
else:
    print("FAILED: target not found")

