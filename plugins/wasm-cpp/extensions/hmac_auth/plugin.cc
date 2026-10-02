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
#include "absl/strings/str_join.h"
#include "absl/strings/str_replace.h"
#include "absl/strings/str_split.h"
#include "absl/strings/strip.h"
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
static constexpr std::string_view CA_SIGNED_CONTENT_TYPE =
    "x-ca-signed-content-type";

static constexpr size_t MILLISEC_MIN_LENGTH = 13;

static constexpr std::array<std::string_view, 5> CHECK_HEADERS{
    Wasm::Common::Http::Header::Method,
    Wasm::Common::Http::Header::Accept,
    Wasm::Common::Http::Header::ContentMD5,
    Wasm::Common::Http::Header::ContentType,
    Wasm::Common::Http::Header::Date,
};

constexpr std::string_view SetDecoderBufferLimitKey =
    "set_decoder_buffer_limit";
constexpr std::string_view DefaultMaxBodyBytes = "33554432";
static constexpr int64_t NANO_SECONDS = 1000 * 1000 * 1000;

namespace {

int hexDigitValue(char digit) {
  if (digit >= '0' && digit <= '9') {
    return digit - '0';
  }
  if (digit >= 'a' && digit <= 'f') {
    return digit - 'a' + 10;
  }
  if (digit >= 'A' && digit <= 'F') {
    return digit - 'A' + 10;
  }
  return -1;
}

bool isUtf8ContinuationByte(uint8_t byte) {
  return (byte & 0xc0) == 0x80;
}

bool isValidUtf8(std::string_view value) {
  const auto* bytes = reinterpret_cast<const uint8_t*>(value.data());
  size_t index = 0;
  while (index < value.size()) {
    const uint8_t byte = bytes[index];
    if (byte <= 0x7f) {
      ++index;
      continue;
    }

    if (byte >= 0xc2 && byte <= 0xdf) {
      if (index + 1 >= value.size() ||
          !isUtf8ContinuationByte(bytes[index + 1])) {
        return false;
      }
      index += 2;
      continue;
    }

    if (byte >= 0xe0 && byte <= 0xef) {
      if (index + 2 >= value.size() ||
          !isUtf8ContinuationByte(bytes[index + 2])) {
        return false;
      }
      const uint8_t second_byte = bytes[index + 1];
      if ((byte == 0xe0 && (second_byte < 0xa0 || second_byte > 0xbf)) ||
          (byte == 0xed && (second_byte < 0x80 || second_byte > 0x9f)) ||
          ((byte != 0xe0 && byte != 0xed) &&
           !isUtf8ContinuationByte(second_byte))) {
        return false;
      }
      index += 3;
      continue;
    }

    if (byte >= 0xf0 && byte <= 0xf4) {
      if (index + 3 >= value.size() ||
          !isUtf8ContinuationByte(bytes[index + 2]) ||
          !isUtf8ContinuationByte(bytes[index + 3])) {
        return false;
      }
      const uint8_t second_byte = bytes[index + 1];
      if ((byte == 0xf0 && (second_byte < 0x90 || second_byte > 0xbf)) ||
          (byte == 0xf4 && (second_byte < 0x80 || second_byte > 0x8f)) ||
          ((byte != 0xf0 && byte != 0xf4) &&
           !isUtf8ContinuationByte(second_byte))) {
        return false;
      }
      index += 4;
      continue;
    }

    return false;
  }
  return true;
}

// The old API Gateway used Undertow 2.2.23 with UTF-8 URL decoding enabled,
// encoded slash decoding disabled, and form decoding disabled for paths. This
// is equivalent to URLUtils.decode(path, "UTF-8", false, false, buffer) for
// valid UTF-8 paths produced by the old SDK.
std::optional<std::string> decodeOldApiGatewayPath(
    std::string_view encoded_path) {
  std::string decoded_path;
  decoded_path.reserve(encoded_path.size());
  for (size_t index = 0; index < encoded_path.size(); ++index) {
    if (encoded_path[index] != '%') {
      // Path decoding is not form decoding, so a raw '+' stays '+'.
      decoded_path.push_back(encoded_path[index]);
      continue;
    }
    if (index + 2 >= encoded_path.size()) {
      return std::nullopt;
    }

    const int high = hexDigitValue(encoded_path[index + 1]);
    const int low = hexDigitValue(encoded_path[index + 2]);
    if (high < 0 || low < 0) {
      return std::nullopt;
    }

    const char decoded = static_cast<char>((high << 4) | low);
    if (decoded == '/' || decoded == '\\') {
      // Undertow keeps the original spelling, including hexadecimal case.
      decoded_path.append(encoded_path.substr(index, 3));
    } else {
      decoded_path.push_back(decoded);
    }
    index += 2;
  }

  // The legacy SDK always encodes paths as UTF-8. Do not create a fallback
  // candidate for malformed byte sequences that it could not have produced.
  if (!isValidUtf8(decoded_path)) {
    return std::nullopt;
  }
  return decoded_path;
}

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

void replaceConsumerGroupHeader(
    const std::optional<std::vector<std::string>>& groups) {
  if (!groups.has_value() || groups->empty()) {
    replaceRequestHeader(ConsumerGroupHeader, "");
    return;
  }
  replaceRequestHeader(ConsumerGroupHeader, absl::StrJoin(*groups, ","));
}

std::optional<std::string_view> findRequestHeaderValue(
    const RequestHeaderPairs& request_headers, std::string_view expected_name) {
  for (const auto& [name, value] : request_headers) {
    if (absl::EqualsIgnoreCase(name, expected_name)) {
      return value;
    }
  }
  return std::nullopt;
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

// just for forward compatibility
std::string getStringToSignOld() {
  std::string message;
  for (const auto& header : CHECK_HEADERS) {
    auto header_value = getRequestHeader(header)->toString();
    absl::StrAppendFormat(&message, "%s\n", header_value);
  }

  auto dynamic_check_headers =
      getRequestHeader(CA_SIGNATURE_HEADERS)->toString();
  std::vector<std::string> header_arr;
  for (const auto& header : absl::StrSplit(dynamic_check_headers, ",")) {
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
        body_params,
    std::optional<std::string_view> canonical_path_override = std::nullopt) {
  // need alphabetical order
  auto params =
      Wasm::Common::Http::parseAndDecodeQueryString(std::string(path));
  if (body_params) {
    for (auto&& param : body_params.value().get()) {
      params.emplace(param);
    }
  }
  const size_t query_start = path.find('?');
  const std::string_view raw_path =
      std::string_view(path).substr(0, query_start);
  str_to_sign->append(canonical_path_override.value_or(raw_path));
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

std::string buildOldApiGatewayStringToSign(
    const RequestHeaderPairs& request_headers, const std::string& path,
    std::string_view raw_form_body, bool use_compat_params = false,
    std::optional<std::string_view> canonical_path_override = std::nullopt) {
  std::string string_to_sign;
  const auto signed_content_type =
      findRequestHeaderValue(request_headers, CA_SIGNED_CONTENT_TYPE);
  for (const auto& header_name : CHECK_HEADERS) {
    auto header_value = findRequestHeaderValue(request_headers, header_name)
                            .value_or(std::string_view{});
    if (header_name == Wasm::Common::Http::Header::ContentType &&
        signed_content_type.has_value()) {
      header_value = *signed_content_type;
    }
    string_to_sign.append(header_value);
    string_to_sign.push_back('\n');
  }

  // The old API Gateway data plane preserves the Header names declared in
  // X-Ca-Signature-Headers and looks up their values case-insensitively.
  const auto signature_header_names =
      findRequestHeaderValue(request_headers, CA_SIGNATURE_HEADERS)
          .value_or(std::string_view{});
  RequestHeaderPairs headers_to_sign;
  for (auto header_name : absl::StrSplit(signature_header_names, ",")) {
    header_name = absl::StripAsciiWhitespace(header_name);
    if (header_name.empty()) {
      continue;
    }
    const auto header_value =
        findRequestHeaderValue(request_headers, header_name)
            .value_or(std::string_view{});
    headers_to_sign.emplace_back(header_name, header_value);
  }
  std::sort(headers_to_sign.begin(), headers_to_sign.end());
  for (const auto& [name, value] : headers_to_sign) {
    absl::StrAppendFormat(&string_to_sign, "%s:%s\n", name, value);
  }

  if (use_compat_params) {
    auto decoded_form_body = Wasm::Common::Http::parseFromBody(raw_form_body);
    getStringToSignWithParam(&string_to_sign, path, decoded_form_body,
                             canonical_path_override);
    return string_to_sign;
  }

  const size_t query_start = path.find('?');
  const std::string_view raw_query =
      query_start == std::string::npos
          ? std::string_view{}
          : std::string_view(path).substr(query_start + 1);

  // QueryParams is ordered. Query is parsed before Form so it also wins when
  // both contain the same parameter name.
  Wasm::Common::Http::QueryParams parameters;
  for (bool is_form_body : {false, true}) {
    const std::string_view raw_parameters =
        is_form_body ? raw_form_body : raw_query;
    size_t start = 0;
    while (start < raw_parameters.size()) {
      size_t end = raw_parameters.find('&', start);
      if (end == std::string_view::npos) {
        end = raw_parameters.size();
      }

      auto raw_parameter = raw_parameters.substr(start, end - start);
      size_t equal_sign = raw_parameter.find('=');
      std::string name(raw_parameter.substr(0, equal_sign));
      std::string value;
      if (equal_sign != std::string_view::npos) {
        value.assign(raw_parameter.substr(equal_sign + 1));
        std::replace(name.begin(), name.end(), '+', ' ');
        std::replace(value.begin(), value.end(), '+', ' ');
        name = Wasm::Common::Http::PercentEncoding::decode(name);
        value = Wasm::Common::Http::PercentEncoding::decode(value);
      } else if (!is_form_body) {
        // The old data plane decodes a bare Query name, but preserves a bare
        // Form name exactly as received.
        std::replace(name.begin(), name.end(), '+', ' ');
        name = Wasm::Common::Http::PercentEncoding::decode(name);
      }
      parameters.emplace(std::move(name), std::move(value));
      start = end + 1;
    }
  }

  const std::string_view raw_path =
      std::string_view(path).substr(0, query_start);
  string_to_sign.append(canonical_path_override.value_or(raw_path));
  bool is_first = true;
  for (const auto& [name, value] : parameters) {
    string_to_sign.append(is_first ? "?" : "&");
    is_first = false;
    string_to_sign.append(name);
    if (!value.empty()) {
      absl::StrAppend(&string_to_sign, "=", value);
    }
  }
  return string_to_sign;
}

bool verifyOldApiGatewaySignature(const std::string& hash_type,
                                  const std::string& secret,
                                  const std::string& signature,
                                  const RequestHeaderPairs& request_headers,
                                  const std::string& path,
                                  std::string_view raw_form_body) {
  const auto string_to_sign =
      buildOldApiGatewayStringToSign(request_headers, path, raw_form_body);
  const auto expected_signature =
      Wasm::Common::Crypto::getShaHmacBase64(hash_type, secret, string_to_sign);
  if (expected_signature == signature) {
    LOG_DEBUG("signature verified by old API Gateway fallback");
    return true;
  }

  const bool use_compat_params = true;
  const auto compat_string_to_sign = buildOldApiGatewayStringToSign(
      request_headers, path, raw_form_body, use_compat_params);
  const auto compat_signature = Wasm::Common::Crypto::getShaHmacBase64(
      hash_type, secret, compat_string_to_sign);
  if (compat_signature == signature) {
    LOG_DEBUG("signature verified by old API Gateway compatibility fallback");
    return true;
  }

  // Preserve the existing raw-path candidates and add both decoded-path
  // parameter variants afterward, so already accepted signatures keep their
  // order.
  const size_t query_start = path.find('?');
  const std::string_view encoded_path =
      std::string_view(path).substr(0, query_start);
  if (encoded_path.find('%') == std::string_view::npos) {
    return false;
  }
  const auto decoded_path = decodeOldApiGatewayPath(encoded_path);
  if (!decoded_path.has_value() || *decoded_path == encoded_path) {
    return false;
  }

  const auto decoded_path_string_to_sign = buildOldApiGatewayStringToSign(
      request_headers, path, raw_form_body,
      /*use_compat_params=*/false, *decoded_path);
  const auto decoded_path_signature = Wasm::Common::Crypto::getShaHmacBase64(
      hash_type, secret, decoded_path_string_to_sign);
  if (decoded_path_signature == signature) {
    LOG_DEBUG(
        "signature verified by old API Gateway decoded-path fallback");
    return true;
  }

  const auto decoded_path_compat_string_to_sign =
      buildOldApiGatewayStringToSign(request_headers, path, raw_form_body,
                                     /*use_compat_params=*/true, *decoded_path);
  const auto decoded_path_compat_signature =
      Wasm::Common::Crypto::getShaHmacBase64(
          hash_type, secret, decoded_path_compat_string_to_sign);
  if (decoded_path_compat_signature == signature) {
    LOG_DEBUG(
        "signature verified by old API Gateway decoded-path compatibility "
        "fallback");
    return true;
  }
  return false;
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
  if (configuration.find("rbac_rules") != configuration.end()) {
    if (!JsonArrayIterate(configuration, "rbac_rules",
                          [&](const json& rbac_rule_config) -> bool {
                            RbacRule rbac_rule;
                            if (!rbac_rule.parse(rbac_rule_config)) {
                              LOG_ERROR(
                                  "failed to parse 'rbac_rules' field in "
                                  "filter configuration.");
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
            auto group_it = consumer.find("group");
            if (group_it != consumer.end()) {
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
                LOG_WARN(absl::StrCat("failed to parse 'group' for consumer: ",
                                      name.first.value()));
                return false;
              }
              rule.key_to_groups.emplace(key.first.value(), std::move(gs));
            }
            return true;
          })) {
    LOG_WARN("failed to parse configuration for credentials.");
    return false;
  }
  // if (rule.credentials.empty()) {
  //   LOG_INFO("at least one credential has to be configured for a rule.");
  //   return false;
  // }

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
  // Drop any client-supplied value so only this gateway's assertion survives.
  removeRequestHeader("X-Mse-Consumer");
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
  replaceConsumerGroupHeader(std::nullopt);
  auto key_to_name_iter = rule.key_to_name.find(std::string(ca_key));
  if (key_to_name_iter != rule.key_to_name.end()) {
    replaceRequestHeader("X-Mse-Consumer", key_to_name_iter->second);
    std::optional<std::vector<std::string>> groups;
    auto kg = rule.key_to_groups.find(std::string(ca_key));
    if (kg != rule.key_to_groups.end()) {
      groups = kg->second;
    }
    replaceConsumerGroupHeader(groups);
    LOG_DEBUG("consumer is " + key_to_name_iter->second);
    if (!checkAuthorization(key_to_name_iter->second, rule, allow_set)) {
      deniedUnauthorizedConsumer();
      return false;
    }
  }
  return true;
}

bool PluginRootContext::checkAuthorization(
    const std::string& consumer, const HmacAuthConfigRule& rule,
    const std::optional<std::unordered_set<std::string>>& allow_set) {
  if (!rule.rbac_rules.empty()) {
    if (!checkRbacRule(rule)) {
      LOG_DEBUG("checkRbacRule denied");
      return false;
    }
  } else if (allow_set) {
    if (allow_set->empty()) {
      LOG_DEBUG("allow set is empty, nobody is allowed");
      return false;
    }
    if (allow_set->find(consumer) == allow_set->end()) {
      LOG_DEBUG(absl::StrCat("consumer is not allowed: ", consumer));
      return false;
    }
  }
  return true;
}

bool PluginRootContext::checkRbacRule(const HmacAuthConfigRule& rule) {
  for (const auto& rbac_rule : rule.rbac_rules) {
    if (rbac_rule.check()) {
      return true;
    }
  }
  return false;
}

bool PluginRootContext::checkPlugin(
    const std::string& ca_key, const std::string& signature,
    const std::string& signature_method, const std::string& path,
    const std::string& date, bool is_timetamp, std::string* sts,
    std::string* sts_old, const RequestHeaderPairs& request_headers,
    const HmacAuthConfigRule& rule, std::string_view raw_form_body) {
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
      LOG_DEBUG(absl::StrFormat("date expired, offset is: %u", time_offset));
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

  std::string tip;
  {
    Wasm::Common::Http::QueryParams decoded_body_params;
    std::optional<std::reference_wrapper<Wasm::Common::Http::QueryParams>>
        body_params;
    if (!raw_form_body.empty()) {
      decoded_body_params = Wasm::Common::Http::parseFromBody(raw_form_body);
      body_params = std::ref(decoded_body_params);
    }

    // Current signature.
    getStringToSignWithParam(sts, path, body_params);
    const auto& str_to_sign = *sts;
    auto hmac =
        Wasm::Common::Crypto::getShaHmacBase64(hash_type, secret, str_to_sign);
    if (hmac == signature) {
      return true;
    }
    tip = absl::StrReplaceAll(str_to_sign, {{"\n", "#"}});
    LOG_DEBUG(
        absl::StrCat("current signature candidate invalid, stringToSign: ", tip,
                     " signature: ", hmac));

    // Forward compatibility.
    getStringToSignWithParam(sts_old, path, body_params);
    const auto& str_to_sign_forward = *sts_old;
    auto hmac_forward = Wasm::Common::Crypto::getShaHmacBase64(
        hash_type, secret, str_to_sign_forward);
    if (hmac_forward == signature) {
      // Keep the historical forward-compatibility behavior unchanged.
      return false;
    }
    auto tip_forward = absl::StrReplaceAll(str_to_sign_forward, {{"\n", "#"}});
    LOG_DEBUG(absl::StrCat(
        "forward compatibility signature also invalid, stringToSign: ",
        tip_forward, " signature: ", hmac_forward));
  }

  // Old API Gateway fallback.
  if (verifyOldApiGatewaySignature(hash_type, secret, signature,
                                   request_headers, path, raw_form_body)) {
    return true;
  }

  deniedInvalidCredentials(absl::StrFormat("Server StringToSign:`%s`", tip));
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

  auto config = rootCtx->getMatchAuthConfig();
  config_ = config.first;
  if (!config_) {
    LOG_DEBUG("no matched config found");
    return FilterHeadersStatus::Continue;
  }
  allow_set_ = config.second;
  if (!allow_set_ && rootCtx->globalAuthDisable()) {
    // No allow set, means no need to check auth if global auth is disable
    LOG_DEBUG(
        "no allow set found, and global auth is disable, no need to auth");
    return FilterHeadersStatus::Continue;
  }

  auto request_header_data = getRequestHeaderPairs();
  auto request_headers = request_header_data->pairs();
  request_headers_.clear();
  request_headers_.reserve(request_headers.size());
  for (const auto& [name, value] : request_headers) {
    request_headers_.emplace_back(name, value);
  }
  ca_key_ = getRequestHeader(CA_KEY)->toString();
  signature_ = getRequestHeader(CA_SIGNATURE)->toString();
  signature_method_ = getRequestHeader(CA_SIGNATURE_METHOD)->toString();
  path_ = getRequestHeader(Wasm::Common::Http::Header::Path)->toString();
  date_ = getRequestHeader(Wasm::Common::Http::Header::Date)->toString();
  str_to_sign_ = getStringToSign();
  str_to_sign_old_ = getStringToSignOld();
  body_md5_ =
      getRequestHeader(Wasm::Common::Http::Header::ContentMD5)->toString();
  GET_HEADER_VIEW(Wasm::Common::Http::Header::ContentType, content_type);
  GET_HEADER_VIEW(Wasm::Common::Http::Header::ContentLength, content_length);
  GET_HEADER_VIEW(Wasm::Common::Http::Header::TransferEncoding,
                  transfer_encoding);

  is_timestamp_ = false;
  if (date_.empty()) {
    date_ = getRequestHeader(CA_TIMESTAMP)->toString();
    is_timestamp_ = true;
  }
  // check if ca_key present in config and it's consumer_name is allowed
  if (!rootCtx->checkConsumer(ca_key_, config_.value(), allow_set_)) {
    return FilterHeadersStatus::StopAllIterationAndBuffer;
  }

  if (absl::StrContains(absl::AsciiStrToLower(content_type),
                        "application/x-www-form-urlencoded") &&
      ((!content_length.empty() && content_length != "0") ||
       transfer_encoding == "chunked")) {
    setFilterState(SetDecoderBufferLimitKey, DefaultMaxBodyBytes);
    LOG_INFO(absl::StrCat("SetRequestBodyBufferLimit: ", DefaultMaxBodyBytes));
    check_body_params_ = true;
    return FilterHeadersStatus::StopIteration;
  }

  return rootCtx->checkPlugin(ca_key_, signature_, signature_method_, path_,
                              date_, is_timestamp_, &str_to_sign_,
                              &str_to_sign_old_, request_headers_,
                              config_.value(), {})
             ? FilterHeadersStatus::Continue
             : FilterHeadersStatus::StopAllIterationAndBuffer;
}

FilterDataStatus PluginContext::onRequestBody(size_t body_size,
                                              bool end_stream) {
  if (!config_) {
    return FilterDataStatus::Continue;
  }
  if (body_md5_.empty() && !check_body_params_) {
    return FilterDataStatus::Continue;
  }
  if (!end_stream) {
    return FilterDataStatus::StopIterationAndBuffer;
  }
  auto body = getBufferBytes(WasmBufferType::HttpRequestBody, 0, body_size);
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
    auto* rootCtx = rootContext();
    return rootCtx->checkPlugin(ca_key_, signature_, signature_method_, path_,
                                date_, is_timestamp_, &str_to_sign_,
                                &str_to_sign_old_, request_headers_,
                                config_.value(), body->view())
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
