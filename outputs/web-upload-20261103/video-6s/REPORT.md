# Web 六秒参考图视频验证

时间：2026-11-03 17:34:29 至 17:35:35，Asia/Shanghai。

正式入口：`POST /v1/videos`，模型 `Web/grok-imagine-video-1.5`，6 秒、720p、9:16、一张 JPEG 参考图。只提交一次外部生成请求。

- 任务：`video__6Pw4hS9jAdZc3frQ084oIgO`。
- 请求：`lDJn9ZiVnyoc8ove`。
- 最终状态：completed，100%。
- 实际渠道：grok_web；成功账号：29087；出口节点：5。
- 总耗时约 66 秒。

## 上传恢复

审计记录包含四次失败，全部处于 `video_upload` 阶段：

| 顺序 | 账号 | 结果 |
| --- | --- | --- |
| 1 | 29074 | SOCKS 代理连接失败 |
| 2 | 29076 | SOCKS 代理连接失败 |
| 3 | 29083 | SOCKS 代理连接失败 |
| 4 | 29085 | SOCKS 代理连接失败 |
| 最终 | 29087 | 上传、生成及成片交付成功 |

四次失败均未收到上游 HTTP 响应，错误为 `general SOCKS server failure`。本次实际验证了参考图上传网络错误后的自动换号恢复，没有切换到 Build；未触发上传 HTTP 500，不能将本轮结果称为线上 500 恢复的直接验证。

## 成片

- 无鉴权内容下载返回 HTTP 200，Content-Type 为 video/mp4。
- 文件大小：3887212 字节。
- H.264 视频：720×1280、24 fps、6.041667 秒。
- AAC 音频：6.000000 秒。
- FFmpeg 全文件解码完成，无错误。
- 抽帧核对：山景、湖面和木屋与参考图相符。

文件：`video.mp4`、`ffprobe.json`、`frame.jpg`。请求及终态回执：`dispatch.json`、`events.jsonl`。

原失败任务的四张图片已清理，本轮使用的是已验证可上传的一张 JPEG，未重放原四图任务。
