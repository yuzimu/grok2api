# Build 选号与自动额度刷新

日期：2026-11-01，Asia/Shanghai。

## 线上诊断

K3s 任务 `video_KLT0McyffRuRcQn30S9qKOEv`：

- request_id：`OOKy3B4nAqaJM6HY`
- 账号：85067
- 失败阶段：`video_create`
- 上游：HTTP 403，`personal-team-blocked:spending-limit`
- 审计只有一次上游尝试。

视频路径将 Build Super 的403按普通拒绝结束，未使用聊天链路的额度耗尽处理。Build 使用 Billing 配额，原后台刷新队列不接受 Billing，活跃账号的账单快照可能长期未更新。

另有5个近期视频失败为上游 `image_download_interrupted`，属于已提交任务的图片下载失败，本次未重建这些任务。

## 当前行为

- 聊天、视频共用 `applyAccountQuotaFailure`，统一处理模型额度、免费额度和账号支付额度耗尽。
- Build 视频创建阶段明确收到402或403 spending-limit时，记录额度恢复状态、使选号缓存失效并尝试其他候选；仍遵循重试次数、密钥范围及固定账号约束。
- 已提交任务的轮询或后处理失败不换号重建，避免重复生成。
- Build 聊天、视频请求及额度耗尽事件加入现有后台刷新队列。账单刷新合并限频、使用现有工作池和跨实例锁，不要求手动点击刷新。
- 新账单确认耗尽时校正恢复时间；显示可用的延迟快照不会解除已确认的额度阻断。恢复继续使用现有到期探测机制。
- 本次没有手工修改账号凭据或额度，也未重置历史任务。

源码：

- `backend/internal/application/account/service.go`
- `backend/internal/application/gateway/service.go`
- `backend/internal/application/gateway/video.go`
- `backend/internal/application/gateway/failure.go`

## 已部署

K3s，namespace `xbrd`，Deployment `grok2api`：

```text
registry.cn-hongkong.aliyuncs.com/xbrd/grok2api@sha256:e0a86b3199e8a08093599e60ab8266fcb63bbed3630c3db0a577a1553a823b95
```

Oracle，`/opt/grok2api-go`，容器 `grok2api-go`：

```text
grok2api:build-quota-20261101-r2
sha256:c95d5614d12801dfe368b78c26b8a60b712714e65d07e45c4016b21c6f375763
```

Oracle 镜像为主机本地构建，不能假定能从公共仓库拉取。原环境文件、Compose、配置备份在 `/opt/grok2api-go/backups/build-quota-20261101/`。业务配置和Compose的SHA-256与备份一致。K3s更新前Deployment保存在 `k3s-before.yaml`。

最终二进制与运行容器内校验一致：

- AMD64：`157f0d5627f3e4962b52d36da76597953f10ae5ae0ae239046e3f9ee0f230b6b`
- ARM64：`53df4b84408833f111745274012e42548d6597c2afe8fef28539f2bacff29170`

## 正式入口验证

请求入口：`POST /v1/chat/completions`

```json
{
  "model": "Build/grok-4.5",
  "messages": [{"role": "user", "content": "Reply exactly OK."}],
  "max_tokens": 16,
  "stream": false
}
```

### K3s

- 最终版本 request_id：`aNeJTGoLeDFFheoi`
- HTTP 200，实际回答 `OK`。
- 审计：provider=`grok_build`，account_id=85066。
- 日志 `build_billing_refreshed`：2026-11-01 09:03:51，账号85066，用量34%。
- 数据库同步时间：`2026-11-01T01:03:51.718297Z`，由后台自动更新。
- 账号85067仍为100%、`exhausted`，next_probe=`2026-11-02T13:40:21.499672Z`。

### Oracle

- request_id：`7b_9bzCcCvUmQbov`
- 账号14049上游返回402：`Grok Build usage balance exhausted`。
- 审计下一条为 `no_accounts`：没有可用上游账号；客户端收到503。
- 该账号此前快照为23%，同步时间为2026-10-15。
- 日志 `build_billing_refreshed`：2026-11-01 09:04:20，账号14049，用量100%。
- 数据库同步时间更新为 `2026-11-01T01:04:20.141818Z`，状态为 `exhausted`，next_probe=`2026-11-03T02:56:28.404128Z`。
- 此结果验证了自动刷新、耗尽摘号及后续选号路径；该路由当时无可用候选，不能宣称Oracle聊天生成成功。

## 验证边界

- Linux AMD64、ARM64均编译通过，diff格式检查通过。
- 未新增、改写或运行自动化测试，未提交或推送源码。
- 未新增收费图片或视频生成。视频403后实际换号并完成新视频的端到端结果未验证；修复依据真实线上创建阶段错误、共享处理代码及上述正式聊天入口证据。
- 已保存的失败视频不会因部署自动重新生成。
