<a name="readme-top"></a>
<h1 align="center">
    <img src="https://img.alicdn.com/imgextra/i2/O1CN01NwxLDd20nxfGBjxmZ_!!6000000006895-2-tps-960-290.png" alt="Higress" width="240" height="72.5">
  <br>
  AIゲートウェイ
</h1>
<h4 align="center"> AIネイティブAPIゲートウェイ </h4>

[![Build Status](https://github.com/alibaba/higress/actions/workflows/build-and-test.yaml/badge.svg?branch=main)](https://github.com/alibaba/higress/actions)
[![license](https://img.shields.io/github/license/alibaba/higress.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![CNCF Sandbox](https://img.shields.io/badge/CNCF-Sandbox-30638E?logo=linuxfoundation&logoColor=white)](https://www.cncf.io/projects/)

[**公式サイト**](https://higress.cn/) &nbsp; |
&nbsp; [**ドキュメント**](https://higress.cn/docs/latest/overview/what-is-higress/) &nbsp; |
&nbsp; [**ブログ**](https://higress.cn/blog/) &nbsp; |
&nbsp; [**電子書籍**](https://higress.cn/docs/ebook/wasm14/) &nbsp; |
&nbsp; [**開発ガイド**](https://higress.cn/docs/latest/dev/architecture/) &nbsp; |
&nbsp; [**AIプラグイン**](https://higress.cn/plugin/) &nbsp;


<p>
   <a href="README.md"> English </a> | <a href="README_ZH.md">中文</a> | 日本語
</p>


## Higressとは？

Higressは、IstioとEnvoyをベースにしたクラウドネイティブAPIゲートウェイで、Go/Rust/JSなどを使用してWasmプラグインを作成できます。数十の既製の汎用プラグインと、すぐに使用できるコンソールを提供しています（デモは[こちら](http://demo.higress.io/)）。

### 主な使用シナリオ

HigressのAIゲートウェイ機能は、国内外のすべての[主要モデルプロバイダー](https://github.com/alibaba/higress/tree/main/plugins/wasm-go/extensions/ai-proxy/provider)をサポートし、vllm/ollamaなどに基づく自己構築DeepSeekモデルにも対応しています。また、プラグインメカニズムを通じてMCP（Model Context Protocol）サーバーをホストすることもでき、AI Agentが様々なツールやサービスを簡単に呼び出せるようにします。[openapi-to-mcpツール](https://github.com/higress-group/openapi-to-mcpserver)を使用すると、OpenAPI仕様を迅速にリモートMCPサーバーに変換してホスティングできます。HigressはLLM APIとMCP APIの統一管理を提供します。

**🌟 今すぐ[https://mcp.higress.ai/](https://mcp.higress.ai/)で体験**してください。HigressがホストするリモートMCPサーバーを直接体験できます:

![Higress MCP Server Platform](https://img.alicdn.com/imgextra/i2/O1CN01nmVa0a1aChgpyyWOX_!!6000000003294-0-tps-3430-1742.jpg)

### 企業での採用

Higressは、Tengineのリロードが長時間接続のビジネスに影響を与える問題や、gRPC/Dubboの負荷分散能力の不足を解決するために、Alibaba内部で誕生しました。Alibaba Cloud内では、HigressのAIゲートウェイ機能がTongyi Qianwen APP、Tongyi Bailian Model Studio、機械学習PAIプラットフォームなどの中核的なAIアプリケーションをサポートしています。また、国内の主要なAIGC企業（例：ZeroOne）やAI製品（例：FastGPT）にもサービスを提供しています。Alibaba Cloudは、Higressを基盤にクラウドネイティブAPIゲートウェイ製品を構築し、多くの企業顧客に99.99％のゲートウェイ高可用性保証サービスを提供しています。


## 目次

- [**クイックスタート**](#クイックスタート)    
- [**機能紹介**](#機能紹介)
- [**使用シナリオ**](#使用シナリオ)
- [**主な利点**](#主な利点)
- [**コミュニティ**](#コミュニティ)

## クイックスタート

HigressはDockerだけで起動でき、個人開発者がローカルで学習用にセットアップしたり、簡易サイトを構築するのに便利です。

```bash
# 作業ディレクトリを作成
mkdir higress; cd higress
# Higressを起動し、設定ファイルを作業ディレクトリに書き込みます
docker run -d --rm --name higress-ai -v ${PWD}:/data \
        -p 8001:8001 -p 8080:8080 -p 8443:8443  \
        higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/all-in-one:latest
```

リスンポートの説明は以下の通りです：

- 8001ポート：Higress UIコンソールのエントリーポイント
- 8080ポート：ゲートウェイのHTTPプロトコルエントリーポイント
- 8443ポート：ゲートウェイのHTTPSプロトコルエントリーポイント

**HigressのすべてのDockerイメージは専用のリポジトリを使用しており、Docker Hubの国内アクセス不可の影響を受けません**

K8sでのHelmデプロイなどの他のインストール方法については、公式サイトの[クイックスタートドキュメント](https://higress.cn/docs/latest/user/quickstart/)を参照してください。


## 使用シナリオ

- **AIゲートウェイ**:

  Higressは、国内外のすべてのLLMモデルプロバイダーと統一されたプロトコルで接続でき、豊富なAI可観測性、多モデル負荷分散/フォールバック、AIトークンフロー制御、AIキャッシュなどの機能を備えています。

  ![](https://img.alicdn.com/imgextra/i1/O1CN01fNnhCp1cV8mYPRFeS_!!6000000003605-0-tps-1080-608.jpg)

- **MCP Server ホスティング**:

  Higressは、EnvoyベースのAPIゲートウェイとして、プラグインメカニズムを通じてMCP Serverをホストすることができます。MCP（Model Context Protocol）は本質的にAIにより親和性の高いAPIであり、AI Agentが様々なツールやサービスを簡単に呼び出せるようにします。Higressはツール呼び出しの認証、認可、レート制限、可観測性などの統一機能を提供し、AIアプリケーションの開発とデプロイを簡素化します。

  ![](https://img.alicdn.com/imgextra/i3/O1CN01K4qPUX1OliZa8KIPw_!!6000000001746-2-tps-1581-615.png)

  Higressを使用してMCP Serverをホストすることで、以下のことが実現できます：
  - 統一された認証と認可メカニズム、AIツール呼び出しのセキュリティを確保
  - きめ細かいレート制限、乱用やリソース枯渇を防止
  - 包括的な監査ログ、すべてのツール呼び出し行動を記録
  - 豊富な可観測性、ツール呼び出しのパフォーマンスと健全性を監視
  - 簡素化されたデプロイと管理、Higressのプラグインメカニズムを通じて新しいMCP Serverを迅速に追加
  - 動的更新による無停止：Envoyの長時間接続に対する友好的なサポートとWasmプラグインの動的更新メカニズムにより、MCP Serverのロジックをリアルタイムで更新でき、トラフィックに完全に影響を与えず、接続が切断されることはありません

- **Kubernetes Ingressゲートウェイ**:

  HigressはK8sクラスターのIngressエントリーポイントゲートウェイとして機能し、多くのK8s Nginx Ingressの注釈に対応しています。K8s Nginx IngressからHigressへのスムーズな移行が可能です。
  
  [Gateway API](https://gateway-api.sigs.k8s.io/)標準をサポートし、ユーザーがIngress APIからGateway APIにスムーズに移行できるようにします。

  ingress-nginxと比較して、リソースの消費が大幅に減少し、ルーティングの変更が10倍速く反映されます。

  ![](https://img.alicdn.com/imgextra/i1/O1CN01bhEtb229eeMNBWmdP_!!6000000008093-2-tps-750-547.png)
  ![](https://img.alicdn.com/imgextra/i1/O1CN01bqRets1LsBGyitj4S_!!6000000001354-2-tps-887-489.png)
  
- **マイクロサービスゲートウェイ**:

  Higressはマイクロサービスゲートウェイとして機能し、Nacos、ZooKeeper、Consul、Eurekaなどのさまざまなサービスレジストリからサービスを発見し、ルーティングを構成できます。
  
  また、[Dubbo](https://github.com/apache/dubbo)、[Nacos](https://github.com/alibaba/nacos)、[Sentinel](https://github.com/alibaba/Sentinel)などのマイクロサービス技術スタックと深く統合されています。Envoy C++ゲートウェイコアの優れたパフォーマンスに基づいて、従来のJavaベースのマイクロサービスゲートウェイと比較して、リソース使用率を大幅に削減し、コストを削減できます。

  ![](https://img.alicdn.com/imgextra/i4/O1CN01v4ZbCj1dBjePSMZ17_!!6000000003698-0-tps-1613-926.jpg)
  
- **セキュリティゲートウェイ**:

  Higressはセキュリティゲートウェイとして機能し、WAF機能を提供し、key-auth、hmac-auth、jwt-auth、basic-auth、oidcなどのさまざまな認証戦略をサポートします。 

## 主な利点

- **プロダクションレベル**

  Alibabaで2年以上のプロダクション検証を経た内部製品から派生し、毎秒数十万のリクエストを処理する大規模なシナリオをサポートします。

  Nginxのリロードによるトラフィックの揺れを完全に排除し、構成変更がミリ秒単位で反映され、ビジネスに影響を与えません。AIビジネスなどの長時間接続シナリオに特に適しています。

- **ストリーム処理**

  リクエスト/レスポンスボディの完全なストリーム処理をサポートし、Wasmプラグインを使用してSSE（Server-Sent Events）などのストリームプロトコルのメッセージをカスタマイズして処理できます。

  AIビジネスなどの大帯域幅シナリオで、メモリ使用量を大幅に削減できます。  
    
- **拡張性**

  AI、トラフィック管理、セキュリティ保護などの一般的な機能をカバーする豊富な公式プラグインライブラリを提供し、90％以上のビジネスシナリオのニーズを満たします。

  Wasmプラグイン拡張を主力とし、サンドボックス隔離を通じてメモリの安全性を確保し、複数のプログラミング言語をサポートし、プラグインバージョンの独立したアップグレードを許可し、トラフィックに影響を与えずにゲートウェイロジックをホットアップデートできます。

- **安全で使いやすい**
  
  Ingress APIおよびGateway API標準に基づき、すぐに使用できるUIコンソールを提供し、WAF保護プラグイン、IP/Cookie CC保護プラグインをすぐに使用できます。

  Let's Encryptの自動証明書発行および更新をサポートし、K8sを使用せずにデプロイでき、1行のDockerコマンドで起動でき、個人開発者にとって便利です。


## 機能紹介

### AIゲートウェイデモ展示

[OpenAIから他の大規模モデルへの移行を30秒で完了
](https://www.bilibili.com/video/BV1dT421a7w7/?spm_id_from=333.788.recommend_more_video.14)


### Higress UIコンソール
    
- **豊富な可観測性**

  すぐに使用できる可観測性を提供し、Grafana＆Prometheusは組み込みのものを使用することも、自分で構築したものを接続することもできます。

  ![](./docs/images/monitor.gif)
    

- **プラグイン拡張メカニズム**

  公式にはさまざまなプラグインが提供されており、ユーザーは[独自のプラグインを開発](./plugins/wasm-go)し、Docker/OCIイメージとして構築し、コンソールで構成して、プラグインロジックをリアルタイムで変更できます。トラフィックに影響を与えずにプラグインロジックをホットアップデートできます。

  ![](./docs/images/plugin.gif)


- **さまざまなサービス発見**

  デフォルトでK8s Serviceサービス発見を提供し、構成を通じてNacos/ZooKeeperなどのレジストリに接続してサービスを発見することも、静的IPまたはDNSに基づいて発見することもできます。

  ![](./docs/images/service-source.gif)
    

- **ドメインと証明書**

  TLS証明書を作成および管理し、ドメインのHTTP/HTTPS動作を構成できます。ドメインポリシーでは、特定のドメインに対してプラグインを適用することができます。

  ![](./docs/images/domain.gif)


- **豊富なルーティング機能**

  上記で定義されたサービス発見メカニズムを通じて、発見されたサービスはサービスリストに表示されます。ルーティングを作成する際に、ドメインを選択し、ルーティングマッチングメカニズムを定義し、ターゲットサービスを選択してルーティングを行います。ルーティングポリシーでは、特定のルーティングに対してプラグインを適用することができます。

  ![](./docs/images/route-service.gif)


## コミュニティ

### 行動規範（Code of Conduct）

Higress コミュニティは
[**CNCF Code of Conduct**](https://github.com/cncf/foundation/blob/main/code-of-conduct.md)
に従っています。プロジェクトが採用している行動規範は
[`CODE_OF_CONDUCT.md`](./CODE_OF_CONDUCT.md) に記載されていますので、コミュニティに参加する前にご確認ください。

### ガバナンスとメンテナー

プロジェクトのガバナンス、メンテナー一覧、コントリビューションモデルについては
[`GOVERNANCE.md`](./GOVERNANCE.md) と [`MAINTAINERS.md`](./MAINTAINERS.md) を参照してください。
初めて貢献される方は [`CONTRIBUTING_EN.md`](./CONTRIBUTING_EN.md) からお読みください。

### セキュリティ

セキュリティに関する脆弱性は [`SECURITY.md`](./SECURITY.md) に記載された手順に従って報告してください。

### 感謝

EnvoyとIstioのオープンソースの取り組みがなければ、Higressは実現できませんでした。これらのプロジェクトに最も誠実な敬意を表します。

### 交流グループ

![image](https://img.alicdn.com/imgextra/i2/O1CN01BkopaB22ZsvamFftE_!!6000000007135-0-tps-720-405.jpg)

### 技術共有

WeChat公式アカウント：

![](https://img.alicdn.com/imgextra/i1/O1CN01WnQt0q1tcmqVDU73u_!!6000000005923-0-tps-258-258.jpg)

### 関連リポジトリ

- Higressコンソール：https://github.com/higress-group/higress-console
- Higress（スタンドアロン版）：https://github.com/higress-group/higress-standalone
- Higress Plugin Server：https://github.com/higress-group/plugin-server
- Higress Wasm Plugin Golang SDK：https://github.com/higress-group/wasm-go

### 貢献者

<a href="https://github.com/alibaba/higress/graphs/contributors">
  <img alt="contributors" src="https://contrib.rocks/image?repo=alibaba/higress"/>
</a>

### スターの歴史

[![スターの歴史チャート](https://api.star-history.com/svg?repos=alibaba/higress&type=Date)](https://star-history.com/#alibaba/higress&Date)

---

## Cloud Native Computing Foundation

<a href="https://www.cncf.io/projects/" target="_blank">
  <img src="https://raw.githubusercontent.com/cncf/artwork/master/other/cncf/horizontal/color/cncf-color.svg" alt="Cloud Native Computing Foundation" width="300"/>
</a>

Higress は [Cloud Native Computing Foundation](https://www.cncf.io/) のサンドボックスプロジェクトです。

The Linux Foundation® (TLF) は登録商標を所有し使用しています。LF の商標一覧については
[Trademark Usage](https://www.linuxfoundation.org/legal/trademark-usage) をご参照ください。

Copyright Higress a Series of LF Projects, LLC. ウェブサイトの利用規約、商標ポリシー、
その他のプロジェクトポリシーについては
[https://lfprojects.org/policies/](https://lfprojects.org/policies/) をご参照ください。

<p align="right" style="font-size: 14px; color: #555; margin-top: 20px;">
    <a href="#readme-top" style="text-decoration: none; color: #007bff; font-weight: bold;">
        ↑ トップに戻る ↑
    </a>
</p>
