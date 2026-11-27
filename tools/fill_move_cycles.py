# 按现实最高航速给每张舰船卡填 move_cycle。
# 换算：25 海里/hex、20 分钟/回合 → 平均每格 N 回合 = 75/N 节。
# 周期 = 平均回合数最接近 N 的整数序列（长度 ≤ 6），牌面结数向下取整。
import json, math, re, glob, collections, os

# 脚本按仓库根目录解析内容路径，从哪调用都一样。
BASE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
os.chdir(BASE)

# 现实最高航速（节）。没列到名字的走 GENERIC / 类型兜底。
KNOTS = {
    # 战列舰
    "北卡罗来纳": 28, "华盛顿": 28, "南达科他": 27.8, "印第安纳": 27.8, "马萨诸塞": 27.8,
    "阿拉巴马": 27.8, "大和": 27.5, "武藏": 27.5, "长门": 26.5, "陆奥": 26.5,
    "金刚": 30.5, "榛名": 30.5, "比叡": 30.5, "雾岛": 30.5, "新泽西": 33, "依阿华": 33,
    "扶桑": 24.7, "山城": 24.7, "伊势": 25.3, "日向": 25.3,
    "田纳西": 21, "加利福尼亚": 21, "马里兰": 21, "科罗拉多": 21,
    "新墨西哥": 22, "爱达荷": 22, "密西西比": 22, "宾夕法尼亚": 21,
    # 重巡
    "妙高": 35.5, "羽黑": 35.5, "高雄": 35.5, "爱宕": 35.5, "摩耶": 35.5, "鸟海": 35.5,
    "利根": 35, "筑摩": 35, "那智": 35.5, "足柄": 35.5, "铃谷": 35, "熊野": 35, "最上": 37,
    "巴尔的摩": 33, "威奇塔": 33, "旧金山": 32.7, "昆西": 32.7, "波士顿": 33,
    "芝加哥": 32.7, "休斯顿": 32.7,
    "青叶": 34.5, "加古": 34.5, "衣笠": 34.5, "古鹰": 34.5,
    "阿斯托里亚": 32.7, "明尼阿波利斯": 32.7, "新奥尔良": 32.7, "波特兰": 32.7,
    "切斯特": 32.5, "澳大利亚": 31.5,
    # 轻巡
    "矢矧": 35, "大淀": 35.5, "能代": 35, "阿贺野": 35, "神通": 35.3, "川内": 35.3, "夕张": 34.8,
    "克利夫兰": 32.5, "蒙彼利埃": 32.5, "丹佛": 32.5, "圣菲": 32.5, "伯明翰": 32.5, "莫比尔": 32.5,
    "霍巴特": 32.5,
    # 航母
    "埃塞克斯": 33, "邦克山": 33, "大黄蜂": 33, "约克城": 33, "黄蜂": 33, "富兰克林": 33,
    "企业": 32.5, "汉考克": 33, "列克星敦": 33.25, "大凤": 33.3, "翔鹤": 34.2, "瑞鹤": 34.2,
    "隼鹰": 25.5, "飞鹰": 25.5, "龙凤": 26.5, "云龙": 34, "天城": 34, "葛城": 34,
    "信浓": 27, "赤城": 31.2, "加贺": 28.3, "飞龙": 34.3, "苍龙": 34.9, "祥凤": 28.2,
    # 护航航母：卡萨布兰卡级 19、桑加蒙级 18.5、博格级（布雷顿/科波西）18
    "马尼拉湾": 19, "纳托马湾": 19, "珊瑚海": 19, "甘比尔湾": 19,
    "切南戈": 18.5, "白平原": 19, "布雷顿": 18, "科波西": 18,
    "苏瓦尼": 18.5, "圣洛": 19, "基特昆湾": 19, "科雷希多": 19,
    "桑加蒙": 18.5, "方肖湾": 19, "加里宁湾": 19, "瓜达尔卡纳尔": 19,
    # 轻航母
    "独立": 31.5, "普林斯顿": 31.5, "贝劳伍德": 31.5, "考彭斯": 31.5,
    "千岁": 28.9, "千代田": 28.9, "瑞凤": 28.2,
    # 珊瑚海的驱逐舰
    "安德森": 37, "有明": 37.3, "杜威": 36.5, "艾尔文": 36.8, "法拉格特": 36.5, "哈曼": 36.5,
    "莫纳汉": 36.5, "莫里斯": 36.5, "睦月": 37.3, "帕金斯": 36.5, "菲尔普斯": 37, "拉塞尔": 37,
    "涟": 37.3, "时雨": 34, "白露": 34, "瓦尔克": 36.5, "惠普尔": 36.5, "夕暮": 37.3,
    # 运输
    "运输船": 14,
}

FALLBACK = [
    (r"^弗莱彻号\d+$", 36.5),
    (r"^驱逐舰\d+$", 36.5),
    (r"^护航驱逐舰\d+$", 24),
    (r"^大型运输船\d+$", 14),
]

TYPE_DEFAULT = {"BB": 25, "CA": 32, "CL": 32, "CV": 30, "CVL": 30, "CVE": 19, "DD": 35, "TRS": 14}


def knots_for(name, ship_type):
    if name in KNOTS:
        return KNOTS[name]
    for pattern, knots in FALLBACK:
        if re.match(pattern, name):
            return knots
    return TYPE_DEFAULT.get(ship_type, 25)


def cycle_for(knots):
    """挑一串周期，让平均每格回合数最接近 75/目标航速；同样接近时挑短的周期。

    按回合数贴近而不是按"向下取整后的结数"，否则 12 节会被 [6]（实际 12.5 节）顶掉，
    而 6.25 回合的 6、6、6、7 才是正好 12 节。牌面结数只在算不出来的时候向下取整。
    """
    target = int(math.floor(knots))
    n = 75.0 / knots
    base = int(math.floor(n))
    best = None
    for length in range(1, 9):
        for extra in range(length + 1):  # 有 extra 个 base+1
            avg = (base * length + extra) / length
            model = 75.0 / avg
            # 先保证牌面结数（向下取整）对得上，再比真实结数误差，最后才比周期长短。
            score = (abs(int(math.floor(model)) - target), round(abs(model - knots), 6), length)
            if best is None or score < best[0]:
                best = (score, build_cycle(base, length, extra))
    return best[1]


def build_cycle(base, length, extra):
    """把 extra 个 base+1 均匀撒进 length 个位置（Bresenham），避免走走停停成块。"""
    if extra == 0:
        return [base]
    if extra >= length:
        return [base + 1]
    out = []
    for i in range(length):
        # 第 i 个位置放 base+1 的个数
        before = (i * extra) // length
        after = ((i + 1) * extra) // length
        out.append(base + 1 if after > before else base)
    return out


def model_knots(cycle):
    return int(math.floor(75.0 / (sum(cycle) / len(cycle))))


report = []


def apply(path_json, get_units):
    data = json.load(open(path_json))
    units = get_units(data)
    for unit in units:
        card = unit.get("card") or {}
        name = unit.get("name") or ""
        knots = knots_for(name, unit.get("type"))
        cycle = cycle_for(knots)
        card["move_cycle"] = cycle
        unit["card"] = card
        report.append((name, unit.get("type"), knots, cycle, model_knots(cycle)))
    text = json.dumps(data, ensure_ascii=False, indent=1)
    # 周期数组一行写完，否则每张卡多出六七行，diff 全是噪声。
    text = re.sub(
        r'"move_cycle": \[([^\]]*)\]',
        lambda m: '"move_cycle": [' + ", ".join(part.strip() for part in m.group(1).split(",")) + "]",
        text,
    )
    with open(path_json, "w") as fh:
        fh.write(text + "\n")


apply("content/units/marianas.json", lambda d: d["units"])
for path in sorted(glob.glob("content/units/ships/*.json")):
    apply(path, lambda d: [d])

seen = {}
for name, ship_type, knots, cycle, mk in report:
    seen.setdefault((ship_type, tuple(cycle)), set()).add((name, knots, mk))

print("周期数与误差：")
bad = 0
for name, ship_type, knots, cycle, mk in report:
    if abs(mk - knots) > 1.5:
        bad += 1
        print("  偏差>1.5节: %-10s %-4s 现实%.1f → %v = %d" % (name, ship_type, knots, cycle, mk))
print("  超过 1.5 节偏差的：%d / %d" % (bad, len(report)))

print("\n按类型分组：")
groups = collections.defaultdict(list)
for name, ship_type, knots, cycle, mk in report:
    groups[ship_type].append((knots, cycle, mk, name))
for ship_type in sorted(groups):
    rows = groups[ship_type]
    spread = collections.defaultdict(list)
    for knots, cycle, mk, name in rows:
        spread[(tuple(cycle), mk)].append(name)
    print(" %s（%d 艘）" % (ship_type, len(rows)))
    for (cycle, mk), names in sorted(spread.items(), key=lambda kv: -kv[0][1]):
        print("    %-18s %2d 节  例: %s" % ("/".join(map(str, cycle)), mk, names[0]))
