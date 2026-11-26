// 服务端推来的是"和上一帧的差异"，这里把它合并进组件手里那份视图。
//
// 关键点：**就地改**，不重建对象。视图是 reactive 的，改哪个节点的属性，
// 就只有读那个属性的组件会重算；换成"造一棵新树再赋给 state.value"，
// 整块依赖都会失效，等于每次推送都全量重渲染一遍。
export type ViewPatch = Record<string, unknown>

// 主键候选顺序必须和 Go 侧 patchKeyFields 一致。事件 id 是数字，统一转字符串比。
const KEY_FIELDS = ['id', 'unit_id', 'task_id', 'carrier_id', 'fleet_id', 'coord']

function keyOf(item: unknown): string {
  if (!item || typeof item !== 'object') return ''
  const record = item as Record<string, unknown>
  for (const field of KEY_FIELDS) {
    const value = record[field]
    if (typeof value === 'string' && value) return value
    if (typeof value === 'number' && Number.isFinite(value)) return String(value)
  }
  return ''
}

function isCollectionPatch(patch: unknown): patch is { upsert?: Record<string, unknown>[]; remove?: string[] } {
  if (!patch || typeof patch !== 'object' || Array.isArray(patch)) return false
  return 'upsert' in (patch as Record<string, unknown>) || 'remove' in (patch as Record<string, unknown>)
}

// isMapPatch 认字符串键 map 的补丁：{"set":{变化的键},"remove":[消失的键]}。
// 只有真的删了键才会出现这个形状，平时 map 补丁还是普通对象。
function isMapPatch(patch: unknown): patch is { set?: Record<string, unknown>; remove?: string[] } {
  if (!patch || typeof patch !== 'object' || Array.isArray(patch)) return false
  const record = patch as Record<string, unknown>
  const hasSet = Boolean(record.set) && typeof record.set === 'object' && !Array.isArray(record.set)
  return hasSet || Array.isArray(record.remove)
}

// applyViewPatch 把补丁就地合进 target，返回同一个对象。
export function applyViewPatch<T>(target: T, patch: ViewPatch): T {
  merge(target, patch)
  return target
}

// mergeIntoArray 按主键把实体补丁并进数组：同一条记录只改变化了的字段，其余保持原引用。
function mergeIntoArray(target: unknown[], patch: { upsert?: Record<string, unknown>[]; remove?: string[] }): unknown[] {
  const index = new Map<string, number>()
  target.forEach((item, position) => index.set(keyOf(item), position))
  for (const item of patch.upsert || []) {
    const key = keyOf(item)
    const existing = index.get(key)
    if (existing === undefined) {
      index.set(key, target.length)
      target.push(item)
    } else {
      merge(target[existing], item)
    }
  }
  for (const key of patch.remove || []) {
    const existing = index.get(key)
    if (existing !== undefined) {
      target.splice(existing, 1)
      index.delete(key)
      for (const [other, position] of index) if (position > existing) index.set(other, position - 1)
    }
  }
  return target
}

function merge(target: unknown, patch: unknown): unknown {
  // 数组底座：实体补丁（{"upsert":[…],"remove":[…]}）按主键并进去，其余整块替换。
  // 这一步必须在"补丁要求对象底座"的检查之前：数组字段的补丁以前会被整块赋成一个对象，
  // 下一帧 players.filter / fleets.find 就直接抛错，整个页面渲染崩成空白。
  if (Array.isArray(target)) {
    if (!isCollectionPatch(patch)) return patch
    return mergeIntoArray(target, patch)
  }
  const mapPatch = isMapPatch(patch)
  // 合并不上就抛，不要静默降级成一个错误的结果：那种"不报错但慢一版"的状态
  // 比崩溃更难查——界面看起来正常，只是永远比服务端旧一点，直到下次重连。
  if (!mapPatch && isCollectionPatch(patch)) {
    throw new Error('补丁要求按主键合并，但底座不是数组：' + JSON.stringify(target)?.slice(0, 80))
  }
  if (!Array.isArray(patch) && patch !== null && typeof patch === 'object'
    && target !== undefined && target !== null && typeof target !== 'object') {
    throw new Error('补丁要求对象底座，实际是：' + JSON.stringify(target)?.slice(0, 80))
  }
  if (mapPatch && target && typeof target === 'object' && !Array.isArray(target)) {
    const targetRecord = target as Record<string, unknown>
    if (patch.set) merge(targetRecord, patch.set)
    for (const key of patch.remove || []) delete targetRecord[key]
    return target
  }
  if (patch && typeof patch === 'object' && !Array.isArray(patch) && target && typeof target === 'object' && !Array.isArray(target)) {
    for (const [key, value] of Object.entries(patch as Record<string, unknown>)) {
      const current = (target as Record<string, unknown>)[key]
      // 数组字段收到实体补丁：就地并进去，不能当成新值整块赋上去。
      if (Array.isArray(current) && isCollectionPatch(value)) {
        merge(current, value)
        continue
      }
      if (typeof value === 'object' && current && typeof current === 'object' && !Array.isArray(current)) merge(current, value)
      else (target as Record<string, unknown>)[key] = value
    }
    return target
  }
  return patch
}
