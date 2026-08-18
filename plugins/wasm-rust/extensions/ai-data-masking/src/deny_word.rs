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
    max_char_size: usize,
    max_byte_size: usize,
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
            if w.len() > deny_word.max_byte_size {
                deny_word.max_byte_size = w.len();
            }
            if w.chars().count() > deny_word.max_char_size {
                deny_word.max_char_size = w.chars().count();
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
            max_char_size: 0,
            max_byte_size: 0,
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
    pub(crate) fn get_max_size(&self) -> (usize, usize) {
        (self.max_char_size, self.max_byte_size)
    }
}
