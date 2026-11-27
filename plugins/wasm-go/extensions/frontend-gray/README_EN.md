---
title: Frontend Gray
keywords: [higress, frontend gray]
description: Frontend Gray Plugin Configuration Reference

## Feature Description
The `frontend-gray` plugin implements frontend user grayscale capabilities. This plugin can be used for business `A/B testing` while ensuring system release stability through `grayscale`, `monitoring`, and `rollback` strategies.

## Runtime Properties

Execution Stage: `Default Stage`  
Execution Priority: `1000`

## Configuration Fields
| Name | Data Type | Required | Default | Description |
|------|-----------|----------|---------|-------------|
| `grayKey` | string | Optional | - | Unique user identifier from Cookie/Header (e.g., userid). If empty, uses `rules[].grayTagKey` and `rules[].grayTagValue` to filter rules. |
| `useManifestAsEntry` | boolean | Optional | false | Whether to use manifest as entry point. When set to true, the system will use manifest file as application entry, suitable for micro-frontend architecture. In this mode, the system loads different versions of frontend resources based on manifest file content. |
| `localStorageGrayKey` | string | Optional | - | When using JWT authentication, user ID comes from `localStorage`. Overrides `grayKey` if configured. |
| `graySubKey` | string | Optional | - | Used when user info is in JSON format (e.g., `userInfo:{ userCode:"001" }`). In this example, `graySubKey` would be `userCode`. |
| `storeMaxAge` | int | Optional | 31536000 | Max cookie storage duration in seconds (default: 1 year). |
| `indexPaths` | string[] | Optional | - | Paths requiring mandatory processing (supports Glob patterns). Example: `/resource/**/manifest-main.json` in micro-frontend scenarios. |
| `skippedPaths` | string[] | Optional | - | Excluded paths (supports Glob patterns). Example: `/api/**` XHR requests in rewrite scenarios. |
| `skippedByHeaders` | map<string, string> | Optional | - | Filter requests via headers. `skippedPaths` has higher priority. HTML page requests are unaffected. |
| `rules` | object[] | Required | - | User-defined grayscale rules for different scenarios. |
| `rewrite` | object | Required | - | Rewrite configuration for OSS/CDN deployments. |
| `baseDeployment` | object | Optional | - | Baseline configuration. |
| `grayDeployments` | object[] | Optional | - | Gray deployment rules and versions. |
| `backendGrayTag` | string | Optional | `x-mse-tag` | Backend grayscale tag. Cookies will carry `${backendGrayTag}:${grayDeployments[].backendVersion}` if configured. |
| `uniqueGrayTag` | string | Optional | `x-higress-uid` | UUID stored in cookies for percentage-based grayscale session stickiness and backend tracking. |
| `injection` | object | Optional | - | Inject global info into HTML (e.g., `<script>window.global = {...}</script>`). |

### `rules` Field
| Name | Data Type | Required | Default | Description |
|------|-----------|----------|---------|-------------|
| `name` | string | Required | - | Unique rule name linked to `grayDeployments[].name`. |
| `grayKeyValue` | string[] | Optional | - | User ID whitelist. |
| `grayTagKey` | string | Optional | - | User tag key from cookies. |
| `grayTagValue` | string[] | Optional | - | User tag values from cookies. |

### `rewrite` Field
> Both `indexRouting` and `fileRouting` use prefix matching. The `{version}` placeholder will be dynamically replaced by `baseDeployment.version` or `grayDeployments[].version`.

| Name | Data Type | Required | Default | Description |
|------|-----------|----------|---------|-------------|
| `host` | string | Optional | - | Host address (use VPC endpoint for OSS). |
| `indexRouting` | map<string, string> | Optional | - | Homepage rewrite rules. Key: route path, Value: target file. Example: `/app1` → `/mfe/app1/{version}/index.html`. |
| `fileRouting` | map<string, string> | Optional | - | Resource rewrite rules. Key: resource path, Value: target path. Example: `/app1/` → `/mfe/app1/{version}`. |

### `baseDeployment` Field
| Name | Data Type | Required | Default | Description |
|------|-----------|----------|---------|-------------|
| `version` | string | Required | - | Baseline version as fallback. |
| `backendVersion` | string | Required | - | Backend grayscale version written to cookies via `${backendGrayTag}`. |
| `versionPredicates` | string | Required | - | Supports multi-version mapping for micro-frontend scenarios. |

### `grayDeployments` Field
| Name | Data Type | Required | Default | Description |
|------|-----------|----------|---------|-------------|
| `version` | string | Required | - | Gray version used when rules match. Adds `x-higress-tag` header for non-CDN deployments. |
| `versionPredicates` | string | Required | - | Multi-version support for micro-frontends. |
| `backendVersion` | string | Required | - | Backend grayscale version for cookies. |
| `name` | string | Required | - | Linked to `rules[].name`. |
| `enabled` | boolean | Required | - | Enable/disable rule. |
| `weight` | int | Optional | - | Traffic percentage (e.g., 50). |

> **Percentage-based Grayscale Notes**:
> 1. Percentage rules override user-based rules when both exist.
> 2. Uses UUID fingerprint hashed via SHA-256 for traffic distribution.

### `injection` Field
| Name | Data Type | Required | Default | Description |
|------|-----------|----------|---------|-------------|
| `globalConfig` | object | Optional | - | Global variables injected into HTML. |
| `head` | string[] | Optional | - | Inject elements into `<head>`. |
| `body` | object | Optional | - | Inject elements into `<body>`. |

#### `globalConfig` Sub-field
| Name | Data Type | Required | Default | Description |
|------|-----------|----------|---------|-------------|
| `key` | string | Optional | `HIGRESS_CONSOLE_CONFIG` | Window global variable key. |
| `featureKey` | string | Optional | `FEATURE_STATUS` | Rule hit status (e.g., `{"beta-user":true,"inner-user":false}`). |
| `value` | string | Optional | - | Custom global value. |
| `enabled` | boolean | Optional | `false` | Enable global injection. |

#### `body` Sub-field
| Name | Data Type | Required | Default | Description |
|------|-----------|----------|---------|-------------|
| `first` | string[] | Optional | - | Inject at body start. |
| `last` | string[] | Optional | - | Inject at body end. |

## Configuration Examples
### Basic Configuration (User-based)
```yml
grayKey: userid
rules:
- name: inner-user
  grayKeyValue:
  - '00000001'
  - '00000005'
- name: beta-user
  grayKeyValue:
  - '00000002'
  - '00000003'
  grayTagKey: level
  grayTagValue:
  - level3
  - level5
baseDeployment:
  version: base
grayDeployments:
  - name: beta-user
    version: gray
    enabled: true

```

The unique user identifier in the cookie is `userid`, and the current grayscale rule configures the `beta-user` rule.

When the following conditions are met, the `version: gray` version will be used:
- `userid` in cookie equals `00000002` or `00000003`
- `level` in cookie equals `level3` or `level5`

Otherwise, the `version: base` version will be used.

### Percentage-based Grayscale
```yml
grayKey: userid
rules:
- name: inner-user
  grayKeyValue:
  - '00000001'
  - '00000005'
baseDeployment:
  version: base
grayDeployments:
  - name: beta-user
    version: gray
    enabled: true
    weight: 80

```
The total grayscale rule is 100%, with the grayscale version weighted at 80% and the baseline version at 20%.

### User Information in JSON Format
```yml
grayKey: appInfo
graySubKey: userId
rules:
- name: inner-user
  grayKeyValue:
  - '00000001'
  - '00000005'
- name: beta-user
  grayKeyValue:
  - '00000002'
  - '00000003'
  grayTagKey: level
  grayTagValue:
  - level3
  - level5
baseDeployment:
  version: base
grayDeployments:
  - name: beta-user
    version: gray
    enabled: true

```

The cookie contains JSON data in `appInfo`, which includes the `userId` field as the unique identifier.
The current grayscale rule configures the `beta-user` rule.
When the following conditions are met, the `version: gray` version will be used:
- `userid` in cookie equals `00000002` or `00000003`
- `level` in cookie equals `level3` or `level5`

Otherwise, the `version: base` version will be used.

### User Information Stored in LocalStorage
Since the gateway plugin needs to identify users by unique identity information, and HTTP protocol can only transmit information in headers, a script can be injected into the homepage to set user information from LocalStorage to cookies if user information is stored in LocalStorage.

```
(function() {
	var grayKey = '@@X_GRAY_KEY';
	var cookies = document.cookie.split('; ').filter(function(row) {
		return row.indexOf(grayKey + '=') === 0;
	});

	try {
		if (typeof localStorage !== 'undefined' && localStorage !== null) {
			var storageValue = localStorage.getItem(grayKey);
			var cookieValue = cookies.length > 0 ? decodeURIComponent(cookies[0].split('=')[1]) : null;
			if (storageValue && storageValue.indexOf('=') < 0 && cookieValue && cookieValue !== storageValue) {
				document.cookie = grayKey + '=' + encodeURIComponent(storageValue) + '; path=/;';
				window.location.reload();
			}
		}
	} catch (error) {
		// xx
	}
})();
```

### Rewrite Configuration
> Generally used for CDN deployment scenarios
```yml
grayKey: userid
rules:
- name: inner-user
  grayKeyValue:
  - '00000001'
  - '00000005'
- name: beta-user
  grayKeyValue:
  - '00000002'
  - '00000003'
  grayTagKey: level
  grayTagValue:
  - level3
  - level5
rewrite:
  host: frontend-gray.oss-cn-shanghai-internal.aliyuncs.com
  indexRouting:
    /app1: '/mfe/app1/{version}/index.html'
    /: '/mfe/app1/{version}/index.html',
  fileRouting:
    /: '/mfe/app1/{version}'
    /app1/: '/mfe/app1/{version}'
baseDeployment:
  version: base
grayDeployments:
  - name: beta-user
    version: gray
    enabled: true

```

The `{version}` will be dynamically replaced with the actual version during runtime.

#### indexRouting: Homepage Routing Configuration
Accessing `/app1`, `/app123`, `/app1/index.html`, `/app1/xxx`, `/xxxx` will all route to '/mfe/app1/{version}/index.html'

#### fileRouting: File Routing Configuration
The following file mappings will be effective:
- `/js/a.js` => `/mfe/app1/v1.0.0/js/a.js`
- `/js/template/a.js` => `/mfe/app1/v1.0.0/js/template/a.js`
- `/app1/js/a.js` => `/mfe/app1/v1.0.0/js/a.js`
- `/app1/js/template/a.js` => `/mfe/app1/v1.0.0/js/template/a.js`

### Injecting Code into HTML Homepage
```yml
grayKey: userid
rules:
- name: inner-user
  grayKeyValue:
  - '00000001'
  - '00000005'
baseDeployment:
  version: base
grayDeployments:
  - name: inner-user
    version: gray
    enabled: true
injection:
  head: 
    - <script>console.log('Header')</script>
  body:
    first:
      - <script>console.log('hello world before')</script>
      - <script>console.log('hello world before1')</script>
    last:
      - <script>console.log('hello world after')</script>
      - <script>console.log('hello world after2')</script>

```
Code can be injected into the HTML homepage through `injection`, either in the `head` tag or at the `first` and `last` positions of the `body` tag.
