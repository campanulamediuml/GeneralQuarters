// 兵牌/机场牌上的数值来自带 doctrine 系数的浮点计算（例如 16.8×0.9 = 15.120000000000001），
// 直接插值会把二进制噪声印到牌面上，这里统一按小数位四舍五入再去掉多余的 0。
export function formatNumber(value: number | undefined | null, digits = 1) {
  if (value === undefined || value === null || !Number.isFinite(value)) return '—'
  const rounded = Number(value.toFixed(digits))
  return String(rounded)
}
