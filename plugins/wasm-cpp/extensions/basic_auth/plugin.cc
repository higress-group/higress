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

#include "extensions/basic_auth/plugin.h"

#include <array>
#include <functional>
#include <optional>
#include <utility>

#include "absl/strings/str_cat.h"
#include "absl/strings/str_split.h"
#include "common/base64.h"
#include "common/common_util.h"
#include "common/crypto_util.h"
#include "common/json_util.h"

using ::nlohmann::json;
using ::Wasm::Common::JsonArrayIterate;
using ::Wasm::Common::JsonGetField;
using ::Wasm::Common::JsonObjectIterate;
using ::Wasm::Common::JsonValueAs;

#ifdef NULL_PLUGIN

namespace proxy_wasm {
namespace null_plugin {
namespace basic_auth {

PROXY_WASM_NULL_PLUGIN_REGISTRY

NullPluginRegistry* context_registry_;
RegisterNullVmPluginFactory register_basic_auth_plugin(
    "envoy.wasm.basic_auth", []() {
      return std::make_unique<NullPlugin>(basic_auth::context_registry_);
    });

#endif

static RegisterContextFactory register_BasicAuth(
    CONTEXT_FACTORY(PluginContext), ROOT_FACTORY(PluginRootContext));

namespace {

void deniedNoBasicAuthData(const std::string& realm) {
  sendLocalResponse(
      401,
      "Request denied by Basic Auth check. No Basic "
      "Authentication information found.",
      "", {{"WWW-Authenticate", absl::StrCat("Basic realm=", realm)}});
}

void deniedInvalidCredentials(const std::string& realm) {
  sendLocalResponse(
      401,
      "Request denied by Basic Auth check. Invalid "
      "username and/or password",
      "", {{"WWW-Authenticate", absl::StrCat("Basic realm=", realm)}});
}

void deniedUnauthorizedConsumer(const std::string& realm) {
  sendLocalResponse(
      403, "Request denied by Basic Auth check. Unauthorized consumer", "",
      {{"WWW-Authenticate", absl::StrCat("Basic realm=", realm)}});
}

}  // namespace

bool PluginRootContext::parsePluginConfig(const json& configuration,
                                          BasicAuthConfigRule& rule) {
  if ((configuration.find("consumers") != configuration.end()) &&
      (configuration.find("credentials") != configuration.end())) {
    LOG_WARN(
        "The consumers field and the credentials field cannot appear at the "
        "same level");
    return false;
  }
  auto it = configuration.find("encrypted");
  if (it != configuration.end()) {
    auto passwd_encrypted = JsonValueAs<bool>(it.value());
    if (passwd_encrypted.second != Wasm::Common::JsonParserResultDetail::OK) {
      LOG_WARN("cannot parse passwd_encrypted");
      return false;
    }
    rule.passwd_encrypted = passwd_encrypted.first.value();
  }
  // no consumer name
  if (!JsonArrayIterate(
          configuration, "credentials", [&](const json& credentials) -> bool {
            auto credential = JsonValueAs<std::string>(credentials);
            if (credential.second != Wasm::Common::JsonParserResultDetail::OK) {
              LOG_WARN("credential cannot be parsed");
              return false;
            }
            // Check if credential has `:` in it. If it has, it needs to be
            // base64 encoded.
            if (absl::StrContains(credential.first.value(), ":")) {
              return addBasicAuthConfigRule(rule, credential.first.value(),
                                            std::nullopt, false);
            }
            if (rule.passwd_encrypted) {
              LOG_WARN("colon not found in encrypted credential");
              return false;
            }
            // Otherwise, try base64 decode and insert into credential list if
            // it can be decoded.
            if (!Base64::decodeWithoutPadding(credential.first.value())
                     .empty()) {
              return addBasicAuthConfigRule(rule, credential.first.value(),
                                            std::nullopt, true);
            }
            return false;
          })) {
    LOG_WARN("failed to parse configuration for credentials.");
    return false;
  }
  // with consumer name
  if (!JsonArrayIterate(
          configuration, "consumers", [&](const json& consumer) -> bool {
            auto item = consumer.find("name");
            if (item == consumer.end()) {
              LOG_WARN("can't find 'name' field in consumer.");
              return false;
            }
            auto name = JsonValueAs<std::string>(item.value());
            if (name.second != Wasm::Common::JsonParserResultDetail::OK ||
                !name.first) {
              LOG_WARN("'name' cannot be parsed");
              return false;
            }
            item = consumer.find("credential");
            if (item == consumer.end()) {
              LOG_WARN("can't find 'credential' field in consumer.");
              return false;
            }
            auto credential = JsonValueAs<std::string>(item.value());
            if (credential.second != Wasm::Common::JsonParserResultDetail::OK ||
                !credential.first) {
              LOG_WARN("field 'credential' cannot be parsed");
              return false;
            }
            // Check if credential has `:` in it. If it has, it needs to be
            // base64 encoded.
            if (absl::StrContains(credential.first.value(), ":")) {
              return addBasicAuthConfigRule(rule, credential.first.value(),
                                            name.first, false);
            }
            if (rule.passwd_encrypted) {
              LOG_WARN("colon not found in encrypted credential");
              return false;
            }
            // Otherwise, try base64 decode and insert into credential list if
            // it can be decoded.
            if (!Base64::decodeWithoutPadding(credential.first.value())
                     .empty()) {
              return addBasicAuthConfigRule(rule, credential.first.value(),
                                            name.first, true);
            }
            return false;
          })) {
    LOG_WARN("failed to parse configuration for credentials.");
    return false;
  }
  if (rule.encoded_credentials.empty() && rule.encrypted_credentials.empty()) {
    LOG_INFO("at least one credential has to be configured for a rule.");
    return false;
  }
  it = configuration.find("realm");
  if (it != configuration.end()) {
    auto realm_string = JsonValueAs<std::string>(it.value());
    if (realm_string.second != Wasm::Common::JsonParserResultDetail::OK) {
      LOG_WARN("cannot parse realm");
      return false;
    }
    rule.realm = realm_string.first.value();
  }
  return true;
}

bool PluginRootContext::addBasicAuthConfigRule(
    BasicAuthConfigRule& rule, const std::string& credential,
    const std::optional<std::string>& name, bool base64_encoded) {
  std::string stored_str;
  const std::string* stored_ptr = nullptr;
  if (!base64_encoded && !rule.passwd_encrypted) {
    stored_str = Base64::encode(credential.data(), credential.size());
    stored_ptr = &stored_str;
  } else {
    stored_ptr = &credential;
  }
  if (!rule.passwd_encrypted) {
    rule.encoded_credentials.insert(*stored_ptr);
  } else {
    std::vector<std::string> pair =
        absl::StrSplit(*stored_ptr, absl::MaxSplits(":", 2));
    if (pair.size() != 2) {
      LOG_WARN(absl::StrCat("invalid encrypted credential: ", *stored_ptr));
      return false;
    }
    rule.encrypted_credentials.emplace(
        std::make_pair(std::move(pair[0]), std::move(pair[1])));
  }
  if (name) {
    if (rule.credential_to_name.find(*stored_ptr) !=
        rule.credential_to_name.end()) {
      LOG_WARN(absl::StrCat("duplicate consumer credential: ", *stored_ptr));
      return false;
    }
    rule.credential_to_name.emplace(std::make_pair(*stored_ptr, name.value()));
  }
  return true;
}

bool PluginRootContext::checkPlugin(
    const BasicAuthConfigRule& rule,
    const std::optional<std::unordered_set<std::string>>& allow_set) {
  // Drop any client-supplied value so only this gateway's assertion survives.
  removeRequestHeader("X-Mse-Consumer");
  auto authorization_header = getRequestHeader("authorization");
  auto authorization = authorization_header->view();
  // Check if the Basic auth header starts with "Basic "
  if (!absl::StartsWith(Wasm::Common::stdToAbsl(authorization), "Basic ")) {
    deniedNoBasicAuthData(rule.realm);
    return false;
  }
  auto authorization_strip =
      absl::StripPrefix(Wasm::Common::stdToAbsl(authorization), "Basic ");

  std::string to_find_name;
  if (!rule.passwd_encrypted) {
    auto auth_credential_iter =
        rule.encoded_credentials.find(std::string(authorization_strip));
    // Check if encoded credential is part of the credential_to_name
    // map from our container to grant or deny access.
    if (auth_credential_iter == rule.encoded_credentials.end()) {
      deniedInvalidCredentials(rule.realm);
      return false;
    }
    to_find_name = std::string(authorization_strip);
  } else {
    auto user_and_passwd = Base64::decodeWithoutPadding(
        Wasm::Common::abslToStd(authorization_strip));
    if (user_and_passwd.empty()) {
      LOG_WARN(
          absl::StrCat("invalid base64 authorization: ", authorization_strip));
      deniedInvalidCredentials(rule.realm);
      return false;
    }
    std::vector<std::string> pair =
        absl::StrSplit(user_and_passwd, absl::MaxSplits(":", 2));
    if (pair.size() != 2) {
      LOG_WARN(
          absl::StrCat("invalid decoded authorization: ", user_and_passwd));
      deniedInvalidCredentials(rule.realm);
      return false;
    }
    auto encrypted_iter = rule.encrypted_credentials.find(pair[0]);
    if (encrypted_iter == rule.encrypted_credentials.end()) {
      LOG_DEBUG(absl::StrCat("username not found: ", pair[0]));
      deniedInvalidCredentials(rule.realm);
      return false;
    }
    auto expect_encrypted = encrypted_iter->second;
    std::string actual_encrypted;
    if (!Wasm::Common::Crypto::crypt(pair[1], expect_encrypted,
                                     actual_encrypted)) {
      LOG_DEBUG(absl::StrCat("crypt failed, expect: ", pair[1]));
      deniedInvalidCredentials(rule.realm);
      return false;
    }
    LOG_DEBUG(absl::StrCat("expect_encrypted: ", expect_encrypted,
                           ", actual_encrypted: ", actual_encrypted));
    if (expect_encrypted != actual_encrypted) {
      LOG_DEBUG(absl::StrCat("invalid encrypted: ", actual_encrypted,
                             ", expect: ", expect_encrypted));
      deniedInvalidCredentials(rule.realm);
      return false;
    }
    to_find_name = absl::StrCat(pair[0], ":", expect_encrypted);
  }

  // Check if this credential has a consumer name. If so, check if this
  // consumer is allowed to access. If allow_set is empty, allow all consumers.
  auto credential_to_name_iter = rule.credential_to_name.find(to_find_name);
  if (credential_to_name_iter != rule.credential_to_name.end()) {
    if (allow_set && !allow_set.value().empty()) {
      if (allow_set.value().find(credential_to_name_iter->second) ==
          allow_set.value().end()) {
        deniedUnauthorizedConsumer(rule.realm);
        return false;
      }
    }
    addRequestHeader("X-Mse-Consumer", credential_to_name_iter->second);
  }
  return true;
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
                          Wasm::Common::stdToAbsl(configuration_data->view())));
    return false;
  }
  if (!parseAuthRuleConfig(result.value())) {
    LOG_WARN(absl::StrCat("cannot parse plugin configuration JSON string: ",
                          Wasm::Common::stdToAbsl(configuration_data->view())));
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

}  // namespace basic_auth
}  // namespace null_plugin
}  // namespace proxy_wasm

#endif
