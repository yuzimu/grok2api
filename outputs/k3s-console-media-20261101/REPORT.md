# K3s Console 媒体实测

日期：2026-11-01，Asia/Shanghai。

使用用户提供的客户端 Key，通过 K3s 正式 API 发起单张图片和15秒视频请求。凭据未写入本报告。使用显式 Console 模型；未修改路由、账号、代理或额度配置。

## 视频

- 入口：`POST /v1/videos`
- 模型：`Console/grok-imagine-video-1.5`
- 参数：duration=15，aspect_ratio=16:9，resolution=480p。
- 提示词：`A red ceramic teapot on a white table. A slow smooth camera move from left to right, consistent studio lighting, no text.`
- 任务：`video_VL6LBSLTJTQQp4Jde5NtilZk`
- request_id：`fQoJK9NdUyNW-87C`
- 实际 provider：`grok_console`，account_id=86094。
- 原任务轮询最终为 completed，progress=100，seconds=15。
- 创建时间1793495902，完成时间1793495945，耗时43秒。
- 产物：`https://grok.ai.hxeos.com/v1/media/videos/vid_KUUGth9tJAQayHy-XqMPJVj5KJ9ofCg0`
- 无 Authorization/Cookie 下载HTTP 200，1394577字节。
- ffprobe确认：H.264，848×480，361帧，时长15.041667秒；AAC音轨15秒。
- ffmpeg解码及抽帧成功；已查看 `video-contact.jpg`，存在有效茶壶画面及视角变化。
- 本地文件：`video.mp4`。

## 图片

- 入口：`POST /v1/images/generations`
- 模型：`Console/grok-imagine-image`
- 参数：n=1，resolution=1k，aspect_ratio=1:1，response_format=url。
- 提示词：`A red ceramic teapot on a white table, clean studio product photograph, no text.`
- request_id：`aOXVOlaudviXPW_6`
- 实际 provider：`grok_console`，account_id=86095。
- 客户端HTTP 503，`upstream_unavailable`。
- 审计记录上游HTTP 402，无图片产物；该次审计没有保留原始错误正文，因此不推断更具体的计费拒绝原因。

## 结论与边界

账号86094的15秒Console视频生成成功；账号86095的本次图片生成失败。额度查询被Cloudflare拦截不能直接推出媒体生成也不可用。仅验证上述两个被实际选中的账号，未遍历全部账号，未重复提交视频，也未将图片失败当作成功。

## 720p、15秒验证

720p、15秒使用同一客户端Key及提示词，模型为 `Console/grok-imagine-video-1.5`，resolution=720p。实际结果如下：

| 任务 | request_id | 实际账号 | 结果 |
| --- | --- | --- | --- |
| `video_X63lKJZYtMiMzAZNRUwTbFDW` | `XcboIpyLHblETgYE` | 86101 | 上游HTTP 400，Incorrect API key provided |
| `video_lvZGsUC8zV8geT8rNlUPVcuu` | `guNmCXswWmKIVi0m` | 86102 | 上游HTTP 400，Incorrect API key provided |
| `video_SfZm7diW-tWRCzcbDHr8DGhW` | `odT80bHaK7hri1fI` | 86108 | completed，实际720p、15.04秒 |

前两个任务均明确失败且没有产物；后续经用户再次明确授权提交一条任务成功，未重复提交任何仍在运行或结果不明的任务。API Key错误来自Console上游账号，不能据此否定720p参数支持。

成功任务的创建时间为1793496667，完成时间为1793496763，耗时96秒。结果URL：

`https://grok.ai.hxeos.com/v1/media/videos/vid_O-DH10RAEGbITTXGjqcYRv4MYz0ncuZ7`

匿名下载HTTP 200，3147270字节，保存为 `video-720p.mp4`。ffprobe确认H.264、1280×720、361帧、15.041667秒，AAC音轨15秒。ffmpeg解码及抽帧成功，已查看 `video-720p-contact.jpg`。

## 720p、10秒及额度核对

- 模型及提示词不变，duration=10，resolution=720p。
- 任务：`video_6qcynIFU8ZYIPHSdx5Ioj1eZ`
- request_id：`fCR2Ud_CqOafIuNT`
- 实际账号86109，provider=`grok_console`。
- 最终failed：video_create阶段上游HTTP 400，`Incorrect API key provided`。无产物，不构成10秒额度门槛验证。

当前代码内置估价：1.5模型720p每秒1400000000 ticks，10000000000 ticks为1美元，即15秒约2.10美元，10秒约1.40美元。此为系统估算，不是本次实际账单金额。

数据库确认成功账号86094和86108的console额度窗口均为total=100、remaining=0、usage_percent=100。额度转换逻辑会将负剩余额度截为0、使用率封顶100，故数据库及页面不能区分恰好用完1美元还是超过1美元；没有原始usedCents或上游账单，不能认定实际扣了2.10美元。

审计确认720p、15秒成功任务在到达86108之前，依次收到86103至86107的HTTP 402：`You have run out of credits or need a Grok subscription`。这些账号的窗口快照仍显示remaining=100、total=100，但恢复状态为exhausted，因此出现页面0/1美元却待重置。该状态来自上游拒绝，不是成功视频被扣到了这些账号；上游错误包含余额或订阅两种可能，不能仅据此归因为单次请求超预算。

## 暂停账号指定规格核验

固定选择两个 `recovery=exhausted`、console窗口remaining=100的账号，计划分别请求：

| 账号 | 目标规格 | 实际到达阶段 | 结果 |
| --- | --- | --- | --- |
| 86096 / 05f175db | 720p、7秒 | Console `/v1/dpop/token` | HTTP 403，Cloudflare HTML |
| 86103 / c0a176cc | 480p、15秒 | Console `/v1/dpop/token` | HTTP 403，Cloudflare HTML |

现有管理端固定账号视频入口硬编码6秒，无法表达本次规格。上述核验采用Console上游正式鉴权入口，固定账号凭据；最终请求从Pod所在节点 `xbrd-flow` 发出，客户端为隔离的Chrome 146 TLS客户端，未改变生产服务配置。

这两次均未进入视频创建接口，没有上游视频request_id和产物。不能将鉴权失败当成7秒720p或15秒480p的生成失败，也不能据此判断额度门槛。未解除账号暂停、未改写额度或替换凭据。
