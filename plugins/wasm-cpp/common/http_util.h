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

#pragma once

#include <array>
#include <chrono>
#include <map>
#include <unordered_set>

#include "absl/strings/string_view.h"
#include "absl/time/time.h"

#ifndef NULL_PLUGIN

#include "proxy_wasm_intrinsics.h"

#else

#include "include/proxy-wasm/null_plugin.h"
using namespace proxy_wasm::null_plugin;
using proxy_wasm::FilterDataStatus;
using proxy_wasm::FilterHeadersStatus;

#endif

namespace Wasm::Common::Http {

using QueryParams = std::map<std::string, std::string>;
using SystemTime = std::chrono::time_point<std::chrono::system_clock>;

namespace Status {
constexpr int OK = 200;
constexpr int InternalServerError = 500;
constexpr int Unauthorized = 401;
}  // namespace Status

namespace Header {
constexpr std::string_view Scheme(":scheme");
constexpr std::string_view Method(":method");
constexpr std::string_view Host(":authority");
constexpr std::string_view Path(":path");
constexpr std::string_view EnvoyOriginalPath("x-envoy-original-path");
constexpr std::string_view Accept("accept");
constexpr std::string_view ContentDisposition("content-disposition");
constexpr std::string_view ContentMD5("content-md5");
constexpr std::string_view ContentType("content-type");
constexpr std::string_view ContentLength("content-length");
constexpr std::string_view TransferEncoding("transfer-encoding");
constexpr std::string_view UserAgent("user-agent");
constexpr std::string_view Date("date");
constexpr std::string_view Cookie("cookie");
constexpr std::string_view StrictTransportSecurity("strict-transport-security");
}  // namespace Header

namespace ContentTypeValues {
constexpr std::string_view Grpc{"application/grpc"};
constexpr std::string_view Json{"application/json"};
constexpr std::string_view MultipartFormData{"multipart/form-data"};
}  // namespace ContentTypeValues

class PercentEncoding {
 public:
  /**
   * Encodes string view to its percent encoded representation. Non-visible
   * ASCII is always escaped, in addition to a given list of reserved chars.
   *
   * @param value supplies string to be encoded.
   * @param reserved_chars list of reserved chars to escape. By default the
   * escaped chars in
   *        https://github.com/grpc/grpc/blob/master/doc/PROTOCOL-HTTP2.md#responses
   * are used.
   * @return std::string percent-encoded string.
   */
  static std::string encode(absl::string_view value,
                            absl::string_view reserved_chars = "%");

  /**
   * Decodes string view from its percent encoded representation.
   * @param encoded supplies string to be decoded.
   * @return std::string decoded string
   * https://tools.ietf.org/html/rfc3986#section-2.1.
   */
  static std::string decode(absl::string_view value);

 private:
  // Encodes string view to its percent encoded representation, with start
  // index.
  static std::string encode(absl::string_view value, size_t index,
                            const std::unordered_set<char>& reserved_char_set);
};

SystemTime httpTime(std::string_view date);

inline bool timePointValid(SystemTime time_point) {
  return std::chrono::duration_cast<std::chrono::milliseconds>(
             time_point.time_since_epoch())
             .count() != 0;
}

std::string_view stripPortFromHost(std::string_view request_host);

/**
 * Parse a URL into query parameters.
 * @param url supplies the url to parse.
 * @return QueryParams the parsed parameters, if any.
 */
QueryParams parseQueryString(absl::string_view url);

/**
 * Parse a URL into query parameters.
 * @param url supplies the url to parse.
 * @return QueryParams the parsed and percent-decoded parameters, if any.
 */
QueryParams parseAndDecodeQueryString(absl::string_view url);

/**
 * Parse a request body into query parameters.
 * @param body supplies the body to parse.
 * @return QueryParams the parsed parameters, if any.
 */
QueryParams parseFromBody(absl::string_view body);

/**
 * Parse query parameters from a URL or body.
 * @param data supplies the data to parse.
 * @param start supplies the offset within the data.
 * @param decode_params supplies the flag whether to percent-decode the parsed
 * parameters (both name and value). Set to false to keep the parameters
 * encoded.
 * @return QueryParams the parsed parameters, if any.
 */
QueryParams parseParameters(absl::string_view data, size_t start,
                            bool decode_params);

std::vector<std::string> getAllOfHeader(std::string_view key);

std::unordered_map<std::string, std::string> parseCookies(
    const std::function<bool(std::string_view)>& key_filter);

std::string buildOriginalUri(std::optional<uint32_t> max_path_length);

void extractHostPathFromUri(const absl::string_view& uri,
                            absl::string_view& host, absl::string_view& path);

void extractPathWithoutArgsFromUri(const std::string_view& uri,
                                   std::string_view& path_without_args);
bool hasRequestBody();
}  // namespace Wasm::Common::Http
