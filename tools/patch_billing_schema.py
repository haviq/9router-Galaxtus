import os

path = r'c:\Users\hanif\Desktop\galaxtus\9router\9router-goGALAXTUS\internal\db\schema.go'
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

target1 = '\t{"apiKeys", "allowedIPs", "TEXT DEFAULT \'\'"},'
repl1 = '\t{"apiKeys", "allowedIPs", "TEXT DEFAULT \'\'"},\n\t// Feature 1: Prepaid Billing Saldo (-1 = unlimited, >= 0 = balance in currency)\n\t{"apiKeys", "balance", "REAL DEFAULT -1"},'

target2 = '\t{\n\t\tname: "guardrail_logs",'
model_pricing_table = '''\t{
\t\tname: "model_pricing",
\t\tcolumns: [][2]string{
\t\t\t{"id", "TEXT PRIMARY KEY"},
\t\t\t{"model_pattern", "TEXT NOT NULL UNIQUE"},
\t\t\t{"price_input_1k", "REAL NOT NULL DEFAULT 0"},
\t\t\t{"price_output_1k", "REAL NOT NULL DEFAULT 0"},
\t\t\t{"currency", "TEXT NOT NULL DEFAULT 'IDR'"},
\t\t\t{"created_at", "TEXT NOT NULL"},
\t\t\t{"updated_at", "TEXT NOT NULL"},
\t\t},
\t\tindexes: []string{
\t\t\t"CREATE INDEX IF NOT EXISTS idx_mp_pattern ON model_pricing(model_pattern)",
\t\t},
\t},
\t{
\t\tname: "guardrail_logs",'''

if target1 in content and target2 in content:
    content = content.replace(target1, repl1, 1)
    content = content.replace(target2, model_pricing_table, 1)
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)
    print("SCHEMA_PATCH_SUCCESS")
else:
    print("TARGETS NOT MATCHED")

