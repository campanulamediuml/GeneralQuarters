package game

import (
	"encoding/json"
	"fmt"
)

// Hex 是地图格子的标识：行*100 + 列。
//
// 它同时是坐标、是键、是索引——Row()/Col() 就是除和取余。整个运行期只讲整数：
// 比较坐标不用比字符串，查格子不用哈希四位字符，建占位索引也是 map[Hex]。
// 对外的 JSON 仍然是 "0101" 这样的四位字符串（剧本可读、git diff 稳定、前端零改动），
// 转换只发生在编解码边界上。
type Hex int

// HexOf 解析四位坐标（"0102" = 行1列2），失败返回 false。
//
// 这是热路径上的解析器：不分配、不构造 error。以前这里走 strconv.Atoi，
// 每查一次格子就要拆两段字符串再拼回去。
func HexOf(coord string) (Hex, bool) {
	if len(coord) != 4 {
		return 0, false
	}
	row0, ok := hexDigit(coord[0])
	if !ok {
		return 0, false
	}
	row1, ok := hexDigit(coord[1])
	if !ok {
		return 0, false
	}
	col0, ok := hexDigit(coord[2])
	if !ok {
		return 0, false
	}
	col1, ok := hexDigit(coord[3])
	if !ok {
		return 0, false
	}
	row := row0*10 + row1
	col := col0*10 + col1
	// 行号和列号都从 1 起：00 不是格子，它是"没有"。
	if row < 1 || col < 1 {
		return 0, false
	}
	return Hex(row*100 + col), true
}

func hexDigit(char byte) (int, bool) {
	if char < '0' || char > '9' {
		return 0, false
	}
	return int(char - '0'), true
}

func ParseHex(coord string) (Hex, error) {
	hex, ok := HexOf(coord)
	if !ok {
		return 0, fmt.Errorf("坐标必须为ABCD格式")
	}
	return hex, nil
}

// Row / Col 拆出行列号：Hex 的编码就是行在前、列在后，各两位十进制。
func (hex Hex) Row() int { return int(hex) / 100 }
func (hex Hex) Col() int { return int(hex) % 100 }

func (hex Hex) String() string { return Coord(hex.Row(), hex.Col()) }

// MarshalJSON / UnmarshalJSON 让 Hex 在 JSON 里仍然是一串四位坐标。
// 线上格式和剧本格式都不动，前端拿到的还是 "0101"。
func (hex Hex) MarshalJSON() ([]byte, error) {
	return []byte("\"" + hex.String() + "\""), nil
}

func (hex *Hex) UnmarshalJSON(data []byte) error {
	var coord string
	if err := json.Unmarshal(data, &coord); err != nil {
		return fmt.Errorf("hex坐标必须是四位字符串")
	}
	parsed, ok := HexOf(coord)
	if !ok {
		return fmt.Errorf("无效hex坐标：%s", coord)
	}
	*hex = parsed
	return nil
}
