# Console 图片账号检查

时间：2026-11-01 12:40:26 至 12:44:45，Asia/Shanghai。

578 个 Console 账号的本轮端到端成功率为 **92/578 = 15.92%**。

| 结果 | 账号数 |
| --- | ---: |
| 生成、下载及图片格式校验成功 | 92 |
| 生成接口 HTTP 402 | 474 |
| 认证前 SOCKS5 连接失败 | 9 |
| 未取得可用 API key ID | 3 |
| 生成成功后下载失败 | 0 |

实际提交生成请求 566 次，涉及 566 个不同账号；其中成功率为 16.25%。另外 12 个账号未提交生成请求，图片能力未验证。没有重复提交、换号兜底或生成重试，没有修改账号风控、启用状态或删除账号。

## 入口与参数

从 K3s 应用工作节点 `xbrd-flow` 调用 Console 正式接口 `POST https://console.x.ai/v1/images/generations`。逐号固定 SSO 和 API key ID，通过生产绑定节点 5 的账号粘性代理配置完成认证、生成和图片下载。

代理目标为内网 Resin Service `10.43.98.242:2260`，使用 SOCKS5h。客户端显式指定代理并禁用 NO_PROXY 绕过。账号身份按生产逻辑从关联 Web 身份、已有账号身份或 SSO 摘要导出，共 578 个不同粘性身份；本轮没有逐号查询公网出口 IP。

```json
{
  "model": "grok-imagine-image-2.0",
  "prompt": "A red ceramic teapot on a white table, clean studio lighting, no text.",
  "n": 1,
  "aspect_ratio": "1:1",
  "resolution": "1k",
  "quality": "low",
  "response_format": "url"
}
```

92 个成功结果均返回图片 URL，经同账号代理下载取得 HTTP 200；Pillow 识别并校验为 1024×1024 JPEG。逐号保存了图片字节数、SHA256、状态码及时间，没有保存 SSO、API key、代理密码或图片 URL。

474 个 402 均返回额度耗尽或需要 Grok 订阅的提示。本轮未出现生成接口 400、403、429；402 的具体账号限制原因不能仅由这条响应确定。

## 未验证账号

- SOCKS5 连接失败：86109、86526、86593、86604、86633、86728、86785、86791、86807。错误为 `curl (97) cannot complete SOCKS5 connection to console.x.ai`。
- API key ID 不可用：87039、87041、87042。数据库无缓存 key ID，经代理访问 Console 首页返回 HTTP 200，但未解析到可用 key ID；没有创建新 key。

## 证据

- `cohort.json`：578 个账号 ID 及粘性身份摘要。
- `results.jsonl`：逐号开始、生成提交和结果记录。
- `accounts.csv`：逐号结果明细。
- `summary.json`：聚合结果、成功率和未验证账号。
- 工作节点原始回执：`xbrd-flow:/tmp/grok-console-image2-20261101.jsonl`。

本次是固定账号的 Console 上游检查，未经过网关自动选号，结果不证明网关兜底或自动额度刷新行为。未修改、提交或部署产品代码。
