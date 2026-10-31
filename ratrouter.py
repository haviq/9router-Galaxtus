#!/usr/bin/env python3
"""
RatRouter: Resilient OpenAI-Compatible Gateway & Request Engine for 9Router
Features:
- Request engine: 6 retries, exponential backoff, 60s+jitter delay on 429, 80s timeout
- Model router: Primary -> Fallback on 429/5xx/timeouts, auto-recovery on cooldown
- Proxy pool: HTTP/SOCKS5, dynamic random session placeholder, auto-prune on 407/timeout/error
- System prompt override injection
- Response validation: scans for refusal phrases, auto-regenerates with temperature + 0.2
- Local JSON prompt cache (24h TTL)
- Daily structured file logging
- Streaming SSE & non-streaming support
"""

import os
import sys
import time
import json
import uuid
import secrets
import hashlib
import random
import logging
from datetime import datetime, timezone
from typing import Dict, Any, List, Optional, Tuple, Generator
from http.server import HTTPServer, BaseHTTPRequestHandler
import socketserver
import urllib.parse
import urllib.request
import urllib.error

try:
    import requests
except ImportError:
    print("[CRITICAL] 'requests' library is required. Install via: pip install requests")
    sys.exit(1)


# ==============================================================================
# CONFIGURATION LOADER
# ==============================================================================
class ConfigManager:
    def __init__(self, config_path: str = "ratconfig.json"):
        self.config_path = config_path
        self.data: Dict[str, Any] = {}
        self.load()

    def load(self):
        if not os.path.exists(self.config_path):
            raise FileNotFoundError(f"Config file '{self.config_path}' not found.")
        with open(self.config_path, "r", encoding="utf-8") as f:
            self.data = json.load(f)

    def get(self, key: str, default: Any = None) -> Any:
        return self.data.get(key, default)


# ==============================================================================
# STRUCTURED DAILY LOGGER
# ==============================================================================
class DailyLogger:
    def __init__(self, log_dir: str = "logs"):
        self.log_dir = log_dir
        os.makedirs(self.log_dir, exist_ok=True)

    def _get_log_filepath(self) -> str:
        date_str = datetime.now(timezone.utc).strftime("%Y-%m-%d")
        return os.path.join(self.log_dir, f"ratrouter_{date_str}.log")

    def log(self, model: str, status_code: Optional[int], latency_ms: float,
            validation_result: str, error: Optional[str] = None, details: Optional[Dict[str, Any]] = None):
        entry = {
            "timestamp": datetime.now(timezone.utc).isoformat(),
            "model": model,
            "status_code": status_code,
            "latency_ms": round(latency_ms, 2),
            "validation_result": validation_result,
            "error": error,
            "details": details or {}
        }
        log_line = json.dumps(entry) + "\n"

        # Console output
        status_tag = f"[{status_code}]" if status_code else "[ERR]"
        print(f"[{datetime.now().strftime('%H:%M:%S')}] {status_tag} Model: {model} | Latency: {latency_ms:.1f}ms | Valid: {validation_result} {f'| Err: {error}' if error else ''}")

        # File output
        try:
            with open(self._get_log_filepath(), "a", encoding="utf-8") as f:
                f.write(log_line)
        except Exception as e:
            sys.stderr.write(f"Failed to write log file: {e}\n")


# ==============================================================================
# LOCAL JSON CACHE (24h TTL)
# ==============================================================================
class ResponseCache:
    def __init__(self, file_path: str = "ratcache.json", ttl_seconds: int = 86400, enabled: bool = True):
        self.file_path = file_path
        self.ttl_seconds = ttl_seconds
        self.enabled = enabled
        self.cache: Dict[str, Dict[str, Any]] = {}
        self._load()

    def _load(self):
        if not self.enabled or not os.path.exists(self.file_path):
            self.cache = {}
            return
        try:
            with open(self.file_path, "r", encoding="utf-8") as f:
                self.cache = json.load(f)
            self._prune_expired()
        except Exception:
            self.cache = {}

    def _save(self):
        if not self.enabled:
            return
        try:
            with open(self.file_path, "w", encoding="utf-8") as f:
                json.dump(self.cache, f, indent=2)
        except Exception as e:
            sys.stderr.write(f"Failed to persist cache: {e}\n")

    def _prune_expired(self):
        now = time.time()
        keys_to_delete = [k for k, v in self.cache.items() if now - v.get("timestamp", 0) > self.ttl_seconds]
        for k in keys_to_delete:
            del self.cache[k]

    @staticmethod
    def compute_key(model: str, messages: List[Dict[str, Any]], custom_instruction: str) -> str:
        raw = json.dumps({"m": model, "msgs": messages, "inst": custom_instruction}, sort_keys=True)
        return hashlib.sha256(raw.encode("utf-8")).hexdigest()

    def get(self, key: str) -> Optional[Dict[str, Any]]:
        if not self.enabled:
            return None
        item = self.cache.get(key)
        if not item:
            return None
        if time.time() - item.get("timestamp", 0) > self.ttl_seconds:
            del self.cache[key]
            self._save()
            return None
        return item.get("response")

    def set(self, key: str, response: Dict[str, Any]):
        if not self.enabled:
            return
        self.cache[key] = {
            "timestamp": time.time(),
            "response": response
        }
        self._save()


# ==============================================================================
# PROXY POOL MANAGER
# ==============================================================================
class ProxyPoolManager:
    def __init__(self, raw_proxies: List[str], options: Dict[str, Any]):
        self.proxies: List[str] = [p.strip() for p in raw_proxies if p.strip()]
        self.options = options
        self.index = 0
        self.enable_random_session = options.get("enable_random_session", True)
        self.session_placeholder = options.get("session_placeholder", "{session_id}")

    def has_proxies(self) -> bool:
        return len(self.proxies) > 0

    def get_proxy(self) -> Optional[Dict[str, str]]:
        if not self.proxies:
            return None
        raw_proxy = self.proxies[self.index % len(self.proxies)]
        self.index = (self.index + 1) % len(self.proxies)

        resolved_proxy = raw_proxy
        if self.enable_random_session and self.session_placeholder in resolved_proxy:
            rand_session = secrets.token_hex(4)
            resolved_proxy = resolved_proxy.replace(self.session_placeholder, rand_session)

        return {
            "http": resolved_proxy,
            "https": resolved_proxy
        }

    def remove_bad_proxy(self, proxy_str: str, reason: str):
        # Match original raw format ignoring runtime session IDs
        target_to_remove = None
        for p in self.proxies:
            prefix = p.split(self.session_placeholder)[0] if self.session_placeholder in p else p
            if prefix in proxy_str:
                target_to_remove = p
                break
        if target_to_remove and target_to_remove in self.proxies:
            self.proxies.remove(target_to_remove)
            print(f"[PROXY-POOL] Pruned dead proxy: {target_to_remove} | Reason: {reason} | Remaining: {len(self.proxies)}")


# ==============================================================================
# MODEL ROUTER
# ==============================================================================
class ModelRouter:
    def __init__(self, primary: str, fallbacks: Any, recovery_cooldown_seconds: float = 300.0):
        self.primary = primary
        if isinstance(fallbacks, str):
            self.fallbacks = [fallbacks]
        elif isinstance(fallbacks, list):
            self.fallbacks = [str(f) for f in fallbacks if f]
        else:
            self.fallbacks = [primary]
        if not self.fallbacks:
            self.fallbacks = [primary]

        self.cooldown_seconds = recovery_cooldown_seconds
        self.active_fallback_idx = -1  # -1 = using primary
        self.fallback_start_time: float = 0.0
        self.consecutive_failures = 0

    @property
    def using_fallback(self) -> bool:
        return self.active_fallback_idx >= 0

    @property
    def fallback(self) -> str:
        if 0 <= self.active_fallback_idx < len(self.fallbacks):
            return self.fallbacks[self.active_fallback_idx]
        return self.fallbacks[0] if self.fallbacks else self.primary

    def get_active_model(self) -> str:
        now = time.time()
        if self.using_fallback:
            if now - self.fallback_start_time >= self.cooldown_seconds:
                print(f"[MODEL-ROUTER] Recovery cooldown expired ({self.cooldown_seconds}s). Restoring primary: {self.primary}")
                self.active_fallback_idx = -1
                self.consecutive_failures = 0
                return self.primary
            return self.fallbacks[self.active_fallback_idx]
        return self.primary

    def record_failure(self, status_code: Optional[int], is_timeout: bool):
        self.consecutive_failures += 1
        trigger_fallback = False

        if status_code in (400, 404, 429, 500, 502, 503, 504) or is_timeout:
            trigger_fallback = True

        if trigger_fallback:
            next_idx = self.active_fallback_idx + 1
            if next_idx < len(self.fallbacks):
                self.active_fallback_idx = next_idx
                chosen = self.fallbacks[self.active_fallback_idx]
                print(f"[MODEL-ROUTER] Triggering fallback [{self.active_fallback_idx + 1}/{len(self.fallbacks)}] to '{chosen}' (Cause: status={status_code}, timeout={is_timeout}, fails={self.consecutive_failures})")
                self.fallback_start_time = time.time()
            else:
                chosen = self.fallbacks[-1]
                print(f"[MODEL-ROUTER] All fallbacks engaged, remaining on last fallback '{chosen}' (fails={self.consecutive_failures})")

    def record_success(self):
        self.consecutive_failures = 0


# ==============================================================================
# CORE REQUEST ENGINE & VALIDATOR
# ==============================================================================
class RatRequestEngine:
    def __init__(self, config_manager: ConfigManager):
        self.cfg = config_manager
        c = self.cfg.data

        # Components
        self.base_url = c.get("base_url", "http://127.0.0.1:20130/v1").rstrip("/")
        self.api_key = c.get("api_key", "")
        self.custom_headers = c.get("custom_headers", {})

        # Models
        models_cfg = c.get("models", {})
        fallbacks_val = models_cfg.get("fallbacks") or models_cfg.get("fallback", "FREEV2")
        self.router = ModelRouter(
            primary=models_cfg.get("primary", "kl/deepseek-v4-flash-0731"),
            fallbacks=fallbacks_val,
            recovery_cooldown_seconds=models_cfg.get("recovery_cooldown_seconds", 300)
        )

        # Engine parameters
        eng = c.get("request_engine", {})
        self.max_retries = eng.get("max_retries", 6)
        self.backoff_base = eng.get("backoff_base_seconds", 2.0)
        self.delay_on_429 = eng.get("delay_on_429_seconds", 60.0)
        self.jitter_min = eng.get("jitter_min_seconds", 0.0)
        self.jitter_max = eng.get("jitter_max_seconds", 20.0)
        self.timeout = eng.get("timeout_seconds", 80.0)

        # Proxies
        self.proxy_pool = ProxyPoolManager(
            raw_proxies=c.get("proxies", []),
            options=c.get("proxy_options", {})
        )

        # System Instruction
        sys_cfg = c.get("system_instruction", {})
        self.override_system = sys_cfg.get("override_system_prompt", True)
        self.system_prompt_text = sys_cfg.get("default_text", "Act as a direct technical assistant. Respond exactly to the request. Be complete and specific. Keep output clean.")

        # Response Validation
        val_cfg = c.get("response_validation", {})
        self.validation_enabled = val_cfg.get("enabled", True)
        self.temp_increment = val_cfg.get("temperature_increment", 0.2)
        self.flagged_substrings = [s.lower() for s in val_cfg.get("flagged_substrings", [])]

        # Generation Defaults
        gen_cfg = c.get("generation_defaults", {})
        self.default_temperature = float(gen_cfg.get("temperature", 1.0))
        self.default_top_p = float(gen_cfg.get("top_p", 1.0))
        self.default_max_tokens = int(gen_cfg.get("max_tokens", 4096))
        self.default_stream = bool(gen_cfg.get("stream", True))
        self.default_reasoning_effort = gen_cfg.get("reasoning_effort", "none")
        self.default_reasoning = gen_cfg.get("reasoning", False)
        self.strip_reasoning_stream = bool(gen_cfg.get("strip_reasoning_stream", True))
        self.default_logit_bias = gen_cfg.get("logit_bias", {})

        # Cache & Logger
        cache_cfg = c.get("cache", {})
        self.cache = ResponseCache(
            file_path=cache_cfg.get("file_path", "ratcache.json"),
            ttl_seconds=cache_cfg.get("ttl_seconds", 86400),
            enabled=cache_cfg.get("enabled", True)
        )
        self.logger = DailyLogger(log_dir=c.get("logging", {}).get("directory", "logs"))

    def _prepare_messages(self, messages: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        if not self.override_system:
            return messages

        cloned = [dict(m) for m in messages]
        system_idx = next((i for i, m in enumerate(cloned) if m.get("role") == "system"), None)
        if system_idx is not None:
            cloned[system_idx]["content"] = self.system_prompt_text
        else:
            cloned.insert(0, {"role": "system", "content": self.system_prompt_text})
        return cloned

    def _validate_response_text(self, text: str) -> Tuple[bool, Optional[str]]:
        if not self.validation_enabled:
            return True, None
        lowered = text.lower()
        for flag in self.flagged_substrings:
            if flag in lowered:
                return False, flag
        return True, None

    def execute(self, request_payload: Dict[str, Any], is_stream: bool = False) -> Tuple[Optional[Dict[str, Any]], Optional[Generator[bytes, None, None]], Optional[str]]:
        """
        Executes request with retries, model fallback, validation, and proxy rotation.
        Returns: (non_stream_dict, stream_generator, error_string)
        """
        payload = dict(request_payload)
        payload["messages"] = self._prepare_messages(payload.get("messages", []))
        payload["stream"] = is_stream
        if "top_p" not in payload and self.default_top_p is not None:
            payload["top_p"] = self.default_top_p
        if "max_tokens" not in payload and self.default_max_tokens is not None:
            payload["max_tokens"] = self.default_max_tokens
        if "reasoning_effort" not in payload and self.default_reasoning_effort is not None:
            payload["reasoning_effort"] = self.default_reasoning_effort
        if "reasoning" not in payload and self.default_reasoning is not None:
            payload["reasoning"] = self.default_reasoning
        if "logit_bias" not in payload and self.default_logit_bias:
            payload["logit_bias"] = self.default_logit_bias

        # Check cache if non-streaming
        active_model = self.router.get_active_model()
        payload["model"] = active_model
        cache_key = self.cache.compute_key(active_model, payload["messages"], self.system_prompt_text)

        if not is_stream:
            cached_resp = self.cache.get(cache_key)
            if cached_resp:
                self.logger.log(active_model, 200, 0.0, "CACHE_HIT")
                return cached_resp, None, None

        current_temp = float(payload.get("temperature", self.default_temperature))
        target_url = f"{self.base_url}/chat/completions"

        headers = {
            "Content-Type": "application/json",
            "Authorization": f"Bearer {self.api_key}" if self.api_key else ""
        }
        headers.update(self.custom_headers)

        last_error = ""

        for attempt in range(1, self.max_retries + 1):
            active_model = self.router.get_active_model()
            payload["model"] = active_model
            payload["temperature"] = round(current_temp, 2)

            proxy_dict = self.proxy_pool.get_proxy() if self.proxy_pool.has_proxies() else None
            active_proxy_str = proxy_dict["http"] if proxy_dict else "DIRECT"

            start_time = time.time()
            try:
                resp = requests.post(
                    target_url,
                    json=payload,
                    headers=headers,
                    proxies=proxy_dict,
                    timeout=self.timeout,
                    stream=is_stream
                )
                latency_ms = (time.time() - start_time) * 1000

                # Handle HTTP 407 (Proxy Authentication Required)
                if resp.status_code == 407:
                    if proxy_dict:
                        self.proxy_pool.remove_bad_proxy(active_proxy_str, "HTTP 407 Auth Failed")
                    last_error = "Proxy authentication failure (407)"
                    continue

                # Handle HTTP 429 (Rate Limit)
                if resp.status_code == 429:
                    self.router.record_failure(status_code=429, is_timeout=False)
                    jitter = random.uniform(self.jitter_min, self.jitter_max)
                    wait_time = self.delay_on_429 + jitter
                    self.logger.log(active_model, 429, latency_ms, "RATE_LIMIT", f"Sleep {wait_time:.1f}s")
                    time.sleep(wait_time)
                    continue

                # Handle 5xx Upstream Server Errors
                if resp.status_code >= 500:
                    self.router.record_failure(status_code=resp.status_code, is_timeout=False)
                    jitter = random.uniform(self.jitter_min, self.jitter_max)
                    wait_time = (self.backoff_base * (2 ** (attempt - 1))) + jitter
                    self.logger.log(active_model, resp.status_code, latency_ms, "SERVER_ERROR", f"Attempt {attempt}/{self.max_retries}, backoff {wait_time:.1f}s")
                    time.sleep(wait_time)
                    continue

                # Handle HTTP 400 parameter rejection (e.g. reasoning_effort or logit_bias not supported by upstream model)
                if resp.status_code == 400:
                    err_lower = resp.text.lower()
                    retry_needed = False
                    if ("reasoning_effort" in err_lower or "reasoning" in err_lower) and any(k in err_lower for k in ["unrecognized", "extra", "not allowed", "invalid", "unexpected", "not supported"]):
                        print(f"[REASONING-FALLBACK] Upstream rejected reasoning parameter. Stripping reasoning_effort and enforcing via system prompt.")
                        payload.pop("reasoning_effort", None)
                        payload.pop("reasoning", None)
                        retry_needed = True
                    if "logit_bias" in err_lower and any(k in err_lower for k in ["unrecognized", "extra", "not allowed", "invalid", "unexpected", "not supported"]):
                        print(f"[LOGIT-BIAS-FALLBACK] Upstream rejected logit_bias parameter. Stripping logit_bias.")
                        payload.pop("logit_bias", None)
                        retry_needed = True
                    if retry_needed:
                        time.sleep(0.1)
                        continue

                # Handle other non-200 errors (400, 404, 401, etc.)
                if resp.status_code != 200:
                    self.router.record_failure(status_code=resp.status_code, is_timeout=False)
                    err_text = resp.text[:300]
                    self.logger.log(active_model, resp.status_code, latency_ms, "UPSTREAM_ERROR", err_text)
                    last_error = f"HTTP {resp.status_code}: {err_text}"
                    if resp.status_code in (400, 404):
                        time.sleep(0.2)
                    else:
                        jitter = random.uniform(self.jitter_min, self.jitter_max)
                        time.sleep((self.backoff_base * (2 ** (attempt - 1))) + jitter)
                    continue

                # ==========================
                # SUCCESS 200 OK HANDLING
                # ==========================
                self.router.record_success()

                if is_stream:
                    # Stream generator
                    strip_reasoning = self.strip_reasoning_stream
                    def stream_generator() -> Generator[bytes, None, None]:
                        try:
                            for chunk in resp.iter_lines():
                                if not chunk:
                                    continue
                                if chunk.strip() == b"data: [DONE]":
                                    yield chunk + b"\n\n"
                                    break

                                if strip_reasoning and chunk.startswith(b"data: "):
                                    try:
                                        chunk_json = json.loads(chunk[6:].decode("utf-8-sig"))
                                        delta = chunk_json.get("choices", [{}])[0].get("delta", {})
                                        if "reasoning_content" in delta:
                                            delta.pop("reasoning_content", None)
                                            # If chunk has neither content nor finish_reason, skip emitting reasoning blob
                                            if not delta.get("content") and not chunk_json.get("choices", [{}])[0].get("finish_reason"):
                                                continue
                                            chunk = b"data: " + json.dumps(chunk_json).encode("utf-8")
                                    except Exception:
                                        pass

                                yield chunk + b"\n\n"
                        finally:
                            resp.close()

                    self.logger.log(active_model, 200, latency_ms, "STREAM_PASSED")
                    return None, stream_generator(), None

                # Non-streaming JSON validation
                data = resp.json()
                if self.strip_reasoning_stream:
                    try:
                        if "choices" in data and len(data["choices"]) > 0:
                            data["choices"][0].get("message", {}).pop("reasoning_content", None)
                    except Exception:
                        pass
                content = ""
                try:
                    content = data["choices"][0]["message"]["content"] or ""
                except (KeyError, IndexError, TypeError):
                    pass

                # Response validation check
                is_valid, triggered_flag = self._validate_response_text(content)
                if not is_valid:
                    current_temp = min(2.0, current_temp + self.temp_increment)
                    self.logger.log(active_model, 200, latency_ms, "VALIDATION_TRIGGERED",
                                    f"Flag: '{triggered_flag}'. Incrementing temp to {current_temp:.2f}")
                    # Re-attempt generation with higher temperature
                    continue

                # Response is valid
                self.logger.log(active_model, 200, latency_ms, "VALID_OK")
                self.cache.set(cache_key, data)
                return data, None, None

            except (requests.exceptions.Timeout, requests.exceptions.ConnectionError) as e:
                latency_ms = (time.time() - start_time) * 1000
                is_timeout = isinstance(e, requests.exceptions.Timeout)
                last_error = f"Network error: {str(e)}"

                if proxy_dict:
                    self.proxy_pool.remove_bad_proxy(active_proxy_str, "Timeout" if is_timeout else "Connection Error")

                self.router.record_failure(status_code=None, is_timeout=is_timeout)
                jitter = random.uniform(self.jitter_min, self.jitter_max)
                wait_time = (self.backoff_base * (2 ** (attempt - 1))) + jitter
                self.logger.log(active_model, None, latency_ms, "NETWORK_FAIL", f"{last_error} (sleep {wait_time:.1f}s)")
                time.sleep(wait_time)

            except Exception as e:
                last_error = f"Unhandled error: {str(e)}"
                self.logger.log(active_model, None, 0.0, "FATAL", last_error)
                break

        return None, None, f"Max retries ({self.max_retries}) exhausted. Last error: {last_error}"


# ==============================================================================
# OPENAI-COMPATIBLE HTTP SERVER
# ==============================================================================
class OpenAIHandler(BaseHTTPRequestHandler):
    engine: RatRequestEngine

    def do_GET(self):
        parsed = urllib.parse.urlparse(self.path)
        if parsed.path in ("/v1/models", "/models"):
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            active = self.engine.router.get_active_model()
            model_items = [{"id": self.engine.router.primary, "object": "model", "owned_by": "ratrouter", "type": "primary"}]
            for fb in self.engine.router.fallbacks:
                model_items.append({"id": fb, "object": "model", "owned_by": "ratrouter", "type": "fallback"})
            models = {
                "object": "list",
                "data": model_items
            }
            self.wfile.write(json.dumps(models).encode("utf-8"))
            return

        if parsed.path in ("/health", "/healthz"):
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            status = {
                "status": "healthy",
                "active_model": self.engine.router.get_active_model(),
                "primary_model": self.engine.router.primary,
                "fallback_active": self.engine.router.using_fallback,
                "active_fallback_index": self.engine.router.active_fallback_idx,
                "fallbacks": self.engine.router.fallbacks,
                "proxy_pool_size": len(self.engine.proxy_pool.proxies)
            }
            self.wfile.write(json.dumps(status).encode("utf-8"))
            return

        self.send_error(404, "Not Found")

    def do_POST(self):
        parsed = urllib.parse.urlparse(self.path)
        if parsed.path not in ("/v1/chat/completions", "/chat/completions"):
            self.send_error(404, "Unknown endpoint. Use /v1/chat/completions")
            return

        content_length = int(self.headers.get("Content-Length", 0))
        if content_length == 0:
            self.send_error(400, "Empty request body")
            return

        body = self.rfile.read(content_length)
        try:
            req_json = json.loads(body.decode("utf-8-sig"))
        except Exception as e:
            self.send_error(400, f"Malformed JSON: {e}")
            return

        is_stream = bool(req_json.get("stream", self.engine.default_stream))
        data, stream_gen, err = self.engine.execute(req_json, is_stream=is_stream)

        if err:
            self.send_response(502)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            err_resp = {"error": {"message": err, "type": "ratrouter_engine_error"}}
            self.wfile.write(json.dumps(err_resp).encode("utf-8"))
            return

        if is_stream and stream_gen:
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.send_header("Connection", "close")
            self.end_headers()
            try:
                for chunk in stream_gen:
                    self.wfile.write(chunk)
                    self.wfile.flush()
            except (BrokenPipeError, ConnectionResetError):
                pass
            self.close_connection = True
            return

        # Non-streaming JSON output
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps(data).encode("utf-8"))


class ThreadedHTTPServer(socketserver.ThreadingMixIn, HTTPServer):
    daemon_threads = True


# ==============================================================================
# MAIN ENTRYPOINT
# ==============================================================================
def main():
    config_file = sys.argv[1] if len(sys.argv) > 1 else "ratconfig.json"
    print(f"[RATROUTER] Loading configuration from '{config_file}'...")
    cfg = ConfigManager(config_file)
    engine = RatRequestEngine(cfg)

    server_cfg = cfg.get("server", {})
    host = server_cfg.get("host", "0.0.0.0")
    port = int(server_cfg.get("port", 8088))

    OpenAIHandler.engine = engine
    httpd = ThreadedHTTPServer((host, port), OpenAIHandler)
    print(f"""
======================================================================
  RATROUTER: High-Resilience OpenAI-Compatible Gateway Active
======================================================================
  Listening on      : http://{host}:{port}/v1/chat/completions
  Upstream Target   : {engine.base_url}
  Primary Model     : {engine.router.primary}
  Fallback Model    : {engine.router.fallback}
  Proxy Pool Count  : {len(engine.proxy_pool.proxies)}
  Cache File        : {engine.cache.file_path} (TTL: {engine.cache.ttl_seconds}s)
  Logging Dir       : {engine.logger.log_dir}
======================================================================
""")
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print("\n[RATROUTER] Shutting down cleanly.")
        httpd.server_close()


if __name__ == "__main__":
    main()

