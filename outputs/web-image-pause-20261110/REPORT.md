# K3s Web 图片路由暂停

2026-11-10，将以下六条图片生成和编辑路由设为 enabled=false：

- 30：Web/grok-imagine-image-2.0，image。
- 31：Web/grok-imagine-image-edit，image_edit。
- 47：Web/grok-imagine-image-lite，image。
- 48：Web/grok-imagine-image，image。
- 49：Web/grok-imagine-image-2.0-web，image。
- 50：Web/grok-imagine-image-2.0-web，image_edit。

全部其他路由逐行回读确认未变，包括已暂停的Web视频、Build和Console。Oracle未修改。无服务重启、账号删除或收费媒体生成。

当前仓库Build适配器只实现Responses和视频，没有GenerateImage或EditImage；线上没有provider=grok_build且capability为image或image_edit的路由。关闭Web已完成，切换图片到Build尚未完成。Console原路由保留，不能将其计作Build。

修改前后快照见routes-before.json、routes-after.json；正式模型目录回读见catalog.json。
