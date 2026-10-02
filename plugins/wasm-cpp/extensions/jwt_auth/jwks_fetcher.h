#pragma once

#include <functional>
#include <string>
#include <unordered_map>

#include "absl/strings/str_cat.h"
#include "absl/strings/str_format.h"
#include "absl/strings/string_view.h"
#include "common/http_util.h"
#include "jwt_verify_lib/check_audience.h"
#include "jwt_verify_lib/jwt.h"
#include "jwt_verify_lib/status.h"
#include "jwt_verify_lib/struct_utils.h"
#include "jwt_verify_lib/verify.h"

#ifndef NULL_PLUGIN

#include "proxy_wasm_intrinsics.h"
#else

#include "include/proxy-wasm/null_plugin.h"

namespace proxy_wasm {
namespace null_plugin {
namespace jwt_auth {

#endif

struct RemoteJwks {
  std::string uri;
  std::string service;
  std::string port;
  // default timeout is 1s
  uint64_t timeout = 1000;
  // default cache ttl is 30s
  uint64_t ttl = 30000;
  std::string cacheKey;
};

using JwksPtr = std::shared_ptr<google::jwt_verify::Jwks>;

using JwksReceiver = std::function<void(const JwksPtr&)>;

class JwksFetcher {
 public:
  void updateCache(const std::string& cacheKey, const JwksPtr& jwks) {
    cache_jwks_[cacheKey] = jwks;
  }
  bool fetch(RootContext* rootCtx, const RemoteJwks& remoteJwks,
             const JwksReceiver& receiver, bool forceUpdate = false) {
    auto& cacheKey = remoteJwks.cacheKey;
    if (!forceUpdate) {
      // find in thread local cache
      auto it = cache_jwks_.find(cacheKey);
      if (it != cache_jwks_.end()) {
        receiver(it->second);
        return true;
      }
      // find in shared cache
      WasmDataPtr jwks_str;
      if (WasmResult::Ok == getSharedData(cacheKey, &jwks_str)) {
        auto jwks = google::jwt_verify::Jwks::createFrom(
            jwks_str->toString(), google::jwt_verify::Jwks::Type::JWKS);
        JwksPtr shared_jwks(std::move(jwks));
        cache_jwks_[cacheKey] = shared_jwks;
        receiver(shared_jwks);
        return true;
      }
    }

    // find through remote fetch
    auto clusterName =
        absl::StrFormat("outbound|%s||%s", remoteJwks.port, remoteJwks.service);
    absl::string_view host, path;
    Wasm::Common::Http::extractHostPathFromUri(remoteJwks.uri, host, path);
    HeaderStringPairs input_headers;
    input_headers.emplace_back(":method", "GET");
    input_headers.emplace_back(":path", path);
    input_headers.emplace_back(":authority", host);
    LOG_DEBUG(absl::StrFormat("remoteJwks clusterName:%s, path:%s, host:%s",
                              clusterName, path, host));
    auto res = rootCtx->httpCall(
        clusterName, input_headers, "", {}, remoteJwks.timeout,
        [=](uint32_t, size_t body_size, uint32_t) {
          auto response_headers =
              getHeaderMapPairs(WasmHeaderMapType::HttpCallResponseHeaders);
          std::string_view response_status_code;
          for (auto& p : response_headers->pairs()) {
            if (p.first == ":status") {
              response_status_code = p.second;
            }
          }
          if (response_status_code != "200") {
            LOG_ERROR(absl::StrFormat(
                "fetch jwks network error, cacheKey:%s, status:%s", cacheKey,
                response_status_code));
            receiver(nullptr);
            return;
          }
          auto body = getBufferBytes(WasmBufferType::HttpCallResponseBody, 0,
                                     body_size);
          if (body->size() == 0) {
            LOG_ERROR(absl::StrFormat(
                "fetch jwks failed: body is empty, cacheKey:%s", cacheKey));
            receiver(nullptr);
            return;
          }
          auto jwks = google::jwt_verify::Jwks::createFrom(
              body->toString(), google::jwt_verify::Jwks::Type::JWKS);
          if (jwks->getStatus() != google::jwt_verify::Status::Ok) {
            LOG_ERROR(absl::StrFormat(
                "fetch jwks failed: invalid jwks:%s, cacheKey:%s",
                body->toString(), cacheKey));
            receiver(nullptr);
            return;
          }
          LOG_DEBUG(
              absl::StrFormat("jwks fetch success, cacheKey:%s", cacheKey));
          setSharedData(cacheKey, body->view());
          JwksPtr shared_jwks(std::move(jwks));
          cache_jwks_[cacheKey] = shared_jwks;
          receiver(shared_jwks);
        });
    if (res != WasmResult::Ok) {
      LOG_ERROR("fetch jwks failed, httpCall error: " + toString(res));
      return true;
    }
    return false;
  }

 private:
  std::unordered_map<std::string /*remoteJwks cacheKey*/, JwksPtr> cache_jwks_;
};

#ifdef NULL_PLUGIN

}  // namespace jwt_auth
}  // namespace null_plugin
}  // namespace proxy_wasm

#endif
