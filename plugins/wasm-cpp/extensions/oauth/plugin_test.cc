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

#include <vector>

#include "gmock/gmock.h"
#include "gtest/gtest.h"
#include "include/proxy-wasm/context.h"
#include "include/proxy-wasm/null.h"

namespace proxy_wasm {
namespace null_plugin {
namespace oauth {

NullPluginRegistry* context_registry_;
RegisterNullVmPluginFactory register_oauth_plugin("oauth", []() {
  return std::make_unique<NullPlugin>(oauth::context_registry_);
});

class MockContext : public proxy_wasm::ContextBase {
 public:
  MockContext(WasmBase* wasm) : ContextBase(wasm) {}

  MOCK_METHOD(BufferInterface*, getBuffer, (WasmBufferType));
  MOCK_METHOD(WasmResult, log, (uint32_t, std::string_view));
  MOCK_METHOD(WasmDataPtr, getBufferBytes, (WasmBufferType, size_t, size_t));
  MOCK_METHOD(WasmResult, getHeaderMapPairs, (WasmHeaderMapType, Pairs*));
  MOCK_METHOD(WasmResult, getHeaderMapValue,
              (WasmHeaderMapType /* type */, std::string_view /* jwt */,
               std::string_view* /*result */));
  MOCK_METHOD(WasmResult, removeHeaderMapValue,
              (WasmHeaderMapType /* type */, std::string_view /* key */));
  MOCK_METHOD(WasmResult, addHeaderMapValue,
              (WasmHeaderMapType /* type */, std::string_view /* jwt */,
               std::string_view /* value */));
  MOCK_METHOD(WasmResult, sendLocalResponse,
              (uint32_t /* response_code */, std::string_view /* body */,
               Pairs /* additional_headers */, uint32_t /* grpc_status */,
               std::string_view /* details */));
  MOCK_METHOD(uint64_t, getCurrentTimeNanoseconds, ());
  MOCK_METHOD(WasmResult, getProperty, (std::string_view, std::string*));
  MOCK_METHOD(WasmResult, httpCall,
              (std::string_view, const Pairs&, std::string_view, const Pairs&,
               int, uint32_t*));
};

class OAuthTest : public ::testing::Test {
 protected:
  OAuthTest() {
    // Initialize test VM
    test_vm_ = createNullVm();
    wasm_base_ = std::make_unique<WasmBase>(
        std::move(test_vm_), "test-vm", "", "",
        std::unordered_map<std::string, std::string>{},
        AllowedCapabilitiesMap{});
    wasm_base_->load("oauth");
    wasm_base_->initialize();

    // Initialize host side context
    mock_context_ = std::make_unique<MockContext>(wasm_base_.get());
    current_context_ = mock_context_.get();

    ON_CALL(*mock_context_, log(testing::_, testing::_))
        .WillByDefault([](uint32_t, std::string_view m) {
          std::cerr << m << "\n";
          return WasmResult::Ok;
        });

    ON_CALL(*mock_context_, getHeaderMapValue(WasmHeaderMapType::RequestHeaders,
                                              testing::_, testing::_))
        .WillByDefault([&](WasmHeaderMapType, std::string_view header,
                           std::string_view* result) {
          if (header == ":authority") {
            *result = authority_;
          }
          if (header == ":scheme") {
            *result = "http";
          }
          if (header == ":path") {
            *result = path_;
          }
          if (header == ":method") {
            *result = method_;
          }
          if (header == "Authorization") {
            *result = jwt_header_;
          }
          if (header == "content-type") {
            *result = content_type_;
          }
          if (header == "x-custom-header") {
            *result = custom_header_;
          }
          return WasmResult::Ok;
        });
    ON_CALL(*mock_context_, addHeaderMapValue(WasmHeaderMapType::RequestHeaders,
                                              testing::_, testing::_))
        .WillByDefault([&](WasmHeaderMapType, std::string_view jwt,
                           std::string_view value) { return WasmResult::Ok; });

    ON_CALL(*mock_context_, getCurrentTimeNanoseconds()).WillByDefault([&]() {
      return current_time_;
    });

    ON_CALL(*mock_context_, getProperty(testing::_, testing::_))
        .WillByDefault([&](std::string_view path, std::string* result) {
          *result = route_name_;
          return WasmResult::Ok;
        });

    ON_CALL(*mock_context_, getBufferBytes(WasmBufferType::HttpCallResponseBody,
                                           testing::_, testing::_))
        .WillByDefault([&](WasmBufferType, size_t, size_t) {
          return std::make_unique<WasmData>(http_call_body_.data(),
                                            http_call_body_.size());
        });

    ON_CALL(*mock_context_,
            getHeaderMapPairs(WasmHeaderMapType::HttpCallResponseHeaders,
                              testing::_))
        .WillByDefault([&](WasmHeaderMapType, Pairs* result) {
          *result = http_call_headers_;
          return WasmResult::Ok;
        });

    ON_CALL(*mock_context_, httpCall(testing::_, testing::_, testing::_,
                                     testing::_, testing::_, testing::_))
        .WillByDefault([&](std::string_view, const Pairs&, std::string_view,
                           const Pairs&, int, uint32_t* token_ptr) {
          root_context_->onHttpCallResponse(
              *token_ptr, http_call_headers_.size(), http_call_body_.size(), 0);
          return WasmResult::Ok;
        });

    // Initialize Wasm sandbox context
    root_context_ = std::make_unique<PluginRootContext>(0, "");
    context_ = std::make_unique<PluginContext>(1, root_context_.get());
  }
  ~OAuthTest() override {}

  std::unique_ptr<WasmBase> wasm_base_;
  std::unique_ptr<WasmVm> test_vm_;
  std::unique_ptr<MockContext> mock_context_;

  std::unique_ptr<PluginRootContext> root_context_;
  std::unique_ptr<PluginContext> context_;

  std::string path_;
  std::string method_;
  std::string authority_;
  std::string route_name_;
  std::string jwt_header_;
  std::string custom_header_;
  std::string content_type_;
  uint64_t current_time_;

  Pairs http_call_headers_;
  std::string http_call_body_;
};

TEST_F(OAuthTest, generateToken) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer1",
            "client_id": "9515b564-0b1d-11ee-9c4c-00163e1250b5",
            "client_secret": "9e55de56-0b1d-11ee-b8ec-00163e1250b5"
        }
    ],
    "auth_path": "test/token"
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  path_ = "/abc/test/token";
  method_ = "GET";
  EXPECT_CALL(*mock_context_,
              sendLocalResponse(
                  400, std::string_view("Authorize parameters are missing"),
                  testing::_, testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  path_ = "/abc/test/token?";
  method_ = "GET";
  EXPECT_CALL(*mock_context_,
              sendLocalResponse(400, std::string_view("grant_type is missing"),
                                testing::_, testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  path_ =
      "/abc/test/"
      "token?grant_type=client_credentials";
  method_ = "GET";
  EXPECT_CALL(*mock_context_,
              sendLocalResponse(400, std::string_view("client_id is missing"),
                                testing::_, testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  path_ =
      "/abc/test/"
      "token?grant_type=client_credentials&client_id=9515b564-0b1d-11ee-9c4c-"
      "00163e1250b5&client_secret=abcd";
  method_ = "GET";
  EXPECT_CALL(*mock_context_,
              sendLocalResponse(
                  400, std::string_view("invalid client_id or client_secret"),
                  testing::_, testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  path_ =
      "/abc/test/"
      "token?grant_type=client_credentials&client_id=9515b564-0b1d-11ee-9c4c-"
      "00163e1250b5&client_secret=9e55de56-0b1d-11ee-b8ec-00163e1250b5";
  method_ = "GET";
  EXPECT_CALL(*mock_context_, sendLocalResponse(200, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  path_ = "/abc/test/token";
  method_ = "POST";
  content_type_ = "application/x-www-form-urlencoded; charset=utf8";
  std::string body = "grant_type=client_credentials&client_id=wrongid";
  BufferBase body_buffer;
  body_buffer.set({body.data(), body.size()});
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::HttpRequestBody))
      .WillOnce([&body_buffer](WasmBufferType) { return &body_buffer; });
  EXPECT_CALL(*mock_context_,
              sendLocalResponse(
                  400, std::string_view("invalid client_id or client_secret"),
                  testing::_, testing::_, testing::_));
  EXPECT_EQ(context_->onRequestBody(body.size(), true),
            FilterDataStatus::StopIterationNoBuffer);

  path_ = "/abc/test/token";
  method_ = "POST";
  content_type_ = "application/x-www-form-urlencoded; charset=utf8";
  body =
      "grant_type=client_credentials&client_id=9515b564-0b1d-11ee-9c4c-"
      "00163e1250b5&client_secret=9e55de56-0b1d-11ee-b8ec-00163e1250b5";
  body_buffer;
  body_buffer.set({body.data(), body.size()});
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::HttpRequestBody))
      .WillOnce([&body_buffer](WasmBufferType) { return &body_buffer; });
  EXPECT_CALL(*mock_context_, sendLocalResponse(200, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestBody(body.size(), true),
            FilterDataStatus::Continue);
}

TEST_F(OAuthTest, invalidToken) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer1",
            "client_id": "9515b564-0b1d-11ee-9c4c-00163e1250b5",
            "client_secret": "9e55de56-0b1d-11ee-b8ec-00163e1250b5"
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  jwt_header_ = R"(Bearer alksdjf)";
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  jwt_header_ = R"(alksdjf)";
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uL2F0K2p3dCJ9.eyJhdWQiOiJkZWZhdWx0IiwiZXhwIjoxNjY1NjczODI5LCJpYXQiOjE2NjU2NzM4MTksImlzcyI6IkhpZ3Jlc3MtR2F0ZXdheSIsImp0aSI6IjEwOTU5ZDFiLThkNjEtNGRlYy1iZWE3LTk0ODEwMzc1YjYzYyIsInNjb3BlIjoidGVzdCIsInN1YiI6ImNvbnN1bWVyMiJ9.al7eoRdoNQlNx8HCqNesj7woiLOJmJLSqnZ)";
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
}

TEST_F(OAuthTest, malformedMatrix) {
  std::string configuration = R"({"issuer":"oauth-v0","consumers":[{"name":"probe","client_id":"probe-client","client_secret":"oauth-v0-test-key"}]})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  ASSERT_TRUE(root_context_->configure(configuration.size()));
  authority_ = "oauth.test";
  path_ = "/anything/native";
  method_ = "GET";
  route_name_ = "v0";
  const auto encode = [](const std::string& value) {
    return jwt::base::trim<jwt::alphabet::base64url>(
        jwt::base::encode<jwt::alphabet::base64url>(value));
  };
  const auto signed_token = [&encode](const std::string& header,
                                       const std::string& payload) {
    const auto input = encode(header) + "." + encode(payload);
    std::error_code error;
    const auto signature = jwt::algorithm::hs256{"oauth-v0-test-key"}.sign(input, error);
    EXPECT_FALSE(error);
    return input + "." + encode(signature);
  };
  const std::string header = R"({"alg":"HS256","typ":"application/at+jwt"})";
  const std::string payload = R"({"client_id":"probe-client","iss":"oauth-v0","sub":"probe","aud":"v0","exp":4102444800,"iat":1700000000})";
  const auto valid = signed_token(header, payload);
  const auto first = valid.find('.');
  const auto second = valid.find('.', first + 1);
  const auto header_segment = valid.substr(0, first);
  const auto payload_segment = valid.substr(first + 1, second - first - 1);
  const auto signature_segment = valid.substr(second + 1);
  const std::vector<std::pair<std::string, std::string>> cases = {
      {"no-separator", "alksdjf"},
      {"one-separator", "a.b"},
      {"extra-separator", valid + ".extra"},
      {"header-alphabet", "%%%%." + payload_segment + "." + signature_segment},
      {"payload-alphabet", header_segment + ".%%%%." + signature_segment},
      {"signature-alphabet", header_segment + "." + payload_segment + ".%%%%"},
      {"header-padding", "====." + payload_segment + "." + signature_segment},
      {"payload-size", header_segment + ".a." + signature_segment},
      {"signature-size", header_segment + "." + payload_segment + ".a"},
      {"header-json", signed_token("{", payload)},
      {"payload-json", signed_token(header, "{")},
      {"header-array", signed_token("[]", payload)},
      {"payload-array", signed_token(header, "[]")},
      {"header-null", signed_token("null", payload)},
      {"payload-null", signed_token(header, "null")},
      {"header-overflow", signed_token(R"({"alg":"HS256","number":1e10000})", payload)},
      {"payload-overflow", signed_token(header, payload.substr(0, payload.size() - 1) + R"(,"number":1e10000})")},
      {"algorithm-null", signed_token(R"({"alg":null,"typ":"application/at+jwt"})", payload)},
      {"client-null", signed_token(header, R"({"client_id":null,"iss":"oauth-v0","sub":"probe","aud":"v0","exp":4102444800,"iat":1700000000})")},
      {"iat-wrong-type", signed_token(header, R"({"client_id":"probe-client","iss":"oauth-v0","sub":"probe","aud":"v0","exp":4102444800,"iat":"wrong"})")},
      {"exp-null", signed_token(header, R"({"client_id":"probe-client","iss":"oauth-v0","sub":"probe","aud":"v0","exp":null,"iat":1700000000})")},
      {"invalid-signature", header_segment + "." + payload_segment + "." + encode("wrong-signature")},
      {"unknown-consumer", signed_token(header, R"({"client_id":"unknown","iss":"oauth-v0","sub":"probe","aud":"v0","exp":4102444800,"iat":1700000000})")},
      {"wrong-issuer", signed_token(header, R"({"client_id":"probe-client","iss":"wrong","sub":"probe","aud":"v0","exp":4102444800,"iat":1700000000})")},
      {"wrong-subject", signed_token(header, R"({"client_id":"probe-client","iss":"oauth-v0","sub":"wrong","aud":"v0","exp":4102444800,"iat":1700000000})")},
  };
  for (const auto& entry : cases) {
    SCOPED_TRACE(entry.first);
    jwt_header_ = "Bearer " + entry.second;
    EXPECT_CALL(*mock_context_, sendLocalResponse(
        401, std::string_view("Invalid Jwt token"),
        testing::ElementsAre(testing::Pair("WWW-Authenticate", "Bearer realm=\"http://oauth.test/anything/native\"")),
        testing::_, std::string_view("oauth_access_denied"))).Times(1);
    EXPECT_NO_THROW({ EXPECT_EQ(context_->onRequestHeaders(0, false), FilterHeadersStatus::StopIteration); });
    testing::Mock::VerifyAndClearExpectations(mock_context_.get());
    jwt_header_ = "Bearer " + valid;
    EXPECT_CALL(*mock_context_, sendLocalResponse(testing::_, testing::_, testing::_, testing::_, testing::_)).Times(0);
    EXPECT_CALL(*mock_context_, addHeaderMapValue(WasmHeaderMapType::RequestHeaders, std::string_view("X-Mse-Consumer"), std::string_view("probe"))).Times(1);
    EXPECT_NO_THROW({ EXPECT_EQ(context_->onRequestHeaders(0, false), FilterHeadersStatus::Continue); });
    testing::Mock::VerifyAndClearExpectations(mock_context_.get());
  }
}

TEST_F(OAuthTest, expire) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer1",
            "client_id": "9515b564-0b1d-11ee-9c4c-00163e1250b5",
            "client_secret": "9e55de56-0b1d-11ee-b8ec-00163e1250b5"
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uL2F0K2p3dCJ9.eyJhdWQiOiJ0ZXN0MiIsImNsaWVudF9pZCI6Ijk1MTViNTY0LTBiMWQtMTFlZS05YzRjLTAwMTYzZTEyNTBiNSIsImV4cCI6MTY2NTY3MzgyOSwiaWF0IjoxNjY1NjczODE5LCJpc3MiOiJIaWdyZXNzLUdhdGV3YXkiLCJqdGkiOiIxMDk1OWQxYi04ZDYxLTRkZWMtYmVhNy05NDgxMDM3NWI2M2MiLCJzY29wZSI6InRlc3QiLCJzdWIiOiJjb25zdW1lcjEifQ.LsZ6mlRxlaqWa0IAZgmGVuDgypRbctkTcOyoCxqLrHY)";
  route_name_ = "test2";
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
}

TEST_F(OAuthTest, routeAuth) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer1",
            "client_id": "9515b564-0b1d-11ee-9c4c-00163e1250b5",
            "client_secret": "9e55de56-0b1d-11ee-b8ec-00163e1250b5"
        }
    ],
    "global_credentials": false,
    "clock_skew_seconds": 3153600000
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uL2F0K2p3dCJ9.eyJhdWQiOiJ0ZXN0MiIsImNsaWVudF9pZCI6Ijk1MTViNTY0LTBiMWQtMTFlZS05YzRjLTAwMTYzZTEyNTBiNSIsImV4cCI6MTY2NTY3MzgyOSwiaWF0IjoxNjY1NjczODE5LCJpc3MiOiJIaWdyZXNzLUdhdGV3YXkiLCJqdGkiOiIxMDk1OWQxYi04ZDYxLTRkZWMtYmVhNy05NDgxMDM3NWI2M2MiLCJzY29wZSI6InRlc3QiLCJzdWIiOiJjb25zdW1lcjEifQ.LsZ6mlRxlaqWa0IAZgmGVuDgypRbctkTcOyoCxqLrHY)";
  EXPECT_CALL(*mock_context_, sendLocalResponse(403, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  route_name_ = "test2";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(OAuthTest, globalAuth) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer1",
            "client_id": "9515b564-0b1d-11ee-9c4c-00163e1250b5",
            "client_secret": "9e55de56-0b1d-11ee-b8ec-00163e1250b5"
        }
    ],
    "clock_skew_seconds": 3153600000
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uL2F0K2p3dCJ9.eyJhdWQiOiJ0ZXN0MiIsImNsaWVudF9pZCI6Ijk1MTViNTY0LTBiMWQtMTFlZS05YzRjLTAwMTYzZTEyNTBiNSIsImV4cCI6MTY2NTY3MzgyOSwiaWF0IjoxNjY1NjczODE5LCJpc3MiOiJIaWdyZXNzLUdhdGV3YXkiLCJqdGkiOiIxMDk1OWQxYi04ZDYxLTRkZWMtYmVhNy05NDgxMDM3NWI2M2MiLCJzY29wZSI6InRlc3QiLCJzdWIiOiJjb25zdW1lcjEifQ.LsZ6mlRxlaqWa0IAZgmGVuDgypRbctkTcOyoCxqLrHY)";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(OAuthTest, AuthZ) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer1",
            "client_id": "9515b564-0b1d-11ee-9c4c-00163e1250b5",
            "client_secret": "9e55de56-0b1d-11ee-b8ec-00163e1250b5"
        },
        {
            "name": "consumer2",
            "client_id": "d001d242-0bf0-11ee-97cb-00163e1250b5",
            "client_secret": "d60bdafc-0bf0-11ee-afba-00163e1250b5"
        }
    ],
    "clock_skew_seconds": 3153600000,
    "global_credentials": true,
    "_rules_": [
        {
            "_match_route_": [
                "test1"
            ],
            "allow": [
                "consumer2"
            ]
        },
        {
            "_match_route_": [
                "test2"
            ],
            "allow": [
                "consumer1"
            ]
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uL2F0K2p3dCJ9.eyJhdWQiOiJ0ZXN0MiIsImNsaWVudF9pZCI6Ijk1MTViNTY0LTBiMWQtMTFlZS05YzRjLTAwMTYzZTEyNTBiNSIsImV4cCI6MTY2NTY3MzgyOSwiaWF0IjoxNjY1NjczODE5LCJpc3MiOiJIaWdyZXNzLUdhdGV3YXkiLCJqdGkiOiIxMDk1OWQxYi04ZDYxLTRkZWMtYmVhNy05NDgxMDM3NWI2M2MiLCJzY29wZSI6InRlc3QiLCJzdWIiOiJjb25zdW1lcjEifQ.LsZ6mlRxlaqWa0IAZgmGVuDgypRbctkTcOyoCxqLrHY)";
  route_name_ = "test1";
  EXPECT_CALL(*mock_context_, sendLocalResponse(403, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  route_name_ = "test2";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uL2F0K2p3dCJ9.eyJhdWQiOiJkZWZhdWx0IiwiY2xpZW50X2lkIjoiZDAwMWQyNDItMGJmMC0xMWVlLTk3Y2ItMDAxNjNlMTI1MGI1IiwiZXhwIjoxNjY1NjczODI5LCJpYXQiOjE2NjU2NzM4MTksImlzcyI6IkhpZ3Jlc3MtR2F0ZXdheSIsImp0aSI6IjEwOTU5ZDFiLThkNjEtNGRlYy1iZWE3LTk0ODEwMzc1YjYzYyIsInNjb3BlIjoidGVzdCIsInN1YiI6ImNvbnN1bWVyMiJ9.whS5U7llGX2BNAX19mjyxiWXa7wVs0_ONVByKVR9ntM)";
  route_name_ = "test2";
  EXPECT_CALL(*mock_context_, sendLocalResponse(403, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  route_name_ = "test1";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(OAuthTest, EmptyConsumer) {
  std::string configuration = R"(
{
    "consumers": [
    ],
    "_rules_": [
        {
            "_match_route_": [
                "test1"
            ],
            "allow": [
            ]
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uL2F0K2p3dCJ9.eyJhdWQiOiJ0ZXN0MiIsImNsaWVudF9pZCI6Ijk1MTViNTY0LTBiMWQtMTFlZS05YzRjLTAwMTYzZTEyNTBiNSIsImV4cCI6MTY2NTY3MzgyOSwiaWF0IjoxNjY1NjczODE5LCJpc3MiOiJIaWdyZXNzLUdhdGV3YXkiLCJqdGkiOiIxMDk1OWQxYi04ZDYxLTRkZWMtYmVhNy05NDgxMDM3NWI2M2MiLCJzY29wZSI6InRlc3QiLCJzdWIiOiJjb25zdW1lcjEifQ.LsZ6mlRxlaqWa0IAZgmGVuDgypRbctkTcOyoCxqLrHY)";
  route_name_ = "test1";
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  route_name_ = "test2";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

}  // namespace oauth
}  // namespace null_plugin
}  // namespace proxy_wasm
