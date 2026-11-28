"""在独立 Docker 网络运行真实 Envoy/Redis，所有 Key 都是合成夹具。

不用宿主机 bind mount，避免 Windows/WSL 路径歧义。仅清理本次成功创建的容器和网络。
"""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import socket
import time
import urllib.request
import urllib.error
import uuid


def docker(*args, check=True, input=None):
    r = subprocess.run(["docker", *args], input=input, capture_output=True, text=True,
                       encoding="utf-8", errors="replace", timeout=90)
    if check and r.returncode:
        raise RuntimeError(f"docker {args[:3]} exit={r.returncode}: {r.stderr[-2500:]}")
    return (r.stdout + (r.stderr if args and args[0] == "logs" else "")).strip()


def cluster(name, host, port):
    return {"name": name, "connect_timeout": "2s", "type": "STRICT_DNS",
            "dns_lookup_family": "V4_ONLY", "lb_policy": "ROUND_ROBIN",
            "load_assignment": {"cluster_name": name, "endpoints": [{"lb_endpoints": [
                {"endpoint": {"address": {"socket_address": {
                    "address": host, "port_value": port}}}}]}]}}


def envoy_config(overlay, redis_delay_ms=0):
    policy = {"@type": "type.googleapis.com/envoy.extensions.http.custom_response.redirect_policy.v3.RedirectPolicy",
              "use_original_request_uri": True, "use_original_request_body": True,
              "keep_original_response_code": False, "only_redirect_upstream_code": True,
              "max_internal_redirects": 10, "request_headers_to_add": [{"header": {
                  "key": "x-higress-fallback-from", "value": "ai-proxy-key-affinity"},
                  "append_action": "OVERWRITE_IF_EXISTS_OR_ADD"}]}
    predicates = []
    for name, typ, extra, value in [
        ("retry_status", "HttpResponseStatusCodeMatchInput", {}, "299"),
        ("retry_header", "HttpResponseHeaderMatchInput", {"header_name": "x-higress-ai-proxy-key-retry"}, "1")]:
        predicates.append({"single_predicate": {"input": {"name": name, "typed_config": {
            "@type": "type.googleapis.com/envoy.type.matcher.v3." + typ, **extra}},
            "value_match": {"exact": value}}})
    custom_response = {"name": "envoy.filters.http.custom_response", "typed_config": {
        "@type": "type.googleapis.com/envoy.extensions.filters.http.custom_response.v3.CustomResponse",
        "custom_response_matcher": {"matcher_list": {"matchers": [{"predicate": {
            "and_matcher": {"predicate": predicates}}, "on_match": {"action": {
                "name": "affinity_retry", "typed_config": policy}}}]}}}}
    provider = {"id": "verification", "type": "openai", "openaiCustomUrl": "http://mock:8080/v1",
                "apiTokens": ["synthetic-A", "synthetic-B", "synthetic-C"],
                "failover": {"enabled": True, "failureThreshold": 1, "cooldownDuration": 60000},
                "apiKeyAffinity": {"enabled": True, "redis": {
                    "serviceName": "redis.static", "servicePort": 80, "timeout": 1000},
                    "bindingTTLDays": 7, "maxRetries": 3, "retryTimeout": 10000}}
    wasm = {"name": "envoy.filters.http.wasm", "typed_config": {
        "@type": "type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm", "config": {
            "name": "ai-proxy", "root_id": "ai-proxy",
            "configuration": {"@type": "type.googleapis.com/google.protobuf.StringValue",
                              "value": json.dumps({"activeProviderId": "verification", "providers": [provider]})},
            "vm_config": {"vm_id": "verification", "runtime": "envoy.wasm.runtime.v8",
                          "code": {"local": {"filename": "/tmp/plugin.wasm"}}}}}}
    hcm = {"@type": "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager",
           "stat_prefix": "verify",
           "route_config": {"name": "verify", "internal_only_headers": ["x-higress-fallback-from", "x-hi-original-auth"],
               "virtual_hosts": [{"name": "verify", "domains": ["*"],
               "routes": [{"match": {"prefix": "/"}, "route": {"cluster": "mock", "timeout": "30s"}}]}]},
           "http_filters": [*([custom_response] if overlay else []), wasm, {
               "name": "envoy.filters.http.router", "typed_config": {
                   "@type": "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"}}],
           "access_log": [{"name": "envoy.access_loggers.stdout", "typed_config": {
               "@type": "type.googleapis.com/envoy.extensions.access_loggers.stream.v3.StdoutAccessLog"}}]}
    return {"admin": {"address": {"socket_address": {"address": "0.0.0.0", "port_value": 9901}}},
            "static_resources": {"listeners": [{"name": "verify", "address": {"socket_address": {
                "address": "0.0.0.0", "port_value": 10000}}, "filter_chains": [{"filters": [{
                    "name": "envoy.filters.network.http_connection_manager", "typed_config": hcm}]}]}],
                "clusters": [cluster("mock", "mock", 8080),
                             cluster("outbound|80||redis.static", "mock" if redis_delay_ms else "redis",
                                     6380 if redis_delay_ms else 6379)]}}


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--wasm", required=True, type=Path)
    p.add_argument("--gateway-image", required=True)
    p.add_argument("--redis-image", default="redis:7.4.10-alpine")
    p.add_argument("--redis-delay-ms", type=int, default=0)
    p.add_argument("--output", required=True, type=Path)
    p.add_argument("--expect-affinity", action="store_true")
    a = p.parse_args()
    a.output.mkdir(parents=True, exist_ok=False)
    if "@sha256:" not in a.gateway_image:
        raise ValueError("gateway image must be pinned by digest")
    ident = "higress226-verify-" + uuid.uuid4().hex[:10]
    names = {n: ident + "-" + n for n in ("mock", "redis", "envoy")}
    created = []
    network_created = False
    results = {"wasm_sha256": hashlib.sha256(a.wasm.read_bytes()).hexdigest(),
               "gateway_image": a.gateway_image, "expected_affinity": a.expect_affinity,
               "redis_delay_ms": a.redis_delay_ms}
    conf = a.output / "envoy.json"
    conf.write_text(json.dumps(envoy_config(True, a.redis_delay_ms), indent=2), encoding="utf-8")
    try:
        docker("network", "create", ident)
        network_created = True
        for role, image, command in [
            ("redis", a.redis_image, ["redis-server", "--save", "", "--appendonly", "no"]),
            ("mock", "python:3.12-slim", ["python", "-u", "/tmp/mock_upstream.py"]),
            ("envoy", a.gateway_image, ["--concurrency", "1", "-c", "/tmp/envoy.json", "-l", "info"])]:
            entry = ["--entrypoint", "/usr/local/bin/envoy"] if role == "envoy" else []
            env = ["-e", "REDIS_DELAY_MS=" + str(a.redis_delay_ms)] if role == "mock" else []
            ports = {"redis": [6379], "mock": [8080], "envoy": [9901, 10000]}[role]
            publish = [v for port in ports for v in ("-p", "127.0.0.1::" + str(port))]
            docker("create", "--name", names[role], "--network", ident, "--network-alias", role, *publish,
                   "--memory", "768m" if role == "envoy" else "128m",
                   "--cpus", "0.5" if role == "envoy" else "0.25", *env, *entry, image, *command)
            created.append(names[role])
            results[role + "_image_id"] = json.loads(docker("inspect", names[role]))[0]["Image"]
            if role == "mock":
                docker("cp", str(Path(__file__).with_name("mock_upstream.py")), names[role] + ":/tmp/mock_upstream.py")
            if role == "envoy":
                docker("cp", str(a.wasm.resolve()), names[role] + ":/tmp/plugin.wasm")
                docker("cp", str(conf.resolve()), names[role] + ":/tmp/envoy.json")
            docker("start", names[role])
        addresses = {}
        for role in names:
            published = json.loads(docker("inspect", names[role]))[0]["NetworkSettings"]["Ports"]
            for key, value in published.items():
                addresses[role, int(key.split("/")[0])] = int(value[0]["HostPort"])

        def http(role, port, path, data=None, headers=None):
            url = f"http://127.0.0.1:{addresses[role, port]}{path}"
            req = urllib.request.Request(url, data=data, headers=headers or {})
            try:
                with urllib.request.urlopen(req, timeout=25) as resp:
                    return resp.status, resp.read().decode()
            except urllib.error.HTTPError as err:
                return err.code, err.read().decode()

        def redis(*args):
            with socket.create_connection(("127.0.0.1", addresses["redis", 6379]), timeout=5) as sock:
                encoded = [str(x).encode() for x in args]
                sock.sendall(b"*" + str(len(args)).encode() + b"\r\n" + b"".join(
                    b"$" + str(len(x)).encode() + b"\r\n" + x + b"\r\n" for x in encoded))
                reader = sock.makefile("rb")

                def read():
                    line = reader.readline()
                    typ, value = line[:1], line[1:-2]
                    if typ == b"*":
                        return [read() for _ in range(int(value))]
                    if typ == b"$":
                        length = int(value)
                        if length < 0:
                            return None
                        raw = reader.read(length)
                        assert reader.read(2) == b"\r\n"
                        return raw.decode()
                    if typ == b":":
                        return int(value)
                    if typ == b"+":
                        return value.decode()
                    raise RuntimeError("Redis reply: " + repr(line))

                return read()
        ready = False
        for _ in range(45):
            state = json.loads(docker("inspect", names["envoy"]))[0]["State"]
            if state["Status"] != "running":
                raise RuntimeError("Envoy exited: " + docker("logs", names["envoy"], check=False)[-4000:])
            try:
                code, text = http("envoy", 9901, "/ready")
                if code == 200 and text.strip() == "LIVE":
                    ready = True
                    break
            except (OSError, urllib.error.URLError):
                pass
            time.sleep(1)
        assert ready, "Envoy readiness timeout"

        def mock_request(path, data=None):
            code, text = http("mock", 8080, path, json.dumps(data).encode() if data else None,
                              {"Content-Type": "application/json"})
            assert code == 200, (code, text)
            return json.loads(text)

        def request(stream=False):
            body = json.dumps({"model": "test-model", "messages": [{"role": "user", "content": "验证重放" * 1024}],
                               "stream": stream}, ensure_ascii=False)
            code, content = http("envoy", 10000, "/v1/chat/completions", body.encode(),
                                 {"Content-Type": "application/json", "x-mse-consumer": "synthetic-consumer"})
            assert code == 200, (code, content[:500])
            return content if stream else json.loads(content)["choices"][0]["message"]["content"]

        selected = request()
        results["first_selected"] = selected
        redis_keys = []
        for _ in range(30):
            redis_keys = redis("KEYS", "higress:ai-proxy:api-key-affinity:v1:*")
            if redis_keys:
                break
            time.sleep(.1)
        if not a.expect_affinity:
            assert not redis_keys, "baseline unexpectedly implements affinity"
            results.update(baseline_missing_affinity=True, passed=True)
            return
        assert len(redis_keys) == 1, ("binding missing", redis_keys)
        results["first_binding"] = json.loads(redis("GET", redis_keys[0]))
        results["stable_selected"] = []
        for _ in range(5):
            current = request()
            results["stable_selected"].append(current)
            assert current == selected, "consumer changed API Key"
        ttl = redis("TTL", redis_keys[0])
        assert ttl > 6 * 86400, ttl
        before = len(mock_request("/events"))
        mock_request("/control/fail", {"key": selected})
        replacement = request()
        assert replacement != selected, "failed Key was not replaced"
        events = mock_request("/events")[before:]
        assert len(events) == 2, events
        assert events[0]["key"] == selected and events[1]["key"] == replacement, events
        assert events[0]["sha256"] == events[1]["sha256"], "request body changed during replay"
        expected_fingerprint = hashlib.sha256(replacement.encode()).hexdigest()
        for _ in range(50):
            binding = json.loads(redis("GET", redis_keys[0]))
            if binding["tokenFingerprint"] == expected_fingerprint:
                break
            time.sleep(.1)
        assert binding["tokenFingerprint"] == expected_fingerprint, binding
        assert request() == replacement, "replacement was not persistent"
        stream = request(stream=True)
        assert "data:" in stream and "[DONE]" in stream and replacement in stream, stream[:300]
        docker("stop", names["redis"])
        assert request() in ("synthetic-A", "synthetic-B", "synthetic-C"), "Redis degradation blocked request"
        results.update(passed=True, stable_requests=6, ttl=ttl, retry_attempts=events,
                       replacement_persisted=True, streaming=True, redis_degradation=True)
    finally:
        if "mock_request" in locals():
            try:
                results["upstream_events"] = mock_request("/events")
            except Exception as err:
                results["events_error"] = type(err).__name__
        for n in reversed(created):
            if n.endswith("-envoy"):
                (a.output / "envoy.log").write_text(docker("logs", n, check=False), encoding="utf-8")
            docker("rm", "-f", n, check=False)
        if network_created:
            docker("network", "rm", ident, check=False)
        results["cleanup_remaining"] = docker("ps", "-aq", "--filter", "name=" + ident)
        (a.output / "result.json").write_text(json.dumps(results, indent=2), encoding="utf-8")
        print(json.dumps(results, indent=2))


if __name__ == "__main__":
    main()
