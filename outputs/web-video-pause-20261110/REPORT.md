# K3s Web 视频临时暂停

2026-11-10 检查最近48小时：Web视频2次失败，Build视频8次成功。Web失败请求 `ahazE5uz6k6ZI7u5`、`WlHqGJDjgcondstD` 各有30次尝试，全部返回429。

额度快照仍有15298条 `video_720p` 记录 remaining>0，因此不能断言额度全部耗尽或上游撤销了视频功能。实际生成连续被限流，按用户要求临时暂停当前失败渠道。

K3s数据库 `grok2api.model_routes`：

- 32：`Web/grok-imagine-video`，enabled=false。
- 44：`Web/grok-imagine-video-1.5`，enabled=false。

其余路由逐行核对未变。Oracle近48小时无视频调用，未修改。没有重启、删除账号或生成收费媒体，操作时没有排队或进行中的视频任务。

备份及回读：`routes-before.json`、`routes-after.json`。恢复时仅重新启用32和44两条视频路由，并先检查当前配置及账号实际可用性。
