import time
from collections import Counter
from concurrent.futures import ThreadPoolExecutor, as_completed
from typing import Any

import requests

BASE = "http://127.0.0.1:44318"
TOTAL = 420
WORKERS = 28


def send(i: int) -> tuple[str, int]:
    m = i % 6
    ns = int(time.time() * 1_000_000_000) + i
    trace = f"{i:032x}"[-32:]
    span = f"{i:016x}"[-16:]
    parent = f"{(i - 1):016x}"[-16:]

    if m == 0:
        payload_logs: dict[str, Any] = {
            "resourceLogs": [
                {
                    "resource": {
                        "attributes": [{"key": "service.name", "value": {"stringValue": f"load-svc-{i % 5}"}}]
                    },
                    "scopeLogs": [
                        {
                            "logRecords": [
                                {
                                    "timeUnixNano": str(ns),
                                    "severityText": "INFO",
                                    "body": {"stringValue": f"concurrent log {i}"},
                                    "traceId": trace,
                                    "spanId": span,
                                }
                            ]
                        }
                    ],
                }
            ]
        }
        r = requests.post(f"{BASE}/v1/logs", json=payload_logs, timeout=8)
        return ("logs", r.status_code)

    if m == 1:
        payload_traces: dict[str, Any] = {
            "resourceSpans": [
                {
                    "resource": {
                        "attributes": [{"key": "service.name", "value": {"stringValue": f"trace-svc-{i % 4}"}}]
                    },
                    "scopeSpans": [
                        {
                            "spans": [
                                {
                                    "traceId": trace,
                                    "spanId": span,
                                    "parentSpanId": parent,
                                    "name": f"load-span-{i}",
                                    "startTimeUnixNano": str(ns),
                                    "endTimeUnixNano": str(ns + 25_000_000),
                                    "status": {"code": 1},
                                    "attributes": [
                                        {"key": "http.method", "value": {"stringValue": "GET"}},
                                        {"key": "http.url", "value": {"stringValue": f"/load/{i}"}},
                                    ],
                                }
                            ]
                        }
                    ],
                }
            ]
        }
        r = requests.post(f"{BASE}/v1/traces", json=payload_traces, timeout=8)
        return ("traces", r.status_code)

    if m == 2:
        payload_error: dict[str, Any] = {
            "service": f"err-svc-{i % 3}",
            "type": "RuntimeError",
            "message": f"simulated error {i}",
            "stack": f"Traceback line {i}",
        }
        r = requests.post(f"{BASE}/v1/errors", json=payload_error, timeout=8)
        return ("errors", r.status_code)

    if m == 3:
        payload_rum: list[dict[str, str]] = [
            {
                "type": "pageview",
                "timestamp": "2026-03-28T12:00:00Z",
                "sessionId": f"sess-{i % 60}",
                "url": f"https://example.test/page/{i}",
                "title": f"Load Page {i}",
            }
        ]
        r = requests.post(f"{BASE}/v1/rum", json=payload_rum, timeout=8)
        return ("rum", r.status_code)

    if m == 4:
        payload_ai: dict[str, Any] = {
            "service": f"ai-svc-{i % 3}",
            "provider": "openai",
            "model": "gpt-4o-mini",
            "prompt": f"Prompt {i}",
            "response": f"Response {i}",
            "tokens_in": 10 + (i % 7),
            "tokens_out": 3 + (i % 5),
            "duration_ms": 90 + (i % 40),
            "trace_id": trace,
            "span_id": span,
        }
        r = requests.post(f"{BASE}/v1/ai", json=payload_ai, timeout=8)
        return ("ai", r.status_code)

    payload_metrics: dict[str, Any] = {
        "resourceMetrics": [
            {
                "resource": {"attributes": [{"key": "service.name", "value": {"stringValue": f"metric-svc-{i % 2}"}}]},
                "scopeMetrics": [{"metrics": [{"name": "requests_total"}]}],
            }
        ]
    }
    r = requests.post(f"{BASE}/v1/metrics", json=payload_metrics, timeout=8)
    return ("metrics", r.status_code)


if __name__ == "__main__":
    start = time.time()
    endpoint_counts: Counter[str] = Counter()
    status_counts: Counter[int] = Counter()
    errors: list[str] = []

    with ThreadPoolExecutor(max_workers=WORKERS) as ex:
        futures = [ex.submit(send, i) for i in range(1, TOTAL + 1)]
        for fut in as_completed(futures):
            try:
                endpoint, status = fut.result()
                endpoint_counts[endpoint] += 1
                status_counts[status] += 1
            except Exception as exc:
                errors.append(str(exc))

    print("elapsed_sec", round(time.time() - start, 2))
    print("endpoint_counts", dict(sorted(endpoint_counts.items())))
    print("status_counts", dict(sorted(status_counts.items())))
    print("errors", len(errors))
    if errors:
        print("sample_error", errors[0])
