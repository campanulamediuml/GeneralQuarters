# General Quarters 接口与扩展协议

运行时OpenAPI文档：`GET /api/v1/openapi.yaml`。在线简版文档：`/api/docs`。架构与数据设计见`DESIGN.md`。

## 通用约定

- 除健康检查和文档外，接口使用创建或加入房间时签发的HttpOnly Cookie`gq_session`。
- 成功响应为JSON，错误为`{"error":"人类可读说明"}`。
- `room_id`标识房间；`join_token`是邀请凭证；二者都不是玩家会话。
- `base_version`来自最近一次私有快照。状态过期返回409，客户端应重新同步。
- `command_id`由客户端生成，在玩家范围内唯一。断线重试必须沿用原ID。
- hex坐标为`ABCD`，AB为行、CD为列，例如`0203`。

## REST接口

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/healthz` | 健康检查 |
| POST | `/api/v1/rooms` | 创建房间并自动载入剧本，默认`_sandbox` |
| POST | `/api/v1/rooms/join` | 使用12位token加入 |
| GET | `/api/v1/rooms/{roomID}/state` | 获取当前阵营私有完整快照 |
| GET | `/api/v1/rooms/{roomID}/events` | 获取最近100条当前阵营可见事件 |
| POST | `/api/v1/rooms/{roomID}/commands` | 设置或取消单位命令 |
| POST | `/api/v1/rooms/{roomID}/ready` | 设置或取消ready；全员ready触发结算 |
| GET | `/api/v1/openapi.yaml` | 机器可读OpenAPI定义 |

### 创建房间

```json
{"name":"玩家A","scenario_id":"_sandbox"}
```

响应201：

```json
{"room_id":"...","player_id":"...","join_token":"7K4M-9Q2X-6T8P"}
```

### 加入房间

```json
{"token":"7K4M-9Q2X-6T8P","name":"玩家B"}
```

成功响应200。满员、token无效或格式错误返回400。

### 设置移动

```json
{
  "command_id":"01J...",
  "base_version":1,
  "type":"SET_MOVE",
  "unit_id":"a-flag",
  "coord":"0203"
}
```

`coord`必须位于该单位服务器返回的`legal_moves`中。相同单位的新命令替换旧命令。`CLEAR_MOVE`只需`command_id`、`base_version`、`type`和`unit_id`。

### Ready

```json
{"base_version":1,"ready":true}
```

响应为最新私有快照。所有当前玩家ready后，服务器同步完成结算并提升`turn`与`version`。单人沙盒中唯一玩家ready即可结算。已ready玩家提交新命令时自动取消自己的ready。

## 私有快照

`state`响应包含：

- `scenario`、`map`、`turn`、`version`、`status`；
- `you`和`players`；
- 当前阵营可见的`units`；己方单位额外包含`legal_moves`；
- 仅自己的`orders`；
- 按受众过滤的`events`；
- 仅房主可见的`join_token`。

客户端不得假设未返回的敌舰不存在，也不得从WebSocket或其他接口推导其真实位置。

## WebSocket

路径：`GET /ws`，使用同一Cookie和同源检查。

```json
{"type":"turn_resolved","version":2,"time":"2026-..."}
```

事件类型：`player_joined`、`orders_updated`、`ready_changed`、`turn_resolved`、`presence_changed`。消息不包含私有状态，收到后应重新请求`state`。连接断开时使用15秒轮询兜底，恢复后重新获取完整快照。

## DeepSeek与文字对手扩展

模型在服务器侧作为`TextOpponent`运行，不直接访问数据库或规则内部状态。输入仅限该AI阵营私有快照和对话消息，输出为结构化候选命令与文字说明。

```text
私有快照/玩家文字 → DeepSeek适配器 → 候选JSON命令
→ 命令schema校验 → 与普通玩家相同的合法性校验 → ready与权威结算
```

建议后续增加独立的AI控制接口，而不复用真人Cookie：

- `POST /api/v1/rooms/{id}/ai/slots`：房主配置AI阵营和模型参数引用；
- `POST /api/v1/rooms/{id}/ai/messages`：向AI发送文字；
- `GET /api/v1/rooms/{id}/ai/plans/{planID}`：查看解析后的候选命令和解释；
- `POST /api/v1/rooms/{id}/ai/plans/{planID}/approve`：确认执行候选命令。

这些接口尚未实现，当前仅定义扩展边界。模型密钥仅通过服务器环境变量注入，不写入SQLite、浏览器、Git或战报。解析失败不得猜测执行，应返回澄清需求。
