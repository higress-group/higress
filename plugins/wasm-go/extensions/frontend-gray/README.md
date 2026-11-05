---
title: 前端灰度
keywords: [higress,frontend gray]
description: 前端灰度插件配置参考
---

## 功能说明
`frontend-gray`插件实现了前端用户灰度的功能，通过此插件，不但可以用于业务`A/B实验`，同时通过`可灰度`配合`可监控`,`可回滚`策略保证系统发布运维的稳定性。

## 运行属性

插件执行阶段：`默认阶段`
插件执行优先级：`1000`


## 配置字段
| 名称             | 数据类型         | 填写要求 | 默认值 | 描述                                                                                                 |
|----------------|--------------|----|-----|----------------------------------------------------------------------------------------------------|
| `grayKey`         | string       | 非必填 | -   | 用户ID的唯一标识，可以来自Cookie或者Header中，比如 userid，如果没有填写则使用`rules[].grayTagKey`和`rules[].grayTagValue`过滤灰度规则 |
| `useManifestAsEntry` | boolean | 非必填 | false | 是否使用manifest作为入口。当设置为true时，系统将使用manifest文件作为应用入口，适用于微前端架构。在这种模式下，系统会根据manifest文件的内容来加载不同版本的前端资源。 |
| `localStorageGrayKey`         | string       | 非必填 | -   | 使用JWT鉴权方式，用户ID的唯一标识来自`localStorage`中，如果配置了当前参数，则`grayKey`失效 |
| `graySubKey`    | string       | 非必填 | -   | 用户身份信息可能以JSON形式透出，比如：`userInfo:{ userCode:"001" }`,当前例子`graySubKey`取值为`userCode` |
| `storeMaxAge`         | int       | 非必填 | 60 * 60 * 24 * 365   | 网关设置Cookie最大存储时长：单位为秒，默认为1年 |
| `indexPaths` | array of strings | 非必填 | - | 强制处理的路径，支持 `Glob` 模式匹配。例如：在 微前端场景下，XHR 接口如： `/resource/**/manifest-main.json`本质是一个资源请求，需要走插件转发逻辑。 |
| `skippedPaths` | array of strings | 非必填 | - | 用于排除特定路径，避免当前插件处理这些请求，支持 `Glob` 模式匹配。例如，在 rewrite 场景下，XHR 接口请求 `/api/**` 如果经过插件转发逻辑，可能会导致非预期的结果。 |
| `skippedByHeaders` | map of string to string   | 非必填  | -   | 用于通过请求头过滤，指定哪些请求不被当前插件
处理。`skippedPaths` 的优先级高于当前配置，且页面HTML请求不受本配置的影响。 |
| `rules`      | array of object | 必填 | -   | 用户定义不同的灰度规则，适配不同的灰度场景   |
| `rewrite`      | object | 必填 | -   | 重写配置，一般用于OSS/CDN前端部署的重写配置  |
| `baseDeployment` | object   | 非必填 | -   | 配置Base基线规则的配置    |
| `grayDeployments` |  array of object   | 非必填 | -   | 配置Gray灰度的生效规则，以及生效版本                                |
| `backendGrayTag`     | string       | 非必填  | `x-mse-tag`   | 后端灰度版本Tag，如果配置了，cookie中将携带值为`${backendGrayTag}:${grayDeployments[].backendVersion}` |
| `uniqueGrayTag`     | string       | 非必填  | `x-higress-uid`   | 开启按照比例灰度时候，网关会生成一个唯一标识存在`cookie`中，一方面用于session黏贴，另一方面后端也可以使用这个值用于全链路的灰度串联 |
| `injection`     | object    | 非必填  | -   | 往首页HTML中注入全局信息，比如`<script>window.global = {...}</script>` |


`rules`字段配置说明：

| 名称             | 数据类型         | 填写要求 | 默认值 | 描述                                                                                |
|----------------|--------------|------|-----|-----------------------------------------------------------------------------------|
| `name`         | string       | 必填   | -   | 规则名称唯一标识，和`deploy.gray[].name`进行关联生效                                          |
| `grayKeyValue`    | array of string       | 非必填   | -   | 用户ID 白名单列表 |
| `grayTagKey`      | string | 非必填  | -   | 用户分类打标的标签key值，来自Cookie                                                             |
| `grayTagValue` | array of string   | 非必填  | -   | 用户分类打标的标签value值，来自Cookie                                                         |

`rewrite`字段配置说明：
> `indexRouting`首页重写和`fileRouting`文件重写，本质都是前缀匹配，比如`/app1`: `/mfe/app1/{version}/index.html`代表/app1为前缀的请求，路由到`/mfe/app1/{version}/index.html`页面上，其中`{version}`代表版本号，在运行过程中会被`baseDeployment.version`或者`grayDeployments[].version`动态替换。

> `{version}` 作为保留字段，在执行过程中会被`baseDeployment.version`或者`grayDeployments[].version`动态替换前端版本。


| 名称         | 数据类型         | 填写要求 | 默认值 | 描述                           |
|------------|--------------|------|-----|------------------------------|
| `host`     | string       | 非必填  | -   | host地址，如果是OSS则设置为 VPC 内网访问地址 |
| `indexRouting`    | map of string to string       | 非必填  | -   | 用于定义首页重写路由规则。每个键 (Key) 表示首页的路由路径，值 (Value) 则指向重定向的目标文件。例如，键为 `/app1` 对应的值为 `/mfe/app1/{version}/index.html`。生效version为`0.0.1`， 访问路径为 `/app1`，则重定向到 `/mfe/app1/0.0.1/index.html`。                     |
| `fileRouting`     | map of string to string       | 非必填  | -   | 用于定义资源文件重写路由规则。每个键 (Key) 表示资源访问路径，值 (Value) 则指向重定向的目标文件。例如，键为 `/app1/` 对应的值为 `/mfe/app1/{version}`。生效version为`0.0.1`，访问路径为 `/app1/js/a.js`，则重定向到 `/mfe/app1/0.0.1/js/a.js`。                     |

`baseDeployment`字段配置说明：

| 名称             | 数据类型         | 填写要求 | 默认值 | 描述                                                                                |
|----------------|--------------|------|-----|-----------------------------------------------------------------------------------|
| `version`         | string       | 必填   | -   | Base版本的版本号，作为兜底的版本 |
| `backendVersion`  | string | 必填   | -   | 后端灰度版本，配合`key`为`${backendGrayTag}`，写入cookie中 |
| `versionPredicates`  | string | 必填   | -   | 和`version`含义相同，但是满足多版本的需求：根据不同路由映射不同的`Version`版本。一般用于微前端的场景：一个主应用需要管理多个微应用 |

`grayDeployments`字段配置说明：

| 名称     | 数据类型   | 填写要求 | 默认值 | 描述                                              |
|--------|--------|------|-----|-------------------------------------------------|
| `version`  | string | 必填   | -   | Gray版本的版本号，如果命中灰度规则，则使用此版本。如果是非CDN部署，在header添加`x-higress-tag` |
| `versionPredicates`  | string | 必填   | -   | 和`version`含义相同，但是满足多版本的需求：根据不同路由映射不同的`Version`版本。一般用于微前端的场景：一个主应用需要管理多个微应用 |
| `backendVersion`  | string | 必填   | -   | 后端灰度版本，配合`key`为`${backendGrayTag}`，写入cookie中 |
| `name` | string | 必填   | -   | 规则名称和`rules[].name`关联 |
| `enabled`  | boolean   | 必填   | -   | 是否启动当前灰度规则                                      |
| `weight`  | int   | 非必填   | -   | 按照比例灰度，比如50。 |
>按照比例灰度注意下面几点:
> 1. 如果同时配置了`按用户灰度`以及`按比例灰度`，按`比例灰度`优先生效
> 2. 采用客户端设备标识符的哈希摘要机制实现流量比例控制，其唯一性判定逻辑遵循以下原则：自动生成全局唯一标识符（UUID）作为设备指纹，可以通过`uniqueGrayTag`配置`cookie`的key值，并通过SHA-256哈希算法生成对应灰度判定基准值。


`injection`字段配置说明：

| 名称     | 数据类型   | 填写要求 | 默认值 | 描述                                              |
|--------|--------|------|-----|-------------------------------------------------|
| `globalConfig`  | object | 非必填   | -   | 注入到HTML首页的全局变量 |
| `head`  | array of string | 非必填   | -   | 注入head信息，比如`<link rel="stylesheet" href="https://cdn.example.com/styles.css">` |
| `body`  | object | 非必填   | -   | 注入Body |

`injection.globalConfig`字段配置说明：
| 名称     | 数据类型   | 填写要求 | 默认值 | 描述                                              |
|--------|--------|------|-----|-------------------------------------------------|
| `key`  | string | 非必填   |  HIGRESS_CONSOLE_CONFIG  | 注入到window全局变量的key值 |
| `featureKey`  | string | 非必填   | FEATURE_STATUS   | 关于`rules`相关规则的命中情况，返回实例`{"beta-user":true,"inner-user":false}` |
| `value`  | string | 非必填   | -   | 自定义的全局变量 |
| `enabled`  | boolean | 非必填   | false   | 是否开启注入全局变量 |

`injection.body`字段配置说明：
| 名称     | 数据类型   | 填写要求 | 默认值 | 描述                                              |
|--------|--------|------|-----|-------------------------------------------------|
| `first`  | array of string | 非必填   | -   | 注入body标签的首部 |
| `after`  | array of string | 非必填   | -   | 注入body标签的尾部 |



## 配置示例
### 基础配置（按用户灰度）
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


cookie中的用户唯一标识为 `userid`，当前灰度规则配置了`beta-user`的规则。

当满足下面调试的时候，会使用`version: gray`版本
- cookie中`userid`等于`00000002`或者`00000003`
- cookie中`level`等于`level3`或者`level5`的用户

否则使用`version: base`版本

### 按比例灰度
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
总的灰度规则为100%，其中灰度版本的权重为80%，基线版本为20%。
### 用户信息存在JSON中

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

cookie存在`appInfo`的JSON数据，其中包含`userId`字段为当前的唯一标识
当前灰度规则配置了`beta-user`的规则。
当满足下面调试的时候，会使用`version: gray`版本
- cookie中`userid`等于`00000002`或者`00000003`
- cookie中`level`等于`level3`或者`level5`的用户

否则使用`version: base`版本

### 用户信息存储在LocalStorage
由于网关插件需要识别用户为唯一身份信息，HTTP协议进行信息传输，只能在Header中传递。如果用户信息存储在LocalStorage，在首页注入一段脚本将LocalStorage中的用户信息设置到cookie中。
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

### rewrite重写配置
> 一般用于CDN部署场景
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

`{version}`会在运行过程中动态替换为真正的版本

#### indexRouting：首页路由配置
访问 `/app1`, `/app123`,`/app1/index.html`, `/app1/xxx`, `/xxxx` 都会路由到'/mfe/app1/{version}/index.html'

#### fileRouting：文件路由配置
下面文件映射均生效
- `/js/a.js` => `/mfe/app1/v1.0.0/js/a.js`
- `/js/template/a.js` => `/mfe/app1/v1.0.0/js/template/a.js`
- `/app1/js/a.js` => `/mfe/app1/v1.0.0/js/a.js`
- `/app1/js/template/a.js` => `/mfe/app1/v1.0.0/js/template/a.js`


### 往HTML首页注入代码
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
通过 `injection`往HTML首页注入代码，可以在`head`标签注入代码，也可以在`body`标签的`first`和`last`位置注入代码。
