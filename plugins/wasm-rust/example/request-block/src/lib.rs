// Copyright (c) 2023 Alibaba Group Holding Ltd.
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

use higress_wasm_rust::log::Log;
use higress_wasm_rust::plugin_wrapper::{HttpContextWrapper, RootContextWrapper};
use higress_wasm_rust::rule_matcher::{on_configure, RuleMatcher, SharedRuleMatcher};
use multimap::MultiMap;
use proxy_wasm::traits::{Context, HttpContext, RootContext};
use proxy_wasm::types::{Bytes, ContextType, DataAction, HeaderAction, LogLevel};
use regex::Regex;
use serde::de::Error;
use serde::Deserialize;
use serde::Deserializer;
use serde_json::Value;
use std::cell::RefCell;
use std::ops::DerefMut;
use std::rc::Rc;

proxy_wasm::main! {{
    proxy_wasm::set_log_level(LogLevel::Trace);
    proxy_wasm::set_root_context(|_|Box::new(RquestBlockRoot::new()));
}}

const PLUGIN_NAME: &str = "request-block";

struct RquestBlockRoot {
    log: Log,
    rule_matcher: SharedRuleMatcher<RquestBlockConfig>,
}

struct RquestBlock {
    log: Log,
    config: Option<Rc<RquestBlockConfig>>,
    cache_request: bool,
}

fn deserialize_block_regexp_urls<'de, D>(deserializer: D) -> Result<Vec<Regex>, D::Error>
where
    D: Deserializer<'de>,
{
    let mut ret = Vec::new();
    let value: Value = Deserialize::deserialize(deserializer)?;
    let block_regexp_urls = value
        .as_array()
        .ok_or(Error::custom("block_regexp_urls error not list"))?;

    for block_regexp_url in block_regexp_urls {
        let reg_exp = block_regexp_url
            .as_str()
            .ok_or(Error::custom("block_regexp_urls error not str"))?;
        if let Ok(reg) = Regex::new(reg_exp) {
            ret.push(reg);
        } else {
            return Err(Error::custom(format!(
                "block_regexp_urls error field {}",
                reg_exp
            )));
        }
    }
    Ok(ret)
}
fn blocked_code_default() -> u32 {
    403
}
fn case_sensitive_default() -> bool {
    true
}
#[derive(Default, Debug, Deserialize, Clone)]
#[serde(default)]
pub struct RquestBlockConfig {
    #[serde(default = "blocked_code_default")]
    blocked_code: u32,
    blocked_message: String,
    #[serde(default = "case_sensitive_default")]
    case_sensitive: bool,
    block_urls: Vec<String>,
    block_exact_urls: Vec<String>,
    block_headers: Vec<String>,
    block_bodies: Vec<String>,
    #[serde(deserialize_with = "deserialize_block_regexp_urls")]
    block_regexp_urls: Vec<Regex>,
}

impl RquestBlockRoot {
    fn new() -> Self {
        RquestBlockRoot {
            log: Log::new(PLUGIN_NAME.to_string()),
            rule_matcher: Rc::new(RefCell::new(RuleMatcher::default())),
        }
    }
}

impl Context for RquestBlockRoot {}

impl RootContext for RquestBlockRoot {
    fn on_configure(&mut self, plugin_configuration_size: usize) -> bool {
        let ret = on_configure(
            self,
            plugin_configuration_size,
            self.rule_matcher.borrow_mut().deref_mut(),
            &self.log,
        );
        ret
    }
    fn create_http_context(&self, context_id: u32) -> Option<Box<dyn HttpContext>> {
        self.create_http_context_use_wrapper(context_id)
    }
    fn get_type(&self) -> Option<ContextType> {
        Some(ContextType::HttpContext)
    }
}

impl RootContextWrapper<RquestBlockConfig> for RquestBlockRoot {
    fn rule_matcher(&self) -> &SharedRuleMatcher<RquestBlockConfig> {
        &self.rule_matcher
    }

    fn create_http_context_wrapper(
        &self,
        _context_id: u32,
    ) -> Option<Box<dyn HttpContextWrapper<RquestBlockConfig>>> {
        Some(Box::new(RquestBlock {
            cache_request: false,
            config: None,
            log: Log::new(PLUGIN_NAME.to_string()),
        }))
    }
}

impl Context for RquestBlock {}
impl HttpContext for RquestBlock {}
impl HttpContextWrapper<RquestBlockConfig> for RquestBlock {
    fn on_config(&mut self, config: Rc<RquestBlockConfig>) {
        self.cache_request = !config.block_bodies.is_empty();
        self.config = Some(config.clone());
    }
    fn cache_request_body(&self) -> bool {
        self.cache_request
    }
    fn on_http_request_complete_headers(
        &mut self,
        headers: &MultiMap<String, String>,
    ) -> HeaderAction {
        if self.config.is_none() {
            return HeaderAction::Continue;
        }
        let config = self.config.as_ref().unwrap();
        if !config.block_urls.is_empty()
            || !config.block_exact_urls.is_empty()
            || !config.block_regexp_urls.is_empty()
        {
            let value = headers.get(":path");

            if value.is_none() {
                self.log.warn("get path failed");
                return HeaderAction::Continue;
            }
            let mut request_url = value.unwrap().clone();

            if !config.case_sensitive {
                request_url = request_url.to_lowercase();
            }
            for block_exact_url in &config.block_exact_urls {
                if *block_exact_url == request_url {
                    self.send_http_response(
                        config.blocked_code,
                        Vec::new(),
                        Some(config.blocked_message.as_bytes()),
                    );
                    return HeaderAction::StopIteration;
                }
            }
            for block_url in &config.block_urls {
                if request_url.contains(block_url) {
                    self.send_http_response(
                        config.blocked_code,
                        Vec::new(),
                        Some(config.blocked_message.as_bytes()),
                    );
                    return HeaderAction::StopIteration;
                }
            }

            for block_reg_exp in &config.block_regexp_urls {
                if block_reg_exp.is_match(&request_url) {
                    self.send_http_response(
                        config.blocked_code,
                        Vec::new(),
                        Some(config.blocked_message.as_bytes()),
                    );
                    return HeaderAction::StopIteration;
                }
            }
        }
        if !config.block_headers.is_empty() {
            let mut header_strs: Vec<String> = Vec::new();
            for (k, v) in headers {
                header_strs.push(k.clone());
                header_strs.push(v.join("\n"));
            }
            let header_str = header_strs.join("\n");
            for block_header in &config.block_headers {
                if header_str.contains(block_header) {
                    self.send_http_response(
                        config.blocked_code,
                        Vec::new(),
                        Some(config.blocked_message.as_bytes()),
                    );
                    return HeaderAction::StopIteration;
                }
            }
        }
        HeaderAction::Continue
    }
    fn on_http_request_complete_body(&mut self, req_body: &Bytes) -> DataAction {
        if self.config.is_none() {
            return DataAction::Continue;
        }
        let config = self.config.as_ref().unwrap();
        if config.block_bodies.is_empty() {
            return DataAction::Continue;
        }
        let mut body = req_body.clone();
        if !config.case_sensitive {
            body = body.to_ascii_lowercase();
        }
        for block_body in &config.block_bodies {
            let s = block_body.as_bytes();
            if body.windows(s.len()).any(|window| window == s) {
                self.send_http_response(
                    config.blocked_code,
                    Vec::new(),
                    Some(config.blocked_message.as_bytes()),
                );
                return DataAction::StopIterationAndBuffer;
            }
        }
        DataAction::Continue
    }
}
