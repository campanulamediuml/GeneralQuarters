// 展示文案字典：枚举 → 文案由服务端下发（content/misc/display.json）。
// 前端只按状态查表渲染，不在代码里写死舰种、机型、损伤、情报等级这些映射。
export interface DisplayDict {
  version?: number
  /** 分组 → 枚举值 → 文案。分组名与后端约定（ship_type、aircraft_kind…）。 */
  labels?: Record<string, unknown>
  /** 错误码 → 文案模板（{key} 占位符）：服务端和前端共用同一份。 */
  errors?: Record<string, string>
}

let dict: DisplayDict = {}

/** 每次收到视图时调用：把服务端那份文案表装进来。 */
export function applyDisplay(display?: DisplayDict) {
  dict = display && display.labels ? display : {}
}

/** 按错误码渲染文案：前端和服务端读同一张 errors 表，不用各自维护报错文案。 */
export function messageFor(code?: string, params?: Record<string, string>, fallback = ''): string {
  if (!code) return fallback
  const table = dict.errors
  const template = table?.[code]
  if (!template) return fallback || code
  return template.replace(/\{(\w+)\}/g, (_, key: string) => params?.[key] ?? '{' + key + '}')
}

/** 查一条文案；查不到时退回枚举 id，保证界面不空白、也不瞎猜。 */
export function label(group: string, key?: string): string {
  if (!key) return ''
  const table = dict.labels?.[group]
  if (!table || typeof table !== 'object') return key
  const value = (table as Record<string, unknown>)[key]
  return typeof value === 'string' ? value : key
}

/** 查"枚举 → { label, hint }"这类嵌套条目里的一个字段。 */
export function nestedLabel(group: string, key: string | undefined, field: string): string {
  if (!key) return ''
  const table = dict.labels?.[group]
  if (!table || typeof table !== 'object') return ''
  const entry = (table as Record<string, unknown>)[key]
  if (!entry || typeof entry !== 'object') return ''
  const value = (entry as Record<string, unknown>)[field]
  return typeof value === 'string' ? value : ''
}
