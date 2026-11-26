// Copyright (c) 2024 Alibaba Group Holding Ltd.
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

/// Parsing MIME type text/event-stream according to https://html.spec.whatwg.org/multipage/server-sent-events.html#parsing-an-event-stream
///
/// The event stream format is as described by the stream production of the following ABNF
///
/// | rule   | expression                |
/// |--------|---------------------------|
/// |stream  |= [ bom ] *event           |
/// |event   |= *( comment / field ) eol |
/// |comment |= colon *any-char eol      |
/// |field   |= 1*name-char [ colon [ space ] *any-char ] eol |
/// |eol     |= ( cr lf / cr / lf )      |
///
/// CRLF is one logical line ending. An empty line, formed by two consecutive
/// logical line endings, terminates an event.

#[derive(Default)]
pub struct EventStream {
    buffer: Vec<u8>,
    processed_offset: usize,
    skip_lf: bool,
}

impl Iterator for EventStream {
    type Item = Vec<u8>;
    /// Get the next event from the event stream. Return the event data if available, otherwise return None.
    /// CRLF pairs are consumed together, including pairs split across updates.
    ///
    /// A delimiter ending in CR at the buffer boundary completes the event
    /// immediately. Waiting for the next byte to distinguish CR from CRLF
    /// would delay dispatch when that byte is not LF, or never arrives.
    /// `update` skips an optional following LF using `skip_lf`, so it does not
    /// become an extra line ending.
    fn next(&mut self) -> Option<Self::Item> {
        let mut i = self.processed_offset;

        while i < self.buffer.len() {
            if let Some(size) = self.is_2eol(i) {
                let event = self.buffer[self.processed_offset..i].to_vec();
                self.processed_offset = i + size;
                self.skip_lf = self.processed_offset == self.buffer.len()
                    && self.buffer.last() == Some(&b'\r');
                return Some(event);
            }

            i += self.is_eol(i).unwrap_or(1);
        }

        None
    }
}

impl EventStream {
    /// Update the event stream by adding new data to the buffer and resetting processed offset if needed.
    pub fn update(&mut self, data: Vec<u8>) {
        if self.processed_offset > 0 {
            self.buffer.drain(0..self.processed_offset);
            self.processed_offset = 0;
        }

        let mut start = 0;
        if self.skip_lf && !data.is_empty() {
            if data[0] == b'\n' {
                start = 1;
            }
            self.skip_lf = false;
        }
        self.buffer.extend_from_slice(&data[start..]);
    }

    /// Flush the event stream and return any remaining unprocessed event data. Return None if there is none.
    pub fn flush(&mut self) -> Option<Vec<u8>> {
        if self.processed_offset < self.buffer.len() {
            let remaining_event = self.buffer[self.processed_offset..].to_vec();
            self.processed_offset = self.buffer.len();
            Some(remaining_event)
        } else {
            None
        }
    }

    fn is_eol(&self, i: usize) -> Option<usize> {
        if i + 1 < self.buffer.len() && self.buffer[i] == b'\r' && self.buffer[i + 1] == b'\n' {
            Some(2)
        } else if self.buffer[i] == b'\r' || self.buffer[i] == b'\n' {
            Some(1)
        } else {
            None
        }
    }

    fn is_2eol(&self, i: usize) -> Option<usize> {
        let size1 = self.is_eol(i)?;
        if i + size1 < self.buffer.len() {
            self.is_eol(i + size1).map(|size2| size1 + size2)
        } else {
            None
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_crlf_events() {
        let mut parser = EventStream::default();
        parser.update(b"event1\n\nevent2\n\n".to_vec());

        assert_eq!(parser.next(), Some(b"event1".to_vec()));
        assert_eq!(parser.next(), Some(b"event2".to_vec()));
    }

    #[test]
    fn test_lf_events() {
        let mut parser = EventStream::default();
        parser.update(b"event3\n\r\nevent4\r\n\r\n".to_vec());

        assert_eq!(parser.next(), Some(b"event3".to_vec()));
        assert_eq!(parser.next(), Some(b"event4".to_vec()));
    }

    #[test]
    fn test_partial_event() {
        let mut parser = EventStream::default();
        parser.update(b"partial_event1".to_vec());

        assert_eq!(parser.next(), None);

        parser.update(b"\n\n".to_vec());
        assert_eq!(parser.next(), Some(b"partial_event1".to_vec()));
    }

    #[test]
    fn test_mixed_eol_events() {
        let mut parser = EventStream::default();
        parser.update(b"event5\r\nevent6\r\n\r\nevent7\r\n".to_vec());

        assert_eq!(parser.next(), Some(b"event5\r\nevent6".to_vec()));
        assert_eq!(parser.next(), None);
        assert_eq!(parser.flush(), Some(b"event7\r\n".to_vec()));
    }

    #[test]
    fn test_mixed2_eol_events() {
        let mut parser = EventStream::default();
        parser.update(b"event5\r\nevent6\r\n".to_vec());
        assert_eq!(parser.next(), None);
        parser.update(b"\r\nevent7\r\n".to_vec());
        assert_eq!(parser.next(), Some(b"event5\r\nevent6".to_vec()));
        assert_eq!(parser.next(), None);
        assert_eq!(parser.flush(), Some(b"event7\r\n".to_vec()));
    }

    #[test]
    fn test_no_event() {
        let mut parser = EventStream::default();
        parser.update(b"no_eol_in_this_string".to_vec());

        assert_eq!(parser.next(), None);
        assert_eq!(parser.flush(), Some(b"no_eol_in_this_string".to_vec()));
    }
    #[test]
    fn test_crlf_is_one_line_ending() {
        let mut parser = EventStream::default();
        parser.update(b"event: custom\r\ndata: {}\r\n\r\n".to_vec());
        assert_eq!(parser.next(), Some(b"event: custom\r\ndata: {}".to_vec()));
        assert_eq!(parser.next(), None);
    }

    #[test]
    fn test_line_endings_across_all_splits() {
        for eol in ["\n", "\r", "\r\n"] {
            for separator in ["\n\n", "\r\r", "\r\n\r\n", "\n\r", "\r\n\n", "\n\r\n"] {
                let first = format!("event: custom{eol}data: {{}}");
                let wire = format!("{first}{separator}data: second{separator}");
                let expected = vec![first.as_bytes().to_vec(), b"data: second".to_vec()];
                for split in 0..=wire.len() {
                    let mut parser = EventStream::default();
                    parser.update(wire.as_bytes()[..split].to_vec());
                    let mut events: Vec<_> = parser.by_ref().collect();
                    parser.update(wire.as_bytes()[split..].to_vec());
                    events.extend(parser.by_ref());
                    assert_eq!(
                        events, expected,
                        "eol={eol:?}, separator={separator:?}, split={split}"
                    );
                    assert_eq!(parser.flush(), None);
                }
                let mut parser = EventStream::default();
                let mut events = Vec::new();
                for byte in wire.as_bytes() {
                    parser.update(vec![*byte]);
                    events.extend(parser.by_ref());
                }
                assert_eq!(events, expected);
                assert_eq!(parser.flush(), None);
            }
        }
    }
}
