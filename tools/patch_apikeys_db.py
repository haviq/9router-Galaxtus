path = r'c:\Users\hanif\Desktop\galaxtus\9router\9router-goGALAXTUS\internal\db\apikeys.go'
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

# 1. Update apiKeyColumns
target1 = '\tcreatedBy, lastUsedIP, allowedIPs`'
repl1 = '\tcreatedBy, lastUsedIP, allowedIPs, balance`'

# 2. Update apiKeyScan
target2 = 'var createdBy, lastUsedIP, allowedIPs sql.NullString'
repl2 = 'var createdBy, lastUsedIP, allowedIPs sql.NullString\n\tvar balance sql.NullFloat64'

target3 = '&createdBy, &lastUsedIP, &allowedIPs}'
repl3 = '&createdBy, &lastUsedIP, &allowedIPs, &balance}'

target4 = 'createdBy, lastUsedIP, allowedIPs)\n\t}'
repl4 = 'createdBy, lastUsedIP, allowedIPs, balance)\n\t}'

# 3. Update applyApiKeyNulls signature and body
target5 = 'createdBy, lastUsedIP, allowedIPs sql.NullString) {'
repl5 = 'createdBy, lastUsedIP, allowedIPs sql.NullString,\n\tbalance sql.NullFloat64) {'

target6 = 'if allowedIPs.Valid {\n\t\tk.AllowedIPs = &allowedIPs.String\n\t}\n}'
repl6 = 'if allowedIPs.Valid {\n\t\tk.AllowedIPs = &allowedIPs.String\n\t}\n\tif balance.Valid {\n\t\tv := balance.Float64\n\t\tk.Balance = &v\n\t}\n}'

for t, r in [(target1, repl1), (target2, repl2), (target3, repl3), (target4, repl4), (target5, repl5), (target6, repl6)]:
    if t not in content:
        print(f"FAILED TO FIND TARGET: {t[:30]}...")
        exit(1)
    content = content.replace(t, r, 1)

with open(path, 'w', encoding='utf-8') as f:
    f.write(content)
print("APIKEYS_DB_PATCH_SUCCESS")

