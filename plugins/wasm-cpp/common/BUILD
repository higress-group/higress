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

cc_library(
    name = "common_util",
    hdrs = [
        "common_util.h",
    ],
    visibility = ["//visibility:public"],
    deps = [
        "@com_google_absl//absl/strings",
    ],
)

cc_library(
    name = "http_util",
    srcs = ["http_util.cc"],
    hdrs = [
        "http_util.h",
    ],
    visibility = ["//visibility:public"],
    deps = [
        ":common_util",
        "@com_google_absl//absl/strings",
        "@com_google_absl//absl/time",
        "@com_google_absl//absl/strings:str_format",
        "@proxy_wasm_cpp_sdk//:proxy_wasm_intrinsics",
    ],
)

cc_library(
    name = "http_util_nullvm",
    srcs = ["http_util.cc"],
    hdrs = [
        "http_util.h",
    ],
    visibility = ["//visibility:public"],
    copts = ["-DNULL_PLUGIN"],
    deps = [
        ":common_util",
        "@com_google_absl//absl/strings",
        "@com_google_absl//absl/time",
        "@com_google_absl//absl/strings:str_format",
        "@proxy_wasm_cpp_host//:null_lib",
    ],
)


cc_library(
    name = "crypto_util",
    srcs = [
        "crypto_util.cc",
        "crypt_blowfish.c",
        "base64.h",
    ],
    hdrs = [
        "crypto_util.h",
    ],
    visibility = ["//visibility:public"],
    deps = [
        ":common_util",
        ":json_util",
        "@com_google_absl//absl/strings",
        "@boringssl//:ssl",
    ],
)

cc_library(
    name = "rule_util",
    hdrs = [
        "route_rule_matcher.h",
    ],
    visibility = ["//visibility:public"],
    deps = [
        ":common_util",
        ":http_util",
        ":regex_util",
    ],
)

cc_library(
    name = "rule_util_nullvm",
    hdrs = [
        "route_rule_matcher.h",
    ],
    visibility = ["//visibility:public"],
    copts = ["-DNULL_PLUGIN"],
    deps = [
        ":common_util",
        ":http_util_nullvm",
        ":regex_util",
    ],
)


cc_library(
    name = "regex_util",
    hdrs = [
        "regex.h",
    ],
    visibility = ["//visibility:public"],
    deps = [
        ":common_util",
        "@com_googlesource_code_re2//:re2",
    ],
)

cc_library(
    name = "random_util",
    srcs = [
        "random.c",
    ],
    visibility = ["//visibility:public"],
    deps = [
        "@boringssl//:ssl",
    ],
)

# genrule(
#     name = "nlohmann_json_hpp",
#     srcs = ["@com_github_nlohmann_json_single_header//file"],
#     outs = ["nlohmann_json.hpp"],
#     cmd = "cp $< $@",
#     visibility = ["//visibility:public"],
# )

cc_library(
    name = "json_util",
    srcs = ["json_util.cc"],
    hdrs = [
        "json_util.h",
        "nlohmann_json.hpp",
    ],
    copts = ["-UNULL_PLUGIN"],
    visibility = ["//visibility:public"],
    deps = [
        ":common_util",
        "@com_google_absl//absl/strings",
        "@com_google_absl//absl/types:optional",
    ],
)

exports_files([
    "base64.h",
    "json_util.cc",
    "json_util.h",
])
