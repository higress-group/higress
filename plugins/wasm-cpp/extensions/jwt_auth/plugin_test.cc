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

#include "extensions/jwt_auth/plugin.h"

#include "common/base64.h"
#include "gmock/gmock.h"
#include "gtest/gtest.h"
#include "include/proxy-wasm/context.h"
#include "include/proxy-wasm/null.h"

namespace proxy_wasm {
namespace null_plugin {
namespace jwt_auth {

NullPluginRegistry* context_registry_;
RegisterNullVmPluginFactory register_jwt_auth_plugin("jwt_auth", []() {
  return std::make_unique<NullPlugin>(jwt_auth::context_registry_);
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
  MOCK_METHOD(WasmResult, addHeaderMapValue,
              (WasmHeaderMapType /* type */, std::string_view /* jwt */,
               std::string_view /* value */));
  MOCK_METHOD(WasmResult, replaceHeaderMapValue,
              (WasmHeaderMapType /* type */, std::string_view /* key */,
              std::string_view /* value */));
  MOCK_METHOD(WasmResult, removeHeaderMapValue,
              (WasmHeaderMapType /* type */, std::string_view /* key */));
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

class JwtAuthTest : public ::testing::Test {
 protected:
  JwtAuthTest() {
    // Initialize test VM
    test_vm_ = createNullVm();
    wasm_base_ = std::make_unique<WasmBase>(
        std::move(test_vm_), "test-vm", "", "",
        std::unordered_map<std::string, std::string>{},
        AllowedCapabilitiesMap{});
    wasm_base_->load("jwt_auth");
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
          if (header == ":path") {
            *result = path_;
          }
          if (header == "Authorization") {
            *result = jwt_header_;
          }
          if (header == "x-custom-header") {
            *result = custom_header_;
          }
          if (header == "x-custom-header") {
            *result = custom_header_;
          }
          std::string hdr(header);
          auto it = header_map_.find(hdr);
          if (it != header_map_.end()) {
            *result = it->second;
          }
          return WasmResult::Ok;
        });
    ON_CALL(*mock_context_, addHeaderMapValue(WasmHeaderMapType::RequestHeaders,
                                              testing::_, testing::_))
        .WillByDefault([&](WasmHeaderMapType, std::string_view key, std::string_view value) {
          header_map_.emplace(key, value); 
          return WasmResult::Ok; 
        });

    ON_CALL(*mock_context_, replaceHeaderMapValue(WasmHeaderMapType::RequestHeaders,
                                              testing::_, testing::_))
        .WillByDefault([&](WasmHeaderMapType, std::string_view key, std::string_view value) {
          header_map_[std::string(key)] = value;
          return WasmResult::Ok;
        });

    ON_CALL(*mock_context_,
            removeHeaderMapValue(WasmHeaderMapType::RequestHeaders, testing::_))
        .WillByDefault([&](WasmHeaderMapType, std::string_view key) {
          header_map_.erase(std::string(key));
          return WasmResult::Ok;
        });

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
  ~JwtAuthTest() override {}

  std::unique_ptr<WasmBase> wasm_base_;
  std::unique_ptr<WasmVm> test_vm_;
  std::unique_ptr<MockContext> mock_context_;

  std::unique_ptr<PluginRootContext> root_context_;
  std::unique_ptr<PluginContext> context_;

  std::string path_;
  std::string authority_;
  std::string route_name_;
  std::string jwt_header_;
  std::string custom_header_;
  uint64_t current_time_;

  Pairs http_call_headers_;
  std::string http_call_body_;
  std::unordered_map<std::string, std::string> header_map_;
};

TEST_F(JwtAuthTest, RSA) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-1",
            "issuer": "abc",
            "jwks": "{\"keys\":[{\"kty\":\"RSA\",\"e\":\"AQAB\",\"use\":\"sig\",\"kid\":\"123\",\"alg\":\"RS256\",\"n\":\"i0B67f1jggT9QJlZ_8QL9QQ56LfurrqDhpuu8BxtVcfxrYmaXaCtqTn7OfCuca7cGHdrJIjq99rz890NmYFZuvhaZ-LMt2iyiSb9LZJAeJmHf7ecguXS_-4x3hvbsrgUDi9tlg7xxbqGYcrco3anmalAFxsbswtu2PAXLtTnUo6aYwZsWA6ksq4FL3-anPNL5oZUgIp3HGyhhLTLdlQcC83jzxbguOim-0OEz-N4fniTYRivK7MlibHKrJfO3xa_6whBS07HW4Ydc37ZN3Rx9Ov3ZyV0idFblU519nUdqp_inXj1eEpynlxH60Ys_aTU2POGZh_25KXGdF_ZC_MSRw\"}]}"
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  current_time_ = 1665673819 * 1e9;
  jwt_header_ =
      R"(Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmMiLCJzdWIiOiJ0ZXN0IiwiaWF0IjoxNjY1NjYwNTI3LCJleHAiOjE2NjU2NzM4MTl9.FwSnlW9NjZ_5w6cm-YqteUy4LjKCXfQCWVCGcM3RsaqBhcHTz_IFOFMLnjI9QAG_IhxPP4s0ln7-duESns4YogkmqWV0ckMKZo9OEYOLpD6kXaA6H6g9RaLedogReKk1bDauFWFBrqMwvnxIqOIPj2ZOEQcKDVxO08mPSXb5-cxbvCA2rcmBk8_JHD8DBW990IfUCrsUFP4w4Zy3HlU__ZZhaCqzukI1ZOOgwu2_wMifvdv2n2PvqRNcmpjuGJ-FUXhAduCTPO9ZLGBOZcxkPl4U28Frfb1hSEV83NfK3iPBoLjC3u-M7kc1FJHcUORy_Bof6mzBX7npYckbsb-SJA)";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(JwtAuthTest, OCT) {
  {
    std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-2",
            "issuer": "abcd",
            "jwks": "{\"keys\":[{\"kty\":\"oct\",\"kid\":\"123\",\"k\":\"hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew\",\"alg\":\"HS256\"}]}"
        }
    ]
})";
    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
    current_time_ = 1665673819 * 1e9;
    jwt_header_ =
        R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKmc)";
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::Continue);
  }
  {
    std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-2",
            "issuer": "abcd",
            "jwks": "{\"keys\":[{\"kty\":\"oct\",\"kid\":\"123\",\"k\":\"hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew\",\"alg\":\"HS256\"}]}"
        }
    ]
})";
    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
    current_time_ = 1665673819 * 1e9;
    jwt_header_ =
        R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKm1)";
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::StopIteration);
  }
  {
    std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-2",
            "issuer": "abcd",
            "jwks": "{\"keys\":[{\"kty\":\"oct\",\"kid\":\"123\",\"k\":\"hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew\",\"alg\":\"HS256\"}]}"
        }
    ],
   "global_auth": false
})";
    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
    current_time_ = 1665673819 * 1e9;
    jwt_header_ =
        R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKm1)";
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::Continue);
  }
}

TEST_F(JwtAuthTest, EmptyConsumer) {
  std::string configuration = R"(
{
    "consumers": [],
    "_rules_": [{
            "_match_route_": [
                "test1"
            ],
            "allow": []
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  current_time_ = 1665673819 * 1e9;
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKmc)";
  route_name_ = "test1";
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  route_name_ = "test2";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(JwtAuthTest, AuthZ) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-1",
            "issuer": "abc",
            "jwks": "{\"keys\":[{\"kty\":\"RSA\",\"e\":\"AQAB\",\"use\":\"sig\",\"kid\":\"123\",\"alg\":\"RS256\",\"n\":\"i0B67f1jggT9QJlZ_8QL9QQ56LfurrqDhpuu8BxtVcfxrYmaXaCtqTn7OfCuca7cGHdrJIjq99rz890NmYFZuvhaZ-LMt2iyiSb9LZJAeJmHf7ecguXS_-4x3hvbsrgUDi9tlg7xxbqGYcrco3anmalAFxsbswtu2PAXLtTnUo6aYwZsWA6ksq4FL3-anPNL5oZUgIp3HGyhhLTLdlQcC83jzxbguOim-0OEz-N4fniTYRivK7MlibHKrJfO3xa_6whBS07HW4Ydc37ZN3Rx9Ov3ZyV0idFblU519nUdqp_inXj1eEpynlxH60Ys_aTU2POGZh_25KXGdF_ZC_MSRw\"}]}"
        },
        {
            "name": "consumer-2",
            "issuer": "abcd",
            "jwks": "{\"keys\":[{\"kty\":\"oct\",\"kid\":\"123\",\"k\":\"hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew\",\"alg\":\"HS256\"}]}"
        }
    ],
    "_rules_": [{
            "_match_route_": [
                "test3"
            ],
            "allow": [
                "consumer-1"
            ],
            "_disable_": false
        },
        {
            "_match_route_": [
                "test2"
            ],
            "allow": [
                "consumer-2"
            ]
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  current_time_ = 1665673819 * 1e9;
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKmc)";
  route_name_ = "test3";
  EXPECT_CALL(*mock_context_, sendLocalResponse(403, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  route_name_ = "test2";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(JwtAuthTest, FallbackMarkerDoesNotBypassAuthentication) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-1",
            "issuer": "abc",
            "jwks": "{\"keys\":[{\"kty\":\"RSA\",\"e\":\"AQAB\",\"use\":\"sig\",\"kid\":\"123\",\"alg\":\"RS256\",\"n\":\"i0B67f1jggT9QJlZ_8QL9QQ56LfurrqDhpuu8BxtVcfxrYmaXaCtqTn7OfCuca7cGHdrJIjq99rz890NmYFZuvhaZ-LMt2iyiSb9LZJAeJmHf7ecguXS_-4x3hvbsrgUDi9tlg7xxbqGYcrco3anmalAFxsbswtu2PAXLtTnUo6aYwZsWA6ksq4FL3-anPNL5oZUgIp3HGyhhLTLdlQcC83jzxbguOim-0OEz-N4fniTYRivK7MlibHKrJfO3xa_6whBS07HW4Ydc37ZN3Rx9Ov3ZyV0idFblU519nUdqp_inXj1eEpynlxH60Ys_aTU2POGZh_25KXGdF_ZC_MSRw\"}]}"
        }
    ],
    "_rules_": [
        {
            "_match_route_": ["test"],
            "allow": ["consumer-1"]
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  route_name_ = "test";
  header_map_["x-higress-fallback-from"] = "original-cluster";
  header_map_["X-Mse-Consumer"] = "consumer-1";

  // Both headers are client-settable, so a marker plus a claimed consumer must
  // not authenticate anything: no JWT was presented.
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  // The consumer header alone must not bypass authentication.
  header_map_.erase("x-higress-fallback-from");
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  // A fallback marker without a consumer must also use normal authentication.
  header_map_["x-higress-fallback-from"] = "original-cluster";
  header_map_.erase("X-Mse-Consumer");
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
}

TEST_F(JwtAuthTest, FallbackConsumerMustPassAuthorization) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-1",
            "issuer": "abc",
            "jwks": "{\"keys\":[{\"kty\":\"RSA\",\"e\":\"AQAB\",\"use\":\"sig\",\"kid\":\"123\",\"alg\":\"RS256\",\"n\":\"i0B67f1jggT9QJlZ_8QL9QQ56LfurrqDhpuu8BxtVcfxrYmaXaCtqTn7OfCuca7cGHdrJIjq99rz890NmYFZuvhaZ-LMt2iyiSb9LZJAeJmHf7ecguXS_-4x3hvbsrgUDi9tlg7xxbqGYcrco3anmalAFxsbswtu2PAXLtTnUo6aYwZsWA6ksq4FL3-anPNL5oZUgIp3HGyhhLTLdlQcC83jzxbguOim-0OEz-N4fniTYRivK7MlibHKrJfO3xa_6whBS07HW4Ydc37ZN3Rx9Ov3ZyV0idFblU519nUdqp_inXj1eEpynlxH60Ys_aTU2POGZh_25KXGdF_ZC_MSRw\"}]}"
        }
    ],
    "_rules_": [
        {
            "_match_route_": ["test"],
            "allow": ["consumer-1"]
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  route_name_ = "test";
  header_map_["x-higress-fallback-from"] = "original-cluster";
  header_map_["X-Mse-Consumer"] = "consumer-2";

  // An off-allow-list consumer used to reach authorization and fail with 403.
  // No consumer is established without a verified JWT, so the request now
  // fails authentication with 401.
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
}

// RS256 token for issuer "abc", signed by the "consumer-1" key below and valid
// at the current_time_ used by the fallback tests. The tampered variant differs
// only in the first character of the signature.
constexpr std::string_view kValidOriginalAuthJwt =
    R"(Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmMiLCJzdWIiOiJ0ZXN0IiwiaWF0IjoxNjY1NjYwNTI3LCJleHAiOjE2NjU2NzM4MTl9.FwSnlW9NjZ_5w6cm-YqteUy4LjKCXfQCWVCGcM3RsaqBhcHTz_IFOFMLnjI9QAG_IhxPP4s0ln7-duESns4YogkmqWV0ckMKZo9OEYOLpD6kXaA6H6g9RaLedogReKk1bDauFWFBrqMwvnxIqOIPj2ZOEQcKDVxO08mPSXb5-cxbvCA2rcmBk8_JHD8DBW990IfUCrsUFP4w4Zy3HlU__ZZhaCqzukI1ZOOgwu2_wMifvdv2n2PvqRNcmpjuGJ-FUXhAduCTPO9ZLGBOZcxkPl4U28Frfb1hSEV83NfK3iPBoLjC3u-M7kc1FJHcUORy_Bof6mzBX7npYckbsb-SJA)";
constexpr std::string_view kTamperedOriginalAuthJwt =
    R"(Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmMiLCJzdWIiOiJ0ZXN0IiwiaWF0IjoxNjY1NjYwNTI3LCJleHAiOjE2NjU2NzM4MTl9.GwSnlW9NjZ_5w6cm-YqteUy4LjKCXfQCWVCGcM3RsaqBhcHTz_IFOFMLnjI9QAG_IhxPP4s0ln7-duESns4YogkmqWV0ckMKZo9OEYOLpD6kXaA6H6g9RaLedogReKk1bDauFWFBrqMwvnxIqOIPj2ZOEQcKDVxO08mPSXb5-cxbvCA2rcmBk8_JHD8DBW990IfUCrsUFP4w4Zy3HlU__ZZhaCqzukI1ZOOgwu2_wMifvdv2n2PvqRNcmpjuGJ-FUXhAduCTPO9ZLGBOZcxkPl4U28Frfb1hSEV83NfK3iPBoLjC3u-M7kc1FJHcUORy_Bof6mzBX7npYckbsb-SJA)";

TEST_F(JwtAuthTest, FallbackRequestVerifiesJwtFromOriginalAuthHeader) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-1",
            "issuer": "abc",
            "jwks": "{\"keys\":[{\"kty\":\"RSA\",\"e\":\"AQAB\",\"use\":\"sig\",\"kid\":\"123\",\"alg\":\"RS256\",\"n\":\"i0B67f1jggT9QJlZ_8QL9QQ56LfurrqDhpuu8BxtVcfxrYmaXaCtqTn7OfCuca7cGHdrJIjq99rz890NmYFZuvhaZ-LMt2iyiSb9LZJAeJmHf7ecguXS_-4x3hvbsrgUDi9tlg7xxbqGYcrco3anmalAFxsbswtu2PAXLtTnUo6aYwZsWA6ksq4FL3-anPNL5oZUgIp3HGyhhLTLdlQcC83jzxbguOim-0OEz-N4fniTYRivK7MlibHKrJfO3xa_6whBS07HW4Ydc37ZN3Rx9Ov3ZyV0idFblU519nUdqp_inXj1eEpynlxH60Ys_aTU2POGZh_25KXGdF_ZC_MSRw\"}]}"
        }
    ],
    "_rules_": [
        {
            "_match_route_": ["test"],
            "allow": ["consumer-1"]
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  route_name_ = "test";
  current_time_ = 1665673819 * 1e9;
  header_map_["x-higress-fallback-from"] = "original-cluster";
  header_map_["x-hi-original-auth"] = std::string(kValidOriginalAuthJwt);

  // Authorization is the only token source on a fallback request, so reaching
  // Continue proves the extractor picked the JWT up from x-hi-original-auth and
  // verified its signature.
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(header_map_["X-Mse-Consumer"], "consumer-1");
}

TEST_F(JwtAuthTest, FallbackRequestRejectsTamperedOriginalAuthJwt) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-1",
            "issuer": "abc",
            "jwks": "{\"keys\":[{\"kty\":\"RSA\",\"e\":\"AQAB\",\"use\":\"sig\",\"kid\":\"123\",\"alg\":\"RS256\",\"n\":\"i0B67f1jggT9QJlZ_8QL9QQ56LfurrqDhpuu8BxtVcfxrYmaXaCtqTn7OfCuca7cGHdrJIjq99rz890NmYFZuvhaZ-LMt2iyiSb9LZJAeJmHf7ecguXS_-4x3hvbsrgUDi9tlg7xxbqGYcrco3anmalAFxsbswtu2PAXLtTnUo6aYwZsWA6ksq4FL3-anPNL5oZUgIp3HGyhhLTLdlQcC83jzxbguOim-0OEz-N4fniTYRivK7MlibHKrJfO3xa_6whBS07HW4Ydc37ZN3Rx9Ov3ZyV0idFblU519nUdqp_inXj1eEpynlxH60Ys_aTU2POGZh_25KXGdF_ZC_MSRw\"}]}"
        }
    ],
    "_rules_": [
        {
            "_match_route_": ["test"],
            "allow": ["consumer-1"]
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  route_name_ = "test";
  current_time_ = 1665673819 * 1e9;
  header_map_["x-higress-fallback-from"] = "original-cluster";
  header_map_["x-hi-original-auth"] = std::string(kTamperedOriginalAuthJwt);

  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
}

TEST_F(JwtAuthTest, RuleDisableTrue) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-1",
            "issuer": "abc",
            "jwks": "{\"keys\":[{\"kty\":\"RSA\",\"e\":\"AQAB\",\"use\":\"sig\",\"kid\":\"123\",\"alg\":\"RS256\",\"n\":\"i0B67f1jggT9QJlZ_8QL9QQ56LfurrqDhpuu8BxtVcfxrYmaXaCtqTn7OfCuca7cGHdrJIjq99rz890NmYFZuvhaZ-LMt2iyiSb9LZJAeJmHf7ecguXS_-4x3hvbsrgUDi9tlg7xxbqGYcrco3anmalAFxsbswtu2PAXLtTnUo6aYwZsWA6ksq4FL3-anPNL5oZUgIp3HGyhhLTLdlQcC83jzxbguOim-0OEz-N4fniTYRivK7MlibHKrJfO3xa_6whBS07HW4Ydc37ZN3Rx9Ov3ZyV0idFblU519nUdqp_inXj1eEpynlxH60Ys_aTU2POGZh_25KXGdF_ZC_MSRw\"}]}"
        },
        {
            "name": "consumer-2",
            "issuer": "abcd",
            "jwks": "{\"keys\":[{\"kty\":\"oct\",\"kid\":\"123\",\"k\":\"hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew\",\"alg\":\"HS256\"}]}"
        }
    ],
    "_rules_": [{
            "_match_route_": [
                "test1"
            ],
            "allow": [
                "consumer-1"
            ],
            "_disable_": true
        },
        {
            "_match_route_": [
                "test2"
            ],
            "allow": [
                "consumer-2"
            ]
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  current_time_ = 1665673819 * 1e9;
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKmc)";
  route_name_ = "test1";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(JwtAuthTest, ClaimToHeader) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-2",
            "issuer": "abcd",
            "claims_to_headers": [
              {
                "claim": "sub",
                "header": "x-sub"
              },
              {
                "claim": "exp",
                "header": "x-exp"
              }
            ],
            "jwks": "{\"keys\":[{\"kty\":\"oct\",\"kid\":\"123\",\"k\":\"hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew\",\"alg\":\"HS256\"}]}"
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  current_time_ = 1665673819 * 1e9;
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKmc)";
  EXPECT_CALL(*mock_context_,
              replaceHeaderMapValue(testing::_, std::string_view("X-Mse-Consumer"),
                                    std::string_view("consumer-2")));
  EXPECT_CALL(*mock_context_,
              replaceHeaderMapValue(
                  testing::_, std::string_view("X-Mse-Consumer-Group"),
                  std::string_view("")));
  EXPECT_CALL(*mock_context_,
              addHeaderMapValue(testing::_, std::string_view("x-sub"),
                                std::string_view("test")));
  EXPECT_CALL(*mock_context_,
              addHeaderMapValue(testing::_, std::string_view("x-exp"),
                                std::string_view("1665673819")));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(JwtAuthTest, ConsumerGroupHeaderCommaJoined) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-2",
            "issuer": "abcd",
            "group": ["partner-a", "partner-b"],
            "jwks": "{\"keys\":[{\"kty\":\"oct\",\"kid\":\"123\",\"k\":\"hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew\",\"alg\":\"HS256\"}]}"
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  current_time_ = 1665673819 * 1e9;
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKmc)";
  header_map_.clear();
  header_map_.emplace("X-Mse-Consumer-Group", "spoofed");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(header_map_["X-Mse-Consumer"], "consumer-2");
  EXPECT_EQ(header_map_["X-Mse-Consumer-Group"], "partner-a,partner-b");
}

TEST_F(JwtAuthTest, ClientSuppliedConsumerHeaderIsReplaced) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-2",
            "issuer": "abcd",
            "group": ["partner-a", "partner-b"],
            "jwks": "{\"keys\":[{\"kty\":\"oct\",\"kid\":\"123\",\"k\":\"hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew\",\"alg\":\"HS256\"}]}"
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  current_time_ = 1665673819 * 1e9;
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKmc)";
  header_map_.clear();
  header_map_.emplace("X-Mse-Consumer", "spoofed-consumer");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(header_map_["X-Mse-Consumer"], "consumer-2");
  EXPECT_EQ(header_map_["X-Mse-Consumer-Group"], "partner-a,partner-b");
}

TEST_F(JwtAuthTest, ClientSuppliedConsumerHeaderRemovedOnSkipAuth) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-1",
            "issuer": "abc",
            "jwks": "{\"keys\":[{\"kty\":\"RSA\",\"e\":\"AQAB\",\"use\":\"sig\",\"kid\":\"123\",\"alg\":\"RS256\",\"n\":\"i0B67f1jggT9QJlZ_8QL9QQ56LfurrqDhpuu8BxtVcfxrYmaXaCtqTn7OfCuca7cGHdrJIjq99rz890NmYFZuvhaZ-LMt2iyiSb9LZJAeJmHf7ecguXS_-4x3hvbsrgUDi9tlg7xxbqGYcrco3anmalAFxsbswtu2PAXLtTnUo6aYwZsWA6ksq4FL3-anPNL5oZUgIp3HGyhhLTLdlQcC83jzxbguOim-0OEz-N4fniTYRivK7MlibHKrJfO3xa_6whBS07HW4Ydc37ZN3Rx9Ov3ZyV0idFblU519nUdqp_inXj1eEpynlxH60Ys_aTU2POGZh_25KXGdF_ZC_MSRw\"}]}"
        },
        {
            "name": "consumer-2",
            "issuer": "abcd",
            "jwks": "{\"keys\":[{\"kty\":\"oct\",\"kid\":\"123\",\"k\":\"hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew\",\"alg\":\"HS256\"}]}"
        }
    ],
    "enable_headers": ["x-custom-header"],
    "_rules_": [{
            "_match_route_": [
                "test1"
            ],
            "allow": [
                "consumer-1"
            ]
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  current_time_ = 1665673819 * 1e9;
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKmc)";
  route_name_ = "test1";
  custom_header_ = "";
  header_map_.clear();
  header_map_.emplace("X-Mse-Consumer", "spoofed-consumer");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(header_map_.find("X-Mse-Consumer"), header_map_.end());
}

TEST_F(JwtAuthTest, ConsumerGroupHeaderEmptyWhenOmitted) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-2",
            "issuer": "abcd",
            "jwks": "{\"keys\":[{\"kty\":\"oct\",\"kid\":\"123\",\"k\":\"hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew\",\"alg\":\"HS256\"}]}"
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  current_time_ = 1665673819 * 1e9;
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKmc)";
  header_map_.clear();
  header_map_.emplace("X-Mse-Consumer-Group", "spoofed");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  auto group_it = header_map_.find("X-Mse-Consumer-Group");
  ASSERT_NE(group_it, header_map_.end());
  EXPECT_EQ(group_it->second, "");
}

TEST_F(JwtAuthTest, RbacAllowsConsumerGroupPrincipal) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-2",
            "issuer": "abcd",
            "group": ["partner-a", "partner-b"],
            "jwks": "{\"keys\":[{\"kty\":\"oct\",\"kid\":\"123\",\"k\":\"hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew\",\"alg\":\"HS256\"}]}"
        }
    ],
    "rbac_rules": [
        {
            "principals": [
                {
                    "or_rules": [
                        {
                            "consumer_group": " partner-b "
                        }
                    ]
                }
            ],
            "permissions": [
                {
                    "route_name": {
                        "exact_match": "test"
                    }
                }
            ]
        }
    ],
    "_rules_": [
        {
            "_match_route_": [
                "test"
            ],
            "enable_auth": true
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  current_time_ = 1665673819 * 1e9;
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKmc)";
  route_name_ = "test";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(header_map_["X-Mse-Consumer-Group"], "partner-a,partner-b");
}

TEST_F(JwtAuthTest, RbacDeniesWhenConsumerGroupMismatch) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-2",
            "issuer": "abcd",
            "group": ["partner-b"],
            "jwks": "{\"keys\":[{\"kty\":\"oct\",\"kid\":\"123\",\"k\":\"hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew\",\"alg\":\"HS256\"}]}"
        }
    ],
    "rbac_rules": [
        {
            "principals": [
                {
                    "or_rules": [
                        {
                            "consumer_group": "partner-a"
                        }
                    ]
                }
            ],
            "permissions": [
                {
                    "route_name": {
                        "exact_match": "test"
                    }
                }
            ]
        }
    ],
    "_rules_": [
        {
            "_match_route_": [
                "test"
            ],
            "enable_auth": true
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  current_time_ = 1665673819 * 1e9;
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKmc)";
  route_name_ = "test";
  EXPECT_CALL(*mock_context_, sendLocalResponse(403, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
}

TEST_F(JwtAuthTest, CustomHeader) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-2",
            "issuer": "abcd",
            "from_headers": [
               {
                 "name": "x-custom-header",
                 "value_prefix": "token "
               },
               {
                 "name": "Authorization"
               }
            ],
            "jwks": "{\"keys\":[{\"kty\":\"oct\",\"kid\":\"123\",\"k\":\"hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew\",\"alg\":\"HS256\"}]}"
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  custom_header_ =
      R"(token eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKmc)";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  custom_header_.clear();
  jwt_header_ =
      R"(eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKmc)";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(JwtAuthTest, SkipAuthHeader) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-1",
            "issuer": "abc",
            "jwks": "{\"keys\":[{\"kty\":\"RSA\",\"e\":\"AQAB\",\"use\":\"sig\",\"kid\":\"123\",\"alg\":\"RS256\",\"n\":\"i0B67f1jggT9QJlZ_8QL9QQ56LfurrqDhpuu8BxtVcfxrYmaXaCtqTn7OfCuca7cGHdrJIjq99rz890NmYFZuvhaZ-LMt2iyiSb9LZJAeJmHf7ecguXS_-4x3hvbsrgUDi9tlg7xxbqGYcrco3anmalAFxsbswtu2PAXLtTnUo6aYwZsWA6ksq4FL3-anPNL5oZUgIp3HGyhhLTLdlQcC83jzxbguOim-0OEz-N4fniTYRivK7MlibHKrJfO3xa_6whBS07HW4Ydc37ZN3Rx9Ov3ZyV0idFblU519nUdqp_inXj1eEpynlxH60Ys_aTU2POGZh_25KXGdF_ZC_MSRw\"}]}"
        },
        {
            "name": "consumer-2",
            "issuer": "abcd",
            "jwks": "{\"keys\":[{\"kty\":\"oct\",\"kid\":\"123\",\"k\":\"hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew\",\"alg\":\"HS256\"}]}"
        }
    ],
    "enable_headers": ["x-custom-header"],
    "_rules_": [{
            "_match_route_": [
                "test1"
            ],
            "allow": [
                "consumer-1"
            ]
        },
        {
            "_match_route_": [
                "test2"
            ],
            "allow": [
                "consumer-2"
            ]
        }
    ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  current_time_ = 1665673819 * 1e9;
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKmc)";
  route_name_ = "test1";
  custom_header_ = "123";
  EXPECT_CALL(*mock_context_, sendLocalResponse(403, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  custom_header_ = "";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(JwtAuthTest, REMOTE_JWKS) {
  std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer",
            "issuer": "abcd",
            "remote_jwks": {
               "uri": "http://127.0.0.1/keys",
               "service": "test.static",
               "port": "80",
               "ttl": 3000,
               "timeout": 3000
            }
        }
    ]
})";
  http_call_headers_ = {{":status", "200"}};
  http_call_body_ = R"(
{
  "keys": [
    {
      "kty": "oct",
      "kid": "123",
      "k": "hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew",
      "alg": "HS256"
    }
  ]
}
)";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });

  EXPECT_TRUE(root_context_->configure(configuration.size()));
  current_time_ = 1665673819 * 1e9;
  jwt_header_ =
      R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKmc)";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_CALL(*mock_context_, sendLocalResponse(403, testing::_, testing::_,
                                                testing::_, testing::_))
      .Times(0);
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_))
      .Times(0);
}

TEST_F(JwtAuthTest, RbacAuthZ) {
    std::string configuration = R"(
{
    "consumers": [
        {
            "name": "consumer-1",
            "issuer": "abc",
            "jwks": "{\"keys\":[{\"kty\":\"RSA\",\"e\":\"AQAB\",\"use\":\"sig\",\"kid\":\"123\",\"alg\":\"RS256\",\"n\":\"i0B67f1jggT9QJlZ_8QL9QQ56LfurrqDhpuu8BxtVcfxrYmaXaCtqTn7OfCuca7cGHdrJIjq99rz890NmYFZuvhaZ-LMt2iyiSb9LZJAeJmHf7ecguXS_-4x3hvbsrgUDi9tlg7xxbqGYcrco3anmalAFxsbswtu2PAXLtTnUo6aYwZsWA6ksq4FL3-anPNL5oZUgIp3HGyhhLTLdlQcC83jzxbguOim-0OEz-N4fniTYRivK7MlibHKrJfO3xa_6whBS07HW4Ydc37ZN3Rx9Ov3ZyV0idFblU519nUdqp_inXj1eEpynlxH60Ys_aTU2POGZh_25KXGdF_ZC_MSRw\"}]}"
        },
        {
            "name": "consumer-2",
            "issuer": "abcd",
            "jwks": "{\"keys\":[{\"kty\":\"oct\",\"kid\":\"123\",\"k\":\"hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew\",\"alg\":\"HS256\"}]}"
        }
    ],
    "rbac_rules": [
        {
            "principals": [
                {
                    "or_rules": [
                        {
                            "consumer": "consumer-1"
                        }
                    ]
                }
            ],
            "permissions": [
                {
                    "or_rules": [
                        {
                            "route_name": {
                                "exact_match": "test1"
                            }
                        }
                    ]
                }
            ]
        },
        {
            "principals": [
                {
                    "or_rules": [
                        {
                            "consumer": "consumer-2"
                        }
                    ]
                }
            ],
            "permissions": [
                {
                    "or_rules": [
                        {
                            "route_name": {
                                "exact_match": "test2"
                            }
                        }
                    ]
                }
            ]
        }
    ],
    "_rules_": [
        {
            "_match_route_": [
                "test1",
                "test2"
            ],
            "enable_auth": true
        }
    ]
})";
    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});
  
    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
    current_time_ = 1665673819 * 1e9;
    jwt_header_ =
        R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKmc)";
    route_name_ = "test1";
    EXPECT_CALL(*mock_context_, sendLocalResponse(403, testing::_, testing::_,
                                                  testing::_, testing::_));
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::StopIteration);
    route_name_ = "test2";
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::Continue);
}

TEST_F(JwtAuthTest, RbacMcpToolAuthZ) {
    std::string configuration = R"EOF(
{
    "consumers": [
        {
            "name": "consumer-1",
            "issuer": "abc",
            "jwks": "{\"keys\":[{\"kty\":\"RSA\",\"e\":\"AQAB\",\"use\":\"sig\",\"kid\":\"123\",\"alg\":\"RS256\",\"n\":\"i0B67f1jggT9QJlZ_8QL9QQ56LfurrqDhpuu8BxtVcfxrYmaXaCtqTn7OfCuca7cGHdrJIjq99rz890NmYFZuvhaZ-LMt2iyiSb9LZJAeJmHf7ecguXS_-4x3hvbsrgUDi9tlg7xxbqGYcrco3anmalAFxsbswtu2PAXLtTnUo6aYwZsWA6ksq4FL3-anPNL5oZUgIp3HGyhhLTLdlQcC83jzxbguOim-0OEz-N4fniTYRivK7MlibHKrJfO3xa_6whBS07HW4Ydc37ZN3Rx9Ov3ZyV0idFblU519nUdqp_inXj1eEpynlxH60Ys_aTU2POGZh_25KXGdF_ZC_MSRw\"}]}"
        },
        {
            "name": "consumer-2",
            "issuer": "abcd",
            "jwks": "{\"keys\":[{\"kty\":\"oct\",\"kid\":\"123\",\"k\":\"hM0k3AbXBPpKOGg__Ql2Obcq7s60myWDpbHXzgKUQdYo7YCRp0gUqkCnbGSvZ2rGEl4YFkKqIqW7mTHdj-bcqXpNr-NOznEyMpVPOIlqG_NWVC3dydBgcsIZIdD-MR2AQceEaxriPA_VmiUCwfwL2Bhs6_i7eolXoY11EapLQtutz0BV6ZxQQ4dYUmct--7PLNb4BWJyQeWu0QfbIthnvhYllyl2dgeLTEJT58wzFz5HeNMNz8ohY5K0XaKAe5cepryqoXLhA-V-O1OjSG8lCNdKS09OY6O0fkyweKEtuDfien5tHHSsHXoAxYEHPFcSRL4bFPLZ0orTt1_4zpyfew\",\"alg\":\"HS256\"}]}"
        }
    ],
    "rbac_rules": [
        {
            "principals": [
                {
                    "or_rules": [
                        {
                            "consumer": "consumer-1"
                        }
                    ]
                }
            ],
            "permissions": [
                {
                    "or_rules": [
                        {
                            "route_name": {
                                "exact_match": "test1"
                            }
                        }
                    ]
                }
            ]
        },
        {
            "principals": [
                {
                    "or_rules": [
                        {
                            "consumer": "consumer-2"
                        }
                    ]
                }
            ],
            "permissions": [
                {
                    "or_rules": [
                        {
                            "and_rules": [
                                {
                                    "header": {
                                        "name": "x-envoy-mcp-tool-name",
                                        "safe_regex_match": {
                                            "google_re2": {},
                                            "regex": "(get_weather|get_ip)"
                                        }
                                    }
                                },
                                {
                                    "route_name": {
                                        "exact_match": "test2"
                                    }
                                }
                            ]
                        }
                    ]
                }
            ]
        }
    ],
    "_rules_": [
        {
            "_match_route_": [
                "test1",
                "test2"
            ],
            "enable_auth": true
        }
    ]
})EOF";
    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});
  
    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
    current_time_ = 1665673819 * 1e9;
    jwt_header_ =
        R"(Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IjEyMyJ9.eyJpc3MiOiJhYmNkIiwic3ViIjoidGVzdCIsImlhdCI6MTY2NTY2MDUyNywiZXhwIjoxNjY1NjczODE5fQ.7BVJOAobz_xYjsenu_CsYhYbgF1gMcqZSpaeQ8HwKmc)";
    route_name_ = "test1";
    // EXPECT_CALL(*mock_context_, sendLocalResponse(403, testing::_, testing::_,
    //                                               testing::_, testing::_));
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::StopIteration);
    route_name_ = "test2";
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::StopIteration);
    route_name_ = "test2";
    header_map_.emplace("x-envoy-mcp-tool-name", "get_location");
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::StopIteration);
    route_name_ = "test2";
    header_map_.clear();
    header_map_.emplace("x-envoy-mcp-tool-name", "get_weather");
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::Continue);
}


}  // namespace jwt_auth
}  // namespace null_plugin
}  // namespace proxy_wasm
