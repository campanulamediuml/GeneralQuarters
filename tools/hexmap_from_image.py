#!/usr/bin/env python3
"""从一张真实海图生成 hex 地图。

思路：图上白色是陆地、蓝色是海；用两个带同心圆的城市标记点定标出
像素->经纬度的 Web Mercator 常数，再把 25 海里/hex 的等距六边形网格
铺回图上，逐格统计六边形内的陆地占比。

用法：
  python3 tools/hexmap_from_image.py --image 素材/coral.webp --rows 52 --cols 64 \
      --out content/maps/_coral_real/map.json

依赖 numpy + Pillow（DSH 自带运行时有）。
"""
import argparse, json, math, os

import numpy as np
from PIL import Image, ImageDraw, ImageFont

# 目标图上的定标点：pixel -> lat/lon（Web Mercator）
GEOREF = dict(x0=374.5, y0=525.8, lon0=147.1803, lat0=-9.4438, k=89.0925)

# 要在地图上命名的地点（lat, lon）。港口格一律按陆地处理：25 海里一格
# 分辨不出半岛和小岛，否则拉包尔/图拉吉会变成“海里的机场”。
PLACES = [
    ("拉包尔", -4.199, 152.168),
    ("莫尔兹比港", -9.4438, 147.1803),
    ("图拉吉", -9.104, 160.149),
    ("凯恩斯", -16.9203, 145.7710),
    ("苦难角", -16.0833, 145.4667),
    ("欣钦布鲁克岛", -18.3500, 146.2500),
    ("胡克岛", -20.0700, 148.9200),
    ("马凯", -21.1411, 149.1861),
    ("维拉港", -17.7333, 168.3273),
    ("努美阿", -22.2758, 166.4580),
]

LAND_THRESHOLD = 0.50      # 六边形内陆地占比达到这个值算陆地
COAST_THRESHOLD = 0.25     # 与陆地相邻、占比超过这个值算浅滩（舰载队进不去）


def merc(lat):
    return math.degrees(math.log(math.tan(math.radians(45 + lat / 2.0))))


def ll_to_px(lon, lat):
    g = GEOREF
    return (g["x0"] + (lon - g["lon0"]) * g["k"],
            g["y0"] - (merc(lat) - merc(g["lat0"])) * g["k"])


def build_mask(img):
    a = np.asarray(img.convert("RGB")).astype(np.int16)
    r, g, b = a[:, :, 0], a[:, :, 1], a[:, :, 2]
    sea = (b - r > 20) & (b > 150)
    land = (r > 205) & (g > 205) & (b > 205)
    # 剩下的（文字、道路、图标、虚线）既不算海也不算陆，不参与占比统计
    return land, sea


CORN = [(0, -2 / 3), (0.5, -1 / 3), (0.5, 1 / 3), (0, 2 / 3), (-0.5, 1 / 3), (-0.5, -1 / 3)]


def cell_center(geom, row, col):
    colpx, rowpx, x0, y0 = geom
    xf = (col - 1) - (0.5 if (row - 1) % 2 == 1 else 0.0)
    return x0 + xf * colpx, y0 + (row - 1) * rowpx


def neighbors(row, col):
    deltas = [(0, -1), (0, 1), (-1, 0), (1, 0)]
    if row % 2 == 0:
        deltas += [(-1, -1), (1, -1)]
    else:
        deltas += [(-1, 1), (1, 1)]
    return [(row + dr, col + dc) for dr, dc in deltas]


def sample(grid_geom, land, sea, row, col, ns=11):
    colpx, rowpx, x0, y0 = grid_geom
    cx, cy = cell_center(grid_geom, row, col)
    h, w = land.shape
    nl = ns_ = 0
    for i in range(ns):
        for j in range(ns):
            u = -0.5 + (i + 0.5) / ns
            v = -2 / 3 + (j + 0.5) * (4 / 3) / ns
            a, b = abs(u), abs(v)
            if not (a <= 0.5 and (b <= 1 / 3 or (b - 1 / 3) * 3 <= (0.5 - a) * 2)):
                continue
            x = int(round(cx + u * colpx))
            y = int(round(cy + v * rowpx))
            if 0 <= x < w and 0 <= y < h:
                if land[y, x]:
                    nl += 1
                elif sea[y, x]:
                    ns_ += 1
    if nl + ns_ == 0:
        return None
    return nl / (nl + ns_)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--image", required=True)
    ap.add_argument("--rows", type=int, default=52)
    ap.add_argument("--cols", type=int, default=64)
    ap.add_argument("--out", required=True)
    ap.add_argument("--map-id", default=None)
    ap.add_argument("--name", default=None)
    ap.add_argument("--preview", default=None)
    args = ap.parse_args()

    img = Image.open(args.image)
    land, sea = build_mask(img)
    w, h = img.size
    colpx = w / args.cols
    rowpx = h / args.rows
    geom = (colpx, rowpx, colpx / 2, rowpx / 2)

    frac = {}
    for r in range(1, args.rows + 1):
        for c in range(1, args.cols + 1):
            frac[(r, c)] = sample(geom, land, sea, r, c)

    terrain = {}
    for (r, c), f in frac.items():
        if f is not None and f >= LAND_THRESHOLD:
            terrain[(r, c)] = "island"
    for (r, c), f in frac.items():
        if (r, c) in terrain or f is None or f < COAST_THRESHOLD:
            continue
        if any(terrain.get(n) == "island" for n in neighbors(r, c)):
            terrain[(r, c)] = "shoal"

    # 港口：最近的格子强制成陆地
    names = {}
    place_cells = {}
    for name, lat, lon in PLACES:
        px, py = ll_to_px(lon, lat)
        best = None
        for r in range(1, args.rows + 1):
            for c in range(1, args.cols + 1):
                cx, cy = cell_center(geom, r, c)
                d = (cx - px) ** 2 + (cy - py) ** 2
                if best is None or d < best[0]:
                    best = (d, r, c)
        _, r, c = best
        terrain[(r, c)] = "island"
        names[(r, c)] = name
        place_cells[name] = "%02d%02d" % (r, c)

    cells = [{"coord": "%02d%02d" % (r, c), "terrain": t, "name": names.get((r, c), "")}
             for (r, c), t in sorted(terrain.items())]
    doc = {
        "id": args.map_id or os.path.basename(os.path.dirname(os.path.abspath(args.out))),
        "version": 2,
        "name": args.name or "珊瑚海 · 真实比例（%d×%d，25海里/hex）" % (args.cols, args.rows),
        "rows": args.rows,
        "cols": args.cols,
        "cells": cells,
    }
    os.makedirs(os.path.dirname(os.path.abspath(args.out)), exist_ok=True)
    with open(args.out, "w") as fh:
        json.dump(doc, fh, ensure_ascii=False, indent=1)

    print("grid %dx%d  col=%.2fpx(%.1fnm) row=%.2fpx(%.1fnm)" % (
        args.rows, args.cols, colpx, colpx / GEOREF["k"] * 60, rowpx, rowpx / GEOREF["k"] * 60))
    print("cells: %d  (island %d, shoal %d)" % (
        len(cells), sum(1 for v in terrain.values() if v == "island"),
        sum(1 for v in terrain.values() if v == "shoal")))
    for name, coord in place_cells.items():
        print("  %-8s %s" % (name, coord))
    # 地点->hex 的对应关系直接打到 stdout，剧本重映射脚本照着用即可，
    # 不往 content/ 里写中间文件（那个目录会整体编进二进制）。
    print(json.dumps(place_cells, ensure_ascii=False))

    if args.preview:
        render(img, geom, args.rows, args.cols, terrain, names, place_cells, args.preview)


def render(img, geom, rows, cols, terrain, names, place_cells, path):
    colpx, rowpx, x0, y0 = geom
    out = Image.new("RGB", (int(cols * colpx), int(rows * rowpx)), (178, 209, 236))
    d = ImageDraw.Draw(out)
    for r in range(1, rows + 1):
        for c in range(1, cols + 1):
            cx, cy = cell_center(geom, r, c)
            pts = [(cx + u * colpx, cy + v * rowpx) for u, v in CORN]
            t = terrain.get((r, c))
            if t == "island":
                d.polygon(pts, fill=(214, 205, 178), outline=(120, 115, 95))
            elif t == "shoal":
                d.polygon(pts, fill=(196, 210, 200), outline=(150, 160, 150))
            else:
                d.polygon(pts, outline=(150, 175, 200))
    try:
        fnt = ImageFont.truetype("/System/Library/Fonts/Supplemental/Songti.ttc", 18)
    except Exception:
        fnt = ImageFont.load_default()
    for name, coord in place_cells.items():
        r, c = int(coord[:2]), int(coord[2:])
        cx, cy = cell_center(geom, r, c)
        d.ellipse([cx - 6, cy - 6, cx + 6, cy + 6], fill=(190, 60, 40))
        d.text((cx + 8, cy - 8), "%s %s" % (name, coord), fill=(20, 20, 20), font=fnt)
    out.save(path)
    print("preview ->", path)


if __name__ == "__main__":
    main()
