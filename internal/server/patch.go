package server

import (
	"reflect"
	"sort"
	"strconv"
)

// 视图补丁：拿两份投影做 diff，算出"和上一帧相比变了哪些东西"。
//
// 为什么不按命令手写补丁：一条命令在投影里往往改好几个地方。换一次航向会同时改
// wind 的风向关系、三艘航母的 units[].deck.length、以及航空面板用的 deck_length；
// 手写"这条命令改了啥"就是给同一份状态写第二份描述，漏一处客户端就和权威对不上
// （界面写着甲板 0/5 满了，服务端却按 8 格校验）。这里直接问投影本身：
// 把新投影和上次推出去的那份比一比，差异就是补丁。
//
// 投影是带类型的 Go 值（[]game.Unit、game.Map……），不是先转成 JSON 再比：
// 转一遍要多花约 3.8ms（marshal 1.6 + unmarshal 2.1），比投影本身还贵。
// 所以这里用反射就地比较，变了的子树才 marshal 进补丁。

// patchNone 表示"这一支没有变化"。用它而不是 nil，是因为 nil 本身是一个有意义的值。
type patchNoneType struct{}

var patchNone patchNoneType

// patchKeyFields 是主键候选字段，按顺序认第一个非空的。
// 客户端（web/src/patch.ts、watch.html）必须用同一套顺序。
var patchKeyFields = []string{"id", "unit_id", "task_id", "carrier_id", "fleet_id", "coord"}

// patchKey 从一个元素里取主键（字段名 + 值）；认不出来返回空串，调用方会退回整块替换。
// 字段名要一起带出来：补丁只发变了的属性，客户端还得知道按哪个字段去对号。
func patchKey(value reflect.Value) (string, string) {
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return "", ""
		}
		value = value.Elem()
	}
	if value.Kind() == reflect.Map {
		if value.Type().Key().Kind() != reflect.String {
			return "", ""
		}
		for _, field := range patchKeyFields {
			entry := value.MapIndex(reflect.ValueOf(field))
			if !entry.IsValid() {
				continue
			}
			if text := keyText(entry); text != "" {
				return field, text
			}
		}
		return "", ""
	}
	if value.Kind() != reflect.Struct {
		return "", ""
	}
	for _, field := range patchKeyFields {
		found := value.FieldByNameFunc(func(name string) bool {
			structField, ok := value.Type().FieldByName(name)
			return ok && jsonFieldName(structField) == field
		})
		if found.IsValid() {
			if text := keyText(found); text != "" {
				return field, text
			}
		}
	}
	return "", ""
}

// keyText 把主键字段转成字符串。事件的 id 是数字，不能只认 string：
// 认不出主键就会退回"整个数组重发"，一次清命令顺手把 22 条战报全带上。
func keyText(value reflect.Value) string {
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return ""
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.String:
		return value.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(value.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(value.Uint(), 10)
	}
	return ""
}

// jsonFieldName 取字段的 JSON 名（和 encoding/json 的规则一致到我们需要的程度）。
func jsonFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	for index := 0; index < len(tag); index++ {
		if tag[index] == ',' {
			return tag[:index]
		}
	}
	return tag
}

// diffView 返回从 prev 到 next 的补丁；没有变化时返回 patchNone。
func diffView(prev, next any) any { return diffAny("", reflect.ValueOf(prev), reflect.ValueOf(next)) }

func diffAny(path string, prev, next reflect.Value) any {
	// 指针字段（比如 units[].deck）先拆开：不拆的话任何变化都整块重发，
	// 一条航向命令就会带上整个甲板对象（升降机、状态……），白多几百字节。
	for next.Kind() == reflect.Pointer && !next.IsNil() {
		if prev.Kind() != reflect.Pointer || prev.IsNil() || prev.Type() != next.Type() {
			break
		}
		prev = prev.Elem()
		next = next.Elem()
	}
	if next.Kind() == reflect.Interface {
		if next.IsNil() {
			if prev.Kind() == reflect.Interface && prev.IsNil() {
				return patchNone
			}
			return next.Interface()
		}
		// 两侧都可能是 interface{}（map[string]any、[]any 的元素），要一起拆开比，
		// 否则会把整个数组当成"类型不同"整块重发。
		if prev.Kind() == reflect.Interface && !prev.IsNil() {
			prev = prev.Elem()
		}
		return diffAny(path, prev, next.Elem())
	}
	if prev.Kind() == reflect.Interface {
		if prev.IsNil() {
			return next.Interface()
		}
		prev = prev.Elem()
	}
	switch next.Kind() {
	case reflect.Map:
		if prev.Kind() != reflect.Map || prev.Type() != next.Type() {
			return next.Interface()
		}
		return diffMap(path, prev, next)
	case reflect.Slice, reflect.Array:
		if !(prev.Kind() == next.Kind() && prev.Type() == next.Type()) {
			return next.Interface()
		}
		return diffSlice(path, prev, next)
	case reflect.Struct:
		if prev.Kind() != reflect.Struct || prev.Type() != next.Type() {
			return next.Interface()
		}
		if prev.NumField() == 0 {
			return patchNone
		}
		patch := map[string]any{}
		for index := 0; index < next.NumField(); index++ {
			field := next.Type().Field(index)
			if field.PkgPath != "" {
				continue // 未导出字段 encoding/json 也不会带出去
			}
			name, skip := structFieldJSONName(field)
			if skip {
				continue
			}
			change := diffAny(joinPath(path, name), prev.Field(index), next.Field(index))
			if change != any(patchNone) {
				patch[name] = change
			}
		}
		if len(patch) == 0 {
			return patchNone
		}
		return patch
	default:
		if valuesEqual(prev, next) {
			return patchNone
		}
		return next.Interface()
	}
}

// structFieldJSONName 返回结构体字段在 JSON 里的名字；skip 表示这个字段不参与比较和下发。
func structFieldJSONName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", true
	}
	name := tag
	if index := indexComma(tag); index >= 0 {
		name = tag[:index]
	}
	if name == "" {
		return field.Name, false
	}
	return name, false
}

func indexComma(tag string) int {
	for index := 0; index < len(tag); index++ {
		if tag[index] == ',' {
			return index
		}
	}
	return -1
}

func diffMap(path string, prev, next reflect.Value) any {
	if next.Type().Key().Kind() != reflect.String {
		if valuesEqual(prev, next) {
			return patchNone
		}
		return next.Interface()
	}
	patch := make(map[string]any, next.Len())
	for _, key := range next.MapKeys() {
		name := key.String()
		before := prev.MapIndex(key)
		if !before.IsValid() {
			patch[name] = next.MapIndex(key).Interface()
			continue
		}
		change := diffAny(joinPath(path, name), before, next.MapIndex(key))
		if change != any(patchNone) {
			patch[name] = change
		}
	}
	for _, key := range prev.MapKeys() {
		if !next.MapIndex(key).IsValid() {
			// 视图的键来自结构体投影，正常不会消失。真丢了就整块重发，别留删不掉的旧键。
			return next.Interface()
		}
	}
	if len(patch) == 0 {
		return patchNone
	}
	return patch
}

// isEntityList 判断一个数组是不是"实体列表"：每个元素都能认出主键，而且主键不重复。
// 认出主键就按 id 合并（只发变了的属性），认不出就整块替换。
//
// 不按路径写死：载舰的飞机、舰队的成员都是嵌在实体里的数组，路径是动态的，
// 写死一份清单迟早漏；这里按元素本身判断，嵌套多深都一样。
func isEntityList(list reflect.Value) bool {
	if list.Len() == 0 {
		return false
	}
	seen := make(map[string]bool, list.Len())
	for index := 0; index < list.Len(); index++ {
		_, key := patchKey(list.Index(index))
		if key == "" || seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

func diffSlice(path string, prev, next reflect.Value) any {
	if !isEntityList(prev) || !isEntityList(next) {
		if valuesEqual(prev, next) {
			return patchNone
		}
		return next.Interface()
	}
	before := make(map[string]reflect.Value, prev.Len())
	for index := 0; index < prev.Len(); index++ {
		item := prev.Index(index)
		_, key := patchKey(item)
		before[key] = item
	}
	upsert := make([]any, 0, next.Len())
	seen := make(map[string]bool, next.Len())
	for index := 0; index < next.Len(); index++ {
		item := next.Index(index)
		keyField, key := patchKey(item)
		seen[key] = true
		old, exists := before[key]
		if !exists {
			// 新出现的元素：整条给，客户端没有底子可合并。
			upsert = append(upsert, item.Interface())
			continue
		}
		change := diffAny("", old, item)
		if change == any(patchNone) {
			continue
		}
		// 只发变了的属性，补上主键让客户端知道改的是谁。换一次航向在投影里
		// 改的是散落在 wind、units[].deck、aviation.carriers[] 里的几个字段，
		// 这样一条补丁只有几十字节，而不是整条记录重发。
		if partial, ok := change.(map[string]any); ok {
			partial[keyField] = key
			upsert = append(upsert, partial)
			continue
		}
		upsert = append(upsert, item.Interface())
	}
	remove := make([]string, 0)
	for key := range before {
		if !seen[key] {
			remove = append(remove, key)
		}
	}
	if len(upsert) == 0 && len(remove) == 0 {
		return patchNone
	}
	patch := map[string]any{}
	if len(upsert) > 0 {
		patch["upsert"] = upsert
	}
	if len(remove) > 0 {
		// 排序只为输出稳定，方便日志和测试对比。
		sort.Strings(remove)
		patch["remove"] = remove
	}
	return patch
}
func valuesEqual(left, right reflect.Value) bool {
	if !left.IsValid() || !right.IsValid() {
		return left.IsValid() == right.IsValid()
	}
	if left.Type() != right.Type() {
		return false
	}
	switch left.Kind() {
	case reflect.Pointer, reflect.Interface:
		if left.IsNil() || right.IsNil() {
			return left.IsNil() && right.IsNil()
		}
		return valuesEqual(left.Elem(), right.Elem())
	}
	return reflect.DeepEqual(left.Interface(), right.Interface())
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}
