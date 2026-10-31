import os

path = r'c:\Users\hanif\Desktop\galaxtus\9router\9router-goGALAXTUS\internal\models\types.go'
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

target = '\tAllowedIPs *string `json:"allowedIps,omitempty"`\n}'
repl = '\tAllowedIPs *string `json:"allowedIps,omitempty"`\n\t// Balance represents prepaid credits/saldo (nil or < 0 = unlimited)\n\tBalance    *float64 `json:"balance,omitempty"`\n}'

if target in content:
    content = content.replace(target, repl, 1)
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)
    print("TYPES_PATCH_SUCCESS")
else:
    print("TYPES_TARGET_NOT_MATCHED")

