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

#[derive(Debug, Clone)]
pub(crate) struct DenyWord {
    jieba: Jieba,
    words: HashSet<String>,
}

// Do NOT derive `Default`. A derived `Default` builds the `jieba` field via
// `Jieba::default()`, which loads jieba's embedded default dictionary
// (tens of thousands of entries) into a Cedar trie. That is expensive, and it
// happens on paths where the deny-word set is empty:
//   - `RuleMatcher::parse_config` calls `AiDataMaskingConfig::default()`
//     unconditionally for an empty plugin config, and
//   - the `deny_words` field falls back to its default when omitted.
// An empty deny-word set can never match anything regardless of the dictionary,
// so loading it there is pure waste and, with one Wasm VM per worker thread,
// was enough to OOM Envoy on low-memory hosts (see issue #4157). Start empty.
impl Default for DenyWord {
    fn default() -> Self {
        Self::empty()
    }
}

impl DenyWord {
    pub(crate) fn from_iter<T: IntoIterator<Item = impl Into<String>>>(words: T) -> Self {
        // Load jieba's default dictionary here: it is required for correct
        // segmentation so that deny words are not spuriously matched as
        // substrings of normal text (e.g. a sensitive word that also appears
        // inside an innocent longer word). Only build it when there are words
        // to match against.
        let mut deny_word = DenyWord {
            jieba: Jieba::new(),
            words: HashSet::new(),
        };

        for word in words {
            let word_s = word.into();
            let w = word_s.trim();
            if w.is_empty() {
                continue;
            }
            deny_word.jieba.add_word(w, None, None);
            deny_word.words.insert(w.to_string());
        }

        deny_word
    }

    pub(crate) fn empty() -> Self {
        DenyWord {
            jieba: Jieba::empty(),
            words: HashSet::new(),
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

    pub(crate) fn check(&self, message: &str) -> Option<String> {
        // Nothing to match against: skip segmentation entirely. This also keeps
        // the empty (dictionary-less) instance cheap.
        if self.words.is_empty() {
            return None;
        }
        for word in self.jieba.cut(message, true) {
            if self.words.contains(word) {
                return Some(word.to_string());
            }
        }
        None
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn empty_matches_nothing() {
        let deny = DenyWord::empty();
        assert!(deny.check("hello world 你好世界").is_none());
    }

    #[test]
    fn default_is_empty() {
        // Guard against regressing to `#[derive(Default)]`: the default instance
        // must not carry any deny words (and must not load a dictionary), so it
        // never matches. This is the path that caused the OOM in issue #4157.
        let deny = DenyWord::default();
        assert!(deny.check("测试一段普通文本").is_none());
    }

    #[test]
    fn from_iter_matches_added_words() {
        let deny = DenyWord::from_iter(vec!["敏感词", "banned"]);
        assert_eq!(deny.check("这是一个敏感词测试").as_deref(), Some("敏感词"));
        assert_eq!(deny.check("this is banned text").as_deref(), Some("banned"));
        assert!(deny.check("这是一段正常文本").is_none());
    }

    #[test]
    fn from_iter_skips_blank_entries() {
        let deny = DenyWord::from_iter(vec!["", "  ", "\n"]);
        assert!(deny.check("任意文本 any text").is_none());
    }

    #[test]
    fn from_iter_does_not_match_deny_word_as_substring() {
        // With jieba's default dictionary loaded, "亲口交代" and "口交换机" are
        // segmented as 亲口/交代 and 口/交换机, so the sensitive word "口交" is
        // not matched as a substring of normal text. Removing the default
        // dictionary would break this and spuriously block the sentence
        // (regression guard for the conformance case in
        // test/e2e/conformance/tests/rust-wasm-ai-data-masking.go).
        let deny = DenyWord::from_iter(vec!["口交"]);
        assert!(deny
            .check("工信处女干事每月经过下属科室都要亲口交代24口交换机等技术性器件的安装工作")
            .is_none());
        // But it still matches when the sensitive word stands on its own.
        assert_eq!(deny.check("这是口交内容").as_deref(), Some("口交"));
    }
}
