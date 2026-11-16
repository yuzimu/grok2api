# Build 视频编辑与视频延长 API

## 环境与鉴权

| 环境 | Base URL |
| --- | --- |
| K3s | `https://grok.ai.hxeos.com` |
| Oracle（非 K3s） | `https://grok.mooo.si` |

创建和查询任务使用项目签发的 API Key：`Authorization: Bearer <API_KEY>`。
无需向客户端提供 Build OAuth、SSO 或上游 API Key。

```bash
export BASE_URL='https://grok.ai.hxeos.com'
export API_KEY='替换为项目 API Key'
export SOURCE_VIDEO_URL='https://你的域名/source.mp4'
```

示例使用 `curl` 和 `jq`。源视频 URL 必须可被上游通过 HTTPS 直接读取，
不应依赖 Cookie、请求头鉴权或很快过期的签名。

## 模型与路由

| `model` | 行为 |
| --- | --- |
| `Build/grok-imagine-video-1.5` | 固定选择 Build 渠道 |
| `grok-imagine-video-1.5` | 同名可用路由中优先选择 Build |
| `Console/grok-imagine-video` | 固定选择 Console 渠道 |

路由仍受模型启用状态、API Key 的模型权限、Provider 范围及账号可用性限制。
同名兼容 Console 路由可在无可用 Build 账号时被选择；
不保证跨模型名称切换，也不代表任务提交到 Build 后失败会自动重放至 Console。

Build 对外使用 `grok-imagine-video-1.5` 路由，出站编辑和延长模型均映射为
`grok-imagine-video`，使用 Build 账号的 OAuth 调用 xAI 对应接口。
编辑和延长不受账号 auto/build/xai **生成模式**影响。

## 视频编辑

`POST /v1/videos/edits`

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 建议 `Build/grok-imagine-video-1.5` |
| `prompt` | string | 是 | 描述对源视频的修改 |
| `video` | object | 是 | `{"url":"https://..."}` 或已有临时输入的 `{"file_id":"input_..."}` |

编辑不接受 `duration`、`seconds`、`size`、`aspect_ratio`、`resolution`、
`image`、`input_reference`、`last_frame`、`keyframes`、
`reference_images` 或 `reference_audios`。`file_id` 与 `url` 只能提供一个；
`file_id` 必须引用本环境中有效的视频临时输入。

```bash
jq -n --arg video "$SOURCE_VIDEO_URL" '{
  model: "Build/grok-imagine-video-1.5",
  prompt: "将整个视频改成黑白画面，保留原有主体、运动和镜头构图",
  video: {url: $video}
}' | curl --fail-with-body --silent --show-error \
  "$BASE_URL/v1/videos/edits" \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  --data-binary @-
```

成功创建返回 HTTP 200 和本地任务 ID：

```json
{"request_id":"video_..."}
```

这是异步受理结果，不代表视频已经完成。编辑不指定输出时长、比例或分辨率；
最终信息以成片及任务查询结果为准。

## 视频延长

`POST /v1/videos/extensions`

| 参数 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 建议 `Build/grok-imagine-video-1.5` |
| `prompt` | string | 是 | 描述后续动作及连续性要求 |
| `video` | object | 是 | 与编辑相同的源视频输入 |
| `duration` | integer | 否 | **新增**时长，2–10 秒，默认 6 秒 |

`duration` 不是成片总时长。标准 Build 延长不指定比例、分辨率，
也不传图片、首尾帧、关键帧或参考音频。
`seconds`、`size`、`input_reference` 属于生成入口的参数，不能用作编辑/延长别名。

```bash
jq -n --arg video "$SOURCE_VIDEO_URL" '{
  model: "Build/grok-imagine-video-1.5",
  prompt: "自然延续当前镜头，保持人物、场景、动作和摄影风格一致",
  video: {url: $video},
  duration: 2
}' | curl --fail-with-body --silent --show-error \
  "$BASE_URL/v1/videos/extensions" \
  -H "Authorization: Bearer $API_KEY" \
  -H 'Content-Type: application/json' \
  --data-binary @-
```

响应同样为 `{"request_id":"video_..."}`。

`video_extension_start_time` 用于 **Web** 延长的绝对续接位置，不是标准 Build 参数。
提供大于 0 的值时会按 Web 延长路由处理，不选择 Build/Console；
Web 新增片段长度为 6–10 秒，并要求相应 Web 路由及账号可用。
使用 Build 延长时省略该字段。

## 查询与下载

使用创建接口返回的 `request_id`：

```bash
export REQUEST_ID='video_替换为返回的任务ID'

curl --fail-with-body --silent --show-error \
  "$BASE_URL/v1/videos/$REQUEST_ID" \
  -H "Authorization: Bearer $API_KEY"
```

| `status` | 含义 |
| --- | --- |
| `queued` | 已入队 |
| `in_progress` | 正在处理 |
| `completed` | 成片已完成 |
| `failed` | 任务失败，读取 `error.code` 和 `error.message` |

建议每 3–5 秒查询同一任务，直至 `completed` 或 `failed`。
完成后响应包含 `video.url`、`content_url`、`seconds` 和 `video.duration`。
`seconds` 是字符串，`video.duration` 是整数秒；编辑/延长完成时读取本地成片元数据，
与请求中的新增 `duration` 含义不同。精确到小数的时长以下载文件的媒体元数据为准。

以下是延长成片查询结果的字段示意，URL、时长与其他元数据以真实响应为准：

```json
{
  "id": "video_...",
  "object": "video",
  "model": "grok-imagine-video-1.5",
  "status": "completed",
  "progress": 100,
  "seconds": "8",
  "video": {
    "url": "https://grok.ai.hxeos.com/v1/media/videos/vid_...",
    "duration": 8,
    "respect_moderation": true
  },
  "content_url": "https://grok.ai.hxeos.com/v1/media/videos/vid_..."
}
```

可直接 GET/HEAD `video.url` 或 `content_url`，无需 API Key 或 Cookie。
也可以按任务 ID 下载：

```bash
curl --fail --location \
  "$BASE_URL/v1/videos/$REQUEST_ID/content" \
  --output result.mp4
```

本地成片支持 Range 分段读取。任务元数据查询仍需鉴权。
完成任务没有可用媒体元数据时可能返回 `502 video_metadata_unavailable`，
这不应被当作重新创建任务的依据。

## 参数边界与错误

客户端不能自行提供 `output.upload_url` 或 `storage_options`；
服务端使用一次性上传票据接收上游成片，并返回公开下载链接。

| 情况 | 可观察结果与处理 |
| --- | --- |
| 请求参数非法 | HTTP 400，例如无效新增时长、编辑传入 duration，修改参数后再提交 |
| 路由不支持该操作 | HTTP 400 `unsupported_model`，检查模型及渠道 |
| Key 无效或权限不足 | HTTP 401/403，检查 Key 及模型权限 |
| 无可用账号/路由 | 通常 HTTP 503，检查渠道启用状态、账号和额度 |
| 上游异步拒绝 | 查询得到 `failed`，检查 `error`；创建时的 HTTP 200 不能证明生成成功 |
| 源视频读取失败 | 检查 URL 的公开访问、有效期和传输稳定性 |

创建请求超时或连接中断时，先依据 `X-Request-ID` 查日志确认是否已经创建任务；
不要盲目重复提交，以免产生多笔收费任务。已有任务只继续查询同一 `request_id`。

视频编辑和延长目前在网关金额账本中记录为未计费，不消耗客户端 Key 的金额额度；
**这不代表上游免费**，Build 账号仍可能产生实际费用。

## 已验证范围

- K3s Build 账号真实编辑：6.04 秒彩色视频修改为 6.04 秒黑白视频，
  1280×720、24 fps、带音轨；上传回调、匿名下载及解码通过。
- 同一 Build 账号真实延长：6.04 秒视频新增 2 秒，成片为 8.04 秒、720p。
- 本地完整 HTTP 集成验证覆盖编辑及延长、auto/build/xai 模式、
  参数校验、账号选择、上传回调、终态持久化与匿名下载。
- 两个生产环境发布后使用无收费请求检查路由；未重复生成收费媒体。
