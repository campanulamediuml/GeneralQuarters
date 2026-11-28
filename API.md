# General Quarters API

本文描述当前服务端实现的 HTTP、WebSocket、命令和快照契约。游戏规则见 `规则书.md`，剧本说明见 `剧本/README.md` 及同目录的独立剧本文档；本文不重复战斗公式。

- 在线速查：`GET /api/docs`
- 大厅 HTTP 的机器可读定义：`GET /api/v1/openapi.yaml`；WebSocket 契约以本文为准
- 架构与存储设计：`DESIGN.md`

## 1. 通用约定

### 1.1 会话与观战凭证

- 创建房间只初始化空房间，不建立玩家会话。以 `join_token` 和指定阵营加入成功后，服务端通过 HttpOnly Cookie `gq_session` 建立玩家会话。Cookie 路径为 `/`，有效期 30 天，SameSite 为 Lax；生产配置可启用 Secure。
- `room_id` 是房间标识，`player_id` 是玩家标识，`join_token` 是邀请及观战凭证。三者不能互相替代。
- A、B 阵营使用相同的加入接口，各最多一名玩家；创建者不会自动占据席位或成为房主。玩家接口要求会话属于路径中的房间。观战接口不使用玩家 Cookie，而用 `join_token` 认证。
- 邀请 token 共 12 位，请求和响应中可显示为 `XXXX-XXXX-XXXX`；服务端比较时会忽略连字符并统一大小写。

### 1.2 请求与响应

- 请求和成功响应使用 JSON。请求体及 WebSocket 单帧上限均为 1 MiB。
- JSON 解码会拒绝未知字段。不要提交 `recon_radius` 等未在命令结构中定义的字段。
- 错误响应为：

```json
{
  "error": "人类可读说明",
  "error_code": "loadout_not_supplied",
  "error_params": {"where": "护航航母「马尼拉湾」", "weapon": "鱼雷"}
}
```

`error` 始终存在；`error_code` 和 `error_params` 仅在带码错误中出现。文案表位于 `content/misc/display.json` 的 `errors` 分组。

### 1.3 坐标、回合与命令 ID

- hex 坐标为 `ABCD`：前两位是行，后两位是列，例如 `0203`。地图行、列从 1 开始。
- `base_turn` 可选；大于 0 时必须等于当前回合，否则返回 409。它不是状态版本号。
- `command_id` 必填，最长 128 字节，在同一玩家的历史命令范围内唯一。
- 网络重试必须复用原来的 `command_id` 和完整请求体。相同 ID、相同命令返回首次回执；相同 ID、不同命令返回 400。
- 战斗阶段的规划命令按“房间、回合、玩家、单位或任务”保存，后写覆盖先写。立即生效命令不属于这套覆盖机制。

### 1.4 房间生命周期

双人房必须有两名玩家才能 ready 和推进回合；单人沙盒由唯一玩家推进。闲置房间连续 12 小时无活动且没有玩家在线时，服务端只释放内存 worker，不删除数据库中的房间、事件、快照或命令回执；后续请求会自动重载。重载竞争期间可能返回 503，重试即可。

## 2. HTTP 接口

HTTP 只用于大厅创建、加入房间，以及解析观战 token。进入房间后，玩家状态、事件、命令、ready 和观战全部使用 WebSocket；服务端不提供房间对局的 HTTP 兼容接口。

| 方法 | 路径 | 认证 | 说明 |
|---|---|---|---|
| GET | `/healthz` | 无 | 返回 `{"status":"ok"}` |
| GET | `/api/docs` | 无 | 在线速查页 |
| GET | `/api/v1/openapi.yaml` | 无 | OpenAPI 3.0 定义 |
| POST | `/api/v1/rooms` | 无 | 创建不含玩家的空房间并返回邀请 token |
| POST | `/api/v1/rooms/join` | 无 | 使用邀请 token 和指定阵营加入并建立玩家会话 |
| GET | `/api/v1/watch/{token}` | token | 解析为 `{"room_id":"..."}`；无效 token 返回 404 |
| GET | `/content/{path}` | 无 | 剧本引用的静态图片等资源 |

`/watch/{token}` 与 `/watch/{roomID}/{token}` 是前端 SPA 路由，不是 JSON API。

### 2.1 创建房间

```http
POST /api/v1/rooms
Content-Type: application/json
```

```json
{
  "scenario_id": "_coral_real",
  "solo": false
}
```

| 字段 | 必填 | 说明 |
|---|---|---|
| `scenario_id` | 否 | `_coral_real` 或 `_marianas`；默认 `_coral_real` |
| `solo` | 否 | `true` 创建只允许一个玩家席位的单人沙盒；默认双人房 |

成功返回 201，不设置会话 Cookie，也不返回 `player_id`：

```json
{
  "room_id": "04ca6f34b85a65d2",
  "join_token": "7K4M-9Q2X-6T8P"
}
```

### 2.2 加入房间

```json
{
  "token": "7K4M-9Q2X-6T8P",
  "faction": "B",
  "name": "玩家B"
}
```

`token` 和 `faction` 必填，`faction` 只能是 `A` 或 `B`；`name` 可选。A、B 使用同一流程，允许 B 先加入。成功返回 200，并设置独立会话 Cookie：

```json
{"room_id":"04ca6f34b85a65d2","player_id":"...","faction":"B"}
```

加入响应不返回 `join_token`。token 无效、格式错误或阵营无效时返回 400；阵营已被占用或单人房已有唯一玩家时返回 409。已有玩家断线后使用原会话 Cookie 重连，不要通过重复 join 恢复身份。

### 2.3 快照与事件结构

以下结构通过 WebSocket RPC 获取，不是 HTTP 接口。`GET state` 返回第 5 节所述私有快照，`GET events` 返回：

```json
{
  "events": [
    {
      "id": 41,
      "turn": 3,
      "sequence": 2,
      "type": "unit_moved",
      "text": "...",
      "created_at": "2026-01-01T04:00:00Z"
    }
  ]
}
```

数组只包含当前阵营可见的最近 100 条事件，并按事件 ID 升序返回。

## 3. 命令 RPC

命令只能通过 WebSocket RPC 提交，请求体结构为：

```json
{
  "command_id": "01J...",
  "base_turn": 1,
  "type": "SET_MOVE",
  "unit_id": "a-flag",
  "coord": "0203"
}
```

成功只表示命令通过校验并被接受，响应不携带完整状态：

```json
{"accepted":true,"command_id":"01J...","turn":1}
```

`turn` 并非所有命令回执都包含。最新状态通过快照或补丁获取。无线电命令的“已接受”也不等于通信判定成功；结果在回合结算后通过事件和快照体现。

### 3.1 通用字段

按需提交字段，不要给无关命令补空字段以外的自定义参数。

| 字段 | 类型 | 用途 |
|---|---|---|
| `unit_id` | string | 舰船 ID；不是 `fleet_id` |
| `target_unit_id` | string | 目标舰队中的舰船 ID |
| `coord` | string | 移动、部署、目标或目的地 hex |
| `heading` | string | `E/SE/SW/W/NW/NE` |
| `engagement` | string | `aggressive/advantage/avoid` |
| `assignments` | object | `舰船ID -> fleet_id` 的整编映射 |
| `split_ids` | string[] | 要拆出的舰船 ID |
| `aircraft_id` | string | 单个航空算子 ID；编队级命令中也可传任务 ID |
| `aircraft_ids` | string[] | 一批航空算子 ID，1–20 个 |
| `task_id` | string | 要加入的航空任务，或无线电命令的任务 ID |
| `task_kind` | string | 无线电改令目标：`attack` 或 `transfer` |
| `target_fleet_id` | string | 明确指定要攻击的敌方舰队 |
| `sector_width` | integer | 侦察扇面：60 或 120 |
| `lane` | integer | 新建打击/侦察编队的面板栏位偏好，从 1 开始 |
| `loadout` | string | 挂载 ID；空字符串表示卸弹 |
| `disperse` | boolean | `true` 分散攻击，`false` 集中攻击 |
| `cycle` | boolean | 设置自动任务循环开关 |

### 3.2 生效时机

| 命令 | 提交时 | 回合结算时 |
|---|---|---|
| 战斗阶段 `SET_MOVE` | 保存规划，不移动舰队 | 执行位移 |
| 部署阶段 `SET_MOVE` | 立即调整部署位置 | 不适用 |
| `CLEAR_MOVE` | 删除对应规划 | 不再执行被删除的规划 |
| 整编、拆分、航向、自动航行、交战设置、潜艇部署 | 立即修改状态 | 不重复执行 |
| 非无线电航空命令 | 立即修改任务或算子状态 | 推进整备、起降、飞行和战斗 |
| 无线电命令 | 保存规划并占用本回合通信机会 | 判定通信并成功时执行 |

玩家 ready 后，本侧所有命令均被拒绝，不能用新命令自动取消 ready。

### 3.3 舰队、部署与潜艇命令

| `type` | 主要字段 | 契约 |
|---|---|---|
| `SET_MOVE` | `unit_id, coord` | 部署阶段放入合法部署格；战斗阶段规划移动到 `legal_moves` 中的相邻格 |
| `CLEAR_MOVE` | `unit_id` | 撤销该舰船所在舰队本回合的移动规划；无需 `coord` |
| `SET_HEADING` | `unit_id, heading` | 设置舰队朝向；是否可改由服务端按移动和每回合限制校验 |
| `SET_AUTO_NAV` | `unit_id, coord, engagement?` | 设置自动航行目的地及交战策略；空 `coord` 取消自动航行 |
| `SET_ENGAGEMENT` | `unit_id, engagement` | 设置交战策略；含航母舰队的限制由服务端校验 |
| `MERGE_FLEET` | `unit_id, target_unit_id` | 将源舰队并入目标舰船所在舰队；两队须同格且分属不同舰队 |
| `REORGANIZE_HEX` | `coord, assignments` | 重编该 hex 的全部己方舰船；映射不得遗漏或包含其他单位 |
| `SPLIT_FLEET` | `unit_id, split_ids` | 将指定舰船拆成新舰队；允许混乱舰队拆分，不附加整编移动锁 |
| `DEPLOY_SUB` | `unit_id, coord` | 部署阶段摆放潜艇；`unit_id` 取 `submarines[].id` |

`MERGE_FLEET` 与 `REORGANIZE_HEX` 在战斗阶段会使相关编队锁定移动 3 回合；混乱舰队不能合并或整编。部署阶段允许整编、合并和拆分，且不附加该锁。拆分不重置移动冷却或停留计时。

整编与拆分结果仍须满足剧本的舰队规模和护航要求。`REORGANIZE_HEX` 示例：

```json
{
  "command_id": "01J...",
  "base_turn": 1,
  "type": "REORGANIZE_HEX",
  "coord": "0302",
  "assignments": {
    "a-flag": "a-flag",
    "a-screen": "a-flag"
  }
}
```

### 3.4 航空命令

航空命令中的任务 ID 取自 `aviation.tasks[].id` 或 `ground_tasks[].id`，由服务端生成。向已有侦察或打击编队加入飞机时使用 `task_id`。修改整个编队时，将任务 ID 放在 `aircraft_id` 中；机场编队级命令还应把同一个任务 ID 填入 `task_id`，确保服务端按岸基任务分流。客户端不应把任一成员飞机 ID 当作全组 ID 使用。

| `type` | 主要字段 | 契约 |
|---|---|---|
| `SET_LOADOUT` | `aircraft_ids, loadout` | 设置挂载；可用 ID 读取算子的 `loadout_options`，空串卸弹 |
| `LIFT_TO_DECK` | `aircraft_ids` | 舰载算子排队上甲板；机场算子表示滑行到跑道 |
| `RETURN_TO_HANGAR` | `aircraft_ids` | 取消未升空算子的任务或上甲板请求，并安排返回机库 |
| `RECOVER_AIR` | `aircraft_ids` | 直接回收空中 CAP；其他任务按自动返航或无线电召回处理 |
| `LAUNCH_CAP` | `aircraft_ids` | 战斗机执行 CAP；机场 CAP 只守本机场，远程掩护使用 `LAUNCH_COVER` |
| `LAUNCH_COVER` | `aircraft_ids, coord?` | 战斗机掩护指定 hex；留空时为本机场上空 |
| `LAUNCH_RECON` | `aircraft_ids, heading, sector_width, task_id?, lane?` | 新建扇面侦察；加入已有组时继承其参数 |
| `LAUNCH_STRIKE` | `aircraft_ids, coord?, target_fleet_id?, task_id?, lane?` | 新建或加入打击编队；允许先建组再设目标 |
| `LAUNCH_ASW` | `aircraft_ids` | 非战斗机执行反潜巡逻 |
| `LAUNCH_TRANSFER` | `aircraft_ids, coord?` | 转场到合法己方机场或具备接收条件的航母；可先建组再设目的地 |
| `SET_STRIKE_TARGET` | `aircraft_id=任务ID, task_id, coord, target_fleet_id?` | 设置未离场打击、掩护或转场编队的目标；`task_id` 填同一任务 ID |
| `SET_STRIKE_DISPERSION` | `aircraft_id=任务ID, task_id, disperse` | 设置整个打击编队集中或分散攻击；`task_id` 填同一任务 ID |
| `PRIORITIZE_AIR_TASK` | `aircraft_id=任务ID, task_id` | 将任务调整为最高优先级；`task_id` 填同一任务 ID |
| `SET_AIR_CYCLE` | `aircraft_ids, cycle` | 设置 CAP、侦察、空中掩护或反潜巡逻的自动循环 |

批量任务的主体、机型、作业阶段、母舰或机场限制不完全相同，均由服务端校验。攻击编队可以跨同一舰队的母舰并混合攻击机与护航战斗机；已离场编队不能再加入飞机。侦察半径由服务端按机型和燃料计算，命令不接受 `recon_radius`。

`SET_STRIKE_TARGET` 不会自动更换已有挂载。目标 hex 同时存在机场和舰队时，`target_fleet_id` 用来明确选择舰队；省略时按服务端的目标优先级处理。已经升空的编队改目标应使用 `RETARGET_AIR`，不要用它绕过无线电判定。

`PREPARE_AIR` 已废弃，固定返回 400；使用 `LIFT_TO_DECK` 或任务命令。前端不得自行推算整备、升降机、跑道、扇面、发现、命中或夜间着舰结果。

### 3.5 无线电命令

无线电命令只提交任务 ID，不提交 `aircraft_ids`。每个空中编队每回合最多登记一次通信，无论最终成功或失败都消耗机会。

| `type` | 主要字段 | 成功后的效果 |
|---|---|---|
| `RECALL_AIR` | `task_id` | CAP、侦察、打击或转场编队返航，并取消自动循环 |
| `RETASK_AIR` | `task_id, task_kind` | 打击与转场之间改令；`task_kind` 只能是 `attack` 或 `transfer` |
| `RETARGET_AIR` | `task_id, coord, target_fleet_id?` | 打击或转场编队改目标，保留任务类型 |
| `CLEAR_MOVE` | `task_id` | 在结算前撤销该任务已登记的无线电规划 |

CAP 和侦察只能召回，不能在空中改任务或改目标。通信成功率和实际结果由服务端在回合结算时计算。

## 4. Ready 与部署

```json
{"base_turn":1,"ready":true}
```

- 所有当前玩家 ready 后同步结算。结算成功时令回执中的 `turn` 为最新回合，双方 ready 自动清零。
- 战斗阶段只接受 `ready:true`；`ready:false` 返回 400。
- `phase == "deploy"` 时允许用 `ready:false` 撤销部署完成状态。除此之外，部署命令和战斗命令一样，在 ready 后会被锁定，不会自动撤销 ready。
- 双方完成部署后，`phase` 从 `deploy` 变为 `battle`，回合仍为 1。该次回执包含 `phase:"battle"`。
- 部署阶段只允许 `SET_MOVE`、`CLEAR_MOVE`、`MERGE_FLEET`、`REORGANIZE_HEX` 和 `DEPLOY_SUB`。未部署完本方潜艇时不能完成部署。

回执示例：

```json
{"accepted":true,"phase":"battle","turn":1}
```

## 5. 私有快照

WebSocket `GET state` 的 RPC 响应体与玩家补丁的全量底座使用同一结构。

### 5.1 顶层字段

| 字段 | 内容 |
|---|---|
| `room_id, status, turn, phase, solo` | 房间标识、状态、当前回合、部署/战斗阶段、是否单人房 |
| `scenario, map, objectives, outcome` | 剧本摘要、地图、胜利目标；对局未结束时 `outcome` 为 null |
| `you, players, join_token` | 当前玩家、房间玩家及在线/ready 状态；双方玩家都获得 `join_token`，可用于邀请和观战 |
| `orders, events` | 仅本玩家本回合的待结算命令；当前阵营可见事件 |
| `units, fleets` | 当前阵营可见舰船及舰队投影；己方包含完整卡面和 `legal_moves`，敌方按情报等级裁剪 |
| `aviation` | 舰载航空：`carriers` 为母舰设施与算子，`tasks` 为任务编队 |
| `airfields, ground_air` | 可见机场，以及机场设施、容量和机场算子 |
| `ground_aircraft, ground_tasks` | 机场算子的通用航空投影、机场任务编队 |
| `enemy_air` | 已发现的敌方空中编队接触，不包含其内部调度数据 |
| `air_combats, sub_battles` | 当前阵营可见的空战报告、双方可见的潜艇战报 |
| `submarines` | 部署阶段的本方可部署潜艇；开战后仅为本方正在追踪的敌方潜艇 |
| `fleet_stays, fleet_sessions` | 己方舰队上一回合位置、连续停留计时 |
| `shore_supplies, enemy_shore_supplies` | 可见滩头物资及机场对应的敌方物资投影 |
| `radio_used, radio_tasks` | 本方已通信任务 ID、当前可通信空中编队及状态 |
| `weather_cells, wind, night, clock` | 天气格、风场与舰队迎风关系、夜间标记、剧本时间线 |
| `losses, scores, score_breakdown` | 按可见性裁剪的损失台账、双方 VP、VP 逐条来源 |
| `display` | 状态枚举和错误的展示文案字典 |

客户端不得假设未返回的敌舰、敌机或潜艇不存在，也不得用其他接口绕过战争迷雾。

### 5.2 常用结构

- `fleets[]` 以 `fleet_id` 为稳定编队标识，`id` 是当前代表舰 ID。成员位于 `members[]`；`count/strength/max_strength` 是服务端汇总值。
- `units[]` 与舰队成员使用舰船 ID。`card.speed_class` 是基础速度档，`effective_speed` 是计入损伤后的当前速度档；`card.move_cycle`、`move_cooldown` 和 `legal_moves` 用于移动展示。
- 敌方 `fleets[]` 的 `contact_level/contact_size/contact_stale_turns` 描述情报等级、规模和未刷新回合数。低等级不会返回完整舰船卡。
- `aviation.carriers[]` 包含机库/甲板占用、作业状态、阻塞原因、升降机和算子。`aviation.tasks[]` 与 `ground_tasks[]` 包含任务阶段、完整算子、坐标、目标、优先级和返航预估。
- 算子的 `loadout_options/loadout_label` 由服务端生成。`aircraft[].status` 是单个算子的作业状态，`tasks[].phase` 是编队阶段，两者不能混用。
- 侦察任务的逐机几何位于 `recon_sectors[]`；距离参考为 `distance_reference:{origin,max_distance,cells[]}`。
- `clock` 包含 `day/time/night/service_open/launch_open/service_start/launch_start/recover_by/turn_minutes/night_in_turns`；剧本没有时间线时为 null。
- `wind` 包含盛行风 `wind`、逐格方向 `cells` 和己方舰队关系 `fleets`。方向数字 0–5 依次为 NE/E/SE/SW/W/NW，记录风的来向。
- `score_breakdown` 以得分阵营分组。损失条目的 `faction` 是损失所属方，因此会出现在对方的得分组中。
- `outcome` 包含 `winner/level/reason/scores/turn/turn_cap`；`turn_cap` 是布尔值，不是回合上限数字。

具体战斗数值和胜负公式以规则书及服务端结果为准，客户端不应另写一套。

## 6. WebSocket RPC

路径：`GET /ws`。升级时校验同源；没有 `Origin` 的非浏览器客户端可直接连接。所有 RPC 和推送均为文本帧。

### 6.1 玩家连接

玩家使用 `gq_session` Cookie 连接。可选查询参数 `room_id` 仅用于校验会话与房间是否一致，不匹配返回 403。

连接后先请求全量快照；服务端不会替玩家连接自动建立补丁底座：

```json
{
  "type": "rpc_request",
  "id": "req-17",
  "method": "GET",
  "path": "/api/v1/rooms/ROOM/state"
}
```

命令请求示例：

```json
{
  "type": "rpc_request",
  "id": "req-18",
  "method": "POST",
  "path": "/api/v1/rooms/ROOM/commands",
  "body": {
    "command_id": "01J...",
    "base_turn": 1,
    "type": "SET_MOVE",
    "unit_id": "a-flag",
    "coord": "0203"
  }
}
```

应答复用请求 `id`，`status` 使用对应 HTTP 状态码：

```json
{"type":"rpc_response","id":"req-18","status":200,"body":{"accepted":true,"command_id":"01J...","turn":1}}
```

RPC 限制：

- `id` 非空且不超过 128 字节，`path` 不超过 512 字节，不能带 query 或 fragment。
- `method` 只能是 GET 或 POST。
- 只允许当前房间的 `GET state`、`GET events`、`POST commands`、`POST ready`。
- 请求体规则、幂等、错误码与 HTTP 一致。
- 会话失效时，首个 RPC 会收到 401 响应，随后连接关闭；客户端应返回大厅重新加入。

### 6.2 观战连接

使用 `GET /ws?mode=spectator`。无 Cookie 连接也按观战流程处理。连接后 10 秒内第一条消息必须是：

```json
{"type":"subscribe","room_id":"ROOM","token":"XXXX-XXXX-XXXX"}
```

订阅失败返回 `{"type":"error","error":"..."}` 并关闭连接。成功后首帧为全量，之后为补丁：

```json
{"type":"snapshot","view":{}}
{"type":"snapshot","patch":{}}
```

观战视图合并双方合法视图，并标记 `spectator:true`。它不包含玩家私有 `orders` 或 `join_token`，也不公开潜艇实时位置。观战连接不使用 RPC。

### 6.3 心跳、重连与队列

- 服务端每 30 秒发送 WebSocket ping；90 秒未收到 pong 会断开。
- 每条连接的发送队列为 32 帧；客户端读取过慢导致队列满时，服务端主动断开。
- 玩家重连后必须重新 `GET state`，以新快照作为补丁底座。不要在房间中轮询 HTTP 状态。

## 7. 状态补丁

玩家收到：

```json
{"type":"state","patch":{}}
```

补丁是同一客户端上一份投影与新投影的差异，不是按命令类型预定义的消息。没有变化时不推帧。

### 7.1 合并规则

1. 普通对象逐键递归合并，未出现的键保持原值。
2. 标量、普通数组以及不能安全增量表达的实体数组整块替换。
3. 实体数组的增量形式为：

```json
{
  "upsert": [{"fleet_id":"TF17","coord":"0203"}],
  "remove": ["TF44"]
}
```

实体主键依次取第一个非空字段：`id`、`unit_id`、`task_id`、`carrier_id`、`fleet_id`、`coord`。`upsert` 只含变化属性，必须合并进原实体，不能直接替换原对象。

4. 字符串键 map 的增删形式为：

```json
{"set":{"A":12.5},"remove":["B"]}
```

5. `null` 是真实业务值，不表示删除键。`0`、`false`、空字符串和空数组同样要照常覆盖。个别按所有权或可选性定义的字段可能不出现在全量快照中。
6. 实体数组只有在原顺序可保持且新元素追加到尾部时才使用增量；其他情况会重发整个数组。事件数组按旧到新排序，便于追加。

如果本地底座缺失、合并失败或曾跳过帧，应断开并重连，再获取全量快照。不能继续把后续补丁应用到可疑底座上。

## 8. 常见状态码

以下状态码用于大厅 HTTP 响应和 WebSocket `rpc_response.status`。旧房间 HTTP 路径一律返回 404，不应再用于重试或降级。

| 状态码 | 含义 |
|---|---|
| 200 | 请求或命令成功；也可能是幂等命令的首次回执 |
| 201 | 房间创建成功 |
| 400 | JSON、未知字段、参数、命令合法性或业务条件不满足 |
| 401 | 玩家 Cookie 缺失或会话失效 |
| 403 | 会话不属于该房间、RPC 路径不允许、观战 token 不正确 |
| 404 | 房间、玩家、接口或观战 token 不存在 |
| 409 | 阵营席位已占用、单人房已满、`base_turn` 与当前回合不一致、ready 后操作被锁、持久化结算冲突 |
| 500 | 状态读取、编码、结算或数据库内部错误 |
| 503 | 闲置房间正在重载；通常可直接重试 |
