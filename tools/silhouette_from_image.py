
# 识别图剪影 -> 蒙版：转灰度 -> 自动拉对比度 -> 白/非白 -> 裁边
# 用法: silhouette_from_silhouette.py 原图 输出 [阈值百分比]
import sys, numpy as np
from PIL import Image
src, out = sys.argv[1], sys.argv[2]
pct = float(sys.argv[3]) if len(sys.argv) > 3 else 96.0

im = Image.open(src).convert('L')
a = np.asarray(im).astype(np.float32)
lo, hi = np.percentile(a, 1), np.percentile(a, 99)      # 拉对比度
if hi - lo < 1: hi = lo + 1
a = np.clip((a - lo) * 255.0 / (hi - lo), 0, 255)
thr = 255.0 * pct / 100.0                                # 白/非白
ink = a < thr
ys, xs = np.nonzero(ink)
if len(ys) == 0:
    raise SystemExit('empty')
sub = ink[ys.min():ys.max()+1, xs.min():xs.max()+1]
sh, sw = sub.shape
rgba = np.zeros((sh, sw, 4), np.uint8)
rgba[:, :, 3] = (sub * 255).astype(np.uint8)
Image.fromarray(rgba, 'RGBA').save(out)
print(out, sw, sh, 'thr', round(thr, 1))
