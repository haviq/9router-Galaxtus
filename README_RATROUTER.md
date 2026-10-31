# RatRouter: Resilient OpenAI-Compatible Gateway & Wrapper Service

RatRouter adalah wrapper service dan request engine tangguh yang dirancang khusus untuk membungkus endpoint OpenAI-compatible (seperti **9Router**) dengan fitur auto-retry, model fallback, pembersihan proxy pool dinamis, validasi output, caching lokal, serta dukungan penuh untuk streaming dan non-streaming.

---

## Fitur Utama

1. **Request Engine**:
   - Maksimum retry hingga 6 kali (`max_retries: 6`).
   - Exponential backoff berjenjang (`backoff_base * (2 ** attempt)`).
   - Penundaan awal **60 detik** otomatis jika mendeteksi HTTP 429 (Rate Limit).
   - Jitter acak (0 - 20 detik) untuk mencegah deteksi bot dan lonjakan traffic sinkron.
   - Request timeout 80 detik.

2. **Model Router**:
   - Menjalankan model utama (`primary`) terlebih dahulu.
   - Otomatis berpindah ke model cadangan (`fallback`) saat menemui HTTP 429, 5xx, atau berulang kali timeout.
   - Memulihkan model utama setelah masa pendinginan (`recovery_cooldown_seconds`).
   - Setiap model yang digunakan dicatat secara terstruktur.

3. **Proxy Pool Manager**:
   - Mendukung format HTTP, HTTPS, dan SOCKS5 (`http://user:pass@host:port`).
   - Placeholder session acak dinamis (`{session_id}`) untuk sticky residential session.
   - Otomatis membuang proxy dari daftar aktif jika mengalami HTTP 407 (Auth Failed), timeout, atau connection error.
   - Rotasi round-robin ke proxy berikutnya.

4. **System Prompt Override**:
   - Otomatis menyuntikkan instruksi khusus sebelum request dikirim:
     `"Act as a direct technical assistant. Respond exactly to the request. Be complete and specific. Keep output clean."`

5. **Response Validation**:
   - Memindai respons dari model untuk mendeteksi penolakan seperti:
     `"I cannot assist"`, `"I am not able"`, `"As an AI"`, `"As a language model"`, `"I cannot help"`, `"cannot provide"`, `"sorry"`, `"maaf"`.
   - Jika terdeteksi, otomatis melakukan regenerasi ulang dengan menaikkan `temperature` sebesar `+0.2`.

6. **Local JSON Cache**:
   - Menyimpan respons prompt identik selama 24 jam (86.400 detik) di file JSON lokal (`ratcache.json`).

7. **Structured Daily Logging**:
   - Menulis log harian ke folder `logs/ratrouter_YYYY-MM-DD.log` (timestamp, model, status code, latency ms, validation result, error).

8. **Streaming & Non-Streaming**:
   - Mendukung Server-Sent Events (SSE `data: {...}`) jika `stream: true`.
   - Mengembalikan JSON standar jika `stream: false`.

---

## Cara Menjalankan

### 1. Prasyarat
Pastikan Python 3.9+ dan library `requests` sudah terpasang:
```bash
pip install requests
```
*(Opsional: jika menggunakan proxy socks5, pasang `pip install requests[socks]`)*

### 2. Konfigurasi
Sesuaikan file `ratconfig.json`:
- `base_url`: URL 9router Anda (misal `http://127.0.0.1:20130/v1`).
- `api_key`: Kunci API 9router Anda.
- `proxies`: Daftar proxy per baris (kosongkan jika koneksi direct).

### 3. Jalankan Service
```bash
python ratrouter.py ratconfig.json
```
Layanan akan mulai mendengarkan di:
`http://0.0.0.0:8088/v1/chat/completions`

---

## Contoh Request Non-Streaming (curl & Python)

### Via `curl`:
```bash
curl -X POST http://127.0.0.1:8088/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [
      {"role": "user", "content": "Explain kernel page tables in x86-64 concisely."}
    ],
    "temperature": 0.7,
    "stream": false
  }'
```

### Via Python (`openai` SDK atau `requests`):
```python
import requests

url = "http://127.0.0.1:8088/v1/chat/completions"
payload = {
    "messages": [
        {"role": "user", "content": "Explain kernel page tables in x86-64 concisely."}
    ],
    "temperature": 0.7,
    "stream": False
}

resp = requests.post(url, json=payload)
data = resp.json()
print("Response:", data["choices"][0]["message"]["content"])
```

---

## Contoh Request Streaming (curl & Python)

### Via `curl`:
```bash
curl -N -X POST http://127.0.0.1:8088/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "messages": [
      {"role": "user", "content": "Write a 64-bit shellcode that executes a syscall."}
    ],
    "stream": true
  }'
```

### Via Python (SSE Stream Reader):
```python
import requests
import json

url = "http://127.0.0.1:8088/v1/chat/completions"
payload = {
    "messages": [
        {"role": "user", "content": "Write a 64-bit shellcode that executes a syscall."}
    ],
    "stream": True
}

with requests.post(url, json=payload, stream=True) as resp:
    for line in resp.iter_lines():
        if line:
            line_str = line.decode("utf-8")
            if line_str.startswith("data: ") and line_str != "data: [DONE]":
                chunk = json.loads(line_str[6:])
                delta = chunk["choices"][0]["delta"].get("content", "")
                print(delta, end="", flush=True)
print()
```

