// Copyright (c) 2026 Alibaba Group Holding Ltd.
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

use serde::{de::Error, Deserialize, Deserializer};
use serde_json::Value;
use std::collections::HashSet;

#[derive(Clone, Debug, Default)]
pub(crate) struct ToolArgumentsConfig {
    pub(crate) check: bool,
    pub(crate) restore: bool,
    pub(crate) restore_allowlist: Vec<RestoreTool>,
    pub(crate) limits: ToolLimits,
}

// Candidate defaults from the approved Design; release values require T5 evidence.
#[derive(Clone, Debug)]
pub(crate) struct ToolLimits {
    pub(crate) max_call_bytes: usize,
    pub(crate) max_buffer_bytes: usize,
    pub(crate) max_calls: usize,
}

impl Default for ToolLimits {
    fn default() -> Self {
        Self {
            max_call_bytes: 1_048_576,
            max_buffer_bytes: 4_194_304,
            max_calls: 128,
        }
    }
}

impl<'de> Deserialize<'de> for ToolLimits {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        let value = Value::deserialize(deserializer)?;
        let object = value
            .as_object()
            .ok_or_else(|| D::Error::custom("tool limits must be an object"))?;
        let default = Self::default();
        let read = |name, fallback| match object.get(name) {
            None => Ok(fallback),
            Some(value) => value
                .as_u64()
                .filter(|number| *number > 0)
                .and_then(|number| usize::try_from(number).ok())
                .ok_or_else(|| {
                    D::Error::custom("tool limit must be a positive representable integer")
                }),
        };
        Ok(Self {
            max_call_bytes: read("max_call_bytes", default.max_call_bytes)?,
            max_buffer_bytes: read("max_buffer_bytes", default.max_buffer_bytes)?,
            max_calls: read("max_calls", default.max_calls)?,
        })
    }
}

fn empty_object() -> Value {
    serde_json::json!({})
}

#[derive(Clone, Debug, Deserialize)]
pub(crate) struct RestoreTool {
    pub(crate) tool: String,
    pub(crate) paths: Vec<String>,
}

#[derive(Clone, Debug, Default, Deserialize)]
#[serde(default)]
pub(crate) struct ToolDefinitionsConfig {
    pub(crate) check: bool,
}

/// Detect enabled or malformed new options before choosing the legacy loader.
/// Inactive dependencies (limits and allowlists) do not activate strict loading.
pub(crate) fn requests_tool_processing(value: &Value) -> bool {
    for (group, fields) in [
        ("tool_arguments", &["check", "restore"][..]),
        ("tool_definitions", &["check"][..]),
    ] {
        if let Some(options) = value.get(group) {
            let Some(options) = options.as_object() else {
                return true;
            };
            if fields.iter().any(|field| {
                options
                    .get(*field)
                    .is_some_and(|value| value != &Value::Bool(false))
            }) {
                return true;
            }
        }
    }
    value.get("_rules_").is_some_and(|rules| {
        if let Some(rules) = rules.as_array() {
            rules.iter().any(requests_tool_processing)
        } else {
            requests_tool_processing(rules)
        }
    })
}

pub(crate) fn pointer_tokens(pointer: &str) -> Result<Vec<String>, &'static str> {
    if pointer.is_empty() {
        return Ok(Vec::new());
    }
    if !pointer.starts_with('/') {
        return Err("tool restore path must be a raw JSON Pointer");
    }
    pointer[1..]
        .split('/')
        .map(|part| {
            let mut token = String::new();
            let mut chars = part.chars();
            while let Some(character) = chars.next() {
                if character == '~' {
                    token.push(match chars.next() {
                        Some('0') => '~',
                        Some('1') => '/',
                        _ => return Err("invalid JSON Pointer escape"),
                    });
                } else {
                    token.push(character);
                }
            }
            Ok(token)
        })
        .collect()
}

impl<'de> Deserialize<'de> for ToolArgumentsConfig {
    fn deserialize<D: Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        #[derive(Default, Deserialize)]
        #[serde(default)]
        struct Raw {
            check: bool,
            restore: bool,
            restore_allowlist: Value,
            #[serde(default = "empty_object")]
            limits: Value,
        }
        let raw = Raw::deserialize(deserializer)?;
        let limits = if raw.check || raw.restore {
            serde_json::from_value(raw.limits).map_err(D::Error::custom)?
        } else {
            ToolLimits::default()
        };
        let restore_allowlist = if raw.restore {
            let entries: Vec<RestoreTool> =
                serde_json::from_value(raw.restore_allowlist).map_err(D::Error::custom)?;
            if entries.is_empty() {
                return Err(D::Error::custom(
                    "tool restore requires an explicit allowlist",
                ));
            }
            let mut tools = HashSet::new();
            for entry in &entries {
                if entry.tool.is_empty() || !tools.insert(&entry.tool) {
                    return Err(D::Error::custom("empty or duplicate tool restore name"));
                }
                if entry.paths.is_empty() {
                    return Err(D::Error::custom("tool restore paths cannot be empty"));
                }
                let mut paths = HashSet::new();
                for path in &entry.paths {
                    let tokens = pointer_tokens(path).map_err(D::Error::custom)?;
                    if !paths.insert(tokens) {
                        return Err(D::Error::custom("duplicate tool restore path"));
                    }
                }
            }
            entries
        } else {
            Vec::new()
        };
        Ok(Self {
            check: raw.check,
            restore: raw.restore,
            restore_allowlist,
            limits,
        })
    }
}
