# Web 签名与额度对照

2026-11-10，从K3s取账号29174的SSO，仅置于本地进程内存。使用Chrome146 TLS客户端，经临时SSH隧道连接原节点5的账号粘性代理。诊断结束已关闭隧道。

同一账号、同一代理、同一请求体对照：

| 请求 | 签名器现有公式、新时间戳 | 不带签名 |
| --- | --- | --- |
| POST /rest/media/imagine/quota_info | 200，video720p unavailable，remainingQueries=0 | 200，相同额度 |
| POST /rest/app-chat/conversations/new，6秒720p | 429，code 8 Too many requests | 429，code 8 Too many requests |

两次视频请求都明确被拒绝，无成片。原始脱敏回执见signature-comparison.json。

当前认证首页及Imagine页面不含旧grok-site-verification和curves。扫描Imagine页面声明的139个CDN脚本，没有命中旧签名盐、x-statsig-id或verification字符串；此静态检查仅覆盖所列139个脚本，不代表全部动态资源。

Oracle签名器仍持有2026-10-13的pair，监控持续capture incomplete、repair_ok=false。此为独立的旧协议失配；现有对照无法支持“429由签名造成”，上游对该账号明确返回视频额度0。

本地已调整Web媒体POST和Imagine额度请求，取消旧签名步骤及对应的签名刷新重放，保留Cloudflare错误识别。只运行带integration标签且精确匹配TestIntegrationUnsignedWebMedia的集成验证，通过真实SQLite、出口管理器和HTTP入口检查无签名额度读取、429及403不重放。未运行单元测试。

这些源码改动尚未部署。用户后续要求关闭Web图片并使用Build，当前工作转为线上路由调整；没有为签名改动重启生产服务，也没有恢复已关闭的Web视频。
