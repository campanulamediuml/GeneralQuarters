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
// 客户端（web/src/patch.ts）必须用同一套顺序。
var patchKeyFields = []string{"id", "unit_id", "task_id", "carrier_id", "fleet_id", "coord"}

// patchKey 从一个元素里取主键（字段名 + 值）；认不出来返回空串，调用方会退回整块替换。
// 字段名要一起带出来：补丁只发变了的属性，客户端还得知道按哪个字段去对号。
func patchKey(cache diffCache, value reflect.Value) (string, string) {
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
	for _, key := range cache.meta(value.Type()).keys {
		found, ok := fieldByPath(value, key.path)
		if !ok {
			continue
		}
		if text := keyText(found); text != "" {
			return key.name, text
		}
	}
	return "", ""
}

// fieldByPath 按嵌入路径取字段；中间的嵌入指针是 nil 就当成没这个字段。
// FieldByIndex 遇到 nil 会直接 panic，而视图里"嵌入指针为空"是正常状态。
func fieldByPath(value reflect.Value, path []int) (reflect.Value, bool) {
	for _, index := range path {
		for value.Kind() == reflect.Pointer {
			if value.IsNil() {
				return reflect.Value{}, false
			}
			value = value.Elem()
		}
		if value.Kind() != reflect.Struct {
			return reflect.Value{}, false
		}
		value = value.Field(index)
	}
	return value, true
}

// structMeta 是一个结构体类型在补丁里的投影：哪些字段参与比较、JSON 名叫什么，
// 以及 patchKeyFields 里这个类型真正有哪几个主键候选。
//
// 这些都只和类型有关，算一次就够。不缓存的话，视图里每个实体每帧都要解一遍 tag、
// 跑一遍 FieldByNameFunc——438 格地图、几十艘舰、上百架飞机，一次推送 11ms 里
// 45% 就烧在这上面（而补丁本身只有几百字节）。
type structMeta struct {
	fields []structField
	keys   []structKeyField
}

type structField struct {
	index int
	name  string
}

type structKeyField struct {
	path []int // 用路径是为了支持嵌入字段（FieldByIndex 能吃多级）
	name string
}

// diffCache 是 diff 用的类型元数据缓存。
//
// 它跟着房间 worker 走：每个房间一份，只在自己的 goroutine 里读写，
// 所以不需要锁，也不需要 sync.Map。类型只有几十个，一局几份内存可以忽略。
type diffCache map[reflect.Type]*structMeta

func newDiffCache() diffCache { return diffCache{} }

func (cache diffCache) meta(t reflect.Type) *structMeta {
	if meta, ok := cache[t]; ok {
		return meta
	}
	meta := buildStructMeta(t)
	cache[t] = meta
	return meta
}

// buildStructMeta 只在某个类型第一次出现时跑一次。
func buildStructMeta(t reflect.Type) *structMeta {
	meta := &structMeta{fields: make([]structField, 0, t.NumField())}
	for index := 0; index < t.NumField(); index++ {
		field := t.Field(index)
		if field.PkgPath != "" {
			continue // 未导出字段 encoding/json 也不会带出去
		}
		name, skip := structFieldJSONName(field)
		if skip {
			continue
		}
		meta.fields = append(meta.fields, structField{index: index, name: name})
	}
	for _, candidate := range patchKeyFields {
		found, ok := t.FieldByNameFunc(func(name string) bool {
			structField, ok := t.FieldByName(name)
			return ok && jsonFieldName(structField) == candidate
		})
		if !ok {
			continue
		}
		meta.keys = append(meta.keys, structKeyField{path: found.Index, name: candidate})
	}
	return meta
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
func diffView(prev, next any) any { return diffViewWith(newDiffCache(), prev, next) }

// diffViewWith 用调用方给的缓存算补丁。房间 worker 会一直复用自己的那一份，
// 没必要每次推送都把类型元数据重建一遍。
func diffViewWith(cache diffCache, prev, next any) any {
	if cache == nil {
		cache = newDiffCache()
	}
	return diffAny(cache, reflect.ValueOf(prev), reflect.ValueOf(next))
}

func diffAny(cache diffCache, prev, next reflect.Value) any {
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
		return diffAny(cache, prev, next.Elem())
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
		return diffMap(cache, prev, next)
	case reflect.Slice, reflect.Array:
		if !(prev.Kind() == next.Kind() && prev.Type() == next.Type()) {
			return next.Interface()
		}
		return diffSlice(cache, prev, next)
	case reflect.Struct:
		if prev.Kind() != reflect.Struct || prev.Type() != next.Type() {
			return next.Interface()
		}
		meta := cache.meta(next.Type())
		if len(meta.fields) == 0 {
			return patchNone
		}
		patch := map[string]any{}
		for _, field := range meta.fields {
			change := diffAny(cache, prev.Field(field.index), next.Field(field.index))
			if change != any(patchNone) {
				patch[field.name] = change
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

func diffMap(cache diffCache, prev, next reflect.Value) any {
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
		change := diffAny(cache, before, next.MapIndex(key))
		if change != any(patchNone) {
			patch[name] = change
		}
	}
	removed := make([]string, 0)
	for _, key := range prev.MapKeys() {
		if !next.MapIndex(key).IsValid() {
			removed = append(removed, key.String())
		}
	}
	if len(removed) > 0 {
		// 客户端是逐键合并，JSON 对象没法表达"这个键没了"。键消失时给
		// {"set":{变化的键},"remove":[消失的键]}，语义和实体数组的 upsert/remove 一致。
		// 平时还是普通对象（合并进去就行），别给每个 map 都套一层。
		sort.Strings(removed)
		return map[string]any{"set": patch, "remove": removed}
	}
	if len(patch) == 0 {
		return patchNone
	}
	return patch
}

// sliceKeys 取出数组里每个元素的主键；认不出来或者主键重复，就不是"实体列表"。
// 认得出就按主键合并（只发变了的属性），认不出就整块替换。
//
// 不按路径写死：载舰的飞机、舰队的成员都是嵌在实体里的数组，路径是动态的，
// 写死一份清单迟早漏；这里按元素本身判断，嵌套多深都一样。
func sliceKeys(cache diffCache, list reflect.Value) ([]string, bool) {
	if list.Len() == 0 {
		return nil, false
	}
	keys := make([]string, list.Len())
	seen := make(map[string]bool, list.Len())
	for index := 0; index < list.Len(); index++ {
		_, key := patchKey(cache, list.Index(index))
		if key == "" || seen[key] {
			return nil, false
		}
		seen[key] = true
		keys[index] = key
	}
	return keys, true
}

// diffSlice 按主键增量合并实体数组。
//
// 顺序也必须合得对：客户端是"老元素原位更新、新元素追加到尾部、删掉的摘掉"，
// 所以只有当新数组恰好是"老元素保持原相对顺序（允许有删除）++ 新元素"时，
// 增量合并出来的顺序才和快照一致。不满足就整块重发——距离参考圈按距离排序，
// 舰队一动新圈会插到数组最前面，照追加合并客户端画出来的同心圆顺序就是错的。
// 事件列表正好相反：新事件永远追加在尾部，所以它走增量补丁，而且快照必须
// 按时间正序返回（见 store.recentEvents）。
func diffSlice(cache diffCache, prev, next reflect.Value) any {
	beforeKeys, okBefore := sliceKeys(cache, prev)
	afterKeys, okAfter := sliceKeys(cache, next)
	if okBefore && okAfter {
		if patch, merged := entityListPatch(cache, prev, next, beforeKeys, afterKeys); merged {
			return patch
		}
	}
	if valuesEqual(prev, next) {
		return patchNone
	}
	return next.Interface()
}

// entityListPatch 真正算增量补丁。merged=false 表示顺序合不拢，调用方整块重发。
func entityListPatch(cache diffCache, prev, next reflect.Value, beforeKeys, afterKeys []string) (any, bool) {
	before := make(map[string]int, len(beforeKeys))
	for index, key := range beforeKeys {
		before[key] = index
	}
	// 顺序检查：新元素后面又出现老元素，或者老元素相对顺序变了，都没法靠追加合并。
	last, metNew := -1, false
	for _, key := range afterKeys {
		position, exists := before[key]
		if !exists {
			metNew = true
			continue
		}
		if metNew || position < last {
			return nil, false
		}
		last = position
	}
	upsert := make([]any, 0, len(afterKeys))
	for index, key := range afterKeys {
		item := next.Index(index)
		oldIndex, exists := before[key]
		if !exists {
			// 新出现的元素：整条给，客户端没有底子可合并。
			upsert = append(upsert, item.Interface())
			continue
		}
		// 处理过就从 before 里摘掉，剩下的就是被删除的元素。
		delete(before, key)
		change := diffAny(cache, prev.Index(oldIndex), item)
		if change == any(patchNone) {
			continue
		}
		// 只发变了的属性，补上主键让客户端知道改的是谁。换一次航向在投影里
		// 改的是散落在 wind、units[].deck、aviation.carriers[] 里的几个字段，
		// 这样一条补丁只有几十字节，而不是整条记录重发。
		if partial, ok := change.(map[string]any); ok {
			keyField, _ := patchKey(cache, item)
			partial[keyField] = key
			upsert = append(upsert, partial)
			continue
		}
		upsert = append(upsert, item.Interface())
	}
	remove := make([]string, 0, len(before))
	for key := range before {
		remove = append(remove, key)
	}
	if len(upsert) == 0 && len(remove) == 0 {
		return patchNone, true
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
	return patch, true
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
	// 标量直接比，不走 DeepEqual：DeepEqual 要 Interface() 装箱，
	// 实体一多（上百架飞机、几百格地图）每帧光装箱就是上万次分配。
	case reflect.String:
		return left.String() == right.String()
	case reflect.Bool:
		return left.Bool() == right.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return left.Int() == right.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return left.Uint() == right.Uint()
	case reflect.Float32, reflect.Float64:
		return left.Float() == right.Float()
	}
	return reflect.DeepEqual(left.Interface(), right.Interface())
}
