// 兵牌/机场牌上的数值来自带 doctrine 系数的浮点计算（例如 16.8×0.9 = 15.120000000000001），
// 直接插值会把二进制噪声印到牌面上，这里统一按小数位四舍五入再去掉多余的 0。
export function formatNumber(value: number | undefined | null, digits = 1) {
  if (value === undefined || value === null || !Number.isFinite(value)) return '—'
  const rounded = Number(value.toFixed(digits))
  return String(rounded)
}

// 兵牌背面：按"每 N 回合走 1 hex、1 hex = 25 海里、1 回合 = 20 分钟"把速度档位换成节。
// 回合数必须和 engine 的 FleetMoveInterval 一致（fast/normal 3、slow 6、very_slow 9），
// 否则牌面写 15 节、实际 12.5 节，玩家按牌面算航程就会差一格。
// 常速/快速 3 回合 → 25 节；慢速 6 回合 → 12.5 节；极慢 9 回合 → 8.3 取整为 8 节。
export const SPEED_MOVE_TURNS: Record<string, number> = { fast: 3, normal: 3, slow: 6, very_slow: 9 }

export function speedActionTurns(speedClass: string | undefined): number {
  return SPEED_MOVE_TURNS[speedClass ?? ''] ?? 3
}

export function speedKnots(speedClass: string | undefined): number {
  return Math.round((25 * 60) / (speedActionTurns(speedClass) * 20))
}
