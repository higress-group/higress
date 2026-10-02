// Copyright (c) 2022 Alibaba Group Holding Ltd.
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

#include "extensions/jwt_auth/plugin.h"

#include <algorithm>
#include <array>
#include <cstdint>
#include <memory>
#include <optional>
#include <string>
#include <unordered_set>
#include <utility>

#include "absl/strings/str_cat.h"
#include "absl/strings/str_format.h"
#include "absl/strings/str_join.h"
#include "absl/strings/str_split.h"
#include "common/common_util.h"
#include "common/http_util.h"
#include "common/json_util.h"

using ::nlohmann::json;
using ::Wasm::Common::JsonArrayIterate;
using ::Wasm::Common::JsonGetField;
using ::Wasm::Common::JsonObjectIterate;
using ::Wasm::Common::JsonValueAs;

#ifdef NULL_PLUGIN

namespace proxy_wasm {
namespace null_plugin {
namespace jwt_auth {

PROXY_WASM_NULL_PLUGIN_REGISTRY

#endif
namespace {
constexpr uint32_t InitPeriod = 50;
constexpr uint32_t MaxUpdatePeriod = 120000;
constexpr uint32_t MinUpdatePeriod = 1000;
constexpr std::string_view LastUpdateKey = "jwt_auth_remote_jwks_update";
constexpr absl::string_view InvalidTokenErrorString =
    ", error=\"invalid_token\"";
constexpr uint32_t MaximumUriLength = 256;
constexpr std::string_view kRcDetailJwtAuthnPrefix = "jwt_authn_access_denied";
std::string generateRcDetails(std::string_view error_msg) {
  // Replace space with underscore since RCDetails may be written to access log.
  // Some log processors assume each log segment is separated by whitespace.
  return absl::StrCat(kRcDetailJwtAuthnPrefix, "{",
                      absl::StrJoin(absl::StrSplit(error_msg, ' '), "_"), "}");
}

void replaceConsumerGroupHeader(
    const std::optional<std::vector<std::string>>& groups) {
  if (!groups.has_value() || groups->empty()) {
    replaceRequestHeader(ConsumerGroupHeader, "");
    return;
  }
  replaceRequestHeader(ConsumerGroupHeader, absl::StrJoin(*groups, ","));
}

}  // namespace
static RegisterContextFactory register_JwtAuth(CONTEXT_FACTORY(PluginContext),
                                               ROOT_FACTORY(PluginRootContext));

#define JSON_FIND_FIELD(dict, field)               \
  auto dict##_##field##_json = dict.find(#field);  \
  if (dict##_##field##_json == dict.end()) {       \
    LOG_WARN("can't find '" #field "' in " #dict); \
    return false;                                  \
  }

#define JSON_VALUE_AS(type, src, dst, err_msg)                      \
  auto dst##_v = JsonValueAs<type>(src);                            \
  if (dst##_v.second != Wasm::Common::JsonParserResultDetail::OK || \
      !dst##_v.first) {                                             \
    LOG_WARN(#err_msg);                                             \
    return false;                                                   \
  }                                                                 \
  auto& dst = dst##_v.first.value();

#define JSON_FIELD_VALUE_AS(type, dict, field)                       \
  JSON_VALUE_AS(type, dict##_##field##_json.value(), dict##_##field, \
                "'" #field "' field in " #dict "convert to " #type " failed")

bool PluginRootContext::parsePluginConfig(const json& configuration,
                                          JwtAuthConfigRule& rule) {
  std::unordered_set<std::string> name_set;
  if (!JsonArrayIterate(
          configuration, "consumers", [&](const json& consumer) -> bool {
            Consumer c;
            JSON_FIND_FIELD(consumer, name);
            JSON_FIELD_VALUE_AS(std::string, consumer, name);
            if (name_set.count(consumer_name) != 0) {
              LOG_WARN("consumer already exists: " + consumer_name);
              return false;
            }
            c.name = consumer_name;
            auto consumer_group_json = consumer.find("group");
            if (consumer_group_json != consumer.end()) {
              std::vector<std::string> gs;
              if (!JsonArrayIterate(
                      consumer, "group", [&](const json& g_json) -> bool {
                        auto g = JsonValueAs<std::string>(g_json);
                        if (g.second !=
                                Wasm::Common::JsonParserResultDetail::OK ||
                            !g.first) {
                          return false;
                        }
                        gs.push_back(g.first.value());
                        return true;
                      })) {
                LOG_WARN("failed to parse 'group' in consumer: " +
                         consumer_name);
                return false;
              }
              c.groups = std::move(gs);
            }
            auto consumer_jwks_json = consumer.find("jwks");
            if (consumer_jwks_json != consumer.end()) {
              JSON_FIELD_VALUE_AS(std::string, consumer, jwks);

              c.jwks = google::jwt_verify::Jwks::createFrom(
                  consumer_jwks, google::jwt_verify::Jwks::JWKS);
              if (c.jwks->getStatus() != Status::Ok) {
                LOG_WARN(absl::StrFormat(
                    "jwks is invalid, consumer:%s, status:%s, jwks:%s",
                    consumer_name,
                    google::jwt_verify::getStatusString(c.jwks->getStatus()),
                    consumer_jwks));
                return false;
              }
            }
            std::unordered_map<std::string, std::string> claims;
            auto consumer_claims_json = consumer.find("claims");
            if (consumer_claims_json != consumer.end()) {
              JSON_FIELD_VALUE_AS(Wasm::Common::JsonObject, consumer, claims);
              if (!JsonObjectIterate(
                      consumer_claims, [&](std::string key) -> bool {
                        auto claims_claim_json = consumer_claims.find(key);
                        JSON_FIELD_VALUE_AS(std::string, claims, claim);
                        claims.emplace(std::make_pair(
                            key, Wasm::Common::trim(claims_claim)));
                        return true;
                      })) {
                LOG_WARN("failed to parse 'claims' in consumer: " +
                         consumer_name);
                return false;
              }
            }
            auto consumer_issuer_json = consumer.find("issuer");
            if (consumer_issuer_json != consumer.end()) {
              JSON_FIELD_VALUE_AS(std::string, consumer, issuer);
              claims.emplace(
                  std::make_pair("iss", Wasm::Common::trim(consumer_issuer)));
            }
            c.allowd_claims = std::move(claims);
            std::vector<FromHeader> from_headers;
            if (!JsonArrayIterate(
                    consumer, "from_headers",
                    [&](const json& from_header) -> bool {
                      JSON_FIND_FIELD(from_header, name);
                      JSON_FIELD_VALUE_AS(std::string, from_header, name);
                      std::string header_value_prefix;
                      auto from_header_value_prefix_json =
                          from_header.find("value_prefix");
                      if (from_header_value_prefix_json != from_header.end()) {
                        JSON_FIELD_VALUE_AS(std::string, from_header,
                                            value_prefix);
                        header_value_prefix = from_header_value_prefix;
                      }
                      from_headers.push_back(
                          FromHeader{from_header_name, header_value_prefix});
                      return true;
                    })) {
              LOG_WARN("failed to parse 'from_headers' in consumer: " +
                       consumer_name);
              return false;
            }
            std::vector<std::string> from_params;
            if (!JsonArrayIterate(consumer, "from_params",
                                  [&](const json& from_param_json) -> bool {
                                    JSON_VALUE_AS(std::string, from_param_json,
                                                  from_param, "invalid item");
                                    from_params.push_back(from_param);
                                    return true;
                                  })) {
              LOG_WARN("failed to parse 'from_params' in consumer: " +
                       consumer_name);
              return false;
            }
            std::vector<std::string> from_cookies;
            if (!JsonArrayIterate(consumer, "from_cookies",
                                  [&](const json& from_cookie_json) -> bool {
                                    JSON_VALUE_AS(std::string, from_cookie_json,
                                                  from_cookie, "invalid item");
                                    from_cookies.push_back(from_cookie);
                                    return true;
                                  })) {
              LOG_WARN("failed to parse 'from_cookies' in consumer: " +
                       consumer_name);
              return false;
            }
            if (!from_headers.empty() || !from_params.empty() ||
                !from_cookies.empty()) {
              c.from_headers = std::move(from_headers);
              c.from_params = std::move(from_params);
              c.from_cookies = std::move(from_cookies);
            }
            std::unordered_map<std::string, ClaimToHeader> claims_to_headers;
            if (!JsonArrayIterate(
                    consumer, "claims_to_headers",
                    [&](const json& item_json) -> bool {
                      JSON_VALUE_AS(Wasm::Common::JsonObject, item_json, item,
                                    "invalid item");
                      JSON_FIND_FIELD(item, claim);
                      JSON_FIELD_VALUE_AS(std::string, item, claim);
                      auto c2h_it = claims_to_headers.find(item_claim);
                      if (c2h_it != claims_to_headers.end()) {
                        LOG_WARN("claim to header already exists: " +
                                 item_claim);
                        return false;
                      }
                      auto& c2h = claims_to_headers[item_claim];
                      JSON_FIND_FIELD(item, header);
                      JSON_FIELD_VALUE_AS(std::string, item, header);
                      c2h.header = std::move(item_header);
                      auto item_override_json = item.find("override");
                      if (item_override_json != item.end()) {
                        JSON_FIELD_VALUE_AS(bool, item, override);
                        c2h.override = item_override;
                      }
                      return true;
                    })) {
              LOG_WARN("failed to parse 'claims_to_headers' in consumer: " +
                       consumer_name);
              return false;
            }
            c.claims_to_headers = std::move(claims_to_headers);
            auto consumer_clock_skew_seconds_json =
                consumer.find("clock_skew_seconds");
            if (consumer_clock_skew_seconds_json != consumer.end()) {
              JSON_FIELD_VALUE_AS(uint64_t, consumer, clock_skew_seconds);
              c.clock_skew = consumer_clock_skew_seconds;
            }
            auto consumer_keep_token_json = consumer.find("keep_token");
            if (consumer_keep_token_json != consumer.end()) {
              JSON_FIELD_VALUE_AS(bool, consumer, keep_token);
              c.keep_token = consumer_keep_token;
            }
            auto consumer_remote_jwks_json = consumer.find("remote_jwks");
            if (consumer_remote_jwks_json != consumer.end()) {
              RemoteJwks remote_jwks;
              JSON_FIELD_VALUE_AS(Wasm::Common::JsonObject, consumer,
                                  remote_jwks);
              JSON_FIND_FIELD(consumer_remote_jwks, uri);
              JSON_FIELD_VALUE_AS(std::string, consumer_remote_jwks, uri);
              remote_jwks.uri = consumer_remote_jwks_uri;
              JSON_FIND_FIELD(consumer_remote_jwks, service);
              JSON_FIELD_VALUE_AS(std::string, consumer_remote_jwks, service);
              remote_jwks.service = consumer_remote_jwks_service;
              JSON_FIND_FIELD(consumer_remote_jwks, port);
              JSON_FIELD_VALUE_AS(std::string, consumer_remote_jwks, port);
              remote_jwks.port = consumer_remote_jwks_port;
              auto consumer_remote_jwks_timeout_json =
                  consumer_remote_jwks.find("timeout");
              if (consumer_remote_jwks_timeout_json !=
                  consumer_remote_jwks.end()) {
                JSON_FIELD_VALUE_AS(int64_t, consumer_remote_jwks, timeout);
                remote_jwks.timeout = consumer_remote_jwks_timeout;
              }
              auto consumer_remote_jwks_ttl_json =
                  consumer_remote_jwks.find("ttl");
              if (consumer_remote_jwks_ttl_json != consumer_remote_jwks.end()) {
                JSON_FIELD_VALUE_AS(int64_t, consumer_remote_jwks, ttl);
                remote_jwks.ttl = consumer_remote_jwks_ttl;
                if (consumer_remote_jwks_ttl < cache_update_period_) {
                  cache_update_period_ = consumer_remote_jwks_ttl;
                }
              }
              remote_jwks.cacheKey =
                  absl::StrCat(remote_jwks.uri, "#", remote_jwks.service, "#",
                               remote_jwks.port);
              auto it = all_remote_jwks_.find(remote_jwks.cacheKey);
              if (it == all_remote_jwks_.end()) {
                all_remote_jwks_[remote_jwks.cacheKey] = remote_jwks;
              } else if (it->second.ttl > remote_jwks.ttl) {
                // use the minimum ttl if cacheKey already exists
                it->second.ttl = remote_jwks.ttl;
              }
              c.remote_jwks = std::move(remote_jwks);
            }
            if (!c.remote_jwks.has_value() && !c.jwks) {
              LOG_WARN(
                  "at least one of the `jwks` and `remote_jwks` fields must be "
                  "configured for a consumer.");
              return false;
            }
            c.extractor = Extractor::create(c);
            rule.consumers.push_back(std::move(c));
            name_set.insert(consumer_name);
            return true;
          })) {
    LOG_WARN("failed to parse configuration for consumers.");
    return false;
  }
  // if (rule.consumers.empty()) {
  //   LOG_INFO("at least one consumer has to be configured for a rule.");
  //   return false;
  // }
  if (configuration.find("rbac_rules") != configuration.end()) {
    if (!JsonArrayIterate(configuration, "rbac_rules", [&](const json& rbac_rule_config) -> bool {
      RbacRule rbac_rule;
      if (!rbac_rule.parse(rbac_rule_config)) {
        LOG_ERROR("failed to parse 'rbac_rules' field in filter configuration.");
        return false;
      } else {
        rule.rbac_rules.push_back(std::move(rbac_rule));
        return true;
      }
    })) {
      LOG_ERROR("failed to parse configuration for rbac_rules");
      return false;
    }
  }
  std::vector<std::string> enable_headers;
  if (!JsonArrayIterate(configuration, "enable_headers",
                        [&](const json& enable_header_json) -> bool {
                          JSON_VALUE_AS(std::string, enable_header_json,
                                        enable_header, "invalid item");
                          enable_headers.push_back(enable_header);
                          return true;
                        })) {
    LOG_WARN("failed to parse 'enable_headers'");
    return false;
  }
  rule.enable_headers = std::move(enable_headers);
  return true;
}

Status PluginRootContext::consumerVerify(
    const Consumer& consumer, uint64_t now,
    std::vector<JwtLocationConstPtr>& jwt_tokens) {
  if (!consumer.jwks) {
    // return jwt verify failed if no jwks, maybe jwks fetch failed
    LOG_ERROR(absl::StrFormat("consumer:%s jwks not exists", consumer.name));
    return Status::JwtVerificationFail;
  }
  auto tokens = consumer.extractor->extract();
  if (tokens.empty()) {
    return Status::JwtMissed;
  }
  for (auto& token : tokens) {
    google::jwt_verify::Jwt jwt;
    Status status = jwt.parseFromString(token->token());
    if (status != Status::Ok) {
      LOG_INFO(absl::StrFormat(
          "jwt parse failed, consumer:%s, token:%s, status:%s", consumer.name,
          token->token(), google::jwt_verify::getStatusString(status)));
      return status;
    }
    StructUtils payload_getter(jwt.payload_pb_);
    if (!consumer.allowd_claims.empty()) {
      for (const auto& claim : consumer.allowd_claims) {
        std::string value;
        if (payload_getter.GetString(claim.first, &value) ==
            StructUtils::WRONG_TYPE) {
          LOG_INFO(absl::StrFormat(
              "jwt payload invalid, consumer:%s, token:%s, claim:%s",
              consumer.name, jwt.payload_str_, claim.first));
          return Status::JwtVerificationFail;
        }
        if (value != claim.second) {
          LOG_INFO(absl::StrFormat(
              "jwt payload invalid, consumer:%s, claim:%s, value:%s, expect:%s",
              consumer.name, claim.first, value, claim.second));
          return Status::JwtVerificationFail;
        }
      }
    }
    status = jwt.verifyTimeConstraint(now, consumer.clock_skew);
    if (status != Status::Ok) {
      LOG_DEBUG(absl::StrFormat(
          "jwt verify time failed, consumer:%s,  token:%s, status:%s",
          consumer.name, token->token(),
          google::jwt_verify::getStatusString(status)));
      return status;
    }
    status =
        google::jwt_verify::verifyJwtWithoutTimeChecking(jwt, *consumer.jwks);
    if (status != Status::Ok) {
      LOG_DEBUG(absl::StrFormat(
          "jwt verify failed, consumer:%s, token:%s, status:%s", consumer.name,
          token->token(), google::jwt_verify::getStatusString(status)));
      return status;
    }
    for (const auto& claim_to_header : consumer.claims_to_headers) {
      std::string value;
      if (payload_getter.GetString(claim_to_header.first, &value) !=
          StructUtils::WRONG_TYPE) {
        token->addClaimToHeader(claim_to_header.second.header, value,
                                claim_to_header.second.override);
      } else {
        uint64_t num_value;
        if (payload_getter.GetUInt64(claim_to_header.first, &num_value) !=
            StructUtils::WRONG_TYPE) {
          token->addClaimToHeader(claim_to_header.second.header,
                                  std::to_string((unsigned long long)num_value),
                                  claim_to_header.second.override);
        }
      }
    }
  }
  jwt_tokens = std::move(tokens);
  return Status::Ok;
}

void PluginRootContext::updateRemoteJwksCache() {
  // sync local cache
  for (auto& pair : all_remote_jwks_) {
    // find in shared cache
    WasmDataPtr jwks_str;
    if (WasmResult::Ok == getSharedData(pair.second.cacheKey, &jwks_str)) {
      auto jwks = google::jwt_verify::Jwks::createFrom(
          jwks_str->toString(), google::jwt_verify::Jwks::Type::JWKS);
      JwksPtr shared_jwks(std::move(jwks));
      jwks_fetcher_.updateCache(pair.second.cacheKey, shared_jwks);
    }
  }
  WasmDataPtr last_update_data;
  uint32_t last_update_cas;
  auto current_time = getCurrentTimeNanoseconds();
  auto res = getSharedData(LastUpdateKey, &last_update_data, &last_update_cas);
  if (res != WasmResult::Ok) {
    setSharedData(LastUpdateKey, {reinterpret_cast<const char*>(&current_time),
                                  sizeof(current_time)});
  } else {
    uint64_t last_update =
        *reinterpret_cast<const uint64_t*>(last_update_data->data());
    if (current_time - last_update < cache_update_period_ * 1e6) {
      // no need to update
      return;
    }
    if (WasmResult::CasMismatch ==
        setSharedData(LastUpdateKey,
                      {reinterpret_cast<const char*>(&current_time),
                       sizeof(current_time)},
                      last_update_cas)) {
      // already updated by other thread
      LOG_DEBUG("remote jwks cache has updated by other worker");
      return;
    }
  }
  for (auto& pair : all_remote_jwks_) {
    const auto& cacheKey = pair.second.cacheKey;
    jwks_fetcher_.fetch(
        this, pair.second,
        [=](const JwksPtr& jwks) {
          if (!jwks) {
            LOG_WARN(absl::StrFormat(
                "update remote jwks cache failed, cacheKey:%s", cacheKey));
          } else {
            LOG_INFO(absl::StrFormat(
                "update remote jwks cache success, cacheKey:%s", cacheKey));
          }
        },
        true);
  }
}

bool PluginRootContext::fetchAllConsumerJwks(std::vector<Consumer>& consumers,
                                             uint32_t http_ctx_id,
                                             std::function<bool()> post_func) {
  auto current_time = getCurrentTimeNanoseconds();
  auto ref_ptr = std::make_shared<char>(0);
  auto all_done = true;
  for (auto& consumer : consumers) {
    if (!consumer.jwks || (consumer.remote_jwks.has_value() &&
                           current_time > consumer.jwks_expire_at)) {
      auto fetched = jwks_fetcher_.fetch(
          this, consumer.remote_jwks.value(),
          [=, &consumer](const JwksPtr& jwks) {
            if (!jwks) {
              LOG_ERROR(absl::StrFormat(
                  "fetch jwks failed, consumer:%s, cacheKey:%s", consumer.name,
                  consumer.remote_jwks.value().cacheKey));
            } else {
              consumer.jwks_expire_at = getCurrentTimeNanoseconds() +
                                        consumer.remote_jwks.value().ttl * 1e6;
              consumer.jwks = jwks;
              LOG_DEBUG(
                  absl::StrCat("jwks received, consumer: ", consumer.name));
            }
            // the last refer need to resume the request
            if (ref_ptr.unique()) {
              auto* context = getContext(http_ctx_id);
              if (context == nullptr) {
                LOG_ERROR(absl::StrFormat("getContext failed, context id:%d",
                                          http_ctx_id));
                return;
              }
              auto* plugin_context = dynamic_cast<PluginContext*>(context);
              if (plugin_context == nullptr || plugin_context->isDone()) {
                LOG_DEBUG(
                    "context recover failed, maybe stream was already "
                    "destroyed");
                return;
              }
              auto res = context->setEffectiveContext();
              if (res != WasmResult::Ok) {
                LOG_ERROR("setEffectiveContext failed, result:" +
                          toString(res));
                return;
              }
              // if checkPlugin success, resume the request
              if (post_func()) {
                res = continueRequest();
                if (res != WasmResult::Ok) {
                  LOG_ERROR("continueRequest failed, result:" + toString(res));
                  return;
                }
              }
              LOG_INFO(absl::StrFormat(
                  "all consumer jwks have been fetched once, resume context:%d",
                  http_ctx_id));
            }
          });
      if (!fetched) {
        all_done = false;
      }
    }
  }

  return all_done;
}

bool PluginRootContext::checkPlugin(
    const JwtAuthConfigRule& rule,
    const std::optional<std::unordered_set<std::string>>& allow_set) {
  // Drop any client-supplied value so only this gateway's assertion survives,
  // including on the enable_headers skip-auth path below.
  removeRequestHeader("X-Mse-Consumer");
  if (!rule.enable_headers.empty()) {
    bool skip_auth = true;
    for (const auto& enable_header : rule.enable_headers) {
      auto header_ptr = getRequestHeader(enable_header);
      if (header_ptr->size() > 0) {
        LOG_DEBUG("enable by header: " + header_ptr->toString());
        skip_auth = false;
        break;
      }
    }
    if (skip_auth) {
      return true;
    }
  }
  std::optional<Status> err_status;
  bool verified = false;
  uint64_t now = getCurrentTimeNanoseconds() / 1e9;
  for (const auto& consumer : rule.consumers) {
    std::vector<JwtLocationConstPtr> tokens;
    auto status = consumerVerify(consumer, now, tokens);
    if (status == Status::Ok) {
      verified = true;
      replaceRequestHeader("X-Mse-Consumer", consumer.name);
      replaceConsumerGroupHeader(consumer.groups);
      if (checkAuthorization(consumer.name, rule, allow_set)) {
        for (auto& token : tokens) {
          if (!consumer.keep_token) {
            token->removeJwt();
          }
          token->claimsToHeaders();
        }
        return true;
      }
    }
    // use the first status
    if (!err_status) {
      err_status = status;
    }
  }
  if (!verified) {
    auto status = err_status ? err_status.value() : Status::JwtMissed;
    auto err_str = google::jwt_verify::getStatusString(status);
    auto authn_value = absl::StrCat(
        "Bearer realm=\"",
        Wasm::Common::Http::buildOriginalUri(MaximumUriLength), "\"");
    if (status != Status::JwtMissed) {
      absl::StrAppend(&authn_value, InvalidTokenErrorString);
    }
    sendLocalResponse(401, generateRcDetails(err_str), err_str,
                      {{"WWW-Authenticate", authn_value}});
  } else {
    sendLocalResponse(403, kRcDetailJwtAuthnPrefix, "Access Denied", {});
  }
  return false;
}

bool PluginRootContext::checkAuthorization(
    const std::string& consumer, const JwtAuthConfigRule& rule,
    const std::optional<std::unordered_set<std::string>>& allow_set) {
  if (!rule.rbac_rules.empty()) {
    return checkRbacRule(rule);
  }
  // Global config without an allow set permits every authenticated consumer.
  return !allow_set || allow_set->find(consumer) != allow_set->end();
}

bool PluginRootContext::checkRbacRule(const JwtAuthConfigRule& rule) {
  for (const auto& rbac_rule : rule.rbac_rules) {
    if (rbac_rule.check()) {
      return true;
    }
  }
  return false;
}

void PluginRootContext::onTick() {
  updateRemoteJwksCache();
  if (!on_tick_triggered_) {
    on_tick_triggered_ = true;
    proxy_set_tick_period_milliseconds(cache_update_period_);
  }
}

bool PluginRootContext::onConfigure(size_t size) {
  // Parse configuration JSON string.
  if (size > 0 && !configure(size)) {
    LOG_WARN("configuration has errors initialization will not continue.");
    setInvalidConfig();
    return false;
  }
  if (cache_update_period_ > MaxUpdatePeriod) {
    // Guarantee a certain update frequency to achieve compensation for fetching
    // failures
    cache_update_period_ = MaxUpdatePeriod;
  } else if (cache_update_period_ < MinUpdatePeriod) {
    cache_update_period_ = MinUpdatePeriod;
  }
  proxy_set_tick_period_milliseconds(InitPeriod);
  return true;
}

bool PluginRootContext::configure(size_t configuration_size) {
  auto configuration_data = getBufferBytes(WasmBufferType::PluginConfiguration,
                                           0, configuration_size);
  // Parse configuration JSON string.
  auto result = ::Wasm::Common::JsonParse(configuration_data->view());
  if (!result) {
    LOG_WARN(absl::StrCat("cannot parse plugin configuration JSON string: ",
                          configuration_data->view()));
    return false;
  }
  if (!parseAuthRuleConfig(result.value())) {
    LOG_WARN(absl::StrCat("cannot parse plugin configuration JSON string: ",
                          configuration_data->view()));
    return false;
  }
  return true;
}

FilterHeadersStatus PluginContext::onRequestHeaders(uint32_t, bool) {
  auto* rootCtx = rootContext();
  auto http_ctx_id = id();
  return rootCtx->checkAuthRule([=](const auto& config, const auto& allow_set) {
    if (!rootCtx->fetchAllConsumerJwks(
            config.consumers, http_ctx_id, [&, allow_set]() {
              return rootCtx->checkPlugin(config, allow_set);
            })) {
      LOG_INFO(absl::StrFormat(
          "not all consumer jwks are fetched, pause context:%u", http_ctx_id));
      return false;
    }
    return rootCtx->checkPlugin(config, allow_set);
  })
             ? FilterHeadersStatus::Continue
             : FilterHeadersStatus::StopIteration;
}

#ifdef NULL_PLUGIN

}  // namespace jwt_auth
}  // namespace null_plugin
}  // namespace proxy_wasm

#endif
