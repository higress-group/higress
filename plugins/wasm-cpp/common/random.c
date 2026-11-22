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

#include <errno.h>
#include <stdint.h>
#include <unistd.h>

#include <openssl/rand.h>

int getentropy(void* buffer, size_t length) {
  if (length > 256) {
    errno = EIO;
    return -1;
  }
  if (RAND_bytes((uint8_t*)buffer, (int)length) != 1) {
    errno = EIO;
    return -1;
  }
  return 0;
}
