// Copyright (c) 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

#include "proxy_wasm_intrinsics.h"
#include "tests/oauth_parse_probe/parse.h"

class ProbeRoot : public RootContext {
 public:
  ProbeRoot(uint32_t context_id, std::string_view root_id)
      : RootContext(context_id, root_id) {}
};

class ProbeContext : public Context {
 public:
  ProbeContext(uint32_t context_id, RootContext* root)
      : Context(context_id, root) {}

  FilterHeadersStatus onRequestHeaders(uint32_t, bool) override {
    const auto authorization = getRequestHeader("authorization")->toString();
    const std::string prefix = "Bearer ";
    const auto encoded = authorization.rfind(prefix, 0) == 0
                             ? authorization.substr(prefix.size())
                             : authorization;
    const auto result = oauth_v0_probe::evaluate(encoded);
    const auto case_name = getRequestHeader("x-oauth-probe-case")->toString();
    LOG_INFO("OAUTH_V0 case=" + case_name + " outcome=" + result.outcome +
             " detail=" + result.detail);
    if (result.allowed) {
      addRequestHeader("x-oauth-probe-result", "valid");
      return FilterHeadersStatus::Continue;
    }
    sendLocalResponse(401, "oauth_v0_probe_rejection",
                      "probe:" + result.outcome + ":" + result.detail,
                      {{"x-oauth-probe-result", result.outcome}});
    return FilterHeadersStatus::StopIteration;
  }
};

static RegisterContextFactory register_probe(CONTEXT_FACTORY(ProbeContext),
                                             ROOT_FACTORY(ProbeRoot));
