load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive", "http_file")
load("@bazel_skylib//rules:copy_file.bzl", "copy_file")
load(
    "@io_bazel_rules_docker//container:container.bzl",
    "container_image",
    "container_push",
)

def build_include_content(includes):
    return """
load("@rules_cc//cc:defs.bzl", "cc_library")
cc_library(
    name = "lib",
    hdrs = glob(["**"]),
    includes = {},
    visibility = ["//visibility:public"],
)
""".format(repr(includes))

def wasm_libraries():
    http_archive(
        name = "com_google_absl",
        sha256 = "3a0bb3d2e6f53352526a8d1a7e7b5749c68cd07f2401766a404fb00d2853fa49",
        strip_prefix = "abseil-cpp-4bbdb026899fea9f882a95cbd7d6a4adaf49b2dd",
        url = "https://github.com/abseil/abseil-cpp/archive/4bbdb026899fea9f882a95cbd7d6a4adaf49b2dd.tar.gz",
        patch_args = ["-p1"],
        patches = ["//bazel:absl.patch"],
    )

    http_file(
        name = "com_github_nlohmann_json_single_header",
        sha256 = "3b5d2b8f8282b80557091514d8ab97e27f9574336c804ee666fda673a9b59926",
        urls = [
            "https://github.com/nlohmann/json/releases/download/v3.7.3/json.hpp",
        ],
    )


    # import google test and cpp host for unit testing
    http_archive(
        name = "com_google_googletest",
        sha256 = "9dc9157a9a1551ec7a7e43daea9a694a0bb5fb8bec81235d8a1e6ef64c716dcb",
        strip_prefix = "googletest-release-1.10.0",
        urls = ["https://github.com/google/googletest/archive/release-1.10.0.tar.gz"],
    )

    PROXY_WASM_CPP_HOST_SHA = "ecf42a27fcf78f42e64037d4eff1a0ca5a61e403"
    PROXY_WASM_CPP_HOST_SHA256 = "9748156731e9521837686923321bf12725c32c9fa8355218209831cc3ee87080"

    http_archive(
        name = "proxy_wasm_cpp_host",
        sha256 = PROXY_WASM_CPP_HOST_SHA256,
        strip_prefix = "proxy-wasm-cpp-host-" + PROXY_WASM_CPP_HOST_SHA,
        url = "https://github.com/higress-group/proxy-wasm-cpp-host/archive/" + PROXY_WASM_CPP_HOST_SHA +".tar.gz",
    )

    http_archive(
        name = "boringssl",
        urls = ["https://github.com/google/boringssl/archive/648cbaf033401b7fe7acdce02f275b06a88aab5c.tar.gz"],
        strip_prefix = "boringssl-648cbaf033401b7fe7acdce02f275b06a88aab5c",
        patch_args = ["-p1"],
        patches = ["//bazel:boringssl.patch"],
    )

    native.bind(
        name = "ssl",
        actual = "@boringssl//:ssl",
    )

    http_archive(
        name = "com_google_protobuf",
        urls = ["https://github.com/protocolbuffers/protobuf/releases/download/v{version}/protobuf-all-3.18.0.tar.gz"],
        strip_prefix = "protobuf-3.18.0",
    )

    native.bind(
        name = "protobuf",
        actual = "@com_google_protobuf//:protobuf",
    )

    http_archive(
        name = "com_googlesource_code_re2",
        urls = ["https://github.com/google/re2/archive/2020-07-06.tar.gz"],
        strip_prefix = "re2-2020-07-06",
        patch_args = ["-p1"],
        patches = ["//bazel:re2.patch"],
    )

    native.bind(
        name = "abseil_flat_hash_set",
        actual = "@com_google_absl//absl/container:flat_hash_set",
    )

    native.bind(
        name = "abseil_strings",
        actual = "@com_google_absl//absl/strings:strings",
    )

    native.bind(
        name = "abseil_time",
        actual = "@com_google_absl//absl/time:time",
    )

    native.bind(
        name = "protobuf",
        actual = "@com_google_protobuf//:protobuf",
    )

    http_archive(
        name = "com_github_google_jwt_verify",
        urls = ["https://github.com/google/jwt_verify_lib/archive/26c22c0ce1bc607eec8fa5dd26b707378adc7a88.tar.gz"],
        strip_prefix = "jwt_verify_lib-26c22c0ce1bc607eec8fa5dd26b707378adc7a88"
    )

    http_archive(
        name = "com_github_thalhammer_jwt_cpp",
        urls = ["https://github.com/Thalhammer/jwt-cpp/archive/ad08c684a2bf9db413786e800e0d92e65772d9d6.tar.gz"],
        strip_prefix = "jwt-cpp-ad08c684a2bf9db413786e800e0d92e65772d9d6",
        build_file_content = build_include_content(includes = ["include"]),
        patch_args = ["-p1"],
        patches = ["//bazel:jwt.patch"],
    )

    http_archive(
        name = "com_github_mariusbancila_stduuid",
        urls = ["https://github.com/mariusbancila/stduuid/archive/3afe7193facd5d674de709fccc44d5055e144d7a.tar.gz"],
        strip_prefix = "stduuid-3afe7193facd5d674de709fccc44d5055e144d7a",
        build_file_content = build_include_content(includes = ["include", "."]),
    )

    
def declare_wasm_image_targets(name, wasm_file):
    # Rename to the spec compatible name.
    copy_file("copy_original_file", wasm_file, "plugin.wasm")
    container_image(
        name = "wasm_image",
        files = [":plugin.wasm"],
    )
    container_push(
        name = "push_wasm_image",
        format = "OCI",
        image = ":wasm_image",
        registry = "ghcr.io",
        repository = "istio-ecosystem/wasm-extensions/"+name,
        tag = "$(WASM_IMAGE_TAG)",
    )
