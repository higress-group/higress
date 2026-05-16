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

use std::collections::HashSet;

use jieba_rs::Jieba;
use rust_embed::Embed;

#[derive(Embed)]
#[folder = "res/"]
struct Asset;

#[derive(Default, Debug, Clone)]
pub(crate) struct DenyWord {
    jieba: Jieba,
    words: HashSet<String>,
    phrase_words: Vec<String>,
}

impl DenyWord {
    pub(crate) fn from_iter<T: IntoIterator<Item = impl Into<String>>>(words: T) -> Self {
        let mut deny_word = DenyWord::default();

        for word in words {
            let word_s = word.into();
            let w = word_s.trim();
            if w.is_empty() {
                continue;
            }
            if w.contains(' ') {
                deny_word.phrase_words.push(w.to_string());
            } else {
                deny_word.jieba.add_word(w, None, None);
                deny_word.words.insert(w.to_string());
            }
        }

        // Sort phrase_words by length descending for longest-match-first
        deny_word.phrase_words.sort_by(|a, b| b.len().cmp(&a.len()));

        deny_word
    }

    pub(crate) fn empty() -> Self {
        DenyWord {
            jieba: Jieba::empty(),
            words: HashSet::new(),
            phrase_words: Vec::new(),
        }
    }

    pub(crate) fn system() -> Self {
        if let Some(file) = Asset::get("sensitive_word_dict.txt") {
            if let Ok(data) = std::str::from_utf8(file.data.as_ref()) {
                return DenyWord::from_iter(data.split('\n'));
            }
        }
        Self::empty()
    }

    pub(crate) fn check(&self, message: &str, allow_end_boundary: bool) -> Option<String> {
        // Phase 1: Check phrase_words (keywords containing spaces) via substring matching
        for keyword in &self.phrase_words {
            let keyword_bytes = keyword.len();
            let mut offset = 0;
            while offset < message.len() {
                if let Some(pos) = message[offset..].find(keyword.as_str()) {
                    let start = offset + pos;
                    let end = start + keyword_bytes;

                    // Left boundary check
                    let left_ok = if start == 0 {
                        true
                    } else {
                        message[..start]
                            .chars()
                            .last()
                            .map_or(false, |c| !c.is_alphanumeric() && c != '_')
                    };

                    // Right boundary check
                    let right_ok = if end == message.len() {
                        allow_end_boundary
                    } else {
                        message[end..]
                            .chars()
                            .next()
                            .map_or(false, |c| !c.is_alphanumeric() && c != '_')
                    };

                    if left_ok && right_ok {
                        return Some(keyword.clone());
                    }

                    // Move past this occurrence to find next
                    offset = start + 1;
                } else {
                    break;
                }
            }
        }

        // Phase 2: Existing Jieba tokenization + HashSet matching for single-word keywords
        for word in self.jieba.cut(message, true) {
            if self.words.contains(word) {
                return Some(word.to_string());
            }
        }
        None
    }

    pub(crate) fn max_phrase_len(&self) -> (usize, usize) {
        self.phrase_words
            .iter()
            .map(|w| (w.chars().count(), w.len()))
            .max_by_key(|&(char_count, _)| char_count)
            .unwrap_or((0, 0))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use proptest::prelude::*;

    /// **Validates: Requirements 1.1, 1.2, 1.3**
    ///
    /// Property 1: Bug Condition - 含空格关键词检测失败
    ///
    /// Bug condition: keyword.contains(' ') AND message contains keyword
    ///                AND check(message) returns None
    ///
    /// This test is EXPECTED TO FAIL on unfixed code, proving the bug exists.
    mod bug_condition_exploration {
        use super::*;

        /// Test case 1: Two-word phrase "hello world" in message
        #[test]
        fn two_word_phrase_detection() {
            let deny = DenyWord::from_iter(vec!["hello world"]);
            let result = deny.check("I say hello world to you", true);
            assert_eq!(
                result,
                Some("hello world".to_string()),
                "Bug confirmed: keyword 'hello world' containing space was not detected in message"
            );
        }

        /// Test case 2: Multi-word phrase "credit card number" in message
        #[test]
        fn multi_word_phrase_detection() {
            let deny = DenyWord::from_iter(vec!["credit card number"]);
            let result = deny.check("Enter your credit card number here", true);
            assert_eq!(
                result,
                Some("credit card number".to_string()),
                "Bug confirmed: keyword 'credit card number' containing spaces was not detected in message"
            );
        }

        /// Test case 3: Mixed keywords - "hello world" (with space) and "敏感词" (without space)
        /// "敏感词" should match but "hello world" should not on unfixed code
        #[test]
        fn mixed_keywords_space_vs_no_space() {
            let deny = DenyWord::from_iter(vec!["hello world", "敏感词"]);

            // "敏感词" (no space) should be detected via Jieba
            let result_chinese = deny.check("这是一个敏感词测试", true);
            assert_eq!(
                result_chinese,
                Some("敏感词".to_string()),
                "Non-space keyword '敏感词' should be detected"
            );

            // "hello world" (with space) should be detected but won't be on unfixed code
            let result_english = deny.check("I say hello world to you", true);
            assert_eq!(
                result_english,
                Some("hello world".to_string()),
                "Bug confirmed: keyword 'hello world' with space not detected while '敏感词' without space works fine"
            );
        }

        /// Property-based test: For any keyword containing a space that appears in a message,
        /// check() should return Some(keyword) but on unfixed code returns None.
        proptest! {
            #![proptest_config(ProptestConfig::with_cases(3))]
            #[test]
            fn space_keyword_not_detected(
                keyword_idx in 0..3usize,
            ) {
                let test_cases: Vec<(&str, &str)> = vec![
                    ("hello world", "I say hello world to you"),
                    ("credit card number", "Enter your credit card number here"),
                    ("open source software", "We use open source software daily"),
                ];
                let (keyword, message) = test_cases[keyword_idx];

                let deny = DenyWord::from_iter(vec![keyword]);
                let result = deny.check(message, true);

                // This assertion encodes the EXPECTED (correct) behavior.
                // On unfixed code, this will FAIL — proving the bug exists.
                prop_assert_eq!(
                    result,
                    Some(keyword.to_string()),
                    "Bug condition: keyword '{}' with space not detected in message '{}'",
                    keyword,
                    message
                );
            }
        }
    }

    /// **Validates: Requirements 3.1, 3.2, 3.3, 3.4, 3.5**
    ///
    /// Property 2: Preservation - 不含空格关键词检测行为不变
    ///
    /// These tests encode the CURRENT behavior of check() for non-space keywords.
    /// They MUST PASS on unfixed code and continue to pass after the fix.
    mod preservation_tests {
        use super::*;

        /// Observation: DenyWord::from_iter(["敏感词"]).check("这是一个敏感词测试") returns Some("敏感词")
        #[test]
        fn single_chinese_keyword_detected() {
            let deny = DenyWord::from_iter(vec!["敏感词"]);
            let result = deny.check("这是一个敏感词测试", true);
            assert_eq!(result, Some("敏感词".to_string()));
        }

        /// Observation: DenyWord::from_iter(["暴力"]).check("这是一条正常消息") returns None
        #[test]
        fn no_match_returns_none() {
            let deny = DenyWord::from_iter(vec!["暴力"]);
            let result = deny.check("这是一条正常消息", true);
            assert_eq!(result, None);
        }

        /// Observation: DenyWord::from_iter([]).check("任意消息") returns None
        #[test]
        fn empty_config_returns_none() {
            let deny = DenyWord::from_iter(Vec::<String>::new());
            let result = deny.check("任意消息", true);
            assert_eq!(result, None);
        }

        /// Observation: DenyWord::from_iter(["hello"]).check("say hello to you") returns Some("hello")
        #[test]
        fn single_english_keyword_detected() {
            let deny = DenyWord::from_iter(vec!["hello"]);
            let result = deny.check("say hello to you", true);
            assert_eq!(result, Some("hello".to_string()));
        }

        // Property-based test: Single-word keyword preservation.
        // For non-space keywords that appear in the message, check() detects them.
        proptest! {
            #![proptest_config(ProptestConfig::with_cases(12))]
            #[test]
            fn single_word_keyword_detected_in_message(
                keyword_idx in 0..4usize,
                prefix_idx in 0..3usize,
                suffix_idx in 0..3usize,
            ) {
                let keywords = ["敏感词", "暴力", "hello", "test"];
                let prefixes = ["这是一个", "I say ", "消息包含"];
                let suffixes = ["测试", " to you", "的内容"];

                let keyword = keywords[keyword_idx];
                let message = format!("{}{}{}", prefixes[prefix_idx], keyword, suffixes[suffix_idx]);

                let deny = DenyWord::from_iter(vec![keyword]);
                let result = deny.check(&message, true);

                prop_assert_eq!(
                    result,
                    Some(keyword.to_string()),
                    "Non-space keyword '{}' should be detected in message '{}'",
                    keyword,
                    message
                );
            }
        }

        // Property-based test: No-match preservation.
        // When message doesn't contain any configured keyword, check() returns None.
        proptest! {
            #![proptest_config(ProptestConfig::with_cases(8))]
            #[test]
            fn no_match_always_returns_none(
                keyword_idx in 0..4usize,
                message_idx in 0..4usize,
            ) {
                // Keywords and messages are chosen so that no keyword appears in any message
                let keywords = ["敏感词", "暴力", "forbidden", "secret"];
                let messages = [
                    "这是一条正常消息",
                    "Today is a good day",
                    "普通的文本内容",
                    "Nothing special here",
                ];

                let keyword = keywords[keyword_idx];
                let message = messages[message_idx];

                let deny = DenyWord::from_iter(vec![keyword]);
                let result = deny.check(message, true);

                prop_assert_eq!(
                    result,
                    None,
                    "Keyword '{}' should NOT match in message '{}' which doesn't contain it",
                    keyword,
                    message
                );
            }
        }

        // Property-based test: Empty config preservation.
        // When deny_words is empty, check() always returns None regardless of message.
        proptest! {
            #![proptest_config(ProptestConfig::with_cases(3))]
            #[test]
            fn empty_config_always_returns_none(
                message_idx in 0..5usize,
            ) {
                let messages = [
                    "任意消息",
                    "hello world",
                    "敏感词测试",
                    "This contains anything",
                    "",
                ];

                let message = messages[message_idx];
                let deny = DenyWord::from_iter(Vec::<String>::new());
                let result = deny.check(message, true);

                prop_assert_eq!(
                    result,
                    None,
                    "Empty config should return None for any message, got result for '{}'",
                    message
                );
            }
        }

        // Property-based test: Partial match no false positive.
        // When message only contains part of a space-containing keyword phrase,
        // it should not match.
        proptest! {
            #![proptest_config(ProptestConfig::with_cases(2))]
            #[test]
            fn partial_match_no_false_positive(
                case_idx in 0..4usize,
            ) {
                // Each case: (keyword_with_space, message_with_only_partial_match)
                let test_cases: Vec<(&str, &str)> = vec![
                    ("hello world", "I say hello to everyone"),
                    ("credit card number", "Enter your credit here"),
                    ("open source software", "We use open tools daily"),
                    ("machine learning model", "This machine is great"),
                ];

                let (keyword, message) = test_cases[case_idx];
                let deny = DenyWord::from_iter(vec![keyword]);
                let result = deny.check(message, true);

                prop_assert_eq!(
                    result,
                    None,
                    "Partial match of keyword '{}' should NOT trigger in message '{}'",
                    keyword,
                    message
                );
            }
        }
    }
}
