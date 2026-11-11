# 非 K3s 代理与直连对照

日期：2026-10-25，Asia/Shanghai。

## 对照条件

- Oracle 上运行中的 `grok2api-go`，镜像摘要 `e028750874a8be4732b37cc2cc2f9be0456a33324069462cf17454c0b00bbd28`。
- 账号 14095、Console 节点 13，与 10:45:47 的拦截日志对应。
- 在主容器的网络命名空间内执行 curl 7.76.1，两组使用相同 User-Agent 和 SSO。
- 代理使用该节点实际配置及账号绑定身份，连接 `x2api-resin:2260`；直连显式关闭代理。
- 当前 Clearance 模式为 manual，该节点和账号没有 Clearance Cookie。
- Cookie 与重定向按域名隔离；没有自动完成交互式账号关联，没有写入生产配置、账号额度或会话缓存。

## 实际结果

| 项目 | 直连 | 代理 |
| --- | --- | --- |
| grok.com trace 返回的出口 IP | 140.245.91.120 | 45.3.52.151 |
| TCP 连接目标 | Cloudflare 地址，443 | 172.21.0.3，2260 |
| HTTP CONNECT | 不使用 | 200 |
| `/dashboard/api/auth/login` | 302 | 302 |
| WorkOS 会话 | 获得完整 Cookie 组合 | 获得完整 Cookie 组合 |
| `/dashboard/api/dashboard/v2/usage-status?scope=user` | 200，正确额度 JSON | 200，正确额度 JSON |
| `budgetCents` | 100 | 100 |
| `usedCents` | 0 | 0 |

完整链路的登录请求与额度读取请求，本轮均没有出现 Cloudflare 拦截页。

额度请求 CF-Ray：

- 直连：`a4fdfd25fc5cc255-NRT`。
- 代理：`a4fdfd831b9864a6-YYZ`。

## 页面提示的原因

生产日志在 10:45:47.514 记录节点 13 被 grok.com 拦截，并进入 1800 秒冷却；对应截止时间约为 11:15:47。11:06:34 检查时，容器没有重启，最近的拦截记录仍是这一条。

`console/quota.go` 在发起上游请求前检查冷却状态，`console/egress_breaker.go` 按节点 ID 共享冷却。因此冷却期间的“当前出口被 Cloudflare 拦截”不代表每次同步都进行了实时探测，也会影响使用同一节点的其他账号。

源码中的 x.ai 直连规则不匹配 grok.com。本轮代理连接及不同出口 IP 也确认了代理路径生效。

## 验证边界

curl 与生产服务的浏览器 TLS 客户端并不相同。本轮证明当前两条网络路径及该账号的真实额度读取可用，不能代替冷却结束后的正式应用同步验收。

没有清除熔断、重启服务、修改代理、写入额度或发起收费生成。

原始脱敏结果：`compare.jsonl`、`redirects.jsonl`、`runtime.log`。
