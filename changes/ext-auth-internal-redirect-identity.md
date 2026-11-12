# Preserve external authorization identity across internal redirects

Add opt-in `http_service.authorization_request.preserve_headers_on_internal_redirect`
to ext-auth. Selected original request headers, including absent headers, are
stored in request-lifespan FilterState and reused only for authorization calls
after an internal redirect. Later upstream credential rewrites remain intact.

The default empty list preserves existing behavior. Each redirect still calls
the authorization service; client-provided original-auth or fallback markers
are not an identity source. Request-time state failures fail closed. The
runtime must support `declare_property` with DownstreamRequest lifespan, and
primary/fallback routes must use the same plugin instance and header list.
