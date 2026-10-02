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

#include "extensions/basic_auth/plugin.h"

#include "common/base64.h"
#include "gmock/gmock.h"
#include "gtest/gtest.h"
#include "include/proxy-wasm/context.h"
#include "include/proxy-wasm/null.h"

namespace proxy_wasm {
namespace null_plugin {
namespace basic_auth {

NullPluginRegistry* context_registry_;
RegisterNullVmPluginFactory register_basic_auth_plugin("basic_auth", []() {
  return std::make_unique<NullPlugin>(basic_auth::context_registry_);
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

class BasicAuthTest : public ::testing::Test {
 protected:
  BasicAuthTest() {
    // Initialize test VM
    test_vm_ = createNullVm();
    wasm_base_ = std::make_unique<WasmBase>(
        std::move(test_vm_), "test-vm", "", "",
        std::unordered_map<std::string, std::string>{},
        AllowedCapabilitiesMap{});
    wasm_base_->load("basic_auth");
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
          if (header == "authorization") {
            if (authorization_header_.empty()) {
              authorization_header_ =
                  "Basic " + Base64::encode(cred_.data(), cred_.size());
            }
            *result = authorization_header_;
          }
          return WasmResult::Ok;
        });
    ON_CALL(*mock_context_, addHeaderMapValue(WasmHeaderMapType::RequestHeaders,
                                              testing::_, testing::_))
        .WillByDefault([&](WasmHeaderMapType, std::string_view key,
                           std::string_view value) { return WasmResult::Ok; });

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
  ~BasicAuthTest() override {}

  std::unique_ptr<WasmBase> wasm_base_;
  std::unique_ptr<WasmVm> test_vm_;
  std::unique_ptr<MockContext> mock_context_;

  std::unique_ptr<PluginRootContext> root_context_;
  std::unique_ptr<PluginContext> context_;

  std::string authority_;
  std::string cred_;
  std::string route_name_;
  std::string authorization_header_;
  std::map<std::string, std::string> header_map_;
};

TEST_F(BasicAuthTest, OnConfigureSuccess) {
  // without consumer
  {
    std::string configuration = R"(
{
  "credentials":[ "ok:test", "admin:admin", "admin2:admin2",
  "YWRtaW4zOmFkbWluMw==" ],
  "_rules_": [
    {
      "_match_route_":[ "abc", "test" ],
      "credentials":[ "ok:test", "admin:admin", "admin2:admin2",
      "YWRtaW4zOmFkbWluMw==" ]
    },
    {
      "_match_domain_":[ "test.com", "*.example.com" ],
      "credentials":[ "admin:admin", "admin2:admin2", "ok:test",
      "YWRtaW4zOmFkbWluMw==" ]
    }
  ]
})";

    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
  }

  // with consumer
  {
    std::string configuration = R"(
{
  "consumers" : [
    {"credential" : "getuser1:123456", "name" : "consumer1"},
    {"credential" : "getuser2:123456", "name" : "consumer2"},
    {"credential" : "postuser1:123456", "name" : "consumer3"},
    {"credential" : "postuser2:123456", "name" : "consumer4"}
  ],
  "_rules_" : [
    {
      "_match_route_" : ["route-1"], 
     "allow" : [ "consumer1", "consumer2" ]
    }, 
    {
      "_match_domain_" : ["*.example.com"],
      "allow" : [ "consumer3", "consumer4" ]
    }
  ]
})";
    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
  }
}

TEST_F(BasicAuthTest, OnConfigureNoRules) {
  // without consumer
  {
    std::string configuration = R"(
{
   "credentials":[ "ok:test", "admin:admin", "admin2:admin2" ]
})";

    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
  }

  // with consumer
  {
    std::string configuration = R"(
{
  "consumers" : [
    {"credential" : "getuser1:123456", "name" : "consumer1"},
    {"credential" : "getuser2:123456", "name" : "consumer2"},
    {"credential" : "postuser1:123456", "name" : "consumer3"},
    {"credential" : "postuser2:123456", "name" : "consumer4"}
  ]
})";
    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
  }
}

TEST_F(BasicAuthTest, OnConfigureOnlyRules) {
  // Rules carry their own credentials/consumers next to a match key, so the
  // matcher keeps a non-zero local config size and parses the rule
  // (route_rule_matcher.h:872). Domain patterns are not validated, so
  // "test.com.*" is accepted as a prefix match (route_rule_matcher.h:1046).
  // without consumer
  {
    std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_domain_":[ "test.com.*"],
      "credentials":[ "ok:test", "admin:admin", "admin2:admin2" ]
    }
  ]
})";

    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
  }

  // with consumer
  {
    std::string configuration = R"(
{
  "_rules_" : [
    {
      "_match_route_" : ["route-1"],
      "consumers" : [
        {"credential" : "getuser1:123456", "name" : "consumer1"},
        {"credential" : "getuser2:123456", "name" : "consumer2"}
      ]
    },
    {
      "_match_domain_" : ["*.example.com"],
      "consumers" : [
        {"credential" : "postuser1:123456", "name" : "consumer3"},
        {"credential" : "postuser2:123456", "name" : "consumer4"}
      ]
    }
  ]
})";

    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
  }
}

TEST_F(BasicAuthTest, OnConfigureEmptyRules) {
  // Each rule holds only its credential material, so the matcher's
  // `config.size() - 1` local config size drops to zero, parsePluginConfig is
  // skipped, and the rule is rejected for missing every match key
  // (route_rule_matcher.h:903). Both sub-cases fail configuration.
  // without consumer
  {
    std::string configuration = R"(
{
  "_rules_": [
    {
      "credentials":[ "ok:test", "admin:admin", "admin2:admin2" ]
    }
  ]
})";

    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_FALSE(root_context_->configure(configuration.size()));
  }

  // with consumer
  {
    std::string configuration = R"(
{
  "_rules_" : [
    {
      "consumers" : [
        {"credential" : "getuser1:123456", "name" : "consumer1"},
        {"credential" : "getuser2:123456", "name" : "consumer2"}
      ]
    }
  ]
})";

    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_FALSE(root_context_->configure(configuration.size()));
  }
}

TEST_F(BasicAuthTest, OnConfigureDuplicateRules) {
  // Carrying several match keys in one rule is not a duplicate error: the
  // matcher only requires at least one (route_rule_matcher.h:903) and resolves
  // the overlap by category precedence, route winning over domain
  // (route_rule_matcher.h:910). Both sub-cases configure successfully.
  // without consumer
  {
    std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_domain_": ["abc.com"],
      "_match_route_": ["abc"],
      "credentials":[ "ok:test", "admin:admin", "admin2:admin2" ]
    }
  ]
})";

    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
  }

  // with consumer
  {
    std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_domain_": ["abc.com"],
      "_match_route_": ["abc"],
      "consumers" : [
        {"credential" : "getuser1:123456", "name" : "consumer1"},
        {"credential" : "getuser2:123456", "name" : "consumer2"}
      ]
    }
  ]
})";

    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
  }
}

TEST_F(BasicAuthTest, OnConfigureNoCredentials) {
  // without consumer
  {
    std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":[ "abc", "test" ],
      "credentials":[ ]
    }
  ]
})";

    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
  }

  // with consumer
  {
    std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_route_":[ "abc", "test" ],
      "consumers":[ ]
    }
  ]
})";

    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
  }
}

TEST_F(BasicAuthTest, OnConfigureEmptyConfig) {
  std::string configuration = "{}";

  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_FALSE(root_context_->configure(configuration.size()));
}

TEST_F(BasicAuthTest, OnConfigureDuplicateCredential) {
  // without consumer
  // "admin:admin" base64 encoded is "YWRtaW46YWRtaW4="
  {
    std::string configuration = R"(
{
   "credentials":[ "admin:admin", "YWRtaW46YWRtaW4=" ]
})";

    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
  }

  // with consumer
  // a consumer credential cannot be mapped to two name
  {
    std::string configuration = R"(
{
  "consumers" : [
    {"credential" : "admin:admin", "name" : "consumer1"},
    {"credential" : "YWRtaW46YWRtaW4=", "name" : "consumer2"},
  ]
})";

    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_FALSE(root_context_->configure(configuration.size()));
  }

  // with consumer
  // two consumer credentials can be mapped to the same name
  {
    std::string configuration = R"(
{
  "consumers" : [
    {"credential" : "admin:admin", "name" : "consumer"},
    {"credential" : "admin2:admin2", "name" : "consumer"}
  ]
})";

    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
  }
}

TEST_F(BasicAuthTest, OnConfigureCredentialsWithConsumers) {
  std::string configuration = R"(
{
  "_rules_" : [
    {
      "_match_route_" : ["route-1"],
      "consumers" : [
        {"credential" : "getuser1:123456", "name" : "consumer1"}
      ],
      "credentials" : ["ok:test", "admin:admin", "admin2:admin2"]
    }
  ]
})";

  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_FALSE(root_context_->configure(configuration.size()));
}

TEST_F(BasicAuthTest, RuleAllow) {
  std::string configuration = R"(
  {
    "_rules_": [
      {
        "_match_route_":[ "test", "config" ],
        "credentials":[ "ok:test", "admin2:admin2", "YWRtaW4zOmFkbWluMw==" ]
      },
      {
        "_match_domain_":[ "test.com", "*.example.com" ],
        "credentials":[ "admin:admin"]
      }
    ]
  })";

  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  cred_ = "ok:test";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  route_name_ = "config";
  cred_ = "admin2:admin2";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  cred_ = "admin3:admin3";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  route_name_ = "nope";
  authority_ = "www.example.com:8080";
  cred_ = "admin:admin";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(BasicAuthTest, EmptyConsumer) {
  std::string configuration = R"(
{
  "consumers" : [],
  "_rules_" : [
    {
      "_match_route_" : ["test"], 
      "allow" : []
    }
  ]
})";

  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  cred_ = "ok:test";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  route_name_ = "config";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(BasicAuthTest, RuleWithConsumerAllow) {
  std::string configuration = R"(
{
  "consumers" : [
    {"credential" : "ok:test", "name" : "consumer_ok"},
    {"credential" : "admin2:admin2", "name" : "consumer2"},
    {"credential" : "YWRtaW4zOmFkbWluMw==", "name" : "consumer3"},
    {"credential" : "admin:admin", "name" : "consumer"}
  ],
  "_rules_" : [
    {
      "_match_route_" : ["test", "config"], 
      "allow" : [ "consumer_ok", "consumer2", "consumer3"]
    }, 
    {
      "_match_domain_" : ["test.com", "*.example.com"],
      "allow" : [ "consumer" ]
    }
  ]
})";

  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  cred_ = "ok:test";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  route_name_ = "config";
  cred_ = "admin2:admin2";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  cred_ = "admin3:admin3";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  route_name_ = "nope";
  authority_ = "www.example.com:8080";
  cred_ = "admin:admin";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(BasicAuthTest, ClientSuppliedConsumerHeaderIsReplaced) {
  std::string configuration = R"(
{
  "consumers" : [
    {"credential" : "ok:test", "name" : "consumer_ok"}
  ],
  "_rules_" : [
    {
      "_match_route_" : ["test"],
      "allow" : [ "consumer_ok"]
    }
  ]
})";

  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  cred_ = "ok:test";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  header_map_.clear();
  header_map_.emplace("X-Mse-Consumer", "spoofed-consumer");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(header_map_["X-Mse-Consumer"], "consumer_ok");
}

TEST_F(BasicAuthTest, ClientSuppliedConsumerHeaderRemovedWithoutName) {
  std::string configuration = R"(
{
  "credentials": ["ok:test"],
  "_rules_" : [
    {
      "_match_route_" : ["test"]
    }
  ]
})";

  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test";
  cred_ = "ok:test";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  header_map_.clear();
  header_map_.emplace("X-Mse-Consumer", "spoofed-consumer");
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
  EXPECT_EQ(header_map_.find("X-Mse-Consumer"), header_map_.end());
}

TEST_F(BasicAuthTest, GlobalAuthRuleWithDomainPort) {
  std::string configuration = R"(
{
  "global_auth": true,
  "consumers" : [
    {"credential" : "ok:test", "name" : "consumer_ok"},
    {"credential" : "admin2:admin2", "name" : "consumer2"},
    {"credential" : "YWRtaW4zOmFkbWluMw==", "name" : "consumer3"},
    {"credential" : "admin:admin", "name" : "consumer"}
  ],
  "_rules_" : [
    {
      "_match_domain_" : ["test.com", "*.example.com"],
      "allow" : [ "consumer" ]
    }
  ]
})";

  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  authority_ = "www.example.com:8080";
  cred_ = "admin:admin";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  cred_ = "admin2:admin2";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  authority_ = "abc.com";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(BasicAuthTest, RuleWithEncryptedConsumerAllow) {
  std::string configuration = R"(
{
  "encrypted": true,
  "consumers" : [
    {"credential" : "myName:$2y$05$c4WoMPo3SXsafkva.HHa6uXQZWr7oboPiC2bT/r7q1BB8I2s0BRqC", "name": "consumer"}
  ],
  "_rules_" : [
    {
      "_match_route_" : ["test_allow"],
      "allow" : [ "consumer"]
    },
    {
      "_match_route_" : ["test_deny"],
      "allow" : []
    }
  ]
})";

  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  route_name_ = "test_allow";
  cred_ = "myName:myPassword";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  route_name_ = "test_deny";
  cred_ = "abc:123";
  authorization_header_ = "";
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
}

TEST_F(BasicAuthTest, RuleDeny) {
  std::string configuration = R"(
{
  "_rules_": [
    {
      "_match_domain_":[ "test.com", "example.*" ],
      "credentials":[ "ok:test", "admin:admin", "admin2:admin2" ]
    }
  ]
})";

  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  authority_ = "example.com";
  cred_ = "wrong-cred";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  authority_ = "example.com";
  cred_ = "admin2:admin2";
  authorization_header_ = Base64::encode(cred_.data(), cred_.size());
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
}

TEST_F(BasicAuthTest, RuleWithConsumerDeny) {
  std::string configuration = R"(
{
  "consumers" : [
    {"credential" : "ok:test", "name" : "consumer_ok"},
    {"credential" : "admin:admin", "name" : "consumer"}
  ],
  "_rules_" : [
    {
      "_match_domain_" : ["test.com", "*.example.com"],
      "allow" : [ "consumer" ]
    }
  ]
})";

  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  authority_ = "www.example.com";
  cred_ = "ok:test";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_CALL(*mock_context_, sendLocalResponse(403, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  authority_ = "www.example.com";
  cred_ = "admin:admin";
  authorization_header_ = Base64::encode(cred_.data(), cred_.size());
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
}

TEST_F(BasicAuthTest, GlobalAllow) {
  std::string configuration = R"(
{
  "credentials":[ "ok:test", "admin:admin", "admin2:admin2" ],
  "_rules_": [
    { 
      "_match_route_":[ "test", "config" ],
      "credentials":[ "admin3:admin3", "YWRtaW4zOmFkbWluMw==" ]
    },
    { 
      "_match_domain_":[ "test.com", "*.example.com" ],
      "credentials":[ "admin4:admin4"]
    },
    {
      "_match_route_":["crypt"],
      "credentials": ["myName:rqXexS6ZhobKA"],
      "encrypted": true
    },
    {
      "_match_route_":["bcrypt"],
      "credentials": ["myName:$2y$05$c4WoMPo3SXsafkva.HHa6uXQZWr7oboPiC2bT/r7q1BB8I2s0BRqC"],
      "encrypted": true
    },
    {
      "_match_route_":["apr1"],
      "credentials": ["myName:$apr1$EXfBN1bF$nuywSFTnPTcqbH5z4x6IG/"],
      "encrypted": true
    },
    {
      "_match_route_":["plain"],
      "credentials": ["myName:{PLAIN}myPassword"],
      "encrypted": true
    },
    {
      "_match_route_":["sha"],
      "credentials": ["myName:{SHA}VBPuJHI7uixaa6LQGWx4s+5GKNE="],
      "encrypted": true
    },
    {
      "_match_route_":["ssha"],
      "credentials": ["myName:{SSHA}98JUfJee5Wb13m5683sLku40P3Y2VjNX"],
      "encrypted": true
    }
  ]
})";

  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  cred_ = "ok:test";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  authority_ = "test.com";
  cred_ = "admin4:admin4";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  route_name_ = "test";
  cred_ = "admin3:admin3";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  authority_ = "";
  authorization_header_ = "";
  cred_ = "myName:myPassword";

  route_name_ = "crypt";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  route_name_ = "bcrypt";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  route_name_ = "apr1";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  route_name_ = "plain";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  route_name_ = "sha";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  route_name_ = "ssha";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(BasicAuthTest, GlobalWithConsumerAllow) {
  std::string configuration = R"(
{
  "consumers" : [
    {"credential" : "ok:test", "name" : "consumer_ok"},
    {"credential" : "admin2:admin2", "name" : "consumer2"},
    {"credential" : "admin:admin", "name" : "consumer"}
  ],
  "_rules_" : [
    {
      "_match_route_" : ["test", "config"], 
      "consumers" : [
        {"credential" : "admin3:admin3", "name" : "consumer3"},
        {"credential" : "YWRtaW41OmFkbWluNQ==", "name" : "consumer5"} 
      ]
    }, 
    {
      "_match_domain_" : ["test.com", "*.example.com"],
      "consumers" : [
        {"credential" : "admin4:admin4", "name" : "consumer4"}
      ]
    },
    {
      "_match_route_" : ["crypt"],
      "encrypted" : true,
      "consumers" : [
        {"credential" : "myName:$2y$05$c4WoMPo3SXsafkva.HHa6uXQZWr7oboPiC2bT/r7q1BB8I2s0BRqC", "name": "consumer crypt"}
      ]
    }
  ]
})";

  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  cred_ = "ok:test";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  authority_ = "test.com";
  cred_ = "admin4:admin4";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  route_name_ = "test";
  cred_ = "admin3:admin3";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);

  authorization_header_ = "";
  authority_ = "";
  route_name_ = "crypt";
  cred_ = "myName:myPassword";
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::Continue);
}

TEST_F(BasicAuthTest, GlobalDeny) {
  std::string configuration = R"(
{
  "credentials":[ "ok:test", "admin:admin", "admin2:admin2" ],
  "_rules_": [
    { 
      "_match_route_":[ "test", "config" ],
      "credentials":[ "admin3:admin3", "YWRtaW4zOmFkbWluMw==" ]
    },
    { 
      "_match_domain_":[ "test.com", "*.example.com" ],
      "credentials":[ "admin4:admin4"]
    }
  ]
})";
  BufferBase buffer;
  buffer.set({configuration.data(), configuration.size()});

  EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
      .WillOnce([&buffer](WasmBufferType) { return &buffer; });
  EXPECT_TRUE(root_context_->configure(configuration.size()));

  cred_ = "wrong-cred";
  route_name_ = "config";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  authority_ = "www.example.com";
  cred_ = "admin2:admin2";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);

  route_name_ = "config";
  cred_ = "admin4:admin4";
  authorization_header_ = "Basic " + Base64::encode(cred_.data(), cred_.size());
  EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                testing::_, testing::_));
  EXPECT_EQ(context_->onRequestHeaders(0, false),
            FilterHeadersStatus::StopIteration);
}

TEST_F(BasicAuthTest, GlobalWithConsumerDeny) {
  {
    std::string configuration = R"(
{
  "consumers" : [
    {"credential" : "ok:test", "name" : "consumer_ok"},
    {"credential" : "admin2:admin2", "name" : "consumer2"},
    {"credential" : "admin:admin", "name" : "consumer"}
  ],
  "_rules_" : [
    {
      "_match_route_" : ["test", "config"], 
      "consumers" : [
        {"credential" : "admin3:admin3", "name" : "consumer3"},
        {"credential" : "YWRtaW41OmFkbWluNQ==", "name" : "consumer5"} 
      ]
    }, 
    {
      "_match_domain_" : ["test.com", "*.example.com"],
      "consumers" : [
        {"credential" : "admin4:admin4", "name" : "consumer4"}
      ]
    }
  ]
})";
    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));

    cred_ = "wrong-cred";
    route_name_ = "not match";
    authorization_header_ =
        "Basic " + Base64::encode(cred_.data(), cred_.size());
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::Continue);

    cred_ = "wrong-cred";
    route_name_ = "config";
    authorization_header_ =
        "Basic " + Base64::encode(cred_.data(), cred_.size());
    EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                  testing::_, testing::_));
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::StopIteration);

    authority_ = "www.example.com";
    cred_ = "admin2:admin2";
    authorization_header_ =
        "Basic " + Base64::encode(cred_.data(), cred_.size());
    EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                  testing::_, testing::_));
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::StopIteration);

    route_name_ = "config";
    cred_ = "admin4:admin4";
    authorization_header_ =
        "Basic " + Base64::encode(cred_.data(), cred_.size());
    EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                  testing::_, testing::_));
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::StopIteration);
  }
  {
    std::string configuration = R"(
{
  "global_auth": true,
  "consumers" : [
    {"credential" : "ok:test", "name" : "consumer_ok"},
    {"credential" : "admin2:admin2", "name" : "consumer2"},
    {"credential" : "admin:admin", "name" : "consumer"}
  ],
  "_rules_" : [
    {
      "_match_route_" : ["test", "config"], 
      "consumers" : [
        {"credential" : "admin3:admin3", "name" : "consumer3"},
        {"credential" : "YWRtaW41OmFkbWluNQ==", "name" : "consumer5"} 
      ]
    }, 
    {
      "_match_domain_" : ["test.com", "*.example.com"],
      "consumers" : [
        {"credential" : "admin4:admin4", "name" : "consumer4"}
      ]
    }
  ]
})";
    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));

    cred_ = "wrong-cred";
    route_name_ = "not match";
    authorization_header_ =
        "Basic " + Base64::encode(cred_.data(), cred_.size());
    EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                  testing::_, testing::_));
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::StopIteration);

    cred_ = "wrong-cred";
    route_name_ = "config";
    authorization_header_ =
        "Basic " + Base64::encode(cred_.data(), cred_.size());
    EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                  testing::_, testing::_));
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::StopIteration);

    authority_ = "www.example.com";
    cred_ = "admin2:admin2";
    authorization_header_ =
        "Basic " + Base64::encode(cred_.data(), cred_.size());
    EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                  testing::_, testing::_));
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::StopIteration);

    route_name_ = "config";
    cred_ = "admin4:admin4";
    authorization_header_ =
        "Basic " + Base64::encode(cred_.data(), cred_.size());
    EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                  testing::_, testing::_));
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::StopIteration);
  }
}

TEST_F(BasicAuthTest, OnConfigureNoRulesAuth) {
  // enable global auth
  {
    std::string configuration = R"(
{
  "consumers" : [
    {"credential" : "getuser1:123456", "name" : "consumer1"}
  ]
})";
    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
    cred_ = "admin:admin";
    authorization_header_ =
        "Basic " + Base64::encode(cred_.data(), cred_.size());
    EXPECT_CALL(*mock_context_, sendLocalResponse(401, testing::_, testing::_,
                                                  testing::_, testing::_));
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::StopIteration);
    cred_ = "getuser1:123456";
    authorization_header_ =
        "Basic " + Base64::encode(cred_.data(), cred_.size());
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::Continue);
  }
  // disable global auth
  {
    std::string configuration = R"(
{
  "consumers" : [
    {"credential" : "getuser1:123456", "name" : "consumer1"}
  ],
  "global_auth": false
})";
    BufferBase buffer;
    buffer.set({configuration.data(), configuration.size()});

    EXPECT_CALL(*mock_context_, getBuffer(WasmBufferType::PluginConfiguration))
        .WillOnce([&buffer](WasmBufferType) { return &buffer; });
    EXPECT_TRUE(root_context_->configure(configuration.size()));
    cred_ = "admin:admin";
    authorization_header_ =
        "Basic " + Base64::encode(cred_.data(), cred_.size());
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::Continue);
    cred_ = "getuser1:123456";
    authorization_header_ =
        "Basic " + Base64::encode(cred_.data(), cred_.size());
    EXPECT_EQ(context_->onRequestHeaders(0, false),
              FilterHeadersStatus::Continue);
  }
}

}  // namespace basic_auth
}  // namespace null_plugin
}  // namespace proxy_wasm
