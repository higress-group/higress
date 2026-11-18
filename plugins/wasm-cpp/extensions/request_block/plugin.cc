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

#include "extensions/request_block/plugin.h"

#include <array>
#include <memory>
#include <optional>

#include "absl/strings/str_cat.h"
#include "absl/strings/str_join.h"
#include "absl/strings/str_split.h"
#include "common/json_util.h"

using ::nlohmann::json;
using ::Wasm::Common::JsonArrayIterate;
using ::Wasm::Common::JsonGetField;
using ::Wasm::Common::JsonObjectIterate;
using ::Wasm::Common::JsonValueAs;

#ifdef NULL_PLUGIN

namespace proxy_wasm {
namespace null_plugin {
namespace request_block {

PROXY_WASM_NULL_PLUGIN_REGISTRY

#endif

static RegisterContextFactory register_RequestBlock(
    CONTEXT_FACTORY(PluginContext), ROOT_FACTORY(PluginRootContext));

static constexpr size_t MAX_BODY_SIZE = 32 * 1024 * 1024;

// Decode once without allowing encoded '?' or '#' to change URL components.
static std::optional<std::string> decodePathForMatch(std::string_view path) {
  auto hex_value = [](char ch) -> int {
    if (ch >= '0' && ch <= '9') {
      return ch - '0';
    }
    if (ch >= 'a' && ch <= 'f') {
      return ch - 'a' + 10;
    }
    if (ch >= 'A' && ch <= 'F') {
      return ch - 'A' + 10;
    }
    return -1;
  };

  std::string decoded;
  decoded.reserve(path.size());
  for (size_t i = 0; i < path.size(); ++i) {
    if (path[i] != '%') {
      decoded.push_back(path[i]);
      continue;
    }
    if (i + 2 >= path.size()) {
      return std::nullopt;
    }
    const auto hi = hex_value(path[i + 1]);
    const auto lo = hex_value(path[i + 2]);
    if (hi < 0 || lo < 0) {
      return std::nullopt;
    }
    const auto decoded_char = static_cast<char>((hi << 4) | lo);
    if (decoded_char == '?' || decoded_char == '#') {
      decoded.append(path.data() + i, 3);
    } else {
      decoded.push_back(decoded_char);
    }
    i += 2;
  }
  return decoded;
}

bool PluginRootContext::parsePluginConfig(const json& configuration,
                                          RequestBlockConfigRule& rule) {
  auto it = configuration.find("blocked_code");
  if (it != configuration.end()) {
    auto blocked_code = JsonValueAs<int64_t>(it.value());
    if (blocked_code.second != Wasm::Common::JsonParserResultDetail::OK) {
      LOG_WARN("cannot parse status code");
      return false;
    }
    rule.blocked_code = blocked_code.first.value();
  }
  it = configuration.find("blocked_message");
  if (it != configuration.end()) {
    auto blocked_message = JsonValueAs<std::string>(it.value());
    if (blocked_message.second != Wasm::Common::JsonParserResultDetail::OK) {
      LOG_WARN("cannot parse blocked_message");
      return false;
    }
    rule.blocked_message = blocked_message.first.value();
  }
  it = configuration.find("case_sensitive");
  if (it != configuration.end()) {
    auto case_sensitive = JsonValueAs<bool>(it.value());
    if (case_sensitive.second != Wasm::Common::JsonParserResultDetail::OK) {
      LOG_WARN("cannot parse case_sensitive");
      return false;
    }
    rule.case_sensitive = case_sensitive.first.value();
  }
  if (!JsonArrayIterate(
          configuration, "block_urls", [&](const json& item) -> bool {
            auto url = JsonValueAs<std::string>(item);
            if (url.second != Wasm::Common::JsonParserResultDetail::OK) {
              LOG_WARN("cannot parse block_urls");
              return false;
            }
            if (rule.case_sensitive) {
              rule.block_urls.push_back(std::move(url.first.value()));
            } else {
              rule.block_urls.push_back(
                  absl::AsciiStrToLower(url.first.value()));
            }
            return true;
          })) {
    LOG_WARN("failed to parse configuration for block_urls.");
    return false;
  }
  if (!JsonArrayIterate(
          configuration, "block_exact_urls", [&](const json& item) -> bool {
            auto url = JsonValueAs<std::string>(item);
            if (url.second != Wasm::Common::JsonParserResultDetail::OK) {
              LOG_WARN("cannot parse block_exact_urls");
              return false;
            }
            if (rule.case_sensitive) {
              rule.block_exact_urls.push_back(std::move(url.first.value()));
            } else {
              rule.block_exact_urls.push_back(
                  absl::AsciiStrToLower(url.first.value()));
            }
            return true;
          })) {
    LOG_WARN("failed to parse configuration for block_exact_urls.");
    return false;
  }
  if (!JsonArrayIterate(
          configuration, "block_regexp_urls", [&](const json& item) -> bool {
            auto url = JsonValueAs<std::string>(item);
            if (url.second != Wasm::Common::JsonParserResultDetail::OK) {
              LOG_WARN("cannot parse block_regexp_urls");
              return false;
            }
            std::string regex;
            if (rule.case_sensitive) {
              regex = url.first.value();
            } else {
              regex = absl::AsciiStrToLower(url.first.value());
            }
            auto re = std::make_unique<ReMatcher>(regex);
            if (!re->error().empty()) {
              LOG_WARN(re->error());
              return false;
            }
            rule.block_regexp_urls.push_back(std::move(re));
            return true;
          })) {
    LOG_WARN("failed to parse configuration for block_regexp_urls.");
    return false;
  }
  if (!JsonArrayIterate(
          configuration, "block_headers", [&](const json& item) -> bool {
            auto header = JsonValueAs<std::string>(item);
            if (header.second != Wasm::Common::JsonParserResultDetail::OK) {
              LOG_WARN("cannot parse block_headers");
              return false;
            }
            if (rule.case_sensitive) {
              rule.block_headers.push_back(std::move(header.first.value()));
            } else {
              rule.block_headers.push_back(
                  absl::AsciiStrToLower(header.first.value()));
            }
            return true;
          })) {
    LOG_WARN("failed to parse configuration for block_headers.");
    return false;
  }
  if (!JsonArrayIterate(
          configuration, "block_bodys", [&](const json& item) -> bool {
            auto body = JsonValueAs<std::string>(item);
            if (body.second != Wasm::Common::JsonParserResultDetail::OK) {
              LOG_WARN("cannot parse block_bodys");
              return false;
            }
            if (rule.case_sensitive) {
              rule.block_bodys.push_back(std::move(body.first.value()));
            } else {
              rule.block_bodys.push_back(
                  absl::AsciiStrToLower(body.first.value()));
            }
            return true;
          })) {
    LOG_WARN("failed to parse configuration for block_bodys.");
    return false;
  }
  // compatible
  if (!JsonArrayIterate(
          configuration, "block_bodies", [&](const json& item) -> bool {
            auto body = JsonValueAs<std::string>(item);
            if (body.second != Wasm::Common::JsonParserResultDetail::OK) {
              LOG_WARN("cannot parse block_bodies");
              return false;
            }
            if (rule.case_sensitive) {
              rule.block_bodys.push_back(std::move(body.first.value()));
            } else {
              rule.block_bodys.push_back(
                  absl::AsciiStrToLower(body.first.value()));
            }
            return true;
          })) {
    LOG_WARN("failed to parse configuration for block_bodies.");
    return false;
  }
  if (rule.block_bodys.empty() && rule.block_headers.empty() &&
      rule.block_urls.empty() && rule.block_exact_urls.empty() &&
      rule.block_regexp_urls.empty()) {
    LOG_WARN("there is no block rules");
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
  if (!result.has_value()) {
    LOG_WARN(absl::StrCat("cannot parse plugin configuration JSON string: ",
                          configuration_data->view()));
    return false;
  }
  if (!parseRuleConfig(result.value())) {
    LOG_WARN(absl::StrCat("cannot parse plugin configuration JSON string: ",
                          configuration_data->view()));
    return false;
  }
  return true;
}

bool PluginRootContext::checkHeader(const RequestBlockConfigRule& rule,
                                    bool& check_body) {
  if (!rule.block_urls.empty() || !rule.block_regexp_urls.empty() ||
      !rule.block_exact_urls.empty()) {
    std::string urlstr;
    std::string_view url;
    GET_HEADER_VIEW(":path", request_url);
    if (rule.case_sensitive) {
      url = request_url;
    } else {
      urlstr = absl::AsciiStrToLower(request_url);
      url = urlstr;
    }

    const auto query_pos = request_url.find('?');
    auto normalized_url = decodePathForMatch(request_url.substr(0, query_pos));
    if (normalized_url && query_pos != std::string_view::npos) {
      normalized_url->append(request_url.data() + query_pos,
                             request_url.size() - query_pos);
    }
    if (normalized_url && !rule.case_sensitive) {
      *normalized_url = absl::AsciiStrToLower(*normalized_url);
    }

    for (const auto& block_url : rule.block_exact_urls) {
      if (url == block_url ||
          (normalized_url && *normalized_url == block_url)) {
        sendLocalResponse(rule.blocked_code, "", rule.blocked_message, {});
        return false;
      }
    }
    for (const auto& block_url : rule.block_regexp_urls) {
      if (block_url->match(url) ||
          (normalized_url && block_url->match(*normalized_url))) {
        sendLocalResponse(rule.blocked_code, "", rule.blocked_message, {});
        return false;
      }
    }
    for (const auto& block_url : rule.block_urls) {
      if (absl::StrContains(url, block_url) ||
          (normalized_url && absl::StrContains(*normalized_url, block_url))) {
        sendLocalResponse(rule.blocked_code, "", rule.blocked_message, {});
        return false;
      }
    }
  }
  if (!rule.block_headers.empty()) {
    auto headersPtr = getRequestHeaderPairs();
    std::string headerstr;
    std::string_view headers;
    if (rule.case_sensitive) {
      headers = headersPtr->view();
    } else {
      headerstr = absl::AsciiStrToLower(headersPtr->view());
      headers = headerstr;
    }
    for (const auto& block_header : rule.block_headers) {
      if (absl::StrContains(headers, block_header)) {
        sendLocalResponse(rule.blocked_code, "", rule.blocked_message, {});
        return false;
      }
    }
  }
  if (!rule.block_bodys.empty()) {
    check_body = true;
  }
  return true;
}
bool PluginRootContext::checkBody(const RequestBlockConfigRule& rule,
                                  std::string_view request_body) {
  std::string bodystr;
  std::string_view body;
  if (rule.case_sensitive) {
    body = request_body;
  } else {
    bodystr = absl::AsciiStrToLower(request_body);
    body = bodystr;
  }
  for (const auto& block_body : rule.block_bodys) {
    if (absl::StrContains(body, block_body)) {
      sendLocalResponse(rule.blocked_code, "", rule.blocked_message, {});
      return false;
    }
  }
  return true;
}

FilterHeadersStatus PluginContext::onRequestHeaders(uint32_t, bool) {
  auto* rootCtx = rootContext();
  auto config = rootCtx->getMatchConfig();
  config_ = config.second;
  if (!config_) {
    return FilterHeadersStatus::Continue;
  }
  return rootCtx->checkHeader(config_.value(), check_body_)
             ? FilterHeadersStatus::Continue
             : FilterHeadersStatus::StopIteration;
}

FilterDataStatus PluginContext::onRequestBody(size_t body_size,
                                              bool end_stream) {
  if (!config_) {
    return FilterDataStatus::Continue;
  }
  if (!check_body_) {
    return FilterDataStatus::Continue;
  }
  body_total_size_ += body_size;
  if (body_total_size_ > MAX_BODY_SIZE) {
    LOG_DEBUG("body_size is too large");
    return FilterDataStatus::Continue;
  }
  if (!end_stream) {
    return FilterDataStatus::StopIterationAndBuffer;
  }
  auto body =
      getBufferBytes(WasmBufferType::HttpRequestBody, 0, body_total_size_);
  auto* rootCtx = rootContext();
  return rootCtx->checkBody(config_.value(), body->view())
             ? FilterDataStatus::Continue
             : FilterDataStatus::StopIterationNoBuffer;
}

#ifdef NULL_PLUGIN

}  // namespace request_block
}  // namespace null_plugin
}  // namespace proxy_wasm

#endif
