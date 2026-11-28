import { GQCLIBridge } from './bridge.mjs'

export const name = 'generalquarters'
export const inject = ['tools']

const string = description => ({ type: 'string', description })
const number = description => ({ type: 'number', description })
const boolean = description => ({ type: 'boolean', description })
const strings = description => ({ type: 'array', items: { type: 'string' }, description })

export function apply(ctx, config = {}) {
  const bridges = new Map()
  const getBridge = exec => {
    const owner = exec.agent?.session?.id || exec.agent?.id || 'standalone'
    if (!bridges.has(owner)) {
      bridges.set(owner, new GQCLIBridge({ ...config, stateDir: undefined, stateKey: owner }))
    }
    return bridges.get(owner)
  }
  ctx.effect(() => async () => {
    await Promise.all([...bridges.values()].map(bridge => bridge.close()))
  })

  const register = (toolName, description, properties = {}, required = []) => {
    ctx.tools.register({
      name: toolName,
      description,
      parameters: { type: 'object', properties, required, additionalProperties: false },
      output: {
        schema: { type: 'object', additionalProperties: true },
        render: (_args, value) => [{ type: 'text', text: JSON.stringify(value, null, 2) }],
      },
      execute: (args, exec) => getBridge(exec).request(toolName, args, { signal: exec.signal }),
    })
  }

  register('gq_create', '创建一个 General Quarters 房间。只返回建房信息；创建后仍需由 A、B 两个独立 session 分别 join。', {
    scenario_id: string('剧本 ID：_coral_real 或 _marianas；省略时使用服务端默认值'),
    solo: boolean('是否单人沙盒；双模型对战必须为 false 或省略'),
  })
  register('gq_join', '使用建房 token 加入指定阵营，并建立本 session 独立的私有 WebSocket 会话。断线恢复时不要重复 join。', {
    token: string('创建房间返回的 join_token'),
    faction: { ...string('加入阵营'), enum: ['A', 'B'] },
    name: string('可选玩家名称'),
  }, ['token', 'faction'])
  register('gq_state', '读取当前阵营私有视图。默认返回紧凑摘要；按 section/id 查询详情。所有输出均已移除 join_token，不包含其他阵营的隐藏状态。', {
    section: string('summary（默认）、full、fleets、units、aviation、airfields、contacts、orders、events、map、clock、wind、scores 等；也支持 fleet/unit/air_task/airfield 单实体查询'),
    id: string('单实体 ID：fleet_id、unit_id、task_id 或 airfield ID'),
    scope: { ...string('fleets/units 的阵营过滤'), enum: ['own', 'enemy', 'all'] },
    offset: number('数组分页偏移，默认 0'),
    limit: number('数组分页大小，默认 100，最大 500'),
    refresh: boolean('读取前是否先向服务端强制拉取最新全量私有状态'),
  })
  register('gq_command', '提交一条游戏命令。CLI 自动填 base_turn、生成并持久化 command_id。accepted=true 只表示服务端接受，不代表规划已执行或通信/战斗判定成功。', {
    type: string('命令类型，如 SET_MOVE、SET_LOADOUT、LAUNCH_RECON、LAUNCH_STRIKE、RECALL_AIR'),
    unit_id: string('舰船 ID，不是 fleet_id'),
    target_unit_id: string('目标舰队中的舰船 ID'),
    coord: string('四位 hex 坐标，如 0203'),
    heading: { ...string('舰队或侦察航向'), enum: ['E', 'SE', 'SW', 'W', 'NW', 'NE'] },
    engagement: { ...string('自动航行交战策略'), enum: ['aggressive', 'advantage', 'avoid'] },
    assignments: { type: 'object', additionalProperties: { type: 'string' }, description: 'REORGANIZE_HEX 的舰船 ID -> fleet_id 完整映射' },
    split_ids: strings('SPLIT_FLEET 要拆出的舰船 ID'),
    aircraft_id: string('航空算子 ID；编队级命令按 API 使用任务 ID'),
    aircraft_ids: strings('一批航空算子 ID，1–20 个'),
    task_id: string('航空任务 ID'),
    task_kind: { ...string('无线电改令目标类型'), enum: ['attack', 'transfer'] },
    target_fleet_id: string('明确指定攻击的敌方 fleet_id'),
    sector_width: { ...number('侦察扇面角度'), enum: [60, 120] },
    lane: number('新建航空编队的栏位偏好，从 1 开始'),
    loadout: string('挂载 ID；空字符串表示卸弹'),
    disperse: boolean('true 分散攻击，false 集中攻击'),
    cycle: boolean('设置自动任务循环开关'),
  }, ['type'])
  register('gq_ready', '完成部署或结束本回合操作。战斗阶段不能撤销 ready；双方 ready 后由服务端统一结算。', {
    ready: boolean('默认 true；false 仅部署阶段可用于撤销部署完成'),
  })
  register('gq_wait', '等待本地私有快照变化。补丁仅触发全量刷新；本工具只返回变化摘要和新增战报，之后按需调用 gq_state。通常在本方 ready 后等待对手。', {
    timeout_ms: number('等待超时，默认 30000，最大 300000；超时返回 timed_out=true'),
  })
}
