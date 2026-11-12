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
        for word in self.jieba.cut(message, true) {
            if self.words.contains(word) {
                return Some(word.to_string());
            }
        }
        None
    }

    pub(crate) fn max_word_bytes(&self) -> usize {
        self.words.iter().map(String::len).max().unwrap_or(0)
    }

    pub(crate) fn is_empty(&self) -> bool {
        self.words.is_empty()
    }
}
