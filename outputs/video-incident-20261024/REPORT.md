# K3s 视频任务故障修复

日期：2026-10-24，Asia/Shanghai。

## 源码与发布

- 工作目录：`/Users/real/Documents/code/grok2api`。
- 上游：`chenyme/grok2api`，`upstream/main` = `e2fd7ab4`，版本 v3.1.8。
- 本地合并提交：`5c5ec66f`，保留 fork 的视频路由、参考素材、风控与发布配置。
- 修复代码保留在工作区，未提交或推送 Git。
- K3s 目标：`xbrd/grok2api`，仅更新主容器。
- 发布镜像：`registry.cn-hongkong.aliyuncs.com/xbrd/grok2api@sha256:3ea225318baf057ccee84a1d8143029e7f9c4cb727c41a8ff116b0b59bb93ad9`。
- 完整源码快照：`source.tar.gz`；SHA-256：`d3509f4d1e1b9e1d97de9086b3d4abc3090e42fd2e006c46da1bf0fed4dee3ef`。
- 相对合并提交的已跟踪文件补丁：`fix.patch`；新增终态收据源码包含在完整源码快照中。

## 修复内容

1. `backend/internal/infra/persistence/relational/model_repository.go`：视频执行读取单个路由和绑定，不再聚合整个账号池；管理端创建、修改响应继续提供统计。
2. `backend/internal/application/gateway/video.go`：区分数据库读取失败和真实模型缺失；终态补偿恢复失败预占释放、审计和输入回收。
3. `backend/internal/infra/persistence/relational/media_terminal_journal.go`、`media_repository.go`、`backend/internal/app/application.go`：终态先以原子文件和 fsync 写入媒体持久卷 `.video-terminals`；恢复扫描先补写终态。认领令牌与活动状态限制阻止旧执行者覆盖新执行者或终态。
4. `backend/internal/infra/persistence/relational/account_links.go`：已完整关联的账号跳过全局关联写锁；真实关系变更仍保留事务和写锁。
5. `backend/internal/infra/provider/console/quota.go`：Console 周额度查询使用 Console 出口及反馈作用域，保留账号原有出口绑定。
6. `backend/internal/app/startup.go`、`model_repository.go`：`/readyz` 读取启用路由后通过真实候选判定可用性，不执行管理端全量账号统计。
7. `backend/internal/transport/http/server.go`、`inference/handler.go`、`gateway/video.go`：图片和视频资产链接保持公开；`/v1/videos/{id}/content` 支持匿名 GET/HEAD，仅提供已完成视频字节。本地成品支持 Range。生成、任务元数据和管理接口继续鉴权。README 与生成的 Swagger 文档已同步。

## 任务处理

13 个任务在数据库重启期间发生终态写入失败，进度残留在 0% 或 1%。没有保存结果 URL、结果资产、上传票据或计费预占，缺少可继续轮询的上游任务 ID。

原始数据备份后，依据精确任务 ID、旧更新时间、活动状态及空结果条件进行条件更新，收尾为 `failed / persistence_interrupted`。没有重新生成视频，也没有将这些任务标记为成功。生成结果仍未确认。

`video_GGSY_hSL5p_2BKDxHUm6-8ux` 的路由实际存在；任务及其原有审计中的误报已更正为 `database_unavailable`。

线上数据库复查：最近 24 小时 11 个完成、14 个失败、0 个进行中，失败任务待补审计为 0。

远程备份位置：`xbrd-newapi:/opt/grok2api-ci/backups/video-incident-20261024/`。其中 `task-recovery.log` 含原始任务及操作结果，按私有权限保存。

## 验证证据

- 前后端及 Linux amd64 发布镜像编译通过；未运行测试。
- 最终 Pod `grok2api-c4758bbd4-8jqvc` 为 `2/2 Running`，重启 0，镜像摘要与发布目标一致。
- 通过任务所属客户端 Key 调用正式 `GET /v1/videos/{id}`，14 个受影响任务全部返回 HTTP 200，响应体为 `status: failed`，且有相应数据库错误原因。
- 示例：`video_f4OCv52i6OomKQjVGRNFM64x`，查询请求 ID `rA_rP7oR4mfzcVQy`。
- 误报修正示例：`video_GGSY_hSL5p_2BKDxHUm6-8ux`，查询请求 ID `g-dBvAlr3Za2zdkX`。
- 两个刷新令牌保存失败的账号 51328、51797 已由服务端自动续期恢复，状态 active，刷新失败计数为 0；未人工替换令牌。
- 共享 PostgreSQL 实际 Pod 的存活探针已为 5 秒超时、连续 6 次失败阈值，命令为 `pg_isready -U postgres -d postgres -t 3`。此配置由本轮之外的操作生效，本轮未修改或重启共享数据库。

## 匿名下载

公网域名：`https://grok.ai.hxeos.com`。以下请求均未携带 Authorization 或 Cookie，没有发起生成：

| 入口 | 实际结果 |
| --- | --- |
| `GET /v1/media/images/img_hoNdC9UKfVhee14u9y8dISE0WxrxDJiD` | 200，完整 JPEG，262108 字节 |
| `GET /v1/media/videos/vid_XXCZjm5QRyIlKiZdIB4k_0snkYATRUea`，Range 0–63 | 206，64 字节 |
| `GET /v1/videos/video_H1wvAddqJBhMakXmLfEIwIcy/content` | 200，完整 MP4，7402660 字节 |
| 同一任务内容入口 HEAD | 200，Content-Length 7402660 |
| 同一任务内容入口 GET，Range 0–63 | 206，Content-Range `bytes 0-63/7402660` |
| `GET /v1/videos/video_H1wvAddqJBhMakXmLfEIwIcy` | 401，任务元数据仍要求鉴权 |

下载后的 SHA-256 与数据库资产记录一致：

- JPEG：`62ee7f2b61fb386436601a0e300355046da5120deb57ff31c185a55b793642ce`。
- MP4：`7dca3ea406f79c0d25d9641a9e31b2293ddab3ef3a48016650cd573d3d13c237`。

HEAD 请求 ID：`5VRMWInPVwNROaP6`。Range 请求 ID：`YGMJo-bKYWyzeWL2`。文件和响应头保存在本目录 `downloads/`。

## 验证边界

未新发收费视频请求，未通过人为中断生产数据库进行故障注入，未验证新的上游生成质量。终态收据补偿机制已编译部署，但生产数据库再次故障时的完整恢复过程未重新触发。

Console 额度查询的本地 scope 错误已消失。2026-10-24 16:14:16 的实际请求在节点 6 遇到 `console_quota_egress_blocked`：WorkOS 会话换取被 Cloudflare 拒绝，提示出口 IP 被封。账号 66401、66414 的额度同步时间仍未更新，因此 Console 额度同步尚未恢复，不能宣称所有上游能力均可用。
