// Copyright (c) 2026 Alibaba Group Holding Ltd.
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

#pragma once

#include <exception>
#include <string>
#include <system_error>

#include "jwt-cpp/jwt.h"

namespace oauth_v0_probe {

struct Result {
  bool allowed;
  std::string outcome;
  std::string detail;
};

inline Result evaluate(const std::string& encoded) {
#ifdef __cpp_exceptions
  try {
#endif
    auto token = jwt::decode(encoded);
    for (const auto& field : {"client_id", "iss", "sub", "aud"}) {
      if (token.get_payload_claim(field).get_type() != jwt::json::type::string) {
        return {false, "rejected", "claim-type"};
      }
    }
    for (const auto& field : {"exp", "iat"}) {
      if (token.get_payload_claim(field).get_type() != jwt::json::type::integer) {
        return {false, "rejected", "claim-type"};
      }
    }
    if (token.get_payload_claim("client_id").as_string() != "probe-client") {
      return {false, "rejected", "client-id"};
    }
    auto verifier = jwt::verify()
                        .allow_algorithm(jwt::algorithm::hs256{"oauth-v0-test-key"})
                        .with_issuer("oauth-v0")
                        .with_subject("probe")
                        .with_type("JWT");
    std::error_code error;
    verifier.verify(token, error);
    if (error) {
      return {false, "rejected", "verify-error"};
    }
    if (token.get_payload_claim("aud").as_string() != "v0") {
      return {false, "rejected", "audience"};
    }
    return {true, "valid", "signed-control"};
#ifdef __cpp_exceptions
  } catch (const std::exception& error) {
    return {false, "caught", error.what()};
  } catch (...) {
    return {false, "caught", "non-std-exception"};
  }
#endif
}

}
