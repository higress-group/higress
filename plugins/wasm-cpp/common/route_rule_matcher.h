#pragma once

#include <cstddef>
#include <functional>
#include <memory>
#include <optional>
#include <string>
#include <unordered_map>
#include <unordered_set>
#include <utility>
#include <vector>

#include "absl/strings/match.h"
#include "absl/strings/str_cat.h"
#include "absl/strings/str_format.h"
#include "absl/strings/ascii.h"
#include "absl/strings/str_split.h"
#include "common/json_util.h"
#include "http_util.h"
#include "regex.h"

#ifndef NULL_PLUGIN

#include "proxy_wasm_intrinsics.h"

#else

#include "include/proxy-wasm/null_plugin.h"
using namespace proxy_wasm::null_plugin;
using proxy_wasm::FilterDataStatus;
using proxy_wasm::FilterHeadersStatus;

#endif

#define GET_HEADER_VIEW(key, name)         \
  auto name##_ptr = getRequestHeader(key); \
  auto name = name##_ptr->view();

#define GET_RESPONSE_HEADER_VIEW(key, name) \
  auto name##_ptr = getResponseHeader(key); \
  auto name = name##_ptr->view();

using ::nlohmann::json;
using ::Wasm::Common::JsonArrayIterate;
using ::Wasm::Common::JsonGetField;
using ::Wasm::Common::JsonObjectIterate;
using ::Wasm::Common::JsonValueAs;
using ReMatcher = Wasm::Common::Regex::CompiledGoogleReMatcher;
using ReMatcherSharedPtr = std::shared_ptr<ReMatcher>;

const std::string ConsumerHeader = "X-Mse-Consumer";
const std::string ConsumerGroupHeader = "X-Mse-Consumer-Group";

struct RuleConfig {
  enum RuleType {
    UNKNOWN,
    REQUEST_HEADER,
    ROUTE_NAME,
    OR_RULES,
    AND_RULES,
    NOT_RULE,
    ANY,
    CONSUMER,
    CONSUMER_GROUP,
  };

  struct Matcher {
    enum MatchType {
      EXACT,
      PREFIX,
      REGEX,
      UNKNOWN,
    };

    MatchType match_type{UNKNOWN};
    std::string matcher;
    ReMatcherSharedPtr regex_matcher{};

    bool parse(const json& config) {
      auto exact_match_it = config.find("exact_match");
      auto prefix_match_it = config.find("prefix_match");
      auto safe_regex_match_it = config.find("safe_regex_match");
      if (exact_match_it != config.end()) {
        auto exact_match_value =
            JsonValueAs<std::string>(exact_match_it.value());
        if (exact_match_value.second !=
                Wasm::Common::JsonParserResultDetail::OK ||
            !exact_match_value.first) {
          LOG_ERROR("failed to parse 'exact_match' field in configuration.\n" +
                    config.dump());
          return false;
        }
        matcher = exact_match_value.first.value();
        match_type = EXACT;
        return true;
      } else if (prefix_match_it != config.end()) {
        auto prefix_match_value =
            JsonValueAs<std::string>(prefix_match_it.value());
        if (prefix_match_value.second !=
                Wasm::Common::JsonParserResultDetail::OK ||
            !prefix_match_value.first) {
          LOG_ERROR("failed to parse 'prefix_match' field in configuration.\n" +
                    config.dump());
          return false;
        }
        matcher = prefix_match_value.first.value();
        match_type = PREFIX;
        return true;
      } else if (safe_regex_match_it != config.end()) {
        auto regex_it = safe_regex_match_it->find("regex");
        if (regex_it != safe_regex_match_it->end()) {
          auto regex_value = JsonValueAs<std::string>(regex_it.value());
          if (regex_value.second != Wasm::Common::JsonParserResultDetail::OK ||
              !regex_value.first) {
            LOG_ERROR("failed to parse 'regex' field in configuration.\n" +
                      config.dump());
            return false;
          }
          auto regex = regex_value.first.value();
          regex_matcher = std::make_shared<ReMatcher>(regex, false);
          match_type = REGEX;
          return true;
        }
      }
      LOG_ERROR("failed to parse matcher configuration.\n" + config.dump());
      return false;
    }

    bool match(const std::string& value) const {
      switch (match_type) {
        case EXACT:
          return matcher == value;
        case PREFIX:
          return value.find(matcher) == 0;
        case REGEX:
          return regex_matcher && regex_matcher->match(value);
        default:
          return false;
      }
    }
  };

  struct Action {
    std::unordered_map<std::string, std::string> add_headers;

    bool parse(const json& config) {
      auto add_header_it = config.find("add_header");
      if (add_header_it != config.end()) {
        for (auto& it : add_header_it->items()) {
          std::string header_name = it.key();
          auto header_value = JsonValueAs<std::string>(it.value());
          if (header_value.second != Wasm::Common::JsonParserResultDetail::OK ||
              !header_value.first) {
            LOG_ERROR("failed to parse 'extra_action' configuration.\n" +
                      config.dump());
            return false;
          }
          add_headers.emplace(header_name, header_value.first.value());
        }
      }
      return true;
    }

    void execute() const {
      for (auto& it : add_headers) {
        replaceRequestHeader(it.first, it.second);
      }
    }
  };

  RuleType rule_type{UNKNOWN};
  Matcher matcher;
  std::string header_name;
  std::string consumer;
  std::string consumer_group;
  std::vector<std::shared_ptr<RuleConfig>> and_rules;
  std::vector<std::shared_ptr<RuleConfig>> or_rules;
  std::shared_ptr<RuleConfig> not_rule{};
  Action extra_action;
  bool any = false;

  bool check() const {
    bool check_result = false;
    if (rule_type == ANY) {
      check_result = any;
    } else if (rule_type == OR_RULES) {
      for (const auto& rule : or_rules) {
        if (rule->check()) {
          check_result = true;
          break;
        }
      }
    } else if (rule_type == AND_RULES) {
      check_result = true;
      for (const auto& rule : and_rules) {
        if (!rule->check()) {
          check_result = false;
          break;
        }
      }
    } else if (rule_type == NOT_RULE) {
      check_result = !not_rule->check();
    } else if (rule_type == REQUEST_HEADER) {
      auto header_ptr = getRequestHeader(header_name);
      check_result =
          header_ptr->size() > 0 && matcher.match(header_ptr->toString());
    } else if (rule_type == ROUTE_NAME) {
      std::string route_name;
      getValue({"route_name"}, &route_name);
      check_result = matcher.match(route_name);
    } else if (rule_type == CONSUMER) {
      auto header_ptr = getRequestHeader(ConsumerHeader);
      check_result =
          header_ptr->size() > 0 && consumer == header_ptr->toString();
    } else if (rule_type == CONSUMER_GROUP) {
      // consumer_group match (not substring): missing/empty header -> no match;
      // split on ',', TrimSpace each segment; any segment equals config -> match.
      auto header_ptr = getRequestHeader(ConsumerGroupHeader);
      if (header_ptr->size() == 0) {
        check_result = false;
      } else {
        check_result = false;
        for (absl::string_view part :
             absl::StrSplit(header_ptr->view(), ',')) {
          std::string token = std::string(absl::StripAsciiWhitespace(part));
          if (!token.empty() && token == consumer_group) {
            check_result = true;
            break;
          }
        }
      }
    }
    if (check_result) {
      extra_action.execute();
    }
    return check_result;
  }

  bool parse(const json& config) {
    auto any_it = config.find("any");
    auto or_rules_it = config.find("or_rules");
    auto and_rules_it = config.find("and_rules");
    auto not_rule_it = config.find("not_rule");
    auto header_it = config.find("header");
    auto route_it = config.find("route_name");
    auto consumer_it = config.find("consumer");
    auto consumer_group_it = config.find("consumer_group");
    auto extra_action_it = config.find("extra_action");
    if (extra_action_it != config.end()) {
      if (!extra_action.parse(extra_action_it.value())) {
        return false;
      }
    }
    if (any_it != config.end()) {
      auto any_value = JsonValueAs<bool>(any_it.value());
      if (any_value.second != Wasm::Common::JsonParserResultDetail::OK ||
          !any_value.first) {
        LOG_ERROR("failed to parse 'any' field in configuration.\n" +
                  config.dump());
        return false;
      }
      any = any_value.first.value();
      rule_type = ANY;
      return true;
    } else if (or_rules_it != config.end()) {
      if (!JsonArrayIterate(config, "or_rules", [&](const json& rule) -> bool {
            RuleConfig permission;
            if (!permission.parse(rule)) {
              return false;
            }
            or_rules.push_back(
                std::make_shared<RuleConfig>(std::move(permission)));
            return true;
          })) {
        LOG_ERROR("failed to parse or_rules configuration\n" + config.dump());
        return false;
      }
      rule_type = OR_RULES;
      return true;
    } else if (and_rules_it != config.end()) {
      if (!JsonArrayIterate(config, "and_rules", [&](const json& rule) -> bool {
            RuleConfig permission;
            if (!permission.parse(rule)) {
              return false;
            }
            and_rules.push_back(
                std::make_shared<RuleConfig>(std::move(permission)));
            return true;
          })) {
        LOG_ERROR("failed to parse and_rules configuration\n" + config.dump());
        return false;
      }
      rule_type = AND_RULES;
      return true;
    } else if (not_rule_it != config.end()) {
      RuleConfig permission;
      if (permission.parse(*not_rule_it)) {
        rule_type = NOT_RULE;
        not_rule = std::make_shared<RuleConfig>(std::move(permission));
        return true;
      } else {
        LOG_ERROR("failed to parse not_rule configuration\n" + config.dump());
        return false;
      }
    } else if (header_it != config.end()) {
      const auto& sub_config = header_it.value();
      auto header_name_it = sub_config.find("name");
      if (header_name_it == sub_config.end()) {
        LOG_ERROR("failed to parse 'header.name' configuration.\n" +
                  sub_config.dump());
        return false;
      }
      auto header_name_value = JsonValueAs<std::string>(header_name_it.value());
      if (header_name_value.second !=
              Wasm::Common::JsonParserResultDetail::OK ||
          !header_name_value.first) {
        LOG_ERROR("failed to parse 'header.name' configuration.\n" +
                  sub_config.dump());
        return false;
      }
      header_name = header_name_value.first.value();
      if (!matcher.parse(sub_config)) {
        LOG_ERROR("failed to parse 'header.matcher' configuration.\n" +
                  sub_config.dump());
        return false;
      }
      rule_type = REQUEST_HEADER;
      return true;
    } else if (route_it != config.end()) {
      if (!matcher.parse(route_it.value())) {
        LOG_ERROR("failed to parse 'route' configuration.\n" + config.dump());
        return false;
      }
      rule_type = ROUTE_NAME;
      return true;
    } else if (consumer_it != config.end()) {
      auto consumer_value = JsonValueAs<std::string>(consumer_it.value());
      if (consumer_value.second != Wasm::Common::JsonParserResultDetail::OK ||
          !consumer_value.first) {
        LOG_ERROR("failed to parse 'consumer' configuration.\n" +
                  config.dump());
        return false;
      }
      consumer = consumer_value.first.value();
      rule_type = CONSUMER;
      return true;
    } else if (consumer_group_it != config.end()) {
      auto consumer_group_value =
          JsonValueAs<std::string>(consumer_group_it.value());
      if (consumer_group_value.second !=
              Wasm::Common::JsonParserResultDetail::OK ||
          !consumer_group_value.first) {
        LOG_ERROR("failed to parse 'consumer_group' configuration.\n" +
                  config.dump());
        return false;
      }
      consumer_group = std::string(
          absl::StripAsciiWhitespace(consumer_group_value.first.value()));
      rule_type = CONSUMER_GROUP;
      return true;
    } else {
      LOG_ERROR("unknown permission type\n" + config.dump());
      return false;
    }
  }
};

struct RbacRule {
  std::vector<RuleConfig> principals;
  std::vector<RuleConfig> permissions;

  bool check() const {
    for (const auto& rule : principals) {
      if (!rule.check()) {
        LOG_DEBUG("principal check denied.");
        return false;
      }
    }
    for (const auto& rule : permissions) {
      if (!rule.check()) {
        LOG_DEBUG("permission check denied.");
        return false;
      }
    }
    return true;
  }

  bool parse(const json& config) {
    auto principals_it = config.find("principals");
    auto permissions_it = config.find("permissions");
    if (principals_it == config.end() || permissions_it == config.end()) {
      LOG_ERROR("no principals or no permissions in rbac rules");
      return false;
    }
    if (!JsonArrayIterate(config, "principals",
                          [&](const json& principal_rule) -> bool {
                            RuleConfig principal;
                            if (!principal.parse(principal_rule)) {
                              LOG_ERROR(
                                  "failed to parse 'principals' field in "
                                  "filter configuration.");
                              return false;
                            } else {
                              principals.push_back(std::move(principal));
                              return true;
                            }
                          })) {
      LOG_ERROR("failed to parse configuration for permissions");
      return false;
    }
    if (!JsonArrayIterate(config, "permissions",
                          [&](const json& permission_rule) -> bool {
                            RuleConfig permission;
                            if (!permission.parse(permission_rule)) {
                              LOG_ERROR(
                                  "failed to parse 'permissions' field in "
                                  "filter configuration.");
                              return false;
                            } else {
                              permissions.push_back(std::move(permission));
                              return true;
                            }
                          })) {
      LOG_ERROR("failed to parse configuration for permissions");
      return false;
    }
    return true;
  }
};

template <typename PluginConfig>
class RouteRuleMatcher {
 public:
  enum CATEGORY { Route, RoutePrefix, Host, Service, RouteAndService };
  enum MATCH_TYPE { Prefix, Exact, Suffix };
  struct RuleConfig {
    CATEGORY category;
    std::unordered_set<std::string> routes;
    std::vector<std::string> route_prefixs;
    std::vector<std::pair<MATCH_TYPE, std::string>> hosts;
    std::unordered_set<std::string> services;
    bool disable = false;
    PluginConfig config;
  };
  struct AuthRuleConfig {
    RuleConfig rule_config;
    std::unordered_set<std::string> allow_set;
    bool has_local_config = false;
  };
  RouteRuleMatcher() = default;
  virtual ~RouteRuleMatcher() = default;

  void setInvalidConfig() { invalid_config_ = true; }

  std::vector<std::pair<int, std::reference_wrapper<PluginConfig>>> getRules() {
    std::vector<std::pair<int, std::reference_wrapper<PluginConfig>>> rules;
    rules.reserve(rule_config_.size() + 1);
    if (global_config_) {
      rules.emplace_back(0, global_config_.value());
    }
    for (int i = 0; i < rule_config_.size(); i++) {
      rules.emplace_back(i + 1, rule_config_[i].config);
    }
    return rules;
  }

  bool globalAuthDisable() { return global_auth_ && !global_auth_.value(); }

  FilterHeadersStatus onHeaders(
      const std::function<FilterHeadersStatus(const PluginConfig&)> process) {
    if (invalid_config_) {
      return FilterHeadersStatus::Continue;
    }
    auto config = getMatchConfig();
    if (!config.second) {
      return FilterHeadersStatus::Continue;
    }
    return process(config.second.value());
  }

  bool checkRule(const std::function<bool(const PluginConfig&)> checkPlugin) {
    if (invalid_config_) {
      return true;
    }
    auto config = getMatchConfig();
    if (!config.second) {
      // No config need to check
      return true;
    }
    return checkPlugin(config.second.value());
  }

  bool checkAuthRule(const std::function<bool(
                         const PluginConfig&,
                         const std::optional<std::unordered_set<std::string>>&)>
                         checkPlugin) {
    if (invalid_config_) {
      return true;
    }
    auto config = getMatchAuthConfig();
    if (!config.first) {
      // No config need to check
      LOG_DEBUG("no match config");
      return true;
    }
    if (!config.second && globalAuthDisable()) {
      // No allow set, means no need to check auth if global auth is disable
      LOG_DEBUG(
          "no allow set found, and global auth is disable, no need to auth");
      return true;
    }
    return checkPlugin(config.first.value(), config.second);
  }

  bool checkRuleWithId(
      const std::function<bool(int, const PluginConfig&)> checkPlugin) {
    if (invalid_config_) {
      return true;
    }
    auto config = getMatchConfig();
    if (!config.second) {
      // No config need to check
      return true;
    }
    return checkPlugin(config.first, config.second.value());
  }

  std::pair<int, std::optional<std::reference_wrapper<PluginConfig>>>
  getMatchConfig() {
    auto request_host_header = getRequestHeader(":authority");
    auto request_host = request_host_header->view();
    std::string route_name;
    getValue({"route_name"}, &route_name);
    std::string service_name;
    getValue({"cluster_name"}, &service_name);
    std::optional<std::reference_wrapper<PluginConfig>> match_config;
    int rule_id;
    if (global_config_) {
      rule_id = 0;
      match_config = global_config_.value();
    }
    bool disable_rule = false;
    for (int i = 0; i < rule_config_.size(); ++i) {
      auto& rule = rule_config_[i];
      if (rule.category == CATEGORY::Host) {
        if (hostMatch(rule, request_host)) {
          rule_id = i + 1;
          match_config = rule.config;
          disable_rule = rule.disable;
          break;
        }
      } else if (rule.category == CATEGORY::Route) {
        // category == Route
        if (rule.routes.find(route_name) != rule.routes.end()) {
          rule_id = i + 1;
          match_config = rule.config;
          disable_rule = rule.disable;
          break;
        }
      } else if (rule.category == CATEGORY::RouteAndService) {
        // category == RouteAndService
        if (rule.routes.find(route_name) != rule.routes.end()) {
          if (serviceMatch(rule, service_name)) {
            rule_id = i + 1;
            match_config = rule.config;
            disable_rule = rule.disable;
            break;
          }
        }
      } else if (rule.category == CATEGORY::Service) {
        // category == Service
        if (serviceMatch(rule, service_name)) {
          rule_id = i + 1;
          match_config = rule.config;
          disable_rule = rule.disable;
          break;
        }
      } else {
        // category == RoutePrefix
        bool is_matched = false;
        for (auto& route_prefix : rule.route_prefixs) {
          if (route_name.length() < route_prefix.length() ||
              route_name.compare(0, route_prefix.length(), route_prefix) != 0) {
            continue;
          }
          is_matched = true;
          rule_id = i + 1;
          match_config = rule.config;
          disable_rule = rule.disable;
          break;
        }
        if (is_matched) {
          break;
        }
      }
    }
    if (disable_rule) {
      return std::make_pair(-1, std::nullopt);
    }
    if (match_config) {
      return std::make_pair(rule_id, match_config);
    }
    return std::make_pair(-1, match_config);
  }

  std::pair<
      std::optional<std::reference_wrapper<PluginConfig>>,
      std::optional<std::reference_wrapper<std::unordered_set<std::string>>>>
  getMatchAuthConfig() {
    auto request_host_header = getRequestHeader(":authority");
    auto request_host = request_host_header->view();
    std::string route_name;
    getValue({"route_name"}, &route_name);
    std::string service_name;
    getValue({"cluster_name"}, &service_name);
    std::optional<std::reference_wrapper<PluginConfig>> match_config;
    std::optional<std::reference_wrapper<std::unordered_set<std::string>>>
        allow_set;
    if (global_config_) {
      match_config = global_config_.value();
    }
    if (auth_rule_config_.empty()) {
      return std::make_pair(match_config, std::nullopt);
    }
    bool is_matched = false;
    bool disable_rule = false;
    for (auto& auth_rule : auth_rule_config_) {
      if (auth_rule.rule_config.category == CATEGORY::Host) {
        if (hostMatch(auth_rule.rule_config, request_host)) {
          LOG_DEBUG(absl::StrFormat("host %s is matched for this request",
                                    request_host));
          is_matched = true;
          if (auth_rule.rule_config.disable) {
            disable_rule = true;
          } else if (auth_rule.has_local_config) {
            match_config = auth_rule.rule_config.config;
          } else {
            allow_set = auth_rule.allow_set;
          }
          break;
        }
      } else if (auth_rule.rule_config.category == CATEGORY::Route) {
        // category == Route
        if (auth_rule.rule_config.routes.find(route_name) !=
            auth_rule.rule_config.routes.end()) {
          LOG_DEBUG(absl::StrFormat("route %s is matched for this request",
                                    route_name));
          is_matched = true;
          if (auth_rule.rule_config.disable) {
            disable_rule = true;
          } else if (auth_rule.has_local_config) {
            match_config = auth_rule.rule_config.config;
          } else {
            allow_set = auth_rule.allow_set;
          }
          break;
        }
      } else if (auth_rule.rule_config.category == CATEGORY::RouteAndService) {
        // category == RouteAndService
        if (auth_rule.rule_config.routes.find(route_name) !=
            auth_rule.rule_config.routes.end()) {
          LOG_DEBUG(absl::StrFormat("route %s is matched for this request",
                                    route_name));
          if (serviceMatch(auth_rule.rule_config, service_name)) {
            LOG_DEBUG(absl::StrFormat("service %s is matched for this request",
                                      service_name));
            is_matched = true;
            if (auth_rule.rule_config.disable) {
              disable_rule = true;
            } else if (auth_rule.has_local_config) {
              match_config = auth_rule.rule_config.config;
            } else {
              allow_set = auth_rule.allow_set;
            }
            break;
          }
        }
      } else if (auth_rule.rule_config.category == CATEGORY::Service) {
        // category == Service
        if (serviceMatch(auth_rule.rule_config, service_name)) {
          LOG_DEBUG(absl::StrFormat("service %s is matched for this request",
                                    service_name));
          is_matched = true;
          if (auth_rule.rule_config.disable) {
            disable_rule = true;
          } else if (auth_rule.has_local_config) {
            match_config = auth_rule.rule_config.config;
          } else {
            allow_set = auth_rule.allow_set;
          }
          break;
        }
      } else {
        // category == RoutePrefix
        for (auto& route_prefix : auth_rule.rule_config.route_prefixs) {
          if (route_name.length() < route_prefix.length() ||
              route_name.compare(0, route_prefix.length(), route_prefix) != 0) {
            continue;
          }
          LOG_DEBUG(absl::StrFormat(
              "route_prefix %s is matched for this request", route_prefix));
          is_matched = true;
          if (auth_rule.rule_config.disable) {
            disable_rule = true;
          } else if (auth_rule.has_local_config) {
            match_config = auth_rule.rule_config.config;
          } else {
            allow_set = auth_rule.allow_set;
          }
          break;
        }
        if (is_matched) {
          break;
        }
      }
    }
    return !disable_rule &&
                   (is_matched || (global_auth_ && global_auth_.value()))
               ? std::make_pair(match_config, allow_set)
               : std::make_pair(std::nullopt, std::nullopt);
  }

  void setEmptyGlobalConfig() { global_config_ = PluginConfig{}; }

  bool parseRuleConfig(const json& config) {
    bool has_rules = false;
    int32_t key_count = config.size();
    auto it = config.find("_rules_");
    if (it != config.end()) {
      has_rules = true;
      key_count--;
    }
    PluginConfig plugin_config;
    // has other config fields
    if (key_count > 0 && parsePluginConfig(config, plugin_config)) {
      global_config_ = std::move(plugin_config);
    }
    if (!has_rules) {
      return global_config_ ? true : false;
    }
    auto rules = it.value();
    if (!rules.is_array()) {
      LOG_WARN("'_rules_' field is not an array");
      return false;
    }
    for (const auto& item : rules.items()) {
      RuleConfig rule;
      auto config = item.value();
      if (!parsePluginConfig(config, rule.config)) {
        LOG_WARN("parse rule's config failed");
        return false;
      }
      if (!parseRouteMatchConfig(config, rule.routes)) {
        LOG_WARN("failed to parse configuration for _match_route_");
        return false;
      }
      if (!parseRoutePrefixMatchConfig(config, rule.route_prefixs)) {
        LOG_WARN("failed to parse configuration for _match_route_prefix_");
        return false;
      }
      if (!parseDomainMatchConfig(config, rule.hosts)) {
        LOG_WARN("failed to parse configuration for _match_domain_");
        return false;
      }
      if (!parseServiceMatchConfig(config, rule.services)) {
        LOG_WARN("failed to parse configuration for _match_service_");
        return false;
      }
      auto has_route = !rule.routes.empty();
      auto has_route_prefix = !rule.route_prefixs.empty();
      auto has_service = !rule.services.empty();
      auto has_host = !rule.hosts.empty();
      if (has_route + has_route_prefix + has_host + has_service == 0) {
        LOG_WARN(
            "there is at least one of  '_match_route_', '_match_domain_', "
            "'_match_route_prefix_' and '_match_service_' can "
            "present in configuration.");
        return false;
      }
      if (has_route) {
        rule.category = CATEGORY::Route;
        if (has_service) {
          rule.category = CATEGORY::RouteAndService;
        }
      } else if (has_route_prefix) {
        rule.category = CATEGORY::RoutePrefix;
      } else if (has_service) {
        rule.category = CATEGORY::Service;
      } else {
        rule.category = CATEGORY::Host;
      }
      auto has_disable = config.find("_disable_");
      if (has_disable != config.end()) {
        auto disable = JsonValueAs<bool>(has_disable.value());
        if (disable.second == Wasm::Common::JsonParserResultDetail::OK) {
          rule.disable = disable.first.value();
        }
      }
      rule_config_.push_back(std::move(rule));
    }
    return true;
  }

  bool parseAuthRuleConfig(const json& config) {
    bool has_rules = false;
    int32_t key_count = config.size();
    auto it = config.find("_rules_");
    if (it != config.end()) {
      has_rules = true;
      key_count--;
    }
    auto auth_it = config.find("global_auth");
    if (auth_it != config.end()) {
      auto global_auth_value = JsonValueAs<bool>(auth_it.value());
      if (global_auth_value.second !=
              Wasm::Common::JsonParserResultDetail::OK ||
          !global_auth_value.first) {
        LOG_WARN(
            "failed to parse 'global_auth' field in filter configuration.");
        return false;
      }
      global_auth_ = global_auth_value.first.value();
    }
    PluginConfig plugin_config;
    // has other config fields
    if (key_count > 0 && parsePluginConfig(config, plugin_config)) {
      global_config_ = std::move(plugin_config);
    }
    if (!has_rules) {
      return global_config_ ? true : false;
    }
    auto rules = it.value();
    if (!rules.is_array()) {
      LOG_WARN("'_rules_' field is not an array");
      return false;
    }
    for (const auto& item : rules.items()) {
      AuthRuleConfig auth_rule;
      auto config = item.value();
      // ignore the '_match_route_' or '_match_domain_' field
      auto local_config_size = config.size() - 1;
      auto has_allow = config.find("allow");
      auto has_enable_auth = config.find("enable_auth");
      if (has_allow != config.end() || has_enable_auth != config.end()) {
        local_config_size -= 1;
        LOG_DEBUG("has allow or enable_auth field");
        if (!JsonArrayIterate(config, "allow", [&](const json& allow) -> bool {
              auto parse_result = JsonValueAs<std::string>(allow);
              if (parse_result.second !=
                      Wasm::Common::JsonParserResultDetail::OK ||
                  !parse_result.first) {
                LOG_WARN(
                    "failed to parse 'allow' field in filter "
                    "configuration.");
                return false;
              }
              auth_rule.allow_set.insert(parse_result.first.value());
              return true;
            })) {
          LOG_WARN("failed to parse configuration for allow");
          return false;
        }
      }
      auto has_disable = config.find("_disable_");
      if (has_disable != config.end()) {
        local_config_size -= 1;
        auto disable = JsonValueAs<bool>(has_disable.value());
        if (disable.second == Wasm::Common::JsonParserResultDetail::OK) {
          auth_rule.rule_config.disable = disable.first.value();
        }
      }
      if (local_config_size > 0) {
        if (!parsePluginConfig(config, auth_rule.rule_config.config)) {
          if (has_allow == config.end()) {
            LOG_WARN("parse rule's config failed");
            return false;
          }
        } else {
          auth_rule.has_local_config = true;
        }
      }
      if (!parseRouteMatchConfig(config, auth_rule.rule_config.routes)) {
        LOG_WARN("failed to parse configuration for _match_route_");
        return false;
      }
      if (!parseRoutePrefixMatchConfig(config,
                                       auth_rule.rule_config.route_prefixs)) {
        LOG_WARN("failed to parse configuration for _match_route_prefix_");
        return false;
      }
      if (!parseServiceMatchConfig(config, auth_rule.rule_config.services)) {
        LOG_WARN("failed to parse configuration for _match_service_");
        return false;
      }
      if (!parseDomainMatchConfig(config, auth_rule.rule_config.hosts)) {
        LOG_WARN("failed to parse configuration for _match_domain_");
        return false;
      }
      auto has_route = !auth_rule.rule_config.routes.empty();
      auto has_route_prefix = !auth_rule.rule_config.route_prefixs.empty();
      auto has_host = !auth_rule.rule_config.hosts.empty();
      auto has_service = !auth_rule.rule_config.services.empty();
      if (has_route + has_route_prefix + has_host + has_service == 0) {
        LOG_WARN(
            "there is at least one of  '_match_route_', '_match_domain_', "
            "'_match_route_prefix_' and '_match_service_' can "
            "present in configuration.");
        return false;
      }
      if (has_route) {
        auth_rule.rule_config.category = CATEGORY::Route;
        if (has_service) {
          auth_rule.rule_config.category = CATEGORY::RouteAndService;
        }
      } else if (has_route_prefix) {
        auth_rule.rule_config.category = CATEGORY::RoutePrefix;
      } else if (has_service) {
        auth_rule.rule_config.category = CATEGORY::Service;
      } else {
        auth_rule.rule_config.category = CATEGORY::Host;
      }
      auth_rule_config_.push_back(std::move(auth_rule));
    }
    return true;
  }

 protected:
  virtual bool parsePluginConfig(const json&, PluginConfig&) = 0;

 private:
  bool hostMatch(const RuleConfig& rule, std::string_view request_host) {
    if (rule.hosts.empty()) {
      // If no host specified, consider this rule applies to all host.
      return true;
    }

    request_host = Wasm::Common::Http::stripPortFromHost(request_host);

    for (const auto& host_match : rule.hosts) {
      const auto& host = host_match.second;
      switch (host_match.first) {
        case MATCH_TYPE::Suffix:
          if (absl::EndsWith(
                  absl::string_view(request_host.data(), request_host.size()),
                  absl::string_view(host.data(), host.size()))) {
            return true;
          }
          break;
        case MATCH_TYPE::Prefix:
          if (absl::StartsWith(
                  absl::string_view(request_host.data(), request_host.size()),
                  absl::string_view(host.data(), host.size()))) {
            return true;
          }
          break;
        case MATCH_TYPE::Exact:
          if (request_host == host_match.second) {
            return true;
          }
          break;
        default:
          LOG_WARN(absl::StrCat("unexpected host match pattern"));
          return false;
      }
    }
    return false;
  }

  bool serviceMatch(const RuleConfig& rule, std::string_view request_service) {
    if (rule.services.empty()) {
      // If no services specified, consider this rule applies to all host.
      return true;
    }
    std::vector<std::string> result = absl::StrSplit(request_service, '|');
    if (result.size() != 4) {
      return false;
    }

    std::string port = result[1];
    std::string fqdn = result[3];

    for (const std::string& service_match : rule.services) {
      if (service_match == fqdn || service_match == fqdn + ":" + port) {
        return true;
      }
    }
    return false;
  }

  bool parseRouteMatchConfig(const json& config,
                             std::unordered_set<std::string>& routes) {
    return JsonArrayIterate(
        config, "_match_route_", [&](const json& route) -> bool {
          auto parse_result = JsonValueAs<std::string>(route);
          if (parse_result.second != Wasm::Common::JsonParserResultDetail::OK ||
              !parse_result.first) {
            LOG_WARN(
                "failed to parse '_match_route_' field in filter "
                "configuration.");
            return false;
          }
          routes.insert(parse_result.first.value());
          return true;
        });
  }

  bool parseRoutePrefixMatchConfig(const json& config,
                                   std::vector<std::string>& route_prefixs) {
    return JsonArrayIterate(
        config, "_match_route_prefix_", [&](const json& route) -> bool {
          auto parse_result = JsonValueAs<std::string>(route);
          if (parse_result.second != Wasm::Common::JsonParserResultDetail::OK ||
              !parse_result.first) {
            LOG_WARN(
                "failed to parse '_match_route_prefix_' field in filter "
                "configuration.");
            return false;
          }
          route_prefixs.emplace_back(parse_result.first.value());
          return true;
        });
  }

  bool parseDomainMatchConfig(
      const json& config,
      std::vector<std::pair<MATCH_TYPE, std::string>>& hosts) {
    return JsonArrayIterate(
        config, "_match_domain_", [&](const json& host) -> bool {
          auto parse_result = JsonValueAs<std::string>(host);
          if (parse_result.second != Wasm::Common::JsonParserResultDetail::OK ||
              !parse_result.first) {
            LOG_WARN(
                "failed to parse '_match_domain_' field in filter "
                "configuration.");
            return false;
          }
          auto& host_str = parse_result.first.value();
          std::pair<MATCH_TYPE, std::string> host_match;
          if (absl::StartsWith(host_str, "*")) {
            // suffix match
            host_match.first = MATCH_TYPE::Suffix;
            host_match.second = host_str.substr(1);
            // if (absl::StartsWith(host_match.second, ".")) {
            //   host_match.second = host_match.second.substr(1);
            // }
          } else if (absl::EndsWith(host_str, "*")) {
            // prefix match
            host_match.first = MATCH_TYPE::Prefix;
            host_match.second = host_str.substr(0, host_str.size() - 1);
            // if (absl::EndsWith(host_match.second, ".")) {
            //   host_match.second = host_match.second.substr(
            //       0, host_match.second.size() - 1);
            // }
          } else {
            host_match.first = MATCH_TYPE::Exact;
            host_match.second = host_str;
          }
          hosts.push_back(host_match);
          return true;
        });
  }

  bool parseServiceMatchConfig(const json& config,
                               std::unordered_set<std::string>& services) {
    return JsonArrayIterate(
        config, "_match_service_", [&](const json& service) -> bool {
          auto parse_result = JsonValueAs<std::string>(service);
          if (parse_result.second != Wasm::Common::JsonParserResultDetail::OK ||
              !parse_result.first) {
            LOG_WARN(
                "failed to parse '_match_service_' field in filter "
                "configuration.");
            return false;
          }
          services.insert(parse_result.first.value());
          return true;
        });
  }

  bool invalid_config_ = false;
  std::optional<bool> global_auth_ = std::nullopt;
  std::vector<RuleConfig> rule_config_;
  std::vector<AuthRuleConfig> auth_rule_config_;
  std::optional<PluginConfig> global_config_ = std::nullopt;
};
