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

use crate::deny_word::DenyWord;
use crate::msg_win_openai::MsgWindow;
use aho_corasick::{AhoCorasick, AhoCorasickBuilder, AhoCorasickKind, Input, MatchKind};
use fancy_regex::Regex;
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
use std::collections::{BTreeMap, HashMap, VecDeque};
use std::fmt::Write;
use std::ops::DerefMut;
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
}
struct AiDataMasking {
    weak: Weak<RefCell<Box<dyn HttpContextWrapper<AiDataMaskingConfig>>>>,
    config: Option<Rc<AiDataMaskingConfig>>,
    mask_map: HashMap<String, RestoreEntry>,
    mask_restore: Option<MaskRestore>,
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

struct RestoreEntry {
    original: Option<String>,
    kind: RestoreKind,
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
        let mut restored = String::with_capacity(message.len());
        let mut last_end = 0;
        let mut next_general = self.next_general_match(message, 0);
        let mut hash_cursor = 0;
        let mut next_hash = next_hash_match(message, &self.hashes, &mut hash_cursor);

        while next_general.is_some() || next_hash.is_some() {
            let use_general = match (&next_general, &next_hash) {
                (Some(general), Some((hash_start, hash_end, _))) => {
                    general.start() < *hash_start
                        || (general.start() == *hash_start
                            && general.end() - general.start() >= hash_end - hash_start)
                }
                (Some(_), None) => true,
                (None, Some(_)) => false,
                (None, None) => break,
            };

            let (start, end, replacement) = if use_general {
                let matched = next_general.take().unwrap();
                let general = self.general.as_ref().unwrap();
                let replacement = &general.replacements[matched.pattern().as_usize()];
                (matched.start(), matched.end(), replacement.as_str())
            } else {
                let (start, end, replacement) = next_hash.take().unwrap();
                (start, end, replacement)
            };

            restored.push_str(&message[last_end..start]);
            restored.push_str(replacement);
            last_end = end;

            if use_general {
                next_general = self.next_general_match(message, last_end);
                if next_hash
                    .as_ref()
                    .is_some_and(|(hash_start, _, _)| *hash_start < last_end)
                {
                    hash_cursor = last_end;
                    next_hash = next_hash_match(message, &self.hashes, &mut hash_cursor);
                }
            } else {
                next_hash = next_hash_match(message, &self.hashes, &mut hash_cursor);
                if next_general
                    .as_ref()
                    .is_some_and(|general| general.start() < last_end)
                {
                    next_general = self.next_general_match(message, last_end);
                }
            }
        }
        if last_end == 0 {
            return message.to_string();
        }
        restored.push_str(&message[last_end..]);
        restored
    }

    fn next_general_match(&self, message: &str, start: usize) -> Option<aho_corasick::Match> {
        self.general.as_ref().and_then(|general| {
            general
                .matcher
                .find(Input::new(message).span(start..message.len()))
        })
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
                "custom deny word {} matched from {}",
                word, message
            ));
            return true;
        } else if self.system_deny {
            if let Some(word) = SYSTEM.deny_word.check(message) {
                log.warn(&format!(
                    "system deny word {} matched from {}",
                    word, message
                ));
                return true;
            }
        }
        false
    }
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
        }
    }
}

impl Context for AiDataMaskingRoot {}

impl RootContext for AiDataMaskingRoot {
    fn on_configure(&mut self, plugin_configuration_size: usize) -> bool {
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
        Some(Box::new(AiDataMasking {
            weak: Weak::default(),
            mask_map: HashMap::new(),
            mask_restore: None,
            config: None,
            is_openai: false,
            is_openai_stream: None,
            stream: false,
            msg_window: MsgWindow::default(),
            log: Log::new(PLUGIN_NAME.to_string()),
            char_window_size: 0,
            byte_window_size: 0,
        }))
    }
}

impl AiDataMasking {
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
        if in_response && self.stream {
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
        HeaderAction::Continue
    }

    fn on_http_response_body(&mut self, body_size: usize, end_of_stream: bool) -> DataAction {
        if !self.stream {
            return DataAction::Continue;
        }
        if body_size > 0 {
            if let Some(body) = self.get_http_response_body(0, body_size) {
                if self.is_openai && self.is_openai_stream.is_none() {
                    self.is_openai_stream = Some(body.starts_with(b"data:"));
                }
                self.msg_window
                    .push(&body, self.is_openai_stream.unwrap_or_default());
                let mut deny = false;
                let log = Log::new(PLUGIN_NAME.to_string());
                let mask_restore = self.mask_restore.as_ref();
                for message in self.msg_window.messages_iter_mut() {
                    if let Ok(mut msg) = String::from_utf8(message.clone()) {
                        if let Some(config) = &self.config {
                            if config.check_message(&msg, &log) {
                                deny = true;
                                break;
                            }
                        }
                        if let Some(mask_restore) = mask_restore {
                            msg = mask_restore.restore(&msg);
                        }
                        message.clear();
                        message.extend_from_slice(msg.as_bytes());
                    }
                }
                if deny {
                    return self.deny(true);
                }
            }
        }
        let new_body = if end_of_stream {
            self.msg_window.finish(self.is_openai_stream.unwrap())
        } else {
            self.msg_window.pop(
                self.char_window_size * 2,
                self.byte_window_size * 2,
                self.is_openai_stream.unwrap(),
            )
        };
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
        !self.stream
    }
    fn on_http_request_complete_body(&mut self, req_body: &Bytes) -> DataAction {
        if self.config.is_none() {
            return DataAction::Continue;
        }
        let config = self.config.as_ref().unwrap();
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
        let config = self.config.as_ref().unwrap();
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
}
