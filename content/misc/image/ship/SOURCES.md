# 剪影素材来源

兵牌上的真图剪影来自识别图、线图或用户提供的照片，
经二值化 / 逐列描边 + 泛洪取轮廓处理后存为透明蒙版，构建时内联进包，运行时零网络请求。
处理工具：`silfill`（阈值 / 自适应阈值 / 红色水线切割 / 裁边 / 缩放 / 去毛刺 / 抗锯齿）。

| 文件 | 来源 | 许可 |
|---|---|---|
| shokaku.png | Wikimedia Commons: File:Shokaku-class carriers.jpg（ONI 41-42 识别卡） | Public domain（US Navy） |
| kongo.png | Wikimedia Commons: File:Kongo class battleship drawing.png | Public domain |
| north_carolina.png | Wikimedia Commons: File:North Carolina class battleship recognition drawings.jpg | Public domain |
| lexington.png | Wikimedia Commons: File:CV Saratoga silhouette.png（同级舰） | Public domain |
| yorktown.png | Wikimedia Commons: File:CV Enterprise silhouette.png（同级舰） | Public domain |
| yubari.png | Internet Archive: oni-4142《Japanese Naval Vessels》p157 页脚剪影 | Public domain |
| farragut.png | Wikimedia Commons: File:DD Farragut silhouette.png（USN 1944 识别卡，逆光照片提取） | Public domain |
| northampton.png | Shipbucket 侧视线图：USS Chicago CA-29（Navixa 绘，北安普敦级） | Shipbucket 自由使用条款（非商业，署名） |
| new_orleans.png | Shipbucket 侧视线图：USS Minneapolis CA-36，1934（Navixa 绘，新奥尔良级） | Shipbucket 自由使用条款（非商业，署名） |
| myoko.png | 用户提供的高精度侧视线图，处理成蒙版 | 用户提供 |
| leander.png | 用户提供的 HMS Leander 侧视照片，逐列描边（天际线 + 水线）生成实心剪影 | 用户提供 |
| shiratsuyu.png | 用户提供的侧视线图，处理成蒙版 | 用户提供 |
| nagato.png | 用户提供的长门侧视剪影，二值化成透明蒙版（2480×466） | 用户提供 |
| portland.png | 用户提供的波特兰级（CA-33 Portland）侧视剪影 | 用户素材 |

## 处理流程

1. `silfill`：非白像素视为前景 → 从边界泛洪标记背景 → 被舰体包围的白洞一并填实 →
   只保留最大连通域（滤掉水印、标尺、孤立噪点）→ 裁边 → 最近邻缩放到 224px → 3×3 开运算去毛刺 → 抗锯齿透明 PNG。
   船头方向沿用原图（当前整套素材都是船头朝左），不做镜像，保证同级舰之间方向一致。
2. 彩色线图：用红色舰底（水线下船体）定位水线，在红色顶部截断，只保留水线以上的干舷轮廓。
3. 低质量扫描照片（如 leander）：不用全局阈值（纸张和海水会把船体连成一片），
   改为逐列求天际线 + 拟合水线，再按列填充成实心剪影。
4. 前端用 CSS `mask-image` + `currentColor` 上色，阵营配色与损伤标记自动跟随。

## 仍需手绘的舰

无。场景内 18 艘舰全部使用上面的具名蒙版；`silhouettes-named.ts` 里的手绘造型只作为
素材缺失时的兜底（例如新加舰船还没找到图）。

## 新增素材的硬性要求

放进本目录的 PNG **必须是带 alpha 的 RGBA 蒙版**（白色不透明 = 舰体，透明 = 背景）。
CSS mask-image 默认读 alpha 通道：灰度图或照片没有 alpha，会被当成整块不透明，
兵牌上就是一个实心方块（new_orleans / furutaka 都踩过这个坑）。
灰度稿先用 alpha = luminance 转一遍再入库：

    from PIL import Image
    src = Image.open('raw.png').convert('L')
    alpha = src.point(lambda v: 0 if v <= 25 else 255 if v >= 180 else int((v - 25) * 255 / 155))
    white = Image.new('L', alpha.size, 255)
    Image.merge('RGBA', (white, white, white, alpha)).save('ship.png', optimize=True)
