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

#include "extensions/hmac_auth/plugin.h"

#include <algorithm>
#include <array>
#include <chrono>
#include <functional>
#include <optional>
#include <string_view>
#include <utility>
#include <valarray>

#include "absl/strings/str_cat.h"
#include "absl/strings/str_format.h"
#include "absl/strings/str_replace.h"
#include "absl/strings/str_split.h"
#include "common/base64.h"
#include "common/crypto_util.h"
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
namespace hmac_auth {

PROXY_WASM_NULL_PLUGIN_REGISTRY

#endif

static RegisterContextFactory register_HmacAuth(
    CONTEXT_FACTORY(PluginContext), ROOT_FACTORY(PluginRootContext));

static constexpr std::string_view CA_KEY = "x-ca-key";
static constexpr std::string_view CA_SIGNATURE_METHOD = "x-ca-signature-method";
static constexpr std::string_view CA_SIGNATURE_HEADERS =
    "x-ca-signature-headers";
static constexpr std::string_view CA_SIGNATURE = "x-ca-signature";
static constexpr std::string_view CA_ERRMSG = "x-ca-error-message";
static constexpr std::string_view CA_TIMESTAMP = "x-ca-timestamp";

static constexpr size_t MILLISEC_MIN_LENGTH = 13;

static constexpr std::array<std::string_view, 5> CHECK_HEADERS{
    Wasm::Common::Http::Header::Method,
    Wasm::Common::Http::Header::Accept,
    Wasm::Common::Http::Header::ContentMD5,
    Wasm::Common::Http::Header::ContentType,
    Wasm::Common::Http::Header::Date,
};

static constexpr size_t MAX_BODY_SIZE = 32 * 1024 * 1024;

static constexpr int64_t NANO_SECONDS = 1000 * 1000 * 1000;

namespace {

void deniedInvalidCaKey() {
  sendLocalResponse(401, "Invalid Key", "Invalid Key", {});
}

void deniedNoSignature() {
  sendLocalResponse(401, "Empty Signature", "Empty Signature", {});
}

void deniedUnauthorizedConsumer() {
  sendLocalResponse(403, "Unauthorized Consumer", "Unauthorized Consumer", {});
}

void deniedInvalidCredentials(const std::string& errmsg) {
  sendLocalResponse(400, "Invalid Signature", "Invalid Signature",
                    {{std::string(CA_ERRMSG), errmsg}});
}

void deniedInvalidContentMD5() {
  sendLocalResponse(400, "Invalid Content-MD5", "Invalid Content-MD5", {});
}

void deniedInvalidDate() {
  sendLocalResponse(400, "Invalid Date", "Invalid Date", {});
}

void deniedBodyTooLarge() {
  sendLocalResponse(413, "Request Body Too Large", "Request Body Too Large",
                    {});
}

std::string getStringToSign() {
  std::string message;
  for (const auto& header : CHECK_HEADERS) {
    auto header_value = getRequestHeader(header)->toString();
    absl::StrAppendFormat(&message, "%s\n", header_value);
  }

  auto dynamic_check_headers =
      getRequestHeader(CA_SIGNATURE_HEADERS)->toString();
  std::vector<std::string> header_arr;
  for (const auto& header : absl::StrSplit(dynamic_check_headers, ",")) {
    if (header.empty()) {
      continue;
    }
    auto lower_header = absl::AsciiStrToLower(header);
    if (lower_header == CA_SIGNATURE || lower_header == CA_SIGNATURE_HEADERS) {
      continue;
    }
    bool is_static = false;
    for (const auto& h : CHECK_HEADERS) {
      if (h == lower_header) {
        is_static = true;
        break;
      }
    }
    if (!is_static) {
      header_arr.push_back(std::move(lower_header));
    }
  }
  std::sort(header_arr.begin(), header_arr.end());
  for (const auto& header : header_arr) {
    auto header_value = getRequestHeader(header)->toString();
    absl::StrAppendFormat(&message, "%s:%s\n", header, header_value);
  }
  return message;
}

void getStringToSignWithParam(
    std::string* str_to_sign, const std::string& path,
    std::optional<std::reference_wrapper<Wasm::Common::Http::QueryParams>>
        body_params) {
  // need alphabetical order
  auto params =
      Wasm::Common::Http::parseAndDecodeQueryString(std::string(path));
  if (body_params) {
    for (auto&& param : body_params.value().get()) {
      params.emplace(param);
    }
  }
  auto url_path = path.substr(0, path.find('?'));
  absl::StrAppend(str_to_sign, url_path);
  if (params.empty()) {
    return;
  }
  str_to_sign->append("?");
  auto it = params.begin();
  for (; it != std::prev(params.end()); it++) {
    absl::StrAppendFormat(str_to_sign, "%s=%s&", it->first, it->second);
  }
  absl::StrAppendFormat(str_to_sign, "%s=%s", it->first, it->second);
  return;
}

}  // namespace

bool PluginRootContext::parsePluginConfig(const json& configuration,
                                          HmacAuthConfigRule& rule) {
  if ((configuration.find("consumers") != configuration.end()) &&
      (configuration.find("credentials") != configuration.end())) {
    LOG_WARN(
        "The consumers field and the credentials field cannot appear at the "
        "same level");
    return false;
  }
  if (!JsonArrayIterate(
          configuration, "credentials", [&](const json& credential) -> bool {
            auto item = credential.find("key");
            if (item == credential.end()) {
              LOG_WARN("can't find 'key' field in credential.");
              return false;
            }
            auto key = JsonValueAs<std::string>(item.value());
            if (key.second != Wasm::Common::JsonParserResultDetail::OK ||
                !key.first) {
              return false;
            }
            item = credential.find("secret");
            if (item == credential.end()) {
              LOG_WARN("can't find 'secret' field in credential.");
              return false;
            }
            auto secret = JsonValueAs<std::string>(item.value());
            if (secret.second != Wasm::Common::JsonParserResultDetail::OK ||
                !secret.first) {
              return false;
            }
            auto result = rule.credentials.emplace(
                std::make_pair(key.first.value(), secret.first.value()));
            if (!result.second) {
              LOG_WARN(absl::StrCat("duplicate credential key: ",
                                    key.first.value()));
              return false;
            }
            return true;
          })) {
    LOG_WARN("failed to parse configuration for credentials.");
    return false;
  }
  if (!JsonArrayIterate(
          configuration, "consumers", [&](const json& consumer) -> bool {
            auto item = consumer.find("key");
            if (item == consumer.end()) {
              LOG_WARN("can't find 'key' field in consumer.");
              return false;
            }
            auto key = JsonValueAs<std::string>(item.value());
            if (key.second != Wasm::Common::JsonParserResultDetail::OK ||
                !key.first) {
              return false;
            }
            item = consumer.find("secret");
            if (item == consumer.end()) {
              LOG_WARN("can't find 'secret' field in consumer.");
              return false;
            }
            auto secret = JsonValueAs<std::string>(item.value());
            if (secret.second != Wasm::Common::JsonParserResultDetail::OK ||
                !secret.first) {
              return false;
            }
            item = consumer.find("name");
            if (item == consumer.end()) {
              LOG_WARN("can't find 'name' field in consumer.");
              return false;
            }
            auto name = JsonValueAs<std::string>(item.value());
            if (name.second != Wasm::Common::JsonParserResultDetail::OK ||
                !name.first) {
              return false;
            }
            if (rule.credentials.find(key.first.value()) !=
                rule.credentials.end()) {
              LOG_WARN(
                  absl::StrCat("duplicate consumer key: ", key.first.value()));
              return false;
            }
            rule.credentials.emplace(
                std::make_pair(key.first.value(), secret.first.value()));
            rule.key_to_name.emplace(
                std::make_pair(key.first.value(), name.first.value()));
            return true;
          })) {
    LOG_WARN("failed to parse configuration for credentials.");
    return false;
  }
  if (rule.credentials.empty()) {
    LOG_INFO("at least one credential has to be configured for a rule.");
    return false;
  }

  auto it = configuration.find("date_offset");
  if (it != configuration.end()) {
    auto date_offset = JsonValueAs<int64_t>(it.value());
    if (date_offset.second != Wasm::Common::JsonParserResultDetail::OK ||
        !date_offset.first) {
      LOG_WARN("failed to parse 'date_offset' field in configuration.");
      return false;
    }
    rule.date_nano_offset = date_offset.first.value() * NANO_SECONDS;
  }
  return true;
}

bool PluginRootContext::checkConsumer(
    const std::string& ca_key, const HmacAuthConfigRule& rule,
    const std::optional<std::unordered_set<std::string>>& allow_set) {
  if (ca_key.empty()) {
    LOG_DEBUG("empty key");
    deniedInvalidCaKey();
    return false;
  }
  auto credentials_iter = rule.credentials.find(std::string(ca_key));
  if (credentials_iter == rule.credentials.end()) {
    LOG_DEBUG(absl::StrCat("can't find secret through key: ", ca_key));
    deniedInvalidCaKey();
    return false;
  }
  auto key_to_name_iter = rule.key_to_name.find(std::string(ca_key));
  if (key_to_name_iter != rule.key_to_name.end()) {
    if (allow_set) {
      if (allow_set.value().empty()) {
        LOG_DEBUG("allow set is empty, nobody is allowed");
        deniedUnauthorizedConsumer();
        return false;
      }
      if (allow_set.value().find(key_to_name_iter->second) ==
          allow_set.value().end()) {
        LOG_DEBUG(absl::StrCat("consumer is not allowed: ",
                               key_to_name_iter->second));
        deniedUnauthorizedConsumer();
        return false;
      }
    }
    addRequestHeader("X-Mse-Consumer", key_to_name_iter->second);
  }
  return true;
}

bool PluginRootContext::checkPlugin(
    const std::string& ca_key, const std::string& signature,
    const std::string& signature_method, const std::string& path,
    const std::string& date, bool is_timetamp, std::string* sts,
    const HmacAuthConfigRule& rule,
    std::optional<std::reference_wrapper<Wasm::Common::Http::QueryParams>>
        body_params) {
  // Drop any client-supplied value so only this gateway's assertion survives.
  removeRequestHeader("X-Mse-Consumer");
  if (ca_key.empty()) {
    LOG_DEBUG("empty key");
    deniedInvalidCaKey();
    return false;
  }
  if (signature.empty()) {
    LOG_DEBUG("empty signature");
    deniedNoSignature();
    return false;
  }
  int64_t time_offset = 0;
  if (rule.date_nano_offset > 0) {
    auto current_time = getCurrentTimeNanoseconds();
    if (!is_timetamp) {
      auto time_from_date = Wasm::Common::Http::httpTime(date);
      if (!Wasm::Common::Http::timePointValid(time_from_date)) {
        LOG_DEBUG(absl::StrFormat("invalid date format: %s", date));
        deniedInvalidDate();
        return false;
      }
      time_offset = std::abs(
          (long long)(std::chrono::duration_cast<std::chrono::nanoseconds>(
                          time_from_date.time_since_epoch())
                          .count() -
                      current_time));
    } else {
      int64_t timestamp;
      if (!absl::SimpleAtoi(date, &timestamp)) {
        LOG_DEBUG(absl::StrFormat("invalid timestamp format: %s", date));
        deniedInvalidDate();
        return false;
      }
      // milliseconds to nanoseconds
      timestamp *= 1e6;
      // seconds
      if (date.size() < MILLISEC_MIN_LENGTH) {
        timestamp *= 1e3;
      }
      time_offset = std::abs((long long)(timestamp - current_time));
    }
    if (time_offset > rule.date_nano_offset) {
      LOG_DEBUG(absl::StrFormat("date expired, offset is: %u",
                                time_offset / NANO_SECONDS));
      deniedInvalidDate();
      return false;
    }
  }
  std::string hash_type{"sha256"};
  if (signature_method == "HmacSHA1") {
    hash_type = "sha1";
  }
  auto credentials_iter = rule.credentials.find(std::string(ca_key));
  if (credentials_iter == rule.credentials.end()) {
    LOG_DEBUG(absl::StrCat("can't find secret through key: ", ca_key));
    deniedInvalidCaKey();
    return false;
  }
  const auto& secret = credentials_iter->second;
  getStringToSignWithParam(sts, path, body_params);
  const auto& str_to_sign = *sts;
  auto hmac =
      Wasm::Common::Crypto::getShaHmacBase64(hash_type, secret, str_to_sign);
  if (hmac != signature) {
    auto tip = absl::StrReplaceAll(str_to_sign, {{"\n", "#"}});
    LOG_DEBUG(absl::StrCat("invalid signature, stringToSign: ", tip,
                           " signature: ", hmac));
    deniedInvalidCredentials(absl::StrFormat("Server StringToSign:`%s`", tip));
    return false;
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
  ca_key_ = getRequestHeader(CA_KEY)->toString();
  signature_ = getRequestHeader(CA_SIGNATURE)->toString();
  signature_method_ = getRequestHeader(CA_SIGNATURE_METHOD)->toString();
  path_ = getRequestHeader(Wasm::Common::Http::Header::Path)->toString();
  date_ = getRequestHeader(Wasm::Common::Http::Header::Date)->toString();
  str_to_sign_ = getStringToSign();
  body_md5_ =
      getRequestHeader(Wasm::Common::Http::Header::ContentMD5)->toString();
  GET_HEADER_VIEW(Wasm::Common::Http::Header::ContentType, content_type);

  if (date_.empty()) {
    date_ = getRequestHeader(CA_TIMESTAMP)->toString();
    is_timestamp_ = true;
  }
  auto* rootCtx = rootContext();

  auto config = rootCtx->getMatchAuthConfig();
  config_ = config.first;
  if (!config_) {
    LOG_DEBUG("no matched config found");
    return FilterHeadersStatus::Continue;
  }
  allow_set_ = config.second;
  // check if ca_key present in config and it's consumer_name is allowed
  if (!rootCtx->checkConsumer(ca_key_, config_.value(), allow_set_)) {
    return FilterHeadersStatus::StopIteration;
  }

  if (absl::StrContains(absl::AsciiStrToLower(content_type),
                        "application/x-www-form-urlencoded")) {
    check_body_params_ = true;
    return FilterHeadersStatus::Continue;
  }

  return rootCtx->checkPlugin(ca_key_, signature_, signature_method_, path_,
                              date_, is_timestamp_, &str_to_sign_,
                              config_.value(), std::nullopt)
             ? FilterHeadersStatus::Continue
             : FilterHeadersStatus::StopIteration;
}

FilterDataStatus PluginContext::onRequestBody(size_t body_size,
                                              bool end_stream) {
  if (!config_) {
    return FilterDataStatus::Continue;
  }
  if (body_md5_.empty() && !check_body_params_) {
    return FilterDataStatus::Continue;
  }
  body_total_size_ += body_size;
  if (body_total_size_ > MAX_BODY_SIZE) {
    LOG_DEBUG("body_size is too large");
    deniedBodyTooLarge();
    return FilterDataStatus::StopIterationNoBuffer;
  }
  if (!end_stream) {
    return FilterDataStatus::StopIterationAndBuffer;
  }
  auto body =
      getBufferBytes(WasmBufferType::HttpRequestBody, 0, body_total_size_);
  LOG_DEBUG("body: " + body->toString());
  if (!body_md5_.empty()) {
    if (body->size() == 0) {
      LOG_DEBUG("got empty body");
      deniedInvalidContentMD5();
      return FilterDataStatus::StopIterationNoBuffer;
    }
    auto md5 = Wasm::Common::Crypto::getMD5Base64(body->view());
    if (md5 != body_md5_) {
      LOG_DEBUG(
          absl::StrFormat("body md5 expect: %s, actual: %s", body_md5_, md5));
      deniedInvalidContentMD5();
      return FilterDataStatus::StopIterationNoBuffer;
    }
  }
  if (check_body_params_) {
    auto body_params = Wasm::Common::Http::parseFromBody(body->view());
    auto* rootCtx = rootContext();
    return rootCtx->checkPlugin(ca_key_, signature_, signature_method_, path_,
                                date_, is_timestamp_, &str_to_sign_,
                                config_.value(), body_params)
               ? FilterDataStatus::Continue
               : FilterDataStatus::StopIterationNoBuffer;
  }
  return FilterDataStatus::Continue;
}

#ifdef NULL_PLUGIN

}  // namespace hmac_auth
}  // namespace null_plugin
}  // namespace proxy_wasm

#endif
