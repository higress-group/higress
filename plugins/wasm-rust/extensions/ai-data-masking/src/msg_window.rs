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

#[derive(Default)]
pub(crate) struct MessageWindow {
    message: Vec<u8>,
}

impl MessageWindow {
    pub(crate) fn update(&mut self, data: &[u8]) {
        self.message.extend(data);
    }

    pub(crate) fn pop(&mut self, char_window_size: usize, byte_window_size: usize) -> Vec<u8> {
        if let Ok(message) = String::from_utf8(self.message.clone()) {
            let chars = message.chars().collect::<Vec<char>>();
            if chars.len() <= char_window_size {
                return Vec::new();
            }
            let ret = chars[..chars.len() - char_window_size]
                .iter()
                .collect::<String>();
            self.message = chars[chars.len() - char_window_size..]
                .iter()
                .collect::<String>()
                .as_bytes()
                .to_vec();
            ret.as_bytes().to_vec()
        } else {
            // A non-UTF-8 buffer can be shorter than byte_window_size (for
            // example, a binary upstream error body). The subtraction must be
            // guarded like the character branch above, otherwise it underflows
            // (or wraps into an out-of-range slice index in release builds).
            if self.message.len() <= byte_window_size {
                return Vec::new();
            }
            let ret = self.message[..self.message.len() - byte_window_size].to_vec();
            self.message = self.message[self.message.len() - byte_window_size..].to_vec();
            ret
        }
    }
    pub(crate) fn finish(&mut self) -> Vec<u8> {
        core::mem::take(&mut self.message)
    }
    pub(crate) fn iter_mut(&mut self) -> impl Iterator<Item = &mut Vec<u8>> {
        std::iter::once(&mut self.message)
    }
}

#[cfg(test)]
mod tests {
    #[test]
    fn test_msg_window() {
        let mut msg_window = super::MessageWindow::default();
        msg_window.update(b"hello world");
        assert_eq!(msg_window.pop(5, 5), b"hello ");
        assert_eq!(msg_window.pop(5, 5), b"");
        msg_window.update(b"hello world");
        assert_eq!(msg_window.pop(5, 5), b"worldhello ");
        assert_eq!(msg_window.finish(), b"world");
    }

    #[test]
    fn test_msg_window_non_utf8_shorter_than_window_does_not_panic() {
        // A non-UTF-8 buffer that is shorter than byte_window_size must not
        // panic; nothing is emitted and the buffer is kept for finish().
        let mut msg_window = super::MessageWindow::default();
        msg_window.update(&[0xff, 0xfe, b'e', b'r', b'r', 0x80]);
        assert_eq!(msg_window.pop(128, 128), Vec::<u8>::new());
        assert_eq!(
            msg_window.finish(),
            vec![0xff, 0xfe, b'e', b'r', b'r', 0x80]
        );
    }

    #[test]
    fn test_msg_window_non_utf8_longer_than_window_keeps_window_bytes() {
        let mut msg_window = super::MessageWindow::default();
        let mut data = vec![b'a'; 20];
        data.push(0xff);
        msg_window.update(&data);
        assert_eq!(msg_window.pop(5, 5), vec![b'a'; 16]);
        assert_eq!(msg_window.finish(), vec![b'a', b'a', b'a', b'a', 0xff]);
    }
}
