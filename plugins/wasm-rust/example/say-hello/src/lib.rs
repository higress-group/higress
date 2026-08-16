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
use higress_wasm_rust::rule_matcher::{on_configure, RuleMatcher, SharedRuleMatcher};
use proxy_wasm::traits::{Context, HttpContext, RootContext};
use proxy_wasm::types::{ContextType, HeaderAction, LogLevel};
use serde::Deserialize;
use std::cell::RefCell;
use std::ops::DerefMut;
use std::rc::Rc;

proxy_wasm::main! {{
    proxy_wasm::set_log_level(LogLevel::Trace);
    proxy_wasm::set_root_context(|_|Box::new(SayHelloRoot::new()));
}}

struct SayHelloRoot {
    log: Log,
    rule_matcher: SharedRuleMatcher<SayHelloConfig>,
}

struct SayHello {
    rule_matcher: SharedRuleMatcher<SayHelloConfig>,
}

#[derive(Default, Debug, Deserialize)]
struct SayHelloConfig {
    name: String,
}

impl SayHelloRoot {
    fn new() -> Self {
        SayHelloRoot {
            log: Log::new("say_hello".to_string()),
            rule_matcher: Rc::new(RefCell::new(RuleMatcher::default())),
        }
    }
}

impl Context for SayHelloRoot {}

impl RootContext for SayHelloRoot {
    fn on_configure(&mut self, _plugin_configuration_size: usize) -> bool {
        on_configure(
            self,
            _plugin_configuration_size,
            self.rule_matcher.borrow_mut().deref_mut(),
            &self.log,
        )
    }

    fn create_http_context(&self, _context_id: u32) -> Option<Box<dyn HttpContext>> {
        Some(Box::new(SayHello {
            rule_matcher: self.rule_matcher.clone(),
        }))
    }

    fn get_type(&self) -> Option<ContextType> {
        Some(ContextType::HttpContext)
    }
}

impl Context for SayHello {}

impl HttpContext for SayHello {
    fn on_http_request_headers(
        &mut self,
        _num_headers: usize,
        _end_of_stream: bool,
    ) -> HeaderAction {
        let binding = self.rule_matcher.borrow();
        let config = match binding.get_match_config() {
            None => {
                self.send_http_response(200, vec![], Some("Hello, World!".as_bytes()));
                return HeaderAction::Continue;
            }
            Some(config) => config.1,
        };

        self.send_http_response(
            200,
            vec![],
            Some(format!("Hello, {}!", config.name).as_bytes()),
        );
        HeaderAction::Continue
    }
}
