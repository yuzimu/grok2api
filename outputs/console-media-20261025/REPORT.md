# 非 K3s Console 图片与视频核验

日期：2026-10-25（Asia/Shanghai）

通过 Oracle 主机 `127.0.0.1:8000` 正式公开 API，以已有客户端密钥发起一次图片生成和一次视频生成。请求明确指定 Console 模型；未修改配置、重启服务或提交代码。

## 图片

入口：`POST /v1/images/generations`

```json
{
  "model": "Console/grok-imagine-image",
  "prompt": "A red ceramic teapot on a plain white table, clean studio product photograph, no text.",
  "n": 1,
  "aspect_ratio": "1:1",
  "resolution": "1k",
  "response_format": "url"
}
```

- HTTP 200，返回一张 JPEG。
- request_id：`VtkJqF1rgUZIkH6h`
- 审计：provider=`grok_console`，account_id=14108，egress_node_id=13，media_output_images=1。
- 结果：`https://gk.ss.zoooo.vip/v1/media/images/img_UIAPSsuExQOLSWfl_1SCmcPh4JFc_2kQ`
- 无 Authorization/Cookie 下载返回 HTTP 200，136680 字节。
- 本地文件：`image.jpg`；已目视确认红色茶壶图片，1024×1024。

## 视频

入口：`POST /v1/videos`

```json
{
  "model": "Console/grok-imagine-video-1.5",
  "prompt": "A red ceramic teapot on a white table. The camera slowly moves from left to right. Clean studio lighting, no text.",
  "duration": 3,
  "aspect_ratio": "16:9",
  "resolution": "480p"
}
```

- 创建返回 HTTP 200，初始状态 queued。
- 原任务：`video_LQbRwFinhlOU2SqrphrZ5zNz`
- 正式轮询：`GET /v1/videos/video_LQbRwFinhlOU2SqrphrZ5zNz`
- 最终状态 completed，progress=100，seconds=3。
- created_at=1792899827，completed_at=1792899861，间隔34秒。
- request_id：`5p9GmWoG_ONojCLn`
- 数据库与审计：provider=`grok_console`，account_id=14109，egress_node_id=13，media_output_seconds=3，无错误。
- 结果：`https://gk.ss.zoooo.vip/v1/media/videos/vid_8OLczowzk0xikC3HJDH505-TZbo5SspG`
- 无 Authorization/Cookie 下载返回 HTTP 200，323575 字节，本地文件 `video.mp4`。
- ffprobe：H.264视频848×480、73帧、3.041667秒，AAC音频3秒。
- ffmpeg完整解码并抽取0、1、2秒画面至 `video-contact.jpg`；已目视确认有效茶壶画面和镜头位置变化。

## Console 与 Build 参数

- 公共视频请求结构相同，使用不同的模型前缀：`Console/grok-imagine-video-1.5` 或 `Build/grok-imagine-video-1.5`。
- 基础字段：model、prompt、duration、aspect_ratio、resolution。
- 公共首尾帧字段：`image: {"url": "https://..."}`、`last_frame: {"url": "https://..."}`。网关接收 url；内部 Console 上游使用 url，Build 上游使用 image_url，由适配器转换。
- 同一1.5模型的参考图片上限14；参考模式最高720p；Build/Console参考音频使用voice_id。
- Console旧视频模型仅支持最多7张参考图，参考图生视频最长10秒；不能将旧模型与1.5模型的限制混用。
- 当前线上 Build 只有视频路由，没有图片生成路由；Console图片入口为 `/v1/images/generations`，n为1–10，resolution为1k或2k，不支持stream或partial_images。
- 参数差异依据当前源码核对；本次仅真实验证上述文生图和文生视频，未额外生成首尾帧、参考图、语音或Build对照视频。
