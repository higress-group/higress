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
#include <functional>
#include <optional>
#include <string>
#include <string_view>
#include <unordered_map>
#include <utility>
#include <vector>

#include "common/http_util.h"
#include "common/route_rule_matcher.h"
#define ASSERT(_X) assert(_X)

#ifndef NULL_PLUGIN

#include "proxy_wasm_intrinsics.h"

#else

#include "include/proxy-wasm/null_plugin.h"

namespace proxy_wasm {
namespace null_plugin {
namespace hmac_auth {

#endif

struct HmacAuthConfigRule {
  std::unordered_map<std::string, std::string> credentials;
  std::unordered_map<std::string, std::string> key_to_name;
  std::unordered_map<std::string, std::vector<std::string>> key_to_groups;
  std::vector<RbacRule> rbac_rules;
  int64_t date_nano_offset = -1;
};

using RequestHeaderPairs = std::vector<std::pair<std::string, std::string>>;

// PluginRootContext is the root context for all streams processed by the
// thread. It has the same lifetime as the worker thread and acts as target for
// interactions that outlives individual stream, e.g. timer, async calls.
class PluginRootContext : public RootContext,
                          public RouteRuleMatcher<HmacAuthConfigRule> {
 public:
  PluginRootContext(uint32_t id, std::string_view root_id)
      : RootContext(id, root_id) {}
  ~PluginRootContext() {}
  bool onConfigure(size_t) override;
  bool checkPlugin(const std::string& ca_key, const std::string& signature,
                   const std::string& signature_method, const std::string& path,
                   const std::string& date, bool is_timestamp, std::string* sts,
                   std::string* sts_old,
                   const RequestHeaderPairs& request_headers,
                   const HmacAuthConfigRule&, std::string_view raw_form_body);
  bool checkRbacRule(const HmacAuthConfigRule&);
  bool checkAuthorization(
      const std::string& consumer, const HmacAuthConfigRule&,
      const std::optional<std::unordered_set<std::string>>&);
  bool checkConsumer(const std::string&, const HmacAuthConfigRule&,
                     const std::optional<std::unordered_set<std::string>>&);
  bool configure(size_t);

 private:
  bool parsePluginConfig(const json&, HmacAuthConfigRule&) override;
};

// Per-stream context.
class PluginContext : public Context {
 public:
  explicit PluginContext(uint32_t id, RootContext* root) : Context(id, root) {}
  FilterHeadersStatus onRequestHeaders(uint32_t, bool) override;
  FilterDataStatus onRequestBody(size_t, bool) override;

 private:
  inline PluginRootContext* rootContext() {
    return dynamic_cast<PluginRootContext*>(this->root());
  }

  std::string ca_key_;
  std::string signature_;
  std::string signature_method_;
  std::string path_;
  std::string date_;
  std::string str_to_sign_;
  std::string str_to_sign_old_;
  RequestHeaderPairs request_headers_;
  std::string body_md5_;
  bool is_timestamp_ = false;
  std::optional<std::reference_wrapper<HmacAuthConfigRule>> config_;
  std::optional<std::unordered_set<std::string>> allow_set_;
  bool check_body_params_ = false;
};

#ifdef NULL_PLUGIN

}  // namespace hmac_auth
}  // namespace null_plugin
}  // namespace proxy_wasm

#endif
