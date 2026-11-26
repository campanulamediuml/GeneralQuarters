# General Quarters 接口与扩展协议

运行时OpenAPI文档：`GET /api/v1/openapi.yaml`。在线简版文档：`/api/docs`。架构与数据设计见`DESIGN.md`。

## 通用约定

- 除健康检查和文档外，接口使用创建或加入房间时签发的HttpOnly Cookie`gq_session`。
- 成功响应为JSON，错误为`{"error":"人类可读说明"}`。
- `room_id`标识房间；`join_token`是邀请凭证；二者都不是玩家会话。
- `base_turn`可选：提交时看到的回合号。只有回合已结算的过期提交会被拒绝（409）；同回合内重复提交一律接受，后写的覆盖先写的。不比对版本号。
- `command_id`由客户端生成，在玩家范围内唯一。断线重试必须沿用原ID。
- hex坐标为`ABCD`，AB为行、CD为列，例如`0203`。
- 闲置房间只释放内存、不删库：房间连续 12 小时没有活动（没有命令/结算，也没有玩家上下线或观战连入），且当前没有玩家在线时，服务端会停掉该房间在内存里的 worker。房间、事件、回合快照和指令回执全部保留，下一次请求按需从库里重新载入，响应跟一直挂着没有区别；只有极小概率撞上释放动作并发时会返回 503 `房间已释放内存，正在重新载入，请重试`，直接重试即可。

## HTTP接口（大厅与脚本兼容）

网页只在**大厅**使用 HTTP：建房、凭邀请 token 加入。进入房间后，状态读取、命令和 ready 全部走 WebSocket RPC，不再轮询。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/healthz` | 健康检查 |
| POST | `/api/v1/rooms` | 创建房间并自动载入剧本，默认`_coral_real` |
| POST | `/api/v1/rooms/join` | 使用12位token加入 |
| GET | `/api/v1/rooms/{roomID}/state` | 房间私有快照；网页不再调用，保留给脚本/旧客户端 |
| GET | `/api/v1/rooms/{roomID}/events` | 最近100条当前阵营可见事件，按时间正序（旧→新）；同上 |
| POST | `/api/v1/rooms/{roomID}/commands` | 设置或取消单位命令；同上，语义与 WS RPC 一致 |
| POST | `/api/v1/rooms/{roomID}/ready` | 设置或取消ready；同上 |
| GET | `/watch/{token}` | 只读观战页面：路径里直接放房间 token（如 `XXXX-XXXX-XXXX`），服务端解析成房间并通过 WebSocket 实时接收快照 |
| GET | `/watch/{roomID}/{token}` | 观战页兼容形式：房间 ID 与 token 都放在路径里 |
| GET | `/api/v1/rooms/{roomID}/spectate?token=...` | 兼容旧客户端的观战 HTTP 快照：用房间join_token认证 |
| GET | `/api/v1/openapi.yaml` | 机器可读OpenAPI定义 |

### 创建房间

```json
{"name":"玩家A","scenario_id":"_coral_real"}
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
  "base_turn":1,
  "type":"SET_MOVE",
  "unit_id":"a-flag",
  "coord":"0203"
}
```

`unit_id`必须是**舰船 id**（`fleets[].members[].id` 或 `units[].id`），不能填 `fleets[].fleet_id`——填舰队号会返回 400"单位不存在或不属于当前玩家"。`legal_moves` 挂在舰队上，但命令收的是旗舰/成员舰的 id。`SET_HEADING` 同理，航向只有 `E/SE/SW/W/NW/NE` 六个方向。

`coord`必须位于该单位服务器返回的`legal_moves`中。`SET_MOVE`提交后**只登记本回合的移动规划**（写入房间的待结算`orders`，舰队原地不动），双方就绪、回合结算时统一执行位移；即时生效的只有`SET_HEADING`/`SET_LOADOUT`/`SET_AUTO_NAV`/`SET_ENGAGEMENT`这类命令。到达可移动时机后，普通/快速舰队每3回合移动一次（移动后冷却2回合），纯高速驱逐舰每2回合（冷却1回合），慢速每6回合（冷却5回合），极慢每9回合（冷却8回合），失去动力每24回合（冷却23回合）。全队按最慢航速计算，进水降低航速档位；冷却在回合结算开始递减，未归零时返回空的`legal_moves`并拒绝移动。`CLEAR_MOVE`用于撤销本回合登记过的移动规划，包含`command_id`、`type`和`unit_id`；撤销后舰队本回合留在原地，也不会因此进入移动冷却。整编后同样锁定3回合，交战后混乱的舰队不能整编。

### 整编舰队

```json
{
  "command_id":"01J...",
  "base_turn":1,
  "type":"MERGE_FLEET",
  "unit_id":"a-screen",
  "target_unit_id":"a-flag"
}
```

`unit_id`为整编源，`target_unit_id`为目标舰队中的单位。双方必须属于当前玩家、位于同一hex且`fleet_id`不同。整编由服务器立即生效。

### 整个hex整编

```json
{
  "command_id":"01J...",
  "base_turn":1,
  "type":"REORGANIZE_HEX",
  "unit_id":"hex-0302",
  "coord":"0302",
  "assignments":{"a-flag":"a-flag","a-screen":"a-flag"}
}
```

`assignments`必须包含该hex全部己方单位，值为目标`fleet_id`。同一操作可以合并、拆分或转移成员；不允许遗漏单位、包含其他阵营单位或跨hex整编。整编由服务器立即生效。快照中每个单位还包含`heading`和`moved`。

### 航空命令

```json
{"command_id":"01J...","base_turn":1,"type":"LAUNCH_CAP","aircraft_ids":["air-b-01","air-b-02"]}
```

- 航空命令可使用`aircraft_ids`提交1至20个算子，旧`aircraft_id`单算子格式继续兼容。CAP和侦察必须同母舰、同机型、同阶段；攻击编队可混合攻击机型并跨同一舰队的多艘母舰。所有设施、容量与任务状态均由服务端一次校验和结算。
- `PREPARE_AIR`：已废弃并返回400。整备、提升、放飞必须绑定任务，客户端不能脱离任务直接切换算子阶段。
- `RETURN_TO_HANGAR`：取消任务、优先级和任务参数。`service`立即重置为`hangar`；`deck/ready`转为`recovery`，等待本舰升降机下降后才进入`hangar`。`elevator`中的算子不能中途撤回。
- `LAUNCH_CAP`：将战斗机分配至CAP任务。机库算子立即形成预备任务并进入`service`；已经在甲板的算子立即独立升空。每支舰队共享`cap-<fleet_id>`任务组和一面优先级旗，同一舰队的多艘航母可以同批发CAP，CAP算子仍独立起飞，不等待攻击编队。
- `LAUNCH_RECON`：将侦察机或攻击机分配至扇面搜索任务，额外提交`heading`和`sector_width`。舰载机只允许`recon`、`torpedo_bomber`（TBF）和`dive_bomber`（SB2C）；**陆基机场除战斗机外都允许**，B-17/B-25/B-26 这类轰炸机同样可以挂扇面搜索（1942 年 5 月第 19 轰炸大队就是这么干的）。两种情况都拒绝战斗机。例如`{"command_id":"01J...","base_turn":1,"type":"LAUNCH_RECON","aircraft_id":"air-b-10","heading":"W","sector_width":60}`。航向为`E/SE/SW/W/NW/NE`，角度只能为60或120。**侦察半径不可提交**（命令里没有这个字段）：服务端一律按机型算`recon_radius_limit`，客户端多传会被忽略；侦察算子独立起飞，不等待其他任务。每支舰队最多同时创建两个侦察组；向已有组加入飞机时提交`task_id`，同一舰队不同母舰的不同可搜索机型可以分批加入，并继承原组的航向和扇面，而不是自动创建或轮流进入另一组。对已经属于该`task_id`的算子重复提交同类型加入命令是幂等操作，不改变任务参数或算子状态；机库整备中的算子不能再作为待分配飞机拖入任务，必须从任务卡拖回机库取消后再分配。跨舰队加入会被拒绝，航空界面也只投影当前选中舰队的舰船和任务。每个侦察任务额外返回`recon_sectors`数组，逐机包含`aircraft_id/status/origin_coord/current_coord/sector_hexes/current_ring_hexes/distance_reference`；同组飞机不再共用唯一扇面条带。未升空的舰上飞机使用母舰当前hex且不生成实际搜索扇面，母舰移动后立即同步坐标；已升空飞机保留自己的起飞顶点和航线位置。
- `LAUNCH_STRIKE`：由舰载俯冲轰炸机和鱼雷机和护航战斗机建立攻击编队，可以先创建编队再设置目标；`target_fleet_id`可在同一hex有机场和舰队时指定打舰队。把另一艘同舰队母舰的算子加入已有编队时提交上一次返回的`task_id`（形如`"air-9f3c1a7b2d4e"`，是编队牌的随机 tag，服务端生成、持久化在存档里，客户端不要自己拼）并继承目标，已离场编队拒绝加入。部分算子先起飞时为`attack_holding`，任务为`assembling`，每等待一回合`remaining_range`减1；全部成员升空后若未设目标继续等待，不会离场。集结是「全员升空后在母舰/机场上空等齐再一起出发」，不是谁升空谁先走；编队一旦离场（成员进入`attack`）就拒绝再加入新算子（400：攻击编队已经离场），要加飞机只能等这一波回来重新编队。
- `SET_STRIKE_TARGET`：通过`aircraft_id`提交攻击`task_id`，通过`coord`提交目标hex，也可以用`target_fleet_id`显式指定敌方舰队；目标hex同时有敌方机场和舰队时，带`target_fleet_id`就打舰队，不带则维持机场优先。服务端仅接受当前视野或情报中已知的敌方目标。改目标导致对舰/对地类型变化时，未升空的算子会自动换挂载并退回整备（鱼雷改炸弹、炸弹改鱼雷都必须重新整备），已升空的算子会拒绝并要求返航。攻击编队每回合推进 2 hex（水上飞机/双翼机 1 hex，规则书 197 行），抵达后才结算；出航途中**每回合**按最新情报校正一次目标——打击编队认的是那支舰队（target_fleet_id），不是起飞时那一格 hex，舰载编队与机场编队一致；入夜前服务器**不再**把打击编队强制掉头，只发一次`night_warning`；入夜后也不凭天色统一召回——还在空中的算子按各自的油量判定决定何时撤，落地那一刻才按夜间规则判摔毁（航母 70%、机场 20%，侦察/掩护/CAP 同样适用），长航程机型熬到天亮落地就不会吃到这个判定。任务快照的`return_turns`、`turns_to_dark`、`night_margin`、`night_safe`给出"这波回不回得来"：`turns_to_dark`是距天黑还剩几回合（没有昼夜循环的剧本为 -1），`night_margin = turns_to_dark - return_turns`，小于等于 0 就是走完任务再返航会摸黑着舰。`night_safe`始终显式输出（false 不会被省略），AI 可以直接据此在出击前放弃一波。攻击编队另外带一份时间表：`turns_to_target`/`turns_to_landed`是从现在到"空袭打在目标上"和"最后一架落回来"的回合数（含整备提升、着舰排队），`eta_target`/`eta_landed`是同一笔账换算成的`HH:MM`时刻，`lands_at_night`表示按预估落舰时已在夜间；前端在点完打击目标后直接弹窗显示，`lands_at_night`为真时加一行夜间返航警告。同样的道理，敌方编队/算子的情报里带`contact_stale_turns`：这条情报已经连续几回合没更新（0 = 本回合刚刷新），配合`contact_level`判断坐标还能不能信。全部成员设置`target_coord`并立即重新判定离场。离场后每回合最多向目标飞行2 hex并逐格耗油，抵达后记录`attack_arrived`战报。任务和算子包含`target_coord`，抵达后由Go完成CAP空战、防空、投弹、撤离空战和返航结算。返航成员是持续拆组的：每回合新到返航下限的算子各自拆成一支新的返航编队（每次拆组都会分配新的随机 tag），一次超远程打击沿途可能出现多支返航编队，同一 hex 上并存"执行中"和"返航中"两张牌时`current_coord`和`returning`都以还在空中的成员为准（ID 最小的成员可能已经落地）；最后剩下的那支保留原 tag。任务快照的`origin_coord`一律是这一次出海的起飞顶点（航母或机场坐标，转场/循环后重算），`current_coord`是当前位置；`target_coord`对打击是目标hex，对掩护/转场是目的地，对CAP是值班空域（随母舰移动或前出点），侦察任务的目标为空、几何看`recon_sectors`。三项都显式输出（空串表示没有）。
- `LAUNCH_COVER`：空中掩护，机场算子也可以下（只有战斗机，其他机型返回400）。语义和 CAP 一致：`coord` 填要掩护的 hex——舰队所在格或机场格都行，留空表示掩护本机场上空；飞机飞过去后覆盖目标周围1 hex；如果掩护目标hex里有己方舰队，编队会自动跟随该舰队移动，而不是钉在起飞时的hex。敌机进入这个范围按 CAP 参战。目的地可以先用 `SET_STRIKE_TARGET` 改，不必重新分配任务。
- `LAUNCH_CAP`（机场算子）：只掩护本机场上空，`coord` 只能留空或等于本机场坐标；远程给其他友军（舰队所在格或其他机场）提供掩护请用 `LAUNCH_COVER`，掩护编队会跟着被掩护的舰队移动。
- `LAUNCH_TRANSFER`：转场。舰载机是换母舰或上岸；机场算子在己方机场之间转移，其中**舰载型飞机（非 `airfield_only`、非水机）也可以转场到己方航母**——`coord` 填舰队所在 hex，要求该 hex 上有还有机库空位的航母，抵达后转为舰载机算子进入 `recovery`。`coord`填目的地机场hex，留空时先建任务、再按打击任务的同样方式用`SET_STRIKE_TARGET`在地图上点选目的地（没有目的地的转场任务停在跑道上不起飞）。服务端校验目的地是己方机场、有可用跑道、能起降该机型（小型机场只收战斗机、中型机场不收战略轰炸机，水上飞机基地只收水上飞机，反过来轮式飞机也进不了水机基地），且距离不超过转场航程（机种基础航程×1.3×1.7=×2.21，副油箱与转场加成乘算）。起飞后每回合飞2 hex，全部抵达后落到新机场并进入`recovery`；目的地中途失守则返航原基地。机场没有机库、默认无停放上限（99999，逻辑上和航母机库共用一套检查但不触发），转场照收；只有剧本在`airfields[].hangar_capacity`写了小值时才可能返回`停放机位已满（N/M）`，此时舰载机备降会跳过该机场。上限只统计停在机场上的算子，升空执行任务的不占名额。
- 任务创建后立即出现在`aviation.tasks`。舰载机第1回合为`servicing`（机库加油整备 1 回合），第2回合占用具体升降机并显示`lifting`，第3回合提升完成当回合由任务放飞并显示`launching`，随后为`active`——整备、提升、起飞各 1 回合。**机场编队没有`lifting`**：机库整备完成后直接`taxiing`滑行到跑道，下一步就升空（整备→滑行→起飞，同样是 3 回合，但少一道升降机容量闸门）；机场的整备和滑行不占跑道作业次数，只有起飞/降落占（每条可用跑道每回合 12 次，用完的编队留在跑道待命，下回合再走）。算子同时记录`elevator_id`；`preparing`表示已经在甲板但因容量或甲板作业而等待，`assembling`仅用于攻击编队。侦察参数在分配时持久化，扇面顶点使用实际起飞时的母舰位置。
- `RECOVER_AIR`：将CAP算子回收到各自母舰甲板，状态改为`recovery`；后续回合一一占用升降机执行`lowering`，完成后才回到机库。允许一次回收同舰队多艘母舰的CAP，服务端逐舰检查甲板容量、损伤和不相容作业。侦察机沿航线自动返航，使用同样的甲板与下降流程。
- `PRIORITIZE_AIR_TASK`：将指定预备、等待或空中任务置为最高优先级，其他任务顺序由服务端调整；优先级参与各舰升降机调度，`queue_position`返回连续队列位置。
- `SET_AIR_CYCLE`：开启或取消自动任务循环（规则书188–191行），提交`aircraft_ids`和`cycle`（true=开启，false=取消）。语义是"设为"而不是"取反"，重复提交同一条命令不会来回翻转。CAP、侦察和空中掩护支持循环：降舰后自动重新整备并按原参数（巡逻空域／航向+扇面+半径／掩护hex）再次升空，侦察循环的掩护hex会跟随被掩护舰队的当前位置。航程不足、母舰失效或玩家取消即结束循环，并产生`air_cycle_end`战报。算子快照返回`on_cycle/cycle_kind/cycle_opt_out`；循环中的算子在前端显示蓝色🔁角标，点击即提交本命令。主动`RECOVER_AIR`或无线电`RECALL_AIR`同样会取消循环。机场算子（`GroundAir`）从同一版本起也支持循环：发起时仍是`aircraft_ids`+`cycle`，服务端按算子 ID 自动分流到机场那套校验，落地后按原巡逻点／航向+扇面重新滑行、整备、起飞；机场失守、跑道被打光或玩家取消即结束循环。
- 这些命令都立即生效。每艘航空舰在`Unit.deck`中持有独立`length/operation/elevators`，每部升降机持有`id/kind/capacity/status/task_id/aircraft_ids`。所有整备、设施调度、编队集合、机库与甲板数量、扇面hex、发现判定、情报、燃油与任务排序仅由Go服务端计算。新自动起飞的算子从下一回合开始耗油和移动，空中CAP、等待及离场攻击算子每回合剩余航程-1，非空中算子补满航程。

### Ready

```json
{"base_turn":1,"ready":true}
```

响应只是一条回执`{"accepted":true,"turn":N}`，最新状态走WebSocket推送。所有当前玩家ready后，服务器同步完成结算并提升`turn`。单人沙盒中唯一玩家ready即可结算。

**部署阶段**（`state.phase == "deploy"`）：双方都ready才算"部署完毕"，服务器把`phase`切到`battle`开战，**回合号不增加**（部署不算回合，仍停在第 1 回合），这一帧的回执是`{"accepted":true,"phase":"battle","turn":1}`。部署阶段是唯一允许`{"ready":false}`的场景，作为脚本兜底；点过部署完毕后本侧的任何部署动作（`SET_MOVE`/`CLEAR_MOVE`/`MERGE_FLEET`/`REORGANIZE_HEX`）都会被拒绝（409），不再自动撤回ready。开战之后ready重新变成不可撤销。

**ready 即冻结**：一名玩家ready之后，本回合该玩家提交的任何 `/commands` 都会被拒绝（409，错误信息说明"本回合已就绪，操作已锁定"），不会再像以前那样自动撤回ready再接受命令。要改就得等下一回合。前端同时把这块棋盘锁成只读：不能拖舰队、不能改航行/交战/整编、不能放飞或改目标，只能查看。`{"ready":false}` 也已禁止：ready 一旦提交就不可撤销，脚本必须等本回合结算。

同hex水面舰队在结算时交战：按权重选目标，每点火炮独立判定，并处理突袭、鱼雷、DCT损伤和混乱。战报包含目标、命中率和命中数量，损伤写入单位`damage`并持久化；沉没舰船及其航空算子移除。回合开始递减移动冷却和旧混乱，处理到期的火灾、进水与甲板修复，新损伤不在同回合提前修复。

## 私有快照

`state`响应包含：

- `scenario`、`map`、`turn`、`status`；`map.clouds` 一律为空字符串——逐格天气只在`weather_cells`里给；
- `phase`：`deploy`（开局部署）或 `battle`（正常对局）。剧本没写`deploy_zones`时直接是`battle`；
- `deploy_zone`：己方部署区，`{anchor,radius,cells}`。`cells` 是行优先的 `'0'/'1'` 串，标出锚点半径内可以摆船的格位；前端已经改用`legal_moves`的绿色高亮来画可放区域，这个字段留给脚本/第三方客户端；
- `weather_cells`：行优先、每格一个字符 `'0'..'3'`（晴/多云/降雨/风暴）。服务端内部是按"压了几层云"记账的（3 层以上就是风暴），**4 层以上到前端同样显示 3**；
- `wind` 和 `wind_cells`：`wind` 是盛行风与本回合各己方舰队所在格的朝向关系（`wind.cells`、`wind.fleets`），`wind_cells` 是行优先、每格一个数字 `0`–`5`，依次对应 NE/E/SE/SW/W/NW（气象惯例记来向：SE = 风从东南吹来）；
- `night`：剧本有没有昼夜循环时的夜间标记；`clock` 见下条；
- `you`和`players`；
- 当前阵营可见的`units`；己方单位额外包含`legal_moves`。每艘舰船包含`definition_id`、当前/初始生命值、服务端生成的`card`角色卡、`move_cooldown`、`confusion`和当前`damage`，字段涵盖火炮、防空、损管、雷达、航速、鱼雷、航空设施、货物、火灾、进水及设备损伤等；
- `scores`：战果分，浮点数（航母/轻母/护航航母按机库容量，瘫痪未沉的航母按机库容量一半（`losses[阵营].crippled`，条件为飞行甲板摧毁 / 结构损失超过 80% / 失去动力，每艘只记一次，沉没后按整艘计）；战列舰与战列巡洋舰 6、重巡 3、轻巡 2、驱逐 1、运输船 1，每摧毁一个航空算子 0.25；完成的投送目标按 `value` 计分）。剧本声明`kind:"score"`目标时，在回合上限（珊瑚海 360 回合）按双方 VP 比值判胜负：小于 1.25 平分秋色，1.25 到 2 战术胜利，大于 2 战略胜利；投送完成/失败不提前结算；`outcome.level`返回`draw/tactical/strategic`，日军未完成 20 单位投送前不能战略胜利；
- `score_breakdown`：战果分的逐条来源，按得分阵营分组为 `{A:[…],B:[…]}`，每条`{kind,faction,text,ship_id?,type?,turn?,count?,points}`。`kind`取`ship`（击沉）、`crippled`（航母瘫痪）、`aircraft`（航空算子损失，按架聚合）、`supply`（完成的投送目标）；`faction`是**这笔损失的归属方**——己方的损失算在对方账上，所以 `score_breakdown.A` 里的条目大多 `faction:"B"`；各条 `points` 之和恒等于 `scores`。前端侧栏 VP 旁的"得分明细"弹窗直接渲染它；
- `outcome`：仅对局结束后出现，包含`winner`（平局为空）、`level`（`draw/tactical/strategic`）、`reason`、双方`scores`、`turn`和`turn_cap`；
- 服务端生成的`fleets`舰队投影，包含`fleet_id`、成员列表、数量、总强度、朝向、`move_interval/move_cooldown`、混乱、合法移动和`distance_reference`；距离参考由`origin/max_distance/cells`组成，每个cell包含服务端计算的`coord/distance`。情报目标额外包含`contact_level/contact_size`，低等级不返回具体舰船属性：1级只返回一个占位算子（无规模、无舰种），2级只给编队大小（`small/medium/large`），3级返回逐舰的舰种但身份和属性仍未知，4级才返回完整舰船卡；`coord`是服务端固化的"最后已知位置"，情报过时只降等级不改坐标——白天脱离接触 4 级满 6 回合降到 3 级、之后每 3 回合降一级，夜间满 4 回合直接归零；
- `aviation.carriers`：全部航空作业舰，包含机库占用/容量、`deck_length/deck_used`、甲板作业类型、独立的`elevators`对象数组和航空算子；`aviation.tasks`：按服务端优先级排序的任务，包含`phase`、`carrier_ids`、`fleet_id`、`airborne_count`、`lane`（编队在面板里占的栏位，1 起；0 = 该编队没记过栏位，按位置兜底）及完整算子列表。任务只代表"还在整备、准备出击"的编队：全员升空后它从任务栏消失（属于海图的编队牌），着舰落地时打击编队直接解散，算子回到甲板/机库投影。`phase`取值 `servicing`（机库整备）、`taxiing`（机场滑行至跑道）、`lifting`（升降机提升）、`preparing`（甲板/跑道待飞）、`launching`、`assembling`、`active`、`recovering`（回收中，含下降回机库）；`lane`跟着编队走而不是按列表位置排：编队全部升空之后栏位交还，下一支新编队重新拿到 I。`LAUNCH_STRIKE`/`LAUNCH_RECON`可以带可选的`lane`（1 或 2）表示玩家把它拖进了哪一栏；那一栏已被占或没带时服务端取最小空栏。空中任务包含以当前位置为中心的`distance_reference`，侦察任务额外包含扇面与航线字段，客户端只渲染这些服务器结果；任务另含`queue_position`；
- `clock`：剧本时间线快照，包含`day/time/night/service_open/launch_open/service_start/launch_start/recover_by/turn_minutes`。`service_open`为false表示还没到整备开工时间，`launch_open`为false表示甲板上的算子还不能起飞，`night`为true且剧本`night_loss`为真时，{`recover_by`}之后仍在空中的算子会被强制摸黑返航，随后按燃油与夜间着舰风险（航母70%、机场20%）逐架判定。剧本没有`day_hours/turns_per_day`时该字段为null；
- `air_combats`：当前阵营可见的空战报告，包含双方机型、数量、损失、防空与投弹命中、胜负和摘要；前端动画只消费这些权威结果；
- 仅自己的`orders`；
- 按受众过滤的`events`；
- 仅房主可见的`join_token`。

客户端不得假设未返回的敌舰不存在，也不得从WebSocket或其他接口推导其真实位置。
## WebSocket

路径：`GET /ws`。创建房间、输入邀请 token 加入房间仍用 HTTP；进房之后状态读取、命令、ready 和房间变更推送全走这一条连接。升级后会做同源检查（没有 `Origin` 的非浏览器客户端直接放行）。

两条身份路径：

- **玩家**：握手带 `gq_session` Cookie（HTTP 建房/加入时下发），连接自动绑定会话所在房间。可选的 `?room_id=` 只用来校验"这条会话是不是这个房间的"，填错直接 403。
- **观战**：`?mode=spectator` 或不带 Cookie。连接后 **10 秒内**第一条消息必须是订阅帧，否则断开：

  `{"type":"subscribe","room_id":"ROOM","token":"XXXX-XXXX-XXXX"}`

  `token` 是房间的 join_token（观战页 `/watch/{token}` 会把它注入页面）。订阅失败回 `{"type":"error","error":"..."}` 并关闭连接。观战不占玩家席位、不改在线状态。

所有帧都是**文本帧**，单帧上限 1 MB。服务端每 30 秒发一次 ping，90 秒收不到 pong 即断开（客户端只需按标准 WebSocket 回 pong）。

### RPC

玩家侧统一 envelope：

```json
{"type":"rpc_request","id":"req-17","method":"GET","path":"/api/v1/rooms/ROOM/state"}
{"type":"rpc_request","id":"req-18","method":"POST","path":"/api/v1/rooms/ROOM/commands","body":{"command_id":"...","type":"SET_MOVE","unit_id":"...","coord":"0203"}}
```

应答沿用同一个 `id`：

```json
{"type":"rpc_response","id":"req-17","status":200,"body":{...}}
```

规则：

- `id` 非空且不超过 128 字节；`method` 必须是 GET/POST；`path` 不超过 512 字节、不能带 query 或 fragment。
- `path` 只允许 `/api/v1/rooms/{当前房间}/state`（GET）、`/events`（GET）、`/commands`（POST）、`/ready`（POST），且必须属于当前会话的房间，否则 403（"WebSocket 请求必须属于当前房间" / "不允许通过 WebSocket 调用此接口"）。
- `body` 就是原 HTTP 接口的请求体（≤1 MB）。服务端复用同一套校验、幂等 `command_id`、`base_turn` 和错误语义，所以错误码和 HTTP 完全一致（400/403/404/409/500）。
- 校验失败（路径、房间、格式）在读取侧就直接回 `rpc_response`，不进房间队列；业务失败（未就绪就提命令、回合已结算等）在 worker 里处理，同样以 `rpc_response` 返回。
- 会话已失效的连接如果先发 `rpc_request`，会收到 `status:401` 的应答后被关闭——前端据此回大厅重新加入，而不是无限重连。

### 推送

玩家连接**不会**主动收全量：首帧和重连都必须自己发一条 `GET state`，那条 `rpc_response` 的 `body` 既是数据，也是这条连接的补丁底座。之后房间状态变化时服务端只推补丁：

```json
{"type":"state","patch":{...}}
```

观战连接反过来：订阅成功后服务端立刻推 `{"type":"snapshot","view":{...}}`，之后是 `{"type":"snapshot","patch":{...}}`。

补丁形态：对象逐键收敛；实体数组（元素带 `id`/`unit_id`/`task_id`/`carrier_id`/`fleet_id`/`coord` 之一）给 `{"upsert":[只含变化属性的记录],"remove":[主键]}`，主键按上面那个顺序认第一个非空的值；其余值整块替换。客户端按主键找到节点、把 upsert 的属性**就地合并**进去（不重建对象，Vue 的响应式才能只重算用到那几个字段的地方），不需要理解业务语义。实测：一条移动命令约 100 B、点 ready 约 70 B、一次夜间结算约 300 B，而全量（严格状态同步、不省零值）约 290 KB，观战合并视图约 690 KB。

三条必须知道的细节：

- **视图是严格状态同步，零值不省略**。`0`、`false`、`""`、`[]`、`null` 都是真实值，会原样出现在快照和补丁里（Go 侧视图类型上不带 `omitempty`）。所以补丁里 `null` 就是 nil，客户端直接覆盖，不要理解成"删掉这个字段"。
- **键消失要显式表达**。字符串键 map 里少了键，JSON 对象本身说不出来，这种补丁给 `{"set":{变化的键},"remove":[消失的键]}`；实体数组本来就有 `{"upsert":[...],"remove":[...]}`。不是删键的时候，map 补丁还是普通对象。
- **顺序也要一致**。实体数组只有在"老元素保持原相对顺序（允许删除）+ 新元素追加在尾部"时才走增量补丁；新元素插在中间或前面（例如舰队一动、距离参考圈整圈重排）会整块重发这个数组。所以事件列表按时间正序返回：补丁只能往尾部追加。

`internal/server/patch_snapshot_test.go` 守着这条不变式：单人房和双人房各打六个回合，每次结算后断言"客户端合并出来的视图 == 重新拉一份全量快照"。

补丁是"上次推给你的投影"和"新投影"之间 diff 出来的，不是按命令手写的，所以不需要维护"这条命令改了哪些字段"。没有变化就一帧都不发——对手的不可见动作、被拒绝的命令都属于这种情况。合并发生在房间 worker 内部：一次结算里连着的几条变更只投影一次、只下发一帧，不存在时间窗，也没有版本号，补丁本身即"变了"的信号。

客户端必须守的几条：

- 收到补丁却还没有底座（例如从 HTTP 快照切过来、或本地那份已经不对了），要主动断线重连并重新 `GET state`；拿着错的底座继续合并会一直歪下去。
- 补丁按入队顺序到达（worker 串行、每条连接一个发送队列），不需要排序，但也不要跳帧。
- 补丁里的 `null` 就是业务值 nil（见上）；要删键只有数组的 `remove` 和 map 的 `{"set":{...},"remove":[...]}`。
- 每条连接的发送队列只有 32 帧。客户端读不动了（队列塞满），服务端会直接断开这条连接——这是有意的：让它重连后重拉全量，好过收一半补丁继续跑。
- 房间内不做 HTTP 轮询；断线用指数退避重连，重连成功后立刻重新拉快照。只有大厅的建房/加入仍用 HTTP。

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
