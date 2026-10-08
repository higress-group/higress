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
  MOCK_METHOD(WasmResult, removeHeaderMapValue,
              (WasmHeaderMapType /* type */, std::string_view /* key */));
  MOCK_METHOD(WasmResult, addHeaderMapValue,
              (WasmHeaderMapType /* type */, std::string_view /* key */,
               std::string_view /* value */));
  MOCK_METHOD(WasmResult, sendLocalResponse,
              (uint32_t /* response_code */, std::string_view /* body */,
               Pairs /* additional_headers */, uint32_t /* grpc_status */,
               std::string_view /* details */));
  MOCK_METHOD(uint64_t, getCurrentTimeNanoseconds, ());
  MOCK_METHOD(WasmResult, getProperty, (std::string_view, std::string*));
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

    ON_CALL(*mock_context_, addHeaderMapValue(WasmHeaderMapType::RequestHeaders,
                                              testing::_, testing::_))
        .WillByDefault([&](WasmHeaderMapType, std::string_view key,
                           std::string_view value) { return WasmResult::Ok; });

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
            FilterHeadersStatus::Continue);
  EXPECT_EQ(context_->onRequestBody(body.size(), true),
            FilterDataStatus::Continue);
}

TEST_F(HmacAuthTest, ParamInBodyWithConsumer) {
  headers_ = {
      {":path", "/Third/User/getNyAccessToken"},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "application/x-www-form-urlencoded"},
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
            FilterHeadersStatus::Continue);
  EXPECT_EQ(context_->onRequestBody(body.size(), true),
            FilterDataStatus::Continue);
}

TEST_F(HmacAuthTest, ParamInBodyWrongSignature) {
  headers_ = {
      {":path", "/Third/User/getNyAccessToken"},
      {":method", "POST"},
      {"accept", "application/json"},
      {"content-type", "application/x-www-form-urlencoded"},
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
            FilterHeadersStatus::Continue);
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
              FilterHeadersStatus::StopIteration);
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
              FilterHeadersStatus::StopIteration);
  }
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
            FilterHeadersStatus::Continue);
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
      {"x-ca-signature", "wcQC8014+HW0TumVfXy8+UXI4JDvkhjPlqp6rTE7cZo="},
      {"x-ca-signature-headers",
       "x-ca-timestamp,x-ca-key,x-ca-nonce,x-ca-signature-method"},
  };
  current_time_ = (uint64_t)1525876230 * 1000 * 1000 * 1000;
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(context_->onRequestBody(0, true),
            FilterDataStatus::StopIterationNoBuffer);
  current_time_ = (uint64_t)1525869027 * 1000 * 1000 * 1000;
  EXPECT_EQ(context_->onRequestBody(0, true),
            FilterDataStatus::StopIterationNoBuffer);
  current_time_ = (uint64_t)1525869029 * 1000 * 1000 * 1000;
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
      {"x-ca-signature", "7yl5Rba+3pnp9weLP3af1Hejz4K3RFp+BHL7N2w98/U="},
      {"x-ca-signature-headers",
       "x-ca-timestamp,x-ca-key,x-ca-nonce,x-ca-signature-method"},
  };
  current_time_ = (uint64_t)1525876230 * 1000 * 1000 * 1000;
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(context_->onRequestBody(0, true),
            FilterDataStatus::StopIterationNoBuffer);
  current_time_ = (uint64_t)1525869027 * 1000 * 1000 * 1000;
  EXPECT_EQ(context_->onRequestBody(0, true),
            FilterDataStatus::StopIterationNoBuffer);
  current_time_ = (uint64_t)1525869029 * 1000 * 1000 * 1000;
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

}  // namespace hmac_auth
}  // namespace null_plugin
}  // namespace proxy_wasm
