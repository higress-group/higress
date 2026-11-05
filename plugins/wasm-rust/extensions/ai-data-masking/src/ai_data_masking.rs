// Copyright (c) 2025 Alibaba Group Holding Ltd.
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

#[path = "tool_raw.rs"]
mod tool_raw;
#[path = "tool_stream.rs"]
mod tool_stream;

use crate::deny_word::DenyWord;
use crate::msg_win_openai::MsgWindow;
use crate::tool_arguments::{rewrite_json, JsonStrings};
use crate::tool_config::{requests_tool_processing, ToolArgumentsConfig, ToolDefinitionsConfig};
use aho_corasick::{AhoCorasick, AhoCorasickBuilder, AhoCorasickKind, Input, MatchKind};
use fancy_regex::{Expander, Regex};
use grok::patterns;
use higress_wasm_rust::log::Log;
use higress_wasm_rust::plugin_wrapper::{HttpContextWrapper, RootContextWrapper};
use higress_wasm_rust::request_wrapper::has_request_body;
use higress_wasm_rust::rule_matcher::{on_configure, RuleMatcher, SharedRuleMatcher};
use jsonpath_rust::{JsonPath, JsonPathValue};
use lazy_static::lazy_static;
use proxy_wasm::traits::{Context, HttpContext, RootContext};
use proxy_wasm::types::{Bytes, ContextType, DataAction, HeaderAction, LogLevel};
use serde::de::Error;
use serde::Deserialize;
use serde::Deserializer;
use serde_json::{json, Value};
use std::cell::RefCell;
use std::collections::{BTreeMap, HashMap, HashSet, VecDeque};
use std::fmt::Write;
use std::ops::{DerefMut, Range};
use std::rc::{Rc, Weak};
use std::str::FromStr;
use std::vec;

proxy_wasm::main! {{
    proxy_wasm::set_log_level(LogLevel::Trace);
    proxy_wasm::set_root_context(|_|Box::new(AiDataMaskingRoot::new()));
}}

const PLUGIN_NAME: &str = "ai-data-masking";
const GROK_PATTERN: &str = r"%\{(?<name>(?<pattern>[A-z0-9]+)(?::(?<alias>[A-z0-9_:;\/\s\.]+))?)\}";

struct System {
    deny_word: DenyWord,
    grok_regex: Regex,
    grok_patterns: BTreeMap<String, String>,
}
lazy_static! {
    static ref SYSTEM: System = System::new();
}

struct AiDataMaskingRoot {
    log: Log,
    rule_matcher: SharedRuleMatcher<AiDataMaskingConfig>,
    tools_configured: bool,
}
struct AiDataMasking {
    weak: Weak<RefCell<Box<dyn HttpContextWrapper<AiDataMaskingConfig>>>>,
    config: Option<Rc<AiDataMaskingConfig>>,
    mask_map: HashMap<String, RestoreEntry>,
    mask_restore: Option<MaskRestore>,
    tools: Option<ToolState>,
    is_openai: bool,
    is_openai_stream: Option<bool>,
    stream: bool,
    log: Log,
    msg_window: MsgWindow,
    char_window_size: usize,
    byte_window_size: usize,
}
fn deserialize_regexp<'de, D>(deserializer: D) -> Result<Regex, D::Error>
where
    D: Deserializer<'de>,
{
    let value: Value = Deserialize::deserialize(deserializer)?;
    if let Some(pattern) = value.as_str() {
        let (p, _) = SYSTEM.grok_to_pattern(pattern);
        if let Ok(reg) = Regex::new(&p) {
            Ok(reg)
        } else if let Ok(reg) = Regex::new(pattern) {
            Ok(reg)
        } else {
            Err(Error::custom(format!("regexp error field {}", pattern)))
        }
    } else {
        Err(Error::custom("regexp error not string".to_string()))
    }
}

fn deserialize_type<'de, D>(deserializer: D) -> Result<Type, D::Error>
where
    D: Deserializer<'de>,
{
    let value: Value = Deserialize::deserialize(deserializer)?;
    if let Some(t) = value.as_str() {
        if t == "replace" {
            Ok(Type::Replace)
        } else if t == "hash" {
            Ok(Type::Hash)
        } else {
            Err(Error::custom(format!("regexp error value {}", t)))
        }
    } else {
        Err(Error::custom("type error not string".to_string()))
    }
}

fn deserialize_denyword<'de, D>(deserializer: D) -> Result<DenyWord, D::Error>
where
    D: Deserializer<'de>,
{
    let value: Vec<String> = Deserialize::deserialize(deserializer)?;
    Ok(DenyWord::from_iter(value))
}

fn deserialize_jsonpath<'de, D>(deserializer: D) -> Result<Vec<JsonPath>, D::Error>
where
    D: Deserializer<'de>,
{
    let value: Vec<String> = Deserialize::deserialize(deserializer)?;
    let mut ret = Vec::new();
    for v in value {
        if v.is_empty() {
            continue;
        }
        match JsonPath::from_str(&v) {
            Ok(jp) => ret.push(jp),
            Err(_) => return Err(Error::custom(format!("jsonpath error value {}", v))),
        }
    }
    Ok(ret)
}

#[derive(Debug, Clone, PartialEq, Eq)]
enum Type {
    Replace,
    Hash,
}

#[derive(Debug, Deserialize, Clone)]
struct Rule {
    #[serde(deserialize_with = "deserialize_regexp")]
    regex: Regex,
    #[serde(deserialize_with = "deserialize_type", alias = "type")]
    type_: Type,
    #[serde(default)]
    restore: bool,
    #[serde(default)]
    value: String,
}

struct MaskRestore {
    general: Option<GeneralRestore>,
    hashes: HashMap<String, String>,
}

struct GeneralRestore {
    matcher: AhoCorasick,
    replacements: Vec<String>,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum RestoreKind {
    Hash,
    General,
}

#[derive(Clone)]
struct RestoreEntry {
    original: Option<String>,
    kind: RestoreKind,
}

#[derive(Default)]
struct ToolRestore {
    values: Option<MaskRestore>,
    conflicts: Option<AhoCorasick>,
    has_entries: bool,
}

fn tool_response_format(bytes: &[u8], end: bool) -> Option<bool> {
    let bytes = if bytes.starts_with(b"\xef\xbb\xbf") {
        &bytes[3..]
    } else if b"\xef\xbb\xbf".starts_with(bytes) && !end {
        return None;
    } else {
        bytes
    };
    let bytes = bytes
        .iter()
        .position(|byte| !byte.is_ascii_whitespace())
        .map_or(&[][..], |at| &bytes[at..]);
    match bytes.first() {
        Some(b'{' | b'[') => Some(false),
        Some(b':') => Some(true),
        None => end.then_some(true),
        _ if bytes.contains(&b':') || bytes.contains(&b'\n') || bytes.contains(&b'\r') || end => {
            Some(true)
        }
        _ => None,
    }
}

struct ToolState {
    request_stream: bool,
    response_stream: Option<bool>,
    body_started: bool,
    probe: Vec<u8>,
    restore: ToolRestore,
    stream: tool_stream::ToolStream,
    raw_restore: tool_raw::RawRestoreWindow,
    raw_deny: tool_raw::RawDenyWindow,
    failed: bool,
}

impl ToolRestore {
    fn from_maps(
        legacy: &HashMap<String, RestoreEntry>,
        tools: &HashMap<String, RestoreEntry>,
    ) -> Result<Self, &'static str> {
        use std::collections::hash_map::Entry;
        let mut merged = legacy.clone();
        for (masked, incoming) in tools {
            match merged.entry(masked.clone()) {
                Entry::Vacant(entry) => {
                    entry.insert(incoming.clone());
                }
                Entry::Occupied(mut entry) => {
                    if entry.get().original != incoming.original {
                        entry.get_mut().original = None;
                    }
                    if incoming.kind == RestoreKind::Hash {
                        entry.get_mut().kind = RestoreKind::Hash;
                    }
                }
            }
        }
        let has_entries = !merged.is_empty();
        let ambiguous: Vec<_> = merged
            .iter()
            .filter(|(_, entry)| entry.original.is_none())
            .map(|(masked, _)| masked.as_str())
            .collect();
        let conflicts = if ambiguous.is_empty() {
            None
        } else {
            Some(
                AhoCorasickBuilder::new()
                    .kind(Some(AhoCorasickKind::ContiguousNFA))
                    .build(ambiguous)
                    .map_err(|_| "tool conflict matcher failed")?,
            )
        };
        let values = MaskRestore::from_map(merged).map_err(|_| "tool restore matcher failed")?;
        Ok(Self {
            values,
            conflicts,
            has_entries,
        })
    }

    fn restore(&self, input: &str, max_bytes: usize) -> Result<Option<String>, &'static str> {
        if self
            .conflicts
            .as_ref()
            .is_some_and(|matcher| matcher.is_match(input))
        {
            return Err("ambiguous tool restore mapping");
        }
        self.values
            .as_ref()
            .map_or(Ok(None), |values| values.restore_bounded(input, max_bytes))
    }
}

struct PreparedToolRequest {
    body: Option<String>,
    legacy_map: HashMap<String, RestoreEntry>,
    tool_map: HashMap<String, RestoreEntry>,
    is_openai: bool,
    legacy_stream: bool,
    tool_stream: bool,
    byte_window_size: usize,
    char_window_size: usize,
}

fn prepare_tool_request(
    config: &AiDataMaskingConfig,
    body: &[u8],
    log: &Log,
) -> Result<Option<PreparedToolRequest>, &'static str> {
    let has_deny = config.system_deny || !config.deny_words.is_empty();
    let check_arguments =
        config.tool_arguments.check && (has_deny || !config.replace_roles.is_empty());
    let check_definitions = config.tool_definitions.check && has_deny;
    if !check_arguments && !check_definitions && !config.tool_arguments.restore {
        return Ok(None);
    }
    let value: Value = serde_json::from_slice(body).map_err(|_| "invalid tool request JSON")?;
    let messages = value
        .get("messages")
        .and_then(Value::as_array)
        .ok_or("invalid tool request messages")?;
    if messages.iter().any(|message| !message.is_object()) {
        return Err("invalid tool request message");
    }
    let normalized = value.to_string();
    let document = JsonStrings::parse(&normalized)?;
    let mut arguments = Vec::new();
    if check_arguments {
        for (message_index, message) in messages.iter().enumerate() {
            if !message.is_object() {
                return Err("invalid tool request message");
            }
            if let Some(calls) = message.get("tool_calls").filter(|calls| !calls.is_null()) {
                for (call_index, call) in calls
                    .as_array()
                    .ok_or("invalid request tool calls")?
                    .iter()
                    .enumerate()
                {
                    if !call.is_object() {
                        return Err("invalid request tool call");
                    }
                    let path = vec![
                        "messages".to_string(),
                        message_index.to_string(),
                        "tool_calls".to_string(),
                        call_index.to_string(),
                        "function".to_string(),
                        "arguments".to_string(),
                    ];
                    collect_request_argument(
                        &document,
                        call.get("function"),
                        path,
                        &mut arguments,
                    )?;
                }
            }
            let path = vec![
                "messages".to_string(),
                message_index.to_string(),
                "function_call".to_string(),
                "arguments".to_string(),
            ];
            collect_request_argument(
                &document,
                message.get("function_call"),
                path,
                &mut arguments,
            )?;
        }
    }
    let owned: HashSet<usize> = arguments.iter().map(|argument| argument.address).collect();
    let legacy = LegacyRequestPlan::from_config(config, &value, &normalized, &owned);
    for text in legacy.checked_strings(&normalized) {
        if config.check_message(text, log) {
            return Err("legacy request deny matched");
        }
    }
    if check_definitions {
        for field in ["tools", "functions"] {
            if let Some(definitions) = value.get(field).filter(|value| !value.is_null()) {
                if !definitions.is_array() {
                    return Err("invalid tool definitions");
                }
                if deny_definition(definitions, false, config, log) {
                    return Err("tool definition deny matched");
                }
            }
        }
    }
    // Inspect every call before recording any new masks.
    for argument in &arguments {
        rewrite_json(
            &argument.text,
            Some(&[]),
            config.tool_arguments.limits.max_call_bytes,
            usize::MAX,
            &mut |text| has_deny && config.check_message(text, log),
            &mut |_| Ok(None),
        )?;
    }
    let mut scoped = ScopedBody {
        text: normalized.clone(),
        protected: arguments
            .iter()
            .map(|argument| argument.span.clone())
            .collect(),
    };
    scoped.protected.sort_by_key(|span| span.start);
    let mut legacy_mask = RequestMasks::default();
    match &legacy {
        LegacyRequestPlan::Strings { values, .. } => {
            for (text, managed) in values {
                if *managed {
                    continue;
                }
                let masked = legacy_mask.plain(config, text)?;
                if masked != *text {
                    scoped.replace_all(
                        &Value::String(text.clone()).to_string(),
                        &Value::String(masked).to_string(),
                    )?;
                }
            }
        }
        LegacyRequestPlan::Raw => {
            for rule in &config.replace_roles {
                scoped.mask_rule(rule, &mut legacy_mask)?;
            }
        }
        LegacyRequestPlan::None => {}
    }
    let legacy_changed = scoped.text != normalized;
    let mut tool_mask = RequestMasks::default();
    let mut edits = Vec::new();
    if !config.replace_roles.is_empty() {
        // Find by its original span index, since old edits can shift offsets.
        let mut original_spans: Vec<_> = arguments
            .iter()
            .map(|argument| argument.span.clone())
            .collect();
        original_spans.sort_by_key(|span| span.start);
        for argument in &arguments {
            if let Some(rewritten) = rewrite_json(
                &argument.text,
                None,
                config.tool_arguments.limits.max_call_bytes,
                usize::MAX,
                &mut |_| false,
                &mut |text| {
                    let masked = tool_mask.plain_bounded(
                        config,
                        text,
                        config.tool_arguments.limits.max_call_bytes,
                    )?;
                    Ok((masked != text).then_some(masked))
                },
            )? {
                let index = original_spans
                    .binary_search_by_key(&argument.span.start, |span| span.start)
                    .map_err(|_| "missing argument location")?;
                edits.push((
                    scoped.protected[index].clone(),
                    serde_json::to_string(&rewritten).map_err(|_| "arguments encoding failed")?,
                ));
            }
        }
    }
    edits.sort_by_key(|(span, _)| span.start);
    let arguments_changed = !edits.is_empty();
    scoped.apply_edits(edits)?;
    let forward = legacy.always_forwards() || legacy_changed || arguments_changed;
    if forward && (config.tool_arguments.check || config.tool_arguments.restore) {
        serde_json::from_str::<Value>(&scoped.text)
            .map_err(|_| "invalid rewritten tool request")?;
    }
    let needs_stream = config.tool_arguments.restore
        || (config.tool_arguments.check && (has_deny || !legacy_mask.map.is_empty()));
    let tool_stream = match value.get("stream") {
        Some(Value::Bool(stream)) => *stream,
        None => false,
        _ if needs_stream => return Err("invalid tool request stream flag"),
        _ => false,
    };
    let (is_openai, legacy_stream) = legacy.openai_state();
    Ok(Some(PreparedToolRequest {
        body: forward.then_some(scoped.text),
        legacy_map: legacy_mask.map,
        tool_map: tool_mask.map,
        is_openai,
        legacy_stream,
        tool_stream,
        byte_window_size: legacy_mask.byte_window_size,
        char_window_size: legacy_mask.char_window_size,
    }))
}

struct ResponseArgument {
    span: Range<usize>,
    text: String,
    name: String,
}

fn check_response_arguments(config: &AiDataMaskingConfig) -> bool {
    config.tool_arguments.check && (config.system_deny || !config.deny_words.is_empty())
}

fn allowed_restore_paths<'a>(
    config: &'a AiDataMaskingConfig,
    restore: &ToolRestore,
    name: &str,
) -> Option<&'a [String]> {
    if !config.tool_arguments.restore || !restore.has_entries {
        return None;
    }
    config
        .tool_arguments
        .restore_allowlist
        .iter()
        .find(|entry| entry.tool == name)
        .map(|entry| entry.paths.as_slice())
}

fn inspect_response_argument(
    config: &AiDataMaskingConfig,
    restore: &ToolRestore,
    name: &str,
    text: &str,
    available_new_bytes: usize,
    log: &Log,
) -> Result<Option<String>, &'static str> {
    let check = check_response_arguments(config);
    let paths = allowed_restore_paths(config, restore, name);
    if text.is_empty() || (!check && paths.is_none()) {
        return Ok(None);
    }
    let max_bytes = config.tool_arguments.limits.max_call_bytes;
    rewrite_json(
        text,
        Some(paths.unwrap_or(&[])),
        max_bytes,
        available_new_bytes,
        &mut |value| check && config.check_message(value, log),
        &mut |value| restore.restore(value, max_bytes.min(available_new_bytes)),
    )
}

fn add_response_bytes(current: &mut usize, bytes: usize, limit: usize) -> Result<(), &'static str> {
    let total = current
        .checked_add(bytes)
        .ok_or("response tool bytes overflow")?;
    if total > limit {
        return Err("response tool buffer limit exceeded");
    }
    *current = total;
    Ok(())
}

fn prepare_tool_response(
    config: &AiDataMaskingConfig,
    body: &[u8],
    is_openai: bool,
    legacy_restore: Option<&MaskRestore>,
    restore: &ToolRestore,
    log: &Log,
) -> Result<Option<String>, &'static str> {
    let value: Value = serde_json::from_slice(body).map_err(|_| "invalid tool response JSON")?;
    let has_error = value.get("error").is_some_and(|error| !error.is_null());
    let choices = match value.get("choices") {
        Some(Value::Array(choices)) => choices.as_slice(),
        None if has_error => &[],
        _ => return Err("invalid tool response choices"),
    };
    let normalized = value.to_string();
    let document = JsonStrings::parse(&normalized)?;
    let legacy_strings = if config.deny_openai && is_openai {
        serde_json::from_str::<Res>(&normalized)
            .ok()
            .map(|response| {
                response
                    .choices
                    .into_iter()
                    .filter_map(|choice| choice.message)
                    .flat_map(|message| [message.content, message.reasoning_content])
                    .collect::<Vec<_>>()
            })
    } else {
        None
    };
    if let Some(strings) = &legacy_strings {
        if strings.iter().any(|text| config.check_message(text, log)) {
            return Err("legacy response deny matched");
        }
    } else if config.deny_raw && config.check_message(&normalized, log) {
        return Err("legacy raw response deny matched");
    }
    let limits = &config.tool_arguments.limits;
    let mut protected = Vec::new();
    let mut work = Vec::new();
    let mut retained = 0usize;
    let mut keys = HashSet::new();
    let mut ids = HashSet::new();
    {
        let mut collect = |choice: &Value,
                           call: &Value,
                           function: Option<&Value>,
                           path: Vec<String>,
                           ordinal: usize,
                           legacy: bool|
         -> Result<(), &'static str> {
            let Some(function) = function.filter(|value| !value.is_null()) else {
                return Ok(());
            };
            if !function.is_object() {
                return Err("invalid response tool function");
            }
            let Some(arguments) = function.get("arguments") else {
                return Ok(());
            };
            let span = document
                .value_span(&path)?
                .ok_or("invalid response arguments type")?;
            protected.push(span.clone());
            if arguments.is_null() {
                return Ok(());
            }
            if arguments.as_str() == Some("") {
                return Ok(());
            }
            let name = function
                .get("name")
                .and_then(Value::as_str)
                .filter(|name| !name.is_empty());
            let needs_name = check_response_arguments(config)
                || (config.tool_arguments.restore && restore.has_entries);
            if needs_name && name.is_none() {
                return Err("missing response tool name");
            }
            let name = name.unwrap_or("");
            if !check_response_arguments(config)
                && allowed_restore_paths(config, restore, name).is_none()
            {
                return Ok(());
            }
            let text = arguments
                .as_str()
                .ok_or("invalid response arguments type")?;
            let choice_index = choice
                .get("index")
                .and_then(Value::as_i64)
                .filter(|index| *index >= 0)
                .ok_or("invalid response choice index")?;
            let key = (choice_index, legacy, ordinal);
            if keys.contains(&key) {
                return Err("duplicate response tool identity");
            }
            if keys.len() >= limits.max_calls {
                return Err("response tool call limit exceeded");
            }
            keys.insert(key);
            let id = match call.get("id") {
                Some(Value::String(id)) => Some(id.as_str()).filter(|id| !id.is_empty()),
                None | Some(Value::Null) => None,
                _ => return Err("invalid response tool id"),
            };
            if id.is_some_and(|id| ids.contains(id)) {
                return Err("conflicting response tool id");
            }
            if text.len() > limits.max_call_bytes {
                return Err("arguments input limit exceeded");
            }
            let bytes = text
                .len()
                .checked_add(name.len())
                .and_then(|bytes| bytes.checked_add(id.map_or(0, str::len)))
                .ok_or("response tool bytes overflow")?;
            add_response_bytes(&mut retained, bytes, limits.max_buffer_bytes)?;
            if let Some(id) = id {
                ids.insert(id.to_string());
            }
            work.push(ResponseArgument {
                span,
                text: text.to_string(),
                name: name.to_string(),
            });
            Ok(())
        };
        for (choice_position, choice) in choices.iter().enumerate() {
            if !choice.is_object() {
                return Err("invalid response choice");
            }
            let Some(message) = choice.get("message").filter(|message| !message.is_null()) else {
                if choice.get("delta").is_some() {
                    return Err("delta in nonstream tool response");
                }
                continue;
            };
            if !message.is_object() {
                return Err("invalid response message");
            }
            if let Some(calls) = message.get("tool_calls").filter(|calls| !calls.is_null()) {
                for (ordinal, call) in calls
                    .as_array()
                    .ok_or("invalid response tool calls")?
                    .iter()
                    .enumerate()
                {
                    if !call.is_object() {
                        return Err("invalid response tool call");
                    }
                    collect(
                        choice,
                        call,
                        call.get("function"),
                        vec![
                            "choices".to_string(),
                            choice_position.to_string(),
                            "message".to_string(),
                            "tool_calls".to_string(),
                            ordinal.to_string(),
                            "function".to_string(),
                            "arguments".to_string(),
                        ],
                        ordinal,
                        false,
                    )?;
                }
            }
            collect(
                choice,
                &Value::Null,
                message.get("function_call"),
                vec![
                    "choices".to_string(),
                    choice_position.to_string(),
                    "message".to_string(),
                    "function_call".to_string(),
                    "arguments".to_string(),
                ],
                0,
                true,
            )?;
        }
    }
    if has_error && !work.is_empty() {
        return Err("tool arguments on upstream error");
    }
    protected.sort_by_key(|span| span.start);
    let original_spans = protected.clone();
    let mut scoped = ScopedBody {
        text: normalized.clone(),
        protected,
    };
    let mut rewritten = Vec::new();
    for argument in work {
        if let Some(text) = inspect_response_argument(
            config,
            restore,
            &argument.name,
            &argument.text,
            limits.max_buffer_bytes - retained,
            log,
        )? {
            add_response_bytes(&mut retained, text.len(), limits.max_buffer_bytes)?;
            rewritten.push((argument.span.start, text));
        }
    }
    if let Some(restore) = legacy_restore {
        if let Some(strings) = &legacy_strings {
            for text in strings {
                let changed = restore.restore(text);
                if changed != *text {
                    scoped.replace_all(
                        &Value::String(text.clone()).to_string(),
                        &Value::String(changed).to_string(),
                    )?;
                }
            }
        } else if config.deny_raw {
            scoped.restore(restore)?;
        }
    }
    let legacy_changed = scoped.text != normalized;
    let changed_arguments = !rewritten.is_empty();
    let mut edits = Vec::new();
    for (start, text) in rewritten {
        let index = original_spans
            .binary_search_by_key(&start, |span| span.start)
            .map_err(|_| "missing response arguments location")?;
        edits.push((
            scoped.protected[index].clone(),
            serde_json::to_string(&text).map_err(|_| "arguments encoding failed")?,
        ));
    }
    edits.sort_by_key(|(span, _)| span.start);
    scoped.apply_edits(edits)?;
    if legacy_strings.is_some() || config.deny_raw || legacy_changed || changed_arguments {
        serde_json::from_str::<Value>(&scoped.text)
            .map_err(|_| "invalid rewritten tool response")?;
        Ok(Some(scoped.text))
    } else {
        Ok(None)
    }
}

struct RequestArgument {
    address: usize,
    span: Range<usize>,
    text: String,
}

fn collect_request_argument(
    document: &JsonStrings,
    function: Option<&Value>,
    path: Vec<String>,
    arguments: &mut Vec<RequestArgument>,
) -> Result<(), &'static str> {
    let Some(function) = function.filter(|value| !value.is_null()) else {
        return Ok(());
    };
    if !function.is_object() {
        return Err("invalid request tool function");
    }
    let Some(value) = function.get("arguments").filter(|value| !value.is_null()) else {
        return Ok(());
    };
    let text = value.as_str().ok_or("invalid request arguments type")?;
    arguments.push(RequestArgument {
        address: value as *const Value as usize,
        span: document
            .string_span(&path)?
            .ok_or("missing request arguments location")?,
        text: text.to_string(),
    });
    Ok(())
}

fn deny_definition(
    value: &Value,
    dynamic_keys: bool,
    config: &AiDataMaskingConfig,
    log: &Log,
) -> bool {
    match value {
        Value::String(text) => config.check_message(text, log),
        Value::Array(values) => values
            .iter()
            .any(|value| deny_definition(value, false, config, log)),
        Value::Object(values) => values.iter().any(|(key, value)| {
            (dynamic_keys && config.check_message(key, log))
                || deny_definition(
                    value,
                    [
                        "properties",
                        "patternProperties",
                        "$defs",
                        "definitions",
                        "dependencies",
                        "dependentRequired",
                        "dependentSchemas",
                    ]
                    .contains(&key.as_str()),
                    config,
                    log,
                )
        }),
        _ => false,
    }
}

enum LegacyRequestPlan {
    Strings {
        values: Vec<(String, bool)>,
        openai: bool,
        stream: bool,
    },
    Raw,
    None,
}

impl LegacyRequestPlan {
    fn from_config(
        config: &AiDataMaskingConfig,
        value: &Value,
        normalized: &str,
        owned: &HashSet<usize>,
    ) -> Self {
        if config.deny_openai {
            if let Ok(request) = serde_json::from_str::<Req>(normalized) {
                return Self::Strings {
                    values: request
                        .messages
                        .into_iter()
                        .flat_map(|message| {
                            [(message.content, false), (message.reasoning_content, false)]
                        })
                        .collect(),
                    openai: true,
                    stream: request.stream,
                };
            }
        }
        if !config.deny_jsonpath.is_empty() {
            let mut values = Vec::new();
            for path in &config.deny_jsonpath {
                for found in path.find_slice(value) {
                    if let JsonPathValue::Slice(value, _) = found {
                        if let Some(text) = value.as_str() {
                            values.push((
                                text.to_string(),
                                owned.contains(&(value as *const Value as usize)),
                            ));
                        }
                    }
                }
            }
            return Self::Strings {
                values,
                openai: false,
                stream: false,
            };
        }
        if config.deny_raw {
            Self::Raw
        } else {
            Self::None
        }
    }

    fn checked_strings<'a>(&'a self, normalized: &'a str) -> Vec<&'a str> {
        match self {
            Self::Strings { values, .. } => {
                values.iter().map(|(value, _)| value.as_str()).collect()
            }
            Self::Raw => vec![normalized],
            Self::None => Vec::new(),
        }
    }

    fn always_forwards(&self) -> bool {
        matches!(self, Self::Strings { .. })
    }

    fn openai_state(&self) -> (bool, bool) {
        match self {
            Self::Strings { openai, stream, .. } => (*openai, *stream),
            _ => (false, false),
        }
    }
}

// Stop replacement expansion before allocating beyond the opted-in call budget.
struct LimitedOutput {
    bytes: Vec<u8>,
    limit: usize,
}
impl LimitedOutput {
    fn new(limit: usize, initial: usize) -> Self {
        Self {
            bytes: Vec::with_capacity(initial.min(limit)),
            limit,
        }
    }
    fn remaining(&self) -> usize {
        self.limit - self.bytes.len()
    }
}
impl std::io::Write for LimitedOutput {
    fn write(&mut self, bytes: &[u8]) -> std::io::Result<usize> {
        if bytes.len() > self.remaining() {
            return Err(std::io::Error::from(std::io::ErrorKind::WriteZero));
        }
        let length = self.bytes.len() + bytes.len();
        if length > self.bytes.capacity() {
            let capacity = self
                .bytes
                .capacity()
                .saturating_mul(2)
                .max(length)
                .min(self.limit);
            self.bytes.reserve_exact(capacity - self.bytes.len());
        }
        self.bytes.extend_from_slice(bytes);
        Ok(bytes.len())
    }
    fn flush(&mut self) -> std::io::Result<()> {
        Ok(())
    }
}

#[derive(Default)]
struct RequestMasks {
    map: HashMap<String, RestoreEntry>,
    byte_window_size: usize,
    char_window_size: usize,
}

impl RequestMasks {
    fn plain_bounded(
        &mut self,
        config: &AiDataMaskingConfig,
        text: &str,
        max_bytes: usize,
    ) -> Result<String, &'static str> {
        use std::io::Write as _;
        if text.len() > max_bytes {
            return Err("tool masking exceeds call limit");
        }
        let mut current = text.to_string();
        for rule in &config.replace_roles {
            let mut output = LimitedOutput::new(max_bytes, current.len());
            let mut last_end = 0;
            for captures in rule.regex.captures_iter(current.as_str()) {
                let captures = captures.map_err(|_| "request masking rule failed")?;
                let matched = captures.get(0).ok_or("missing rule match")?;
                output
                    .write_all(current[last_end..matched.start()].as_bytes())
                    .map_err(|_| "tool masking exceeds call limit")?;
                let start = output.bytes.len();
                match rule.type_ {
                    Type::Hash => {
                        if output.remaining() < 64 {
                            return Err("tool masking exceeds call limit");
                        }
                        let digest = hmac_sha256::Hash::hash(matched.as_str().as_bytes());
                        for byte in digest {
                            write!(output, "{byte:02x}")
                                .map_err(|_| "tool masking exceeds call limit")?;
                        }
                    }
                    Type::Replace => Expander::default()
                        .write_expansion(&mut output, &rule.value, &captures)
                        .map_err(|_| "tool masking exceeds call limit")?,
                }
                let masked = std::str::from_utf8(&output.bytes[start..])
                    .map_err(|_| "invalid masking output")?;
                self.record(rule, matched.as_str(), masked);
                last_end = matched.end();
            }
            output
                .write_all(current[last_end..].as_bytes())
                .map_err(|_| "tool masking exceeds call limit")?;
            current = String::from_utf8(output.bytes).map_err(|_| "invalid masking output")?;
        }
        Ok(current)
    }

    fn plain(&mut self, config: &AiDataMaskingConfig, text: &str) -> Result<String, &'static str> {
        let mut current = text.to_string();
        for rule in &config.replace_roles {
            current = replace_rule_message(
                rule,
                &current,
                &mut self.map,
                &mut self.byte_window_size,
                &mut self.char_window_size,
            )
            .map_err(|_| "request masking rule failed")?
            .0;
        }
        Ok(current)
    }

    fn record(&mut self, rule: &Rule, original: &str, masked: &str) {
        if rule.type_ == Type::Hash || rule.restore {
            self.byte_window_size = self.byte_window_size.max(masked.len());
            self.char_window_size = self.char_window_size.max(masked.chars().count());
        }
        if rule.restore && !masked.is_empty() {
            record_restore_mapping(
                &mut self.map,
                masked.to_string(),
                original,
                if rule.type_ == Type::Hash {
                    RestoreKind::Hash
                } else {
                    RestoreKind::General
                },
            );
        }
    }
}

struct ScopedBody {
    text: String,
    protected: Vec<Range<usize>>,
}

impl ScopedBody {
    fn restore(&mut self, restore: &MaskRestore) -> Result<(), &'static str> {
        let mut changes = Vec::new();
        for (span, replacement) in RestoreMatches::new(restore, &self.text) {
            if self.is_outside(&span)? {
                changes.push((span, replacement.to_string()));
            }
        }
        self.apply_edits(changes)
    }
    fn is_outside(&self, range: &Range<usize>) -> Result<bool, &'static str> {
        let index = self
            .protected
            .partition_point(|span| span.end <= range.start);
        if let Some(span) = self.protected.get(index) {
            let intersects =
                span.start < range.end || (range.is_empty() && span.start <= range.start);
            if intersects {
                if span.start <= range.start && range.end <= span.end {
                    return Ok(false);
                }
                return Err("legacy rewrite crosses protected arguments");
            }
        }
        Ok(true)
    }

    fn replace_all(&mut self, original: &str, replacement: &str) -> Result<(), &'static str> {
        let mut changes = Vec::new();
        for (start, _) in self.text.match_indices(original) {
            let span = start..start + original.len();
            if self.is_outside(&span)? {
                changes.push((span, replacement.to_string()));
            }
        }
        self.apply_edits(changes)
    }

    fn mask_rule(&mut self, rule: &Rule, masks: &mut RequestMasks) -> Result<(), &'static str> {
        let mut changes = Vec::new();
        for captures in rule.regex.captures_iter(self.text.as_str()) {
            let captures = captures.map_err(|_| "request masking rule failed")?;
            let matched = captures.get(0).ok_or("missing rule match")?;
            let span = matched.start()..matched.end();
            if !self.is_outside(&span)? {
                continue;
            }
            let masked = match rule.type_ {
                Type::Hash => hmac_sha256::Hash::hash(matched.as_str().as_bytes())
                    .iter()
                    .fold(String::new(), |mut text, byte| {
                        let _ = write!(text, "{byte:02x}");
                        text
                    }),
                Type::Replace => {
                    let mut text = String::new();
                    captures.expand(&rule.value, &mut text);
                    text
                }
            };
            masks.record(rule, matched.as_str(), &masked);
            changes.push((span, masked));
        }
        self.apply_edits(changes)
    }

    fn apply_edits(&mut self, changes: Vec<(Range<usize>, String)>) -> Result<(), &'static str> {
        if changes.is_empty() {
            return Ok(());
        }
        let mut delta = 0i128;
        let mut prefix = Vec::new();
        for (span, value) in &changes {
            delta += value.len() as i128 - span.len() as i128;
            prefix.push(delta);
        }
        let shifted = |position: usize| -> Result<usize, &'static str> {
            let count = changes.partition_point(|(span, _)| span.end <= position);
            usize::try_from(position as i128 + if count == 0 { 0 } else { prefix[count - 1] })
                .map_err(|_| "rewrite size overflow")
        };
        let next_spans = self
            .protected
            .iter()
            .map(|span| Ok(shifted(span.start)?..shifted(span.end)?))
            .collect::<Result<Vec<_>, &'static str>>()?;
        let capacity = usize::try_from(self.text.len() as i128 + delta)
            .map_err(|_| "rewrite size overflow")?;
        let mut output = String::with_capacity(capacity);
        let mut cursor = 0;
        for (span, value) in changes {
            if span.start < cursor || span.end > self.text.len() {
                return Err("overlapping rewrite locations");
            }
            output.push_str(&self.text[cursor..span.start]);
            output.push_str(&value);
            cursor = span.end;
        }
        output.push_str(&self.text[cursor..]);
        self.text = output;
        self.protected = next_spans;
        Ok(())
    }
}

#[derive(Debug)]
enum RestoreBuildError {
    TooManyGeneralPatterns(usize),
    GeneralPatternBytesExceeded(usize),
    Matcher(aho_corasick::BuildError),
}

impl std::fmt::Display for RestoreBuildError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::TooManyGeneralPatterns(count) => write!(
                formatter,
                "general restore pattern count {} exceeds limit {}",
                count, MAX_GENERAL_RESTORE_PATTERNS
            ),
            Self::GeneralPatternBytesExceeded(bytes) => write!(
                formatter,
                "general restore pattern bytes {} exceeds limit {}",
                bytes, MAX_GENERAL_RESTORE_PATTERN_BYTES
            ),
            Self::Matcher(error) => write!(formatter, "{}", error),
        }
    }
}

impl From<aho_corasick::BuildError> for RestoreBuildError {
    fn from(error: aho_corasick::BuildError) -> Self {
        Self::Matcher(error)
    }
}

const HASH_MASK_BYTES: usize = 64;
const MAX_GENERAL_RESTORE_PATTERNS: usize = 1_024;
const MAX_GENERAL_RESTORE_PATTERN_BYTES: usize = 64 * 1_024;

impl MaskRestore {
    fn from_map(
        mask_map: HashMap<String, RestoreEntry>,
    ) -> Result<Option<Self>, RestoreBuildError> {
        let mut hashes = HashMap::new();
        let mut general_pairs = Vec::new();
        let mut general_pattern_bytes = 0;
        for (masked, entry) in mask_map {
            let Some(original) = entry.original else {
                continue;
            };
            match entry.kind {
                RestoreKind::Hash => {
                    hashes.insert(masked, original);
                }
                RestoreKind::General => {
                    general_pattern_bytes += masked.len();
                    general_pairs.push((masked, original));
                }
            }
        }

        if general_pairs.len() > MAX_GENERAL_RESTORE_PATTERNS {
            return Err(RestoreBuildError::TooManyGeneralPatterns(
                general_pairs.len(),
            ));
        }
        if general_pattern_bytes > MAX_GENERAL_RESTORE_PATTERN_BYTES {
            return Err(RestoreBuildError::GeneralPatternBytesExceeded(
                general_pattern_bytes,
            ));
        }
        if hashes.is_empty() && general_pairs.is_empty() {
            return Ok(None);
        }
        general_pairs.sort_unstable_by(|left, right| left.0.cmp(&right.0));

        let general = if general_pairs.is_empty() {
            None
        } else {
            let matcher = AhoCorasickBuilder::new()
                .match_kind(MatchKind::LeftmostLongest)
                .kind(Some(AhoCorasickKind::ContiguousNFA))
                .build(general_pairs.iter().map(|(masked, _)| masked))?;
            let replacements = general_pairs
                .into_iter()
                .map(|(_, original)| original)
                .collect();
            Some(GeneralRestore {
                matcher,
                replacements,
            })
        };
        Ok(Some(Self { general, hashes }))
    }

    fn restore(&self, message: &str) -> String {
        self.restore_bounded(message, usize::MAX)
            .expect("unbounded restore size overflow")
            .unwrap_or_else(|| message.to_string())
    }

    fn restore_bounded(
        &self,
        message: &str,
        max_bytes: usize,
    ) -> Result<Option<String>, &'static str> {
        let mut restored = String::with_capacity(message.len().min(max_bytes));
        let mut last_end = 0;
        for (span, replacement) in RestoreMatches::new(self, message) {
            let bytes = restored
                .len()
                .checked_add(span.start - last_end)
                .and_then(|bytes| bytes.checked_add(replacement.len()))
                .ok_or("tool restore size overflow")?;
            if bytes > max_bytes {
                return Err("tool restore output limit exceeded");
            }
            restored.push_str(&message[last_end..span.start]);
            restored.push_str(replacement);
            last_end = span.end;
        }
        if last_end == 0 {
            return Ok(None);
        }
        if restored
            .len()
            .checked_add(message.len() - last_end)
            .ok_or("tool restore size overflow")?
            > max_bytes
        {
            return Err("tool restore output limit exceeded");
        }
        restored.push_str(&message[last_end..]);
        Ok(Some(restored))
    }

    fn next_general_match(&self, message: &str, start: usize) -> Option<aho_corasick::Match> {
        self.general.as_ref().and_then(|general| {
            general
                .matcher
                .find(Input::new(message).span(start..message.len()))
        })
    }
}

struct RestoreMatches<'a> {
    owner: &'a MaskRestore,
    message: &'a str,
    general: Option<aho_corasick::Match>,
    hash: Option<(usize, usize, &'a str)>,
    hash_cursor: usize,
}

impl<'a> RestoreMatches<'a> {
    fn new(owner: &'a MaskRestore, message: &'a str) -> Self {
        let mut hash_cursor = 0;
        Self {
            owner,
            message,
            general: owner.next_general_match(message, 0),
            hash: next_hash_match(message, &owner.hashes, &mut hash_cursor),
            hash_cursor,
        }
    }
}

impl<'a> Iterator for RestoreMatches<'a> {
    type Item = (Range<usize>, &'a str);
    fn next(&mut self) -> Option<Self::Item> {
        let use_general = match (&self.general, &self.hash) {
            (Some(general), Some((start, end, _))) => {
                general.start() < *start
                    || (general.start() == *start && general.end() - general.start() >= end - start)
            }
            (Some(_), None) => true,
            (None, Some(_)) => false,
            (None, None) => return None,
        };
        let (start, end, replacement) = if use_general {
            let matched = self.general.take().unwrap();
            let general = self.owner.general.as_ref().unwrap();
            (
                matched.start(),
                matched.end(),
                general.replacements[matched.pattern().as_usize()].as_str(),
            )
        } else {
            self.hash.take().unwrap()
        };
        if use_general {
            self.general = self.owner.next_general_match(self.message, end);
            if self.hash.as_ref().is_some_and(|(start, _, _)| *start < end) {
                self.hash_cursor = end;
                self.hash =
                    next_hash_match(self.message, &self.owner.hashes, &mut self.hash_cursor);
            }
        } else {
            self.hash = next_hash_match(self.message, &self.owner.hashes, &mut self.hash_cursor);
            if self
                .general
                .as_ref()
                .is_some_and(|matched| matched.start() < end)
            {
                self.general = self.owner.next_general_match(self.message, end);
            }
        }
        Some((start..end, replacement))
    }
}

fn next_hash_match<'a>(
    message: &str,
    hashes: &'a HashMap<String, String>,
    cursor: &mut usize,
) -> Option<(usize, usize, &'a str)> {
    let bytes = message.as_bytes();
    while *cursor + HASH_MASK_BYTES <= bytes.len() {
        let start = *cursor;
        *cursor += 1;
        let candidate = &bytes[start..start + HASH_MASK_BYTES];
        if candidate
            .iter()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(byte))
        {
            let candidate = std::str::from_utf8(candidate).unwrap();
            if let Some(original) = hashes.get(candidate) {
                *cursor = start + HASH_MASK_BYTES;
                return Some((start, start + HASH_MASK_BYTES, original));
            }
        }
    }
    None
}

fn record_restore_mapping(
    mask_map: &mut HashMap<String, RestoreEntry>,
    masked: String,
    original: &str,
    kind: RestoreKind,
) {
    use std::collections::hash_map::Entry;

    match mask_map.entry(masked) {
        Entry::Vacant(entry) => {
            entry.insert(RestoreEntry {
                original: Some(original.to_string()),
                kind,
            });
        }
        Entry::Occupied(mut entry) => match entry.get().original.as_deref() {
            Some(existing) if existing == original => {
                if kind == RestoreKind::Hash {
                    entry.get_mut().kind = RestoreKind::Hash;
                }
            }
            Some(_) => {
                entry.get_mut().original = None;
            }
            None => {}
        },
    }
}

fn replace_rule_message(
    rule: &Rule,
    message: &str,
    mask_map: &mut HashMap<String, RestoreEntry>,
    byte_window_size: &mut usize,
    char_window_size: &mut usize,
) -> Result<(String, usize), fancy_regex::Error> {
    let mut replaced = String::with_capacity(message.len());
    let mut last_end = 0;
    let mut match_count = 0;

    for captures in rule.regex.captures_iter(message) {
        let captures = captures?;
        let matched = captures.get(0).unwrap();
        replaced.push_str(&message[last_end..matched.start()]);

        let original = matched.as_str();
        let masked = match rule.type_ {
            Type::Hash => {
                let digest = hmac_sha256::Hash::hash(original.as_bytes());
                digest.iter().fold(String::new(), |mut output, byte| {
                    let _ = write!(output, "{byte:02x}");
                    output
                })
            }
            Type::Replace => {
                let mut expanded = String::new();
                captures.expand(&rule.value, &mut expanded);
                expanded
            }
        };

        if rule.type_ == Type::Hash || rule.restore {
            *byte_window_size = (*byte_window_size).max(masked.len());
            *char_window_size = (*char_window_size).max(masked.chars().count());
        }
        if rule.restore && !masked.is_empty() {
            let kind = if rule.type_ == Type::Hash {
                RestoreKind::Hash
            } else {
                RestoreKind::General
            };
            record_restore_mapping(mask_map, masked.clone(), original, kind);
        }

        replaced.push_str(&masked);
        last_end = matched.end();
        match_count += 1;
    }

    if match_count == 0 {
        return Ok((message.to_string(), 0));
    }
    replaced.push_str(&message[last_end..]);
    Ok((replaced, match_count))
}
fn default_deny_openai() -> bool {
    true
}
fn default_deny_raw() -> bool {
    false
}
fn default_system_deny() -> bool {
    false
}
fn default_deny_code() -> u16 {
    200
}
fn default_deny_content_type() -> String {
    "application/json".to_string()
}
fn default_deny_raw_message() -> String {
    "{\"errmsg\":\"提问或回答中包含敏感词，已被屏蔽\"}".to_string()
}
fn default_deny_message() -> String {
    "提问或回答中包含敏感词，已被屏蔽".to_string()
}
#[derive(Default, Debug, Deserialize, Clone)]
pub struct AiDataMaskingConfig {
    #[serde(default)]
    tool_arguments: ToolArgumentsConfig,
    #[serde(default)]
    tool_definitions: ToolDefinitionsConfig,
    #[serde(default = "default_deny_openai")]
    deny_openai: bool,
    #[serde(default = "default_deny_raw")]
    deny_raw: bool,
    #[serde(default, deserialize_with = "deserialize_jsonpath")]
    deny_jsonpath: Vec<JsonPath>,
    #[serde(default = "default_system_deny")]
    system_deny: bool,
    #[serde(default = "default_deny_code")]
    deny_code: u16,
    #[serde(default = "default_deny_message")]
    deny_message: String,
    #[serde(default = "default_deny_raw_message")]
    deny_raw_message: String,
    #[serde(default = "default_deny_content_type")]
    deny_content_type: String,
    #[serde(default)]
    replace_roles: Vec<Rule>,
    #[serde(deserialize_with = "deserialize_denyword", default = "DenyWord::empty")]
    deny_words: DenyWord,
}

impl AiDataMaskingConfig {
    fn check_message(&self, message: &str, log: &Log) -> bool {
        if let Some(word) = self.deny_words.check(message) {
            log.warn(&format!(
                "custom deny word {} matched; input_bytes={}",
                word,
                message.len()
            ));
            return true;
        } else if self.system_deny {
            if let Some(word) = SYSTEM.deny_word.check(message) {
                log.warn(&format!(
                    "system deny word {} matched; input_bytes={}",
                    word,
                    message.len()
                ));
                return true;
            }
        }
        false
    }
}

fn install_tool_configuration(
    matcher: &mut RuleMatcher<AiDataMaskingConfig>,
    value: &Value,
) -> Result<(), &'static str> {
    let object = value
        .as_object()
        .ok_or("plugin configuration must be an object")?;
    if object.keys().any(|key| key != "_rules_") {
        serde_json::from_value::<AiDataMaskingConfig>(value.clone())
            .map_err(|_| "invalid global tool configuration")?;
    }
    if let Some(rules) = object.get("_rules_") {
        for rule in rules.as_array().ok_or("plugin rules must be an array")? {
            if !rule.is_object() {
                return Err("plugin rule must be an object");
            }
            serde_json::from_value::<AiDataMaskingConfig>(rule.clone())
                .map_err(|_| "invalid per-rule tool configuration")?;
        }
    }
    let mut candidate = RuleMatcher::<AiDataMaskingConfig>::default();
    candidate
        .parse_rule_config(value)
        .map_err(|_| "invalid tool route configuration")?;
    *matcher = candidate;
    Ok(())
}
#[derive(Debug, Deserialize, Clone)]
struct Message {
    #[serde(default)]
    content: String,
    #[serde(default)]
    reasoning_content: String,
}
#[derive(Debug, Deserialize, Clone)]
struct Req {
    #[serde(default)]
    stream: bool,
    messages: Vec<Message>,
}

#[derive(Default, Debug, Deserialize)]
struct ResMessage {
    #[serde(default)]
    message: Option<Message>,
}

#[derive(Default, Debug, Deserialize)]
struct Res {
    #[serde(default)]
    choices: Vec<ResMessage>,
}

static SYSTEM_PATTERNS: &[(&str, &str)] = &[
    ("MOBILE", r#"\d{8,11}"#),
    ("IDCARD", r#"\d{17}[0-9xX]|\d{15}"#),
];

impl System {
    fn new() -> Self {
        let grok_regex = Regex::new(GROK_PATTERN).unwrap();
        let grok_patterns = BTreeMap::new();
        let mut system = System {
            deny_word: DenyWord::system(),
            grok_regex,
            grok_patterns,
        };
        system.init();
        system
    }
    fn init(&mut self) {
        let mut grok_temp_patterns = VecDeque::new();
        for patterns in [patterns(), SYSTEM_PATTERNS] {
            for &(key, value) in patterns {
                if self.grok_regex.is_match(value).is_ok_and(|r| r) {
                    grok_temp_patterns.push_back((String::from(key), String::from(value)));
                } else {
                    self.grok_patterns
                        .insert(String::from(key), String::from(value));
                }
            }
        }
        let mut last_ok: Option<String> = None;

        while let Some((key, value)) = grok_temp_patterns.pop_front() {
            if let Some(k) = &last_ok {
                if k == &key {
                    break;
                }
            }
            let (v, ok) = self.grok_to_pattern(&value);
            if ok {
                self.grok_patterns.insert(key, v);
                last_ok = None;
            } else {
                if last_ok.is_none() {
                    last_ok = Some(key.clone());
                }
                grok_temp_patterns.push_back((key, v));
            }
        }
    }
    fn grok_to_pattern(&self, pattern: &str) -> (String, bool) {
        let mut ok = true;
        let mut ret = pattern.to_string();
        for capture in self.grok_regex.captures_iter(pattern) {
            if capture.is_err() {
                ok = false;
                continue;
            }
            let c = capture.unwrap();
            if let (Some(full), Some(name)) = (c.get(0), c.name("pattern")) {
                if let Some(p) = self.grok_patterns.get(name.as_str()) {
                    if let Some(alias) = c.name("alias") {
                        ret = ret.replace(full.as_str(), &format!("(?P<{}>{})", alias.as_str(), p));
                    } else {
                        ret = ret.replace(full.as_str(), p);
                    }
                } else {
                    ok = false;
                }
            }
        }
        (ret, ok)
    }
}

impl AiDataMaskingRoot {
    fn new() -> Self {
        AiDataMaskingRoot {
            log: Log::new(PLUGIN_NAME.to_string()),
            rule_matcher: Rc::new(RefCell::new(RuleMatcher::default())),
            tools_configured: false,
        }
    }
}

impl Context for AiDataMaskingRoot {}

impl RootContext for AiDataMaskingRoot {
    fn on_configure(&mut self, plugin_configuration_size: usize) -> bool {
        let Some(bytes) = self.get_plugin_configuration() else {
            self.log.error("missing plugin configuration");
            return false;
        };
        let Ok(value) = serde_json::from_slice::<Value>(&bytes) else {
            self.log.error("invalid plugin configuration JSON");
            return false;
        };
        let requested = requests_tool_processing(&value);
        if requested || self.tools_configured {
            match install_tool_configuration(self.rule_matcher.borrow_mut().deref_mut(), &value) {
                Ok(()) => {
                    self.tools_configured = requested;
                    return true;
                }
                Err(reason) => {
                    self.log.error(reason);
                    return false;
                }
            }
        }
        on_configure(
            self,
            plugin_configuration_size,
            self.rule_matcher.borrow_mut().deref_mut(),
            &self.log,
        )
    }
    fn create_http_context(&self, context_id: u32) -> Option<Box<dyn HttpContext>> {
        self.create_http_context_use_wrapper(context_id)
    }
    fn get_type(&self) -> Option<ContextType> {
        Some(ContextType::HttpContext)
    }
}

impl RootContextWrapper<AiDataMaskingConfig> for AiDataMaskingRoot {
    fn rule_matcher(&self) -> &SharedRuleMatcher<AiDataMaskingConfig> {
        &self.rule_matcher
    }

    fn create_http_context_wrapper(
        &self,
        _context_id: u32,
    ) -> Option<Box<dyn HttpContextWrapper<AiDataMaskingConfig>>> {
        Some(Box::new(AiDataMasking::new()))
    }
}

impl AiDataMasking {
    fn new() -> Self {
        Self {
            weak: Weak::default(),
            mask_map: HashMap::new(),
            mask_restore: None,
            tools: None,
            config: None,
            is_openai: false,
            is_openai_stream: None,
            stream: false,
            msg_window: MsgWindow::default(),
            log: Log::new(PLUGIN_NAME.to_string()),
            char_window_size: 0,
            byte_window_size: 0,
        }
    }
    fn fail_tool_stream(&mut self, mut tools: ToolState, reason: &'static str) -> DataAction {
        tools.failed = true;
        tools.stream = tool_stream::ToolStream::default();
        tools.raw_restore = tool_raw::RawRestoreWindow::default();
        tools.raw_deny = tool_raw::RawDenyWindow::default();
        self.msg_window = MsgWindow::default();
        self.tools = Some(tools);
        self.log.warn(reason);
        self.deny(true)
    }

    fn process_tool_stream(&mut self, body: &[u8], end: bool) -> DataAction {
        use tool_stream::StreamAction;
        let mut tools = self.tools.take().expect("active tool stream");
        if tools.failed {
            self.tools = Some(tools);
            self.replace_http_response_body(&[]);
            return DataAction::Continue;
        }
        tools.body_started = true;
        let mut buffered = None;
        if tools.response_stream != Some(true) {
            tools.probe.extend_from_slice(body);
            if tools.response_stream.is_none() {
                tools.response_stream = tool_response_format(&tools.probe, end);
            }
            match tools.response_stream {
                Some(false) if end => {
                    let bytes = std::mem::take(&mut tools.probe);
                    self.tools = Some(tools);
                    self.replace_http_response_body(&bytes);
                    return self.on_http_response_complete_body(&bytes);
                }
                Some(true) => buffered = Some(std::mem::take(&mut tools.probe)),
                _ => {
                    self.tools = Some(tools);
                    self.replace_http_response_body(&[]);
                    return DataAction::Continue;
                }
            }
        }
        let body = buffered.as_deref().unwrap_or(body);
        let config = self
            .config
            .as_ref()
            .expect("configured tool stream")
            .clone();
        let legacy_text = config.deny_openai && self.is_openai;
        let raw_mode = !legacy_text && config.deny_raw;
        if raw_mode {
            if let Err(reason) = tools.raw_deny.push(body, end, &config, &self.log) {
                return self.fail_tool_stream(tools, reason);
            }
        }
        let actions = match tools
            .stream
            .push(body, end, &config, &tools.restore, &self.log)
        {
            Ok(actions) => actions,
            Err(reason) => {
                tools.failed = true;
                tools.stream = tool_stream::ToolStream::default();
                self.msg_window = MsgWindow::default();
                self.tools = Some(tools);
                self.log.warn(reason);
                return self.deny(true);
            }
        };
        if raw_mode {
            let result = (|| -> Result<Vec<u8>, &'static str> {
                if tools.stream.errored() {
                    tools.raw_restore.discard_arguments();
                }
                let mut output = Vec::new();
                let keep = self.byte_window_size.saturating_mul(2);
                for action in actions {
                    match action {
                        StreamAction::Event { raw, .. } => output.extend(tools.raw_restore.push(
                            &raw,
                            false,
                            self.mask_restore.as_ref(),
                            keep,
                            false,
                        )?),
                        StreamAction::Arguments(value) => output.extend(tools.raw_restore.push(
                            format!("data: {value}\n\n").as_bytes(),
                            true,
                            self.mask_restore.as_ref(),
                            keep,
                            false,
                        )?),
                        StreamAction::FlushChoice(_) | StreamAction::FlushAll => {}
                    }
                }
                if end {
                    if tools.stream.finish_raw() {
                        output.extend(tools.raw_restore.push(
                            b"data: [DONE]\n\n",
                            false,
                            self.mask_restore.as_ref(),
                            keep,
                            false,
                        )?);
                    }
                    output.extend(tools.raw_restore.push(
                        &[],
                        false,
                        self.mask_restore.as_ref(),
                        keep,
                        true,
                    )?);
                }
                tools
                    .stream
                    .retain_queued(tools.raw_restore.pending_arguments())?;
                Ok(output)
            })();
            match result {
                Ok(output) => {
                    self.tools = Some(tools);
                    self.replace_http_response_body(&output);
                    return DataAction::Continue;
                }
                Err(reason) => return self.fail_tool_stream(tools, reason),
            }
        }
        let mut denied = false;
        let legacy_restore = self.mask_restore.as_ref();
        let log = &self.log;
        let mut inspect = |message: &mut Vec<u8>| {
            if denied {
                return false;
            }
            if legacy_text {
                if let Ok(text) = std::str::from_utf8(message) {
                    if config.check_message(text, log) {
                        denied = true;
                        return false;
                    }
                    if let Some(restore) = legacy_restore {
                        *message = restore.restore(text).into_bytes();
                    }
                }
            }
            true
        };
        let chars = self.char_window_size.saturating_mul(2);
        let bytes = self.byte_window_size.saturating_mul(2);
        for action in actions {
            match action {
                StreamAction::Event { raw, payload } => {
                    self.msg_window
                        .push_tool_event(raw, &payload, chars, bytes, &mut inspect)
                }
                StreamAction::FlushChoice(index) => {
                    self.msg_window.flush_tool_choice(index, &mut inspect)
                }
                StreamAction::FlushAll => self.msg_window.flush_tool_text(&mut inspect),
                StreamAction::Arguments(value) => self.msg_window.queue_tool_arguments(value),
            }
        }
        let output = if end {
            self.msg_window.finish(true, &mut inspect)
        } else {
            self.msg_window.pop(chars, bytes, true)
        };
        if denied {
            tools.failed = true;
            tools.stream = tool_stream::ToolStream::default();
            self.msg_window = MsgWindow::default();
        } else {
            tools.stream.release_queued();
        }
        self.tools = Some(tools);
        if denied {
            return self.deny(true);
        }
        self.replace_http_response_body(&output);
        DataAction::Continue
    }

    fn check_message(&self, message: &str) -> bool {
        if let Some(config) = &self.config {
            config.check_message(message, self.log())
        } else {
            false
        }
    }
    fn msg_to_response(&self, msg: &str, raw_msg: &str, content_type: &str) -> (String, String) {
        if !self.is_openai {
            (raw_msg.to_string(), content_type.to_string())
        } else if self.stream {
            (
                format!(
                    "data:{}\n\n",
                    json!({"choices": [{"index": 0, "delta": {"role": "assistant", "content": msg}}], "usage": {}})
                ),
                "text/event-stream;charset=UTF-8".to_string(),
            )
        } else {
            (
                json!({"choices": [{"index": 0, "message": {"role": "assistant", "content": msg}}], "usage": {}}).to_string(),
                 "application/json".to_string()
            )
        }
    }
    fn deny(&mut self, in_response: bool) -> DataAction {
        if in_response
            && self.tools.as_ref().map_or(self.stream, |tools| {
                tools.response_stream.unwrap_or(tools.request_stream)
            })
        {
            self.replace_http_response_body(&[]);
            return DataAction::Continue;
        }
        let (deny_code, (deny_message, content_type)) = if let Some(config) = &self.config {
            (
                config.deny_code,
                self.msg_to_response(
                    &config.deny_message,
                    &config.deny_raw_message,
                    &config.deny_content_type,
                ),
            )
        } else {
            (
                default_deny_code(),
                self.msg_to_response(
                    &default_deny_message(),
                    &default_deny_raw_message(),
                    &default_deny_content_type(),
                ),
            )
        };
        if in_response {
            self.replace_http_response_body(deny_message.as_bytes());
            return DataAction::Continue;
        }
        self.send_http_response(
            deny_code as u32,
            vec![("Content-Type", &content_type)],
            Some(deny_message.as_bytes()),
        );
        DataAction::StopIterationAndBuffer
    }

    fn replace_request_msg(&mut self, message: &str) -> Result<String, fancy_regex::Error> {
        let config = self.config.as_ref().unwrap();
        let mut msg = message.to_string();
        let mut match_count = 0;
        for rule in &config.replace_roles {
            let (new_msg, rule_match_count) = replace_rule_message(
                rule,
                &msg,
                &mut self.mask_map,
                &mut self.byte_window_size,
                &mut self.char_window_size,
            )?;
            msg = new_msg;
            match_count += rule_match_count;
        }
        if msg != message {
            self.log().debug(&format!(
                "replace_request_msg completed: input_bytes={}, output_bytes={}, matches={}",
                message.len(),
                msg.len(),
                match_count
            ));
        }
        Ok(msg)
    }

    fn forward_request_body(&mut self, body: &str) -> DataAction {
        match MaskRestore::from_map(std::mem::take(&mut self.mask_map)) {
            Ok(mask_restore) => self.mask_restore = mask_restore,
            Err(error) => {
                self.log().error(&format!(
                    "failed to build response restore matcher: {}",
                    error
                ));
                return self.deny(false);
            }
        }
        self.replace_http_request_body(body.as_bytes());
        DataAction::Continue
    }

    fn deny_request_masking_error(&mut self, error: &fancy_regex::Error) -> DataAction {
        self.log()
            .error(&format!("request masking regex failed: {}", error));
        self.deny(false)
    }
}

impl Context for AiDataMasking {}

impl HttpContext for AiDataMasking {
    fn on_http_request_headers(
        &mut self,
        _num_headers: usize,
        _end_of_stream: bool,
    ) -> HeaderAction {
        if has_request_body() {
            self.set_http_request_header("Content-Length", None);
            HeaderAction::StopIteration
        } else {
            HeaderAction::Continue
        }
    }
    fn on_http_response_headers(
        &mut self,
        _num_headers: usize,
        _end_of_stream: bool,
    ) -> HeaderAction {
        self.set_http_response_header("Content-Length", None);
        let tool_content_type = if self.tools.is_some() {
            self.get_http_response_header("content-type")
        } else {
            None
        };
        if let Some(tools) = &mut self.tools {
            if let Some(content_type) = tool_content_type {
                let sse = content_type
                    .split(';')
                    .next()
                    .is_some_and(|media| media.trim().eq_ignore_ascii_case("text/event-stream"));
                tools.response_stream = Some(sse);
            }
        }
        // The first body callback may contain only part of the data: prefix.
        if self.is_openai
            && self
                .get_http_response_header("content-type")
                .is_some_and(|value| {
                    value.split(';').next().is_some_and(|media_type| {
                        media_type.trim().eq_ignore_ascii_case("text/event-stream")
                    })
                })
        {
            self.is_openai_stream = Some(true);
        }
        HeaderAction::Continue
    }

    fn on_http_response_body(&mut self, body_size: usize, end_of_stream: bool) -> DataAction {
        if self.tools.as_ref().is_some_and(|tools| {
            tools.body_started || tools.response_stream.unwrap_or(tools.request_stream)
        }) {
            let body = if body_size > 0 {
                self.get_http_response_body(0, body_size)
                    .unwrap_or_default()
            } else {
                Vec::new()
            };
            return self.process_tool_stream(&body, end_of_stream);
        }
        if !self.stream {
            return DataAction::Continue;
        }
        let body = if body_size > 0 {
            self.get_http_response_body(0, body_size)
        } else {
            None
        };
        if let Some(body) = &body {
            if self.is_openai && self.is_openai_stream.is_none() {
                self.is_openai_stream = Some(body.starts_with(b"data:"));
            }
        }
        let is_openai_stream = self.is_openai_stream.unwrap_or_default();
        let mut deny = false;
        let log = Log::new(PLUGIN_NAME.to_string());
        let config = self.config.as_ref();
        let mask_restore = self.mask_restore.as_ref();
        let mut inspect = |message: &mut Vec<u8>| {
            if deny {
                return false;
            }
            if let Ok(mut msg) = String::from_utf8(message.clone()) {
                if let Some(config) = config {
                    if config.check_message(&msg, &log) {
                        deny = true;
                        return false;
                    }
                }
                if let Some(mask_restore) = mask_restore {
                    msg = mask_restore.restore(&msg);
                }
                message.clear();
                message.extend_from_slice(msg.as_bytes());
            }
            true
        };
        if let Some(body) = body {
            self.msg_window.push(
                &body,
                is_openai_stream,
                self.char_window_size * 2,
                self.byte_window_size * 2,
                &mut inspect,
            );
        }
        let new_body = if end_of_stream {
            // Parse and inspect the final event before flushing buffered text.
            self.msg_window.finish(is_openai_stream, &mut inspect)
        } else {
            self.msg_window.pop(
                self.char_window_size * 2,
                self.byte_window_size * 2,
                is_openai_stream,
            )
        };
        if deny {
            return self.deny(true);
        }
        self.replace_http_response_body(&new_body);
        DataAction::Continue
    }
}

impl HttpContextWrapper<AiDataMaskingConfig> for AiDataMasking {
    fn init_self_weak(
        &mut self,
        self_weak: std::rc::Weak<RefCell<Box<dyn HttpContextWrapper<AiDataMaskingConfig>>>>,
    ) {
        self.weak = self_weak;
    }
    fn log(&self) -> &Log {
        &self.log
    }
    fn on_config(&mut self, config: Rc<AiDataMaskingConfig>) {
        self.config = Some(config.clone());
    }
    fn cache_request_body(&self) -> bool {
        true
    }
    fn cache_response_body(&self) -> bool {
        !self.tools.as_ref().map_or(self.stream, |tools| {
            tools.body_started || tools.response_stream.unwrap_or(tools.request_stream)
        })
    }
    fn on_http_request_complete_body(&mut self, req_body: &Bytes) -> DataAction {
        if self.config.is_none() {
            return DataAction::Continue;
        }
        let config = self.config.as_ref().unwrap().clone();
        if config.tool_arguments.check
            || config.tool_arguments.restore
            || config.tool_definitions.check
        {
            match prepare_tool_request(&config, req_body, self.log()) {
                Ok(Some(prepared)) => {
                    self.is_openai = prepared.is_openai;
                    self.stream = prepared.legacy_stream;
                    self.byte_window_size = prepared.byte_window_size;
                    self.char_window_size = prepared.char_window_size;
                    let restore = if config.tool_arguments.restore {
                        match ToolRestore::from_maps(&prepared.legacy_map, &prepared.tool_map) {
                            Ok(restore) => restore,
                            Err(reason) => {
                                self.log.error(reason);
                                return self.deny(false);
                            }
                        }
                    } else {
                        ToolRestore::default()
                    };
                    self.mask_restore = match MaskRestore::from_map(prepared.legacy_map) {
                        Ok(restore) => restore,
                        Err(_) => {
                            self.log.error("legacy restore matcher failed");
                            return self.deny(false);
                        }
                    };
                    let has_deny = config.system_deny || !config.deny_words.is_empty();
                    let arguments_owned =
                        config.tool_arguments.check || config.tool_arguments.restore;
                    if (config.tool_arguments.check && has_deny)
                        || (config.tool_arguments.restore && restore.has_entries)
                        || (arguments_owned && self.mask_restore.is_some())
                    {
                        self.tools = Some(ToolState {
                            request_stream: prepared.tool_stream,
                            response_stream: None,
                            body_started: false,
                            probe: Vec::new(),
                            restore,
                            stream: tool_stream::ToolStream::default(),
                            raw_restore: tool_raw::RawRestoreWindow::default(),
                            raw_deny: tool_raw::RawDenyWindow::default(),
                            failed: false,
                        });
                    }
                    if let Some(body) = prepared.body {
                        self.replace_http_request_body(body.as_bytes());
                    }
                    return DataAction::Continue;
                }
                Ok(None) => {}
                Err(reason) => {
                    // Keep the existing refusal format when the old text
                    // protocol could have been recognized on this request.
                    if config.deny_openai {
                        if let Some(request) = serde_json::from_slice::<Value>(req_body)
                            .ok()
                            .and_then(|value| serde_json::from_value::<Req>(value).ok())
                        {
                            self.is_openai = true;
                            self.stream = request.stream;
                        }
                    }
                    self.log.warn(reason);
                    return self.deny(false);
                }
            }
        }
        let mut req_body = match serde_json::from_slice::<Value>(req_body) {
            Ok(r) => r.to_string(),
            Err(_) => {
                if let Ok(r) = String::from_utf8(req_body.clone()) {
                    r
                } else {
                    return DataAction::Continue;
                }
            }
        };
        if config.deny_openai {
            if let Ok(req) = serde_json::from_str::<Req>(req_body.as_str()) {
                self.is_openai = true;
                self.stream = req.stream;
                for msg in req.messages {
                    if self.check_message(&msg.content)
                        || self.check_message(&msg.reasoning_content)
                    {
                        return self.deny(false);
                    }
                    let new_content = match self.replace_request_msg(&msg.content) {
                        Ok(content) => content,
                        Err(error) => return self.deny_request_masking_error(&error),
                    };
                    let new_reasoning_content =
                        match self.replace_request_msg(&msg.reasoning_content) {
                            Ok(content) => content,
                            Err(error) => return self.deny_request_masking_error(&error),
                        };
                    if new_content != msg.content {
                        req_body = req_body.replace(
                            &Value::String(msg.content).to_string(),
                            &Value::String(new_content).to_string(),
                        );
                    }
                    if new_reasoning_content != msg.reasoning_content {
                        req_body = req_body.replace(
                            &Value::String(msg.reasoning_content).to_string(),
                            &Value::String(new_reasoning_content).to_string(),
                        );
                    }
                }
                return self.forward_request_body(&req_body);
            }
        }
        if !config.deny_jsonpath.is_empty() {
            if let Ok(json) = serde_json::from_str::<Value>(req_body.as_str()) {
                for jsonpath in config.deny_jsonpath.clone() {
                    for v in jsonpath.find_slice(&json) {
                        if let JsonPathValue::Slice(d, _) = v {
                            if let Some(s) = d.as_str() {
                                if self.check_message(s) {
                                    return self.deny(false);
                                }
                                let content = s.to_string();
                                let new_content = match self.replace_request_msg(&content) {
                                    Ok(content) => content,
                                    Err(error) => return self.deny_request_masking_error(&error),
                                };
                                if new_content != content {
                                    req_body = req_body.replace(
                                        &Value::String(content).to_string(),
                                        &Value::String(new_content).to_string(),
                                    );
                                }
                            }
                        }
                    }
                }
                return self.forward_request_body(&req_body);
            }
        }
        if config.deny_raw {
            if self.check_message(&req_body) {
                return self.deny(false);
            }
            let new_body = match self.replace_request_msg(&req_body) {
                Ok(body) => body,
                Err(error) => return self.deny_request_masking_error(&error),
            };
            if new_body != req_body {
                return self.forward_request_body(&new_body);
            }
            return DataAction::Continue;
        }
        DataAction::Continue
    }
    fn on_http_response_complete_body(&mut self, res_body: &Bytes) -> DataAction {
        if self.config.is_none() {
            return DataAction::Continue;
        }
        let config = self.config.as_ref().unwrap().clone();
        if let Some(tools) = &mut self.tools {
            if !tools.body_started && tools.response_stream.is_none() {
                tools.response_stream = tool_response_format(res_body, true);
                if tools.response_stream == Some(true) {
                    return self.process_tool_stream(res_body, true);
                }
            }
        }
        if let Some(tools) = &self.tools {
            if tools.failed {
                self.replace_http_response_body(&[]);
                return DataAction::Continue;
            }
            match prepare_tool_response(
                &config,
                res_body,
                self.is_openai,
                self.mask_restore.as_ref(),
                &tools.restore,
                self.log(),
            ) {
                Ok(Some(body)) => self.replace_http_response_body(body.as_bytes()),
                Ok(None) => {}
                Err(reason) => {
                    if let Some(tools) = &mut self.tools {
                        tools.failed = true;
                    }
                    self.log.warn(reason);
                    return self.deny(true);
                }
            }
            return DataAction::Continue;
        }
        let mut res_body = match serde_json::from_slice::<Value>(res_body) {
            Ok(r) => r.to_string(),
            Err(_) => {
                if let Ok(r) = String::from_utf8(res_body.clone()) {
                    r
                } else {
                    return DataAction::Continue;
                }
            }
        };
        if config.deny_openai && self.is_openai {
            if let Ok(res) = serde_json::from_str::<Res>(res_body.as_str()) {
                for msg in res.choices {
                    if let Some(message) = msg.message {
                        if self.check_message(&message.content)
                            || self.check_message(&message.reasoning_content)
                        {
                            return self.deny(true);
                        }

                        let Some(mask_restore) = &self.mask_restore else {
                            continue;
                        };
                        let new_content = mask_restore.restore(&message.content);
                        let new_reasoning_content =
                            mask_restore.restore(&message.reasoning_content);
                        if new_content != message.content {
                            res_body = res_body.replace(
                                &Value::String(message.content).to_string(),
                                &Value::String(new_content).to_string(),
                            );
                        }
                        if new_reasoning_content != message.reasoning_content {
                            res_body = res_body.replace(
                                &Value::String(message.reasoning_content).to_string(),
                                &Value::String(new_reasoning_content).to_string(),
                            );
                        }
                    }
                }
                self.replace_http_response_body(res_body.as_bytes());

                return DataAction::Continue;
            }
        }
        if config.deny_raw {
            if self.check_message(&res_body) {
                return self.deny(true);
            }
            if let Some(mask_restore) = &self.mask_restore {
                res_body = mask_restore.restore(&res_body);
            }
            self.replace_http_response_body(res_body.as_bytes());
            return DataAction::Continue;
        }
        DataAction::Continue
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::{Duration, Instant};

    fn rule(regex: &str, type_: Type, restore: bool, value: &str) -> Rule {
        Rule {
            regex: Regex::new(regex).unwrap(),
            type_,
            restore,
            value: value.to_string(),
        }
    }

    fn replace(rule: &Rule, message: &str) -> (String, HashMap<String, RestoreEntry>, usize) {
        let mut mask_map = HashMap::new();
        let mut byte_window_size = 0;
        let mut char_window_size = 0;
        let (replaced, match_count) = replace_rule_message(
            rule,
            message,
            &mut mask_map,
            &mut byte_window_size,
            &mut char_window_size,
        )
        .unwrap();
        (replaced, mask_map, match_count)
    }

    #[test]
    fn replacement_expands_captures_for_each_match_context() {
        let rule = rule(
            r"(?<=(?<context>[ab]))x",
            Type::Replace,
            false,
            "${context}-x",
        );

        let (replaced, _, match_count) = replace(&rule, "ax bx");

        assert_eq!(replaced, "aa-x bb-x");
        assert_eq!(match_count, 2);
    }

    #[test]
    fn repeated_hash_keeps_restore_mapping() {
        let rule = rule(r"\d{8}", Type::Hash, true, "");
        let original = "12345678 12345678";

        let (masked, mask_map, match_count) = replace(&rule, original);

        assert_eq!(match_count, 2);
        assert_eq!(mask_map.len(), 1);
        assert_eq!(
            mask_map.values().next().unwrap().original.as_deref(),
            Some("12345678")
        );
        assert_eq!(mask_map.values().next().unwrap().kind, RestoreKind::Hash);
        let mask_restore = MaskRestore::from_map(mask_map).unwrap().unwrap();
        assert_eq!(mask_restore.restore(&masked), original);

        let mut collision_map = HashMap::new();
        record_restore_mapping(
            &mut collision_map,
            "token".to_string(),
            "12345678",
            RestoreKind::Hash,
        );
        record_restore_mapping(
            &mut collision_map,
            "token".to_string(),
            "12345678",
            RestoreKind::Hash,
        );
        assert_eq!(
            collision_map.get("token").unwrap().original.as_deref(),
            Some("12345678")
        );
        record_restore_mapping(
            &mut collision_map,
            "token".to_string(),
            "87654321",
            RestoreKind::General,
        );
        assert!(collision_map.get("token").unwrap().original.is_none());
    }

    #[test]
    fn restores_more_than_five_thousand_unique_values() {
        let mask_map: HashMap<_, _> = (0..6_001)
            .map(|index| {
                (
                    format!("{index:064x}"),
                    RestoreEntry {
                        original: Some(format!("secret-{index:06}")),
                        kind: RestoreKind::Hash,
                    },
                )
            })
            .collect();
        let masked = (0..6_001)
            .map(|index| format!("{index:064x}"))
            .collect::<Vec<_>>()
            .join("|");
        let expected = (0..6_001)
            .map(|index| format!("secret-{index:06}"))
            .collect::<Vec<_>>()
            .join("|");

        let mask_restore = MaskRestore::from_map(mask_map).unwrap().unwrap();

        assert!(mask_restore.general.is_none());
        assert_eq!(mask_restore.hashes.len(), 6_001);
        assert_eq!(mask_restore.restore(&masked), expected);
    }

    #[test]
    fn regex_runtime_error_is_returned_without_retrying() {
        let regex = fancy_regex::RegexBuilder::new("(x+x+)+(?>y)")
            .backtrack_limit(1)
            .build()
            .unwrap();
        let rule = Rule {
            regex,
            type_: Type::Replace,
            restore: false,
            value: "masked".to_string(),
        };
        let mut mask_map = HashMap::new();
        let mut byte_window_size = 0;
        let mut char_window_size = 0;
        let started = Instant::now();

        let result = replace_rule_message(
            &rule,
            "xxxxxxxxxxy",
            &mut mask_map,
            &mut byte_window_size,
            &mut char_window_size,
        );

        assert!(matches!(
            result,
            Err(fancy_regex::Error::RuntimeError(
                fancy_regex::RuntimeError::BacktrackLimitExceeded
            ))
        ));
        assert!(started.elapsed() < Duration::from_secs(5));
    }

    #[test]
    fn general_restore_limit_is_explicit() {
        let mask_map: HashMap<_, _> = (0..=MAX_GENERAL_RESTORE_PATTERNS)
            .map(|index| {
                (
                    format!("mask-{index}"),
                    RestoreEntry {
                        original: Some(format!("secret-{index}")),
                        kind: RestoreKind::General,
                    },
                )
            })
            .collect();

        assert!(matches!(
            MaskRestore::from_map(mask_map),
            Err(RestoreBuildError::TooManyGeneralPatterns(count))
                if count == MAX_GENERAL_RESTORE_PATTERNS + 1
        ));

        let mask_map = HashMap::from([(
            "x".repeat(MAX_GENERAL_RESTORE_PATTERN_BYTES + 1),
            RestoreEntry {
                original: Some("secret".to_string()),
                kind: RestoreKind::General,
            },
        )]);
        assert!(matches!(
            MaskRestore::from_map(mask_map),
            Err(RestoreBuildError::GeneralPatternBytesExceeded(bytes))
                if bytes == MAX_GENERAL_RESTORE_PATTERN_BYTES + 1
        ));
    }

    #[test]
    fn hash_and_general_matches_use_global_leftmost_longest_order() {
        let hash = "a".repeat(HASH_MASK_BYTES);
        let longer_general = "a".repeat(HASH_MASK_BYTES + 1);
        let later_general = format!("{}Z", "a".repeat(HASH_MASK_BYTES - 1));
        let earlier_general = format!("g{}", "a".repeat(HASH_MASK_BYTES / 2));
        let mut mask_map = HashMap::new();
        for (masked, original, kind) in [
            (hash.clone(), "HASH", RestoreKind::Hash),
            (longer_general.clone(), "LONG", RestoreKind::General),
            (later_general, "LATER", RestoreKind::General),
            (earlier_general.clone(), "EARLIER", RestoreKind::General),
        ] {
            record_restore_mapping(&mut mask_map, masked, original, kind);
        }
        let message = format!(
            "{}|{}Z|{}{}",
            longer_general,
            hash,
            earlier_general,
            "a".repeat(HASH_MASK_BYTES / 2)
        );
        let expected = format!("LONG|HASHZ|EARLIER{}", "a".repeat(HASH_MASK_BYTES / 2));

        let mask_restore = MaskRestore::from_map(mask_map).unwrap().unwrap();

        assert_eq!(mask_restore.restore(&message), expected);
    }

    #[test]
    fn tool_restore_requires_an_explicit_allowlist() {
        let config = json!({"tool_arguments": {"restore": true}});
        assert!(
            serde_json::from_value::<AiDataMaskingConfig>(config).is_err(),
            "enabling tool restoration without an allowlist must reject configuration"
        );
    }

    #[test]
    fn tool_restore_rejects_invalid_or_ambiguous_allowlists() {
        for entries in [
            json!([{"tool": "", "paths": ["/email"]}]),
            json!([{"tool": "lookup", "paths": []}]),
            json!([{"tool": "lookup", "paths": ["#/email"]}]),
            json!([{"tool": "lookup", "paths": ["/bad~2escape"]}]),
            json!([{"tool": "lookup", "paths": ["/email", "/email"]}]),
            json!([
                {"tool": "lookup", "paths": ["/email"]},
                {"tool": "lookup", "paths": ["/name"]}
            ]),
        ] {
            assert!(
                serde_json::from_value::<AiDataMaskingConfig>(json!({
                    "tool_arguments": {"restore": true, "restore_allowlist": entries}
                }))
                .is_err(),
                "invalid or ambiguous allowlist must reject configuration"
            );
        }
    }

    #[test]
    fn tool_limits_are_positive_integers_only_when_arguments_are_enabled() {
        for value in [json!(0), json!(-1), json!(1.5), json!("10"), json!(null)] {
            for check in [false, true] {
                let config = serde_json::from_value::<AiDataMaskingConfig>(json!({
                    "tool_arguments": {
                        "check": check,
                        "limits": {"max_call_bytes": value}
                    }
                }));
                assert_eq!(config.is_err(), check);
            }
        }
    }

    #[test]
    fn tool_configuration_update_is_all_or_nothing() {
        let mut matcher = RuleMatcher::<AiDataMaskingConfig>::default();
        install_tool_configuration(&mut matcher, &json!({"tool_arguments": {"check": true}}))
            .unwrap();
        let result = install_tool_configuration(
            &mut matcher,
            &json!({
                "tool_arguments": {"restore": true},
                "_rules_": [{"_match_route_": ["example"], "tool_arguments": {"check": true}}]
            }),
        );
        assert!(
            result.is_err(),
            "a valid rule must not hide invalid global configuration"
        );

        let result = install_tool_configuration(
            &mut matcher,
            &json!({
                "tool_arguments": {"check": false},
                "_rules_": [{"_match_route_": ["example"], "deny_code": "invalid"}]
            }),
        );
        assert!(result.is_err());
        matcher.rewrite_config(|config| {
            assert!(
                config.tool_arguments.check,
                "the previous check must remain installed"
            );
            config.clone()
        });
    }

    #[test]
    fn tool_switches_are_independent_and_inactive_dependencies_are_ignored() {
        let default: AiDataMaskingConfig = serde_json::from_value(json!({})).unwrap();
        assert!(!default.tool_arguments.check);
        assert!(!default.tool_arguments.restore);
        assert!(!default.tool_definitions.check);
        for flags in 0..8 {
            let value = json!({
                "tool_arguments": {
                    "check": flags & 1 != 0,
                    "restore": flags & 2 != 0,
                    "restore_allowlist": [{"tool": "Lookup", "paths": ["", "/a~1b", "/~0", "/01", "/-"]}],
                    "limits": {"max_call_bytes": 123, "max_buffer_bytes": 456, "max_calls": 7}
                },
                "tool_definitions": {"check": flags & 4 != 0}
            });
            let config: AiDataMaskingConfig = serde_json::from_value(value.clone()).unwrap();
            assert_eq!(config.tool_arguments.check, flags & 1 != 0);
            assert_eq!(config.tool_arguments.restore, flags & 2 != 0);
            assert_eq!(config.tool_definitions.check, flags & 4 != 0);
            assert_eq!(requests_tool_processing(&value), flags != 0);
            if flags & 3 != 0 {
                assert_eq!(config.tool_arguments.limits.max_call_bytes, 123);
                assert_eq!(config.tool_arguments.limits.max_buffer_bytes, 456);
                assert_eq!(config.tool_arguments.limits.max_calls, 7);
            }
            assert_eq!(
                config.tool_arguments.restore_allowlist.len(),
                usize::from(flags & 2 != 0)
            );
        }
        assert!(serde_json::from_value::<AiDataMaskingConfig>(json!({
            "tool_arguments": {
                "check": false, "restore": false,
                "limits": "ignored", "restore_allowlist": "ignored"
            },
            "tool_definitions": {"check": true}
        }))
        .is_ok());
    }

    #[test]
    fn tool_request_masks_once_with_raw_and_argument_jsonpath_rules() {
        let input = json!({
            "stream": true, "copy": "SECRET",
            "messages": [{"role": "assistant", "content": null, "tool_calls": [{
                "function": {"name": "lookup", "arguments": "{ \"email\":\"SECRET\", \"n\":1e2 }"}
            }]}]
        });
        let hash =
            hmac_sha256::Hash::hash(b"SECRET")
                .iter()
                .fold(String::new(), |mut text, byte| {
                    let _ = write!(text, "{byte:02x}");
                    text
                });
        for raw in [true, false] {
            let config: AiDataMaskingConfig = serde_json::from_value(json!({
                "deny_openai": false,
                "deny_raw": raw,
                "deny_jsonpath": if raw { json!([]) } else { json!(["$.messages[*].tool_calls[*].function.arguments"]) },
                "tool_arguments": {"check": true},
                "replace_roles": [{"regex": "SECRET|[a-f0-9]{64}", "type": "hash", "restore": true}]
            })).unwrap();
            let result = prepare_tool_request(
                &config,
                input.to_string().as_bytes(),
                &Log::new("test".to_string()),
            )
            .unwrap()
            .unwrap();
            let body: Value = serde_json::from_str(result.body.as_ref().unwrap()).unwrap();
            let arguments = body["messages"][0]["tool_calls"][0]["function"]["arguments"]
                .as_str()
                .unwrap();
            assert_eq!(
                serde_json::from_str::<Value>(arguments).unwrap()["email"],
                hash
            );
            assert!(arguments.contains("1e2"));
            assert_eq!(result.tool_map[&hash].original.as_deref(), Some("SECRET"));
            assert_eq!(result.tool_map.len(), 1);
            assert_eq!(body["copy"], if raw { hash.as_str() } else { "SECRET" });
            assert_eq!(result.legacy_map.contains_key(&hash), raw);
            assert!(!result.is_openai);
            assert!(!result.legacy_stream);
            assert!(result.tool_stream);
        }
    }

    #[test]
    fn tool_restore_shares_maps_in_one_direction_and_rejects_collisions() {
        let mut legacy = HashMap::new();
        let mut tools = HashMap::new();
        record_restore_mapping(&mut legacy, "P".to_string(), "A", RestoreKind::General);
        record_restore_mapping(&mut tools, "A".to_string(), "SECRET", RestoreKind::General);
        record_restore_mapping(&mut legacy, "C".to_string(), "first", RestoreKind::General);
        record_restore_mapping(&mut tools, "C".to_string(), "second", RestoreKind::General);
        let restore = ToolRestore::from_maps(&legacy, &tools).unwrap();
        assert_eq!(
            restore.restore("P A", 1024).unwrap().as_deref(),
            Some("A SECRET")
        );
        assert_eq!(restore.restore("P", 1024).unwrap().as_deref(), Some("A"));
        assert!(restore.restore("C", 1024).is_err());
        assert_eq!(restore.restore("unchanged", 1024).unwrap(), None);
        assert!(restore.restore("A", 2).is_err());
        assert_eq!(
            MaskRestore::from_map(legacy).unwrap().unwrap().restore("A"),
            "A"
        );
    }

    #[test]
    fn opt_in_arguments_enter_the_request_callback_with_legacy_openai_disabled() {
        crate::test_host::reset();
        let config = serde_json::from_value(json!({
            "deny_openai": false,
            "tool_arguments": {"check": true},
            "replace_roles": [{"regex": "SECRET", "type": "hash", "restore": true}]
        }))
        .unwrap();
        let mut plugin = AiDataMasking::new();
        plugin.on_config(Rc::new(config));
        let request = json!({"messages": [{"role": "assistant", "content": null, "tool_calls": [{
            "function": {"name": "lookup", "arguments": "{\"email\":\"\\u0053ECRET\"}"}
        }]}]})
        .to_string()
        .into_bytes();
        assert_eq!(
            plugin.on_http_request_complete_body(&request),
            DataAction::Continue
        );
        let written =
            crate::test_host::take_request_body().expect("the opted-in argument must be masked");
        let value: Value = serde_json::from_slice(&written).unwrap();
        let arguments: Value = serde_json::from_str(
            value["messages"][0]["tool_calls"][0]["function"]["arguments"]
                .as_str()
                .unwrap(),
        )
        .unwrap();
        assert_ne!(arguments["email"], "SECRET");
        assert_eq!(arguments["email"].as_str().unwrap().len(), HASH_MASK_BYTES);
        assert!(!plugin.is_openai);
        assert!(
            plugin.mask_restore.is_none(),
            "new parameter maps must not enter old content restore"
        );
    }

    #[test]
    fn request_callback_keeps_all_eight_tool_switch_combinations_separate() {
        let request = json!({
            "tools": [{"function": {"name": "lookup", "description": "BLOCKED"}}],
            "messages": [{"role": "assistant", "content": null, "tool_calls": [{
                "function": {"name": "lookup", "arguments": "{\"email\":\"SECRET\"}"}
            }]}]
        })
        .to_string()
        .into_bytes();
        for flags in 0..8 {
            crate::test_host::reset();
            let config = serde_json::from_value(json!({
                "deny_openai": false, "deny_words": ["BLOCKED"],
                "tool_arguments": {
                    "check": flags & 1 != 0, "restore": flags & 2 != 0,
                    "restore_allowlist": [{"tool": "lookup", "paths": ["/email"]}]
                },
                "tool_definitions": {"check": flags & 4 != 0},
                "replace_roles": [{"regex": "SECRET", "type": "hash", "restore": true}]
            }))
            .unwrap();
            let mut plugin = AiDataMasking::new();
            plugin.on_config(Rc::new(config));
            let result = plugin.on_http_request_complete_body(&request);
            if flags & 4 != 0 {
                assert_eq!(result, DataAction::StopIterationAndBuffer);
                assert_eq!(crate::test_host::last_rejection().unwrap().0, 200);
            } else {
                assert_eq!(result, DataAction::Continue);
                assert!(crate::test_host::last_rejection().is_none());
                let written =
                    crate::test_host::take_request_body().unwrap_or_else(|| request.clone());
                let value: Value = serde_json::from_slice(&written).unwrap();
                let arguments: Value = serde_json::from_str(
                    value["messages"][0]["tool_calls"][0]["function"]["arguments"]
                        .as_str()
                        .unwrap(),
                )
                .unwrap();
                assert_eq!(arguments["email"] != "SECRET", flags & 1 != 0);
                assert_eq!(value["tools"][0]["function"]["description"], "BLOCKED");
            }
        }
    }

    #[test]
    fn request_history_does_not_use_response_buffer_or_call_limits() {
        let arguments = json!({"email": "SECRET", "padding": "x".repeat(35_000)}).to_string();
        let messages: Vec<_> = (0..129).map(|_| json!({"content": null, "tool_calls": [{"function": {"name": "lookup", "arguments": arguments}}]})).collect();
        let request = json!({"messages": messages}).to_string();
        assert!(request.len() > 4 * 1024 * 1024);
        let config = serde_json::from_value(json!({
            "deny_openai": false,
            "tool_arguments": {"check": true, "limits": {"max_call_bytes": 100_000, "max_buffer_bytes": 1, "max_calls": 1}},
            "replace_roles": [{"regex": "SECRET", "type": "hash"}]
        })).unwrap();
        let prepared =
            prepare_tool_request(&config, request.as_bytes(), &Log::new("test".to_string()))
                .unwrap()
                .unwrap();
        assert!(prepared.body.unwrap().len() > 4 * 1024 * 1024);
    }

    #[test]
    fn nonstream_callback_restores_only_the_allowed_tool_string_value() {
        crate::test_host::reset();
        let config = serde_json::from_value(json!({
            "deny_openai": false,
            "tool_arguments": {"check": true, "restore": true,
                "restore_allowlist": [{"tool": "lookup", "paths": ["/email"]}]},
            "replace_roles": [{"regex": "SECRET", "type": "hash", "restore": true}]
        }))
        .unwrap();
        let mut plugin = AiDataMasking::new();
        plugin.on_config(Rc::new(config));
        let request = json!({"messages": [{"content": null, "tool_calls": [{"function": {
            "name": "lookup", "arguments": "{\"email\":\"SECRET\"}"
        }}]}]})
        .to_string()
        .into_bytes();
        plugin.on_http_request_complete_body(&request);
        let written: Value =
            serde_json::from_slice(&crate::test_host::take_request_body().unwrap()).unwrap();
        let argument: Value = serde_json::from_str(
            written["messages"][0]["tool_calls"][0]["function"]["arguments"]
                .as_str()
                .unwrap(),
        )
        .unwrap();
        let mask = argument["email"].as_str().unwrap();
        crate::test_host::reset();
        let untouched = format!("not JSON {mask}");
        let arguments = format!("{{ \"email\":\"{mask}\", \"other\":\"{mask}\", \"n\":1e2 }}");
        let response = json!({"choices": [{"index": 0, "message": {"content": mask, "tool_calls": [
            {"id": "one", "function": {"name": "lookup", "arguments": arguments}},
            {"id": "two", "function": {"name": "other", "arguments": untouched}}
        ]}}]})
        .to_string()
        .into_bytes();
        assert_eq!(
            plugin.on_http_response_complete_body(&response),
            DataAction::Continue
        );
        let output = crate::test_host::take_response_body()
            .expect("the whitelisted argument must be restored");
        let value: Value = serde_json::from_slice(&output).unwrap();
        assert_eq!(value["choices"][0]["message"]["content"], mask);
        let text = value["choices"][0]["message"]["tool_calls"][0]["function"]["arguments"]
            .as_str()
            .unwrap();
        let restored: Value = serde_json::from_str(text).unwrap();
        assert_eq!(restored["email"], "SECRET");
        assert_eq!(restored["other"], mask);
        assert!(text.contains("1e2"));
        assert_eq!(
            value["choices"][0]["message"]["tool_calls"][1]["function"]["arguments"],
            untouched
        );
    }

    #[test]
    fn response_callback_respects_all_eight_tool_switch_combinations() {
        for flags in 0..8 {
            crate::test_host::reset();
            let config = serde_json::from_value(json!({
                "deny_words": ["BLOCKED"],
                "tool_arguments": {"check": flags & 1 != 0, "restore": flags & 2 != 0,
                    "restore_allowlist": [{"tool": "lookup", "paths": ["/email"]}]},
                "tool_definitions": {"check": flags & 4 != 0},
                "replace_roles": [{"regex": "SECRET", "type": "hash", "restore": true}]
            }))
            .unwrap();
            let mut plugin = AiDataMasking::new();
            plugin.on_config(Rc::new(config));
            let request = json!({"messages": [{"content": "SECRET"}]})
                .to_string()
                .into_bytes();
            assert_eq!(
                plugin.on_http_request_complete_body(&request),
                DataAction::Continue
            );
            let written: Value =
                serde_json::from_slice(&crate::test_host::take_request_body().unwrap()).unwrap();
            let mask = written["messages"][0]["content"].as_str().unwrap();
            crate::test_host::reset();
            let response = json!({"choices": [{"index": 0, "message": {"content": mask,
                "tool_calls": [{"function": {"name": "lookup", "arguments": json!({"email": mask, "extra": "BLOCKED"}).to_string()}}]
            }}]}).to_string().into_bytes();
            plugin.on_http_response_complete_body(&response);
            let output = crate::test_host::take_response_body().unwrap_or_else(|| response.clone());
            let value: Value = serde_json::from_slice(&output).unwrap();
            if flags & 1 != 0 {
                assert!(value["choices"][0]["message"].get("tool_calls").is_none());
                assert!(plugin.tools.as_ref().unwrap().failed);
                crate::test_host::reset();
                plugin.on_http_response_complete_body(&response);
                assert_eq!(crate::test_host::take_response_body(), Some(Vec::new()));
            } else {
                assert_eq!(value["choices"][0]["message"]["content"], "SECRET");
                let arguments: Value = serde_json::from_str(
                    value["choices"][0]["message"]["tool_calls"][0]["function"]["arguments"]
                        .as_str()
                        .unwrap(),
                )
                .unwrap();
                assert_eq!(
                    arguments["email"],
                    if flags & 2 != 0 { "SECRET" } else { mask }
                );
                assert_eq!(arguments["extra"], "BLOCKED");
            }
        }
    }

    #[test]
    fn old_raw_restore_cannot_change_null_arguments() {
        let config = serde_json::from_value(json!({
            "deny_openai": false, "deny_raw": true,
            "tool_arguments": {"restore": true, "restore_allowlist": [{"tool": "lookup", "paths": ["/email"]}]},
            "replace_roles": [{"regex": "SECRET", "type": "replace", "value": "null", "restore": true}]
        })).unwrap();
        let mut plugin = AiDataMasking::new();
        plugin.on_config(Rc::new(config));
        plugin.on_http_request_complete_body(
            &json!({"messages": [{"content": "SECRET"}]})
                .to_string()
                .into_bytes(),
        );
        crate::test_host::reset();
        plugin.on_http_response_complete_body(&json!({"choices": [{"index": 0, "message": {
            "content": "null", "tool_calls": [{"function": {"name": "lookup", "arguments": null}}]
        }}]}).to_string().into_bytes());
        let output: Value =
            serde_json::from_slice(&crate::test_host::take_response_body().unwrap()).unwrap();
        assert_eq!(output["choices"][0]["message"]["content"], "SECRET");
        assert!(
            output["choices"][0]["message"]["tool_calls"][0]["function"]["arguments"].is_null()
        );
    }

    #[test]
    fn response_limits_ignore_nonparameters_and_apply_before_rewrite_allocation() {
        let config: AiDataMaskingConfig = serde_json::from_value(json!({
            "deny_openai": false, "deny_words": ["BLOCKED"],
            "tool_arguments": {"check": true, "limits": {"max_call_bytes": 2, "max_buffer_bytes": 3, "max_calls": 1}}
        })).unwrap();
        let response = json!({"choices": [{"index": 0, "message": {"content": "x".repeat(100_000),
            "tool_calls": [{"function": {"name": "t", "arguments": "{}"}}]
        }}]})
        .to_string()
        .into_bytes();
        assert!(prepare_tool_response(
            &config,
            &response,
            false,
            None,
            &ToolRestore::default(),
            &Log::new("test".to_string())
        )
        .is_ok());
        let mut calls = serde_json::from_slice::<Value>(&response).unwrap();
        calls["choices"][0]["message"]["tool_calls"]
            .as_array_mut()
            .unwrap()
            .push(json!({"function": {"name": "t", "arguments": "{}"}}));
        let mut call_config = config.clone();
        call_config.tool_arguments.limits.max_buffer_bytes = 1000;
        assert!(prepare_tool_response(
            &call_config,
            calls.to_string().as_bytes(),
            false,
            None,
            &ToolRestore::default(),
            &Log::new("test".to_string())
        )
        .is_err());
        let mut map = HashMap::new();
        record_restore_mapping(&mut map, "H".to_string(), "SECRET", RestoreKind::General);
        let restore = ToolRestore::from_maps(&map, &HashMap::new()).unwrap();
        let mut restore_config: AiDataMaskingConfig = serde_json::from_value(json!({"deny_openai": false,
            "tool_arguments": {"restore": true, "restore_allowlist": [{"tool": "t", "paths": ["/x"]}],
                "limits": {"max_call_bytes": 1000, "max_buffer_bytes": 20, "max_calls": 1}}
        })).unwrap();
        let response = json!({"choices": [{"index": 0, "message": {"tool_calls": [{"function": {"name": "t", "arguments": "{\"x\":\"H\"}"}}]}}]}).to_string();
        assert!(prepare_tool_response(
            &restore_config,
            response.as_bytes(),
            false,
            None,
            &restore,
            &Log::new("test".to_string())
        )
        .is_err());
        restore_config.tool_arguments.limits.max_buffer_bytes = 30;
        assert!(prepare_tool_response(
            &restore_config,
            response.as_bytes(),
            false,
            None,
            &restore,
            &Log::new("test".to_string())
        )
        .is_ok());
    }

    #[test]
    fn raw_rules_crossing_arguments_are_rejected_instead_of_truncated() {
        let config = serde_json::from_value(json!({"deny_openai": false, "deny_raw": true,
            "tool_arguments": {"check": true},
            "replace_roles": [{"regex": "\"arguments\":\"", "type": "replace", "value": "bad"}]
        }))
        .unwrap();
        let request =
            json!({"messages": [{"tool_calls": [{"function": {"arguments": "{}"}}]}]}).to_string();
        assert!(
            prepare_tool_request(&config, request.as_bytes(), &Log::new("test".to_string()))
                .is_err()
        );
    }

    #[test]
    fn tool_literal_masking_limits_expansion_and_keeps_capture_semantics() {
        let mut config: AiDataMaskingConfig = serde_json::from_value(json!({
            "tool_arguments": {"check": true},
            "replace_roles": [{"regex": "(?P<name>SECRET)-(\\d+)", "type": "replace",
                "value": "${name}-$2-$$-${missing}", "restore": true}]
        }))
        .unwrap();
        let input = "prefix SECRET-12 suffix";
        let expected = RequestMasks::default().plain(&config, input).unwrap();
        assert_eq!(
            RequestMasks::default()
                .plain_bounded(&config, input, 1024)
                .unwrap(),
            expected
        );
        config.replace_roles[0].value = "$0".repeat(100_000);
        assert!(RequestMasks::default()
            .plain_bounded(&config, input, 64)
            .is_err());
    }

    #[test]
    fn masks_forty_five_thousand_unique_values_without_per_word_replacement() {
        let rule = rule(r"\d{11}", Type::Hash, true, "");
        let message = (0..45_000)
            .map(|index| format!("{index:011}"))
            .collect::<Vec<_>>()
            .join(",");
        let started = Instant::now();

        let (masked, mask_map, match_count) = replace(&rule, &message);
        let mask_count = mask_map.len();
        let mask_restore = MaskRestore::from_map(mask_map).unwrap().unwrap();
        let restored = mask_restore.restore(&masked);

        assert_eq!(match_count, 45_000);
        assert_eq!(mask_count, 45_000);
        assert!(mask_restore.general.is_none());
        assert_eq!(mask_restore.hashes.len(), 45_000);
        assert_eq!(masked.len(), 45_000 * 64 + 44_999);
        assert_eq!(restored, message);
        assert!(
            started.elapsed() < Duration::from_secs(30),
            "45,000-value regression guard exceeded its intentionally broad limit"
        );
    }

    #[test]
    fn streaming_callback_runs_without_legacy_openai_and_keeps_failure_state() {
        for size in [1, 17, usize::MAX] {
            crate::test_host::reset();
            let config = serde_json::from_value(json!({"deny_openai":false,
                "tool_arguments":{"check":true,"restore":true,"restore_allowlist":[{"tool":"lookup","paths":["/email"]}]},
                "replace_roles":[{"regex":"SECRET","type":"hash","restore":true}]})).unwrap();
            let mut plugin = AiDataMasking::new();
            plugin.on_config(Rc::new(config));
            let request = json!({"stream":true,"messages":[{"content":null,"tool_calls":[{"function":{"arguments":"{\"email\":\"SECRET\"}"}}]}]}).to_string().into_bytes();
            plugin.on_http_request_complete_body(&request);
            assert!(!plugin.is_openai);
            assert!(
                !plugin.cache_response_body(),
                "tools SSE must not depend on the legacy stream flag"
            );
            let request: Value =
                serde_json::from_slice(&crate::test_host::take_request_body().unwrap()).unwrap();
            let argument: Value = serde_json::from_str(
                request["messages"][0]["tool_calls"][0]["function"]["arguments"]
                    .as_str()
                    .unwrap(),
            )
            .unwrap();
            let first = json!({"choices":[{"index":0,"delta":{"content":"MASK stays text","tool_calls":[{"index":0,"id":"one","function":{"name":"lookup","arguments":argument.to_string()}}]}}]});
            let last = json!({"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]});
            let wire = format!("data: {first}\n\ndata: {last}\n\ndata: [DONE]\n\n");
            let mut output = Vec::new();
            for chunk in wire.as_bytes().chunks(size) {
                crate::test_host::reset();
                crate::test_host::response_input(chunk);
                plugin.on_http_response_body(chunk.len(), false);
                output.extend(
                    crate::test_host::take_response_body().expect("tool stream callback output"),
                );
            }
            crate::test_host::response_input(&[]);
            plugin.on_http_response_body(0, true);
            output.extend(crate::test_host::take_response_body().unwrap());
            let text = String::from_utf8(output).unwrap();
            let events: Vec<Value> = text
                .split("\n\n")
                .filter_map(|event| event.strip_prefix("data: "))
                .filter_map(|data| serde_json::from_str(data).ok())
                .collect();
            let arguments: String = events
                .iter()
                .filter_map(|value| {
                    value["choices"][0]["delta"]["tool_calls"][0]["function"]["arguments"].as_str()
                })
                .collect();
            assert_eq!(
                serde_json::from_str::<Value>(&arguments).unwrap()["email"],
                "SECRET"
            );
            assert!(
                text.find("SECRET").unwrap()
                    < text.find("\"finish_reason\":\"tool_calls\"").unwrap()
            );
            assert_eq!(text.matches("[DONE]").count(), 1);
        }
        let config = serde_json::from_value(
            json!({"deny_openai":false,"deny_words":["BLOCKED"],"tool_arguments":{"check":true}}),
        )
        .unwrap();
        let mut plugin = AiDataMasking::new();
        plugin.on_config(Rc::new(config));
        plugin.on_http_request_complete_body(
            &json!({"stream":true,"messages":[]})
                .to_string()
                .into_bytes(),
        );
        for input in [b"data: {bad}\n\n".as_slice(), b"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"later\"}}]}\n\ndata: [DONE]\n\n"] {
            crate::test_host::reset();
            crate::test_host::response_input(input);
            plugin.on_http_response_body(input.len(), false);
            assert_eq!(crate::test_host::take_response_body().unwrap(), b"");
        }
        assert!(plugin.tools.as_ref().unwrap().failed);
    }

    #[test]
    fn raw_stream_callback_restores_content_and_keeps_tool_arguments_masked() {
        for size in [1, 17, usize::MAX] {
            crate::test_host::reset();
            let config=serde_json::from_value(json!({"deny_openai":false,"deny_raw":true,"tool_arguments":{"check":true},"replace_roles":[{"regex":"SECRET","type":"hash","restore":true}]})).unwrap();
            let mut plugin = AiDataMasking::new();
            plugin.on_config(Rc::new(config));
            plugin.on_http_request_complete_body(
                &json!({"stream":true,"messages":[{"content":"SECRET"}]})
                    .to_string()
                    .into_bytes(),
            );
            let request: Value =
                serde_json::from_slice(&crate::test_host::take_request_body().unwrap()).unwrap();
            let mask = request["messages"][0]["content"].as_str().unwrap();
            let arguments = json!({"email":mask}).to_string();
            let first = json!({"choices":[{"index":0,"delta":{"content":mask,"tool_calls":[{"index":0,"function":{"name":"lookup","arguments":arguments}}]}}]});
            let last = json!({"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]});
            let wire = format!("data: {first}\n\ndata: {last}\n\ndata: [DONE]\n\n");
            let mut output = Vec::new();
            for chunk in wire.as_bytes().chunks(size) {
                crate::test_host::reset();
                crate::test_host::response_input(chunk);
                plugin.on_http_response_body(chunk.len(), false);
                output.extend(crate::test_host::take_response_body().unwrap());
            }
            crate::test_host::reset();
            plugin.on_http_response_body(0, true);
            output.extend(crate::test_host::take_response_body().unwrap());
            let text = String::from_utf8(output).unwrap();
            let first: Value = serde_json::from_str(
                text.split("\n\n")
                    .next()
                    .unwrap()
                    .strip_prefix("data: ")
                    .unwrap(),
            )
            .unwrap();
            assert_eq!(first["choices"][0]["delta"]["content"], "SECRET");
            assert_eq!(
                first["choices"][0]["delta"]["tool_calls"][0]["function"]["arguments"],
                arguments
            );
        }
    }

    #[test]
    fn stream_request_accepts_a_json_error_response_without_content_type() {
        let config = serde_json::from_value(
            json!({"deny_openai":false,"tool_arguments":{"check":true},"deny_words":["BLOCKED"]}),
        )
        .unwrap();
        let mut plugin = AiDataMasking::new();
        plugin.on_config(Rc::new(config));
        plugin.on_http_request_complete_body(
            &json!({"stream":true,"messages":[]})
                .to_string()
                .into_bytes(),
        );
        let input = b"{\"error\":{\"message\":\"upstream unavailable\"}}";
        let mut output = Vec::new();
        for (position, chunk) in input.chunks(1).enumerate() {
            crate::test_host::reset();
            crate::test_host::response_input(chunk);
            plugin.on_http_response_body(chunk.len(), position + 1 == input.len());
            output.extend(crate::test_host::take_response_body().unwrap());
        }
        assert_eq!(
            serde_json::from_slice::<Value>(&output).unwrap(),
            serde_json::from_slice::<Value>(input).unwrap()
        );
        assert_eq!(plugin.tools.as_ref().unwrap().response_stream, Some(false));
    }
    #[test]
    fn terminal_tool_frame_flushes_checked_text_before_arguments_and_finish() {
        let config: AiDataMaskingConfig=serde_json::from_value(json!({"tool_arguments":{"restore":true,"restore_allowlist":[{"tool":"lookup","paths":["/email"]}]},"replace_roles":[{"regex":"SECRET","type":"replace","value":"MASK","restore":true}]})).unwrap();
        for size in [1, 17, usize::MAX] {
            let mut plugin = AiDataMasking::new();
            plugin.on_config(Rc::new(config.clone()));
            crate::test_host::reset();
            plugin.on_http_request_complete_body(
                &json!({"stream":true,"messages":[{"role":"user","content":"SECRET"}]})
                    .to_string()
                    .into_bytes(),
            );
            let first = json!({"choices":[{"index":0,"delta":{"content":"MA","tool_calls":[{"index":0,"id":"one","function":{"name":"lookup","arguments":"{\"email\":"}}]}}]});
            let last = json!({"usage":{"total_tokens":2},"custom_top":"once","choices":[{"index":0,"delta":{"content":"SK","tool_calls":[{"index":0,"function":{"arguments":"\"MASK\"}"}}]},"finish_reason":""}]});
            let wire = format!("data: {first}\n\ndata: {last}");
            let mut output = Vec::new();
            for chunk in wire.as_bytes().chunks(size) {
                crate::test_host::reset();
                crate::test_host::response_input(chunk);
                plugin.on_http_response_body(chunk.len(), false);
                output.extend(crate::test_host::take_response_body().unwrap());
            }
            crate::test_host::reset();
            plugin.on_http_response_body(0, true);
            output.extend(crate::test_host::take_response_body().unwrap());
            let text = String::from_utf8(output).unwrap();
            let events: Vec<Value> = text
                .split("\n\n")
                .filter_map(|line| line.strip_prefix("data: "))
                .filter_map(|data| serde_json::from_str(data).ok())
                .collect();
            let content: Vec<_> = events
                .iter()
                .enumerate()
                .filter(|(_, event)| {
                    event["choices"][0]["delta"]["content"].as_str() == Some("SECRET")
                })
                .collect();
            let arguments: Vec<_> = events
                .iter()
                .enumerate()
                .filter(|(_, event)| {
                    event["choices"][0]["delta"]["tool_calls"][0]["function"]
                        .get("arguments")
                        .is_some()
                })
                .collect();
            let finish = events
                .iter()
                .position(|event| event["choices"][0].get("finish_reason") == Some(&json!("")))
                .unwrap();
            assert_eq!(content.len(), 1);
            assert_eq!(arguments.len(), 1);
            assert!(content[0].0 < arguments[0].0 && arguments[0].0 < finish);
            assert_eq!(
                events
                    .iter()
                    .filter(|event| event.get("usage").is_some())
                    .count(),
                1
            );
            assert_eq!(
                events
                    .iter()
                    .filter(|event| event.get("custom_top").is_some())
                    .count(),
                1
            );
            assert_eq!(text.matches("[DONE]").count(), 1);
            assert!(text.ends_with("data: [DONE]\n\n"));
        }
    }

    #[test]
    fn system_deny_enters_only_the_explicit_new_check_scopes() {
        let word = include_str!("../res/sensitive_word_dict.txt")
            .lines()
            .map(str::trim)
            .find(|word| word.chars().count() > 1)
            .unwrap();
        assert!(SYSTEM.deny_word.check(word).is_some());
        for flags in 0..8 {
            let config=serde_json::from_value(json!({"deny_openai":false,"system_deny":true,
                "tool_arguments":{"check":flags&1!=0,"restore":flags&2!=0,"restore_allowlist":[{"tool":"lookup","paths":["/x"]}]},
                "tool_definitions":{"check":flags&4!=0}})).unwrap();
            let request=json!({"messages":[{"content":null,"tool_calls":[{"function":{"arguments":json!({"x":word}).to_string()}}]}],"tools":[{"function":{"name":"lookup","description":word}}]}).to_string();
            assert_eq!(
                prepare_tool_request(&config, request.as_bytes(), &Log::new("test".into()))
                    .is_err(),
                flags & 5 != 0,
                "flags {flags}"
            );
        }
    }

    #[test]
    fn ambiguous_hash_restore_preserves_existing_substring_matching() {
        let mask = "a".repeat(64);
        let mut map = HashMap::new();
        record_restore_mapping(&mut map, mask.clone(), "one", RestoreKind::Hash);
        record_restore_mapping(&mut map, mask.clone(), "two", RestoreKind::Hash);
        let restore = ToolRestore::from_maps(&map, &HashMap::new()).unwrap();
        // Existing hash restoration also matches inside a longer hex string.
        assert!(restore.restore(&format!("b{mask}c"), 1000).is_err());
        assert!(restore.restore(&mask, 1000).is_err());
    }

    #[test]
    fn tool_generated_maps_do_not_expand_legacy_content_restoration() {
        crate::test_host::reset();
        let config=serde_json::from_value(json!({"tool_arguments":{"check":true,"restore":true,"restore_allowlist":[{"tool":"lookup","paths":["/email"]}]},"replace_roles":[{"regex":"SECRET","type":"hash","restore":true}]})).unwrap();
        let mut plugin = AiDataMasking::new();
        plugin.on_config(Rc::new(config));
        plugin.on_http_request_complete_body(&json!({"messages":[{"role":"assistant","content":"","tool_calls":[{"function":{"name":"lookup","arguments":"{\"email\":\"SECRET\"}"}}]}]}).to_string().into_bytes());
        assert!(plugin.is_openai);
        assert!(plugin.mask_restore.is_none());
        let request: Value =
            serde_json::from_slice(&crate::test_host::take_request_body().unwrap()).unwrap();
        let args = request["messages"][0]["tool_calls"][0]["function"]["arguments"]
            .as_str()
            .unwrap();
        let mask: Value = serde_json::from_str(args).unwrap();
        crate::test_host::reset();
        plugin.on_http_response_complete_body(&json!({"choices":[{"index":0,"message":{"content":mask["email"],"tool_calls":[{"function":{"name":"lookup","arguments":args}}]}}]}).to_string().into_bytes());
        let output: Value =
            serde_json::from_slice(&crate::test_host::take_response_body().unwrap()).unwrap();
        assert_eq!(output["choices"][0]["message"]["content"], mask["email"]);
        let args: Value = serde_json::from_str(
            output["choices"][0]["message"]["tool_calls"][0]["function"]["arguments"]
                .as_str()
                .unwrap(),
        )
        .unwrap();
        assert_eq!(args["email"], "SECRET");
    }

    #[test]
    fn unterminated_usage_event_is_separated_from_http_end_tool_arguments() {
        crate::test_host::reset();
        let config=serde_json::from_value(json!({"tool_arguments":{"restore":true,"restore_allowlist":[{"tool":"lookup","paths":["/email"]}]},"replace_roles":[{"regex":"SECRET","type":"replace","value":"MASK","restore":true}]})).unwrap();
        let mut plugin = AiDataMasking::new();
        plugin.on_config(Rc::new(config));
        plugin.on_http_request_complete_body(
            &json!({"stream":true,"messages":[{"content":"SECRET"}]})
                .to_string()
                .into_bytes(),
        );
        let first = json!({"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"lookup","arguments":"{\"email\":\"MASK\"}"}}]}}]});
        let wire =
            format!("data: {first}\n\ndata: {{\"choices\":[],\"usage\":{{\"total_tokens\":2}}}}");
        crate::test_host::reset();
        crate::test_host::response_input(wire.as_bytes());
        plugin.on_http_response_body(wire.len(), true);
        let body = String::from_utf8(crate::test_host::take_response_body().unwrap()).unwrap();
        let events: Vec<Value> = body
            .split("\n\n")
            .filter(|event| !event.is_empty())
            .map(|event| {
                serde_json::from_str(event.strip_prefix("data: ").unwrap())
                    .expect("each output SSE event must be valid JSON")
            })
            .collect();
        assert_eq!(
            events
                .iter()
                .filter(|event| event.get("usage").is_some())
                .count(),
            1
        );
        let arguments: Vec<_> = events
            .iter()
            .filter_map(|event| {
                event["choices"][0]["delta"]["tool_calls"][0]["function"]["arguments"].as_str()
            })
            .collect();
        assert_eq!(arguments, vec!["{\"email\":\"SECRET\"}"]);
        assert!(!body.contains("[DONE]"));
    }
}
