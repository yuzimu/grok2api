# 非 K3s 更新结果

日期：2026-10-24，Asia/Shanghai。

## 部署

- 主机：Oracle `instance-20261009-2002`，架构 ARM64。
- 服务：`/opt/grok2api-go` 下的 `grok2api-go`。
- 版本：v3.1.8，包含本轮视频终态、账号同步错误分类、后台刷新和匿名媒体下载修复。
- 镜像：`ghcr.io/yuzimu/grok2api@sha256:e028750874a8be4732b37cc2cc2f9be0456a33324069462cf17454c0b00bbd28`。
- 源码标识：`5c5ec66f-console-recovery-r3`，与 K3s 修复源码一致。
- 源码快照 SHA-256：`0d28d53ededb8b1344056b7921f6e38d644fcff49e3ff88b27af9e242f58db2e`。
- 镜像经 SSH 导入，使用现有 Compose 的 `grok2api` 服务、固定摘要和 `--no-deps --pull never` 完成更新。服务器 `.env` 已保存新摘要。

线上旧镜像的源码标识为 `9bee1be9`，源码归档指纹与本地同一提交一致，且该提交已包含在当前分支中。未发现额外挂载的源码或二进制覆盖文件。

## 配置与备份

业务配置、数据库连接、媒体挂载和质量守护保留。

更新前后校验值一致：

- `config.yaml`：`f88bb637dc6fade046084f05a04e8075bcb9ab774f5f4a92af6ca4c4e991d9e1`。
- `compose.yaml`：`a9df800916aa4c03cb4f17f0a7143f006baa291b25b31dee588272db2b0bd5d0`。

质量守护容器 ID 和启动时间均未变化。更新前没有活动视频任务。

备份目录：`oracle:/opt/grok2api-go/backups/nonk3s-20261024.YhPaPw/`。

- `database.dump`：6110763 字节，更新前完整数据库备份。
- `compose.yaml`、`config.yaml`、`.env`、`container.json`：更新前部署快照。
- `release-image.tar.gz`：38960085 字节，本次 ARM64 镜像，可离线导入恢复。

本次未向 GHCR 推送镜像；迁机或清理本地镜像后，应使用保留的镜像归档恢复，不应假定该摘要可从 GHCR 重新拉取。

## 实际验证

- 容器健康检查通过。
- `/readyz` 返回 HTTP 200、`ready: true`。Build、Web、运行态存储和计费账本就绪，Statsig 已预热。
- Console 显示没有可用于启用路由的账号，因此总体状态为 `degraded`，不能宣称 Console 已恢复。
- 通过实际客户端 Key 查询已有完成视频，返回 `completed` 和公开媒体地址。
- 公网图片匿名 GET 返回 200，JPEG 完整下载 100168 字节。
- 公网视频匿名 GET 返回 200，MP4 完整下载 2099034 字节。
- `/v1/videos/video_BHHs2UKYWqz75EWdSBVcSphr/content` 匿名 Range GET 返回 206，`Content-Range: bytes 0-63/2099034`；请求 ID `ZxVsyd4uWiVPgP3O`。

下载文件与数据库 SHA-256 一致：

- JPEG：`d6ced1e3852d05d83bb4ed8f8f3b7c3741a748dff5524b5d459725b9849e1ff7`。
- MP4：`3f10f5eb5cc7e5c7bcb00953587d74f31222f719247dd3268d74c262c510a17d`。

样本和响应头保存在本目录。未运行测试、未提交推送源码，也未发起新的收费生成。
