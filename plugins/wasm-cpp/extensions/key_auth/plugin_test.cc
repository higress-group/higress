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

#include "extensions/key_auth/plugin.h"

#include "common/base64.h"
#include "gmock/gmock.h"
#include "gtest/gtest.h"
#include "include/proxy-wasm/context.h"
#include "include/proxy-wasm/null.h"

namespace proxy_wasm {
namespace null_plugin {
namespace key_auth {

NullPluginRegistry* context_registry_;
RegisterNullVmPluginFactory register_key_auth_plugin("key_auth", []() {
  return std::make_unique<NullPlugin>(key_auth::context_registry_);
});

class MockContext : public proxy_wasm::ContextBase {
 public:
  MockContext(WasmBase* wasm) : ContextBase(wasm) {}

  MOCK_METHOD(BufferInterface*, getBuffer, (WasmBufferType));
  MOCK_METHOD(WasmResult, log, (uint32_t, std::string_view));
  MOCK_METHOD(WasmResult, getHeaderMapValue,
              (WasmHeaderMapType /* type */, std::string_view /* key */,
               std::string_view* /*result */));
  MOCK_METHOD(WasmResult, addHeaderMapValue,
              (WasmHeaderMapType /* type */, std::string_view /* key */,
               std::string_view /* value */));
  MOCK_METHOD(WasmResult, sendLocalResponse,
              (uint32_t /* response_code */, std::string_view /* body */,
               Pairs /* additional_headers */, uint32_t /* grpc_status */,
               std::string_view /* details */));
  MOCK_METHOD(WasmResult, getProperty, (std::string_view, std::string*));
};

class KeyAuthTest : public ::testing::Test {
 protected:
  KeyAuthTest() {
    // Initialize test VM
    test_vm_ = createNullVm();
    wasm_base_ = std::make_unique<WasmBase>(
        std::move(test_vm_), "test-vm", "", "",
        std::unordered_map<std::string, std::string>{},
        AllowedCapabilitiesMap{});
    wasm_base_->load("key_auth");
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
          if (header == "x-api-key") {
            *result = key_header_;
          }
          auto it = headers_.find(std::string(header));
          if (it != headers_.end()) {
            *result = it->second;
          }
          return WasmResult::Ok;
        });
    ON_CALL(*mock_context_, addHeaderMapValue(WasmHeaderMapType::RequestHeaders,
                                              testing::_, testing::_))
        .WillByDefault([&](WasmHeaderMapType, std::string_view key,
                           std::string_view value) { return WasmResult::Ok; });

    ON_CALL(*mock_context_, getProperty(testing::_, testing::_))
        .WillByDefault([&](std::string_view path, std::string* result) {
          *result = route_name_;
          return WasmResult::Ok;
        });

    // Initialize Wasm sandbox context
    root_context_ = std::make_unique<PluginRootContext>(0, "");
    context_ = std::make_unique<PluginContext>(1, root_context_.get());
  }
  ~KeyAuthTest() override {}

  std::unique_ptr<WasmBase> wasm_base_;
  std::unique_ptr<WasmVm> test_vm_;
  std::unique_ptr<MockContext> mock_context_;

  std::unique_ptr<PluginRootContext> root_context_;
  std::unique_ptr<PluginContext> context_;

  std::string path_;
  std::string authority_;
  std::string route_name_;
  std::string key_header_;
  std::unordered_map<std::string, std::string> headers_;
};

TEST_F(KeyAuthTest, InQuery) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_": ["test"],
      "credentials":["abc","def"],
      "keys": ["apiKey", "x-api-key"]
    }
  ]  
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  path_ = "/test?hello=123&apiKey=abc";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  path_ = "/test?hello=123";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  path_ = "/test?hello=123&apiKey=123";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  path_ = "/test?hello=123&apiKey=123&x-api-key=def";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  
  route_name_ = "pass";
  path_ = "/pass?hello=123&apiKey=123";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(KeyAuthTest, InQueryWithConsumer) {
  std::string configuration = R"(
{
  "consumers" : [ {"credential" : "abc", "name" : "consumer1"} ],
  "keys" : [ "apiKey", "x-api-key" ],
  "_rules_" : [ {"_match_route_" : ["test"], "allow" : ["consumer1"]} ]
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  path_ = "/test?hello=1&apiKey=abc";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  path_ = "/test?hello=123";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  path_ = "/test?hello=123&apiKey=123";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
}

TEST_F(KeyAuthTest, EmptyAllowSet) {
  std::string configuration = R"(
{
  "consumers" : [{"credential" : "abc", "name" : "consumer1"}],
  "keys" : [ "apiKey", "x-api-key" ],
  "_rules_" : [ {"_match_route_" : ["test"], "allow" : []}, {"_match_route_prefix_" : ["prefix"], "allow" : []} ]
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  path_ = "/test?hello=1&apiKey=abc";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  route_name_ = "noauth";
  path_ = "/test?hello=1&apiKey=abc";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  route_name_ = "prefix@operation";
  path_ = "/test?hello=1";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
}

TEST_F(KeyAuthTest, EmptyConsumer) {
  std::string configuration = R"(
{
  "consumers" : [],
  "keys" : [ "apiKey", "x-api-key" ],
  "_rules_" : [ {"_match_route_" : ["test"], "allow" : []} ]
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  path_ = "/test?hello=1&apiKey=abc";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  route_name_ = "test2";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(KeyAuthTest, InHeader) {
  std::string configuration = R"(
{
  "credentials":["abc", "xyz"],
  "keys": ["x-api-key"]
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  path_ = "/test?hello=123";
  key_header_ = "abc";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  path_ = "/test?hello=123";
  key_header_ = "xyz";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  path_ = "/test?hello=123";
  key_header_ = "";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  path_ = "/test?hello=123";
  key_header_ = "123";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
}

TEST_F(KeyAuthTest, InHeaderWithConsumer) {
  std::string configuration = R"(
{
  "consumers" : [ {"credential" : "abc", "name" : "consumer1"},
                  {"credential" : "xyz", "name" : "consumer1"} ],
  "keys": ["x-api-key"]
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  path_ = "/test?hello=123";
  key_header_ = "abc";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  path_ = "/test?hello=123";
  key_header_ = "xyz";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  path_ = "/test?hello=123";
  key_header_ = "";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  path_ = "/test?hello=123";
  key_header_ = "123";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
}

TEST_F(KeyAuthTest, ConsumerDifferentKey) {
  std::string configuration = R"(
{
  "consumers" : [ {"credential" : "abc", "name" : "consumer1", "keys" : [ "apiKey" ]}, {"credential" : "123", "name" : "consumer2"} ],
  "keys" : [ "apiKey2" ],
  "_rules_" : [ {"_match_route_" : ["test"], "allow" : ["consumer1"]}, {"_match_route_" : ["test2"], "allow" : ["consumer2"]} ]
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  path_ = "/test?hello=1&apiKey=abc";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  route_name_ = "test";
  path_ = "/test?hello=1&apiKey2=abc";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  route_name_ = "test";
  path_ = "/test?hello=123&apiKey2=123";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  route_name_ = "test2";
  path_ = "/test?hello=123&apiKey2=123";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(KeyAuthTest, ConsumerMultiCredentials) {
  std::string configuration = R"(
{
  "global_auth": false,
  "consumers": [
    {
      "name": "c1",
      "credentials":["123","345"],
      "keys": ["c1key"],
      "in_header": false,
      "in_query": true
    },
    {
      "name": "c2",
      "credentials":["abc","def"],
      "keys": ["c2key"],
      "in_header": false,
      "in_query": true
    }
  ],
  "_rules_": [
    {
      "_match_route_": ["test"],
      "allow": ["c1"]
    }
  ]  
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  path_ = "/test?c1key=123";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  path_ = "/test?c2key=adc";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
}

TEST_F(KeyAuthTest, ConsumerDefaultKey) {
  std::string configuration = R"(
{
  "global_auth": false,
  "consumers": [
    {
      "name": "c1",
      "credentials":["123","345"],
      "keys": ["c1key"],
      "in_header": false,
      "in_query": true
    },
    {
      "name": "c2",
      "credentials":["abc","def"]
    }
  ],
  "_rules_": [
    {
      "_match_route_": ["test"],
      "allow": ["c2"]
    }
  ],
  "keys": ["defaultkey"]
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  path_ = "/test?c1key=123";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  path_ = "/test?defaultkey=def";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(KeyAuthTest, SharedKeyNameDifferentExtractionSource) {
  std::string configuration = R"(
{
  "global_auth": false,
  "consumers": [
    {
      "name": "c1",
      "credentials": ["query-cred"],
      "keys": ["apikey"],
      "in_query": true,
      "in_header": false
    },
    {
      "name": "c2",
      "credentials": ["header-cred"],
      "keys": ["apikey"],
      "in_query": false,
      "in_header": true
    }
  ],
  "_rules_": [
    {
      "_match_route_": ["test"],
      "allow": ["c1"]
    },
    {
      "_match_route_": ["test2"],
      "allow": ["c2"]
    }
  ]
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  // c1 extracts from query
  route_name_ = "test";
  path_ = "/test?apikey=query-cred";
  headers_.clear();
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  // c1 should not match header credential
  path_ = "/test";
  headers_["apikey"] = "query-cred";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  // c2 extracts from header
  route_name_ = "test2";
  path_ = "/test";
  headers_["apikey"] = "header-cred";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  // c2 should not match query credential
  headers_.clear();
  path_ = "/test?apikey=header-cred";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
}

TEST_F(KeyAuthTest, NoGlobalKeySetting) {
  std::string configuration = R"(
{
  "global_auth": false,
  "consumers": [
    {
      "name": "c1",
      "credentials":["123","345"],
      "keys": ["c1key"],
      "in_header": false,
      "in_query": true
    },
    {
      "name": "c2",
      "credentials":["abc","def"],
      "keys": ["c2key"]
    }
  ],
  "_rules_": [
    {
      "_match_route_": ["test"],
      "allow": ["c2"]
    }
  ]
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  path_ = "/test?c1key=123";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  path_ = "/test?c2key=def";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

}  // namespace key_auth
}  // namespace null_plugin
}  // namespace proxy_wasm
