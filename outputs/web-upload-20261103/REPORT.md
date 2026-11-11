# Web 参考图上传错误修复

日期：2026-11-03，Asia/Shanghai。目标：K3s `xbrd/grok2api`。

## 线上事实

- 任务：`video_k_pYoyehcoexfjYAh3ip8-KB`。
- 请求：`2fujws9SUdetz7sA`；审计 ID：8973。
- 实际渠道：`grok_web`；账号：29083；出口节点：5。
- 输入：6 秒、9:16、720p、4 张参考图。
- 16:28:45 失败，上游错误为 `500 Internal: Failed to store uploaded file`。
- 审计仅有一次尝试，阶段为 `video_submitted`。参考图上传实际发生在视频创建之前，该阶段标记不正确。
- 任务终态记账后，原始 `input_json` 已按现有清理逻辑置为 `{}`，因此不能重放原始四张图片。

## REA 与协议核对

使用 REA CLI 6.4.0 对当前 Grok Imagine 网页实际发布的上传脚本进行静态分析。脚本来自页面声明的 `cdn.grok.com`，未向 CDN 发送账号 Cookie。

- 原始脚本：`https://cdn.grok.com/_next/static/chunks/125ccbyezb-pz.js`。
- 脚本 SHA-256：`1f9faaebfc704b1a28b0fd8d5b86c0d91d373a7f7ebf5020204850cd46f10e22`。
- REA Evidence：`ev_d0f2ca44ea1018985d18f9f5370da74fb387995951958c358187ff108c60eb9e`。
- 完整证据：`rea-upload-evidence.json`；采集来源：`discovery.jsonl`。

脚本中的直接上传路径仍为 `/http/upload-file-v2/direct`，采用 multipart `file` 和 `file_source`。Imagine 文件来源仍为 `IMAGINE_SELF_UPLOAD_FILE_SOURCE`，与本项目现有实现一致。

REA 解析了实际发布的 JavaScript，未执行网页，也没有观察浏览器上传动作。协议字段核对属于静态证据；以下真实上传是独立的运行证据。

## 真实上传

17:07:13，通过工作节点 `xbrd-flow`，使用原失败账号 29083 及其粘性代理，上传一张已有的有效 JPEG：

- 输入大小：96179 字节。
- 输入 SHA-256：`aa340c4577a7faf9688a03541bacda108f9cec6a9f363da43fc1a6b6c4a0c08f`。
- HTTP 200，用时约 0.49 秒。
- 同时取得 upload ID、文件 ID 和文件 URI，没有 terminal error。
- Cloudflare Ray：`a54a3a382ce1f46c-YYZ`。
- 回执：`live-upload.json`。

这证明该账号、粘性代理和上传协议当前可以完成有效 JPEG 上传。不证明原四张图片均可上传，也不能确定上游当时保存文件失败的内部原因。

## 修复行为

新增明确的 `upload` 视频错误阶段，供 Web 参考图上传失败使用。首帧、尾帧、参考图及关键帧复用的上传路径均保留该阶段，不再被外层包装误判成 `submitted`。

对于该阶段的 500、502、503、504，网关可在既有尝试次数内切换到同一 Web 路由的其他账号。不新增内层重试次数，不跨渠道切到 Build。

保持固定账号禁止换号、全局尝试次数限制、上传 400 不重试，以及真正的视频创建 500 不重放的边界。

修改范围：

- `backend/internal/infra/provider/provider.go`
- `backend/internal/infra/provider/web/video.go`
- `backend/internal/application/gateway/video.go`
- `backend/internal/application/gateway/web_upload_integration_test.go`

## 集成验证

新增隔离集成验证，组合真实 SQLite、账号选择器、Web 适配器及本地 HTTP 上游。仅运行 `TestIntegrationWebUploadHTTPRecovery`，未运行单元测试。

五项通过：

1. 首账号上传 500，第二账号上传成功，再进入视频创建。
2. 尝试次数为 1 时，只上传一次。
3. 固定账号时，不换号。
4. 上传 400 时，不重试。
5. 视频创建返回 500 时，不重复创建。

集成上游在视频创建入口返回受控错误，不生成视频。日志：`integration.log`、`release-integration.log`。

## 发布

已重建并确认发布前基底二进制与线上完全一致：

`dbbf7af8fcb9a89415ddd63d66f4eb32f4d584f8d64fc52784a7b655d8650a6c`

仅将 `release.patch` 中三个生产源码文件的本次改动加入该基底。本地未上线的 Console 身份同步改动没有进入发布。

新镜像：

`registry.cn-hongkong.aliyuncs.com/xbrd/grok2api@sha256:b8af0fd20884ffd9d1761d2ef96c3f3c54989d9929471738a921592b2fd38993`

线上运行二进制 SHA-256：

`5b893279852472985ff55ec85b80d09f3e2523c15dd7f69f62a36bdb9d354027`

更新前排队及进行中的视频任务数为 0。沿用 Deployment 的 Recreate 策略，更新完成后 1 个副本 Ready，observed generation 与 generation 均为 102。部署回执见 `deployment.json`，更新前快照见 `k3s-before.json`。

原四图任务未重放，未修改账号风控或路由优先级，未提交或推送源码。上传 500 恢复逻辑由隔离集成验证覆盖；线上未人为制造上传 500。

正式 API 的六秒参考图视频验证已完成：一张有效 JPEG，最终得到 720×1280、6.041667 秒成片。过程中四个 Web 账号在上传阶段发生 SOCKS 连接失败，自动切换到账号 29087 后成功。该真实验证覆盖上传网络错误恢复，没有触发 HTTP 500；详见 `video-6s/REPORT.md`。
