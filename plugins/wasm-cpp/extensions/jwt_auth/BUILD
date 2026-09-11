# Copyright (c) 2022 Alibaba Group Holding Ltd.
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
load("//bazel:wasm.bzl", "declare_wasm_image_targets")

proxy_wasm_cc_binary(
    name = "jwt_auth.wasm",
    srcs = [
        "plugin.cc",
        "plugin.h",
        "extractor.cc",
        "extractor.h",
        "//common:base64.h",
    ],
    deps = [
        "@com_github_google_jwt_verify//:jwt_verify_lib",
        "@com_google_absl//absl/container:btree",
        "@com_google_absl//absl/strings",
        "@com_google_absl//absl/strings:str_format",
        "@com_google_absl//absl/time",
        "//common:json_util",
        "//common:http_util",
        "//common:rule_util",
    ],
)

cc_library(
    name = "jwt_auth_lib",
    srcs = [
        "plugin.cc",
        "extractor.cc",
        "//common:base64.h",
    ],
    hdrs = [
        "plugin.h",
        "extractor.h",
    ],
    copts = ["-DNULL_PLUGIN"],
    deps = [
        "@com_github_google_jwt_verify//:jwt_verify_lib",
        "@com_google_absl//absl/container:btree",
        "@com_google_absl//absl/strings",
        "@com_google_absl//absl/strings:str_format",
        "@com_google_absl//absl/time",
        "//common:json_util",
        "@proxy_wasm_cpp_host//:lib",
        "//common:http_util_nullvm",
        "//common:rule_util_nullvm",
    ],
)

cc_test(
    name = "jwt_auth_test",
    srcs = [
        "plugin_test.cc",
    ],
    copts = ["-DNULL_PLUGIN"],
    deps = [
        ":jwt_auth_lib",
        "@com_google_googletest//:gtest",
        "@com_google_googletest//:gtest_main",
        "@proxy_wasm_cpp_host//:lib",
    ],
)

declare_wasm_image_targets(
    name = "jwt_auth",
    wasm_file = ":jwt_auth.wasm",
)
