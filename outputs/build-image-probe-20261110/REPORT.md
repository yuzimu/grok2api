# Build 图片模型真实验证

2026-11-10 23:02:50 至 23:03:03，Asia/Shanghai。

结论：Build主地址支持图片生成模型 `grok-imagine-image-2.0`，即使该模型未出现在账号 `/models` 目录中。

## 请求

- 账号ID：85066，使用其现有Build OAuth access token，无刷新或重新登录。
- 入口：`POST https://cli-chat-proxy.grok.com/v1/images/generations`。
- 请求头参照现有Build视频实现：Bearer OAuth、X-XAI-Token-Auth、客户端版本与标识、headless模式、请求及trace标识、模型覆盖头。
- 从应用工作节点 `xbrd-flow` 执行直连请求。

```json
{
  "model": "grok-imagine-image-2.0",
  "prompt": "A red ceramic teapot on a white table, clean studio lighting, no text.",
  "n": 1,
  "response_format": "b64_json"
}
```

## 结果

- 生成POST仅1次；没有换号、重试、切Console或Web，也没有调用xAI备用地址。
- HTTP 200，响应字段 `data`、`usage`，返回1张图片。
- 耗时13.13秒。
- Base64解码后的JPEG：1248×832、155418字节；Pillow格式校验通过。
- SHA-256：`e277f210b4064d3bf96df00c3215a0e92e751e94bf2da6d818a28e80dd57d866`。
- 已查看实际图片，内容与红色陶瓷茶壶提示一致。

图片：`image-0.jpg`。脱敏请求及结果：`events.jsonl`。没有记录OAuth token、邮箱或完整用户身份。

当前项目Build适配器尚未实现GenerateImage及EditImage，线上也没有Build图片路由。本轮只验证文生图的上述模型及参数；图片编辑、其他图片模型、指定比例/分辨率、批量及流式输出未验证。未修改线上路由、账号或部署。
