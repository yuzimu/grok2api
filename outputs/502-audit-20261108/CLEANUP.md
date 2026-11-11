# 熔断节点清理回执

按用户要求，对两端 Resin 的熔断节点调用正式管理接口：

`POST /api/v1/subscriptions/{id}/actions/cleanup-circuit-open-nodes`

| 环境 | 清理前 | 接口确认清理 | 清理后节点总数 |
| --- | ---: | ---: | ---: |
| K3s | 3185 | 3185 | 0 |
| Oracle | 3894 | 3894 | 0 |

合计清理7079个。操作前重新核对，两端健康节点数均为0。操作后分别通过节点列表和订阅详情回读，节点总数、订阅节点数均为0。

使用 Resin 自带的订阅节点剔除机制，未直接修改 SQLite。服务会保留其管理所需的剔除记录；这里的清理指移出节点池，不是擦除历史日志或备份。

保留两个 `proxysource` 订阅、平台配置、应用账号、Cookie 和 Browser Profile。未重启服务，未触发登录或重新导入节点。

服务器本地受限备份：

- K3s，`xbrd-flow:/var/backups/resin-circuit-cleanup-20261108T100955Z`
- Oracle，`oracle:/var/backups/resin-circuit-cleanup-20261108T101159Z`

备份留在各服务器，不包含在本报告中。当前两边节点池为空；删除失效节点并不能修复上游407，需要有效节点才能恢复代理服务。
