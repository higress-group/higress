---
title: IP Access Restriction
keywords: [higress, ip restriction]
description: IP access restriction plugin configuration reference
---
## Function Description
The `ip-restriction` plugin can restrict access to services or routes by whitelisting or blacklisting IP addresses. It supports restrictions on a single IP address, multiple IP addresses, and CIDR ranges like 10.10.10.0/24.

## Running Attributes
Plugin execution phase: `Authentication Phase`

Plugin execution priority: `210`

## Configuration Description
| Configuration Item  | Type    | Required | Default Value                   | Description                                 |
|---------------------|---------|----------|---------------------------------|---------------------------------------------|
| ip_source_type      | string  | No       | origin-source                   | Optional values: 1. Peer socket IP: `origin-source`; 2. Get from header: `header` |
| ip_header_name      | string  | No       | x-forwarded-for                | When `ip_source_type` is `header`, specify the custom IP source header            |
| allow               | array   | No       | []                              | Whitelist                                    |
| deny                | array   | No       | []                              | Blacklist                                    |
| status              | int     | No       | 403                             | HTTP status code when access is denied      |
| message             | string  | No       | Your IP address is blocked.     | Return message when access is denied         |

```yaml
ip_source_type: origin-source
allow:
  - 10.0.0.1
  - 192.168.0.0/16
```

```yaml
ip_source_type: header
ip_header_name: x-real-ip
deny:
  - 10.0.0.1
  - 192.169.0.0/16
```
