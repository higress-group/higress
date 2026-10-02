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
#include <unordered_map>

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
  MOCK_METHOD(WasmResult, replaceHeaderMapValue,
              (WasmHeaderMapType /* type */, std::string_view /* key */,
               std::string_view /* value */));
  MOCK_METHOD(WasmResult, removeHeaderMapValue,
              (WasmHeaderMapType /* type */, std::string_view /* key */));
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
          header_map_[std::string(key)] = value;
          return WasmResult::Ok;
        });
    ON_CALL(*mock_context_,
            replaceHeaderMapValue(WasmHeaderMapType::RequestHeaders,
                                  testing::_, testing::_))
        .WillByDefault([&](WasmHeaderMapType, std::string_view key,
                           std::string_view value) {
          header_map_[std::string(key)] = value;
          return WasmResult::Ok;
        });
    ON_CALL(*mock_context_,
            removeHeaderMapValue(WasmHeaderMapType::RequestHeaders, testing::_))
        .WillByDefault([&](WasmHeaderMapType, std::string_view key) {
          header_map_.erase(std::string(key));
          if (key == "x-api-key") {
            key_header_.clear();
          }
          return WasmResult::Ok;
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
  std::map<std::string, std::string> header_map_;
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

TEST_F(KeyAuthTest, KeepCredentialDefaultKeepsCredentialHeader) {
  std::string configuration = R"(
{
  "credentials":["abc"],
  "keys": ["x-api-key"]
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  path_ = "/test?hello=123";
  header_map_.clear();
  header_map_.emplace("x-api-key", "abc");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(header_map_["x-api-key"], "abc");
}

TEST_F(KeyAuthTest, KeepCredentialFalseRemovesCredentialHeader) {
  std::string configuration = R"(
{
  "credentials":["abc"],
  "keys": ["x-api-key"],
  "keep_credential": false
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  path_ = "/test?hello=123";
  header_map_.clear();
  header_map_.emplace("x-api-key", "abc");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(header_map_.find("x-api-key"), header_map_.end());
}

TEST_F(KeyAuthTest, KeepCredentialFalseDoesNotRemoveQueryCredential) {
  std::string configuration = R"(
{
  "credentials":["abc"],
  "keys": ["apiKey"],
  "in_header": false,
  "keep_credential": false
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  path_ = "/test?apiKey=abc";
  header_map_.clear();
  header_map_.emplace("apiKey", "backend-visible");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(header_map_["apiKey"], "backend-visible");
}

TEST_F(KeyAuthTest, KeepCredentialFalseKeepsHeaderOnFailure) {
  std::string configuration = R"(
{
  "credentials":["abc"],
  "keys": ["x-api-key"],
  "keep_credential": false
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  path_ = "/test?hello=123";
  header_map_.clear();
  header_map_.emplace("x-api-key", "bad");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
  EXPECT_EQ(header_map_["x-api-key"], "bad");
}

TEST_F(KeyAuthTest, KeepCredentialFalseRemovesOriginalAuthHeader) {
  std::string configuration = R"(
{
  "credentials":["abc"],
  "keys": ["x-api-key"],
  "keep_credential": false
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  path_ = "/test?hello=123";
  header_map_.clear();
  header_map_.emplace("X-HI-ORIGINAL-AUTH", "abc");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(header_map_.find("X-HI-ORIGINAL-AUTH"), header_map_.end());
}

TEST_F(KeyAuthTest, KeepCredentialFalseRemovesHeaderAfterRbac) {
  std::string configuration = R"({
  "global_auth": false,
  "consumers": [
    {
      "name": "c1",
      "credential": "abc",
      "keys": ["x-api-key"],
      "in_header": true,
      "in_query": false
    }
  ],
  "keep_credential": false,
  "rbac_rules": [
    {
      "principals": [{"any": true}],
      "permissions": [
        {
          "or_rules": [
            {
              "header": {
                "name": "x-api-key",
                "exact_match": "abc"
              }
            }
          ]
        }
      ]
    }
  ],
  "_rules_": [{"_match_route_": ["test"], "enable_auth": true}]
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  path_ = "/test?hello=123";
  header_map_.clear();
  header_map_.emplace("x-api-key", "abc");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(header_map_.find("x-api-key"), header_map_.end());
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

TEST_F(KeyAuthTest, RbacNormalRoute) {
  std::string configuration = R"EOF(
{
  "global_auth": false,
  "consumers": [
    {
      "name": "c1",
      "credentials": [
        "123",
        "345"
      ],
      "keys": [
        "c1key"
      ],
      "in_header": false,
      "in_query": true
    }
  ],
  "rbac_rules": [
    {
      "principals": [
        {
          "or_rules": [
            {
              "consumer": "c1"
            }
          ]
        }
      ],
      "permissions": [
        {
          "or_rules": [
            {
              "route_name": {
                "exact_match": "ip"
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
        "ip"
      ],
      "enable_auth": true
    }
  ]
})EOF";

  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "ip";
  path_ = "/ip?c1key=123";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  path_ = "/ip?c1key=def";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
}

TEST_F(KeyAuthTest, RbacMCPTool) {
  std::string configuration = R"EOF(
{
  "global_auth": false,
  "consumers": [
    {
      "name": "c1",
      "credentials": [
        "c1-k1",
        "c1-k2"
      ],
      "in_header": false,
      "in_query": true,
      "keys": [
        "x-c1-key"
      ]
    },
    {
      "name": "c2",
      "credentials": [
        "c2-k1",
        "c2-k2",
        "c2-k3"
      ],
      "in_header": true,
      "in_query": false,
      "keys": [
        "x-c2-key"
      ]
    }
  ],
  "rbac_rules": [
    {
      "principals": [
        {
          "or_rules": [
            {
              "consumer": "c1"
            },
            {
              "consumer": "c2"
            }
          ]
        }
      ],
      "permissions": [
        {
          "or_rules": [
            {
              "route_name": {
                "exact_match": "ip"
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
              "consumer": "c2"
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
                    "exact_match": "get"
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
        "ip",
        "get"
      ],
      "enable_auth": true
    }
  ]
})EOF";

  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "ip";
  path_ = "/ip?x-c1-key=c1-k1";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  path_ = "/ip";
  header_map_.clear();
  header_map_.emplace("x-c2-key", "c2-k1");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  path_ = "/ip";
  header_map_.clear();
  header_map_.emplace("x-c2-key", "c2-k1-error");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  route_name_ = "get";
  path_ = "/get";

  header_map_.clear();
  header_map_.emplace("x-c2-key", "c2-k1");
  header_map_.emplace("x-envoy-mcp-tool-name", "get_location");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  header_map_.clear();
  header_map_.emplace("x-c2-key", "c2-k1");
  header_map_.emplace("x-envoy-mcp-tool-name", "get_weather");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(KeyAuthTest, RbacMatchTest) {
  std::string configuration = R"EOF(
{
  "global_auth": false,
  "consumers": [
    {
      "name": "c2",
      "credentials": [
        "c2-k1",
        "c2-k2"
      ],
      "in_header": true,
      "in_query": false,
      "keys": [
        "x-c2-key"
      ]
    }
  ],
  "rbac_rules": [
    {
      "principals": [
        {
          "or_rules": [
            {
              "consumer": "c2"
            }
          ]
        }
      ],
      "permissions": [
        {
          "or_rules": [
            {
              "route_name": {
                "prefix_match": "foo_a"
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
              "consumer": "c2"
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
                    "exact_match": "get_weather"
                  }
                },
                {
                  "route_name": {
                    "exact_match": "foo_b_1"
                  }
                }
              ]
            },
            {
              "and_rules": [
                {
                  "header": {
                    "name": "x-envoy-mcp-tool-param",
                    "prefix_match": "get"
                  }
                },
                {
                  "route_name": {
                    "exact_match": "foo_b_2"
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
      "_match_route_prefix_": [
        "foo"
      ],
      "enable_auth": true
    }
  ]
})EOF";

  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "foo_a_1"; // prefix match foo_a
  path_ = "/test";
  header_map_.clear();
  header_map_.emplace("x-c2-key", "c2-k1");
  EXPECT_EQ(context_->onRequestHeaders(0, false), FilterHeadersStatus::Continue);

  route_name_ = "foo_c_1"; // prefix match foo_a
  path_ = "/test";
  header_map_.clear();
  header_map_.emplace("x-c2-key", "c2-k1");
  EXPECT_EQ(context_->onRequestHeaders(0, false), FilterHeadersStatus::StopIteration);

  route_name_ = "foo_b_1";
  path_ = "/test";
  header_map_.clear();
  header_map_.emplace("x-c2-key", "c2-k1");
  header_map_.emplace("x-envoy-mcp-tool-name", "get_weather"); // exact match successfully
  EXPECT_EQ(context_->onRequestHeaders(0, false), FilterHeadersStatus::Continue);

  route_name_ = "foo_b_1";
  path_ = "/test";
  header_map_.clear();
  header_map_.emplace("x-c2-key", "c2-k1");
  header_map_.emplace("x-envoy-mcp-tool-name", "get_weather_error"); // exact match failed
  EXPECT_EQ(context_->onRequestHeaders(0, false), FilterHeadersStatus::StopIteration);

  route_name_ = "foo_b_2";
  path_ = "/test";
  header_map_.clear();
  header_map_.emplace("x-c2-key", "c2-k1");
  header_map_.emplace("x-envoy-mcp-tool-param", "get_xxxx"); // prefix match successfully
  EXPECT_EQ(context_->onRequestHeaders(0, false), FilterHeadersStatus::Continue);

  route_name_ = "foo_b_2";
  path_ = "/test";
  header_map_.clear();
  header_map_.emplace("x-c2-key", "c2-k1");
  header_map_.emplace("x-envoy-mcp-tool-param", "ge_xxxx"); // prefix match failed
  EXPECT_EQ(context_->onRequestHeaders(0, false), FilterHeadersStatus::StopIteration);
}

TEST_F(KeyAuthTest, MCPRbacAddHeaderTest) {
  std::string configuration = R"EOF(
{
  "global_auth": false,
  "consumers": [
    {
      "name": "c2",
      "credentials": [
        "c2-k1",
        "c2-k2"
      ],
      "in_header": true,
      "in_query": false,
      "keys": [
        "x-c2-key"
      ]
    }
  ],
  "rbac_rules": [
    {
      "principals": [
        {
          "or_rules": [
            {
              "consumer": "c2"
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
                  "route_name": {
                    "exact_match": "get"
                  }
                },
                {
                  "or_rules": [
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
                      "not_rule": {
                        "header": {
                          "name": "x-envoy-jsonrpc-method",
                          "exact_match": "tools/call"
                        }
                      },
                      "extra_action": {
                        "add_header": {
                          "x-envoy-allow-mcp-tools": "get_weather,get_ip"
                        }
                      }
                    }
                  ]
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
        "get"
      ],
      "enable_auth": true
    }
  ]
})EOF";

  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "get";
  path_ = "/get";
  header_map_.clear();
  header_map_.emplace("x-c2-key", "c2-k1");
  EXPECT_EQ(context_->onRequestHeaders(0, false), FilterHeadersStatus::Continue);
  EXPECT_EQ(header_map_["x-envoy-allow-mcp-tools"], "get_weather,get_ip");

  header_map_.clear();
  header_map_.emplace("x-c2-key", "c2-k1");
  header_map_.emplace("x-envoy-jsonrpc-method", "tools/call");
  EXPECT_EQ(context_->onRequestHeaders(0, false), FilterHeadersStatus::StopIteration);

  header_map_.clear();
  header_map_.emplace("x-c2-key", "c2-k1");
  header_map_.emplace("x-envoy-jsonrpc-method", "tools/call");
  header_map_.emplace("x-envoy-mcp-tool-name", "get_weather");
  EXPECT_EQ(context_->onRequestHeaders(0, false), FilterHeadersStatus::Continue);
}

TEST_F(KeyAuthTest, ConsumerGroupHeaderCommaJoined) {
  std::string configuration = R"({
  "consumers": [
    {"name": "c1", "credential": "abc", "group": ["g1", "g2"]}
  ],
  "keys": ["apiKey"],
  "rbac_rules": [
    {
      "principals": [{"any": true}],
      "permissions": [
        {
          "or_rules": [
            {"route_name": {"exact_match": "test"}}
          ]
        }
      ]
    }
  ],
  "_rules_": [{"_match_route_": ["test"], "enable_auth": true}]
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  path_ = "/test?apiKey=abc";
  header_map_.clear();
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(header_map_["X-Mse-Consumer"], "c1");
  EXPECT_EQ(header_map_["X-Mse-Consumer-Group"], "g1,g2");
}

TEST_F(KeyAuthTest, ConsumerGroupHeaderEmptyWhenOmitted) {
  std::string configuration = R"({
  "consumers": [
    {"name": "c1", "credential": "abc"}
  ],
  "keys": ["apiKey"],
  "rbac_rules": [
    {
      "principals": [{"any": true}],
      "permissions": [
        {
          "or_rules": [
            {"route_name": {"exact_match": "test"}}
          ]
        }
      ]
    }
  ],
  "_rules_": [{"_match_route_": ["test"], "enable_auth": true}]
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  path_ = "/test?apiKey=abc";
  header_map_.clear();
  header_map_.emplace("X-Mse-Consumer-Group", "spoofed");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(header_map_["X-Mse-Consumer"], "c1");
  auto group_it = header_map_.find("X-Mse-Consumer-Group");
  ASSERT_NE(group_it, header_map_.end());
  EXPECT_EQ(group_it->second, "");
}

TEST_F(KeyAuthTest, ClientSuppliedConsumerHeaderIsReplacedOnConsumerPath) {
  std::string configuration = R"({
  "consumers": [
    {"name": "c1", "credential": "abc"}
  ],
  "keys": ["apiKey"],
  "_rules_": [{"_match_route_": ["test"], "allow": ["c1"]}]
})";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  path_ = "/test?apiKey=abc";
  header_map_.clear();
  header_map_.emplace("X-Mse-Consumer", "spoofed-consumer");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(header_map_["X-Mse-Consumer"], "c1");
}

TEST_F(KeyAuthTest, ClientSuppliedConsumerHeaderRemovedOnCredentialOnlyPath) {
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
  header_map_.clear();
  header_map_.emplace("X-Mse-Consumer", "spoofed-consumer");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(header_map_.find("X-Mse-Consumer"), header_map_.end());
}

TEST_F(KeyAuthTest, RbacAllowsConsumerGroupPrincipal) {
  std::string configuration = R"EOF(
{
  "global_auth": false,
  "consumers": [
    {
      "name": "c1",
      "credentials": ["123"],
      "keys": ["c1key"],
      "in_header": false,
      "in_query": true,
      "group": ["partner-a"]
    }
  ],
  "rbac_rules": [
    {
      "principals": [
        {
          "or_rules": [
            {
              "consumer_group": " partner-a "
            }
          ]
        }
      ],
      "permissions": [
        {
          "or_rules": [
            {
              "route_name": {
                "exact_match": "ip"
              }
            }
          ]
        }
      ]
    }
  ],
  "_rules_": [
    {
      "_match_route_": ["ip"],
      "enable_auth": true
    }
  ]
})EOF";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "ip";
  path_ = "/ip?c1key=123";
  header_map_.clear();
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(header_map_["X-Mse-Consumer-Group"], "partner-a");
}

TEST_F(KeyAuthTest, RbacDeniesWhenConsumerGroupMismatch) {
  std::string configuration = R"EOF(
{
  "global_auth": false,
  "consumers": [
    {
      "name": "c1",
      "credentials": ["123"],
      "keys": ["c1key"],
      "in_header": false,
      "in_query": true,
      "group": ["partner-b"]
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
          "or_rules": [
            {
              "route_name": {
                "exact_match": "ip"
              }
            }
          ]
        }
      ]
    }
  ],
  "_rules_": [
    {
      "_match_route_": ["ip"],
      "enable_auth": true
    }
  ]
})EOF";
  BufferBase buffer;
  buffer.set(configuration);
  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "ip";
  path_ = "/ip?c1key=123";
  header_map_.clear();
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
}


}  // namespace key_auth
}  // namespace null_plugin
}  // namespace proxy_wasm
