// Copyright (c) 2026 Alibaba Group Holding Ltd.
// SPDX-License-Identifier: Apache-2.0

use super::*;

#[derive(Default)]
struct SseDecoder {
    pending: Vec<u8>,
    scan: usize,
    line_start: usize,
}
impl SseDecoder {
    fn push(&mut self, data: &[u8], end: bool) -> Vec<Vec<u8>> {
        self.pending.extend_from_slice(data);
        let mut events = Vec::new();
        let mut consumed = 0;
        while self.scan < self.pending.len() {
            let position = self.scan;
            let byte = self.pending[position];
            if byte != b'\r' && byte != b'\n' {
                self.scan += 1;
                continue;
            }
            if byte == b'\r' && position + 1 == self.pending.len() && !end {
                break;
            }
            let length = if byte == b'\r' && self.pending.get(position + 1) == Some(&b'\n') {
                2
            } else {
                1
            };
            self.scan += length;
            if position == self.line_start {
                events.push(self.pending[consumed..self.scan].to_vec());
                consumed = self.scan;
            }
            self.line_start = self.scan;
        }
        if consumed != 0 {
            self.pending.drain(..consumed);
            self.scan -= consumed;
            self.line_start -= consumed;
        }
        if end && !self.pending.is_empty() {
            events.push(std::mem::take(&mut self.pending));
            self.scan = 0;
            self.line_start = 0;
        }
        events
    }
}

#[derive(Debug)]
pub(super) enum StreamAction {
    Event { raw: Vec<u8>, payload: Vec<u8> },
    FlushChoice(i64),
    FlushAll,
    Arguments(Value),
}
#[derive(Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash, Debug)]
enum CallIndex {
    Modern(u64),
    Legacy,
}
type CallKey = (i64, CallIndex);

struct CallFragment<'a> {
    choice: i64,
    call: &'a mut Value,
    legacy: bool,
    metadata: &'a serde_json::Map<String, Value>,
}

struct Call {
    name: String,
    id: String,
    arguments: String,
    index: Value,
    metadata: serde_json::Map<String, Value>,
}
impl Call {
    fn retained(&self) -> usize {
        self.name.len() + self.id.len() + self.arguments.len()
    }
}

#[derive(Default)]
pub(super) struct ToolStream {
    parser: SseDecoder,
    calls: BTreeMap<CallKey, Call>,
    seen: HashSet<CallKey>,
    ended: HashSet<CallKey>,
    bypass: HashSet<CallKey>,
    ids: HashMap<[u8; 32], CallKey>,
    retained: usize,
    queued: usize,
    done: bool,
    errored: bool,
    finish_reasons: BTreeMap<i64, bool>,
}

fn payload(raw: &[u8]) -> Vec<u8> {
    let raw = raw.strip_prefix(b"\xef\xbb\xbf").unwrap_or(raw);
    let mut result = Vec::new();
    let mut had_data = false;
    for line in raw.split(|byte| *byte == b'\r' || *byte == b'\n') {
        if let Some(data) = line.strip_prefix(b"data:") {
            if had_data {
                result.push(b'\n');
            }
            result.extend_from_slice(data.strip_prefix(b" ").unwrap_or(data));
            had_data = true;
        } else if line == b"data" {
            if had_data {
                result.push(b'\n');
            }
            had_data = true;
        }
    }
    result
}

fn line_ending_prefix(bytes: &[u8]) -> Option<&[u8]> {
    bytes
        .strip_suffix(b"\r\n")
        .or_else(|| bytes.strip_suffix(b"\n"))
        .or_else(|| bytes.strip_suffix(b"\r"))
}

fn ensure_event_boundary(raw: &mut Vec<u8>) {
    let terminated = line_ending_prefix(raw)
        .is_some_and(|before| before.is_empty() || line_ending_prefix(before).is_some());
    if !terminated {
        raw.extend_from_slice(b"\n\n");
    }
}

fn render_event(raw: &[u8], value: &Value) -> Vec<u8> {
    let mut event = Vec::new();
    for line in raw.split(|byte| *byte == b'\r' || *byte == b'\n') {
        if !line.is_empty() && !line.starts_with(b"data:") && line != b"data" {
            event.extend_from_slice(line);
            event.push(b'\n');
        }
    }
    event.extend_from_slice(b"data: ");
    serde_json::to_writer(&mut event, value).expect("JSON value serialization");
    event.extend_from_slice(b"\n\n");
    event
}

impl ToolStream {
    pub(super) fn release_queued(&mut self) {
        self.retain_queued(0).expect("release queued arguments");
    }
    pub(super) fn retain_queued(&mut self, remaining: usize) -> Result<(), &'static str> {
        if remaining > self.queued {
            return Err("unaccounted queued arguments");
        }
        self.retained -= self.queued - remaining;
        self.queued = remaining;
        Ok(())
    }
    pub(super) fn errored(&self) -> bool {
        self.errored
    }
    pub(super) fn finish_raw(&mut self) -> bool {
        if !self.done
            && !self.errored
            && !self.finish_reasons.is_empty()
            && self.finish_reasons.values().all(|finished| *finished)
        {
            self.done = true;
            true
        } else {
            false
        }
    }

    fn discard_calls(&mut self) {
        self.calls.clear();
        self.retained = self.queued;
    }

    fn key(&self, choice: i64, call: &Value, legacy: bool) -> Result<CallKey, &'static str> {
        if legacy {
            return Ok((choice, CallIndex::Legacy));
        }
        let index = match call.get("index") {
            Some(Value::Number(number)) => number.as_u64().ok_or("invalid stream tool index")?,
            Some(Value::String(text))
                if !text.is_empty() && text.bytes().all(|b| b.is_ascii_digit()) =>
            {
                text.parse::<u64>()
                    .map_err(|_| "invalid stream tool index")?
            }
            None | Some(Value::Null) => {
                let id = call
                    .get("id")
                    .and_then(Value::as_str)
                    .filter(|id| !id.is_empty())
                    .ok_or("unattributed stream tool call")?;
                let key = self
                    .ids
                    .get(&hmac_sha256::Hash::hash(id.as_bytes()))
                    .copied()
                    .ok_or("unknown stream tool id")?;
                if key.0 != choice {
                    return Err("conflicting stream tool id");
                }
                return Ok(key);
            }
            _ => return Err("invalid stream tool index"),
        };
        Ok((choice, CallIndex::Modern(index)))
    }

    fn handle_call(
        &mut self,
        fragment: CallFragment<'_>,
        config: &AiDataMaskingConfig,
        restore: &ToolRestore,
        actions: &mut Vec<StreamAction>,
    ) -> Result<bool, &'static str> {
        let CallFragment {
            choice,
            call,
            legacy,
            metadata,
        } = fragment;
        if !call.is_object() {
            return Err("invalid stream tool call");
        }
        let function = if legacy {
            &*call
        } else {
            match call.get("function") {
                None | Some(Value::Null) => &Value::Null,
                Some(value) => value,
            }
        };
        if !function.is_null() && !function.is_object() {
            return Err("invalid stream tool function");
        }
        let name = match function.get("name") {
            None | Some(Value::Null) => "",
            Some(Value::String(name)) => name.as_str(),
            _ => return Err("invalid stream tool name"),
        };
        let check = check_response_arguments(config);
        let can_restore = config.tool_arguments.restore && restore.has_entries;
        if !check && !can_restore {
            return Ok(false);
        }
        // A full non-prefix on its first frame cannot become an allowed name
        // under the supported delta contract. It needs no new argument budget.
        let key = match self.key(choice, call, legacy) {
            Ok(key) => key,
            Err(_)
                if !check
                    && !name.is_empty()
                    && !config
                        .tool_arguments
                        .restore_allowlist
                        .iter()
                        .any(|entry| entry.tool.starts_with(name)) =>
            {
                return Ok(false)
            }
            Err(reason) => return Err(reason),
        };
        if self.bypass.contains(&key) {
            return Ok(false);
        }
        if !self.calls.contains_key(&key)
            && !check
            && !name.is_empty()
            && !config
                .tool_arguments
                .restore_allowlist
                .iter()
                .any(|entry| entry.tool.starts_with(name))
        {
            self.bypass.insert(key);
            return Ok(false);
        }
        if self.ended.contains(&key) {
            return Err("stream tool call continued after completion");
        }
        if self.done {
            return Err("stream tool call after DONE");
        }
        let id = match call.get("id") {
            None | Some(Value::Null) => "",
            Some(Value::String(id)) => id.as_str(),
            _ => return Err("invalid stream tool id"),
        };
        if !legacy
            && call
                .get("type")
                .filter(|value| !value.is_null())
                .is_some_and(|value| value.as_str() != Some("function"))
        {
            return Err("unsupported stream tool type");
        }
        if !id.is_empty() {
            let hash = hmac_sha256::Hash::hash(id.as_bytes());
            if self.ids.get(&hash).is_some_and(|existing| existing != &key) {
                return Err("conflicting stream tool id");
            }
            if self
                .calls
                .get(&key)
                .is_some_and(|entry| !entry.id.is_empty() && entry.id != id)
            {
                return Err("changed stream tool id");
            }
            self.ids.insert(hash, key);
        }
        let limits = &config.tool_arguments.limits;
        if let std::collections::btree_map::Entry::Vacant(entry) = self.calls.entry(key) {
            if self.seen.len() >= limits.max_calls {
                return Err("response tool call limit exceeded");
            }
            self.seen.insert(key);
            entry.insert(Call {
                name: String::new(),
                id: String::new(),
                arguments: String::new(),
                index: call.get("index").cloned().unwrap_or(Value::Null),
                metadata: metadata.clone(),
            });
        }
        let entry = self.calls.get_mut(&key).ok_or("missing stream tool call")?;
        let id_bytes = if entry.id.is_empty() { id.len() } else { 0 };
        let added = name
            .len()
            .checked_add(id_bytes)
            .ok_or("response tool bytes overflow")?;
        add_response_bytes(&mut self.retained, added, limits.max_buffer_bytes)?;
        entry.name.push_str(name);
        if entry.id.is_empty() {
            entry.id.push_str(id);
        }
        let eligible = check
            || config
                .tool_arguments
                .restore_allowlist
                .iter()
                .any(|allowed| allowed.tool.starts_with(&entry.name));
        if !eligible {
            let entry = self.calls.remove(&key).ok_or("missing stream tool call")?;
            self.retained -= entry.name.len() + entry.id.len();
            self.bypass.insert(key);
            if !entry.arguments.is_empty() {
                actions.push(StreamAction::FlushChoice(choice));
                self.queued += entry.arguments.len();
                actions.push(StreamAction::Arguments(argument_frame(key, entry)));
            }
            return Ok(false);
        }
        let arguments = match function.get("arguments") {
            None | Some(Value::Null) => return Ok(false),
            Some(Value::String(arguments)) if arguments.is_empty() => return Ok(false),
            Some(Value::String(arguments)) => arguments.as_str(),
            _ => return Err("invalid stream arguments type"),
        };
        let length = entry
            .arguments
            .len()
            .checked_add(arguments.len())
            .ok_or("arguments size overflow")?;
        if length > limits.max_call_bytes {
            return Err("arguments input limit exceeded");
        }
        add_response_bytes(&mut self.retained, arguments.len(), limits.max_buffer_bytes)?;
        entry.arguments.push_str(arguments);
        let function = if legacy {
            call
        } else {
            call.get_mut("function").ok_or("missing stream function")?
        };
        function
            .as_object_mut()
            .ok_or("invalid stream function")?
            .remove("arguments");
        Ok(true)
    }

    fn complete_choice(
        &mut self,
        choice: Option<i64>,
        config: &AiDataMaskingConfig,
        restore: &ToolRestore,
        log: &Log,
        actions: &mut Vec<StreamAction>,
    ) -> Result<(), &'static str> {
        let keys: Vec<_> = self
            .calls
            .keys()
            .copied()
            .filter(|key| choice.is_none() || choice == Some(key.0))
            .collect();
        for key in keys {
            let mut entry = self.calls.remove(&key).ok_or("missing stream tool call")?;
            self.ended.insert(key);
            let retained = entry.retained();
            if entry.arguments.is_empty() {
                self.retained -= retained;
                continue;
            }
            if entry.name.is_empty() {
                return Err("missing completed stream tool name");
            }
            let output = inspect_response_argument(
                config,
                restore,
                &entry.name,
                &entry.arguments,
                config.tool_arguments.limits.max_buffer_bytes - self.retained,
                log,
            )?;
            if let Some(output) = output {
                add_response_bytes(
                    &mut self.retained,
                    output.len(),
                    config.tool_arguments.limits.max_buffer_bytes,
                )?;
                self.retained -= entry.arguments.len();
                entry.arguments = output;
            }
            self.retained -= entry.name.len() + entry.id.len();
            self.queued += entry.arguments.len();
            actions.push(StreamAction::FlushChoice(key.0));
            actions.push(StreamAction::Arguments(argument_frame(key, entry)));
        }
        Ok(())
    }

    pub(super) fn push(
        &mut self,
        data: &[u8],
        end: bool,
        config: &AiDataMaskingConfig,
        restore: &ToolRestore,
        log: &Log,
    ) -> Result<Vec<StreamAction>, &'static str> {
        let mut actions = Vec::new();
        for mut raw in self.parser.push(data, end) {
            // A flushed HTTP tail must end before any buffered args we append.
            ensure_event_boundary(&mut raw);
            let data = payload(&raw);
            if data.iter().all(u8::is_ascii_whitespace) {
                actions.push(StreamAction::Event { raw, payload: data });
                continue;
            }
            if data
                .iter()
                .copied()
                .filter(|byte| !byte.is_ascii_whitespace())
                .eq(b"[DONE]".iter().copied())
            {
                if self.done {
                    continue;
                }
                if !self.errored {
                    self.complete_choice(None, config, restore, log, &mut actions)?;
                }
                actions.push(StreamAction::FlushAll);
                actions.push(StreamAction::Event { raw, payload: data });
                self.done = true;
                continue;
            }
            let mut value: Value =
                serde_json::from_slice(&data).map_err(|_| "invalid tool SSE JSON")?;
            if !value.is_object() {
                return Err("invalid tool SSE object");
            }
            if value.get("error").is_some_and(|error| !error.is_null()) {
                self.errored = true;
                self.discard_calls();
                actions.retain(|action| !matches!(action, StreamAction::Arguments(_)));
                self.release_queued();
                actions.push(StreamAction::Event { raw, payload: data });
                continue;
            }
            let mut metadata = serde_json::Map::new();
            for field in ["id", "object", "created", "model", "system_fingerprint"] {
                if let Some(value) = value.get(field) {
                    metadata.insert(field.to_string(), value.clone());
                }
            }
            let choices = value
                .get_mut("choices")
                .and_then(Value::as_array_mut)
                .ok_or("invalid tool SSE choices")?;
            let mut changed = false;
            let mut finishes = Vec::new();
            for choice in choices {
                if !choice.is_object() {
                    return Err("invalid tool SSE choice");
                }
                let has_calls = choice.get("delta").is_some_and(|delta| {
                    ["tool_calls", "function_call"]
                        .iter()
                        .any(|field| delta.get(*field).is_some_and(|value| !value.is_null()))
                });
                let terminal = choice
                    .get("finish_reason")
                    .is_some_and(|reason| !reason.is_null());
                let index = choice
                    .get("index")
                    .and_then(Value::as_i64)
                    .filter(|index| *index >= 0);
                if let Some(index) = index {
                    let finished = self.finish_reasons.entry(index).or_default();
                    *finished |= terminal;
                }
                if has_calls && index.is_none() && !check_response_arguments(config) {
                    let delta = &choice["delta"];
                    let known_nonallowed = |function: &Value| {
                        function
                            .get("name")
                            .and_then(Value::as_str)
                            .is_some_and(|name| {
                                !name.is_empty()
                                    && !config
                                        .tool_arguments
                                        .restore_allowlist
                                        .iter()
                                        .any(|entry| entry.tool.starts_with(name))
                            })
                    };
                    let modern_nonallowed = delta
                        .get("tool_calls")
                        .filter(|calls| !calls.is_null())
                        .map_or(true, |calls| {
                            calls.as_array().is_some_and(|calls| {
                                calls.iter().all(|call| known_nonallowed(&call["function"]))
                            })
                        });
                    let legacy_nonallowed = delta
                        .get("function_call")
                        .filter(|function| !function.is_null())
                        .map_or(true, known_nonallowed);
                    if modern_nonallowed && legacy_nonallowed {
                        continue;
                    }
                }
                if (has_calls || (terminal && !self.calls.is_empty())) && index.is_none() {
                    return Err("invalid stream choice index");
                }
                if let Some(delta) = choice.get_mut("delta").filter(|delta| !delta.is_null()) {
                    if !delta.is_object() {
                        return Err("invalid tool SSE delta");
                    }
                    if let Some(calls) =
                        delta.get_mut("tool_calls").filter(|calls| !calls.is_null())
                    {
                        for call in calls.as_array_mut().ok_or("invalid stream tool calls")? {
                            if self.errored {
                                if let Some(function) =
                                    call.get_mut("function").and_then(Value::as_object_mut)
                                {
                                    changed |= function.remove("arguments").is_some();
                                }
                            } else {
                                changed |= self.handle_call(
                                    CallFragment {
                                        choice: index.ok_or("invalid stream choice index")?,
                                        call,
                                        legacy: false,
                                        metadata: &metadata,
                                    },
                                    config,
                                    restore,
                                    &mut actions,
                                )?;
                            }
                        }
                    }
                    if let Some(function) = delta
                        .get_mut("function_call")
                        .filter(|function| !function.is_null())
                    {
                        if self.errored {
                            if let Some(function) = function.as_object_mut() {
                                changed |= function.remove("arguments").is_some();
                            }
                        } else {
                            changed |= self.handle_call(
                                CallFragment {
                                    choice: index.ok_or("invalid stream choice index")?,
                                    call: function,
                                    legacy: true,
                                    metadata: &metadata,
                                },
                                config,
                                restore,
                                &mut actions,
                            )?;
                        }
                    }
                }
                if terminal {
                    if let Some(index) = index {
                        finishes.push(index);
                    }
                }
            }
            // Queue this frame's text and metadata before releasing complete
            // arguments. The actual terminal frame follows both.
            let mut terminal_event = None;
            if !finishes.is_empty() && !self.errored {
                let mut terminals = Vec::new();
                for choice in value["choices"]
                    .as_array_mut()
                    .ok_or("invalid tool SSE choices")?
                {
                    if choice
                        .get("finish_reason")
                        .is_some_and(|reason| !reason.is_null())
                    {
                        let reason = choice
                            .as_object_mut()
                            .ok_or("invalid tool SSE choice")?
                            .remove("finish_reason")
                            .ok_or("missing finish reason")?;
                        terminals.push(
                            json!({"index":choice["index"],"delta":{},"finish_reason":reason}),
                        );
                    }
                }
                terminal_event = Some(json!({"choices":terminals}));
                changed = true;
            }
            let updated = if changed {
                render_event(&raw, &value)
            } else {
                raw
            };
            let data = if changed {
                serde_json::to_vec(&value).map_err(|_| "SSE encoding failed")?
            } else {
                data
            };
            actions.push(StreamAction::Event {
                raw: updated,
                payload: data,
            });
            for index in finishes {
                if !self.errored {
                    self.complete_choice(Some(index), config, restore, log, &mut actions)?;
                }
            }
            if let Some(value) = terminal_event {
                actions.push(StreamAction::Event {
                    raw: render_event(&[], &value),
                    payload: serde_json::to_vec(&value).map_err(|_| "SSE encoding failed")?,
                });
            }
        }
        if end && !self.errored {
            self.complete_choice(None, config, restore, log, &mut actions)?;
        }
        Ok(actions)
    }
}

fn argument_frame(key: CallKey, mut call: Call) -> Value {
    let delta = match key.1 {
        CallIndex::Modern(_) => {
            json!({"tool_calls":[{"index":call.index,"function":{"arguments":call.arguments}}]})
        }
        CallIndex::Legacy => json!({"function_call":{"arguments":call.arguments}}),
    };
    call.metadata.insert(
        "choices".to_string(),
        json!([{"index":key.0,"delta":delta,"finish_reason":null}]),
    );
    Value::Object(call.metadata)
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn sse_decoder_handles_crlf_fragments_and_unterminated_tail() {
        let input =
            b": heartbeat\r\nevent: message\r\nid: 1\r\ndata: {\"choices\":[]}\r\n\r\ndata: [DONE]";
        for size in [1, 2, 17, input.len()] {
            let mut parser = SseDecoder::default();
            let mut events = Vec::new();
            for chunk in input.chunks(size) {
                events.extend(parser.push(chunk, false));
            }
            assert_eq!(events.len(), 1, "size {size}");
            events.extend(parser.push(&[], true));
            assert_eq!(events.len(), 2);
            assert_eq!(events.concat(), input);
        }
    }

    fn fixture() -> (AiDataMaskingConfig, ToolRestore) {
        let config = serde_json::from_value(json!({"deny_openai":false,"tool_arguments":{
            "restore":true,"restore_allowlist":[{"tool":"lookup_customer","paths":["/email"]}]}}))
        .unwrap();
        let mut map = HashMap::new();
        record_restore_mapping(
            &mut map,
            "MASK".to_string(),
            "alice@example.test",
            RestoreKind::General,
        );
        (
            config,
            ToolRestore::from_maps(&map, &HashMap::new()).unwrap(),
        )
    }

    #[test]
    fn stream_waits_for_full_name_and_orders_arguments_before_finish() {
        let (config, restore) = fixture();
        for suffix in ["", "_other"] {
            let mut stream = ToolStream::default();
            let log = Log::new("test".to_string());
            let first = json!({"model":"test","choices":[{"index":0,"delta":{"tool_calls":[{"index":"0","id":"call_1","function":{"name":"lookup_customer","arguments":"{\"email\":\"MASK\"}"}}]}}]});
            let actions = stream
                .push(
                    format!("data: {first}\n\n").as_bytes(),
                    false,
                    &config,
                    &restore,
                    &log,
                )
                .unwrap();
            assert!(actions
                .iter()
                .all(|action| !matches!(action, StreamAction::Arguments(_))));
            assert!(!actions
                .iter()
                .any(|action| format!("{action:?}").contains("alice@example.test")));
            let last = json!({"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":suffix,"arguments":null}}]},"finish_reason":""}]});
            let actions = stream
                .push(
                    format!("data: {last}\n\n").as_bytes(),
                    false,
                    &config,
                    &restore,
                    &log,
                )
                .unwrap();
            let position = actions
                .iter()
                .position(|action| matches!(action, StreamAction::Arguments(_)))
                .unwrap();
            assert!(matches!(
                actions[position - 1],
                StreamAction::FlushChoice(0)
            ));
            let StreamAction::Arguments(value) = &actions[position] else {
                unreachable!()
            };
            let text = value["choices"][0]["delta"]["tool_calls"][0]["function"]["arguments"]
                .as_str()
                .unwrap();
            assert_eq!(text.contains("alice@example.test"), suffix.is_empty());
            assert!(matches!(
                actions.last().unwrap(),
                StreamAction::Event { .. }
            ));
            stream.release_queued();
            assert_eq!(stream.retained, 0);
        }
    }
    fn send(
        stream: &mut ToolStream,
        value: Value,
        end: bool,
        config: &AiDataMaskingConfig,
        restore: &ToolRestore,
    ) -> Result<Vec<StreamAction>, &'static str> {
        stream.push(
            format!("data: {value}\n\n").as_bytes(),
            end,
            config,
            restore,
            &Log::new("test".to_string()),
        )
    }

    #[test]
    fn known_nonallowed_tools_do_not_consume_restore_budgets() {
        let (mut config, restore) = fixture();
        config.tool_arguments.limits.max_buffer_bytes = 1;
        config.tool_arguments.limits.max_calls = 1;
        for choice in [json!(0), json!("0")] {
            let mut stream = ToolStream::default();
            for index in 0..3 {
                let input = json!({"choices":[{"index":choice,"delta":{"tool_calls":[{"index":index,"function":{"name":"another_tool","arguments":"not JSON MASK"}}]}}]});
                assert!(send(&mut stream, input, false, &config, &restore).is_ok());
                assert_eq!(stream.retained, 0);
                assert!(stream.seen.is_empty());
            }
        }
    }

    #[test]
    fn stream_keys_calls_by_choice_normalized_index_and_legacy_scope() {
        let (config, restore) = fixture();
        let mut stream = ToolStream::default();
        let first = json!({"choices":[
            {"index":0,"delta":{"tool_calls":[{"index":"01","id":"modern","function":{"name":"lookup_","arguments":"{\"email\":"}}],"function_call":{"name":"lookup_customer","arguments":"{\"email\":\"MASK\"}"}}},
            {"index":1,"delta":{"tool_calls":[{"index":1,"id":"other","function":{"name":"lookup_customer","arguments":"{\"email\":\"MASK\"}"}}]}}
        ]});
        send(&mut stream, first, false, &config, &restore).unwrap();
        let last = json!({"choices":[{"index":0,"delta":{"tool_calls":[{"id":"modern","function":{"name":"customer","arguments":"\"MASK\"}"}}]},"finish_reason":"tool_calls"}]});
        let mut actions = send(&mut stream, last, false, &config, &restore).unwrap();
        assert_eq!(
            actions
                .iter()
                .filter(|action| matches!(action, StreamAction::Arguments(_)))
                .count(),
            2
        );
        actions.extend(
            stream
                .push(
                    b"data: [DONE]\n\n",
                    true,
                    &config,
                    &restore,
                    &Log::new("test".to_string()),
                )
                .unwrap(),
        );
        assert_eq!(
            actions
                .iter()
                .filter(|action| matches!(action, StreamAction::Arguments(_)))
                .count(),
            3
        );
        assert!(actions
            .iter()
            .filter_map(|action| if let StreamAction::Arguments(value) = action {
                Some(value.to_string())
            } else {
                None
            })
            .all(|text| text.contains("alice@example.test")));
        stream.release_queued();
        assert_eq!(stream.retained, 0);
        assert_eq!(stream.seen.len(), 3);
    }

    #[test]
    fn stream_rejects_ambiguous_identity_and_counts_metadata_before_arguments() {
        let (config, restore) = fixture();
        for choice in [
            json!("0"),
            json!(-1),
            json!(0.5),
            Value::Null,
            json!(u64::MAX),
        ] {
            let value = json!({"choices":[{"index":choice,"delta":{"tool_calls":[{"index":0,"function":{"name":"lookup_customer","arguments":"{}"}}]}}]});
            assert!(send(&mut ToolStream::default(), value, false, &config, &restore).is_err());
        }
        for index in [
            Value::Null,
            json!(-1),
            json!(0.5),
            json!("18446744073709551616"),
            json!("+1"),
        ] {
            let value = json!({"choices":[{"index":0,"delta":{"tool_calls":[{"index":index,"function":{"name":"lookup_customer","arguments":"{}"}}]}}]});
            assert!(send(&mut ToolStream::default(), value, false, &config, &restore).is_err());
        }
        let mut stream = ToolStream::default();
        send(&mut stream,json!({"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"same","function":{"name":"lookup_customer"}}]}}]}),false,&config,&restore).unwrap();
        assert!(send(&mut stream,json!({"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"same","function":{"arguments":"{}"}}]}}]}),false,&config,&restore).is_err());
        let mut config = config;
        config.tool_arguments.limits.max_buffer_bytes = "lookup_customer".len() - 1;
        assert!(send(&mut ToolStream::default(),json!({"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"lookup_customer"}}]}}]}),false,&config,&restore).is_err());
    }

    #[test]
    fn stream_limits_input_rewrite_queue_and_retains_completed_call_count() {
        let (mut config, restore) = fixture();
        config.tool_arguments.limits.max_call_bytes = 16;
        let input = json!({"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"lookup_customer","arguments":"{\"email\":\"MASK\"}"}}]}}]});
        assert!(send(
            &mut ToolStream::default(),
            input.clone(),
            false,
            &config,
            &restore
        )
        .is_ok());
        config.tool_arguments.limits.max_call_bytes = 15;
        assert!(send(&mut ToolStream::default(), input, false, &config, &restore).is_err());
        config.tool_arguments.limits.max_call_bytes = 100;
        let input = json!({"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"lookup_customer","arguments":"{\"email\":\"MASK\"}"}}]},"finish_reason":"tool_calls"}]});
        config.tool_arguments.limits.max_buffer_bytes = 40;
        assert!(send(
            &mut ToolStream::default(),
            input.clone(),
            false,
            &config,
            &restore
        )
        .is_err());
        config.tool_arguments.limits.max_buffer_bytes = 1000;
        config.tool_arguments.limits.max_calls = 1;
        let mut stream = ToolStream::default();
        send(&mut stream, input, false, &config, &restore).unwrap();
        stream.release_queued();
        assert_eq!(stream.retained, 0);
        assert!(send(&mut stream,json!({"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"name":"lookup_customer","arguments":"{}"}}]}}]}),false,&config,&restore).is_err());
    }

    #[test]
    fn upstream_error_discards_unreleased_arguments_and_preserves_error() {
        let (config, restore) = fixture();
        let value = json!({"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"lookup_customer","arguments":"{\"email\":\"MASK\"}"}}]},"finish_reason":"tool_calls"}]});
        let wire =
            format!("data: {value}\n\ndata: {{\"error\":{{\"message\":\"upstream failed\"}}}}\n\n");
        let mut stream = ToolStream::default();
        let actions = stream
            .push(
                wire.as_bytes(),
                true,
                &config,
                &restore,
                &Log::new("test".to_string()),
            )
            .unwrap();
        assert!(!actions
            .iter()
            .any(|action| matches!(action, StreamAction::Arguments(_))));
        assert!(actions.iter().any(|action| matches!(action,StreamAction::Event{raw,..} if String::from_utf8_lossy(raw).contains("upstream failed"))));
        assert_eq!(stream.retained, 0);
        assert!(!stream.done);
    }
}
