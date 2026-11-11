# Console 同步故障处理

日期：2026-10-24，Asia/Shanghai。

## 当前实现

- `backend/internal/infra/provider/console/workos.go`：只有明确的 `cursorMerged=false` 才返回未关联错误。字段缺失返回未知状态，不触发账号失效或自动删除。
- WorkOS 跳转中的 Cloudflare 403 优先归为出口故障。其他域名的拦截不会清除 grok.com 的 Clearance。
- `backend/internal/application/account/service.go`：后台额度刷新先检查账号状态。已删除、停用或待认证账号不再请求上游；沿现有共享代次与 CAS 流程结束刷新。
- 手动刷新保留正式入口，并记录 `account_quota_sync_failed`，便于关联账号与上游错误。
- `backend/internal/infra/provider/console/quota.go`：出口拦截日志包含触发账号 ID。共享节点仍采用节点级指数冷却。
- `backend/internal/transport/http/account/handler.go`：明确未关联返回 HTTP 409 / `consoleAccountUnlinked`；出口冷却返回 HTTP 503 / `accountEgressBlocked`，并提供 `Retry-After`。

## 发布与备份

- 目标：K3s `xbrd/grok2api` 主容器，保留先前匿名图片、视频下载与视频终态修复。
- 镜像：`registry.cn-hongkong.aliyuncs.com/xbrd/grok2api@sha256:787a101f6e5519636c70b28af4842cce92370751eaef8d1966444fb77df43128`。
- 完整源码：本目录 `source.tar.gz`，SHA-256 `0d28d53ededb8b1344056b7921f6e38d644fcff49e3ff88b27af9e242f58db2e`。
- 已跟踪文件补丁：本目录 `fix.patch`；新增视频终态收据文件包含在完整源码包中。
- 远程备份：`xbrd-newapi:/opt/grok2api-ci/backups/console-recovery-20261024.rpptAA/`，包含部署配置和 Console 账号状态快照，不含账号令牌。
- 修复未提交或推送 Git，未修改共享数据库部署，未删除账号、批量恢复账号或新发收费媒体。

## 实际验证

- Linux amd64 镜像编译通过，未运行测试。
- 最终 Pod `grok2api-6f88dbfccd-qtgc4` 为 `2/2 Running`，重启 0，镜像摘要与发布目标一致；活动视频任务数为 0。
- 线上 `quota_refresh_events / refresh / inactive` 在 17:10:54、17:11:54 分别记录 550、10 条，共 560 条无效账号刷新结束。
- 账号 66401 的正式刷新日志明确为出口被 Cloudflare 拦截，额度同步进入冷却，未取得新的额度快照；关联请求 ID `e522t0_uCXd4Pup4`。
- 最终版本通过 `POST /api/admin/v1/accounts/66414/refresh-quota` 返回 HTTP 409 / `consoleAccountUnlinked`，明确提示需要上游关联并核验额度；请求 ID `2b7fzQQU0sw1Hwog`。
- 该账号的上游关联检查明确为 `cursorMerged=false`，不是缺失字段。不能根据这个样本判定其他账号。
- 相关日志保存于本目录 `runtime-evidence.log`、`final-runtime-evidence.log`。

## 剩余限制

Console 尚未恢复上游可用性。节点 6 的实际代理用户名为 `gk`，没有账号占位符；当前为手动 Clearance，节点没有 Clearance Cookie，配置中的 FlareSolverr 地址没有对应的运行服务。

当前只确认样本账号 66414 明确未关联，不能据此判定全部历史标记，因此未批量改写这些账号。明确未关联的账号需要先通过上游正式流程完成关联并核验额度；仍被 Cloudflare 拦截的账号需要可用出口或与实际出口匹配的有效 Clearance。不能根据 403 推断账号无额度，也不能伪造关联状态或额度。
