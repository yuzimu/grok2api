# K3s Console 400账号核查

日期：2026-11-01，Asia/Shanghai。

范围：当前数据库保留的全部Console请求审计、上游尝试记录及视频任务；不是历史已清理记录的全量保证。Console审计共8条，时间范围2026-11-01 09:18:33至09:42:51（北京时间）。

## 明确匹配的账号

| 账号ID | 名称后缀 | request_id | 阶段 | 上游状态 |
| --- | --- | --- | --- | --- |
| 86101 | 5225d420 | XcboIpyLHblETgYE | video_create | 400 Incorrect API key provided |
| 86102 | e91a23f9 | guNmCXswWmKIVi0m | video_create | 400 Incorrect API key provided |
| 86109 | e25817c4 | fCR2Ud_CqOafIuNT | video_create | 400 Incorrect API key provided |

每个账号各有一条匹配的上游尝试，视频任务与请求审计相互对应。未发现额外匹配账号。

三个账号均enabled=true、auth_status=active，缓存API Key ID非空且指纹互不相同。额度窗口均remaining=100、total=100，快照时间约09:16，不能当作当前上游Key有效的证明。三个账号均没有额度恢复exhausted记录。

## 最小验证状态

拟通过正式管理入口 `POST /api/admin/v1/accounts/{id}/model-tests` 固定账号发起小型文本探测，避免视频费用和网关自动换号掩盖账号故障。

本轮该探测尚未执行：

- 以部署配置中的初始化凭据调用正式登录接口一次，返回invalidCredentials；未继续猜测密码。
- 尝试复用已登录Chrome管理会话，浏览器控制超时，无法通过该会话完成正式请求。
- 未伪造管理员会话、创建管理员、改密码、解除暂停或改写Key。
- 未用普通客户端接口的换号成功结果作为固定账号验证证据。

因此当前结论限于已定位3个历史400账号，不能宣称它们现在恢复或仍会复现400。需要可用的正式管理员认证入口完成每账号一次小型文本探测。
