# Patch routes.go
with open('internal/handlers/dashboard/routes.go', 'r', encoding='utf-8') as f:
    c = f.read()

target = '''		r.Get("/models/health", h.HandleGetModelHealth)
		r.Put("/models/health", h.HandleSaveModelHealth)'''

replacement = '''		r.Get("/models/health", h.HandleGetModelHealth)
		r.Put("/models/health", h.HandleSaveModelHealth)
		r.Get("/models/remask", h.HandleGetModelRemaskRules)
		r.Post("/models/remask", h.HandleSetModelRemaskRule)
		r.Put("/models/remask", h.HandleSetModelRemaskRule)
		r.Delete("/models/remask", h.HandleDeleteModelRemaskRule)
		r.Delete("/models/remask/{alias}", h.HandleDeleteModelRemaskRule)'''

if target in c:
    c = c.replace(target, replacement, 1)
    with open('internal/handlers/dashboard/routes.go', 'w', encoding='utf-8', newline='\n') as f:
        f.write(c)
    print("SUCCESS: routes.go patched")
else:
    print("FAILED: routes.go target not found")

# Patch router.go
with open('internal/handlers/router.go', 'r', encoding='utf-8') as f:
    c2 = f.read()

target2 = '''	r.Put("/api/models/alias", dashH.HandleSetModelAlias)
	r.Delete("/api/models/alias", dashH.HandleDeleteModelAlias)'''

replacement2 = '''	r.Put("/api/models/alias", dashH.HandleSetModelAlias)
	r.Delete("/api/models/alias", dashH.HandleDeleteModelAlias)
	r.Get("/api/models/remask", dashH.HandleGetModelRemaskRules)
	r.Post("/api/models/remask", dashH.HandleSetModelRemaskRule)
	r.Put("/api/models/remask", dashH.HandleSetModelRemaskRule)
	r.Delete("/api/models/remask", dashH.HandleDeleteModelRemaskRule)
	r.Delete("/api/models/remask/{alias}", dashH.HandleDeleteModelRemaskRule)'''

if target2 in c2:
    c2 = c2.replace(target2, replacement2, 1)
    with open('internal/handlers/router.go', 'w', encoding='utf-8', newline='\n') as f:
        f.write(c2)
    print("SUCCESS: router.go patched")
else:
    print("FAILED: router.go target not found")

