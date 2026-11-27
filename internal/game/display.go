package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Display 是给界面用的展示字典：枚举值 → 文案。
//
// 服务端下发的状态（舰种、机型、状态、损伤、情报等级……）都有枚举值，
// 但"每个值显示成什么字"不能写死在客户端：文案表放在 content/misc/display.json，
// 由服务端随视图一起下发，前端只按枚举查表渲染。
// 没配这个文件时下发空表，界面退回显示枚举 id（不崩，也不猜）。
type Display struct {
	Version int `json:"version"`
	// Labels 是"分组 → 枚举值 → 文案"。分组名由前后端约定（aircraft_kind、ship_type…），
	// 结构保持 map[string]any 原样透传：文案是内容，服务端不需要理解每一条。
	Labels map[string]any `json:"labels"`
	// Errors 是错误码 → 文案模板（{key} 占位符），前后端共用同一份：
	// 服务端渲染一份文本随错误下发，前端拿到 error_code 也能用同一张表本地化，
	// 两边不用各维护一套报错文案。
	Errors map[string]string `json:"errors"`
}

// RenderError 按错误码渲染文案；没有这个错误码时返回 false，调用方退回引擎自己的文本。
func (d Display) RenderError(code string, params map[string]string) (string, bool) {
	template, ok := d.Errors[code]
	if !ok {
		return "", false
	}
	text := template
	for key, value := range params {
		text = strings.ReplaceAll(text, "{"+key+"}", value)
	}
	return text, true
}

// LoadDisplay 读取 content/misc/display.json。
// 文件不存在不算错误（内容目录可以还不配文案表）；解析失败要报错，
// 否则前端会静默退回枚举 id，排查起来看不出是内容写坏了。
func LoadDisplay(contentDir string) (Display, error) {
	path := filepath.Join(contentDir, "misc", "display.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Display{}, nil
		}
		return Display{}, err
	}
	var display Display
	if err := json.Unmarshal(data, &display); err != nil {
		return Display{}, fmt.Errorf("解析%s失败：%w", filepath.Base(path), err)
	}
	return display, nil
}
