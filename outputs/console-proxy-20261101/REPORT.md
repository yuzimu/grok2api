# K3s Console 代理出站

日期：2026-11-01，Asia/Shanghai。

Console鉴权、图片/视频生成和素材下载使用配置代理。Console作用域及Console素材作用域不再应用x.ai默认直连规则；未配置代理时拒绝出站。Build和其他作用域沿用原有策略。

修改文件：

- `backend/internal/infra/egress/tlsclient.go`
- `backend/internal/infra/egress/manager.go`
- `backend/internal/infra/egress/direct.go`
- `README.md`
- `README.zh-CN.md`

## 部署

K3s namespace=xbrd，Deployment=grok2api，滚动更新完成：

```text
registry.cn-hongkong.aliyuncs.com/xbrd/grok2api@sha256:c2335fdaa345a20b63f6ce32c814722fe7d9ddec731703d41884c160893cd5b3
```

运行中二进制SHA-256：

```text
dbbf7af8fcb9a89415ddd63d66f4eb32f4d584f8d64fc52784a7b655d8650a6c
```

## 出口证据

代理节点6，`resin-gk-sticky`，SOCKS5服务 `resin.xbrd.svc.cluster.local:2260`，ClusterIP为 `10.43.98.242`。

在实际工作节点xbrd-flow进行的对照：

- 直连出口：198.46.187.4。
- 显式代理出口：45.3.44.250。
- 代理连接peer：10.43.98.242。
- 账号86101经代理进行Console DPoP鉴权返回200；一次1秒480p视频创建返回402 `personal-team-blocked:spending-limit`，未产生视频任务。该结果不证明原400已修复，但证明此代理路径可完成鉴权且该次生成受到额度/订阅拒绝。

部署后，在实际应用Pod网络命名空间抓取了12个出站TCP包：

```text
10.42.2.18:45354 -> 10.43.98.242:2260
10.42.2.18:37798 -> 10.43.98.242:2260
```

抓包只记录连接元数据，不记录认证负载。代码作用域检查与生产代理连接证据共同核验Console出站策略；没有宣称所有请求都固定使用同一个公网代理IP。

## 边界

Linux AMD64编译和diff格式检查通过；未新增或运行自动化测试。本轮没有为图片另外发起生成。Console视频通过既有固定账号任务队列继续进行一次性最小探测，最终账号汇总见 `../console-video-risk-20261101/`。

本次仅更新K3s，未更新Oracle，未提交或推送源码。
