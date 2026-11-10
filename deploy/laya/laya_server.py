#!/usr/bin/env python3
"""Serve Laya as a decision endpoint for System 1 Arcade.

This is the built-in agent: the app starts it and POSTs each prompt to it.

    POST /predict  {"state": ..., "questions": {...}}             -> {"answers": {name: answer}}
    POST /predict  {"batch": {key: {"state": ..., "questions": ...}}} -> {"answers": {"key.name": answer}}
    GET  /health   -> {"ok": true}
    GET  /stats    -> answer cache hits, misses and hit rate, and request counts

Answers are cached by (state, question), since Laya gives the same answer to
the same prompt; only misses reach the model. --cache-size 0 (or
SYSTEM1_LAYA_CACHE=0) turns the cache off.

--key (or SYSTEM1_LAYA_KEY) requires "Authorization: Bearer <key>" on
/predict and /stats; /health stays open for monitoring.

The model answers one request at a time. --max-queue (or
SYSTEM1_LAYA_MAX_QUEUE, default 4) bounds how many may wait for it; past that
/predict answers 503 busy at once, and the caller plays without the model
that turn. A request carrying X-Deadline-Ms (Unix milliseconds) that is
still waiting when its deadline passes is dropped with 503 rather than
answered for nobody.

--weights (or SYSTEM1_LAYA_WEIGHTS) applies a head tuned by agents/finetune.py
on top of the base model; without it the base model answers.

A custom agent implements the same POST contract with its own logic.
"""
import argparse
import hmac
import json
import os
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

sys.path.insert(0, __file__.rsplit("/", 1)[0])


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--host", default="127.0.0.1", help="address to listen on (0.0.0.0 to serve other machines)")
    p.add_argument("--port", type=int, default=0, help="port to listen on (0 = any free port)")
    p.add_argument("--model", help="base checkpoint (default: $SYSTEM1_LAYA_MODEL or convaiinnovations/laya)")
    p.add_argument("--weights", help="tuned weights from agents/finetune.py (default: $SYSTEM1_LAYA_WEIGHTS, else none)")
    p.add_argument("--device", help="mps, cuda or cpu (default: auto)")
    p.add_argument("--cache-size", type=int, help="answers to cache, 0 = off (default: $SYSTEM1_LAYA_CACHE or 10000)")
    p.add_argument("--stats-every", type=float, default=0, help="print cache stats every N seconds (0 = never)")
    p.add_argument("--max-queue", type=int, help="requests that may wait for the model; more get 503 busy (default: $SYSTEM1_LAYA_MAX_QUEUE or 4)")
    p.add_argument("--key", help="require this Bearer token on /predict and /stats (default: $SYSTEM1_LAYA_KEY, else none)")
    args = p.parse_args()
    key = args.key or os.environ.get("SYSTEM1_LAYA_KEY", "").strip()
    want = f"Bearer {key}".encode()

    def authorized(headers):
        return not key or hmac.compare_digest(headers.get("Authorization", "").encode(), want)

    from laya_batch import AnswerCache, answer_cached, cache_size_from_env, load_model, predict_many

    model = load_model(args.model, args.weights, device=args.device)
    cache = AnswerCache(cache_size_from_env() if args.cache_size is None else args.cache_size)
    lock = threading.Lock()
    max_queue = args.max_queue if args.max_queue is not None else int(os.environ.get("SYSTEM1_LAYA_MAX_QUEUE", "4"))
    # Request counts, guarded by counts_lock (not the model lock, so /stats
    # answers while the model is busy).
    counts_lock = threading.Lock()
    counts = {"answered": 0, "busy": 0, "expired": 0, "unauthorized": 0, "bad_request": 0, "waiting": 0, "max_waiting": 0}

    def count(name, n=1):
        with counts_lock:
            counts[name] += n
            if name == "waiting":
                counts["max_waiting"] = max(counts["max_waiting"], counts["waiting"])

    def expired(headers):
        try:
            return time.time() * 1000 > float(headers.get("X-Deadline-Ms", "inf"))
        except ValueError:
            return False

    def single(prompts):
        return {k: model.predict(p["state"], p["questions"])["answers"] for k, p in prompts.items()}

    class Handler(BaseHTTPRequestHandler):
        def reply(self, code, body):
            data = json.dumps(body).encode()
            self.send_response(code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def do_GET(self):
            if self.path == "/health":
                return self.reply(200, {"ok": True})
            if self.path == "/stats":
                if not authorized(self.headers):
                    count("unauthorized")
                    return self.reply(401, {"error": "unauthorized"})
                with counts_lock:
                    requests = dict(counts, max_queue=max_queue)
                return self.reply(200, {**cache.stats(), "requests": requests})
            self.reply(404, {"error": "not found"})

        def do_POST(self):
            if self.path != "/predict":
                return self.reply(404, {"error": "POST /predict"})
            if not authorized(self.headers):
                count("unauthorized")
                return self.reply(401, {"error": "unauthorized"})
            try:
                body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))))
            except Exception as e:  # report bad requests instead of dropping the connection
                count("bad_request")
                return self.reply(400, {"error": str(e)})
            with counts_lock:
                full = counts["waiting"] >= max_queue
            if full:
                count("busy")
                return self.reply(503, {"error": "busy"})
            count("waiting")
            try:
                with lock:
                    if expired(self.headers):
                        count("expired")
                        return self.reply(503, {"error": "expired"})
                    if "batch" in body:
                        grouped = answer_cached(body["batch"], lambda ps: predict_many(model, ps), cache)
                        answers = {f"{k}.{q}": a for k, qs in grouped.items() for q, a in qs.items()}
                    else:
                        answers = answer_cached({"": body}, single, cache)[""]
            except Exception as e:
                count("bad_request")
                return self.reply(400, {"error": str(e)})
            finally:
                count("waiting", -1)
            count("answered")
            self.reply(200, {"answers": answers})

        def log_message(self, *a):
            pass

    server = ThreadingHTTPServer((args.host, args.port), Handler)
    if args.stats_every > 0:
        def report():
            while True:
                time.sleep(args.stats_every)
                with counts_lock:
                    c = dict(counts)
                print(cache.summary(), f"answered {c['answered']}, busy {c['busy']}, expired {c['expired']}, unauthorized {c['unauthorized']}, most waiting {c['max_waiting']}", flush=True)
        threading.Thread(target=report, daemon=True).start()
    print(f"agent ready http://{args.host}:{server.server_port}/predict" + (" (key required)" if key else ""), flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
