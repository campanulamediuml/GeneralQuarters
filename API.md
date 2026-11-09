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

`coord`必须位于该单位服务器返回的`legal_moves`中。移动由服务器立即生效并递增`version`，不生成待结算移动命令；同一舰队每回合只能移动一次，只要任一成员已移动，整个编队就不能再次移动。`CLEAR_MOVE`仅用于取消旧版协议遗留的待结算移动命令，包含`command_id`、`base_version`、`type`和`unit_id`。

### 整编舰队

```json
{
  "command_id":"01J...",
  "base_version":1,
  "type":"MERGE_FLEET",
  "unit_id":"a-screen",
  "target_unit_id":"a-flag"
}
```

`unit_id`为整编源，`target_unit_id`为目标舰队中的单位。双方必须属于当前玩家、位于同一hex且`fleet_id`不同。整编由服务器立即生效并递增`version`。

### 整个hex整编

```json
{
  "command_id":"01J...",
  "base_version":1,
  "type":"REORGANIZE_HEX",
  "unit_id":"hex-0302",
  "coord":"0302",
  "assignments":{"a-flag":"a-flag","a-screen":"a-flag"}
}
```

`assignments`必须包含该hex全部己方单位，值为目标`fleet_id`。同一操作可以合并、拆分或转移成员；不允许遗漏单位、包含其他阵营单位或跨hex整编。整编由服务器立即生效并递增`version`。快照中每个单位还包含`heading`和`moved`。

### 航空命令

```json
{"command_id":"01J...","base_version":1,"type":"LAUNCH_CAP","aircraft_ids":["air-b-01","air-b-02"]}
```

- 所有航空命令都可使用`aircraft_ids`批量提交同一母舰的1至20个算子，旧`aircraft_id`单算子格式继续兼容。批量任务要求同型、同整备阶段算子并共享`task_id`，服务端一次校验和结算。
- `PREPARE_AIR`：手动整备必须跨两个准备回合。第一回合把`hangar`改为`service`，完成机库加油整备；第二回合把舰载机`service`改为`deck`，或把水机改为`ready`。舰载机甲板停放容量在第二阶段校验，母舰进行整备的回合不能起飞。
- `RETURN_TO_HANGAR`：将`service/deck/ready`算子批量拉回机库并重置为`hangar`，同时取消`task_id/task_kind`、优先级和侦察配置；之后必须重新完成两个准备回合。
- `LAUNCH_CAP`：将战斗机分配至CAP任务。`hangar/service`算子立即形成预备任务并进入`service`，不会直接升空；已经位于`deck`的算子立即升空并消耗母舰当回合起飞容量。非战斗机、容量不足或母舰本回合已进行其他不相容作业时返回400。
- `LAUNCH_RECON`：将侦察机或攻击机分配至扇面搜索任务，额外提交`heading`和`sector_width`。允许`recon`、`torpedo_bomber`（TBF）和`dive_bomber`（SB2C），拒绝战斗机。例如`{"command_id":"01J...","base_version":1,"type":"LAUNCH_RECON","aircraft_id":"air-b-10","heading":"W","sector_width":60}`。机库算子先形成预备任务，`deck`舰载机或`ready`水机立即起飞。航向为`E/SE/SW/W/NW/NE`，角度只能为60或120。`recon_radius`为可选高级参数，省略或为0时由服务端使用`recon_radius_limit`作为默认最大安全半径，手动值不能超过该上限；可搜索的各机型都会获得服务端计算的`recon_radius_limit`。
- 预备任务在后续回合结算中自动从`service`进入`deck/ready`，再于下一回合自动起飞。任务创建后立即出现在`aviation.tasks`，`phase`依次为`servicing`（整备中）、`preparing`（起飞准备中）、`launching`（起飞中）和`active`（任务执行中）；飞机状态仍在对应算子的`status`中。侦察参数在分配时持久化，扇面顶点使用实际起飞时的母舰位置。甲板或起飞容量不足时自动作业推迟，不跳过阶段。
- `RECOVER_AIR`：将CAP算子批量回收至机库并重置为未整备的`hangar`；母舰本回合已起飞或整备时返回400。侦察机沿航线自动返航，不能在空中直接回收。
- `PRIORITIZE_AIR_TASK`：将指定预备或空中CAP/侦察任务置为最高优先级，其他任务顺序由服务端调整。航空界面任务边缘的灰色旗子调用此命令，最高优先级旗子高亮。
- 这些命令都立即生效并递增`version`，所有校验、整备阶段、机库与甲板数量、扇面hex、发现判定、情报、燃油与任务排序仅由Go服务端计算。回合结算清除航空作业标记但保留未分配算子的`service/deck/ready`准备进度；新自动起飞的算子从下一回合开始耗油和移动，空中CAP剩余航程-1，非空中算子补满航程。普通侦察机每回合移动2 hex，水机1 hex，飞出和返航逐距离圈搜索。

### Ready

```json
{"base_version":1,"ready":true}
```

响应为最新私有快照。所有当前玩家ready后，服务器同步完成结算并提升`turn`与`version`。单人沙盒中唯一玩家ready即可结算。已ready玩家提交新命令时自动取消自己的ready。

## 私有快照

`state`响应包含：

- `scenario`、`map`、`turn`、`version`、`status`；
- `you`和`players`；
- 当前阵营可见的`units`；己方单位额外包含`legal_moves`。每艘舰船包含`definition_id`、当前/初始生命值和服务端生成的`card`角色卡，字段涵盖火炮、防空、损管、雷达、航速、鱼雷、航空设施、货物等；
- 服务端生成的`fleets`舰队投影，包含`fleet_id`、成员列表、数量、总强度、朝向、移动状态、合法移动和`distance_reference`；距离参考由`origin/max_distance/cells`组成，每个cell包含服务端计算的`coord/distance`；
- `aviation.carriers`：全部航空作业舰，包含机库占用/容量、甲板占用/容量、作业类型和航空算子；`aviation.tasks`：按服务端优先级排序的预备和空中任务，包含`phase`及完整算子列表。空中任务包含以当前位置为中心的`distance_reference`，已起飞侦察任务额外包含`origin_coord`、`current_coord`、`heading`、`sector_width`、`recon_radius`、`recon_distance`、`returning`、`tracking_fleet_id`、`sector_hexes`和`current_ring_hexes`，客户端只渲染这些服务器结果；
- 仅自己的`orders`；
- 按受众过滤的`events`；
- 仅房主可见的`join_token`。

客户端不得假设未返回的敌舰不存在，也不得从WebSocket或其他接口推导其真实位置。

## WebSocket

路径：`GET /ws`，使用同一Cookie和同源检查。

```json
{"type":"turn_resolved","version":2,"time":"2026-..."}
```

事件类型：`player_joined`、`orders_updated`、`state_changed`、`ready_changed`、`turn_resolved`、`presence_changed`。消息不包含私有状态，收到后应重新请求`state`。连接断开时使用15秒轮询兜底，恢复后重新获取完整快照。

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
