// 兵牌剪影：素材在 content/misc/image/ship/ 下，由舰船卡的 picture 字段指路
// （例如 "misc/image/ship/new_orleans.png"），服务端按 /content/... 直接静态下发。
// 这样换图、加船都不需要重新构建前端包；老存档里的卡没有 picture，用下面这张
// 按舰名的兼容表兜底（只存路径字符串，不再把 PNG 打进 JS bundle）。

export type PictureCard = { picture?: string } | undefined | null

const LEGACY_BY_NAME: Record<string, string> = {
  赤城: 'misc/image/ship/akagi.png',
  加贺: 'misc/image/ship/kaga.png',
  瑞鹤: 'misc/image/ship/shokaku.png',
  翔鹤: 'misc/image/ship/shokaku.png',
  金刚: 'misc/image/ship/kongo.png',
  比叡: 'misc/image/ship/hiei.png',
  榛名: 'misc/image/ship/haruna.png',
  雾岛: 'misc/image/ship/kirishima.png',
  北卡罗来纳: 'misc/image/ship/north_carolina.png',
  华盛顿: 'misc/image/ship/north_carolina.png',
  约克城: 'misc/image/ship/yorktown.png',
  企业: 'misc/image/ship/yorktown.png',
  大黄蜂: 'misc/image/ship/yorktown.png',
  列克星敦: 'misc/image/ship/lexington.png',
  萨拉托加: 'misc/image/ship/lexington.png',
  夕张: 'misc/image/ship/yubari.png',
  妙高: 'misc/image/ship/myoko.png',
  羽黑: 'misc/image/ship/myoko.png',
  青叶: 'misc/image/ship/furutaka.png',
  加古: 'misc/image/ship/furutaka.png',
  古鹰: 'misc/image/ship/furutaka.png',
  衣笠: 'misc/image/ship/furutaka.png',
  长门: 'misc/image/ship/nagato.png',
  陆奥: 'misc/image/ship/nagato.png',
  扶桑: 'misc/image/ship/fuso.png',
  山城: 'misc/image/ship/fuso.png',
  伊势: 'misc/image/ship/ise.png',
  日向: 'misc/image/ship/ise.png',
  信浓: 'misc/image/ship/shinano.png',
  苍龙: 'misc/image/ship/soryu.png',
  飞龙: 'misc/image/ship/hiryu.png',
  最上: 'misc/image/ship/mogami.png',
  铃谷: 'misc/image/ship/mogami.png',
  熊野: 'misc/image/ship/mogami.png',
  芝加哥: 'misc/image/ship/northampton.png',
  阿斯托里亚: 'misc/image/ship/northampton.png',
  切斯特: 'misc/image/ship/northampton.png',
  明尼阿波利斯: 'misc/image/ship/new_orleans.png',
  新奥尔良: 'misc/image/ship/new_orleans.png',
  波特兰: 'misc/image/ship/portland.png',
  利安德: 'misc/image/ship/leander.png',
  法拉格特: 'misc/image/ship/farragut.png',
  莫里斯: 'misc/image/ship/farragut.png',
  安德森: 'misc/image/ship/farragut.png',
  春雨: 'misc/image/ship/shiratsuyu.png',
  时雨: 'misc/image/ship/shiratsuyu.png',
  白露: 'misc/image/ship/shiratsuyu.png',
  有明: 'misc/image/ship/shiratsuyu.png',
  夕暮: 'misc/image/ship/shiratsuyu.png',
}

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

export function shipMaskURL(picture?: string, name?: string): string | undefined {
  const path = (picture || '').trim() || LEGACY_BY_NAME[(name || '').trim()]
  return contentImageURL(path)
}

// 航空算子的 picture 来自 units/*.json 的 aircraft[].picture；默认是彩图，
// picture_mode=mask 时才当单色剪影用。没有配置或图片加载失败时回退到内置矢量剪影。
export function aircraftPictureURL(picture?: string): string | undefined {
  return contentImageURL(picture)
}

export function shipMask(name?: string): string | undefined {
  return shipMaskURL('', name)
}
