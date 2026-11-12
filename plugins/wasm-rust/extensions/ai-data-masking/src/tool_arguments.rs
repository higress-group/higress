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

use crate::tool_config::pointer_tokens;
use serde_json::Value;
use std::collections::HashSet;
use std::ops::Range;

struct Literal {
    span: Range<usize>,
    value: String,
    key: bool,
}

enum Node {
    String(usize),
    Object(Vec<(usize, Node)>, Range<usize>),
    Array(Vec<Node>, Range<usize>),
    Other(Range<usize>),
}

/// Keep string positions and duplicate keys; a Value alone loses both.
pub(crate) struct JsonStrings {
    literals: Vec<Literal>,
    root: Node,
}

impl JsonStrings {
    pub(crate) fn parse(input: &str) -> Result<Self, &'static str> {
        // Use serde's JSON and depth validation, but never serialize its Value.
        serde_json::from_str::<Value>(input).map_err(|_| "invalid arguments JSON")?;
        let mut scanner = Scanner {
            input,
            at: 0,
            literals: Vec::new(),
        };
        let root = scanner.value()?;
        scanner.whitespace();
        if scanner.at != input.len() {
            return Err("trailing arguments data");
        }
        Ok(Self {
            literals: scanner.literals,
            root,
        })
    }

    fn resolve(&self, tokens: &[String]) -> Result<Option<&Node>, &'static str> {
        let mut node = &self.root;
        for token in tokens {
            node = match node {
                Node::Object(entries, _) => {
                    let mut matches = entries
                        .iter()
                        .filter(|(key, _)| self.literals[*key].value == *token);
                    let Some((_, child)) = matches.next() else {
                        return Ok(None);
                    };
                    if matches.next().is_some() {
                        return Err("ambiguous duplicate arguments key");
                    }
                    child
                }
                Node::Array(entries, _) => {
                    if token.is_empty()
                        || !token.bytes().all(|byte| byte.is_ascii_digit())
                        || (token.len() > 1 && token.starts_with('0'))
                    {
                        return Err("invalid arguments array index");
                    }
                    let index = token
                        .parse::<usize>()
                        .map_err(|_| "arguments array index overflow")?;
                    let Some(child) = entries.get(index) else {
                        return Ok(None);
                    };
                    child
                }
                _ => return Ok(None),
            };
        }
        Ok(Some(node))
    }

    pub(crate) fn string_span(
        &self,
        tokens: &[String],
    ) -> Result<Option<Range<usize>>, &'static str> {
        Ok(match self.resolve(tokens)? {
            Some(Node::String(index)) => Some(self.literals[*index].span.clone()),
            _ => None,
        })
    }
}

impl JsonStrings {
    pub(crate) fn value_span(
        &self,
        tokens: &[String],
    ) -> Result<Option<Range<usize>>, &'static str> {
        Ok(match self.resolve(tokens)? {
            Some(Node::String(index)) => Some(self.literals[*index].span.clone()),
            Some(Node::Other(span) | Node::Object(_, span) | Node::Array(_, span)) => {
                Some(span.clone())
            }
            _ => None,
        })
    }
}

struct Scanner<'a> {
    input: &'a str,
    at: usize,
    literals: Vec<Literal>,
}

impl Scanner<'_> {
    fn byte(&self) -> Option<u8> {
        self.input.as_bytes().get(self.at).copied()
    }

    fn whitespace(&mut self) {
        while self.byte().is_some_and(|byte| byte.is_ascii_whitespace()) {
            self.at += 1;
        }
    }

    fn string(&mut self, key: bool) -> Result<usize, &'static str> {
        if self.byte() != Some(b'"') {
            return Err("expected JSON string");
        }
        let start = self.at;
        self.at += 1;
        loop {
            match self.byte() {
                Some(b'\\') => self.at += 2,
                Some(b'"') => {
                    self.at += 1;
                    break;
                }
                Some(_) => self.at += 1,
                None => return Err("incomplete JSON string"),
            }
        }
        let value =
            serde_json::from_str(&self.input[start..self.at]).map_err(|_| "invalid JSON string")?;
        let index = self.literals.len();
        self.literals.push(Literal {
            span: start..self.at,
            value,
            key,
        });
        Ok(index)
    }

    fn value(&mut self) -> Result<Node, &'static str> {
        self.whitespace();
        let start = self.at;
        match self.byte() {
            Some(b'"') => self.string(false).map(Node::String),
            Some(b'{') => {
                self.at += 1;
                self.whitespace();
                let mut entries = Vec::new();
                if self.byte() == Some(b'}') {
                    self.at += 1;
                    return Ok(Node::Object(entries, start..self.at));
                }
                loop {
                    self.whitespace();
                    let key = self.string(true)?;
                    self.whitespace();
                    if self.byte() != Some(b':') {
                        return Err("expected JSON colon");
                    }
                    self.at += 1;
                    entries.push((key, self.value()?));
                    self.whitespace();
                    match self.byte() {
                        Some(b',') => self.at += 1,
                        Some(b'}') => {
                            self.at += 1;
                            break;
                        }
                        _ => return Err("incomplete JSON object"),
                    }
                }
                Ok(Node::Object(entries, start..self.at))
            }
            Some(b'[') => {
                self.at += 1;
                self.whitespace();
                let mut entries = Vec::new();
                if self.byte() == Some(b']') {
                    self.at += 1;
                    return Ok(Node::Array(entries, start..self.at));
                }
                loop {
                    entries.push(self.value()?);
                    self.whitespace();
                    match self.byte() {
                        Some(b',') => self.at += 1,
                        Some(b']') => {
                            self.at += 1;
                            break;
                        }
                        _ => return Err("incomplete JSON array"),
                    }
                }
                Ok(Node::Array(entries, start..self.at))
            }
            Some(_) => {
                let start = self.at;
                while self
                    .byte()
                    .is_some_and(|byte| !byte.is_ascii_whitespace() && !b",]}".contains(&byte))
                {
                    self.at += 1;
                }
                if start == self.at {
                    return Err("invalid JSON value");
                }
                Ok(Node::Other(start..self.at))
            }
            None => Err("incomplete arguments JSON"),
        }
    }
}

fn encoded_string_len(value: &str) -> Result<usize, &'static str> {
    value.chars().try_fold(2usize, |bytes, character| {
        let width = match character {
            '"' | '\\' | '\n' | '\r' | '\t' | '\u{08}' | '\u{0c}' => 2,
            '\u{00}'..='\u{1f}' => 6,
            _ => character.len_utf8(),
        };
        bytes.checked_add(width).ok_or("arguments size overflow")
    })
}

/// Return None when unchanged, so the caller can keep the original bytes.
/// available_new_bytes excludes input already charged by the response window.
pub(crate) fn rewrite_json(
    input: &str,
    paths: Option<&[String]>,
    max_call_bytes: usize,
    available_new_bytes: usize,
    deny: &mut impl FnMut(&str) -> bool,
    replace: &mut impl FnMut(&str) -> Result<Option<String>, &'static str>,
) -> Result<Option<String>, &'static str> {
    if input.len() > max_call_bytes {
        return Err("arguments input limit exceeded");
    }
    if input.is_empty() {
        return Ok(None);
    }
    let document = JsonStrings::parse(input)?;
    if document.literals.iter().any(|literal| deny(&literal.value)) {
        return Err("arguments deny matched");
    }
    let selected = if let Some(paths) = paths {
        let mut selected = HashSet::new();
        for path in paths {
            if let Some(Node::String(index)) = document.resolve(&pointer_tokens(path)?)? {
                selected.insert(*index);
            }
        }
        selected
    } else {
        document
            .literals
            .iter()
            .enumerate()
            .filter(|(_, literal)| !literal.key)
            .map(|(index, _)| index)
            .collect()
    };
    let mut changes = Vec::new();
    let mut output_bytes = input.len();
    let mut rewritten_bytes = 0usize;
    for (index, literal) in document.literals.iter().enumerate() {
        if !selected.contains(&index) {
            continue;
        }
        if let Some(value) = replace(&literal.value)?.filter(|value| *value != literal.value) {
            rewritten_bytes = rewritten_bytes
                .checked_add(value.len())
                .ok_or("arguments size overflow")?;
            if rewritten_bytes > available_new_bytes {
                return Err("arguments rewrite buffer limit exceeded");
            }
            output_bytes = output_bytes
                .checked_sub(literal.span.len())
                .and_then(|bytes| bytes.checked_add(encoded_string_len(&value).ok()?))
                .ok_or("arguments size overflow")?;
            changes.push((literal.span.clone(), value));
        }
    }
    if changes.is_empty() {
        return Ok(None);
    }
    if output_bytes > max_call_bytes {
        return Err("arguments output limit exceeded");
    }
    if rewritten_bytes
        .checked_add(output_bytes)
        .ok_or("arguments size overflow")?
        > available_new_bytes
    {
        return Err("arguments rewrite buffer limit exceeded");
    }
    let mut output = Vec::with_capacity(output_bytes);
    let mut cursor = 0;
    for (span, value) in changes {
        output.extend_from_slice(&input.as_bytes()[cursor..span.start]);
        serde_json::to_writer(&mut output, &value).map_err(|_| "arguments encoding failed")?;
        cursor = span.end;
    }
    output.extend_from_slice(&input.as_bytes()[cursor..]);
    serde_json::from_slice::<Value>(&output).map_err(|_| "rewritten arguments JSON invalid")?;
    String::from_utf8(output)
        .map(Some)
        .map_err(|_| "rewritten arguments UTF-8 invalid")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn decoded_values_are_reencoded_without_reserializing_other_json() {
        let input = r#"{ "z":1e2, "secret":"\u0053ECRET", "a\/b":"same", "dup":1, "dup":2 }"#;
        let replacement = "value with a quote\", a slash\\ and a newline\n";
        let result = rewrite_json(input, None, 4096, 8192, &mut |_| false, &mut |value| {
            Ok((value == "SECRET").then(|| replacement.to_string()))
        })
        .unwrap()
        .unwrap();
        assert_eq!(
            result,
            input.replace(
                r#""\u0053ECRET""#,
                &serde_json::to_string(replacement).unwrap()
            )
        );
    }

    #[test]
    fn deny_checks_decoded_keys_and_values_before_rewriting() {
        for input in [r#"{"\u0053ECRET":"value"}"#, r#"{"safe":"\u0053ECRET"}"#] {
            let mut calls = 0;
            assert!(rewrite_json(
                input,
                None,
                4096,
                8192,
                &mut |value| value == "SECRET",
                &mut |_| {
                    calls += 1;
                    Ok(Some("changed".to_string()))
                }
            )
            .is_err());
            assert_eq!(calls, 0);
        }
    }

    #[test]
    fn restore_paths_are_exact_and_ambiguous_locations_fail() {
        let cases = [
            (r#"{"a":["MASK"]}"#, "/a/0", Some(r#"{"a":["original"]}"#)),
            (
                r#"{"a":{"01":"MASK","-":"MASK"}}"#,
                "/a/01",
                Some(r#"{"a":{"01":"original","-":"MASK"}}"#),
            ),
            (
                r#"{"a/b":{"~key":"MASK"}}"#,
                "/a~1b/~0key",
                Some(r#"{"a/b":{"~key":"original"}}"#),
            ),
            (r#""MASK""#, "", Some(r#""original""#)),
            (r#"{"a":1e2}"#, "/a", None),
            (
                r#"{"a":"MASK","dup":1,"dup":2}"#,
                "/a",
                Some(r#"{"a":"original","dup":1,"dup":2}"#),
            ),
        ];
        for (input, path, expected) in cases {
            let paths = vec![path.to_string()];
            let actual = rewrite_json(input, Some(&paths), 4096, 8192, &mut |_| false, &mut |_| {
                Ok(Some("original".to_string()))
            })
            .unwrap();
            assert_eq!(actual.as_deref(), expected);
        }
        for (input, path) in [
            (r#"{"a":["MASK"]}"#, "/a/-"),
            (r#"{"a":["MASK"]}"#, "/a/00"),
            (r#"{"a":["MASK"]}"#, "/a/01"),
            (r#"{"a":"MASK","a":"MASK"}"#, "/a"),
            (r#"{"a":{"x":"MASK"},"a":{"x":"MASK"}}"#, "/a/x"),
        ] {
            assert!(rewrite_json(
                input,
                Some(&[path.to_string()]),
                4096,
                8192,
                &mut |_| false,
                &mut |_| Ok(Some("original".to_string()))
            )
            .is_err());
        }
    }

    #[test]
    fn malformed_or_oversized_arguments_cannot_be_released() {
        for input in [
            r#"{"a":"unfinished}"#,
            r#"{"a":"\uD800"}"#,
            r#"{"a":"\q"}"#,
            "{}tail",
        ] {
            assert!(
                rewrite_json(input, None, 4096, 8192, &mut |_| false, &mut |_| Ok(None)).is_err()
            );
        }
        assert!(rewrite_json("12345", None, 4, 8192, &mut |_| false, &mut |_| Ok(None)).is_err());
        assert!(
            rewrite_json(r#""x""#, None, 10, 8192, &mut |_| false, &mut |_| Ok(Some(
                "0123456789".to_string()
            )))
            .is_err()
        );
        assert!(
            rewrite_json(r#""x""#, None, 4096, 4, &mut |_| false, &mut |_| Ok(Some(
                "long".to_string()
            )))
            .is_err()
        );
        for input in ["", "null", "true", "1e2", r#"{"x": false}"#] {
            assert_eq!(
                rewrite_json(input, None, 4096, 8192, &mut |_| false, &mut |_| Ok(None)).unwrap(),
                None
            );
        }
    }

    #[test]
    fn equivalent_string_encodings_match_without_changing_keys_or_unmatched_literals() {
        for literal in [
            r#""中文/\"\n""#,
            r#""\u4e2d\u6587\/\u0022\u000a""#,
            r#""\u4E2D\u6587/\"\n""#,
        ] {
            let input = format!("{{\"unchanged\":\"\\u0041\",\"key\":{literal}}}");
            let actual = rewrite_json(&input, None, 4096, 8192, &mut |_| false, &mut |value| {
                Ok((value == "中文/\"\n").then(|| "restored".to_string()))
            })
            .unwrap()
            .unwrap();
            assert_eq!(actual, r#"{"unchanged":"\u0041","key":"restored"}"#);
        }
    }
}
