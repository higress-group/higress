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

use serde_json::{json, Number, Value};

fn merge_number(target: &mut Value, add: &Value) {
    if target.is_null() {
        if add.is_object() {
            *target = json!({});
        } else if add.is_number() {
            *target = Value::from(0i64);
        } else {
            return;
        }
    }
    match (target, add) {
        (Value::Object(t), Value::Object(a)) => {
            for (key, value) in a.iter() {
                if let Some(v) = t.get_mut(key) {
                    merge_number(v, value);
                } else {
                    t.insert(key.clone(), value.clone());
                }
            }
        }
        (Value::Number(t), Value::Number(a)) => {
            match (t.as_i64(), a.as_i64()) {
                (Some(tv), Some(av)) => *t = Number::from(tv.saturating_add(av)),
                // Preserve float semantics: some OpenAI-compatible upstreams
                // serialize usage counters as JSON floats, and as_i64() would
                // silently turn them into 0 via unwrap_or_default().
                _ => {
                    if let Some(sum) =
                        Number::from_f64(t.as_f64().unwrap_or_default() + a.as_f64().unwrap_or_default())
                    {
                        *t = sum;
                    }
                }
            }
        }
        _ => {}
    }
}
#[derive(Default, Clone)]
pub(crate) struct NumberMerge {
    value: Value,
}

impl NumberMerge {
    pub(crate) fn add(&mut self, number: &Value) {
        merge_number(&mut self.value, number);
    }
    pub(crate) fn peek(&self) -> Value {
        self.value.clone()
    }
    pub(crate) fn finish(&mut self) -> Value {
        core::mem::replace(&mut self.value, Value::Null)
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    #[test]
    fn integer_usage_values_are_summed() {
        let mut merge = NumberMerge::default();
        merge.add(&json!({"total_tokens": 5}));
        merge.add(&json!({"total_tokens": 9}));
        assert_eq!(merge.finish(), json!({"total_tokens": 14}));
    }

    #[test]
    fn float_usage_values_are_preserved() {
        let mut merge = NumberMerge::default();
        merge.add(&json!({
            "prompt_tokens": 10.5,
            "completion_tokens": 20.5,
            "total_tokens": 31.0
        }));
        // Merging the same float counter again must keep the fractional part.
        merge.add(&json!({"prompt_tokens": 10.5}));
        let value = merge.finish();
        assert_eq!(value["prompt_tokens"].as_f64(), Some(21.0));
        assert_eq!(value["completion_tokens"].as_f64(), Some(20.5));
        assert_eq!(value["total_tokens"].as_f64(), Some(31.0));
    }

    #[test]
    fn float_and_integer_usage_values_add_as_floats() {
        let mut merge = NumberMerge::default();
        merge.add(&json!({"total_tokens": 1.5}));
        merge.add(&json!({"total_tokens": 2}));
        assert_eq!(merge.finish()["total_tokens"].as_f64(), Some(3.5));
    }

    #[test]
    fn peek_does_not_consume_the_accumulated_value() {
        let mut merge = NumberMerge::default();
        merge.add(&json!({"total_tokens": 7}));
        assert_eq!(merge.peek(), json!({"total_tokens": 7}));
        assert_eq!(merge.peek(), json!({"total_tokens": 7}));
        assert_eq!(merge.finish(), json!({"total_tokens": 7}));
        assert_eq!(merge.peek(), Value::Null);
    }
}
