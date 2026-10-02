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

#include <cstdint>
#include <optional>

#include "common/base64.h"
#include "common/crypto_util.h"
#include "gmock/gmock.h"
#include "gtest/gtest.h"
#include "include/proxy-wasm/context.h"
#include "include/proxy-wasm/null.h"

namespace proxy_wasm {
namespace null_plugin {
namespace hmac_auth {

NullPluginRegistry* context_registry_;
RegisterNullVmPluginFactory register_hmac_auth_plugin("hmac_auth", []() {
  return std::make_unique<NullPlugin>(hmac_auth::context_registry_);
});

class MockContext : public proxy_wasm::ContextBase {
 public:
  MockContext(WasmBase* wasm) : ContextBase(wasm) {}

  MOCK_METHOD(BufferInterface*, getBuffer, (WasmBufferType));
  MOCK_METHOD(WasmResult, log, (uint32_t, std::string_view));
  MOCK_METHOD(WasmResult, getHeaderMapValue,
              (WasmHeaderMapType /* type */, std::string_view /* key */,
               std::string_view* /*result */));
  MOCK_METHOD(WasmResult, getHeaderMapPairs, (WasmHeaderMapType, Pairs*));
  MOCK_METHOD(WasmResult, addHeaderMapValue,
              (WasmHeaderMapType /* type */, std::string_view /* key */,
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
  MOCK_METHOD(WasmResult, setProperty, (std::string_view, std::string_view));
};

class HmacAuthTest : public ::testing::Test {
 protected:
  HmacAuthTest() {
    // Initialize test VM
    test_vm_ = createNullVm();
    wasm_base_ = std::make_unique<WasmBase>(
        std::move(test_vm_), "test-vm", "", "",
        std::unordered_map<std::string, std::string>{},
        AllowedCapabilitiesMap{});
    wasm_base_->load("hmac_auth");
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
          auto it = headers_.find(std::string(header));
          if (it == headers_.end()) {
            std::cerr << header << " not found.\n";
            return WasmResult::NotFound;
          }
          *result = it->second;
          return WasmResult::Ok;
        });
    ON_CALL(*mock_context_,
            getHeaderMapPairs(WasmHeaderMapType::RequestHeaders, testing::_))
        .WillByDefault([&](WasmHeaderMapType, Pairs* result) {
          header_pairs_.clear();
          for (const auto& [key, value] : headers_) {
            header_pairs_.push_back({key, value});
          }
          *result = header_pairs_;
          return WasmResult::Ok;
        });

    ON_CALL(*mock_context_, addHeaderMapValue(WasmHeaderMapType::RequestHeaders,
                                              testing::_, testing::_))
        .WillByDefault([&](WasmHeaderMapType, std::string_view key,
                           std::string_view value) {
          headers_[std::string(key)] = value;
          return WasmResult::Ok;
        });

    ON_CALL(*mock_context_,
            replaceHeaderMapValue(WasmHeaderMapType::RequestHeaders, testing::_,
                                  testing::_))
        .WillByDefault([&](WasmHeaderMapType, std::string_view key,
                           std::string_view value) {
          headers_[std::string(key)] = value;
          return WasmResult::Ok;
        });

    ON_CALL(*mock_context_,
            removeHeaderMapValue(WasmHeaderMapType::RequestHeaders, testing::_))
        .WillByDefault([&](WasmHeaderMapType, std::string_view key) {
          headers_.erase(std::string(key));
          return WasmResult::Ok;
        });

    ON_CALL(*mock_context_, getBuffer(testing::_))
        .WillByDefault([&](WasmBufferType type) {
          if (type == WasmBufferType::HttpRequestBody) {
            return &body_;
          }
          return &config_;
        });

    ON_CALL(*mock_context_, getCurrentTimeNanoseconds()).WillByDefault([&]() {
      return current_time_;
    });

    ON_CALL(*mock_context_, getProperty(testing::_, testing::_))
        .WillByDefault([&](std::string_view path, std::string* result) {
          *result = route_name_;
          return WasmResult::Ok;
        });

    ON_CALL(*mock_context_, setProperty(testing::_, testing::_))
        .WillByDefault(
            [&](std::string_view, std::string_view) { return WasmResult::Ok; });

    // Initialize Wasm sandbox context
    root_context_ = std::make_unique<PluginRootContext>(0, "");
    context_ = std::make_unique<PluginContext>(1, root_context_.get());
  }
  ~HmacAuthTest() override {}

  std::unique_ptr<WasmBase> wasm_base_;
  std::unique_ptr<WasmVm> test_vm_;
  std::unique_ptr<MockContext> mock_context_;

  std::unique_ptr<PluginRootContext> root_context_;
  std::unique_ptr<PluginContext> context_;

  std::map<std::string, std::string> headers_;
  std::string route_name_;
  BufferBase body_;
  BufferBase config_;
  uint64_t current_time_;
  Pairs header_pairs_;
};

TEST_F(HmacAuthTest, Sign) {
  headers_ = {
      {":path",
       "/http2test/test?param1=test&username=xiaoming&password=123456789"},
      {":method", "POST"},
      {"accept", "application/json; charset=utf-8"},
      {"ca_version", "1"},
      {"content-type", "application/x-www-form-urlencoded; charset=utf-8"},
      {"x-ca-timestamp", "1525872629832"},
      {"date", "Wed, 09 May 2018 13:30:29 GMT+00:00"},
      {"user-agent", "ALIYUN-ANDROID-DEMO"},
      {"x-ca-nonce", "c9f15cbf-f4ac-4a6c-b54d-f51abf4b5b44"},
      {"content-length", "33"},
      {"username", "xiaoming&password=123456789"},
      {"x-ca-key", "203753385"},
      {"x-ca-signature-method", "HmacSHA256"},
      {"x-ca-signature", "xfX+bZxY2yl7EB/qdoDy9v/uscw3Nnj1pgoU+Bm6xdM="},
      {"x-ca-signature-headers",
       "x-ca-timestamp,x-ca-key,x-ca-nonce,x-ca-signature-method"},
  };
  //   auto actual = root_context_->getStringToSign(
  //       "/http2test/test?param1=test&username=xiaoming&password=123456789",
  //       std::nullopt);
  //   EXPECT_EQ(actual, R"(POST
  // application/json; charset=utf-8

  // application/x-www-form-urlencoded; charset=utf-8
  // Wed, 09 May 2018 13:30:29 GMT+00:00
  // x-ca-key:203753385
  // x-ca-nonce:c9f15cbf-f4ac-4a6c-b54d-f51abf4b5b44
  // x-ca-signature-method:HmacSHA256
  // x-ca-timestamp:1525872629832
  // /http2test/test?param1=test&password=123456789&username=xiaoming)");

  headers_ = {
      {":path", "/Third/Tools/checkSign"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"content-type", "application/json"},
      {"x-ca-timestamp", "1646365291734"},
      {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
      {"x-ca-signature", "EdJSFAMOWyXZOpXhevZnjuS0ZafnwnCqaSk5hz+tXo8="},
  };
  HmacAuthConfigRule rule;
  rule.credentials = {{"appKey", "appSecret"}};
  //  EXPECT_EQ(root_context_->checkPlugin(rule, std::nullopt), true);

  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(HmacAuthTest, SignWithoutDynamicHeader) {
  headers_ = {
      {":path", "/Third/Tools/checkSign"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature", "ZpJhkHdtjLTJiR6CJWHL8ikLtPB2z6CoztG21wG3PT4="},
  };
  HmacAuthConfigRule rule;
  rule.credentials = {{"appKey", "appSecret"}};
  //  EXPECT_EQ(root_context_->checkPlugin(rule, std::nullopt), true);

  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(HmacAuthTest, SignWithConsumer) {
  headers_ = {
      {":path", "/Third/Tools/checkSign"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"content-type", "application/json"},
      {"x-ca-timestamp", "1646365291734"},
      {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
      {"x-ca-signature", "EdJSFAMOWyXZOpXhevZnjuS0ZafnwnCqaSk5hz+tXo8="},
  };
  HmacAuthConfigRule rule;
  rule.credentials = {{"appKey", "appSecret"}};
  //  EXPECT_EQ(root_context_->checkPlugin(rule, std::nullopt), true);

  std::string configuration = R"(
{
  "consumers": [{"key": "appKey", "secret": "appSecret", "name": "consumer"}],
  "_rules_": [
    {
      "_match_route_":["test"],
      "allow":["consumer"]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(HmacAuthTest, ParamInBody) {
  headers_ = {
      {":path", "/http2test/test?param1=test"},
      {":method", "POST"},
      {"accept", "application/json; charset=utf-8"},
      {"ca_version", "1"},
      {"content-type", "application/x-www-form-urlencoded; charset=utf-8"},
      {"x-ca-timestamp", "1525872629832"},
      {"date", "Wed, 09 May 2018 13:30:29 GMT+00:00"},
      {"user-agent", "ALIYUN-ANDROID-DEMO"},
      {"x-ca-nonce", "c9f15cbf-f4ac-4a6c-b54d-f51abf4b5b44"},
      {"content-length", "33"},
      {"username", "xiaoming&password=123456789"},
      {"x-ca-key", "203753385"},
      {"x-ca-signature-method", "HmacSHA256"},
      {"x-ca-signature", "xfX+bZxY2yl7EB/qdoDy9v/uscw3Nnj1pgoU+Bm6xdM="},
      {"x-ca-signature-headers",
       "x-ca-timestamp,x-ca-key,x-ca-nonce,x-ca-signature-method"},
  };
  Wasm::Common::Http::QueryParams body_params = {{"username", "xiaoming"},
                                                 {"password", "123456789"}};
  //   auto actual =
  //   root_context_->getStringToSign("/http2test/test?param1=test",
  //                                                body_params);
  //   EXPECT_EQ(actual, R"(POST
  // application/json; charset=utf-8

  // application/x-www-form-urlencoded; charset=utf-8
  // Wed, 09 May 2018 13:30:29 GMT+00:00
  // x-ca-key:203753385
  // x-ca-nonce:c9f15cbf-f4ac-4a6c-b54d-f51abf4b5b44
  // x-ca-signature-method:HmacSHA256
  // x-ca-timestamp:1525872629832
  // /http2test/test?param1=test&password=123456789&username=xiaoming)");

  headers_ = {
      {":path", "/Third/User/getNyAccessToken"},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "application/x-www-form-urlencoded"},
      {"content-length", "52"},
      {"x-ca-timestamp", "1646646682418"},
      {"x-ca-nonce", "ca5a6753-b76c-4fff-a9d9-e5bb643e8cdf"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
      {"x-ca-signature", "gmf9xq0hc95Hmt+7G+OocS009ka3v1v0rvfshKzYc3w="},
  };
  HmacAuthConfigRule rule;
  rule.credentials = {{"appKey", "appSecret"}};
  body_params = {{"nickname", "nickname"},
                 {"room_id", "6893"},
                 {"uuid", "uuid"},
                 {"photo", "photo"}};
  //  EXPECT_EQ(root_context_->checkPlugin(rule, body_params), true);
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  std::string body("nickname=nickname&room_id=6893&uuid=uuid&photo=photo");
  body_.set(body);
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(context_->onRequestBody(body.size(), true),
            FilterDataStatus::Continue);
}

TEST_F(HmacAuthTest, ParamInBodyWithConsumer) {
  headers_ = {
      {":path", "/Third/User/getNyAccessToken"},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "application/x-www-form-urlencoded"},
      {"content-length", "52"},
      {"x-ca-timestamp", "1646646682418"},
      {"x-ca-nonce", "ca5a6753-b76c-4fff-a9d9-e5bb643e8cdf"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
      {"x-ca-signature", "gmf9xq0hc95Hmt+7G+OocS009ka3v1v0rvfshKzYc3w="},
  };
  HmacAuthConfigRule rule;
  rule.credentials = {{"appKey", "appSecret"}};
  Wasm::Common::Http::QueryParams body_params = {{"nickname", "nickname"},
                                                 {"room_id", "6893"},
                                                 {"uuid", "uuid"},
                                                 {"photo", "photo"}};
  //  EXPECT_EQ(root_context_->checkPlugin(rule, body_params), true);
  std::string configuration = R"(
{
  "consumers": [{"key": "appKey", "secret": "appSecret", "name": "consumer"}],
  "_rules_": [
    {
      "_match_route_":["test"],
      "allow":["consumer"]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  std::string body("nickname=nickname&room_id=6893&uuid=uuid&photo=photo");
  body_.set(body);
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(context_->onRequestBody(body.size(), true),
            FilterDataStatus::Continue);
}

TEST_F(HmacAuthTest, ParamInBodyWrongSignature) {
  headers_ = {
      {":path", "/Third/User/getNyAccessToken"},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "application/x-www-form-urlencoded"},
      {"content-length", "52"},
      {"x-ca-timestamp", "1646646682418"},
      {"x-ca-nonce", "ca5a6753-b76c-4fff-a9d9-e5bb643e8cdf"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
      {"x-ca-signature", "wrong"},
  };
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  std::string body("nickname=nickname&room_id=6893&uuid=uuid&photo=photo");
  body_.set(body);
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_CALL(*mock_context_, sendLocalResponse(400, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestBody(body.size(), true),
            FilterDataStatus::StopIterationNoBuffer);
}

TEST_F(HmacAuthTest, InvalidSecret) {
  {
    headers_ = {
        {":path", "/Third/Tools/checkSign"},
        {":method", "GET"},
        {"accept", "application/json"},
        {"content-type", "application/json"},
        {"x-ca-timestamp", "1646365291734"},
        {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
        {"x-ca-key", "appKey"},
        {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
        {"x-ca-signature", "EdJSFAMOWyXZOpXhevZnjuS0ZafnwnCqaSk5hz+tXo8="},
    };
    std::string configuration = R"(
{
     "credentials":[
        {"key": "appKey", "secret": ""}
      ]
})";
    config_.set(configuration);
    EXPECT_TRUE(root_context_->configure(configuration.size()));
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::StopAllIterationAndBuffer);
  }

  {
    headers_ = {
        {":path", "/Third/Tools/checkSign"},
        {":method", "GET"},
        {"accept", "application/json"},
        {"content-type", "application/json"},
        {"x-ca-timestamp", "1646365291734"},
        {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
        {"x-ca-key", "appKey"},
        {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
        {"x-ca-signature", "EdJSFAMOWyXZOpXhevZnjuS0ZafnwnCqaSk5hz+tXo8="},
    };
    std::string configuration = R"(
{
     "consumers":[
        {"key": "appKey", "secret": "", "name": "consumer1"}
      ]
})";
    config_.set(configuration);
    EXPECT_TRUE(root_context_->configure(configuration.size()));
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::StopAllIterationAndBuffer);
  }
}

TEST_F(HmacAuthTest, GlobalAuthDisable) {
  {
    headers_ = {
        {":path", "/Third/Tools/checkSign"},
        {":method", "GET"},
        {"accept", "application/json"},
        {"content-type", "application/json"},
        {"x-ca-timestamp", "1646365291734"},
        {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
        {"x-ca-key", "appKey"},
        {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
        {"x-ca-signature", "EdJSFAMOWyXZOpXhevZnjuS0ZafnwnCqaSk5hz+tXo8="},
    };
    std::string configuration = R"(
{
     "consumers":[
        {"key": "appKey", "secret": "", "name": "consumer1"}
      ],
     "global_auth":false
})";
    config_.set(configuration);
    EXPECT_TRUE(root_context_->configure(configuration.size()));
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::Continue);
  }
  {
    headers_ = {
        {":path", "/Third/Tools/checkSign"},
        {":method", "GET"},
        {"accept", "application/json"},
        {"content-type", "application/json"},
        {"x-ca-timestamp", "1646365291734"},
        {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
        {"x-ca-key", "appKey"},
        {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
    };
    HmacAuthConfigRule rule;
    rule.credentials = {{"appKey", "appSecret"}};
    //  EXPECT_EQ(root_context_->checkPlugin(rule, std::nullopt), true);

    std::string configuration = R"(
{
  "consumers": [{"key": "appKey", "secret": "appSecret", "name": "consumer"}],
  "_rules_": [
    {
      "_match_route_":["test"],
      "allow":["consumer"]
    }
  ],
  "global_auth":false
})";
    route_name_ = "test2";
    config_.set(configuration);
    EXPECT_TRUE(root_context_->configure(configuration.size()));
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::Continue);
  }
}

TEST_F(HmacAuthTest, GlobalAuthEnable) {
  headers_ = {
      {":path", "/Third/Tools/checkSign"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"content-type", "application/json"},
      {"x-ca-timestamp", "1646365291734"},
      {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
  };
  HmacAuthConfigRule rule;
  rule.credentials = {{"appKey", "appSecret"}};
  //  EXPECT_EQ(root_context_->checkPlugin(rule, std::nullopt), true);

  std::string configuration = R"(
{
  "consumers": [{"key": "appKey", "secret": "appSecret", "name": "consumer"}],
  "_rules_": [
    {
      "_match_route_":["test"],
      "allow":["consumer"]
    }
  ],
  "global_auth":true
})";
  route_name_ = "test2";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopAllIterationAndBuffer);
}

TEST_F(HmacAuthTest, RuleDisableFalse) {
  {
    headers_ = {
        {":path", "/Third/Tools/checkSign"},
        {":method", "GET"},
        {"accept", "application/json"},
        {"content-type", "application/json"},
        {"x-ca-timestamp", "1646365291734"},
        {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
        {"x-ca-key", "appKey"},
        {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
    };
    HmacAuthConfigRule rule;
    rule.credentials = {{"appKey", "appSecret"}};
    //  EXPECT_EQ(root_context_->checkPlugin(rule, std::nullopt), true);

    std::string configuration = R"(
{
  "consumers": [{"key": "appKey", "secret": "appSecret", "name": "consumer"}],
  "_rules_": [
    {
      "_match_route_":["test"],
      "allow":[],
      "_disable_": false
    }
  ],
  "global_auth":true
})";
    route_name_ = "test";
    config_.set(configuration);
    EXPECT_TRUE(root_context_->configure(configuration.size()));
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::StopAllIterationAndBuffer);
  }
}

TEST_F(HmacAuthTest, RuleDisableTrue) {
  {
    headers_ = {
        {":path", "/Third/Tools/checkSign"},
        {":method", "GET"},
        {"accept", "application/json"},
        {"content-type", "application/json"},
        {"x-ca-timestamp", "1646365291734"},
        {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
        {"x-ca-key", "appKey"},
        {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
    };
    HmacAuthConfigRule rule;
    rule.credentials = {{"appKey", "appSecret"}};
    //  EXPECT_EQ(root_context_->checkPlugin(rule, std::nullopt), true);

    std::string configuration = R"(
{
  "consumers": [{"key": "appKey", "secret": "appSecret", "name": "consumer"}],
  "_rules_": [
    {
      "_match_route_":["test"],
      "allow":[],
      "_disable_": true
    }
  ],
  "global_auth":true
})";
    route_name_ = "test";
    config_.set(configuration);
    EXPECT_TRUE(root_context_->configure(configuration.size()));
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::Continue);
  }
}

TEST_F(HmacAuthTest, EmptyConsumer) {
  headers_ = {
      {":path", "/Third/Tools/checkSign"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"content-type", "application/json"},
      {"x-ca-timestamp", "1646365291734"},
      {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
  };
  HmacAuthConfigRule rule;
  rule.credentials = {{"appKey", "appSecret"}};
  //  EXPECT_EQ(root_context_->checkPlugin(rule, std::nullopt), true);

  std::string configuration = R"(
{
  "consumers": [],
  "_rules_": [
    {
      "_match_route_":["test"],
      "allow":[]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopAllIterationAndBuffer);
  route_name_ = "test2";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(HmacAuthTest, DuplicateKey) {
  {
    std::string configuration = R"(
  {
       "credentials":[
        {"key": "appKey", "secret": ""},
        {"key": "appKey", "secret": "123"}
      ]
  })";
    BufferBase buffer;
    config_.set(configuration);
    EXPECT_FALSE(root_context_->configure(configuration.size()));
  }

  {
    std::string configuration = R"(
  {
       "consumers":[
        {"key": "appKey", "secret": "", "name": "consumer1"},
        {"key": "appKey", "secret": "123", "name": "consumer2"}
      ]
  })";
    BufferBase buffer;
    config_.set(configuration);
    EXPECT_FALSE(root_context_->configure(configuration.size()));
  }
}

TEST_F(HmacAuthTest, BodyMD5) {
  body_.set("abc");
  headers_ = {{"content-md5", "kAFQmDzST7DWlj99KOF/cg=="}};
  context_->onRequestHeaders(0, false);
  EXPECT_EQ(context_->onRequestBody(3, true), FilterDataStatus::Continue);

  headers_ = {};
  context_->onRequestHeaders(0, false);
  EXPECT_EQ(context_->onRequestBody(0, false), FilterDataStatus::Continue);
}

TEST_F(HmacAuthTest, DateCheck) {
  std::string configuration = R"(
{
      "credentials":[
        {"key": "203753385", "secret": "123456"}
      ],
      "date_offset": 3600
})";
  BufferBase buffer;
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  headers_ = {
      {":path",
       "/http2test/test?param1=test&username=xiaoming&password=123456789"},
      {":method", "POST"},
      {"accept", "application/json; charset=utf-8"},
      {"ca_version", "1"},
      {"content-type", "application/x-www-form-urlencoded; charset=utf-8"},
      {"x-ca-timestamp", "1525872629832"},
      {"date", "Wed, 09 May 2018 13:30:29 GMT+00:00"},
      {"user-agent", "ALIYUN-ANDROID-DEMO"},
      {"x-ca-nonce", "c9f15cbf-f4ac-4a6c-b54d-f51abf4b5b44"},
      {"content-length", "33"},
      {"username", "xiaoming&password=123456789"},
      {"x-ca-key", "203753385"},
      {"x-ca-signature-method", "HmacSHA256"},
      {"x-ca-signature", "FJbhmAFYz9zfl1FrThxzxBt79BvaHQIzy8Wpctn+xXE="},
      {"x-ca-signature-headers",
       "x-ca-timestamp,x-ca-key,x-ca-nonce,x-ca-signature-method"},
  };
  current_time_ = (uint64_t)1525876230 * 1000 * 1000 * 1000;
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(context_->onRequestBody(0, true),
            FilterDataStatus::StopIterationNoBuffer);
  current_time_ = (uint64_t)1525869027 * 1000 * 1000 * 1000;
  EXPECT_EQ(context_->onRequestBody(0, true),
            FilterDataStatus::StopIterationNoBuffer);
  current_time_ = (uint64_t)1525869029 * 1000 * 1000 * 1000;
  EXPECT_EQ(context_->onRequestBody(0, true), FilterDataStatus::Continue);
}

TEST_F(HmacAuthTest, TimestampCheck) {
  std::string configuration = R"(
{
      "credentials":[
        {"key": "203753385", "secret": "123456"}
      ],
      "date_offset": 3600
})";
  BufferBase buffer;
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  headers_ = {
      {":path",
       "/http2test/test?param1=test&username=xiaoming&password=123456789"},
      {":method", "POST"},
      {"accept", "application/json; charset=utf-8"},
      {"ca_version", "1"},
      {"content-type", "application/x-www-form-urlencoded; charset=utf-8"},
      {"x-ca-timestamp", "1525872629832"},
      {"user-agent", "ALIYUN-ANDROID-DEMO"},
      {"x-ca-nonce", "c9f15cbf-f4ac-4a6c-b54d-f51abf4b5b44"},
      {"content-length", "33"},
      {"username", "xiaoming&password=123456789"},
      {"x-ca-key", "203753385"},
      {"x-ca-signature-method", "HmacSHA256"},
      {"x-ca-signature", "aKS8DtmSjdNns8+uk3eVFZNpY4MxhK2z4Zb7XH8NoLE="},
      {"x-ca-signature-headers",
       "x-ca-timestamp,x-ca-key,x-ca-nonce,x-ca-signature-method"},
  };
  current_time_ = (uint64_t)1525876230 * 1000 * 1000 * 1000;
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(context_->onRequestBody(0, true),
            FilterDataStatus::StopIterationNoBuffer);
  current_time_ = (uint64_t)1525869027 * 1000 * 1000 * 1000;
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(context_->onRequestBody(0, true),
            FilterDataStatus::StopIterationNoBuffer);
  current_time_ = (uint64_t)1525869029832 * 1000 * 1000;
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(context_->onRequestBody(0, true), FilterDataStatus::Continue);
}

TEST_F(HmacAuthTest, TimestampSecCheck) {
  std::string configuration = R"(
{
      "credentials":[
        {"key": "203753385", "secret": "123456"}
      ],
      "date_offset": 3600
})";
  BufferBase buffer;
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  headers_ = {
      {":path",
       "/http2test/test?param1=test&username=xiaoming&password=123456789"},
      {":method", "POST"},
      {"accept", "application/json; charset=utf-8"},
      {"ca_version", "1"},
      {"content-type", "application/x-www-form-urlencoded; charset=utf-8"},
      {"x-ca-timestamp", "1525872629"},
      {"user-agent", "ALIYUN-ANDROID-DEMO"},
      {"x-ca-nonce", "c9f15cbf-f4ac-4a6c-b54d-f51abf4b5b44"},
      {"content-length", "33"},
      {"username", "xiaoming&password=123456789"},
      {"x-ca-key", "203753385"},
      {"x-ca-signature-method", "HmacSHA256"},
      {"x-ca-signature", "oFlnmkAPv45BVLLajfk7avGBwDFjOu0VElMrNyCFlGs="},
      {"x-ca-signature-headers",
       "x-ca-timestamp,x-ca-key,x-ca-nonce,x-ca-signature-method"},
  };
  current_time_ = (uint64_t)1525876230 * 1000 * 1000 * 1000;
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(context_->onRequestBody(0, true),
            FilterDataStatus::StopIterationNoBuffer);
  current_time_ = (uint64_t)1525869027 * 1000 * 1000 * 1000;
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(context_->onRequestBody(0, true),
            FilterDataStatus::StopIterationNoBuffer);
  current_time_ = (uint64_t)1525869029 * 1000 * 1000 * 1000;
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(context_->onRequestBody(0, true), FilterDataStatus::Continue);
}

TEST_F(HmacAuthTest, EmptyAllowSet) {
  headers_ = {
      {":path", "/Third/Tools/checkSign"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"content-type", "application/json"},
      {"x-ca-timestamp", "1646365291734"},
      {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
      {"x-ca-signature", "EdJSFAMOWyXZOpXhevZnjuS0ZafnwnCqaSk5hz+tXo8="},
  };
  HmacAuthConfigRule rule;
  rule.credentials = {{"appKey", "appSecret"}};
  //  EXPECT_EQ(root_context_->checkPlugin(rule, std::nullopt), true);

  std::string configuration = R"(
{
  "consumers": [{"key": "appKey", "secret": "appSecret", "name": "consumer"}],
  "_rules_": [
    {
      "_match_route_prefix_":["test"],
      "allow":[]
    }
  ]
})";
  route_name_ = "test@op1";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  EXPECT_CALL(*mock_context_, sendLocalResponse(403, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopAllIterationAndBuffer);
}

TEST_F(HmacAuthTest, FallbackMarkerDoesNotBypassAuthentication) {
  headers_ = {
      {"x-higress-fallback-from", "original-cluster"},
      {"X-Mse-Consumer", "consumer"},
  };
  std::string configuration = R"(
{
  "consumers": [{"key": "appKey", "secret": "appSecret", "name": "consumer"}],
  "_rules_": [
    {
      "_match_route_": ["test"],
      "allow": ["consumer"]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  // Both headers are client-settable, so a marker plus a claimed consumer must
  // not authenticate anything: no HMAC signature is presented.
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopAllIterationAndBuffer);

  // The consumer header alone must not bypass authentication.
  headers_.erase("x-higress-fallback-from");
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopAllIterationAndBuffer);

  // A fallback marker without a consumer must also use normal authentication.
  headers_["x-higress-fallback-from"] = "original-cluster";
  headers_.erase("X-Mse-Consumer");
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopAllIterationAndBuffer);
}

TEST_F(HmacAuthTest, FallbackConsumerMustPassAuthorization) {
  headers_ = {
      {"x-higress-fallback-from", "original-cluster"},
      {"X-Mse-Consumer", "consumer-2"},
  };
  std::string configuration = R"(
{
  "consumers": [{"key": "appKey", "secret": "appSecret", "name": "consumer-1"}],
  "_rules_": [
    {
      "_match_route_": ["test"],
      "allow": ["consumer-1"]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  // An off-allow-list consumer used to reach authorization and fail with 403.
  // No consumer is established without a verified signature, so the request now
  // fails authentication with 401.
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopAllIterationAndBuffer);
}

TEST_F(HmacAuthTest, ForgedFallbackIdentityIsSanitized) {
  headers_ = {
      {":path", "/Third/Tools/checkSign"},
      {":method", "GET"},
      {"x-higress-fallback-from", "original-cluster"},
      {"x-ca-key", "appKey"},
      {"X-Mse-Consumer", "consumer-vip"},
      {"X-Mse-Consumer-Group", "forged-group"},
  };
  std::string configuration = R"(
{
  "consumers": [
    {"key": "appKey", "secret": "appSecret", "name": "consumer-basic"},
    {"key": "vipKey", "secret": "vipSecret", "name": "consumer-vip"}
  ],
  "_rules_": [
    {
      "_match_route_": ["test"],
      "allow": ["consumer-basic", "consumer-vip"]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  // Holding only appKey, the caller claims the identity of another allowed
  // consumer. Reusing that claim on a fallback request used to be enough to
  // pass; the request must now fail closed for want of a signature, and neither
  // forged value may survive onto the wire.
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopAllIterationAndBuffer);
  // The gateway replaces the claimed consumer with its own assertion for the
  // presented key and clears a group it never authenticated.
  EXPECT_EQ(headers_["X-Mse-Consumer"], "consumer-basic");
  EXPECT_EQ(headers_["X-Mse-Consumer-Group"], "");
}

TEST_F(HmacAuthTest, SignWithConsumerRbac) {
  headers_ = {
      {":path", "/Third/Tools/checkSign"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"content-type", "application/json"},
      {"x-ca-timestamp", "1646365291734"},
      {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
      {"x-ca-signature", "EdJSFAMOWyXZOpXhevZnjuS0ZafnwnCqaSk5hz+tXo8="},
  };
  HmacAuthConfigRule rule;
  rule.credentials = {{"appKey", "appSecret"}};
  //  EXPECT_EQ(root_context_->checkPlugin(rule, std::nullopt), true);

  std::string configuration = R"(
{
  "consumers": [
    {
      "key": "appKey",
      "secret": "appSecret",
      "name": "consumer1"
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
  ],
  "rbac_rules": [
    {
      "principals": [
        {
          "or_rules": [
            {
              "consumer": "consumer1"
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
              "consumer": "consumer2"
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
  ]
})";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  route_name_ = "test1";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  route_name_ = "test2";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopAllIterationAndBuffer);
}

TEST_F(HmacAuthTest, ConsumerGroupHeaderCommaJoined) {
  headers_ = {
      {":path", "/Third/Tools/checkSign"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"content-type", "application/json"},
      {"x-ca-timestamp", "1646365291734"},
      {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
      {"x-ca-signature", "EdJSFAMOWyXZOpXhevZnjuS0ZafnwnCqaSk5hz+tXo8="},
  };

  std::string configuration = R"(
{
  "consumers": [
    {
      "key": "appKey",
      "secret": "appSecret",
      "name": "consumer1",
      "group": ["partner-a", "partner-b"]
    }
  ],
  "_rules_": [
    {
      "_match_route_": ["test"],
      "enable_auth": true
    }
  ],
  "rbac_rules": [
    {
      "principals": [{"any": true}],
      "permissions": [
        {"route_name": {"exact_match": "test"}}
      ]
    }
  ]
})";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(headers_["X-Mse-Consumer"], "consumer1");
  EXPECT_EQ(headers_["X-Mse-Consumer-Group"], "partner-a,partner-b");
}

TEST_F(HmacAuthTest, ConsumerGroupHeaderEmptyWhenOmitted) {
  headers_ = {
      {":path", "/Third/Tools/checkSign"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"content-type", "application/json"},
      {"x-ca-timestamp", "1646365291734"},
      {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
      {"x-ca-signature", "EdJSFAMOWyXZOpXhevZnjuS0ZafnwnCqaSk5hz+tXo8="},
      {"X-Mse-Consumer-Group", "spoofed"},
  };

  std::string configuration = R"(
{
  "consumers": [
    {
      "key": "appKey",
      "secret": "appSecret",
      "name": "consumer1"
    }
  ],
  "_rules_": [
    {
      "_match_route_": ["test"],
      "enable_auth": true
    }
  ],
  "rbac_rules": [
    {
      "principals": [{"any": true}],
      "permissions": [
        {"route_name": {"exact_match": "test"}}
      ]
    }
  ]
})";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  auto group_it = headers_.find("X-Mse-Consumer-Group");
  ASSERT_NE(group_it, headers_.end());
  EXPECT_EQ(group_it->second, "");
}

TEST_F(HmacAuthTest, ConsumerGroupHeaderEmptyWithCredentials) {
  headers_ = {
      {":path", "/Third/Tools/checkSign"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"content-type", "application/json"},
      {"x-ca-timestamp", "1646365291734"},
      {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
      {"x-ca-signature", "EdJSFAMOWyXZOpXhevZnjuS0ZafnwnCqaSk5hz+tXo8="},
      {"X-Mse-Consumer-Group", "spoofed"},
  };

  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_": ["test"],
      "enable_auth": true,
      "credentials": [
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  auto group_it = headers_.find("X-Mse-Consumer-Group");
  ASSERT_NE(group_it, headers_.end());
  EXPECT_EQ(group_it->second, "");
}

TEST_F(HmacAuthTest, ClientSuppliedConsumerHeaderIsReplaced) {
  headers_ = {
      {":path", "/Third/Tools/checkSign"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"content-type", "application/json"},
      {"x-ca-timestamp", "1646365291734"},
      {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
      {"x-ca-signature", "EdJSFAMOWyXZOpXhevZnjuS0ZafnwnCqaSk5hz+tXo8="},
      {"X-Mse-Consumer", "spoofed-consumer"},
  };

  std::string configuration = R"(
{
  "consumers": [
    {
      "key": "appKey",
      "secret": "appSecret",
      "name": "consumer1"
    }
  ],
  "_rules_": [
    {
      "_match_route_": ["test"],
      "enable_auth": true
    }
  ],
  "rbac_rules": [
    {
      "principals": [{"any": true}],
      "permissions": [
        {"route_name": {"exact_match": "test"}}
      ]
    }
  ]
})";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(headers_["X-Mse-Consumer"], "consumer1");
}

TEST_F(HmacAuthTest, ClientSuppliedConsumerHeaderRemovedWithoutName) {
  headers_ = {
      {":path", "/Third/Tools/checkSign"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"content-type", "application/json"},
      {"x-ca-timestamp", "1646365291734"},
      {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
      {"x-ca-signature", "EdJSFAMOWyXZOpXhevZnjuS0ZafnwnCqaSk5hz+tXo8="},
      {"X-Mse-Consumer", "spoofed-consumer"},
  };

  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_": ["test"],
      "enable_auth": true,
      "credentials": [
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(headers_.find("X-Mse-Consumer"), headers_.end());
}

TEST_F(HmacAuthTest, RbacAllowsConsumerGroupPrincipal) {
  headers_ = {
      {":path", "/Third/Tools/checkSign"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"content-type", "application/json"},
      {"x-ca-timestamp", "1646365291734"},
      {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
      {"x-ca-signature", "EdJSFAMOWyXZOpXhevZnjuS0ZafnwnCqaSk5hz+tXo8="},
  };

  std::string configuration = R"(
{
  "consumers": [
    {
      "key": "appKey",
      "secret": "appSecret",
      "name": "consumer1",
      "group": ["partner-a", "partner-b"]
    }
  ],
  "_rules_": [
    {
      "_match_route_": ["test"],
      "enable_auth": true
    }
  ],
  "rbac_rules": [
    {
      "principals": [
        {
          "or_rules": [
            {"consumer_group": " partner-b "}
          ]
        }
      ],
      "permissions": [
        {"route_name": {"exact_match": "test"}}
      ]
    }
  ]
})";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(headers_["X-Mse-Consumer-Group"], "partner-a,partner-b");
}

TEST_F(HmacAuthTest, RbacDeniesWhenConsumerGroupMismatch) {
  headers_ = {
      {":path", "/Third/Tools/checkSign"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"content-type", "application/json"},
      {"x-ca-timestamp", "1646365291734"},
      {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key,x-ca-nonce,x-ca-timestamp"},
      {"x-ca-signature", "EdJSFAMOWyXZOpXhevZnjuS0ZafnwnCqaSk5hz+tXo8="},
  };

  std::string configuration = R"(
{
  "consumers": [
    {
      "key": "appKey",
      "secret": "appSecret",
      "name": "consumer1",
      "group": ["partner-b"]
    }
  ],
  "_rules_": [
    {
      "_match_route_": ["test"],
      "enable_auth": true
    }
  ],
  "rbac_rules": [
    {
      "principals": [
        {
          "or_rules": [
            {"consumer_group": "partner-a"}
          ]
        }
      ],
      "permissions": [
        {"route_name": {"exact_match": "test"}}
      ]
    }
  ]
})";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  EXPECT_CALL(*mock_context_, sendLocalResponse(403, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopAllIterationAndBuffer);
}

TEST_F(HmacAuthTest, SignWithConsumerRbacCaseInsensitiveHeadersForZeeker) {
  headers_ = {
      {":path", "/get"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"content-type", "application/json"},
      {"x-ca-timestamp", "1646365291734"},
      {"x-ca-nonce", "787dd0c2-7bd8-41cd-9c19-62c05ea524a2"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key,X-Ca-Nonce,X-Ca-Timestamp"},
      {"x-ca-signature", "0hYDi3ww9RjOBMDG6ikg6TjRpO212qsP/ixgmJxwCSQ="},
  };
  HmacAuthConfigRule rule;
  rule.credentials = {{"appKey", "appSecret"}};

  std::string configuration = R"(
{
  "consumers": [
    {
      "key": "appKey",
      "secret": "appSecret",
      "name": "consumer1"
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
  ],
  "rbac_rules": [
    {
      "principals": [
        {
          "or_rules": [
            {
              "consumer": "consumer1"
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
              "consumer": "consumer2"
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
  ]
})";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));
  route_name_ = "test1";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  route_name_ = "test2";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopAllIterationAndBuffer);
}

TEST_F(HmacAuthTest, OldApiGatewayFallbackDecodesPathLikeUndertow) {
  const std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  ASSERT_TRUE(root_context_->configure(configuration.size()));

  struct PathCase {
    std::string_view encoded_path;
    std::string_view canonical_path;
    std::string_view signature_method;
    std::string_view hash_type;
  };
  // Canonical paths are golden outputs from Undertow 2.2.23
  // URLUtils.decode(path, "UTF-8", false, false, buffer).
  const std::vector<PathCase> path_cases = {
      {"/%E4%BD%A0%E5%A5%BD/get", "/你好/get", "", "sha256"},
      {"/jv-bpm/internal/api/process/routineOpenAccount/N-%E6%88%90%E9%83%BD"
       "%E5%B8%82%E5%93%B2%E6%96%B0%E7%89%A9%E6%B5%81%E8%82%A1%E4%BB%BD"
       "%E6%9C%89%E9%99%90%E5%85%AC%E5%8F%B8",
       "/jv-bpm/internal/api/process/routineOpenAccount/"
       "N-成都市哲新物流股份有限公司",
       "", "sha256"},
      {"/%e4%bd%a0+%E5%A5%BD", "/你+好", "", "sha256"},
      {"/%E4%BD%A0%2F%E5%A5%BD", "/你%2F好", "", "sha256"},
      {"/%E4%BD%A0%2f%E5%A5%BD", "/你%2f好", "", "sha256"},
      {"/%E4%BD%A0%5C%E5%A5%BD", "/你%5C好", "", "sha256"},
      {"/%E4%BD%A0%2B%E5%A5%BD", "/你+好", "", "sha256"},
      {"/%E4%BD%A0%20world", "/你 world", "", "sha256"},
      {"/%E4%BD%A0/%41", "/你/A", "", "sha256"},
      {"/%25E4%25BD%25A0", "/%E4%BD%A0", "", "sha256"},
      {"/%E4%BD%A0%3F%E5%A5%BD", "/你?好", "", "sha256"},
      {"/%F0%9F%98%80", "/😀", "HmacSHA1", "sha1"},
  };

  for (const auto& path_case : path_cases) {
    SCOPED_TRACE(path_case.encoded_path);
    std::string string_to_sign =
        "GET\napplication/json\n\n\n\nX-Ca-Key:appKey\n";
    string_to_sign.append(path_case.canonical_path);
    headers_ = {
        {":path", std::string(path_case.encoded_path)},
        {":method", "GET"},
        {"accept", "application/json"},
        {"x-ca-key", "appKey"},
        {"x-ca-signature-headers", "X-Ca-Key"},
        {"x-ca-signature-method", std::string(path_case.signature_method)},
        {"x-ca-signature",
         Wasm::Common::Crypto::getShaHmacBase64(
             path_case.hash_type, "appSecret", string_to_sign)},
    };

    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::Continue);
    EXPECT_EQ(headers_.at(":path"), path_case.encoded_path);
  }
}

TEST_F(HmacAuthTest,
       OldApiGatewayDecodedPathFallbackKeepsQueryAndFormSemantics) {
  const std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  ASSERT_TRUE(root_context_->configure(configuration.size()));

  const std::string body = "body=hello+world&empty=";
  headers_ = {
      {":path", "/%E4%BD%A0%E5%A5%BD?query=a%2Bb"},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "application/x-www-form-urlencoded"},
      {"content-length", std::to_string(body.size())},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key"},
  };
  const std::string string_to_sign =
      "POST\napplication/json\n\napplication/x-www-form-urlencoded\n\n"
      "X-Ca-Key:appKey\n"
      "/你好?body=hello world&empty&query=a+b";
  headers_["x-ca-signature"] = Wasm::Common::Crypto::getShaHmacBase64(
      "sha256", "appSecret", string_to_sign);
  body_.set(body);

  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(context_->onRequestBody(body.size(), true),
            FilterDataStatus::Continue);
  EXPECT_EQ(headers_.at(":path"),
            "/%E4%BD%A0%E5%A5%BD?query=a%2Bb");
}

TEST_F(HmacAuthTest,
       OldApiGatewayDecodedPathFallbackSupportsCompatibilityQueryParams) {
  const std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  ASSERT_TRUE(root_context_->configure(configuration.size()));

  const std::string request_path =
      "/%E4%BD%A0%E5%A5%BD?empty=&keyword=hello+world";
  headers_ = {
      {":path", request_path},
      {":method", "GET"},
      {"accept", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key"},
  };
  const std::string string_to_sign =
      "GET\napplication/json\n\n\n\nX-Ca-Key:appKey\n"
      "/你好?empty=&keyword=hello+world";
  headers_["x-ca-signature"] = Wasm::Common::Crypto::getShaHmacBase64(
      "sha256", "appSecret", string_to_sign);

  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(headers_.at(":path"), request_path);
}

TEST_F(HmacAuthTest,
       OldApiGatewayDecodedPathFallbackSupportsCompatibilityFormParams) {
  const std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  ASSERT_TRUE(root_context_->configure(configuration.size()));

  const std::string request_path = "/%E4%BD%A0%E5%A5%BD";
  const std::string body = "empty=&keyword=hello+world";
  headers_ = {
      {":path", request_path},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "application/x-www-form-urlencoded"},
      {"content-length", std::to_string(body.size())},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key"},
  };
  const std::string string_to_sign =
      "POST\napplication/json\n\napplication/x-www-form-urlencoded\n\n"
      "X-Ca-Key:appKey\n"
      "/你好?empty=&keyword=hello+world";
  headers_["x-ca-signature"] = Wasm::Common::Crypto::getShaHmacBase64(
      "sha256", "appSecret", string_to_sign);
  body_.set(body);

  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(context_->onRequestBody(body.size(), true),
            FilterDataStatus::Continue);
  EXPECT_EQ(headers_.at(":path"), request_path);
}

TEST_F(HmacAuthTest,
       OldApiGatewayDecodedPathFallbackUsesSignedContentType) {
  const std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  ASSERT_TRUE(root_context_->configure(configuration.size()));

  headers_ = {
      {":path", "/%E4%BD%A0%E5%A5%BD"},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "multipart/form-data; boundary=abc123"},
      {"x-ca-signed-content-type", "multipart/form-data"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key,X-Ca-Signed-Content-Type"},
  };
  const std::string string_to_sign =
      "POST\napplication/json\n\nmultipart/form-data\n\n"
      "X-Ca-Key:appKey\n"
      "X-Ca-Signed-Content-Type:multipart/form-data\n"
      "/你好";
  headers_["x-ca-signature"] = Wasm::Common::Crypto::getShaHmacBase64(
      "sha256", "appSecret", string_to_sign);

  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(HmacAuthTest, EncodedRawPathSignatureStillWorks) {
  const std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  ASSERT_TRUE(root_context_->configure(configuration.size()));

  const std::string encoded_path = "/%E4%BD%A0%E5%A5%BD";
  headers_ = {
      {":path", encoded_path},
      {":method", "GET"},
      {"accept", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key"},
  };
  const std::string string_to_sign =
      "GET\napplication/json\n\n\n\nx-ca-key:appKey\n" + encoded_path;
  headers_["x-ca-signature"] = Wasm::Common::Crypto::getShaHmacBase64(
      "sha256", "appSecret", string_to_sign);

  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(HmacAuthTest, OldApiGatewayDecodedPathFallbackRejectsInvalidPath) {
  const std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  ASSERT_TRUE(root_context_->configure(configuration.size()));

  struct InvalidPathCase {
    std::string_view request_path;
    std::string_view signed_path;
  };
  const std::vector<InvalidPathCase> path_cases = {
      {"/%E4%BD%A0%E5%A5%BD", "/您好"},
      {"/incomplete%", "/incomplete"},
      {"/invalid%GG", "/invalidGG"},
  };
  EXPECT_CALL(*mock_context_, sendLocalResponse(400, testing::_, testing::_,
                                                testing::_, testing::_))
      .Times(path_cases.size());

  for (const auto& path_case : path_cases) {
    SCOPED_TRACE(path_case.request_path);
    std::string string_to_sign =
        "GET\napplication/json\n\n\n\nX-Ca-Key:appKey\n";
    string_to_sign.append(path_case.signed_path);
    headers_ = {
        {":path", std::string(path_case.request_path)},
        {":method", "GET"},
        {"accept", "application/json"},
        {"x-ca-key", "appKey"},
        {"x-ca-signature-headers", "X-Ca-Key"},
        {"x-ca-signature",
         Wasm::Common::Crypto::getShaHmacBase64("sha256", "appSecret",
                                                string_to_sign)},
    };
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::StopAllIterationAndBuffer);
  }
}

// URL has '=' with empty value (e.g., "?b="), but the signature is computed
// without '=' (e.g., "?b") — matching old apigateway signing behavior.
TEST_F(HmacAuthTest, EmptyValueUrlHasEqualsSignWithout) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  // URL "?a=1&b=", signature from "?a=1&b"
  headers_ = {
      {":path", "/test?a=1&b="},
      {":method", "GET"},
      {"accept", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature", "I3uKKIML0ARjfF2Wovs/umFjn6BoG5Mg4uUZ9RzTJS8="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  // URL "?name=", single empty-value param, signature from "?name"
  headers_[":path"] = "/test?name=";
  headers_["x-ca-signature"] = "oHgbZOT5iJxKars1LxtO2vU3XiqKfr6/xmRoABOKzKM=";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  // URL "?a=1&b=2&c=", mixed params, signature from "?a=1&b=2&c"
  headers_[":path"] = "/test?a=1&b=2&c=";
  headers_["x-ca-signature"] = "GwwElzw9xxE2/9D2kZiTSgEisZnT0lJ0cfzY/ziZ1mE=";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

// POST: URL "?a=1&b=" + form body "x=1", signature from "?a=1&b&x=1".
TEST_F(HmacAuthTest, EmptyValueUrlHasEqualsWithBody) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  headers_ = {
      {":path", "/test?a=1&b="},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "application/x-www-form-urlencoded"},
      {"content-length", "3"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature", "mSHsqCNIZOavIxgAEwzxI37RWt/qn8GgB14adT6JpNM="},
  };
  std::string body("x=1");
  body_.set(body);
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(context_->onRequestBody(body.size(), true),
            FilterDataStatus::Continue);
}

// Verify that query params without '=' in the URL are handled correctly.
// e.g., "?a=1&b" is signed as "?a=1&b", not "?a=1&b=".
TEST_F(HmacAuthTest, NoEqualsQueryParam) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  // Signature computed from: "GET\napplication/json\n\n\n\n/test?a=1&b"
  // i.e., param b has no '=' in the string-to-sign.
  headers_ = {
      {":path", "/test?a=1&b"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature", "I3uKKIML0ARjfF2Wovs/umFjn6BoG5Mg4uUZ9RzTJS8="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  // Same signature with params reordered: "?b&a=1" → sorted as "?a=1&b".
  headers_[":path"] = "/test?b&a=1";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

// Mixed: some params have values, some don't.
// "?a=1&b=2&c" → preserved string-to-sign uses "?a=1&b=2&c".
TEST_F(HmacAuthTest, NoEqualsQueryParamMixed) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  // Signature computed from: "GET\napplication/json\n\n\n\n/test?a=1&b=2&c"
  headers_ = {
      {":path", "/test?a=1&b=2&c"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature", "GwwElzw9xxE2/9D2kZiTSgEisZnT0lJ0cfzY/ziZ1mE="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

// POST with form body params merged with URL no-equals param.
// URL "?a=1&b" + body "x=1&y=2" → preserved string-to-sign uses
// "?a=1&b&x=1&y=2".
TEST_F(HmacAuthTest, NoEqualsQueryParamWithBody) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  headers_ = {
      {":path", "/test?a=1&b"},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "application/x-www-form-urlencoded"},
      {"content-length", "7"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature", "C4gxNVx/hQlKC2oS4N9Bng2MSq7HJhE07KEcDJVZIWM="},
  };
  std::string body("x=1&y=2");
  body_.set(body);
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(context_->onRequestBody(body.size(), true),
            FilterDataStatus::Continue);
}

// Signature computed with "?a=1&b=" (old format with trailing '=') must still
// pass via the current/old path when the URL is "?a=1&b=".
TEST_F(HmacAuthTest, EmptyValueQueryParamStillWorks) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  // Signature computed from: "GET\napplication/json\n\n\n\n/test?a=1&b="
  headers_ = {
      {":path", "/test?a=1&b="},
      {":method", "GET"},
      {"accept", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature", "LGd+30EL4RbXeL5QtCQj9MU31lKm3pYgO7poetCJZYU="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

// Old API Gateway canonicalizes both "q" and "q=" as an empty value without
// '=' in the StringToSign.
TEST_F(HmacAuthTest, OldApiGatewayFallbackTreatsQAndQEqualsTheSame) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  // StringToSign: "GET\napplication/json\n\n\n\nX-Ca-Key:appKey\n/test?a=1&b"
  // URL has "?a=1&b=", but signature omits '=' for empty value.
  headers_ = {
      {":path", "/test?a=1&b="},
      {":method", "GET"},
      {"accept", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key"},
      {"x-ca-signature", "2g9amlIqrQt6rBh8+h2OyU8LqNpmXXcFq8xaRSgK2MI="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  headers_[":path"] = "/test?a=1&b";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

// Preserve the historical old API Gateway fallback that keeps '=' for an
// empty value.
TEST_F(HmacAuthTest, OldApiGatewayFallbackAcceptsEmptyValueWithEquals) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  // StringToSign: "GET\napplication/json\n\n\n\nX-Ca-Key:appKey\n/test?a=1&b="
  // URL has "?a=1&b=", signature includes '=' for empty value.
  headers_ = {
      {":path", "/test?a=1&b="},
      {":method", "GET"},
      {"accept", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key"},
      {"x-ca-signature", "HWXIBAc70pGir4h3h1pj238P8bimPzPw3uOR26uJ8tg="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(HmacAuthTest,
       OldApiGatewayFallbackAcceptsEmptyValueWithEqualsInFormBody) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  std::string body("x=1");
  headers_ = {
      {":path", "/test?a=1&b="},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "application/x-www-form-urlencoded"},
      {"content-length", std::to_string(body.size())},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key"},
      // Historical StringToSign ends with "/test?a=1&b=&x=1".
      {"x-ca-signature", "yR47nmK0G0W15bYWpFeYHykrB+ewycw29q0Du2hnesU="},
  };
  body_.set(body);

  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(context_->onRequestBody(body.size(), true),
            FilterDataStatus::Continue);
}

TEST_F(HmacAuthTest, OldApiGatewayFallbackPreservesRawPlusInCompatCandidate) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  headers_ = {
      {":path", "/test?keyword=hello+world&empty="},
      {":method", "GET"},
      {"accept", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key"},
      // Historical StringToSign ends with
      // "/test?empty=&keyword=hello+world".
      {"x-ca-signature", "d7gV/bqc68MjAZOxysyDeVJrOnW6lEuGwprDZV4Tk6U="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(HmacAuthTest, OldApiGatewayFallbackCanonicalizesQueryAndFormBody) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  std::string body(
      "body=hello+world&duplicate=body+value&literal=a%2Bb&blank=&na+me=value");
  headers_ = {
      {":path", "/test?duplicate=query+value&empty=&query=a%2Bb"},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "application/x-www-form-urlencoded"},
      {"content-length", std::to_string(body.size())},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key"},
      {"x-ca-signature", "vgjITqFrjX4Q9J3tpxdu/O9B7KK6twDQcQq+ejJHFr0="},
  };
  body_.set(body);

  // Query wins the duplicate name. Empty values omit '=', raw '+' becomes
  // SP in names and values, and '%2B' remains a literal plus.
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(context_->onRequestBody(body.size(), true),
            FilterDataStatus::Continue);
}

TEST_F(HmacAuthTest, OldApiGatewayFallbackPreservesBareFormParameterName) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  std::string body("na+me");
  headers_ = {
      {":path", "/test"},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "application/x-www-form-urlencoded"},
      {"content-length", std::to_string(body.size())},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key"},
      // The old data plane preserves a bare Form name as "na+me".
      {"x-ca-signature", "+bgIXEEQvGSYpAHge5+CRn5AXA5Jr1oUq+3Dn7oXnZE="},
  };
  body_.set(body);

  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(context_->onRequestBody(body.size(), true),
            FilterDataStatus::Continue);
}

// A raw '+' in the query is decoded as SP only by the final old API Gateway
// fallback. The existing RFC 3986-compatible verification paths run first.
TEST_F(HmacAuthTest, OldApiGatewayFallbackDecodesRawQueryPlusAsSpace) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  // StringToSign ends with "/test?keyword=hello world".
  headers_ = {
      {":path", "/test?keyword=hello+world"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key"},
      {"x-ca-signature", "rB7XMSp9OAfZwft2rGNrd6J7fW+nzbSC6MsSZWvJfdk="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(HmacAuthTest, OldApiGatewayFallbackPreservesPercentEncodedPlus) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  // Raw '+' becomes SP, while '%2B' remains a literal plus in the same
  // fallback candidate: "/test?literal=a+b&space=a b".
  headers_ = {
      {":path", "/test?literal=a%2Bb&space=a+b"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key"},
      {"x-ca-signature", "8WwTiB5krhLRb4iEkfBC5o0PcA3GT4+hEgklqqhygIM="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(HmacAuthTest, OldApiGatewayFallbackDoesNotDecodeEncodedPlusAsSpace) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  // This signature is valid for "/test?keyword=hello world", but '%2B'
  // represents a literal plus and must not enter the form-query fallback.
  headers_ = {
      {":path", "/test?keyword=hello%2Bworld"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key"},
      {"x-ca-signature", "rB7XMSp9OAfZwft2rGNrd6J7fW+nzbSC6MsSZWvJfdk="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopAllIterationAndBuffer);
}

TEST_F(HmacAuthTest, RawQueryPlusLiteralSignatureStillWorks) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  // Current keeps raw '+' literal; only the old API Gateway fallback applies
  // form decoding.
  headers_ = {
      {":path", "/test?keyword=hello+world"},
      {":method", "GET"},
      {"accept", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key"},
      {"x-ca-signature", "vGa7xXOrblEO4aQDU0Xl5BMj8W0lbVXH6KNfNCmL64I="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(HmacAuthTest, RawFormBodyPlusLiteralSignatureStillWorks) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  std::string body("keyword=hello+world");
  headers_ = {
      {":path", "/test"},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "application/x-www-form-urlencoded"},
      {"content-length", std::to_string(body.size())},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key"},
      {"x-ca-signature", "xotrANV3KVv6VMdTFcsrBk24MukWnA6PcMsAl+t+63U="},
  };
  body_.set(body);
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(context_->onRequestBody(body.size(), true),
            FilterDataStatus::Continue);
}

TEST_F(HmacAuthTest, OldApiGatewayFallbackCombinesPlusAndEmptyValueRules) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  // Old API Gateway form-decodes raw '+' and omits '=' for the empty value.
  headers_ = {
      {":path", "/test?a=hello+world&b="},
      {":method", "GET"},
      {"accept", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key"},
      {"x-ca-signature", "8KQ9GOttxqmQbOh1ZVKtR6tCpxqaG0YT0keS0Qi890Y="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(HmacAuthTest, OldApiGatewayFallbackSignsEveryDeclaredHeader) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  headers_ = {
      {":path", "/test?keyword=hello+world&empty="},
      {":method", "GET"},
      {"accept", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "Accept,X-Ca-Key"},
      // The old data plane signs Accept both as a fixed field and as an
      // explicitly declared Header.
      {"x-ca-signature", "CUZLdpg3/BB/cwnd+iFEmgmJMDsKMSIGtscLfaPp6TU="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(HmacAuthTest, OldApiGatewayFallbackUsesSignedContentType) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  headers_ = {
      {":path", "/test"},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "multipart/form-data; boundary=abc123"},
      {"x-ca-signed-content-type", "multipart/form-data"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key"},
      // The old data plane replaces the actual Content-Type with
      // X-Ca-Signed-Content-Type in the fixed Content-Type field.
      {"x-ca-signature", "TbpmwAqzCXLNj16aqfSEI3akrTaQeropGRpL0ARPWkw="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  headers_ = {
      {":path", "/test"},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "multipart/form-data; boundary=abc123"},
      {"X-Ca-Signed-Content-Type", "multipart/form-data"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key,X-Ca-Signed-Content-Type"},
      // X-Ca-Signed-Content-Type is used in the fixed field and is also
      // signed as a declared Header, matching the old data plane.
      {"x-ca-signature", "fUuAN4b6k7cFhl2o/vjwSFUQkqcYr1aNPYhQCM2bYIc="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  headers_ = {
      {":path", "/test"},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "multipart/form-data; boundary=abc123"},
      {"x-ca-signed-content-type", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "x-ca-key"},
      // The current signature path still uses the actual Content-Type.
      {"x-ca-signature", "VSrgBSeNr0/GI41fiONAAXNrFPMsMkq8k9SAX+mGSYo="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  headers_ = {
      {":path", "/test"},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "multipart/form-data; boundary=abc123"},
      {"x-ca-signed-content-type", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key"},
      // This signature uses the actual Content-Type. The old API Gateway
      // fallback must use X-Ca-Signed-Content-Type instead and reject it.
      {"x-ca-signature", "NsinyncWBZxFKIk1jOdpA2AsgzxtNP1s598N6LK4Zm0="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopAllIterationAndBuffer);
}

TEST_F(HmacAuthTest, OldApiGatewayFallbackDoesNotDecodeBodyEncodedPlusAsSpace) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  std::string body("keyword=hello%2Bworld");
  headers_ = {
      {":path", "/test"},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "application/x-www-form-urlencoded"},
      {"content-length", std::to_string(body.size())},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key"},
      // Signature for "/test?keyword=hello world". The actual canonical value
      // is "hello+world", so it must not pass.
      {"x-ca-signature", "KR2Db8Dfia2DWde4ZbYVB3hiuU2Ng4Dn5JXp/FdJ/Gc="},
  };
  body_.set(body);
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(context_->onRequestBody(body.size(), true),
            FilterDataStatus::StopIterationNoBuffer);
}

TEST_F(HmacAuthTest, OldApiGatewayFallbackDoesNotUseActualHeaderNames) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  headers_ = {
      {":path", "/test?keyword=hello+world&empty="},
      {":method", "GET"},
      {"accept", "application/json"},
      {"x-ca-key", "appKey"},
      {"X-Custom", "customValue"},
      {"x-ca-signature-headers", "X-Ca-Key,x-custom"},
      // Uses the actual names "X-Custom" and "x-ca-key". The plugin only
      // supports the old SDK's declared Header names, so this must not pass.
      {"x-ca-signature", "zmHMTu34mklp6m+gQxzgzvwXUOBBMfqfZUAe8XTCml4="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopAllIterationAndBuffer);
}

TEST_F(HmacAuthTest, OldApiGatewayFallbackDoesNotOmitMissingSignedHeader) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":["test"],
      "credentials":[
        {"key": "appKey", "secret": "appSecret"}
      ]
    }
  ]
})";
  route_name_ = "test";
  config_.set(configuration);
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  headers_ = {
      {":path", "/test?keyword=hello+world&empty="},
      {":method", "GET"},
      {"accept", "application/json"},
      {"x-ca-key", "appKey"},
      {"x-ca-signature-headers", "X-Ca-Key,X-Missing"},
      // This signature omits X-Missing. The fallback must not silently drop a
      // declared signed Header.
      {"x-ca-signature", "OR6p+EF9UyyIDGjMTp5LR9QtGKZxuMQU4GP6RECMk4g="},
  };
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopAllIterationAndBuffer);
}

}  // namespace hmac_auth
}  // namespace null_plugin
}  // namespace proxy_wasm
