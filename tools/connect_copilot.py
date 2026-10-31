import urllib.request
import urllib.parse
import json
import time
import os
import sys
import sqlite3
import hashlib
import webbrowser

CLIENT_ID = "Iv1.b507a08c87ecfe98"

def short_hash(s: str) -> str:
    return hashlib.sha256(s.encode("utf-8")).hexdigest()[:8]

def main():
    print("=" * 60)
    print(" 9ROUTER - GITHUB COPILOT DIRECT AUTHENTICATOR")
    print("=" * 60)
    
    # 1. Request device code
    print("\n[*] Menghubungi GitHub Device Flow API...")
    data = urllib.parse.urlencode({"client_id": CLIENT_ID, "scope": "read:user"}).encode()
    req = urllib.request.Request(
        "https://github.com/login/device/code",
        data=data,
        headers={"Accept": "application/json", "User-Agent": "GitHubCopilotChat/0.26.7"}
    )
    
    try:
        with urllib.request.urlopen(req) as resp:
            dev_res = json.loads(resp.read().decode())
    except Exception as e:
        print(f"[!] Gagal meminta device code dari GitHub: {e}")
        return

    device_code = dev_res.get("device_code")
    user_code = dev_res.get("user_code")
    verification_uri = dev_res.get("verification_uri", "https://github.com/login/device")
    interval = dev_res.get("interval", 5)

    print("\n" + "#" * 50)
    print(f"  KODE ANDA:  {user_code}")
    print(f"  BUKA URL :  {verification_uri}")
    print("#" * 50 + "\n")
    print(f"[*] Membuka browser ke {verification_uri}...")
    try:
        webbrowser.open(verification_uri)
    except Exception:
        pass

    print(f"[*] Menunggu otorisasi di browser (polling setiap {interval}s)...")
    
    access_token = None
    refresh_token = ""
    expires_in = 0
    
    while True:
        time.sleep(interval)
        poll_data = urllib.parse.urlencode({
            "client_id": CLIENT_ID,
            "device_code": device_code,
            "grant_type": "urn:ietf:params:oauth:grant-type:device_code"
        }).encode()
        
        poll_req = urllib.request.Request(
            "https://github.com/login/oauth/access_token",
            data=poll_data,
            headers={"Accept": "application/json", "User-Agent": "GitHubCopilotChat/0.26.7"}
        )
        
        try:
            with urllib.request.urlopen(poll_req) as resp:
                poll_res = json.loads(resp.read().decode())
        except Exception as e:
            print(f"[!] Error saat polling: {e}")
            continue

        if "error" in poll_res:
            err = poll_res["error"]
            if err == "authorization_pending":
                print(".", end="", flush=True)
                continue
            elif err == "slow_down":
                print("s", end="", flush=True)
                time.sleep(5)
                continue
            else:
                print(f"\n[!] GitHub Error: {err} - {poll_res.get('error_description')}")
                return
        
        if "access_token" in poll_res:
            access_token = poll_res["access_token"]
            refresh_token = poll_res.get("refresh_token", "")
            expires_in = poll_res.get("expires_in", 0)
            print(f"\n[+] Sukses! Access token GitHub diperoleh: {access_token[:10]}...")
            break

    # 2. Get user info
    print("[*] Mengambil informasi akun GitHub...")
    gh_headers = {
        "Authorization": f"Bearer {access_token}",
        "X-GitHub-Api-Version": "2022-11-28",
        "User-Agent": "GitHubCopilotChat/0.26.7",
        "Accept": "application/json"
    }
    
    user_info = {}
    try:
        user_req = urllib.request.Request("https://api.github.com/user", headers=gh_headers)
        with urllib.request.urlopen(user_req) as resp:
            user_info = json.loads(resp.read().decode())
            print(f"[+] Login sebagai: {user_info.get('login')} ({user_info.get('name')})")
    except Exception as e:
        print(f"[!] Gagal mengambil user info: {e}")

    # 3. Get Copilot internal token
    print("[*] Mengambil Copilot internal token...")
    copilot_info = {}
    try:
        cop_req = urllib.request.Request("https://api.github.com/copilot_internal/v2/token", headers=gh_headers)
        with urllib.request.urlopen(cop_req) as resp:
            copilot_info = json.loads(resp.read().decode())
            print(f"[+] Token Copilot aktif! Expires at: {copilot_info.get('expires_at')}")
    except urllib.error.HTTPError as e:
        err_body = e.read().decode()
        print(f"[!] Warning HTTP {e.code} saat mengambil Copilot token: {err_body}")
    except Exception as e:
        print(f"[!] Warning saat mengambil Copilot token: {e}")

    # 4. Save to 9router SQLite database
    db_path = os.path.expandvars(r"%APPDATA%\9router\db\data.sqlite")
    print(f"[*] Menyimpan koneksi ke database 9Router: {db_path}...")
    
    if not os.path.exists(db_path):
        print(f"[!] Database tidak ditemukan di {db_path}")
        return

    conn = sqlite3.connect(db_path)
    c = conn.cursor()
    
    conn_id = f"github-{short_hash(access_token)}"
    name = user_info.get("name") or user_info.get("login") or "GitHub Copilot"
    email = user_info.get("email") or ""
    
    data_map = {
        "apiKey": access_token,
        "accessToken": access_token,
        "providerSpecificData": {
            "authMethod": "device",
            "copilotToken": copilot_info.get("token", ""),
            "copilotTokenExpiresAt": copilot_info.get("expires_at", ""),
            "githubUserId": user_info.get("id"),
            "githubLogin": user_info.get("login", ""),
            "githubName": user_info.get("name", ""),
            "githubEmail": email
        }
    }
    if refresh_token:
        data_map["refreshToken"] = refresh_token
    if email:
        data_map["email"] = email
        
    now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    raw_data = json.dumps(data_map)
    
    # Check if exists
    c.execute("SELECT id FROM providerConnections WHERE id = ?", (conn_id,))
    if c.fetchone():
        c.execute(
            "UPDATE providerConnections SET name = ?, data = ?, isActive = 1, updatedAt = ? WHERE id = ?",
            (name, raw_data, now, conn_id)
        )
        print(f"[+] Koneksi {conn_id} berhasil diperbarui di database!")
    else:
        c.execute(
            "INSERT INTO providerConnections (id, provider, authType, name, priority, isActive, data, createdAt, updatedAt) VALUES (?, 'github', 'oauth', ?, 1, 1, ?, ?, ?)",
            (conn_id, name, raw_data, now, now)
        )
        print(f"[+] Koneksi baru {conn_id} berhasil ditambahkan ke database!")
        
    conn.commit()
    conn.close()

    print("\n" + "=" * 60)
    print(" [SUCCESS] GITHUB COPILOT BERHASIL TERSAMBUNG KE 9ROUTER!")
    print(" Silakan refresh dashboard 9Router (tekan F5 di browser).")
    print("=" * 60)

if __name__ == "__main__":
    main()

