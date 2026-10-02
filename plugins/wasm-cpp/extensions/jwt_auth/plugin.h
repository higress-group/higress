/*
 * Copyright (c) 2022 Alibaba Group Holding Ltd.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

#include <assert.h>

#include <cstdint>
#include <string>
#include <unordered_map>
#include <unordered_set>

#include "common/route_rule_matcher.h"
#include "extensions/jwt_auth/extractor.h"
#include "extensions/jwt_auth/jwks_fetcher.h"
#include "jwt_verify_lib/check_audience.h"
#include "jwt_verify_lib/jwt.h"
#include "jwt_verify_lib/status.h"
#include "jwt_verify_lib/struct_utils.h"
#include "jwt_verify_lib/verify.h"
#define ASSERT(_X) assert(_X)

#ifndef NULL_PLUGIN

#include "proxy_wasm_intrinsics.h"

#else

#include "include/proxy-wasm/null_plugin.h"

namespace proxy_wasm {
namespace null_plugin {
namespace jwt_auth {

#endif

using ::google::jwt_verify::Status;
using ::google::jwt_verify::StructUtils;

struct FromHeader {
  std::string header;
  std::string value_prefix;
};

struct ClaimToHeader {
  std::string header;
  bool override = true;
};

using ClaimsMap =
    std::unordered_map<std::string /*claim*/, std::string /*claim value*/>;

struct Consumer {
  std::string name;
  std::optional<std::vector<std::string>> groups;
  std::optional<RemoteJwks> remote_jwks = std::nullopt;
  mutable JwksPtr jwks;
  mutable uint64_t jwks_expire_at = 0;
  ClaimsMap allowd_claims;
  std::vector<FromHeader> from_headers = {{"Authorization", "Bearer "}};
  std::vector<std::string> from_params = {"access_token"};
  std::vector<std::string> from_cookies;
  uint64_t clock_skew = 60;
  bool keep_token = true;
  std::unordered_map<std::string /*claim*/, ClaimToHeader> claims_to_headers;
  ExtractorConstPtr extractor;
};

struct JwtAuthConfigRule {
  mutable std::vector<Consumer> consumers;
  std::vector<RbacRule> rbac_rules;
  std::vector<std::string> enable_headers;
};

// PluginRootContext is the root context for all streams processed by the
// thread. It has the same lifetime as the worker thread and acts as target for
// interactions that outlives individual stream, e.g. timer, async calls.
class PluginRootContext : public RootContext,
                          public RouteRuleMatcher<JwtAuthConfigRule> {
 public:
  PluginRootContext(uint32_t id, std::string_view root_id)
      : RootContext(id, root_id) {}
  ~PluginRootContext() {}
  bool onConfigure(size_t) override;
  void onTick() override;
  bool checkPlugin(const JwtAuthConfigRule&,
                   const std::optional<std::unordered_set<std::string>>&);
  bool checkAuthorization(
      const std::string& consumer, const JwtAuthConfigRule&,
      const std::optional<std::unordered_set<std::string>>&);
  bool checkRbacRule(const JwtAuthConfigRule&);
  bool configure(size_t);
  bool fetchAllConsumerJwks(std::vector<Consumer>& consumers,
                            uint32_t http_ctx_id,
                            std::function<bool()> post_func);
  void updateRemoteJwksCache();

 private:
  bool parsePluginConfig(const json&, JwtAuthConfigRule&) override;
  Status consumerVerify(const Consumer&, uint64_t,
                        std::vector<JwtLocationConstPtr>&);
  std::string extractCredential(const JwtAuthConfigRule&);

  JwksFetcher jwks_fetcher_;
  uint64_t cache_update_period_ = UINT64_MAX;
  bool on_tick_triggered_ = false;
  std::unordered_map<std::string /*remoteJwks cacheKey*/, RemoteJwks>
      all_remote_jwks_;
};

// Per-stream context.
class PluginContext : public Context {
 public:
  explicit PluginContext(uint32_t id, RootContext* root) : Context(id, root) {}
  FilterHeadersStatus onRequestHeaders(uint32_t, bool) override;
  void onDone() override { is_done_ = true; }
  bool isDone() { return is_done_; }

 private:
  inline PluginRootContext* rootContext() {
    return dynamic_cast<PluginRootContext*>(this->root());
  }
  bool is_done_ = false;
};

#ifdef NULL_PLUGIN

}  // namespace jwt_auth
}  // namespace null_plugin
}  // namespace proxy_wasm

#endif
