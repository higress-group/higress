# Request and Response Logging

The `log-request-response` plugin captures selected request and response
headers and bodies in Envoy filter state. Configure the access-log formatter
to emit the corresponding values.

## Configuration

Both `request` and `response` accept `headers` and `body` sections:

- `headers.enabled` enables header capture.
- `body.enabled` enables body capture.
- `body.maxSize` limits captured bytes and defaults to 10240.
- `body.contentTypes` selects content types. The plugin supplies common text
  and structured-data defaults when this list is omitted.

```yaml
request:
  headers:
    enabled: true
  body:
    enabled: true
    maxSize: 10240
    contentTypes:
    - application/json
    - text/plain
response:
  headers:
    enabled: true
  body:
    enabled: true
    maxSize: 10240
    contentTypes:
    - application/json
    - text/plain
```

Captured bodies may contain sensitive data. Enable only the required fields,
apply appropriate log access controls, and choose a conservative size limit.
