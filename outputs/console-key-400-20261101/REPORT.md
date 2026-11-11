# Console 400 Key 拒绝与账号轮换

日期：2026-11-01，Asia/Shanghai。

## 线上证据

K3s正式视频链路记录了以下创建阶段错误：

| request_id | 账号 | 上游状态 | 错误 |
| --- | --- | --- | --- |
| XcboIpyLHblETgYE | 86101 | 400 | Incorrect API key provided |
| guNmCXswWmKIVi0m | 86102 | 400 | Incorrect API key provided |
| fCR2Ud_CqOafIuNT | 86109 | 400 | Incorrect API key provided |

三个账号均仍enabled、auth_status=active，存在缓存的Console API Key ID，额度窗口remaining=100、total=100。错误发生于video_create，不是Cloudflare登录403，也不是客户端传入的g2a Key认证失败。

## 采用的修复

参考用户提供的上游PR #1094：

- 标题：`fix(gateway): rotate accounts when upstream rejects the API key`
- 核验时状态：open、未合并。
- base：`e2fd7ab481a30a1965bb61395d437154cf247c92`
- head：`5cb0f56c42f3a3f7a1cbfac3c244a9225e8c06d2`
- 通过GitHub API核对元数据，通过Git PR ref取得实际diff。

接入PR的生产逻辑，未修改或运行其测试文件：

- 400且错误明确包含Incorrect API key时，分类为账号级错误。
- 聊天链路允许账号轮换，即使上游附带X-Should-Retry:false。
- 普通参数400仍直接返回。
- 视频创建阶段复用相同分类：冷却当前账号、排除当前请求内的该账号，并按现有次数限制选择后续账号。
- 已提交任务的轮询或后处理错误不重建视频；固定账号探测不轮换。

本次没有自动创建或更换Console Key，也没有解除额度暂停状态。修复解决的是错误分类和换号，不证明被拒绝的Key本身恢复。

源码范围：

- `backend/internal/application/gateway/failure.go`
- `backend/internal/application/gateway/service.go`
- `backend/internal/application/gateway/video.go`

## K3s部署

namespace=xbrd，Deployment=grok2api。

最终镜像：

```text
registry.cn-hongkong.aliyuncs.com/xbrd/grok2api@sha256:162610bbd5a62d455fa062a5a870506a4ee137e1d759c8369b3343e0490666c2
```

运行中二进制与本地构建SHA-256一致：

```text
28f8baeb1d1d7644e60d7c1be428d49d499e11fd099bc028899e2bf9922b6b56
```

部署前无活动视频任务。滚动发布完成，更新前Deployment保存在 `k3s-before.yaml`。本次未更新Oracle。

## 验证与边界

- Linux AMD64编译通过，diff格式检查通过。
- 正式GET `/v1/videos/video_SfZm7diW-tWRCzcbDHr8DGhW` 返回completed和原720p、15秒产物。
- 正式GET `/v1/videos/video_6qcynIFU8ZYIPHSdx5Ioj1eZ` 返回原failed记录和400错误，没有自动重建历史任务。
- 未新增收费生成，因此400后换号并成功生成的端到端行为尚未实测。
- 未创建、修改或运行测试，未提交或推送源码。工作区原有改动保留。
