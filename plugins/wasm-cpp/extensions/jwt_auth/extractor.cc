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

// modified base on envoy/source/extensions/filters/http/jwt_authn/extractor.cc
#include "extensions/jwt_auth/extractor.h"

#include <memory>
#include <tuple>
#include <unordered_map>

#include "absl/container/btree_map.h"
#include "common/http_util.h"
#include "extensions/jwt_auth/plugin.h"

#ifdef NULL_PLUGIN

namespace proxy_wasm {
namespace null_plugin {
namespace jwt_auth {

#endif

namespace {

constexpr std::string_view FallbackFromHeader = "x-higress-fallback-from";
constexpr std::string_view OriginalAuthHeader = "x-hi-original-auth";

/**
 * Check Claims specified in Provider
 */
class JwtClaimChecker {
 public:
  JwtClaimChecker(const ClaimsMap& claims) : allowed_claims_(claims) {}

  // check if a jwt issuer is allowed
  bool check(const std::string& key, const std::string& value) const {
    if (allowed_claims_.empty()) {
      return true;
    }
    auto it = allowed_claims_.find(key);
    return it != allowed_claims_.end() && it->second == value;
  }

 private:
  // Only these specified claims are allowed.
  const ClaimsMap& allowed_claims_;
};

using JwtClaimCheckerPtr = std::unique_ptr<JwtClaimChecker>;

// A base JwtLocation object to store token and claim_checker.
class JwtLocationBase : public JwtLocation {
 public:
  JwtLocationBase(const std::string& token,
                  const JwtClaimChecker& claim_checker)
      : token_(token), claim_checker_(claim_checker) {}

  // Get the token string
  const std::string& token() const override { return token_; }

  // Check if an claim has specified the location.
  bool isClaimAllowed(const std::string& key,
                      const std::string& value) const override {
    return claim_checker_.check(key, value);
  }

  void addClaimToHeader(const std::string& header, const std::string& value,
                        bool override) const override {
    claims_to_headers_.emplace_back(header, value, override);
  }

  void claimsToHeaders() const override {
    for (const auto& claim_to_header : claims_to_headers_) {
      const auto& header_key = std::get<0>(claim_to_header);
      const auto& header_value = std::get<1>(claim_to_header);
      if (std::get<2>(claim_to_header)) {
        auto header_ptr = getRequestHeader(header_key);
        if (!header_ptr->view().empty()) {
          replaceRequestHeader(header_key, header_value);
          continue;
        }
      }
      addRequestHeader(header_key, header_value);
    }
  }

 private:
  mutable std::vector<std::tuple<std::string, std::string, bool>>
      claims_to_headers_;
  // Extracted token.
  const std::string token_;
  // Claim checker
  const JwtClaimChecker& claim_checker_;
};

// The JwtLocation for header extraction.
class JwtHeaderLocation : public JwtLocationBase {
 public:
  JwtHeaderLocation(const std::string& token,
                    const JwtClaimChecker& claim_checker,
                    const std::string& header)
      : JwtLocationBase(token, claim_checker), header_(header) {}

  void removeJwt() const override { removeRequestHeader(header_); }

 private:
  // the header name the JWT is extracted from.
  const std::string& header_;
};

// The JwtLocation for param extraction.
class JwtParamLocation : public JwtLocationBase {
 public:
  JwtParamLocation(const std::string& token,
                   const JwtClaimChecker& claim_checker, const std::string&)
      : JwtLocationBase(token, claim_checker) {}

  void removeJwt() const override {
    // TODO(qiwzhang): remove JWT from parameter.
  }
};

// The JwtLocation for cookie extraction.
class JwtCookieLocation : public JwtLocationBase {
 public:
  JwtCookieLocation(const std::string& token,
                    const JwtClaimChecker& claim_checker)
      : JwtLocationBase(token, claim_checker) {}

  void removeJwt() const override {
    // TODO(theshubhamp): remove JWT from cookies.
  }
};

class ExtractorImpl : public Extractor {
 public:
  ExtractorImpl(const Consumer& provider);

  std::vector<JwtLocationConstPtr> extract() const override;

 private:
  // add a header config
  void addHeaderConfig(const ClaimsMap& claims, const std::string& header_name,
                       const std::string& value_prefix);
  // add a query param config
  void addQueryParamConfig(const ClaimsMap& claims, const std::string& param);
  // add a query param config
  void addCookieConfig(const ClaimsMap& claims, const std::string& cookie);
  // ctor helper for a jwt provider config
  void addProvider(const Consumer& provider);

  // HeaderMap value type to store prefix and issuers that specified this
  // header.
  struct HeaderLocationSpec {
    HeaderLocationSpec(const std::string& header,
                       const std::string& value_prefix)
        : header_(header), value_prefix_(value_prefix) {}
    // The header name.
    std::string header_;
    // The value prefix. e.g. for "Bearer <token>", the value_prefix is "Bearer
    // ".
    std::string value_prefix_;
    // Issuers that specified this header.
    JwtClaimCheckerPtr claim_checker_;
  };
  using HeaderLocationSpecPtr = std::unique_ptr<HeaderLocationSpec>;
  // The map of (header + value_prefix) to HeaderLocationSpecPtr
  std::map<std::string, HeaderLocationSpecPtr> header_locations_;

  // ParamMap value type to store issuers that specified this header.
  struct ParamLocationSpec {
    // Issuers that specified this param.
    JwtClaimCheckerPtr claim_checker_;
  };
  // The map of a parameter key to set of issuers specified the parameter
  std::map<std::string, ParamLocationSpec> param_locations_;

  // CookieMap value type to store issuers that specified this cookie.
  struct CookieLocationSpec {
    // Issuers that specified this param.
    JwtClaimCheckerPtr claim_checker_;
  };
  // The map of a cookie key to set of issuers specified the cookie.
  absl::btree_map<std::string, CookieLocationSpec> cookie_locations_;
};

ExtractorImpl::ExtractorImpl(const Consumer& provider) {
  addProvider(provider);
}

void ExtractorImpl::addProvider(const Consumer& provider) {
  for (const auto& header : provider.from_headers) {
    addHeaderConfig(provider.allowd_claims, header.header, header.value_prefix);
  }
  for (const std::string& param : provider.from_params) {
    addQueryParamConfig(provider.allowd_claims, param);
  }
  for (const std::string& cookie : provider.from_cookies) {
    addCookieConfig(provider.allowd_claims, cookie);
  }
}

void ExtractorImpl::addHeaderConfig(const ClaimsMap& claims,
                                    const std::string& header_name,
                                    const std::string& value_prefix) {
  const std::string map_key = header_name + value_prefix;
  auto& header_location_spec = header_locations_[map_key];
  if (!header_location_spec) {
    header_location_spec =
        std::make_unique<HeaderLocationSpec>(header_name, value_prefix);
  }
  header_location_spec->claim_checker_ =
      std::make_unique<JwtClaimChecker>(claims);
}

void ExtractorImpl::addQueryParamConfig(const ClaimsMap& claims,
                                        const std::string& param) {
  auto& param_location_spec = param_locations_[param];
  param_location_spec.claim_checker_ =
      std::make_unique<JwtClaimChecker>(claims);
}

void ExtractorImpl::addCookieConfig(const ClaimsMap& claims,
                                    const std::string& cookie) {
  auto& cookie_location_spec = cookie_locations_[cookie];
  cookie_location_spec.claim_checker_ =
      std::make_unique<JwtClaimChecker>(claims);
}

std::vector<JwtLocationConstPtr> ExtractorImpl::extract() const {
  std::vector<JwtLocationConstPtr> tokens;

  // Check header locations first
  for (const auto& location_it : header_locations_) {
    const auto& location_spec = location_it.second;
    // Check if fallback request
    std::string header;
    auto fallback_from = getRequestHeader(FallbackFromHeader)->toString();
    if (!fallback_from.empty()) {
      header = getRequestHeader(OriginalAuthHeader)->toString();
    } else {
      header = getRequestHeader(location_spec->header_)->toString();
    }
    if (!header.empty()) {
      const auto pos = header.find(location_spec->value_prefix_);
      if (pos != 0) {
        continue;
      }
      auto header_strip =
          header.substr(pos + location_spec->value_prefix_.length());
      tokens.push_back(std::make_unique<const JwtHeaderLocation>(
          header_strip, *location_spec->claim_checker_,
          location_spec->header_));
    }
  }

  // Check query parameter locations only if query parameter locations specified
  // and Path() is not null
  auto path = getRequestHeader(Wasm::Common::Http::Header::Path)->toString();
  if (!param_locations_.empty() && !path.empty()) {
    const auto& params = Wasm::Common::Http::parseAndDecodeQueryString(path);
    for (const auto& location_it : param_locations_) {
      const auto& param_key = location_it.first;
      const auto& location_spec = location_it.second;
      const auto& it = params.find(param_key);
      if (it != params.end()) {
        tokens.push_back(std::make_unique<const JwtParamLocation>(
            it->second, *location_spec.claim_checker_, param_key));
      }
    }
  }

  // Check cookie locations.
  if (!cookie_locations_.empty()) {
    const auto& cookies =
        Wasm::Common::Http::parseCookies([&](absl::string_view k) -> bool {
          return cookie_locations_.contains(k);
        });

    for (const auto& location_it : cookie_locations_) {
      const auto& cookie_key = location_it.first;
      const auto& location_spec = location_it.second;
      const auto& it = cookies.find(cookie_key);
      if (it != cookies.end()) {
        tokens.push_back(std::make_unique<const JwtCookieLocation>(
            it->second, *location_spec.claim_checker_));
      }
    }
  }
  return tokens;
}

}  // namespace

ExtractorConstPtr Extractor::create(const Consumer& provider) {
  return std::make_unique<ExtractorImpl>(provider);
}

#ifdef NULL_PLUGIN

}  // namespace jwt_auth
}  // namespace null_plugin
}  // namespace proxy_wasm

#endif
