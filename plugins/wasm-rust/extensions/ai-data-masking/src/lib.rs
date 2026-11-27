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

mod ai_data_masking;
mod deny_word;
mod msg_win_openai;
mod msg_window;
mod tool_arguments;
mod tool_config;

// Native callback tests record host I/O. T5 still runs the actual Wasm module
// in Envoy; these imports do not replace runtime verification.
#[cfg(all(test, not(target_arch = "wasm32")))]
mod test_host {
    use proxy_wasm::types::BufferType;
    use std::cell::RefCell;

    thread_local! {
        static INPUT: RefCell<Vec<u8>> = const { RefCell::new(Vec::new()) };
        static BUFFERS: RefCell<Vec<(u32, Vec<u8>)>> = const { RefCell::new(Vec::new()) };
        static REJECTIONS: RefCell<Vec<(u32, Vec<u8>)>> = const { RefCell::new(Vec::new()) };
    }

    pub(crate) fn response_input(bytes: &[u8]) {
        INPUT.with(|input| *input.borrow_mut() = bytes.to_vec());
    }

    #[no_mangle]
    unsafe extern "C" fn proxy_get_buffer_bytes(
        _kind: u32,
        start: usize,
        size: usize,
        data: *mut *mut u8,
        len: *mut usize,
    ) -> u32 {
        let bytes = INPUT.with(|input| {
            let input = input.borrow();
            input
                .get(start..start.saturating_add(size).min(input.len()))
                .unwrap_or(&[])
                .to_vec()
                .into_boxed_slice()
        });
        // Match the SDK's ownership transfer: it frees a buffer of exactly len bytes.
        unsafe {
            *len = bytes.len();
            *data = Box::into_raw(bytes) as *mut u8;
        }
        0
    }

    pub(crate) fn reset() {
        BUFFERS.with(|buffers| buffers.borrow_mut().clear());
        REJECTIONS.with(|responses| responses.borrow_mut().clear());
    }

    pub(crate) fn take_request_body() -> Option<Vec<u8>> {
        BUFFERS.with(|buffers| {
            buffers
                .borrow_mut()
                .iter()
                .rev()
                .find(|(kind, _)| *kind == BufferType::HttpRequestBody as u32)
                .map(|(_, bytes)| bytes.clone())
        })
    }

    pub(crate) fn last_rejection() -> Option<(u32, Vec<u8>)> {
        REJECTIONS.with(|responses| responses.borrow().last().cloned())
    }

    pub(crate) fn take_response_body() -> Option<Vec<u8>> {
        BUFFERS.with(|buffers| {
            buffers
                .borrow()
                .iter()
                .rev()
                .find(|(kind, _)| *kind == BufferType::HttpResponseBody as u32)
                .map(|(_, bytes)| bytes.clone())
        })
    }

    #[no_mangle]
    extern "C" fn proxy_log(_level: u32, _message: *const u8, _size: usize) -> u32 {
        0
    }

    #[no_mangle]
    unsafe extern "C" fn proxy_set_buffer_bytes(
        kind: u32,
        _start: usize,
        _size: usize,
        data: *const u8,
        len: usize,
    ) -> u32 {
        // The SDK supplies a live slice for this synchronous host call.
        let bytes = if len == 0 {
            Vec::new()
        } else {
            unsafe { std::slice::from_raw_parts(data, len) }.to_vec()
        };
        BUFFERS.with(|buffers| buffers.borrow_mut().push((kind, bytes)));
        0
    }

    #[no_mangle]
    unsafe extern "C" fn proxy_send_local_response(
        code: u32,
        _details: *const u8,
        _details_len: usize,
        data: *const u8,
        len: usize,
        _headers: *const u8,
        _headers_len: usize,
        _grpc: i32,
    ) -> u32 {
        // The SDK supplies a live slice for this synchronous host call.
        let bytes = if len == 0 {
            Vec::new()
        } else {
            unsafe { std::slice::from_raw_parts(data, len) }.to_vec()
        };
        REJECTIONS.with(|responses| responses.borrow_mut().push((code, bytes)));
        0
    }
}
