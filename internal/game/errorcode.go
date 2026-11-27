package game

import "errors"

// CodedError 是带错误码的引擎错误。
//
// 错误文案放在 content/misc/display.json 的 errors 表里，前后端共用同一份：
// 服务端按 code + params 渲染一份文本给老客户端，同时把 code/params 一起下发，
// 前端按同一张表本地化——两边不用各维护一套报错文案。
type CodedError struct {
	Code   string
	Params map[string]string
	// Text 是引擎兜底文本：内容表里没有这个 code 时直接用它。
	Text string
}

func (e *CodedError) Error() string {
	if e == nil {
		return ""
	}
	return e.Text
}

// AsCoded 提取错误链里的 CodedError。
func AsCoded(err error) (*CodedError, bool) {
	var coded *CodedError
	if errors.As(err, &coded) && coded != nil {
		return coded, true
	}
	return nil, false
}
