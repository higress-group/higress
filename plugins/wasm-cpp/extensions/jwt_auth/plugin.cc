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
            JSON_FIND_FIELD(consumer, jwks);
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
            c.extractor = Extractor::create(c);
            rule.consumers.push_back(std::move(c));
            name_set.insert(consumer_name);
            return true;
          })) {
    LOG_WARN("failed to parse configuration for consumers.");
    return false;
  }
  if (rule.consumers.empty()) {
    LOG_INFO("at least one consumer has to be configured for a rule.");
    return false;
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
      // global config without allow_set field allows any consumers
      if (!allow_set ||
          allow_set.value().find(consumer.name) != allow_set.value().end()) {
        addRequestHeader("X-Mse-Consumer", consumer.name);
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

bool PluginRootContext::onConfigure(size_t size) {
  // Parse configuration JSON string.
  if (size > 0 && !configure(size)) {
    LOG_WARN("configuration has errors initialization will not continue.");
    return false;
  }
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
  return rootCtx->checkAuthRule(
             [rootCtx](const auto& config, const auto& allow_set) {
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
