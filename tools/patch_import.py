with open('internal/handlers/chat/model_remask.go', 'r', encoding='utf-8') as f:
    c = f.read()

target = '''import (
	"bytes"
	"context"'''

replacement = '''import (
	"bytes"
	"context"
	"net/http"'''

c = c.replace(target, replacement, 1)

with open('internal/handlers/chat/model_remask.go', 'w', encoding='utf-8', newline='\n') as f:
    f.write(c)

print("SUCCESS")

