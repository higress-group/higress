# Copyright (c) 2026 Alibaba Group Holding Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

load("@proxy_wasm_cpp_sdk//bazel:defs.bzl", "proxy_wasm_cc_binary")

cc_binary(
    name = "native_probe",
    srcs = ["native.cc", "parse.h"],
    deps = [
        "@com_github_thalhammer_jwt_cpp//:lib",
        "@boringssl//:ssl",
    ],
    testonly = True,
)

proxy_wasm_cc_binary(
    name = "default_disabled.wasm",
    srcs = ["probe.cc", "parse.h"],
    deps = [
        "@com_github_thalhammer_jwt_cpp//:lib",
        "@boringssl//:ssl",
    ],
    tags = ["oauth-v0-test-only"],
)

proxy_wasm_cc_binary(
    name = "candidate_eh.wasm",
    srcs = ["probe.cc", "parse.h"],
    features = ["exceptions"],
    copts = ["-fwasm-exceptions"],
    linkopts = ["-fwasm-exceptions"],
    deps = [
        "@com_github_thalhammer_jwt_cpp//:lib",
        "@boringssl//:ssl",
    ],
    tags = ["oauth-v0-test-only"],
)

proxy_wasm_cc_binary(
    name = "candidate_eh_nodiag.wasm",
    srcs = ["probe.cc", "parse.h"],
    features = ["exceptions"],
    copts = ["-fwasm-exceptions"],
    linkopts = [
        "-fwasm-exceptions",
        "-sASSERTIONS=0",
        "-sEXCEPTION_STACK_TRACES=0",
    ],
    deps = [
        "@com_github_thalhammer_jwt_cpp//:lib",
        "@boringssl//:ssl",
    ],
    tags = ["oauth-v0-test-only"],
)
