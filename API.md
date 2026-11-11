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

`coord`必须位于该单位服务器返回的`legal_moves`中。到达可移动时机后，移动由服务器立即生效并递增`version`，不生成待结算移动命令；普通/快速舰队随后冷却3回合，纯高速驱逐舰2回合，慢速5回合，极慢10回合，失去动力24回合。全队按最慢航速计算，进水降低航速档位；冷却在回合结算开始递减，未归零时返回空的`legal_moves`并拒绝移动。`CLEAR_MOVE`仅用于取消旧版协议遗留的待结算移动命令，包含`command_id`、`base_version`、`type`和`unit_id`。整编后同样锁定3回合，交战后混乱的舰队不能整编。

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

- 航空命令可使用`aircraft_ids`提交1至20个算子，旧`aircraft_id`单算子格式继续兼容。CAP和侦察必须同母舰、同机型、同阶段；攻击编队可混合攻击机型并跨同一舰队的多艘母舰。所有设施、容量与任务状态均由服务端一次校验和结算。
- `PREPARE_AIR`：已废弃并返回400。整备、提升、放飞必须绑定任务，客户端不能脱离任务直接切换算子阶段。
- `RETURN_TO_HANGAR`：取消任务、优先级和任务参数。`service`立即重置为`hangar`；`deck/ready`转为`recovery`，等待本舰升降机下降后才进入`hangar`。`elevator`中的算子不能中途撤回。
- `LAUNCH_CAP`：将战斗机分配至CAP任务。机库算子立即形成预备任务并进入`service`；已经在甲板的算子立即独立升空。每支舰队共享`cap-<fleet_id>`任务组和一面优先级旗，CAP算子仍独立起飞，不等待攻击编队。
- `LAUNCH_RECON`：将侦察机或攻击机分配至扇面搜索任务，额外提交`heading`和`sector_width`。允许`recon`、`torpedo_bomber`（TBF）和`dive_bomber`（SB2C），拒绝战斗机。例如`{"command_id":"01J...","base_version":1,"type":"LAUNCH_RECON","aircraft_id":"air-b-10","heading":"W","sector_width":60}`。航向为`E/SE/SW/W/NW/NE`，角度只能为60或120。`recon_radius`省略或为0时使用服务端`recon_radius_limit`；侦察算子独立起飞，不等待其他任务。每支舰队最多同时创建两个侦察组；向已有组加入飞机时提交`task_id`，同一舰队不同母舰的不同可搜索机型可以分批加入，并继承原组的航向、扇面及半径，而不是自动创建或轮流进入另一组。对已经属于该`task_id`的算子重复提交同类型加入命令是幂等操作，不改变任务参数或算子状态；机库整备中的算子不能再作为待分配飞机拖入任务，必须从任务卡拖回机库取消后再分配。跨舰队加入会被拒绝，航空界面也只投影当前选中舰队的舰船和任务。每个侦察任务额外返回`recon_sectors`数组，逐机包含`aircraft_id/status/origin_coord/current_coord/sector_hexes/current_ring_hexes/distance_reference`；同组飞机不再共用唯一扇面条带。未升空的舰上飞机使用母舰当前hex且不生成实际搜索扇面，母舰移动后立即同步坐标；已升空飞机保留自己的起飞顶点和航线位置。
- `LAUNCH_STRIKE`：由舰载俯冲轰炸机和鱼雷机和护航战斗机建立攻击编队，可以先创建编队再设置目标。把另一艘同舰队母舰的算子加入已有编队时提交`"task_id":"air-b-03"`并继承目标，已离场编队拒绝加入。部分算子先起飞时为`attack_holding`，任务为`assembling`，每等待一回合`remaining_range`减1；全部成员升空后若未设目标继续等待，不会离场。
- `SET_STRIKE_TARGET`：通过`aircraft_id`提交攻击`task_id`，通过`coord`提交目标hex，服务端仅接受当前视野或情报中已知的敌方目标。全部成员设置`target_coord`并立即重新判定离场。离场后每回合最多向目标飞行2 hex并逐格耗油，抵达后记录`attack_arrived`战报。任务和算子包含`target_coord`，抵达后由Go完成CAP空战、防空、投弹、撤离空战和返航结算。
- 任务创建后立即出现在`aviation.tasks`。通常第1回合为`servicing`，第2回合占用具体升降机并显示`lifting`，第3回合完成提升后由任务放飞并显示`launching`，随后为`active`。算子同时记录`elevator_id`；`preparing`表示已经在甲板但因容量或甲板作业而等待，`assembling`仅用于攻击编队。侦察参数在分配时持久化，扇面顶点使用实际起飞时的母舰位置。
- `RECOVER_AIR`：将CAP算子回收到各自母舰甲板，状态改为`recovery`；后续回合一一占用升降机执行`lowering`，完成后才回到机库。允许一次回收同舰队多艘母舰的CAP，服务端逐舰检查甲板容量、损伤和不相容作业。侦察机沿航线自动返航，使用同样的甲板与下降流程。
- `PRIORITIZE_AIR_TASK`：将指定预备、等待或空中任务置为最高优先级，其他任务顺序由服务端调整；优先级参与各舰升降机调度，`queue_position`返回连续队列位置。
- 这些命令都立即生效并递增`version`。每艘航空舰在`Unit.deck`中持有独立`length/operation/elevators`，每部升降机持有`id/kind/capacity/status/task_id/aircraft_ids`。所有整备、设施调度、编队集合、机库与甲板数量、扇面hex、发现判定、情报、燃油与任务排序仅由Go服务端计算。新自动起飞的算子从下一回合开始耗油和移动，空中CAP、等待及离场攻击算子每回合剩余航程-1，非空中算子补满航程。

### Ready

```json
{"base_version":1,"ready":true}
```

响应为最新私有快照。所有当前玩家ready后，服务器同步完成结算并提升`turn`与`version`。单人沙盒中唯一玩家ready即可结算。已ready玩家提交新命令时自动取消自己的ready。

同hex水面舰队在结算时交战：按权重选目标，每点火炮独立判定，并处理突袭、鱼雷、DCT损伤和混乱。战报包含目标、命中率和命中数量，损伤写入单位`damage`并持久化；沉没舰船及其航空算子移除。回合开始递减移动冷却和旧混乱，处理到期的火灾、进水与甲板修复，新损伤不在同回合提前修复。

## 私有快照

`state`响应包含：

- `scenario`、`map`、`turn`、`version`、`status`；
- `you`和`players`；
- 当前阵营可见的`units`；己方单位额外包含`legal_moves`。每艘舰船包含`definition_id`、当前/初始生命值、服务端生成的`card`角色卡、`move_cooldown`、`confusion`和当前`damage`，字段涵盖火炮、防空、损管、雷达、航速、鱼雷、航空设施、货物、火灾、进水及设备损伤等；
- 服务端生成的`fleets`舰队投影，包含`fleet_id`、成员列表、数量、总强度、朝向、`move_interval/move_cooldown`、混乱、合法移动和`distance_reference`；距离参考由`origin/max_distance/cells`组成，每个cell包含服务端计算的`coord/distance`。情报目标额外包含`contact_level/contact_size`：1级仅规模文字、2级舰队剪影、3级编队剪影、4级具体舰船卡，低等级不返回具体舰船属性；
- `aviation.carriers`：全部航空作业舰，包含机库占用/容量、`deck_length/deck_used`、甲板作业类型、独立的`elevators`对象数组和航空算子；`aviation.tasks`：按服务端优先级排序的任务，包含`phase`、`carrier_ids`、`fleet_id`、`airborne_count`及完整算子列表。空中任务包含以当前位置为中心的`distance_reference`，侦察任务额外包含扇面与航线字段，客户端只渲染这些服务器结果；任务另含`queue_position`；
- `air_combats`：当前阵营可见的空战报告，包含双方机型、数量、损失、防空与投弹命中、胜负和摘要；前端动画只消费这些权威结果；
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
