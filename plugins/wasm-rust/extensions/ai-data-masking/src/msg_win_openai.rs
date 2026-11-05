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

use std::collections::BTreeMap;

use higress_wasm_rust::event_stream::EventStream;
#[cfg(test)]
use serde::Deserialize;
use serde_json::Value;

use crate::msg_window::MessageWindow;

#[derive(PartialEq, Eq, Clone, Copy)]
enum MsgFlag {
    None,
    Content,
    ReasoningContent,
}
impl Default for MsgFlag {
    fn default() -> Self {
        Self::None
    }
}
#[cfg(test)]
#[derive(Deserialize)]
struct Delta {
    #[serde(default)]
    content: Option<String>,
    #[serde(default)]
    reasoning_content: Option<String>,
}
#[cfg(test)]
#[derive(Deserialize)]
struct Choices {
    #[serde(default)]
    delta: Option<Delta>,
}

const CHOICES_PATH: &str = "choices";

type MessageLine = Vec<(MsgFlag, Vec<u8>)>;

#[derive(Clone, Default)]
struct FrameMetadata {
    id: Option<Value>,
    object: Option<Value>,
    created: Option<Value>,
    model: Option<Value>,
    system_fingerprint: Option<Value>,
}

impl FrameMetadata {
    fn from_value(value: &Value) -> Self {
        Self {
            id: value.get("id").cloned(),
            object: value.get("object").cloned(),
            created: value.get("created").cloned(),
            model: value.get("model").cloned(),
            system_fingerprint: value.get("system_fingerprint").cloned(),
        }
    }

    fn frame(&self, choice: Value) -> Value {
        let mut frame = serde_json::Map::new();
        for (key, value) in [
            ("id", &self.id),
            ("object", &self.object),
            ("created", &self.created),
            ("model", &self.model),
            ("system_fingerprint", &self.system_fingerprint),
        ] {
            if let Some(value) = value {
                frame.insert(key.to_string(), value.clone());
            }
        }
        frame.insert(CHOICES_PATH.to_string(), Value::Array(vec![choice]));
        Value::Object(frame)
    }
}

#[derive(Default)]
struct MessageWindowOpenAi {
    message_window: MessageWindow,
    ret_messages: MessageLine,
    flag: MsgFlag,
    index: i64,
    metadata: FrameMetadata,
    role: Option<Value>,
    needs_check: bool,
}

impl MessageWindowOpenAi {
    fn update(
        &mut self,
        data: &[u8],
        flag: MsgFlag,
        index: i64,
        metadata: &FrameMetadata,
        role: Option<Value>,
    ) {
        if data.is_empty() {
            return;
        }
        self.index = index;
        self.metadata = metadata.clone();
        if let Some(role) = role {
            self.role = Some(role);
        }
        if self.flag == MsgFlag::None {
            self.flag = flag;
        }
        if self.flag != flag {
            let last_flag = core::mem::replace(&mut self.flag, flag);
            let msg = self.message_window.finish();
            self.ret_messages.push((last_flag, msg));
        }
        self.message_window.update(data);
        self.needs_check = true;
    }

    fn check(&mut self, inspect: &mut impl FnMut(&mut Vec<u8>) -> bool) {
        if self.needs_check {
            let accepted = self.iter_mut().all(inspect);
            // A rejected tail must be checked again before a later flush.
            self.needs_check = !accepted;
        }
    }

    fn gen_value(&mut self, flag: MsgFlag, msg: &[u8]) -> Value {
        let mut delta = serde_json::Map::new();
        if let Some(role) = self.role.take() {
            delta.insert("role".to_string(), role);
        }
        let text = Value::String(String::from_utf8_lossy(msg).to_string());
        match flag {
            MsgFlag::Content => {
                delta.insert("content".to_string(), text);
            }
            MsgFlag::ReasoningContent => {
                delta.insert("reasoning_content".to_string(), text);
                delta.insert("content".to_string(), Value::String(String::new()));
            }
            MsgFlag::None => {}
        }
        self.metadata.frame(serde_json::json!({
            "index": self.index,
            "delta": delta,
            "finish_reason": null,
        }))
    }

    fn messages_to_value(&mut self) -> Vec<Value> {
        let mut ret = Vec::new();
        for (flag, msg) in core::mem::take(&mut self.ret_messages) {
            ret.push(self.gen_value(flag, &msg));
        }
        ret
    }

    fn pop(&mut self, char_window_size: usize, byte_window_size: usize) -> Vec<Value> {
        let mut ret = self.messages_to_value();
        let msg = self.message_window.pop(char_window_size, byte_window_size);
        if !msg.is_empty() {
            ret.push(self.gen_value(self.flag, &msg));
        }
        ret
    }

    fn take_prefix(&mut self, char_window_size: usize, byte_window_size: usize) -> Option<Self> {
        let mut ret_messages = core::mem::take(&mut self.ret_messages);
        let message = self.message_window.pop(char_window_size, byte_window_size);
        if !message.is_empty() {
            ret_messages.push((self.flag, message));
        }
        if ret_messages.is_empty() {
            return None;
        }
        Some(Self {
            ret_messages,
            index: self.index,
            metadata: self.metadata.clone(),
            role: self.role.take(),
            ..Self::default()
        })
    }

    fn finish(&mut self) -> Vec<Value> {
        let mut ret = self.messages_to_value();
        let msg = self.message_window.finish();
        let flag = core::mem::replace(&mut self.flag, MsgFlag::None);
        if !msg.is_empty() {
            ret.push(self.gen_value(flag, &msg));
        }
        ret
    }

    fn iter_mut(&mut self) -> impl Iterator<Item = &mut Vec<u8>> {
        self.ret_messages
            .iter_mut()
            .map(|(_, msg)| msg)
            .chain(self.message_window.iter_mut())
    }
}

enum QueuedEvent {
    Text(Box<MessageWindowOpenAi>),
    Raw(Vec<u8>),
}

#[derive(Default)]
pub(crate) struct MsgWindow {
    stream_parser: EventStream,
    base_message_window: MessageWindow,
    message_windows: BTreeMap<i64, MessageWindowOpenAi>,
    finish_reasons: BTreeMap<i64, Option<Value>>,
    queued_events: Vec<QueuedEvent>,
    done_sent: bool,
    error_received: bool,
    char_window_size: usize,
    byte_window_size: usize,
}

impl MsgWindow {
    fn queue_text(
        &mut self,
        index: i64,
        terminal: bool,
        inspect: &mut impl FnMut(&mut Vec<u8>) -> bool,
    ) {
        if let Some(window) = self.message_windows.get_mut(&index) {
            // Check the complete text before a prefix and its retained tail
            // become separate buffers.
            window.check(inspect);
        }
        let window = if terminal {
            self.message_windows.remove(&index)
        } else {
            self.message_windows
                .get_mut(&index)
                .and_then(|window| window.take_prefix(self.char_window_size, self.byte_window_size))
        };
        if let Some(window) = window {
            self.queued_events.push(QueuedEvent::Text(Box::new(window)));
        }
    }

    fn queue_all_text(&mut self, inspect: &mut impl FnMut(&mut Vec<u8>) -> bool) {
        for (_, mut window) in core::mem::take(&mut self.message_windows) {
            window.check(inspect);
            self.queued_events.push(QueuedEvent::Text(Box::new(window)));
        }
    }

    fn queue_raw(&mut self, mut event: Vec<u8>) {
        event.extend_from_slice(b"\n\n");
        self.queued_events.push(QueuedEvent::Raw(event));
    }

    fn has_protocol_data(choice: &Value) -> bool {
        !choice["finish_reason"].is_null()
            || choice["delta"].as_object().is_some_and(|delta| {
                delta.iter().any(|(key, value)| {
                    !["content", "reasoning_content"].contains(&key.as_str()) && !value.is_null()
                })
            })
    }

    fn update_event(
        &mut self,
        event: Vec<u8>,
        inspect: &mut impl FnMut(&mut Vec<u8>) -> bool,
    ) -> bool {
        if event.is_empty() {
            return false;
        }
        if event.strip_prefix(b"data:").is_some_and(|data| {
            data.iter()
                .copied()
                .filter(|b| !b.is_ascii_whitespace())
                .eq(b"[DONE]".iter().copied())
        }) {
            self.queue_all_text(inspect);
            if !self.done_sent {
                self.queue_raw(b"data: [DONE]".to_vec());
                self.done_sent = true;
            }
            return false;
        }
        let Ok(res) = event
            .strip_prefix(b"data:")
            .ok_or(())
            .and_then(|data| serde_json::from_slice::<Value>(data).map_err(|_| ()))
        else {
            self.queue_all_text(inspect);
            self.queue_raw(event);
            return false;
        };
        self.error_received |= !res["error"].is_null();
        let Some(choices) = res[CHOICES_PATH].as_array().filter(|a| !a.is_empty()) else {
            self.queue_all_text(inspect);
            self.queue_raw(event);
            return false;
        };
        for choice in choices {
            let index = choice["index"].as_i64().unwrap_or_default();
            let finish_reason = self.finish_reasons.entry(index).or_default();
            if let Some(reason) = choice.get("finish_reason").filter(|value| !value.is_null()) {
                *finish_reason = Some(reason.clone());
            }
        }
        let has_text = |choice: &Value| {
            ["content", "reasoning_content"].iter().any(|field| {
                choice["delta"][field]
                    .as_str()
                    .is_some_and(|text| !text.is_empty())
            })
        };
        // Non-terminal protocol events retain the tail used for text matching.
        if choices.iter().all(|choice| !has_text(choice)) {
            for choice in choices {
                if Self::has_protocol_data(choice) {
                    self.queue_text(
                        choice["index"].as_i64().unwrap_or_default(),
                        !choice["finish_reason"].is_null(),
                        inspect,
                    );
                }
            }
            self.queue_raw(event);
            return false;
        }

        let metadata = FrameMetadata::from_value(&res);
        let has_extra_fields = res.as_object().is_some_and(|object| {
            object.keys().any(|key| {
                ![
                    "id",
                    "object",
                    "created",
                    "model",
                    "system_fingerprint",
                    "usage",
                    CHOICES_PATH,
                ]
                .contains(&key.as_str())
            })
        });
        let mut controls = Vec::new();
        for choice in choices {
            let index = choice["index"].as_i64().unwrap_or_default();
            if !has_text(choice) {
                if Self::has_protocol_data(choice) {
                    self.queue_text(index, !choice["finish_reason"].is_null(), inspect);
                }
                controls.push(choice.clone());
                continue;
            }
            let delta = choice["delta"].as_object().unwrap();
            let mut control = choice.clone();
            let mut role = delta.get("role").cloned();
            for (field, flag) in [
                ("reasoning_content", MsgFlag::ReasoningContent),
                ("content", MsgFlag::Content),
            ] {
                if let Some(text) = delta
                    .get(field)
                    .and_then(Value::as_str)
                    .filter(|s| !s.is_empty())
                {
                    self.message_windows.entry(index).or_default().update(
                        text.as_bytes(),
                        flag,
                        index,
                        &metadata,
                        role.take(),
                    );
                }
                if let Some(object) = control["delta"].as_object_mut() {
                    if object
                        .get(field)
                        .is_some_and(|value| value.is_string() || value.is_null())
                    {
                        object.remove(field);
                    }
                }
            }
            if let Some(object) = control["delta"].as_object_mut() {
                object.remove("role");
            }
            if let Some(object) = control.as_object_mut() {
                for field in ["logprobs", "token_ids"] {
                    if object.get(field).is_some_and(Value::is_null) {
                        object.remove(field);
                    }
                }
            }
            let has_control = control["delta"]
                .as_object()
                .is_some_and(|object| !object.is_empty())
                || !control["finish_reason"].is_null()
                || control.as_object().is_some_and(|object| {
                    object
                        .keys()
                        .any(|key| !["index", "delta", "finish_reason"].contains(&key.as_str()))
                });
            if has_control {
                // Ancillary choice metadata must survive without breaking
                // text matches that span several input frames.
                if Self::has_protocol_data(&control) {
                    self.queue_text(index, !control["finish_reason"].is_null(), inspect);
                }
                controls.push(control);
            }
        }
        // Unknown metadata and usage retain their input event rather than
        // becoming a template for every rewritten text chunk.
        if !controls.is_empty() || !res["usage"].is_null() || has_extra_fields {
            let mut control_event = res;
            control_event[CHOICES_PATH] = Value::Array(controls);
            self.queue_raw(format!("data: {}", control_event).into_bytes());
        }
        true
    }

    fn write_values(output: &mut Vec<u8>, values: Vec<Value>) {
        for value in values {
            output.extend(format!("data: {}\n\n", value).as_bytes());
        }
    }

    fn drain_queued(&mut self) -> Vec<u8> {
        let mut output = Vec::new();
        for event in core::mem::take(&mut self.queued_events) {
            match event {
                QueuedEvent::Raw(raw) => output.extend(raw),
                QueuedEvent::Text(mut window) => {
                    Self::write_values(&mut output, window.finish());
                }
            }
        }
        output
    }

    pub(crate) fn push(
        &mut self,
        data: &[u8],
        is_openai: bool,
        char_window_size: usize,
        byte_window_size: usize,
        inspect: &mut impl FnMut(&mut Vec<u8>) -> bool,
    ) {
        self.char_window_size = char_window_size;
        self.byte_window_size = byte_window_size;
        if is_openai {
            self.stream_parser.update(data.to_vec());
            while let Some(event) = self.stream_parser.next() {
                self.update_event(event, inspect);
            }
            for window in self.message_windows.values_mut() {
                window.check(inspect);
            }
        } else {
            self.base_message_window.update(data);
            for message in self.base_message_window.iter_mut() {
                inspect(message);
            }
        }
    }

    pub(crate) fn pop(
        &mut self,
        char_window_size: usize,
        byte_window_size: usize,
        is_openai: bool,
    ) -> Vec<u8> {
        if !is_openai {
            return self
                .base_message_window
                .pop(char_window_size, byte_window_size);
        }
        let mut ret = self.drain_queued();
        for window in self.message_windows.values_mut() {
            Self::write_values(&mut ret, window.pop(char_window_size, byte_window_size));
        }
        ret
    }

    pub(crate) fn finish(
        &mut self,
        is_openai: bool,
        inspect: &mut impl FnMut(&mut Vec<u8>) -> bool,
    ) -> Vec<u8> {
        if !is_openai {
            return self.base_message_window.finish();
        }
        self.complete(is_openai, inspect);
        self.queue_all_text(inspect);
        let mut output = self.drain_queued();
        let all_finished =
            !self.finish_reasons.is_empty() && self.finish_reasons.values().all(Option::is_some);
        if !self.done_sent && all_finished && !self.error_received {
            output.extend_from_slice(b"data: [DONE]\n\n");
            self.done_sent = true;
        }
        output
    }

    // Return whether the trailing event added text that needs checking.
    pub(crate) fn complete(
        &mut self,
        is_openai: bool,
        inspect: &mut impl FnMut(&mut Vec<u8>) -> bool,
    ) -> bool {
        if is_openai {
            if let Some(event) = self.stream_parser.flush() {
                let added_text = self.update_event(event, inspect);
                for window in self.message_windows.values_mut() {
                    window.check(inspect);
                }
                return added_text;
            }
        }
        false
    }

    #[cfg(test)]
    pub(crate) fn messages_iter_mut(&mut self) -> impl Iterator<Item = &mut Vec<u8>> {
        self.base_message_window
            .iter_mut()
            .chain(
                self.queued_events
                    .iter_mut()
                    .filter_map(|event| match event {
                        QueuedEvent::Text(window) => Some(window),
                        QueuedEvent::Raw(_) => None,
                    })
                    .flat_map(|window| window.iter_mut()),
            )
            .chain(
                self.message_windows
                    .values_mut()
                    .flat_map(|window| window.iter_mut()),
            )
    }
}

#[cfg(test)]
mod tests {

    use std::fmt::Write;

    use rust_embed::Embed;

    use super::*;
    #[derive(Embed)]
    #[folder = "test/"]
    struct Asset;
    #[derive(Deserialize)]
    struct Res {
        choices: Vec<Choices>,
    }

    impl Res {
        fn get_text(&self) -> (String, String) {
            let mut content = String::new();
            let mut reasoning_content = String::new();
            for choice in self.choices.iter() {
                if let Some(delta) = &choice.delta {
                    if let Some(c) = &delta.content {
                        content += c;
                    }
                    if let Some(rc) = &delta.reasoning_content {
                        reasoning_content += rc;
                    }
                }
            }
            (content, reasoning_content)
        }
    }

    #[test]
    fn test_msg() {
        let mut msg_win = MsgWindow::default();
        let data = raw_message("raw_message.txt");
        let mut buffer = Vec::new();
        let mut inspect = |message: &mut Vec<u8>| {
            if let Ok(msg) = String::from_utf8(message.clone()) {
                *message = msg.replace("Higress", "***higress***").into_bytes();
            }
            true
        };
        for line in data.split("\n") {
            msg_win.push(line.as_bytes(), true, 7, 7, &mut inspect);
            msg_win.push(b"\n\n", true, 7, 7, &mut inspect);

            buffer.extend(msg_win.pop(7, 7, true));
        }
        buffer.extend(msg_win.finish(true, &mut inspect));
        let mut message = String::new();
        let mut reasoning_message = String::new();
        for line in buffer.split(|&x| x == b'\n') {
            if line.is_empty() {
                continue;
            }
            assert!(line.starts_with(b"data:"));
            if line.starts_with(b"data: [DONE]") {
                continue;
            }
            let des = serde_json::from_slice::<Res>(&line[b"data:".len()..]);
            assert!(des.is_ok());
            let res = des.unwrap();
            let (c, rc) = res.get_text();
            message.push_str(&c);
            reasoning_message.push_str(&rc);
        }
        let res = "***higress*** 是一个基于 Istio 的高性能服务网格数据平面项目，旨在提供高吞吐量、低延迟和可扩展的服务通信管理。它为企业级应用提供了丰富的流量治理功能，如负载均衡、熔断、限流等，并支持多协议代理（包括 HTTP/1.1, HTTP/2, gRPC）。***higress*** 的设计目标是优化 Istio 在大规模集群中的性能表现，满足高并发场景下的需求。";
        assert_eq!(message, res);
        assert_eq!(reasoning_message, res);
    }

    fn restore_test_message(message: &mut Vec<u8>) -> bool {
        *message = String::from_utf8(message.clone())
            .unwrap()
            .replace("MASKED", "original")
            .into_bytes();
        true
    }

    fn rewrite_stream(input: &str, fragment_size: usize) -> String {
        let mut window = MsgWindow::default();
        let mut output = Vec::new();
        for fragment in input.as_bytes().chunks(fragment_size) {
            window.push(fragment, true, 48, 48, &mut restore_test_message);
            output.extend(window.pop(48, 48, true));
        }
        output.extend(window.finish(true, &mut restore_test_message));
        String::from_utf8(output).unwrap()
    }

    fn stream_events(output: &str) -> Vec<Value> {
        output
            .lines()
            .filter_map(|line| line.strip_prefix("data:"))
            .filter(|data| data.trim() != "[DONE]")
            .map(|data| serde_json::from_str(data).unwrap())
            .collect()
    }

    fn encode_events(events: &[Value]) -> String {
        let mut input = String::new();
        for event in events {
            writeln!(&mut input, "data: {}\n", event).unwrap();
        }
        input.push_str("data: [DONE]\n\n");
        input
    }

    #[test]
    fn test_preserve_tool_calls_after_buffered_text() {
        let calls = vec![
            serde_json::json!([{"index":0,"id":"call_read","type":"function","function":{"name":"read","arguments":""}}]),
            serde_json::json!([{"index":1,"id":"call_search","type":"function","function":{"name":"search","arguments":""}}]),
            serde_json::json!([{"index":0,"function":{"arguments":"{\"path\":"}}]),
            serde_json::json!([{"index":1,"function":{"arguments":"{\"query\":"}}]),
            serde_json::json!([{"index":0,"function":{"arguments":"\"README.md\"}"}}]),
            serde_json::json!([{"index":1,"function":{"arguments":"\"demo\"}"}}]),
        ];
        let mut events = vec![
            serde_json::json!({"choices":[{"index":0,"delta":{"content":"Checking files."}}]}),
        ];
        for call in &calls {
            events.push(serde_json::json!({"choices":[{"index":0,"delta":{"tool_calls":call}}]}));
        }
        events.push(
            serde_json::json!({"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}),
        );
        let input = encode_events(&events);
        for size in [1, 19, input.len()] {
            let output = rewrite_stream(&input, size);
            let mut actual = Vec::new();
            let mut content = String::new();
            let mut finishes = Vec::new();
            for event in stream_events(&output) {
                for choice in event["choices"].as_array().unwrap() {
                    content.push_str(choice["delta"]["content"].as_str().unwrap_or_default());
                    if let Some(call) = choice["delta"].get("tool_calls") {
                        assert!("Checking files.".starts_with(&content));
                        actual.push(call.clone());
                    }
                    if let Some(reason) = choice["finish_reason"].as_str() {
                        finishes.push(reason.to_string());
                    }
                }
            }
            assert_eq!(actual, calls, "fragment size {size}");
            assert_eq!(content, "Checking files.");
            assert_eq!(finishes, vec!["tool_calls"]);
            assert_eq!(output.matches("data: [DONE]").count(), 1);
            assert!(output.ends_with("data: [DONE]\n\n"));
        }
    }

    #[test]
    fn test_preserve_empty_terminal_and_usage_events() {
        let usage = serde_json::json!({"prompt_tokens":10,"completion_tokens":5,"total_tokens":15});
        for terminal in [
            serde_json::json!({"index":0,"delta":{},"finish_reason":"stop"}),
            serde_json::json!({"index":0,"delta":null,"finish_reason":"stop"}),
            serde_json::json!({"index":0,"finish_reason":"stop"}),
        ] {
            let input = encode_events(&[
                serde_json::json!({"choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}),
                serde_json::json!({"choices":[{"index":0,"delta":{"reasoning_content":"Thinking.","content":"Answer."}}]}),
                serde_json::json!({"choices":[terminal.clone()]}),
                serde_json::json!({"choices":[],"usage":usage.clone()}),
            ]);
            for size in [1, 19, input.len()] {
                let output = rewrite_stream(&input, size);
                let events = stream_events(&output);
                let mut content = String::new();
                let mut reasoning = String::new();
                let mut finishes = Vec::new();
                let mut usages = Vec::new();
                for event in &events {
                    if !event["usage"].is_null() {
                        usages.push(event["usage"].clone());
                    }
                    for choice in event["choices"].as_array().unwrap() {
                        content.push_str(choice["delta"]["content"].as_str().unwrap_or_default());
                        reasoning.push_str(
                            choice["delta"]["reasoning_content"]
                                .as_str()
                                .unwrap_or_default(),
                        );
                        if !choice["finish_reason"].is_null() {
                            finishes.push(choice.clone());
                        }
                    }
                }
                assert_eq!(content, "Answer.");
                assert_eq!(reasoning, "Thinking.");
                assert_eq!(finishes, vec![terminal.clone()]);
                assert_eq!(usages, vec![usage.clone()]);
                assert_eq!(output.matches("data: [DONE]").count(), 1);
                assert!(output.ends_with("data: [DONE]\n\n"));
            }
        }
    }

    #[test]
    fn test_restore_text_without_changing_tool_arguments() {
        let call = serde_json::json!([{"index":0,"id":"call_read","type":"function","function":{"name":"read","arguments":"{\"path\":\"MASKED\"}"}}]);
        let input = encode_events(&[
            serde_json::json!({"choices":[{"index":0,"delta":{"content":"MA"}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{"content":"SKED","tool_calls":call.clone()}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}),
        ]);
        for size in [1, 19, input.len()] {
            let output = rewrite_stream(&input, size);
            let mut content = String::new();
            let mut calls = Vec::new();
            for event in stream_events(&output) {
                for choice in event["choices"].as_array().unwrap() {
                    content.push_str(choice["delta"]["content"].as_str().unwrap_or_default());
                    if let Some(value) = choice["delta"].get("tool_calls") {
                        assert!("original".starts_with(&content));
                        calls.push(value.clone());
                    }
                }
            }
            assert_eq!(content, "original");
            assert_eq!(calls, vec![call.clone()]);
        }
    }

    #[test]
    fn test_restore_text_across_same_choice_tool_event() {
        let call = serde_json::json!([{"index":0,"id":"call_read","type":"function","function":{"name":"read","arguments":"{\"path\":\"MASKED\"}"}}]);
        let input = encode_events(&[
            serde_json::json!({"choices":[{"index":0,"delta":{"content":"MA"}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{"tool_calls":call.clone()}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{"content":"SKED"}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}),
        ]);
        for size in [1, 19, input.len()] {
            let output = rewrite_stream(&input, size);
            let mut content = String::new();
            let mut calls = Vec::new();
            let mut finishes = Vec::new();
            for event in stream_events(&output) {
                for choice in event["choices"].as_array().unwrap() {
                    content.push_str(choice["delta"]["content"].as_str().unwrap_or_default());
                    if let Some(value) = choice["delta"].get("tool_calls") {
                        calls.push(value.clone());
                    }
                    if let Some(reason) = choice["finish_reason"].as_str() {
                        finishes.push(reason.to_string());
                    }
                }
            }
            assert_eq!(content, "original", "fragment size {size}");
            assert_eq!(calls, vec![call.clone()], "fragment size {size}");
            assert_eq!(finishes, vec!["tool_calls"], "fragment size {size}");
            assert_eq!(output.matches("data: [DONE]").count(), 1);
        }
    }

    #[test]
    fn test_restore_before_popping_protocol_prefix() {
        let content = format!("{}MASKED{}", "x".repeat(100), "y".repeat(45));
        let input = encode_events(&[
            serde_json::json!({"choices":[{"index":0,"delta":{"content":content}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"read","arguments":"{}"}}]}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}),
        ]);
        for size in [1, 19, input.len()] {
            let output = rewrite_stream(&input, size);
            let actual: String = stream_events(&output)
                .iter()
                .flat_map(|event| event["choices"].as_array().unwrap())
                .filter_map(|choice| choice["delta"]["content"].as_str())
                .collect();
            assert_eq!(
                actual,
                content.replace("MASKED", "original"),
                "fragment size {size}"
            );
        }
    }

    #[test]
    fn test_same_choice_tool_event_keeps_text_for_deny_check() {
        let events = [
            serde_json::json!({"choices":[{"index":0,"delta":{"content":"MA"}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"read","arguments":"{}"}}]}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{"content":"SKED"}}]}),
        ];
        for size in [1, 19, usize::MAX] {
            let mut window = MsgWindow::default();
            for (index, event) in events.iter().enumerate() {
                let data = format!("data: {event}\n\n");
                for fragment in data.as_bytes().chunks(size) {
                    window.push(fragment, true, 48, 48, &mut |_| true);
                    window.pop(48, 48, true);
                }
                let messages: Vec<_> = window
                    .messages_iter_mut()
                    .map(|message| message.clone())
                    .collect();
                let expected = if index < 2 {
                    b"MA".as_slice()
                } else {
                    b"MASKED".as_slice()
                };
                assert!(
                    messages.iter().any(|message| message == expected),
                    "event {index}, fragment size {size}: {messages:?}"
                );
            }
        }
    }

    #[test]
    fn test_preserve_multiple_choices_and_long_arguments() {
        let arguments = serde_json::json!({"content":"report detail\n".repeat(1000)}).to_string();
        let mut events = vec![serde_json::json!({"choices":[
            {"index":1,"delta":{"tool_calls":[{"index":0,"id":"call_write","type":"function","function":{"name":"write","arguments":""}}]}},
            {"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_read","type":"function","function":{"name":"read","arguments":"{}"}}]}}
        ]})];
        for chunk in arguments.as_bytes().chunks(512) {
            events.push(serde_json::json!({"choices":[{"index":1,"delta":{"tool_calls":[{"index":0,"function":{"arguments":std::str::from_utf8(chunk).unwrap()}}]}}]}));
        }
        events.push(serde_json::json!({"choices":[
            {"index":1,"delta":{},"finish_reason":"tool_calls"},
            {"index":0,"delta":{},"finish_reason":"tool_calls"}
        ]}));
        let input = encode_events(&events);
        for size in [1, 19, input.len()] {
            let output = rewrite_stream(&input, size);
            let actual: Vec<Value> = stream_events(&output)
                .into_iter()
                .flat_map(|event| event["choices"].as_array().unwrap().clone())
                .collect();
            let expected: Vec<Value> = events
                .iter()
                .flat_map(|event| event["choices"].as_array().unwrap().clone())
                .collect();
            assert_eq!(actual, expected, "fragment size {size}");
        }
    }

    #[test]
    fn test_preserve_error_without_synthesizing_completion() {
        let input =
            "data: {\"error\":{\"message\":\"upstream failure\",\"type\":\"server_error\"}}\n\n";
        for size in [1, 19, input.len()] {
            let output = rewrite_stream(input, size);
            assert_eq!(output, input);
            assert!(!output.contains("finish_reason"));
            assert!(!output.contains("[DONE]"));
        }
    }

    #[test]
    fn test_check_final_event_before_flushing_text() {
        let input = "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"MASKED\"}}]}";
        let mut window = MsgWindow::default();
        window.push(input.as_bytes(), true, 48, 48, &mut restore_test_message);
        assert!(window.complete(true, &mut restore_test_message));
        assert!(!window.complete(true, &mut restore_test_message));
        let output = String::from_utf8(window.finish(true, &mut restore_test_message)).unwrap();
        assert_eq!(
            stream_events(&output)[0]["choices"][0]["delta"]["content"],
            "original"
        );
        assert!(!output.contains("[DONE]"));
    }

    #[test]
    fn test_protocol_barrier_keeps_other_choice_buffered() {
        let call = serde_json::json!([{"index":0,"id":"call_read","type":"function","function":{"name":"read","arguments":"{}"}}]);
        let input = encode_events(&[
            serde_json::json!({"choices":[{"index":0,"delta":{"content":"Checking."}},{"index":1,"delta":{"content":"MA"}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{"tool_calls":call.clone()}}]}),
            serde_json::json!({"choices":[{"index":1,"delta":{"content":"SKED"}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"},{"index":1,"delta":{},"finish_reason":"stop"}]}),
        ]);
        for size in [1, 19, input.len()] {
            let output = rewrite_stream(&input, size);
            let mut texts = BTreeMap::<i64, String>::new();
            let mut calls = Vec::new();
            for event in stream_events(&output) {
                for choice in event["choices"].as_array().unwrap() {
                    let index = choice["index"].as_i64().unwrap();
                    texts
                        .entry(index)
                        .or_default()
                        .push_str(choice["delta"]["content"].as_str().unwrap_or_default());
                    if let Some(value) = choice["delta"].get("tool_calls") {
                        assert!("Checking.".starts_with(&texts[&0]));
                        calls.push(value.clone());
                    }
                }
            }
            assert_eq!(texts[&0], "Checking.", "fragment size {size}");
            assert_eq!(texts[&1], "original", "fragment size {size}");
            assert_eq!(calls, vec![call.clone()]);
        }
    }

    #[test]
    fn test_text_frames_do_not_duplicate_protocol_fields() {
        let call = serde_json::json!([{"index":0,"id":"call_read","type":"function","function":{"name":"read","arguments":"MASKED"}}]);
        let input = encode_events(&[
            serde_json::json!({"id":"test-id","object":"chat.completion.chunk","created":7,"model":"test-model","system_fingerprint":"test-fingerprint","custom_top":{"tag":"keep"},"choices":[{"index":0,"delta":{"role":"assistant","content":"MASKED","tool_calls":call.clone(),"refusal":"keep-refusal","custom_delta":null},"logprobs":{"tokens":[]},"custom_choice":null}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}),
        ]);
        for size in [1, 19, input.len()] {
            let events = stream_events(&rewrite_stream(&input, size));
            assert_eq!(
                events
                    .iter()
                    .filter(|event| event.get("custom_top").is_some())
                    .count(),
                1
            );
            let mut calls = Vec::new();
            let mut roles = Vec::new();
            let mut refusals = Vec::new();
            for event in &events {
                for choice in event["choices"].as_array().unwrap() {
                    if choice["delta"]["content"] == "original" {
                        assert_eq!(event["id"], "test-id");
                        assert_eq!(event["created"], 7);
                        assert_eq!(event["system_fingerprint"], "test-fingerprint");
                        assert!(event.get("custom_top").is_none());
                        assert!(choice.get("logprobs").is_none());
                        assert!(choice["delta"].get("tool_calls").is_none());
                    }
                    if let Some(value) = choice["delta"].get("tool_calls") {
                        calls.push(value.clone());
                        assert_eq!(choice["logprobs"], serde_json::json!({"tokens":[]}));
                        assert_eq!(choice.get("custom_choice"), Some(&Value::Null));
                        assert_eq!(choice["delta"].get("custom_delta"), Some(&Value::Null));
                    }
                    if let Some(value) = choice["delta"].get("role") {
                        roles.push(value.clone());
                    }
                    if let Some(value) = choice["delta"].get("refusal") {
                        refusals.push(value.clone());
                    }
                }
            }
            assert_eq!(calls, vec![call.clone()]);
            assert_eq!(roles, vec![serde_json::json!("assistant")]);
            assert_eq!(refusals, vec![serde_json::json!("keep-refusal")]);
        }
    }

    #[test]
    fn test_role_is_emitted_once_when_text_is_split() {
        let text = "plain text ".repeat(30);
        let input = encode_events(&[
            serde_json::json!({"choices":[{"index":0,"delta":{"role":"assistant","content":text}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}),
        ]);
        for size in [1, 19, input.len()] {
            let events = stream_events(&rewrite_stream(&input, size));
            let roles = events
                .iter()
                .flat_map(|event| event["choices"].as_array().unwrap())
                .filter(|choice| choice["delta"].get("role").is_some())
                .count();
            assert_eq!(roles, 1, "fragment size {size}");
        }
    }

    #[test]
    fn test_synthesize_done_after_all_choices_and_usage() {
        let events = [
            serde_json::json!({"choices":[{"index":0,"delta":{"content":"first"}},{"index":1,"delta":{"content":"second"}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}),
            serde_json::json!({"choices":[{"index":1,"delta":{},"finish_reason":"stop"}]}),
            serde_json::json!({"choices":[],"usage":{"total_tokens":2}}),
        ];
        let input = encode_events(&events).replace("data: [DONE]\n\n", "");
        for size in [1, 19, input.len()] {
            let output = rewrite_stream(&input, size);
            assert_eq!(output.matches("data: [DONE]").count(), 1);
            assert!(output.ends_with("data: [DONE]\n\n"));
            assert_eq!(
                stream_events(&output).last().unwrap()["usage"]["total_tokens"],
                2
            );
        }
        let input = encode_events(&events) + "data: [DONE]\n\n";
        assert_eq!(rewrite_stream(&input, 1).matches("data: [DONE]").count(), 1);
    }

    #[test]
    fn test_unfinished_choice_does_not_synthesize_done() {
        let input = encode_events(&[
            serde_json::json!({"choices":[{"index":0,"delta":{"content":"first"}},{"index":1,"delta":{"content":"partial"}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}),
        ]).replace("data: [DONE]\n\n", "");
        for size in [1, 19, input.len()] {
            assert!(!rewrite_stream(&input, size).contains("[DONE]"));
        }
    }

    #[test]
    fn test_preserve_non_data_event_and_error_after_text() {
        let tail =
            ": keepalive\n\ndata: {\"error\":{\"message\":\"failed\"},\"custom_error\":null}\n\n";
        let input = format!("data: {{\"choices\":[{{\"index\":0,\"delta\":{{\"content\":\"MASKED\"}}}}]}}\n\n{tail}");
        for size in [1, 19, input.len()] {
            let output = rewrite_stream(&input, size);
            assert!(output.ends_with(tail));
            assert!(output.find("original").unwrap() < output.find(": keepalive").unwrap());
            assert!(!output.contains("[DONE]"));
        }
    }

    #[test]
    fn test_error_after_finish_does_not_synthesize_done() {
        let input = "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"error\":{\"message\":\"failed\"}}\n\n";
        for size in [1, 19, input.len()] {
            let output = rewrite_stream(input, size);
            assert!(output.contains("failed"));
            assert!(!output.contains("[DONE]"));
        }
    }

    #[test]
    fn test_forward_done_without_waiting_for_http_end() {
        let mut window = MsgWindow::default();
        window.push(b"data: [DONE]\n\n", true, 48, 48, &mut |_| true);
        assert_eq!(window.pop(48, 48, true), b"data: [DONE]\n\n");
        assert!(window.finish(true, &mut restore_test_message).is_empty());
    }

    #[test]
    fn test_denied_text_is_not_released_on_terminal_event() {
        let events = [
            serde_json::json!({"choices":[{"index":0,"delta":{"content":"FOR"}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"read","arguments":"{}"}}]}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{"content":"BIDDEN"}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}),
        ];
        for size in [1, 19, usize::MAX] {
            let mut window = MsgWindow::default();
            let mut output = Vec::new();
            let mut rejected = 0;
            for event in &events {
                let data = format!("data: {event}\n\n");
                for fragment in data.as_bytes().chunks(size) {
                    let mut deny = false;
                    window.push(fragment, true, 48, 48, &mut |message| {
                        if String::from_utf8_lossy(message).contains("FORBIDDEN") {
                            deny = true;
                            rejected += 1;
                        }
                        !deny
                    });
                    let pending = window.pop(48, 48, true);
                    if !deny {
                        output.extend(pending);
                    }
                }
            }
            assert!(rejected > 0, "fragment size {size}");
            assert!(
                !String::from_utf8(output).unwrap().contains("FORBIDDEN"),
                "fragment size {size}"
            );
        }
    }

    #[test]
    fn test_protocol_event_follows_ready_prefix_and_retains_tail() {
        let content = "Checking.".repeat(16);
        let call = serde_json::json!([{"index":0,"function":{"name":"read","arguments":"{}"}}]);
        let input = encode_events(&[
            serde_json::json!({"choices":[{"index":0,"delta":{"content":content}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{"tool_calls":call.clone()}}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}),
        ]);
        for size in [1, 19, input.len()] {
            let mut actual = String::new();
            let mut calls = 0;
            for event in stream_events(&rewrite_stream(&input, size)) {
                for choice in event["choices"].as_array().unwrap() {
                    actual.push_str(choice["delta"]["content"].as_str().unwrap_or_default());
                    if let Some(value) = choice["delta"].get("tool_calls") {
                        assert_eq!(value, &call);
                        assert_eq!(actual.len(), content.len() - 48, "fragment size {size}");
                        assert!(content.starts_with(&actual));
                        calls += 1;
                    }
                }
            }
            assert_eq!(actual, content, "fragment size {size}");
            assert_eq!(calls, 1);
        }
    }

    #[test]
    fn test_null_ancillary_fields_do_not_emit_control_frames() {
        let input = encode_events(&[
            serde_json::json!({"choices":[{"index":0,"delta":{"content":"First "},"logprobs":null,"token_ids":null}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{"content":"second."},"logprobs":null,"token_ids":null}]}),
            serde_json::json!({"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}),
        ]);
        for size in [1, 19, input.len()] {
            let events = stream_events(&rewrite_stream(&input, size));
            assert_eq!(events.len(), 2, "fragment size {size}");
            assert_eq!(events[0]["choices"][0]["delta"]["content"], "First second.");
            assert_eq!(events[1]["choices"][0]["finish_reason"], "stop");
            assert!(events
                .iter()
                .all(|event| event["choices"]
                    .as_array()
                    .unwrap()
                    .iter()
                    .all(|choice| choice.get("logprobs").is_none()
                        && choice.get("token_ids").is_none())));
        }
    }

    #[test]
    fn test_restore_across_text_frames_with_ancillary_metadata() {
        for logprobs in [Value::Null, serde_json::json!({"tokens":[]})] {
            let input = encode_events(&[
                serde_json::json!({"custom_top":null,"choices":[{"index":0,"delta":{"content":"MA","custom_delta":null},"logprobs":logprobs,"token_ids":null}]}),
                serde_json::json!({"custom_top":null,"choices":[{"index":0,"delta":{"content":"SK","custom_delta":null},"logprobs":logprobs,"token_ids":null}]}),
                serde_json::json!({"custom_top":null,"choices":[{"index":0,"delta":{"content":"ED","custom_delta":null},"logprobs":logprobs,"token_ids":null,"finish_reason":"stop"}]}),
            ]);
            for size in [1, 19, input.len()] {
                let events = stream_events(&rewrite_stream(&input, size));
                let choices: Vec<&Value> = events
                    .iter()
                    .flat_map(|event| event["choices"].as_array().unwrap())
                    .collect();
                let text: String = choices
                    .iter()
                    .filter_map(|choice| choice["delta"]["content"].as_str())
                    .collect();
                assert_eq!(text, "original", "fragment size {size}");
                assert_eq!(
                    events
                        .iter()
                        .filter(|event| event.get("custom_top").is_some())
                        .count(),
                    3
                );
                assert_eq!(
                    choices
                        .iter()
                        .filter(|choice| choice.get("logprobs") == Some(&logprobs))
                        .count(),
                    if logprobs.is_null() { 0 } else { 3 }
                );
                assert_eq!(
                    choices
                        .iter()
                        .filter(|choice| choice["delta"].get("custom_delta") == Some(&Value::Null))
                        .count(),
                    3
                );
            }
        }
    }

    fn raw_message(file_name: &str) -> String {
        if let Some(file) = Asset::get(file_name) {
            if let Ok(data) = std::str::from_utf8(file.data.as_ref()) {
                return data.to_string();
            }
        }
        String::new()
    }
}
