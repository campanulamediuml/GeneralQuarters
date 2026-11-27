#!/usr/bin/env bash
# 把 素材/ 里的原始素材同步进 content/，让 units/*.json 可以直接引用素材原图。
#
#   舰船剪影：整包复制到 content/misc/image/ship/library/，保留素材原文件名
#            （JSON 里写 "misc/image/ship/library/JAP_BB1_ship.png" 这种路径）。
#   航空算子：素材包是机头朝下的俯视图，入库时旋转 180° 变成机头朝上，
#            文件名仍按机型代号（f6f.png 这种），JSON 引用 "misc/image/aircraft/f6f.png"。
#
# 用法：scripts/import-assets.sh         同步素材（会覆盖 content/ 里的同名文件）
#      scripts/import-assets.sh --check  只报告素材库里还没被引用的舰船剪影
set -euo pipefail
cd "$(dirname "$0")/.."

if [[ "${1:-}" == "--check" ]]; then
  echo "素材库未引用的舰船剪影："
  python3 - <<'PY'
import glob, os
used = set()
for path in glob.glob('content/units/**/*.json', recursive=True):
    for line in open(path, encoding='utf-8'):
        if '"picture"' in line and 'library/' in line:
            used.add(line.split('library/')[1].split('"')[0])
for f in sorted(os.listdir('content/misc/image/ship/library')):
    if f.endswith('.png') and f not in used:
        print('  ', f)
PY
  exit 0
fi

if ! command -v rsync >/dev/null || ! command -v sips >/dev/null; then
  echo "需要 rsync 和 macOS 的 sips" >&2
  exit 1
fi

mkdir -p content/misc/image/ship/library content/misc/image/aircraft
rsync -a 素材/船只剪影/正常/ content/misc/image/ship/library/

# 素材文件 → 机型代号：和 content/misc/image/aircraft/SOURCES.md 的表保持一致。
planes=(
  "f4f:F4FTop@2x.png"
  "sbd:SBDTop@2x.png"
  "tbd:TBDTop@2x.png"
  "soc:SOCTop@2x.png"
  "p40:P40Top@2x.png"
  "b25:B25Top@2x.png"
  "b26:B26Top@2x.png"
  "a6m2:A6MTop@2x.png"
  "a5m:A5MTop@2x.png"
  "d3a1:D3ATop@2x.png"
  "b5n2:B5NTop@2x.png"
  "e13a:E13ATop@2x.png"
  "h6k:H6KTop@2x.png"
  "g4m:G4MTop@2x.png"
  "b17:B17Top@2x.png"
  "pby:PBYTop@2x.png"
  "g3m:G3MTop@2x.png"
  "f6f:F6FTop@2x.png"
  "sb2c:SB2CTop@2x.png"
  "tbf:TBFTop@2x.png"
  "a6m5:A6M3Top@2x.png"
  "d4y:D4YTop@2x.png"
  "b6n:B6NTop@2x.png"
  "p1y:P1YTop@2x.png"
  "h8k:H8KTop@2x.png"
  "os2u:OS2UTop@2x.png"
  "f1m:F1M2Top@2x.png"
)
for entry in "${planes[@]}"; do
  out="${entry%%:*}"
  src="素材/飞机算子/俯视/${entry#*:}"
  if [[ ! -f "$src" ]]; then
    echo "缺少素材：$src" >&2
    exit 1
  fi
  sips -r 180 --out "content/misc/image/aircraft/$out.png" "$src" >/dev/null
done

echo "已同步：舰船剪影 $(ls content/misc/image/ship/library/*.png | wc -l | tr -d ' ') 张，航空算子 ${#planes[@]} 张"
echo "改完记得同步到运行目录：rsync -a content/ ../GeneralQuarterData/content/"
