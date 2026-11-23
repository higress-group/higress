#!/usr/bin/env python3
# Copyright (c) 2026 Alibaba Group Holding Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import base64
import hashlib
import hmac
import json
import os
import sys
from pathlib import Path


def encode(raw):
    return base64.urlsafe_b64encode(raw.encode()).decode().rstrip("=")


def token(header, payload):
    signing_input = encode(header) + "." + encode(payload)
    signature = hmac.new(b"oauth-v0-test-key", signing_input.encode(), hashlib.sha256).digest()
    return signing_input + "." + base64.urlsafe_b64encode(signature).decode().rstrip("=")


def main():
    output = Path(sys.argv[1])
    output.mkdir(parents=True, exist_ok=True)
    header = '{"alg":"HS256","typ":"JWT"}'
    payload = '{"client_id":"probe-client","iss":"oauth-v0","sub":"probe","aud":"v0","exp":4102444800,"iat":1700000000}'
    production = os.environ.get("OAUTH_PROBE_VERSION") == "production"
    if production:
        header = '{"alg":"HS256","typ":"application/at+jwt"}'
    valid = token(header, payload)
    header_segment, payload_segment, signature_segment = valid.split(".")
    cases = [
        ("valid", "valid", valid),
        ("no-separator", "caught", "alksdjf"),
        ("one-separator", "caught", "a.b"),
        ("extra-separator", "caught", valid + ".extra"),
        ("header-alphabet", "caught", "%%%%" + "." + payload_segment + "." + signature_segment),
        ("payload-alphabet", "caught", header_segment + ".%%%%." + signature_segment),
        ("signature-alphabet", "caught", header_segment + "." + payload_segment + ".%%%%"),
        ("header-padding", "caught", "====." + payload_segment + "." + signature_segment),
        ("payload-size", "caught", header_segment + ".a." + signature_segment),
        ("signature-size", "caught", header_segment + "." + payload_segment + ".a"),
        ("header-json", "caught", token("{", payload)),
        ("payload-json", "caught", token(header, "{")),
        ("header-array", "caught", token("[]", payload)),
        ("payload-array", "caught", token(header, "[]")),
        ("header-null", "caught", token("null", payload)),
        ("payload-null", "caught", token(header, "null")),
        ("header-overflow", "caught", token('{"alg":"HS256","typ":"JWT","number":1e10000}', payload)),
        ("payload-overflow", "caught", token(header, payload[:-1] + ',"number":1e10000}')),
        ("algorithm-null", "caught", token('{"alg":null,"typ":"JWT"}', payload)),
        ("client-null", "caught", token(header, payload.replace('"probe-client"', "null"))),
        ("iat-wrong-type", "rejected", token(header, payload.replace("1700000000", '"wrong"'))),
        ("exp-null", "caught", token(header, payload.replace("4102444800", "null"))),
        ("invalid-signature", "rejected", header_segment + "." + payload_segment + "." + encode("wrong-signature")),
    ]
    records = [{"name": name, "expected": expected, "token": encoded} for name, expected, encoded in cases]
    if production:
        for name, changed in (("unknown-consumer", payload.replace('"probe-client"', '"unknown"')), ("wrong-issuer", payload.replace('"oauth-v0"', '"wrong"')), ("wrong-subject", payload.replace('"probe"', '"wrong"'))):
            records.append({"name": name, "expected": "rejected", "token": token(header, changed)})
        records.append({"name": "wrong-audience", "expected": "forbidden", "token": token(header, payload.replace('"v0"', '"wrong"'))})
        records.append({"name": "allow-set-forbidden", "expected": "forbidden", "token": valid, "prefix": "/forbidden/"})
        records.append({"name": "keep-token-false", "expected": "valid", "token": token(header, payload.replace('"v0"', '"v0-strip"')), "prefix": "/strip/", "strip_token": True})
        configuration = {"issuer": "oauth-v0", "global_auth": True, "global_credentials": False, "consumers": [{"name": "probe", "client_id": "probe-client", "client_secret": "oauth-v0-test-key"}]}
        strip = dict(configuration, _match_route_=["v0-strip"], keep_token=False)
        configuration["_rules_"] = [{"_match_route_": ["forbidden"], "allow": ["other"]}, strip]
        (output / "config.json").write_text(json.dumps(configuration, sort_keys=True, indent=2) + "\n")
    (output / "cases.json").write_text(json.dumps(records, indent=2) + "\n")
    (output / "cases.tsv").write_text("".join("\t".join(record) + "\n" for record in cases))


if __name__ == "__main__":
    main()
