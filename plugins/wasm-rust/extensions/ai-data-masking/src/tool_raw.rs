// Copyright (c) 2026 Alibaba Group Holding Ltd.
// SPDX-License-Identifier: Apache-2.0
use super::*;

#[derive(Default)]
pub(super) struct RawRestoreWindow {
    text: String,
    protected: Vec<ProtectedPart>,
}

struct ProtectedPart {
    span: Range<usize>,
    frame: Option<Range<usize>>,
    logical: usize,
}

impl ProtectedPart {
    fn advance(&mut self, cutoff: usize) {
        if let Some(frame) = &mut self.frame {
            // Charged argument frames are retained or emitted as a whole.
            debug_assert!(cutoff <= frame.start || cutoff >= frame.end);
            if cutoff >= frame.end {
                self.logical = 0;
            }
            *frame = frame.start.saturating_sub(cutoff)..frame.end.saturating_sub(cutoff);
        }
        self.span = self.span.start.saturating_sub(cutoff)..self.span.end.saturating_sub(cutoff);
    }
}

fn data_positions(raw: &[u8]) -> (Vec<u8>, Vec<usize>) {
    let mut data = Vec::new();
    let mut positions = Vec::new();
    let mut start = if raw.starts_with(b"\xef\xbb\xbf") {
        3
    } else {
        0
    };
    let mut had_data = false;
    while start < raw.len() {
        let end = (start..raw.len())
            .find(|&at| raw[at] == b'\r' || raw[at] == b'\n')
            .unwrap_or(raw.len());
        let line = &raw[start..end];
        let content = if line.starts_with(b"data:") {
            Some(start + 5 + usize::from(raw.get(start + 5) == Some(&b' ')))
        } else if line == b"data" {
            Some(end)
        } else {
            None
        };
        if let Some(content) = content {
            if had_data {
                data.push(b'\n');
                positions.push(start);
            }
            data.extend_from_slice(&raw[content..end]);
            positions.extend(content..end);
            had_data = true;
        }
        start = end + 1;
        if raw.get(end) == Some(&b'\r') && raw.get(end + 1) == Some(&b'\n') {
            start += 1;
        }
    }
    (data, positions)
}

fn protected_arguments(
    raw: &[u8],
    offset: usize,
    charged: bool,
) -> Result<Vec<ProtectedPart>, &'static str> {
    let (data, positions) = data_positions(raw);
    if data.is_empty()
        || data
            .iter()
            .copied()
            .filter(|byte| !byte.is_ascii_whitespace())
            .eq(b"[DONE]".iter().copied())
    {
        return Ok(Vec::new());
    }
    let text = std::str::from_utf8(&data).map_err(|_| "invalid raw SSE data")?;
    let value: Value = serde_json::from_str(text).map_err(|_| "invalid raw SSE JSON")?;
    let document = JsonStrings::parse(text)?;
    let mut parts = Vec::new();
    let Some(choices) = value.get("choices").and_then(Value::as_array) else {
        return Ok(parts);
    };
    for (choice_position, choice) in choices.iter().enumerate() {
        let Some(delta) = choice.get("delta") else {
            continue;
        };
        let mut collect = |function: &Value, path: Vec<String>| -> Result<(), &'static str> {
            let Some(arguments) = function.get("arguments") else {
                return Ok(());
            };
            let span = document
                .value_span(&path)?
                .ok_or("missing raw arguments location")?;
            let start = *positions
                .get(span.start)
                .ok_or("missing raw arguments offset")?
                + offset;
            let end = *positions
                .get(span.end - 1)
                .ok_or("missing raw arguments offset")?
                + 1
                + offset;
            let logical = if charged {
                arguments.as_str().map_or(0, str::len)
            } else {
                0
            };
            parts.push(ProtectedPart {
                span: start..end,
                frame: charged.then_some(offset..offset + raw.len()),
                logical,
            });
            Ok(())
        };
        if let Some(calls) = delta.get("tool_calls").and_then(Value::as_array) {
            for (call_position, call) in calls.iter().enumerate() {
                collect(
                    &call["function"],
                    vec![
                        "choices".into(),
                        choice_position.to_string(),
                        "delta".into(),
                        "tool_calls".into(),
                        call_position.to_string(),
                        "function".into(),
                        "arguments".into(),
                    ],
                )?;
            }
        }
        collect(
            &delta["function_call"],
            vec![
                "choices".into(),
                choice_position.to_string(),
                "delta".into(),
                "function_call".into(),
                "arguments".into(),
            ],
        )?;
    }
    parts.sort_by_key(|part| part.span.start);
    Ok(parts)
}

impl RawRestoreWindow {
    pub(super) fn discard_arguments(&mut self) {
        let mut frames: Vec<_> = self
            .protected
            .iter()
            .filter_map(|part| part.frame.clone())
            .collect();
        frames.sort_by_key(|frame| frame.start);
        frames.dedup();
        for frame in frames.into_iter().rev() {
            self.protected
                .retain(|part| part.span.end <= frame.start || part.span.start >= frame.end);
            for part in &mut self.protected {
                if part.span.start >= frame.end {
                    part.span = part.span.start - frame.len()..part.span.end - frame.len();
                    if let Some(retained) = &mut part.frame {
                        *retained = retained.start - frame.len()..retained.end - frame.len();
                    }
                }
            }
            self.text.replace_range(frame, "");
        }
    }

    pub(super) fn pending_arguments(&self) -> usize {
        self.protected.iter().map(|part| part.logical).sum()
    }

    pub(super) fn push(
        &mut self,
        raw: &[u8],
        charged: bool,
        restore: Option<&MaskRestore>,
        keep: usize,
        end: bool,
    ) -> Result<Vec<u8>, &'static str> {
        let Some(restore) = restore else {
            return Ok(raw.to_vec());
        };
        let parts = protected_arguments(raw, self.text.len(), charged)?;
        self.text
            .push_str(std::str::from_utf8(raw).map_err(|_| "invalid raw SSE bytes")?);
        self.protected.extend(parts);
        let mut cutoff = if end {
            self.text.len()
        } else {
            self.text.len().saturating_sub(keep)
        };
        while !self.text.is_char_boundary(cutoff) {
            cutoff -= 1;
        }
        loop {
            let previous = cutoff;
            for part in &self.protected {
                if let Some(frame) = &part.frame {
                    if frame.start < cutoff && cutoff < frame.end {
                        cutoff = frame.start;
                    }
                }
            }
            for (span, _) in RestoreMatches::new(restore, &self.text) {
                if span.start >= cutoff {
                    break;
                }
                if span.end > cutoff {
                    cutoff = span.start;
                    break;
                }
            }
            if cutoff == previous {
                break;
            }
        }
        let scope = ScopedBody {
            text: String::new(),
            protected: self
                .protected
                .iter()
                .map(|part| part.span.clone())
                .collect(),
        };
        let mut output = Vec::new();
        let mut copied = 0;
        for (span, replacement) in RestoreMatches::new(restore, &self.text) {
            if span.start >= cutoff {
                break;
            }
            debug_assert!(span.end <= cutoff);
            if scope.is_outside(&span)? {
                output.extend_from_slice(self.text[copied..span.start].as_bytes());
                output.extend_from_slice(replacement.as_bytes());
                copied = span.end;
            }
        }
        output.extend_from_slice(self.text[copied..cutoff].as_bytes());
        for part in &mut self.protected {
            part.advance(cutoff);
        }
        self.protected.retain(|part| !part.span.is_empty());
        self.text.drain(..cutoff);
        Ok(output)
    }
}

#[derive(Default)]
pub(super) struct RawDenyWindow {
    bytes: Vec<u8>,
}
impl RawDenyWindow {
    pub(super) fn push(
        &mut self,
        data: &[u8],
        end: bool,
        config: &AiDataMaskingConfig,
        log: &Log,
    ) -> Result<(), &'static str> {
        if config.deny_words.is_empty() && !config.system_deny {
            return Ok(());
        }
        self.bytes.extend_from_slice(data);
        let valid = match std::str::from_utf8(&self.bytes) {
            Ok(text) => text.len(),
            Err(error) if error.error_len().is_none() && !end => error.valid_up_to(),
            Err(_) => return Err("invalid raw response UTF-8"),
        };
        let text =
            std::str::from_utf8(&self.bytes[..valid]).map_err(|_| "invalid raw response UTF-8")?;
        // An unfinished word must not be denied just because a callback split
        // it after a deny-word prefix (for example bad + minton).
        let stable = if end {
            valid
        } else {
            text.char_indices()
                .rev()
                .find(|(_, character)| !character.is_alphanumeric() && *character != '_')
                .map_or(0, |(at, character)| at + character.len_utf8())
        };
        if config.check_message(&text[..stable], log) {
            return Err("legacy raw response deny matched");
        }
        let custom = config.deny_words.max_word_bytes();
        let system = if config.system_deny {
            SYSTEM.deny_word.max_word_bytes()
        } else {
            0
        };
        let keep = custom.max(system).saturating_mul(2);
        let mut cutoff = stable.saturating_sub(keep);
        while !text.is_char_boundary(cutoff) {
            cutoff -= 1;
        }
        while cutoff > 0 {
            let (at, character) = text[..cutoff]
                .char_indices()
                .next_back()
                .ok_or("raw deny boundary")?;
            if !character.is_alphanumeric() && character != '_' {
                break;
            }
            cutoff = at;
        }
        self.bytes.drain(..cutoff);
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    fn restore(mask: &str, original: &str) -> MaskRestore {
        let mut map = HashMap::new();
        record_restore_mapping(&mut map, mask.to_string(), original, RestoreKind::General);
        MaskRestore::from_map(map).unwrap().unwrap()
    }
    #[test]
    fn raw_restore_preserves_arguments_and_matches_across_events() {
        let value = json!({"choices":[{"index":0,"delta":{"content":"MASK","tool_calls":[{"index":0,"function":{"arguments":"{\"email\":\"MASK\"}"}}]}}]});
        let input = format!("data: {value}\n\n");
        let mut window = RawRestoreWindow::default();
        let output = window
            .push(
                input.as_bytes(),
                false,
                Some(&restore("MASK", "original")),
                8,
                true,
            )
            .unwrap();
        let output = String::from_utf8(output).unwrap();
        let value: Value =
            serde_json::from_str(output.trim().strip_prefix("data: ").unwrap()).unwrap();
        assert_eq!(value["choices"][0]["delta"]["content"], "original");
        assert_eq!(
            value["choices"][0]["delta"]["tool_calls"][0]["function"]["arguments"],
            "{\"email\":\"MASK\"}"
        );
        let restore = restore("MASK\n\n: next", "joined");
        let mut window = RawRestoreWindow::default();
        let mut output = window
            .push(b": MASK\n\n", false, Some(&restore), 32, false)
            .unwrap();
        output.extend(
            window
                .push(b": next\n\n", false, Some(&restore), 32, true)
                .unwrap(),
        );
        assert_eq!(output, b": joined\n\n");
    }
    #[test]
    fn queued_raw_arguments_are_counted_until_the_complete_event_is_released() {
        let arguments = "{\"x\":\"🌟\\nMASK\"}";
        let event = json!({"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":arguments}}]}}]});
        let raw = format!("data: {event}\n\n");
        let restore = restore("MASK", "original");
        let mut window = RawRestoreWindow::default();
        let first = window
            .push(raw.as_bytes(), true, Some(&restore), 16, false)
            .unwrap();
        assert!(first.is_empty());
        assert_eq!(window.pending_arguments(), arguments.len());
        window.discard_arguments();
        assert_eq!(window.pending_arguments(), 0);
        let output = window
            .push(
                b"data: {\"error\":{\"message\":\"failed\"}}\n\n",
                false,
                Some(&restore),
                16,
                true,
            )
            .unwrap();
        assert!(!String::from_utf8_lossy(&output).contains("tool_calls"));
        assert!(String::from_utf8_lossy(&output).contains("failed"));
        let mut window = RawRestoreWindow::default();
        window
            .push(raw.as_bytes(), true, Some(&restore), 16, false)
            .unwrap();
        let end = window
            .push(
                b": sufficiently long heartbeat after the queued event\n\n",
                false,
                Some(&restore),
                16,
                true,
            )
            .unwrap();
        assert!(String::from_utf8_lossy(&end).contains("MASK"));
        assert_eq!(window.pending_arguments(), 0);
    }

    #[test]
    fn raw_restore_rejects_matches_crossing_arguments_boundaries() {
        let raw=b"data: {\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"MASK\"}}]}}]}\n\n";
        let crossing = restore("MASK\"}", "original");
        assert!(RawRestoreWindow::default()
            .push(raw, false, Some(&crossing), 32, true)
            .is_err());
    }

    #[test]
    fn raw_deny_keeps_word_tail_and_matches_the_existing_whole_body_checker() {
        let config = serde_json::from_value(json!({"deny_words":["bad","BLOCKED"]})).unwrap();
        let log = Log::new("test".into());
        let mut window = RawDenyWindow::default();
        window
            .push(b"data: {\"text\":\"bad", false, &config, &log)
            .unwrap();
        let expected = config.check_message("data: {\"text\":\"badminton\"}\n\n", &log);
        assert_eq!(
            window.push(b"minton\"}\n\n", true, &config, &log).is_err(),
            expected
        );
        let mut window = RawDenyWindow::default();
        window
            .push(b"data: {\"text\":\"BLO", false, &config, &log)
            .unwrap();
        assert!(window.push(b"CKED\"}\n\n", true, &config, &log).is_err());
    }
}
