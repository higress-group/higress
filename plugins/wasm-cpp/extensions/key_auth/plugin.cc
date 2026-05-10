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

#include "extensions/key_auth/plugin.h"

#include <array>

#include "absl/strings/str_cat.h"
#include "absl/strings/str_split.h"
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
namespace key_auth {

PROXY_WASM_NULL_PLUGIN_REGISTRY

#endif

static RegisterContextFactory register_KeyAuth(CONTEXT_FACTORY(PluginContext),
                                               ROOT_FACTORY(PluginRootContext));

namespace {

const std::string OriginalAuthKey("X-HI-ORIGINAL-AUTH");

void deniedInvalidCredentials(const std::string& realm) {
  sendLocalResponse(401, "Request denied by Key Auth check. Invalid API key", "",
                    {{"WWW-Authenticate", absl::StrCat("Key realm=", realm)}});
}

void deniedUnauthorizedConsumer(const std::string& realm) {
  sendLocalResponse(403, "Request denied by Key Auth check. Unauthorized consumer", "",
                    {{"WWW-Authenticate", absl::StrCat("Key realm=", realm)}});
}

std::string maskCredential(const std::string& credential) {
  if (credential.size() <= 3) {
    return "***";
  }
  return credential.substr(0, 3) + "***";
}

}  // namespace

bool PluginRootContext::parsePluginConfig(const json& configuration,
                                          KeyAuthConfigRule& rule) {
  if ((configuration.find("consumers") != configuration.end()) &&
      (configuration.find("credentials") != configuration.end())) {
    LOG_WARN("The consumers field and the credentials field cannot appear at the same level");
    return false;
  }
  if ((configuration.find("consumers") == configuration.end()) &&
      (configuration.find("credentials") == configuration.end())) {
    LOG_WARN("No consumers and no credentials");
    return false;
  }
  if (configuration.find("credentials") != configuration.end()) {
    if (!JsonArrayIterate(
          configuration, "credentials", [&](const json& credentials) -> bool {
            auto credential = JsonValueAs<std::string>(credentials);
            if (credential.second != Wasm::Common::JsonParserResultDetail::OK) {
              return false;
            }
            rule.credentials.insert(credential.first.value());
            return true;
          })) {
      LOG_WARN("failed to parse configuration for credentials.");
      return false;
    }
    if (!JsonArrayIterate(configuration, "keys", [&](const json& item) -> bool {
          auto key = JsonValueAs<std::string>(item);
          if (key.second != Wasm::Common::JsonParserResultDetail::OK) {
            return false;
          }
          rule.keys.push_back(key.first.value());
          return true;
        })) {
      LOG_WARN("failed to parse configuration for keys.");
      return false;
    }
    if (rule.keys.empty()) {
      LOG_WARN("at least one key has to be configured for a rule.");
      return false;
    }
    rule.keys.push_back(OriginalAuthKey);
    auto it = configuration.find("realm");
    if (it != configuration.end()) {
      auto realm_string = JsonValueAs<std::string>(it.value());
      if (realm_string.second != Wasm::Common::JsonParserResultDetail::OK) {
        LOG_WARN("failed to parse 'realm' field in filter configuration.");
        return false;
      }
      rule.realm = realm_string.first.value();
    }
    it = configuration.find("in_query");
    if (it != configuration.end()) {
      auto in_query = JsonValueAs<bool>(it.value());
      if (in_query.second != Wasm::Common::JsonParserResultDetail::OK ||
          !in_query.first) {
        LOG_WARN("failed to parse 'in_query' field in filter configuration.");
        return false;
      }
      rule.in_query = in_query.first.value();
    }
    it = configuration.find("in_header");
    if (it != configuration.end()) {
      auto in_header = JsonValueAs<bool>(it.value());
      if (in_header.second != Wasm::Common::JsonParserResultDetail::OK ||
          !in_header.first) {
        LOG_WARN("failed to parse 'in_header' field in filter configuration.");
        return false;
      }
      rule.in_header = in_header.first.value();
    }
    if (!rule.in_query && !rule.in_header) {
      LOG_WARN("at least one of 'in_query' and 'in_header' must set to true");
      return false;
    }
    // LOG_DEBUG(rule.debugString("parse phase, credentials branch"));
  }
  if (configuration.find("consumers") != configuration.end()) {
    bool need_global_keys = false;
    if (!JsonArrayIterate(
        configuration, "consumers", [&](const json& consumer) -> bool {
          Consumer c;
          auto item = consumer.find("name");
          if (item == consumer.end()) {
            LOG_WARN("can't find 'name' field in consumer.");
            return false;
          }
          auto name = JsonValueAs<std::string>(item.value());
          if (name.second != Wasm::Common::JsonParserResultDetail::OK ||
              !name.first) {
            return false;
          }
          c.name = name.first.value();
          if (consumer.find("credential") != consumer.end() &&
              consumer.find("credentials") != consumer.end()) {
            LOG_WARN("'credential' and 'credentials' can't appear at the same time.");
            return false;
          }
          if (consumer.find("credential") == consumer.end() &&
              consumer.find("credentials") == consumer.end()) {
            LOG_WARN("at least one of 'credential' and 'credentials' should be set.");
            return false;
          }
          item = consumer.find("credential");
          if (item != consumer.end()) {
            auto credential = JsonValueAs<std::string>(item.value());
            if (credential.second != Wasm::Common::JsonParserResultDetail::OK ||
                !credential.first) {
              return false;
            }
            c.credentials.insert(credential.first.value());
            if (rule.credential_to_name.find(credential.first.value()) !=
                rule.credential_to_name.end()) {
              LOG_WARN(absl::StrCat("duplicate consumer credential: ",
                                    credential.first.value()));
              return false;
            }
            rule.credentials.insert(credential.first.value());
            rule.credential_to_name.emplace(
                std::make_pair(credential.first.value(), name.first.value()));
          }
          item = consumer.find("credentials");
          if (item != consumer.end()) {
              if (!JsonArrayIterate(
                    consumer, "credentials", [&](const json& credential_json) -> bool {
                      auto credential = JsonValueAs<std::string>(credential_json);
                      if (credential.second != Wasm::Common::JsonParserResultDetail::OK ||
                          !credential.first) {
                        return false;
                      }
                      c.credentials.insert(credential.first.value());
                      if (rule.credential_to_name.find(credential.first.value()) !=
                          rule.credential_to_name.end()) {
                        LOG_WARN(absl::StrCat("duplicate consumer credential: ",
                                              credential.first.value()));
                        return false;
                      }
                      rule.credentials.insert(credential.first.value());
                      rule.credential_to_name.emplace(
                          std::make_pair(credential.first.value(), name.first.value()));
                      return true;
                    })) {
                LOG_WARN(absl::StrCat("failed to parse credentials for consumer: ", c.name));
                return false;
              }
          }
          item = consumer.find("keys");
          if (item == consumer.end()) {
            LOG_DEBUG("not found keys configuration for consumer " + c.name + ", will use global configuration to extract keys");
            need_global_keys = true;
          } else {
            c.keys = std::vector<std::string>{OriginalAuthKey};
            if (!JsonArrayIterate(
                  consumer, "keys", [&](const json& key_json) -> bool {
                    auto key = JsonValueAs<std::string>(key_json);
                    if (key.second !=
                        Wasm::Common::JsonParserResultDetail::OK) {
                      return false;
                    }
                    c.keys->push_back(key.first.value());
                    return true;
                  })) {
              LOG_WARN("failed to parse configuration for consumer keys.");
              return false;
            }
            item = consumer.find("in_query");
            if (item != consumer.end()) {
              auto in_query = JsonValueAs<bool>(item.value());
              if (in_query.second != Wasm::Common::JsonParserResultDetail::OK || !in_query.first) {
                LOG_WARN("failed to parse 'in_query' field in consumer configuration.");
                return false;
              }
              c.in_query = in_query.first;
            }
            item = consumer.find("in_header");
            if (item != consumer.end()) {
              auto in_header = JsonValueAs<bool>(item.value());
              if (in_header.second !=
                      Wasm::Common::JsonParserResultDetail::OK ||
                  !in_header.first) {
                LOG_WARN(
                    "failed to parse 'in_header' field in consumer "
                    "configuration.");
                return false;
              }
              c.in_header = in_header.first;
            }
          }
          rule.consumers.push_back(std::move(c));
          return true;
        })) {
      LOG_WARN("failed to parse configuration for credentials.");
      return false;
    }
    if (need_global_keys) {
      if (!JsonArrayIterate(configuration, "keys", [&](const json& item) -> bool {
            auto key = JsonValueAs<std::string>(item);
            if (key.second != Wasm::Common::JsonParserResultDetail::OK) {
              return false;
            }
            rule.keys.push_back(key.first.value());
            return true;
          })) {
        LOG_WARN("failed to parse configuration for keys.");
        return false;
      }
      auto it = configuration.find("in_query");
      if (it != configuration.end()) {
        auto in_query = JsonValueAs<bool>(it.value());
        if (in_query.second != Wasm::Common::JsonParserResultDetail::OK ||
            !in_query.first) {
          LOG_WARN("failed to parse 'in_query' field in filter configuration.");
          return false;
        }
        rule.in_query = in_query.first.value();
      }
      it = configuration.find("in_header");
      if (it != configuration.end()) {
        auto in_header = JsonValueAs<bool>(it.value());
        if (in_header.second != Wasm::Common::JsonParserResultDetail::OK ||
            !in_header.first) {
          LOG_WARN("failed to parse 'in_header' field in filter configuration.");
          return false;
        }
        rule.in_header = in_header.first.value();
      }
      if (!rule.in_query && !rule.in_header) {
        LOG_WARN("at least one of 'in_query' and 'in_header' must set to true");
        return false;
      }
    }
    // LOG_DEBUG(rule.debugString("parse phase, consumers branch"));
  }
  return true;
}

// Helper: check allow_set and return true if allowed, false if denied
static bool checkAllowSet(
    const std::optional<std::unordered_set<std::string>>& allow_set,
    const std::string& consumer_name, const std::string& realm) {
  if (allow_set) {
    if (allow_set->empty()) {
      LOG_DEBUG("allow set is empty, nobody is allowed");
      deniedUnauthorizedConsumer(realm);
      return false;
    }
    if (allow_set->find(consumer_name) == allow_set->end()) {
      deniedUnauthorizedConsumer(realm);
      LOG_DEBUG("unauthorized consumer: " + consumer_name);
      return false;
    }
  }
  return true;
}

// Helper: check if a consumer has per-consumer key configuration
static bool hasPerConsumerKeyConfig(const Consumer& consumer) {
  return consumer.keys.has_value() || consumer.in_header.has_value() ||
         consumer.in_query.has_value();
}

bool PluginRootContext::checkPlugin(
    const KeyAuthConfigRule& rule,
    const std::optional<std::unordered_set<std::string>>& allow_set) {
  // LOG_DEBUG(rule.debugString("check phase"));
  if (rule.consumers.empty()) {
    for (const auto& key : rule.keys) {
      auto credential = extractCredential(rule.in_header, rule.in_query, key);
      if (credential.empty()) {
        LOG_DEBUG("empty credential for key: " + key);
        continue;
      }

      auto auth_credential_iter = rule.credentials.find(credential);
      if (auth_credential_iter == rule.credentials.end()) {
        LOG_DEBUG("api key not found: " + maskCredential(credential));
        continue;
      }

      auto credential_to_name_iter = rule.credential_to_name.find(credential);
      if (credential_to_name_iter != rule.credential_to_name.end()) {
        addRequestHeader("X-Mse-Consumer", credential_to_name_iter->second);
        if (!checkAllowSet(allow_set, credential_to_name_iter->second,
                           rule.realm)) {
          return false;
        }
      }
      return true;
    }
  } else {
    // Collect consumers that have per-consumer key configuration
    std::vector<const Consumer*> perConsumerKeyConsumers;
    for (const auto& consumer : rule.consumers) {
      if (hasPerConsumerKeyConfig(consumer)) {
        perConsumerKeyConsumers.push_back(&consumer);
      }
    }

    // Fast path: no per-consumer key consumers, use O(1) credential_to_name
    // lookup
    if (perConsumerKeyConsumers.empty()) {
      for (const auto& key : rule.keys) {
        auto credential = extractCredential(rule.in_header, rule.in_query, key);
        if (credential.empty()) {
          LOG_DEBUG("empty credential for key: " + key);
          continue;
        }

        auto iter = rule.credential_to_name.find(credential);
        if (iter == rule.credential_to_name.end()) {
          LOG_DEBUG("api key not found: " + maskCredential(credential));
          continue;
        }

        addRequestHeader("X-Mse-Consumer", iter->second);
        if (!checkAllowSet(allow_set, iter->second, rule.realm)) {
          return false;
        }
        return true;
      }
      LOG_DEBUG("No valid credentials were found (fast path).");
      deniedInvalidCredentials(rule.realm);
      return false;
    }

    // Build a set of per-consumer key consumer names for quick lookup
    std::unordered_set<std::string> perConsumerNames;
    for (const auto* consumer : perConsumerKeyConsumers) {
      perConsumerNames.insert(consumer->name);
    }

    // Try O(1) fast path first for consumers without per-consumer keys
    for (const auto& key : rule.keys) {
      auto credential = extractCredential(rule.in_header, rule.in_query, key);
      if (credential.empty()) {
        LOG_DEBUG("empty credential for key: " + key);
        continue;
      }

      auto iter = rule.credential_to_name.find(credential);
      if (iter == rule.credential_to_name.end()) {
        LOG_DEBUG("api key not found: " + maskCredential(credential));
        continue;
      }

      // Skip if this credential belongs to a per-consumer key consumer
      if (perConsumerNames.count(iter->second) > 0) {
        continue;
      }

      addRequestHeader("X-Mse-Consumer", iter->second);
      if (!checkAllowSet(allow_set, iter->second, rule.realm)) {
        return false;
      }
      return true;
    }

    // Slow path: check per-consumer key consumers with cached credentials
    std::unordered_map<std::string, std::string> credentialCache;
    for (const auto* consumer : perConsumerKeyConsumers) {
      std::vector<std::string> keys_to_check =
          consumer->keys.value_or(rule.keys);
      bool in_query = consumer->in_query.value_or(rule.in_query);
      bool in_header = consumer->in_header.value_or(rule.in_header);

      for (const auto& key : keys_to_check) {
        // Cache key includes extraction source to avoid cross-contamination
        // between consumers with different in_header/in_query settings
        std::string cache_key = absl::StrCat(
            key, ":", in_header ? "h" : "", in_query ? "q" : "");
        if (credentialCache.find(cache_key) == credentialCache.end()) {
          credentialCache[cache_key] =
              extractCredential(in_header, in_query, key);
        }
        const auto& credential = credentialCache[cache_key];
        if (credential.empty()) {
          LOG_DEBUG("empty credential for key: " + key);
          continue;
        }

        if (consumer->credentials.find(credential) ==
            consumer->credentials.end()) {
          LOG_DEBUG("credential " + maskCredential(credential) +
                    " does not match the consumer " + consumer->name);
          continue;
        }

        auto iter = rule.credential_to_name.find(credential);
        if (iter == rule.credential_to_name.end()) {
          LOG_DEBUG("api key not found: " + maskCredential(credential));
          continue;
        }

        addRequestHeader("X-Mse-Consumer", iter->second);
        if (!checkAllowSet(allow_set, iter->second, rule.realm)) {
          return false;
        }
        return true;
      }
    }

    LOG_DEBUG("No valid credentials were found after checking all consumers.");
    deniedInvalidCredentials(rule.realm);
    return false;
  }

  LOG_DEBUG("No valid credentials were found after checking all keys.");
  deniedInvalidCredentials(rule.realm);
  return false;
}

std::string PluginRootContext::extractCredential(bool in_header, bool in_query,
                                                 const std::string& key) const {
  if (in_header) {
    auto header = getRequestHeader(key);
    if (header->size() != 0) {
      return header->toString();
    }
  }
  if (in_query) {
    auto request_path_header = getRequestHeader(":path");
    auto path = request_path_header->view();
    auto params = Wasm::Common::Http::parseAndDecodeQueryString(path);
    auto it = params.find(key);
    if (it != params.end()) {
      return it->second;
    }
  }
  return "";
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

}  // namespace key_auth
}  // namespace null_plugin
}  // namespace proxy_wasm

#endif
