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

#include "extensions/oauth/plugin.h"

#include <algorithm>
#include <array>
#include <chrono>
#include <cstdint>
#include <memory>
#include <optional>
#include <string>
#include <system_error>
#include <unordered_set>
#include <utility>

#include "absl/strings/match.h"
#include "absl/strings/str_cat.h"
#include "absl/strings/str_format.h"
#include "absl/strings/str_join.h"
#include "absl/strings/str_split.h"
#include "common/common_util.h"
#include "common/http_util.h"
#include "common/json_util.h"
#include "uuid.h"

using ::nlohmann::json;
using ::Wasm::Common::JsonArrayIterate;
using ::Wasm::Common::JsonGetField;
using ::Wasm::Common::JsonObjectIterate;
using ::Wasm::Common::JsonValueAs;

#ifdef NULL_PLUGIN

namespace proxy_wasm {
namespace null_plugin {
namespace oauth {

PROXY_WASM_NULL_PLUGIN_REGISTRY

#endif
namespace {
constexpr absl::string_view TokenResponseTemplate = R"(
{
  "token_type": "bearer",
  "access_token": "%s",
  "expires_in": %u
})";
const std::string& DefaultAudience = "default";
const std::string& TypeHeader = "application/at+jwt";
const std::string& BearerPrefix = "Bearer ";
const std::string& ClientCredentialsGrant = "client_credentials";
constexpr uint32_t MaximumUriLength = 256;
constexpr std::string_view kRcDetailOAuthPrefix = "oauth_access_denied";
std::string generateRcDetails(std::string_view error_msg) {
  // Replace space with underscore since RCDetails may be written to access log.
  // Some log processors assume each log segment is separated by whitespace.
  return absl::StrCat(kRcDetailOAuthPrefix, "{",
                      absl::StrJoin(absl::StrSplit(error_msg, ' '), "_"), "}");
}
}  // namespace
static RegisterContextFactory register_OAuth(CONTEXT_FACTORY(PluginContext),
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

bool PluginRootContext::generateToken(const OAuthConfigRule& rule,
                                      const std::string& route_name,
                                      const absl::string_view& raw_params,
                                      std::string* token,
                                      std::string* err_msg) {
  auto params = Wasm::Common::Http::parseParameters(raw_params, 0, true);
  auto it = params.find("grant_type");
  if (it == params.end()) {
    *err_msg = "grant_type is missing";
    return false;
  }
  if (it->second != ClientCredentialsGrant) {
    *err_msg = absl::StrFormat("grant_type:%s is not support", it->second);
    return false;
  }
  it = params.find("client_id");
  if (it == params.end()) {
    *err_msg = "client_id is missing";
    return false;
  }
  auto c_it = rule.consumers.find(it->second);
  if (c_it == rule.consumers.end()) {
    *err_msg = "invalid client_id or client_secret";
    return false;
  }
  const auto& consumer = c_it->second;
  it = params.find("client_secret");
  if (it == params.end()) {
    *err_msg = "client_secret is missing";
    return false;
  }
  if (it->second != consumer.client_secret) {
    *err_msg = "invalid client_id or client_secret";
    return false;
  }
  auto jwt = jwt::create();
  if (rule.global_credentials) {
    jwt.set_audience(DefaultAudience);
  } else {
    jwt.set_audience(route_name);
  }
  it = params.find("scope");
  if (it != params.end()) {
    jwt.set_payload_claim("scope", jwt::claim(it->second));
  }
  std::random_device rd;
  auto seed_data = std::array<int, std::mt19937::state_size>{};
  std::generate(std::begin(seed_data), std::end(seed_data), std::ref(rd));
  std::seed_seq seq(std::begin(seed_data), std::end(seed_data));
  std::mt19937 generator(seq);
  uuids::uuid_random_generator gen{generator};
  std::error_code ec;
  *token = jwt.set_issuer(rule.issuer)
               .set_type(TypeHeader)
               .set_subject(consumer.name)
               .set_issued_at(std::chrono::system_clock::now())
               .set_expires_at(std::chrono::system_clock::now() +
                               std::chrono::seconds{rule.token_ttl})
               .set_payload_claim("client_id", jwt::claim(consumer.client_id))
               .set_id(uuids::to_string(gen()))
               .sign(jwt::algorithm::hs256{consumer.client_secret}, ec);
  if (ec) {
    *err_msg = absl::StrCat("jwt sign failed: %s", ec.message());
    return false;
  }
  return true;
}

bool PluginRootContext::parsePluginConfig(const json& conf,
                                          OAuthConfigRule& rule) {
  std::unordered_set<std::string> name_set;
  if (!JsonArrayIterate(conf, "consumers", [&](const json& consumer) -> bool {
        Consumer c;
        JSON_FIND_FIELD(consumer, name);
        JSON_FIELD_VALUE_AS(std::string, consumer, name);
        if (name_set.count(consumer_name) != 0) {
          LOG_WARN("consumer already exists: " + consumer_name);
          return false;
        }
        c.name = consumer_name;
        JSON_FIND_FIELD(consumer, client_id);
        JSON_FIELD_VALUE_AS(std::string, consumer, client_id);
        c.client_id = consumer_client_id;
        if (rule.consumers.find(c.client_id) != rule.consumers.end()) {
          LOG_WARN("consumer client_id already exists: " + c.client_id);
          return false;
        }
        JSON_FIND_FIELD(consumer, client_secret);
        JSON_FIELD_VALUE_AS(std::string, consumer, client_secret);
        c.client_secret = consumer_client_secret;
        rule.consumers.emplace(c.client_id, std::move(c));
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
  auto conf_issuer_json = conf.find("issuer");
  if (conf_issuer_json != conf.end()) {
    JSON_FIELD_VALUE_AS(std::string, conf, issuer);
    rule.issuer = conf_issuer;
  }
  auto conf_auth_header_json = conf.find("auth_header");
  if (conf_auth_header_json != conf.end()) {
    JSON_FIELD_VALUE_AS(std::string, conf, auth_header);
    rule.auth_header_name = conf_auth_header;
  }
  auto conf_auth_path_json = conf.find("auth_path");
  if (conf_auth_path_json != conf.end()) {
    JSON_FIELD_VALUE_AS(std::string, conf, auth_path);
    if (conf_auth_path.empty()) {
      conf_auth_path = "/";
    } else if (conf_auth_path[0] != '/') {
      conf_auth_path = absl::StrCat("/", conf_auth_path);
    }
    rule.auth_path = conf_auth_path;
  }
  auto conf_global_credentials_json = conf.find("global_credentials");
  if (conf_global_credentials_json != conf.end()) {
    JSON_FIELD_VALUE_AS(bool, conf, global_credentials);
    rule.global_credentials = conf_global_credentials;
  }
  auto conf_token_ttl_json = conf.find("token_ttl");
  if (conf_token_ttl_json != conf.end()) {
    JSON_FIELD_VALUE_AS(uint64_t, conf, token_ttl);
    rule.token_ttl = conf_token_ttl;
  }
  auto conf_keep_token_json = conf.find("keep_token");
  if (conf_keep_token_json != conf.end()) {
    JSON_FIELD_VALUE_AS(bool, conf, keep_token);
    rule.keep_token = conf_keep_token;
  }
  auto conf_clock_skew_seconds_json = conf.find("clock_skew_seconds");
  if (conf_clock_skew_seconds_json != conf.end()) {
    JSON_FIELD_VALUE_AS(uint64_t, conf, clock_skew_seconds);
    rule.clock_skew = conf_clock_skew_seconds;
  }
  return true;
}

#define CLAIM_CHECK(token, claim, type)                     \
  if (!token.has_payload_claim(#claim)) {                   \
    LOG_DEBUG("claim is missing: " #claim);                 \
    goto failed;                                            \
  }                                                         \
  if (token.get_payload_claim(#claim).get_type() != type) { \
    LOG_DEBUG("claim is invalid: " #claim);                 \
    goto failed;                                            \
  }

bool PluginRootContext::checkPlugin(
    const OAuthConfigRule& rule,
    const std::optional<std::unordered_set<std::string>>& allow_set,
    const std::string& route_name) {
  // Drop any client-supplied value so only this gateway's assertion survives.
  removeRequestHeader("X-Mse-Consumer");
  auto auth_header = getRequestHeader(rule.auth_header_name)->toString();
  bool verified = false;
  std::string token_str;
  {
    size_t pos;
    if (auth_header.empty()) {
      LOG_DEBUG("auth header is empty");
      goto failed;
    }
    pos = auth_header.find(BearerPrefix);
    if (pos == std::string::npos) {
      LOG_DEBUG("auth header is not a bearer token");
      goto failed;
    }
    auto start = pos + BearerPrefix.size();
    token_str =
        std::string{auth_header.c_str() + start, auth_header.size() - start};
    auto token = jwt::decode(token_str);
    CLAIM_CHECK(token, client_id, jwt::json::type::string);
    CLAIM_CHECK(token, iss, jwt::json::type::string);
    CLAIM_CHECK(token, sub, jwt::json::type::string);
    CLAIM_CHECK(token, aud, jwt::json::type::string);
    CLAIM_CHECK(token, exp, jwt::json::type::integer);
    CLAIM_CHECK(token, iat, jwt::json::type::integer);
    auto client_id = token.get_payload_claim("client_id").as_string();
    auto it = rule.consumers.find(client_id);
    if (it == rule.consumers.end()) {
      LOG_DEBUG(absl::StrFormat("client_id not found:%s", client_id));
      goto failed;
    }
    auto consumer = it->second;
    auto verifier =
        jwt::verify()
            .allow_algorithm(jwt::algorithm::hs256{consumer.client_secret})
            .with_issuer(rule.issuer)
            .with_subject(consumer.name)
            .with_type(TypeHeader)
            .leeway(rule.clock_skew);
    std::error_code ec;
    verifier.verify(token, ec);
    if (ec) {
      LOG_INFO(absl::StrFormat("token verify failed, token:%s, reason:%s",
                               token_str, ec.message()));
      goto failed;
    }
    verified = true;
    if (allow_set &&
        allow_set.value().find(consumer.name) == allow_set.value().end()) {
      LOG_DEBUG(absl::StrFormat("consumer:%s is not in route's:%s allow_set",
                                consumer.name, route_name));
      goto failed;
    }
    if (!rule.global_credentials) {
      auto audience_json = token.get_payload_claim("aud");
      if (audience_json.get_type() != jwt::json::type::string) {
        LOG_DEBUG(absl::StrFormat("invalid audience, token:%s", token_str));
        goto failed;
      }
      auto audience = audience_json.as_string();
      if (audience != route_name) {
        LOG_DEBUG(absl::StrFormat("audience:%s not match this route:%s",
                                  audience, route_name));
        goto failed;
      }
    }
    if (!rule.keep_token) {
      removeRequestHeader(rule.auth_header_name);
    }
    addRequestHeader("X-Mse-Consumer", consumer.name);
    return true;
  }
failed:
  if (!verified) {
    auto authn_value = absl::StrCat(
        "Bearer realm=\"",
        Wasm::Common::Http::buildOriginalUri(MaximumUriLength), "\"");
    sendLocalResponse(401, kRcDetailOAuthPrefix, "Invalid Jwt token",
                      {{"WWW-Authenticate", authn_value}});
  } else {
    sendLocalResponse(403, kRcDetailOAuthPrefix, "Access Denied", {});
  }
  return false;
}

bool PluginRootContext::onConfigure(size_t size) {
  // Parse configuration JSON string.
  if (size > 0 && !configure(size)) {
    LOG_WARN("configuration has errors initialization will not continue.");
    setInvalidConfig();
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
  auto config = rootCtx->getMatchAuthConfig();
  if (!config.first) {
    return FilterHeadersStatus::Continue;
  }
  config_ = config.first;
  getValue({"route_name"}, &route_name_);
  auto path = getRequestHeader(Wasm::Common::Http::Header::Path)->toString();
  auto params_pos = path.find('?');
  size_t uri_end;
  if (params_pos == std::string::npos) {
    uri_end = path.size();
  } else {
    uri_end = params_pos;
  }
  // Authorize request
  if (absl::EndsWith({path.c_str(), uri_end},
                     config_.value().get().auth_path)) {
    std::string err_msg, token;
    auto method =
        getRequestHeader(Wasm::Common::Http::Header::Method)->toString();
    if (method == "GET") {
      if (params_pos == std::string::npos) {
        err_msg = "Authorize parameters are missing";
        goto done;
      }
      params_pos++;
      rootCtx->generateToken(
          config_.value(), route_name_,
          {path.c_str() + params_pos, path.size() - params_pos}, &token,
          &err_msg);
      goto done;
    }
    if (method == "POST") {
      auto content_type =
          getRequestHeader(Wasm::Common::Http::Header::ContentType)->toString();
      if (!absl::StrContains(absl::AsciiStrToLower(content_type),
                             "application/x-www-form-urlencoded")) {
        err_msg = "Invalid content-type";
        goto done;
      }
      check_body_params_ = true;
    }
  done:
    if (!err_msg.empty()) {
      sendLocalResponse(400, generateRcDetails(err_msg), err_msg, {});
      return FilterHeadersStatus::StopIteration;
    }
    if (!token.empty()) {
      sendLocalResponse(200, "",
                        absl::StrFormat(TokenResponseTemplate, token,
                                        config_.value().get().token_ttl),
                        {{"Content-Type", "application/json"}});
    }
    return FilterHeadersStatus::Continue;
  }
  return rootCtx->checkAuthRule(
             [rootCtx, this](const auto& config, const auto& allow_set) {
               return rootCtx->checkPlugin(config, allow_set, route_name_);
             })
             ? FilterHeadersStatus::Continue
             : FilterHeadersStatus::StopIteration;
}

FilterDataStatus PluginContext::onRequestBody(size_t body_size,
                                              bool end_stream) {
  if (!check_body_params_) {
    return FilterDataStatus::Continue;
  }
  body_total_size_ += body_size;
  if (!end_stream) {
    return FilterDataStatus::StopIterationAndBuffer;
  }
  auto* rootCtx = rootContext();
  auto body =
      getBufferBytes(WasmBufferType::HttpRequestBody, 0, body_total_size_);
  LOG_DEBUG(absl::StrFormat("authorize request body: %s", body->toString()));
  std::string token, err_msg;
  if (rootCtx->generateToken(config_.value(), route_name_, body->view(), &token,
                             &err_msg)) {
    sendLocalResponse(200, "",
                      absl::StrFormat(TokenResponseTemplate, token,
                                      config_.value().get().token_ttl),
                      {{"Content-Type", "application/json"}});
    return FilterDataStatus::Continue;
  }
  sendLocalResponse(400, generateRcDetails(err_msg), err_msg, {});
  return FilterDataStatus::StopIterationNoBuffer;
}

#ifdef NULL_PLUGIN

}  // namespace oauth
}  // namespace null_plugin
}  // namespace proxy_wasm

#endif
