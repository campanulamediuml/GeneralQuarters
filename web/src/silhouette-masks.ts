// 素材路径解析：服务端下发的 picture 是 content/ 下的相对路径
// （例如 "misc/image/ship/new_orleans.png"），这里只负责拼成 /content/... 的站内地址。
// 前端不保存任何"舰名 → 图片文件"之类的映射表：缺图就是内容目录缺图，由后端补。

export type PictureCard = { picture?: string } | undefined | null

// 素材库里的原始文件名带空格（JAP_CV1 Akagi_ship.png 这类），URL 里必须逐段转义，
// 否则 CSS url() 和 <img src> 都会截断到空格处。只转义本机路径，外链原样返回（可能自带 query）。
function encodeContentPath(path: string): string {
  return path.split('/').map((segment) => encodeURIComponent(segment)).join('/')
}

// picture 既有可能是 content 相对路径，也可能已经是站内绝对 URL。
// 舰船和航空算子共用同一套解析规则：misc/image/... → /content/misc/image/...。
export function contentImageURL(picture?: string): string | undefined {
  const path = (picture || '').trim()
  if (!path) return undefined
  if (path.startsWith('http://') || path.startsWith('https://')) return path
  if (path.startsWith('/')) return encodeContentPath(path)
  return '/content/' + encodeContentPath(path)
}

// 舰船兵牌剪影：卡上的 picture 直接当 mask 用，没有就什么都不画。
export function shipMaskURL(picture?: string): string | undefined {
  return contentImageURL(picture)
}

// 航空算子的 picture 来自 units/*.json 的 aircraft[].picture；默认是彩图，
// picture_mode=mask 时才当单色剪影用。
export function aircraftPictureURL(picture?: string): string | undefined {
  return contentImageURL(picture)
}
