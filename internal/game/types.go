package game

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// HangarAdvantageLevel 是机库油气爆炸判定的骰子优势档位：
//
//	0 = 劣势（投两次取大，默认）
//	1 = 单投（中性）
//	2 = 2 骰优势（投两次取小）
//	3 = 3 骰优势（投三次取小）
//
// 老存档里这个字段是布尔值（true/false），反序列化时按 true→2、false→0 兼容。
// 档位由剧本的时代决定：1942 珊瑚海的美军航母是 2 骰，1944 马里亚纳是 3 骰（损管与
// 机库通风设计在改进），日军航母基本没有优势。
type HangarAdvantageLevel int

func (l *HangarAdvantageLevel) UnmarshalJSON(data []byte) error {
	switch string(data) {
	case "true":
		*l = 2
		return nil
	case "false", "null":
		*l = 0
		return nil
	}
	var level int
	if err := json.Unmarshal(data, &level); err != nil {
		return err
	}
	*l = HangarAdvantageLevel(level)
	return nil
}

func (l HangarAdvantageLevel) MarshalJSON() ([]byte, error) {
	return json.Marshal(int(l))
}

type MapCell struct {
	Coord   string `json:"coord"`
	Terrain string `json:"terrain"`
	Weather string `json:"weather"`
	Name    string `json:"name"`
}

type MapDefinition struct {
	ID      string    `json:"id"`
	Version int       `json:"version"`
	Name    string    `json:"name"`
	Rows    int       `json:"rows"`
	Cols    int       `json:"cols"`
	Cells   []MapCell `json:"cells"`
	// Clouds 是天气层：行优先、每格一个字节，记这个 hex 压了几层云。
	// 0 晴 / 1 多云 / 2 降雨 / 3 层及以上风暴——层数没有上限，云飘过来叠多少算多少，
	// 每层各自独立移动，不会因为"目标格已经有云"就消散。
	// 开局只铺 1–3 层是为了顺手造出四种天气；之后全靠云自己飘。
	// 字节存层数，前端拿到的是档位串（见 CloudCells），二者别混。
	Clouds string `json:"clouds"`
	// board 是载入时编译出来的地形表（见 board.go）：运行期查地形不再扫 Cells。
	// 不进 JSON——UnmarshalJSON 解完会重新编译，所以从库里读出来也是就绪的。
	board *board
}

// MapForView 返回给前端的海图定义：天气层是服务端内部记账（每格一个字节的层数），
// 不进视图——前端看图只认 weather_cells 的档位串，别把控制字符塞进 JSON 里。
func MapForView(gameMap MapDefinition) MapDefinition {
	gameMap.Clouds = ""
	return gameMap
}

// DeployZone 是一个阵营的开局部署区：锚点 hex + 半径（六边形距离）。
// 区内的格子按地形过滤（暗礁/陆地/机场格按舰种规则排除），舰队在这些格子里随机出生。
type DeployZone struct {
	Faction string `json:"faction"`
	Anchor  string `json:"anchor"`
	Radius  int    `json:"radius"`
	// Fleets 可选：这些舰队用这个部署区。为空表示该阵营没被别的区认领的舰队都落在这里。
	// 马里亚纳日军有两个区：甲/乙/丙三支航母群在地图西北角，丁部队和栗田的第一游击部队在西南。
	Fleets []string `json:"fleets,omitempty"`
}

type Deployment struct {
	ID   string `json:"id"`
	Unit string `json:"unit"`
	// Ship 指向 content/units/ships/<ship>.json 的具名舰卡（如 "lexington"）；
	// 写了 Ship 就用那张卡，Unit 只作为回退的舰级模板。
	Ship    string `json:"ship"`
	Faction string `json:"faction"`
	Name    string `json:"name"`
	Coord   string `json:"coord"`
	// Coords 是随机出生区：写一组候选 hex，部署时按种子随机挑一个（同一舰队共用）。
	// 用于"双方随机搜索遭遇"的测试剧本。
	Coords []string `json:"coords"`
	// Box 是矩形随机出生区：按行列给一个范围，引擎自己展开成候选 hex（跳过陆地、暗礁和不可通行的格）。
	// 珊瑚海这种"日军在索罗门群岛方向、美军在新赫布里底方向随机出现"的剧本用它，
	// 免得把几十个候选 hex 一条条抄进 JSON。coord 与 coords 都为空时才生效。
	Box *DeployBox `json:"box"`
	// Near 让舰队贴着另一支已部署舰队生成（例如 TF44 在 TF17 西北 10 hex 内）。
	// 只有同一舰队的第一艘舰会选取坐标，后续舰沿用 fleetHomes。
	Near           *DeploymentNear `json:"near"`
	FleetID        string          `json:"fleet_id"`
	FleetName      string          `json:"fleet_name"`
	HangarCapacity int             `json:"hangar_capacity"`
	// Overrides 是剧本对单舰属性的覆盖：只覆盖显式写了的字段，没写的沿用单位库。
	// 珊瑚海这类"史实编成"需要按真实参数微调单舰（例如四艘航母统一甲板长度 8），
	// 与其为每艘舰复制一份单位定义，不如在部署里写覆盖。
	Overrides *DeploymentOverrides `json:"overrides"`
}

// DeploymentNear 是相对另一支舰队的出生区。
type DeploymentNear struct {
	FleetID     string `json:"fleet_id"`
	Direction   string `json:"direction"`
	MinDistance int    `json:"min_distance"`
	MaxDistance int    `json:"max_distance"`
}

// DeployBox 是矩形随机出生区，行列都是 1 起的包含边界。
type DeployBox struct {
	RowMin int `json:"row_min"`
	RowMax int `json:"row_max"`
	ColMin int `json:"col_min"`
	ColMax int `json:"col_max"`
}

// DeploymentOverrides 是单舰属性覆盖。全部用指针，nil 表示"沿用单位库"。
type DeploymentOverrides struct {
	Strength        *int                  `json:"strength"`
	DeckLength      *int                  `json:"deck_length"`
	HangarCapacity  *int                  `json:"hangar_capacity"`
	ElevatorCount   *int                  `json:"elevator_count"`
	HeavyAA         *float64              `json:"heavy_aa"`
	LightAA         *float64              `json:"light_aa"`
	MainGunCount    *int                  `json:"main_gun_count"`
	MainGunCaliber  *int                  `json:"main_gun_caliber"`
	DamageControl   *int                  `json:"damage_control"`
	SpeedClass      *string               `json:"speed_class"`
	Gunnery         *int                  `json:"gunnery"`
	ArmoredCarrier  *bool                 `json:"armored_carrier"`
	HangarAdvantage *HangarAdvantageLevel `json:"hangar_advantage"`
}

type AirDeployment struct {
	ID      string `json:"id"`
	Unit    string `json:"unit"`
	Carrier string `json:"carrier"`
	// Skill 是这架舰载机的飞行员熟练度（rookie/regular/veteran/ace，留空按普通）。
	Skill string `json:"skill"`
}

type ScenarioDefinition struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Name    string `json:"name"`
	Map     string `json:"map"`
	// Units 指向单位库文件名（content/units/<Units>.json），为空时用 sandbox。
	Units       string `json:"units"`
	Description string `json:"description"`
	Year        int    `json:"year"`
	Night       bool   `json:"night"`
	// DayHours / TurnsPerDay 开启昼夜循环：开局时刻（小时，可带小数）与每天回合数。
	// TurnsPerDay 为 0 时不循环，夜间状态固定为 Night 字段（兼容老剧本）。
	DayHours    float64 `json:"day_hours"`
	TurnsPerDay int     `json:"turns_per_day"`
	Wind        string  `json:"wind"`
	// WeatherTurns 是天气刷新间隔（0 = 静态天气）。
	WeatherTurns int               `json:"weather_turns"`
	Doctrines    map[string]string `json:"doctrines"`
	// DeployZones 是开局部署区：每个阵营一个锚点 + 半径，舰队在区内随机出生，
	// 部署阶段也只能在区内调整。没有配部署区的剧本直接进第 1 回合。
	DeployZones    []DeployZone         `json:"deploy_zones"`
	Deployments    []Deployment         `json:"deployments"`
	AirDeployments []AirDeployment      `json:"air_deployments"`
	Airfields      []AirfieldDefinition `json:"airfields"`
	// MaxTurns 是剧本回合上限，到达上限后按目标点判定胜负；0 表示不限。
	MaxTurns int `json:"max_turns"`
	// Objectives 是剧本胜利条件（DESIGN.md 49 行）。
	Objectives []Objective `json:"objectives"`
	Timeline   *Timeline   `json:"timeline"`
	// FleetMaxShips 是整编时一支舰队允许的最大舰船数；0 表示按规则书的 20 艘。
	// 马里亚纳这种航母特混大队开局就 29 艘，剧本里放宽到 40 才有拆编的空间。
	FleetMaxShips int `json:"fleet_max_ships"`
	// WithdrawDirections 是各阵营的撤离方向（E/W/NE/…）：舰船重创脱离、运输队卸空后，
	// 都朝这个方向驶向地图边缘。没配的阵营退回"离开地图中心"的通用逻辑。
	WithdrawDirections map[string]string `json:"withdraw_directions"`
}

type UnitDefinition struct {
	ID string `json:"id"`
	// Faction 为空表示通用定义。美日同型舰船的角色卡不同（目前主要差在防空值），
	// 所以同一个 ID 允许多份按阵营区分的定义，部署时按所属阵营取用。
	Faction  string `json:"faction"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Strength int    `json:"strength"`
	// Picture 是兵牌剪影，相对 content/ 的路径，例如 misc/image/ship/new_orleans.png。
	// 素材放数据目录而不是打进前端包：换图、加船都不用重新构建前端。
	Picture string   `json:"picture"`
	Card    ShipCard `json:"card"`
}

type ShipCard struct {
	// Class 是舰级（"埃塞克斯级"、"克利夫兰级"…）。舰名会跨舰级重名——1942 年的
	// 阿斯托里亚是新奥尔良级重巡，1944 年那条是克利夫兰级轻巡——所以名册和兵牌都得
	// 带上舰级才分得清。为空时前端只显示舰种。
	Class string `json:"class"`
	// Picture 由舰船卡注入（来自 units/ships/*.json 顶层的 picture 字段），
	// 前端拿它当剪影蒙版；为空时回落到内置的按舰名映射。
	Picture string `json:"picture"`
	// Nation 是可选的国别标识（目前用于澳大利亚舰船的土黄色兵牌）；为空时按阵营上色。
	Nation         string  `json:"nation"`
	Gunnery        int     `json:"gunnery"`
	DamageControl  int     `json:"damage_control"`
	MainGunCount   int     `json:"main_gun_count"`
	MainGunCaliber float64 `json:"main_gun_caliber"`
	HeavyAA        float64 `json:"heavy_aa"`
	LightAA        float64 `json:"light_aa"`
	SurfaceRadar   bool    `json:"surface_radar"`
	AirRadar       bool    `json:"air_radar"`
	// FireControlRadar 是火控雷达（Mk.8 这类）：只有它给炮击 +10%，对海搜索看 SurfaceRadar。
	// 用指针是为了区分"老存档没写"（nil → 回落到 SurfaceRadar）和"明确没有"（false）。
	FireControlRadar *bool  `json:"fire_control_radar,omitempty"`
	SpeedClass       string `json:"speed_class"`
	TorpedoSalvos    int    `json:"torpedo_salvos"`
	Aircraft         int    `json:"aircraft"`
	HangarCapacity   int    `json:"hangar_capacity"`
	DeckCapacity     int    `json:"deck_capacity"`
	ElevatorCount    int    `json:"elevator_count"`
	DepthCharges     int    `json:"depth_charges"`
	Cargo            int    `json:"cargo"`
	ArmoredCarrier   bool   `json:"armored_carrier"`
	// HangarAdvantage 是机库爆炸判定的骰子优势档位：0 = 劣势（两投取大）、
	// 2 = 2 骰优势（两投取小）、3 = 3 骰优势（三投取小）。这是单舰属性，和国籍无关：
	// 美军航母在 1942 珊瑚海是 2 骰、1944 马里亚纳是 3 骰，列克星敦级没有优势。
	HangarAdvantage HangarAdvantageLevel `json:"hangar_advantage"`
	HistoricalLoss  string               `json:"historical_loss"`
	// Supply 是这条船的弹药供给：例如护航航母（CVE）不提供鱼雷，
	// 停在它上面的鱼雷机就只能挂炸弹/侦察。
	Supply Supply `json:"supply"`
	// MoveCycle 是这级舰的移动节奏：一圈里第 i 格要等 cycle[i] 回合再走下一格。
	// 25 海里/hex、20 分钟/回合，平均 N 回合走一格就是 75/N 节，所以固定的整数周期只能落
	// 在 37.5/25/12.5/8.3 节这几档上；交替周期把航速填细：[2,3] 平均 2.5 → 30 节，
	// [4,4,4,3] 平均 3.75 → 20 节，[6,6,6,7] 平均 6.25 → 12 节。为空时按 speed_class 老四档兜底。
	MoveCycle []int `json:"move_cycle"`
}

type AircraftDefinition struct {
	ID string `json:"id"`
	// Faction 为空表示通用机型。美日机型的航程与型号不同，同 ID 可有多份定义。
	Faction string `json:"faction"`
	Name    string `json:"name"`
	Model   string `json:"model"`
	Kind    string `json:"kind"`
	// Picture 是航空算子图片，相对 content/ 的路径，例如 misc/image/aircraft/f6f.png。
	// 与舰船一致：素材放数据目录，换图、加机型不用重新构建前端；为空时前端回退到内置矢量剪影。
	Picture string `json:"picture"`
	// PictureMode 决定前端怎么画这张图：空/image = 原样显示彩图，mask = 只取 alpha 当单色剪影。
	PictureMode string `json:"picture_mode"`
	Strength    int    `json:"strength"`
	Range       int    `json:"range"`
	AirCombat   int    `json:"air_combat"`
	AntiShip    int    `json:"anti_ship"`
	AntiGround  int    `json:"anti_ground"`
	Seaplane    bool   `json:"seaplane"`
	// 空军型：只能从机场起降，不能部署到航母上
	AirfieldOnly bool `json:"airfield_only"`
	// TorpedoCapable：该机型有鱼雷挂架（B-26 这类中型轰炸机也能挂鱼雷）。
	TorpedoCapable bool `json:"torpedo_capable"`
	// HighAltitudeBomber：挂炸弹攻击舰船时按高空轰炸结算，免疫轻防空但吃高空命中惩罚。
	HighAltitudeBomber bool `json:"high_altitude_bomber"`
}

// AirfieldType 是机场规模：小型 / 中型 / 大型，决定能起降哪些机型；
// 水上飞机基地是另一类，只能操作水上飞机（带浮筒或飞行艇）。
type AirfieldType string

const (
	// AirfieldSeaplane 是水上飞机基地：锚泊在海上，只收水上飞机。
	AirfieldSeaplane AirfieldType = "seaplane"
	AirfieldSmall    AirfieldType = "small"
	AirfieldMedium   AirfieldType = "medium"
	AirfieldLarge    AirfieldType = "large"
)

// NormalizeAirfieldType 把旧剧本/旧存档里的类型名映射到现在的三档规模。
// 早期写的是野战/重型/大型对应的 field/heavy/major，"重型"是错别字，规则里就是中型。
func NormalizeAirfieldType(airfieldType AirfieldType) AirfieldType {
	switch strings.ToLower(strings.TrimSpace(string(airfieldType))) {
	case "field", "small":
		return AirfieldSmall
	case "heavy", "medium":
		return AirfieldMedium
	case "major", "large":
		return AirfieldLarge
	case "seaplane":
		return AirfieldSeaplane
	}
	return airfieldType
}

// AirfieldPicture 返回机场算子中间的标识图片（规则书 92 行"机场标识图片"）。
// 三种规模各有一张跑道符号：小型一条、中型两条、大型三条；水机基地没有跑道，返回空串由前端退化。
// 剧本可以用 definition.picture 覆盖，单独给某个机场画一张图标。
func AirfieldPicture(airfieldType AirfieldType, override string) string {
	if strings.TrimSpace(override) != "" {
		return override
	}
	switch NormalizeAirfieldType(airfieldType) {
	case AirfieldSmall:
		return "misc/image/airfield/small.png"
	case AirfieldMedium:
		return "misc/image/airfield/medium.png"
	case AirfieldLarge:
		return "misc/image/airfield/large.png"
	case AirfieldSeaplane:
		// 水机基地没有跑道，用一张水上飞机停泊的圆形徽章，和三条跑道符号分开。
		return "misc/image/airfield/seaplane.png"
	}
	return ""
}

// AirfieldDefinition 是剧本里部署的机场：跑道数、是否带雷达、防空与岸防值。
type AirfieldDefinition struct {
	ID      string       `json:"id"`
	Faction string       `json:"faction"`
	Name    string       `json:"name"`
	Coord   string       `json:"coord"`
	Type    AirfieldType `json:"type"`
	// Picture 可覆盖按规模自动选的机场标识图片（相对 content/ 的路径）。
	Picture string  `json:"picture"`
	Runways int     `json:"runways"`
	Radar   bool    `json:"radar"`
	HeavyAA float64 `json:"heavy_aa"`
	LightAA float64 `json:"light_aa"`
	Coastal int     `json:"coastal"`
	// HangarCapacity 是停机上限（航空算子数），剧本可覆盖；留空时按初始跑道数 ×6 推导。
	// 航母有独立机库容量，机场以前完全没有上限，转场可以把一堆飞机堆进同一个机场。
	HangarCapacity int      `json:"hangar_capacity"`
	Aircraft       []string `json:"aircraft"`
	// Supply 是这个机场的弹药供给：剧本可以写"这个基地不给鱼雷挂载"。
	Supply Supply `json:"supply"`
}

// Airfield 是运行时的机场状态。生命值 = 初始跑道数 × 5，可用跑道由生命值推导。
type Airfield struct {
	ID      string       `json:"id"`
	Faction string       `json:"faction"`
	Name    string       `json:"name"`
	Coord   string       `json:"coord"`
	Type    AirfieldType `json:"type"`
	Picture string       `json:"picture"`
	Runways int          `json:"runways"`
	// HangarCapacity 是这个机场能停放的航空算子上限（含在外执行任务、返航后仍要落回来的）。
	// 转场进入已满的机场会被拒绝；返航/备降不受限，避免自家飞机被容量卡在天上。
	HangarCapacity int     `json:"hangar_capacity"`
	Strength       int     `json:"strength"`
	MaxStrength    int     `json:"max_strength"`
	Radar          bool    `json:"radar"`
	HeavyAA        float64 `json:"heavy_aa"`
	LightAA        float64 `json:"light_aa"`
	Coastal        int     `json:"coastal"`
	// 本回合剩余的起降通行次数：每条可用跑道每回合最多 12 次（规则书 250 行）。
	// 机场受袭会把正在起降的算子挤回机库，并占用通行次数（规则书 218 行）。
	OpsRemaining int `json:"ops_remaining"`
	// Supply 是这个机场的弹药供给（剧本可配"不提供某种挂载"）。
	Supply Supply `json:"supply"`
}

// runwaySortiesPerTurn 是每条跑道每回合最多支持的起降算子数。
const runwaySortiesPerTurn = 12

// AirfieldAirUnit 是机场上的飞机算子，状态与舰载机保持同一套命名：
// hangar（机库整备）→ service（整备中）→ deck（滑行到跑道）→ ready（可起飞）→ cap/recon/attack_holding/attack → recovery。
type AirfieldAirUnit struct {
	ID           string `json:"id"`
	DefinitionID string `json:"definition_id"`
	AirfieldID   string `json:"airfield_id"`
	Faction      string `json:"faction"`
	Model        string `json:"model"`
	Kind         string `json:"kind"`
	// Skill 是飞行员熟练度，和舰载机同一套定义；机场算子的熟练度写在 ID 里（…-rookie-…）。
	Skill string `json:"skill"`
	// Picture 与 AirUnit.Picture 同源，来自 units/*.json 的 aircraft[].picture，
	// 机场算子落舰/上舰转场时一起带走，前后端都用同一份配置换图。
	Picture     string `json:"picture"`
	PictureMode string `json:"picture_mode"`
	Status      string `json:"status"`
	Strength    int    `json:"strength"`
	MaxRange    int    `json:"max_range"`
	// Fuel 是燃料账本；RemainingRange 是兼容投影，读写走 Range()/Spend()。
	Fuel           Fuel `json:"fuel"`
	RemainingRange int  `json:"remaining_range"`
	AirCombat      int  `json:"air_combat"`
	AntiShip       int  `json:"anti_ship"`
	AntiGround     int  `json:"anti_ground"`
	// AirfieldOnly：陆基机型，只能在机场起降（陆攻等）。
	AirfieldOnly bool `json:"airfield_only"`
	// TorpedoCapable：该机型有鱼雷挂架，挂鱼雷出击时按鱼雷机结算。
	TorpedoCapable bool `json:"torpedo_capable"`
	// HighAltitudeBomber：挂炸弹攻击舰船时按高空轰炸结算。
	HighAltitudeBomber bool `json:"high_altitude_bomber"`
	// Seaplane：带浮筒或飞行艇，只能从水上飞机基地起降。
	Seaplane bool   `json:"seaplane"`
	TaskID   string `json:"task_id"`
	TaskKind string `json:"task_kind"`
	Priority int    `json:"priority"`
	// TaskLane 见 AirUnit.TaskLane：机场编队同样按栏位组队，飞走就空出栏位。
	TaskLane    int    `json:"task_lane"`
	Coord       string `json:"coord"`
	OriginCoord string `json:"origin_coord"`
	TargetCoord string `json:"target_coord"`
	// TargetFleetID 是打击编队认领的敌方舰队：目标 hex 打空之后靠它去追最新情报位置。
	TargetFleetID string `json:"target_fleet_id"`
	// CoverUnitID 是空中掩护跟随的友军单位：目标 hex 里有己方舰队时自动认领，
	// 之后每回合把掩护点更新到该舰队当前位置，而不是钉在起飞时的 hex。
	CoverUnitID      string `json:"cover_unit_id"`
	Heading          string `json:"heading"`
	SectorWidth      int    `json:"sector_width"`
	ReconRadius      int    `json:"recon_radius"`
	ReconDistance    int    `json:"recon_distance"`
	ReconRadiusLimit int    `json:"recon_radius_limit"`
	Returning        bool   `json:"returning"`
	TrackingFleetID  string `json:"tracking_fleet_id"`
	Damaged          bool   `json:"damaged"`
	Maintenance      int    `json:"maintenance"`
	// —— 任务循环（规则书 188–191 行）——
	// 机场算子与舰载机共用同一套循环语义：CAP、空中掩护、侦察降落后自动重新整备、
	// 按原参数再次出动，直到机场失守、跑道被打光、机型不再适合或玩家取消。
	// 落地（landGroundAircraft）会清空 TaskID/TaskKind，所以参数必须在落地前抄进 Cycle*。
	OnCycle      bool   `json:"on_cycle"`
	CycleKind    string `json:"cycle_kind"`
	CycleTaskID  string `json:"cycle_task_id"`
	CycleHeading string `json:"cycle_heading"`
	CycleSector  int    `json:"cycle_sector"`
	CycleTarget  string `json:"cycle_target"`
	CycleOptOut  bool   `json:"cycle_opt_out"`
	// NightWarned 只用于"入夜前回不来"的提示去重：打击编队不再被服务器强制掉头，
	// 但必须提醒一次，别让玩家以为没事。
	NightWarned bool `json:"night_warned"`
	// Disperse：false=集中攻击（权重表），true=分散攻击（均匀随机）（规则书 624–631 行）。
	Disperse bool `json:"disperse"`
	// 挂载与燃料：与舰载机同一套规则（规则书 43–63 行）
	BaseRange     int     `json:"base_range"`
	Loadout       Loadout `json:"loadout"`
	TankDropped   bool    `json:"tank_dropped"`
	Bombed        bool    `json:"bombed"`
	LoadoutManual bool    `json:"loadout_manual"`
}

// AirfieldAirStatus 是前端航空面板用的机场整备状态。
type AirfieldAirStatus struct {
	UnitID         string       `json:"unit_id"`
	Faction        string       `json:"faction"`
	Name           string       `json:"name"`
	Type           AirfieldType `json:"type"`
	Picture        string       `json:"picture"`
	Coord          string       `json:"coord"`
	Strength       int          `json:"strength"`
	MaxStrength    int          `json:"max_strength"`
	MaxRunways     int          `json:"max_runways"`
	Runways        int          `json:"runways"`
	Radar          bool         `json:"radar"`
	HeavyAA        float64      `json:"heavy_aa"`
	LightAA        float64      `json:"light_aa"`
	Coastal        int          `json:"coastal"`
	HangarUsed     int          `json:"hangar_used"`
	HangarCapacity int          `json:"hangar_capacity"`
	// EnemySupplies 是敌方卸在本机场的滩头物资：只要有货，维修就停摆，炮击清空后才恢复维修。
	EnemySupplies int `json:"enemy_supplies"`
	// SupplyDamage 是这批物资永久打掉的生命值：清滩之后也修不回来，维修上限 = MaxStrength - SupplyDamage。
	SupplyDamage int               `json:"supply_damage"`
	Aircraft     []AirfieldAirUnit `json:"aircraft"`
}

// PictureOrDefault 让老存档（还没有 picture 字段）也能拿到按规模算出来的机场标识图片。
func (a Airfield) PictureOrDefault() string {
	return AirfieldPicture(a.Type, a.Picture)
}

// RunwaysAvailable 按规则书计算可用跑道：min(初始跑道, max(生命值,0)/5 向上取整)。
func RunwaysAvailable(current, max int) int {
	if current <= 0 {
		return 0
	}
	available := (current + 4) / 5
	if available > max {
		available = max
	}
	return available
}

// HardensFor 判断机场能否起降某机型：
//
//	小型 = 只能战斗机；中型 = 除战略轰炸机外全部；大型 = 全部；
//	水上飞机基地只能操作水上飞机（带浮筒或飞行艇），反过来轮式飞机也不能进水机基地。
//
// 规则书 98 行：中型机场的跑道长度不够 B-17 这一档起降，只有大型机场能放。
func (a Airfield) HardensFor(aircraftKind string, seaplane bool) bool {
	switch NormalizeAirfieldType(a.Type) {
	case AirfieldSeaplane:
		return seaplane
	case AirfieldSmall:
		return !seaplane && aircraftKind == "fighter"
	case AirfieldMedium:
		return !seaplane && aircraftKind != "strategic_bomber"
	default:
		return !seaplane
	}
}

type UnitCatalog struct {
	ID      string               `json:"id"`
	Version int                  `json:"version"`
	Units   []UnitDefinition     `json:"units"`
	Air     []AircraftDefinition `json:"aircraft"`
}

type AircraftElevator struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Capacity    int      `json:"capacity"`
	Status      string   `json:"status"`
	TaskID      string   `json:"task_id"`
	AircraftIDs []string `json:"aircraft_ids"`
}

type FlightDeck struct {
	Length    int                 `json:"length"`
	Operation string              `json:"operation"`
	Elevators []*AircraftElevator `json:"elevators"`
	// BaseLength 是不含风向修正的基础甲板长度；Length 是加上风向修正后的本回合容量（规则书 815–825 行）
	BaseLength int `json:"base_length"`
}

type ShipDamage struct {
	Fire string `json:"fire"`
	// SmallFireHits 记录本回合内已经触发过几次小型火灾：第二次触发即升级为起火燃烧（规则书 432 行）。
	// 小型火灾在回合开始时自动扑灭，同时清零这个计数。
	SmallFireHits int   `json:"small_fire_hits"`
	Flooding      []int `json:"flooding"`
	Disabled      bool  `json:"disabled"`
	Tilt          bool  `json:"tilt"`
	OilLeak       bool  `json:"oil_leak"`
	// OilLeakTurns 是漏油修复判定的倒计时：漏油后每 3 回合掷一次 d10，
	// 小于等于当前损管值即修复；失败则重置为 3 再等一轮。
	OilLeakTurns    int  `json:"oil_leak_turns"`
	DeckDamaged     bool `json:"deck_damaged"`
	DeckDestroyed   bool `json:"deck_destroyed"`
	ElevatorDamage  int  `json:"elevator_damage"`
	TorpedoDisabled bool `json:"torpedo_disabled"`
	// 对海雷达有两种损坏，各管一摊：搜索雷达提供侦查/突袭判定，火控雷达提供炮击命中加成。
	// 规则书原来把"火控雷达损坏"和"对海搜索雷达损坏"写成都失去对海雷达加成，
	// 这里拆开，让命中率只受火控影响、搜索相关判定只受搜索雷达影响。
	SurfaceRadarDisabled     bool `json:"surface_radar_disabled"`
	FireControlRadarDisabled bool `json:"fire_control_radar_disabled"`
	AirRadarDisabled         bool `json:"air_radar_disabled"`
	SonarDisabled            bool `json:"sonar_disabled"`
}

type Unit struct {
	ID           string `json:"id"`
	DefinitionID string `json:"definition_id"`
	FleetID      string `json:"fleet_id"`
	FleetName    string `json:"fleet_name"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Faction      string `json:"faction"`
	Coord        string `json:"coord"`
	Heading      string `json:"heading"`
	Strength     int    `json:"strength"`
	MaxStrength  int    `json:"max_strength"`
	Moved        bool   `json:"moved"`
	MoveCooldown int    `json:"move_cooldown"`
	// CyclePos 是整支舰队在移动节奏里走到第几格：交替周期靠它决定这次挂几回合冷却。
	// 队内所有船共用同一个值，移动时一起推进。
	CyclePos     int `json:"cycle_pos"`
	Confusion    int `json:"confusion"`
	ContactLevel int `json:"contact_level"`
	// ContactStaleTurns 是这条情报已经连续几回合没更新（0 = 本回合刚刷新）。
	// 事件里一直有"情报已过期N回合"的文字，但机器玩家读不到。
	ContactStaleTurns int         `json:"contact_stale_turns"`
	Card              ShipCard    `json:"card"`
	Damage            *ShipDamage `json:"damage"`
	Deck              *FlightDeck `json:"deck"`
	LegalMoves        []string    `json:"legal_moves"`
	// EffectiveSpeed 是算上进水层数之后的实际航速档位（fast/normal/slow/very_slow/disabled）。
	// 卡面 speed_class 是设计值，玩家需要看的是"现在到底跑多快"：兵牌上的白条就按它画。
	EffectiveSpeed string `json:"effective_speed"`
	// Withdrawing 表示这艘船处于强制/重创撤退状态，每回合自动驶向地图边缘
	Withdrawing bool `json:"withdrawing"`
	// Destination / Engagement 是自动航行的目的地与交战设置（规则书 280–283、362–370 行）。
	// 整支舰队共用，写在队内首舰上。
	Destination string `json:"destination"`
	Engagement  string `json:"engagement"`
}

type DistanceHex struct {
	Coord    string `json:"coord"`
	Distance int    `json:"distance"`
}

type DistanceReference struct {
	Origin      string        `json:"origin"`
	MaxDistance int           `json:"max_distance"`
	Cells       []DistanceHex `json:"cells"`
}

type FleetGroup struct {
	ID           string `json:"id"`
	FleetID      string `json:"fleet_id"`
	FleetName    string `json:"fleet_name"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Faction      string `json:"faction"`
	Coord        string `json:"coord"`
	Heading      string `json:"heading"`
	Strength     int    `json:"strength"`
	MaxStrength  int    `json:"max_strength"`
	Moved        bool   `json:"moved"`
	MoveCooldown int    `json:"move_cooldown"`
	MoveInterval int    `json:"move_interval"`
	// MoveKnots 是按移动节奏折算出来的平均航速，面板上直接显示"约 N 节"。
	MoveKnots    int   `json:"move_knots"`
	MoveCycle    []int `json:"move_cycle"`
	Confusion    int   `json:"confusion"`
	ContactLevel int   `json:"contact_level"`
	// ContactStaleTurns 是这条情报已经连续几回合没更新（0 = 本回合刚刷新）。
	ContactStaleTurns int    `json:"contact_stale_turns"`
	ContactSize       string `json:"contact_size"`
	// ContactCapitals 是情报 3 级能看出的主力舰类别：carrier / battleship / heavy_cruiser。
	ContactCapitals []string `json:"contact_capitals"`
	LegalMoves      []string `json:"legal_moves"`
	Count           int      `json:"count"`
	// Destination / Engagement 是这个舰队自己的自动航行设置（规则书 280–283、362–370 行）。
	Destination       string             `json:"destination"`
	Engagement        string             `json:"engagement"`
	Members           []Unit             `json:"members"`
	DistanceReference *DistanceReference `json:"distance_reference"`
}

type AirUnit struct {
	ID           string `json:"id"`
	DefinitionID string `json:"definition_id"`
	CarrierID    string `json:"carrier_id"`
	Faction      string `json:"faction"`
	Name         string `json:"name"`
	Model        string `json:"model"`
	Kind         string `json:"kind"`
	// Skill 是飞行员熟练度（rookie/regular/veteran/ace，缺省按普通）；它只决定 DC 掷骰档位，
	// 不改对空/对舰数值，部署时精锐机组规则已经折进这一档。
	Skill string `json:"skill"`
	// Picture/PictureMode 由 aircraft[].picture/picture_mode 注入；老存档缺图时前端回退到内置剪影。
	Picture     string `json:"picture"`
	PictureMode string `json:"picture_mode"`
	Status      string `json:"status"`
	Strength    int    `json:"strength"`
	MaxRange    int    `json:"max_range"`
	// BaseRange 是机种的基础航程（不含副油箱与转场加成）；挂载变化时用它换算新的 MaxRange。
	BaseRange int `json:"base_range"`
	// Fuel 是燃料账本（副油箱 / 机内 / 备用），是航程的唯一真相来源。
	Fuel Fuel `json:"fuel"`
	// Loadout 是当前武器挂载，决定航程加成与攻击值（规则书 43–63 行）。
	Loadout Loadout `json:"loadout"`
	// TankDropped 表示副油箱已抛弃：Tank 里程归零，剩余航程随之下降（规则书 48 行）。
	TankDropped bool `json:"tank_dropped"`
	// Bombed 表示已投弹：剩余航程获得 20% 轻载加成（规则书 83 行）。
	Bombed bool `json:"bombed"`
	// LoadoutManual 表示玩家在机库里手动选过挂载：分配任务时不再用默认值覆盖。
	LoadoutManual bool `json:"loadout_manual"`
	// RemainingRange 是 Fuel 的兼容投影：所有读写都应走 Range()/SetRange()，
	// 保留字段只是为了让旧存档和老代码在迁移期仍能工作。
	RemainingRange int  `json:"remaining_range"`
	AirCombat      int  `json:"air_combat"`
	AntiShip       int  `json:"anti_ship"`
	AntiGround     int  `json:"anti_ground"`
	Seaplane       bool `json:"seaplane"`
	// AirfieldOnly：陆基机型（陆攻、P-40、B-25 这类），只能在机场起降，
	// 前端据此不再显示"舰载机"的蓝色航程框。
	AirfieldOnly bool `json:"airfield_only"`
	// TorpedoCapable / HighAltitudeBomber 从机种定义带过来，决定挂载表与攻击结算。
	TorpedoCapable     bool `json:"torpedo_capable"`
	HighAltitudeBomber bool `json:"high_altitude_bomber"`
	// SupplyBlocked 是给界面用的投影：当前补给点（母舰/机场）不提供哪些武器类别。
	// 只由视图层填，引擎判定走 SupplyOfAircraft / SupplyOfGroundAircraft。
	SupplyBlocked []Weapon `json:"supply_blocked,omitempty"`
	Launched      bool     `json:"launched"`
	ElevatorID    string   `json:"elevator_id"`
	Priority      int      `json:"priority"`
	TaskID        string   `json:"task_id"`
	TaskKind      string   `json:"task_kind"`
	// TaskLane 是编队在航空面板里占的栏位（1 起；0 = 老存档没记过）。
	// 栏位只表示"正在组队的位置"：编队全部升空之后栏位就空出来，
	// 下一支新编队重新拿到 I，编号不会随着列表里挂着的已出发编队往后漂。
	TaskLane    int    `json:"task_lane"`
	OriginCoord string `json:"origin_coord"`
	Coord       string `json:"coord"`
	TargetCoord string `json:"target_coord"`
	// TargetFleetID 是打击编队认领的敌方舰队：目标 hex 打空之后靠它去追最新情报位置。
	TargetFleetID string `json:"target_fleet_id"`
	// CoverUnitID 是空中掩护跟随的友军单位：目标 hex 里有己方舰队时自动认领，
	// 之后每回合把掩护点更新到该舰队当前位置，而不是钉在起飞时的 hex。
	CoverUnitID string `json:"cover_unit_id"`
	// CannotLand 表示转场/备降的算子到了目标位置却降不下去（舰队机库已满），
	// 只能挂在天上等玩家改转场目标。
	CannotLand bool `json:"cannot_land"`
	// DivertAirfield 是"母舰沉没或甲板彻底报废"之后选定的备降机场：
	// 算子自己一格一格飞过去，到了才降落，不做瞬移。
	DivertAirfield string `json:"divert_airfield"`
	// Disperse：false=集中攻击（按权重表），true=分散攻击（均匀随机选船）（规则书 624–631 行）。
	Disperse         bool   `json:"disperse"`
	Heading          string `json:"heading"`
	SectorWidth      int    `json:"sector_width"`
	ReconRadius      int    `json:"recon_radius"`
	ReconDistance    int    `json:"recon_distance"`
	ReconRadiusLimit int    `json:"recon_radius_limit"`
	Returning        bool   `json:"returning"`
	TrackingFleetID  string `json:"tracking_fleet_id"`
	Confusion        bool   `json:"confusion"`
	Damaged          bool   `json:"damaged"`
	Maintenance      int    `json:"maintenance"`
	// ServiceTurns 记录机库加油整备已经消耗的回合数（规则书 240–244 行：整备、提升、起飞各 1 回合）。
	ServiceTurns int `json:"service_turns"`
	// Engaged 表示本编队已成功接敌，下一回合可以进入目标上空发起空袭（规则书 609 行）
	Engaged bool `json:"engaged"`
	// SearchHeading 是打击编队搜索接敌时的当前航向：接敌失败后沿这个方向继续
	// 前出（正前 50%、左斜前 25%、右斜前 25%），每回合朝选中的方向漂一格，
	// 不做“折回旧坐标”的往返，搜索区才是逐渐外扩的扇形。
	SearchHeading string `json:"search_heading"`
	// SearchTurns 记录打击编队在目标空域连续搜索却没能接敌的回合数：
	// 到上限就放弃返航，避免对着过期情报一直兜圈、把油烧到临近销毁。
	SearchTurns int `json:"search_turns"`
	// WarnedIntercept 表示途中被敌方雷达截获，后续空袭命中的 DCT 临时 +1（规则书 408 行）
	WarnedIntercept bool `json:"warned_intercept"`
	// NightWarned 是"这波天黑前回不来"的提示去重标记：打击编队不再被强制掉头（玩家自己决定），
	// 但服务器仍然提醒一次。
	NightWarned bool `json:"night_warned"`
	// OnCycle 是任务循环开关（规则书 188 行：CAP、反潜、侦察任务自动循环；
	// 空中掩护按同一套循环处理）。降落后自动重新整备、按原参数再次升空，
	// 直到航程不足、母舰不可用、损伤未修复或玩家取消（规则书 190、191 行）。
	// PatrolCoord 是 CAP 的巡逻空域（默认为母舰格）；ForwardCoord 是雷达引导前出的接敌点。
	// 掩护范围 = 巡逻点/前出点及其周围 1 hex（共 7 格），不对外绘制范围。
	OnCycle      bool   `json:"on_cycle"`
	PatrolCoord  string `json:"patrol_coord"`
	ForwardCoord string `json:"forward_coord"`
	// —— 任务循环参数 ——
	// 降落时 LandAircraft 会调用 clearAirTask 清空任务参数，循环必须在落地前把这些值存下来，
	// 否则"再飞一次"只能飞回一个空任务（航向/扇面/掩护点全丢）。
	CycleKind    string `json:"cycle_kind"`
	CycleTaskID  string `json:"cycle_task_id"`
	CycleHeading string `json:"cycle_heading"`
	CycleSector  int    `json:"cycle_sector"`

	CycleTarget string `json:"cycle_target"`
	CyclePatrol string `json:"cycle_patrol"`
	// CycleOptOut 表示玩家手动关掉了这台算子的自动循环（规则书 191 行）：
	// 落地之后不会自己恢复，直到重新编任务或玩家再点一次。
	CycleOptOut bool `json:"cycle_opt_out"`
	// InterceptedThisTurn 只用于界面提示"这架 CAP 本回合已经打过"，
	// 不再限制参战次数：CAP 一直在目标上空，直到被击中或弹药耗尽才退出。
	InterceptedThisTurn bool `json:"intercepted_this_turn"`
	// Armed 表示这架飞机已经整备完毕（挂好弹、加好油），属于"整装待发"：
	// 机库里或甲板上被炸弹命中的整装待发飞机会被摧毁（规则书 421–423 行）。
	// 由 aircraftReady 统一推导：有挂载 + 整备回合走完 + 没损伤/检修，和具体任务无关。
	Armed bool `json:"armed"`
	// DeckService 表示这架飞机正在甲板上整备：status 仍然是 service，
	// 但作业位置在甲板（占甲板位、不占机库位），整备完成后直接转成 deck 待发。
	// 空载算子拖上甲板后再选挂载，走的就是这条路径。
	DeckService bool `json:"deck_service"`
	// LiftRequested 表示玩家已经把机库里的算子拖进甲板栏，等升降机把它提上去。
	// 整备还没走完的算子会把请求留在那里，整备一完成就排队提升。
	LiftRequested bool `json:"lift_requested"`
	// ReturnToAirfield 是编队选定的备降机场；为空则返航回母舰（规则书 202–205 行）
	ReturnToAirfield string `json:"return_to_airfield"`
}

type ReconSector struct {
	AircraftID        string             `json:"aircraft_id"`
	Status            string             `json:"status"`
	OriginCoord       string             `json:"origin_coord"`
	CurrentCoord      string             `json:"current_coord"`
	Heading           string             `json:"heading"`
	SectorWidth       int                `json:"sector_width"`
	ReconRadius       int                `json:"recon_radius"`
	ReconDistance     int                `json:"recon_distance"`
	Returning         bool               `json:"returning"`
	TrackingFleetID   string             `json:"tracking_fleet_id"`
	CannotLand        bool               `json:"cannot_land"`
	SectorHexes       []string           `json:"sector_hexes"`
	CurrentRingHexes  []string           `json:"current_ring_hexes"`
	DistanceReference *DistanceReference `json:"distance_reference"`
}

type AirTask struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	Phase         string    `json:"phase"`
	Name          string    `json:"name"`
	CarrierID     string    `json:"carrier_id"`
	CarrierIDs    []string  `json:"carrier_ids"`
	FleetID       string    `json:"fleet_id"`
	AircraftIDs   []string  `json:"aircraft_ids"`
	Aircraft      []AirUnit `json:"aircraft"`
	AirborneCount int       `json:"airborne_count"`
	Priority      int       `json:"priority"`
	// Lane 是这支编队在面板里占的栏位（1 起；0 = 老存档没记过，前端按位置兜底）。
	Lane          int    `json:"lane"`
	QueuePosition int    `json:"queue_position"`
	OriginCoord   string `json:"origin_coord"`
	CurrentCoord  string `json:"current_coord"`
	TargetCoord   string `json:"target_coord"`
	// Disperse 是攻击编队的攻击方式（规则书 624–631 行）。
	Disperse         bool     `json:"disperse"`
	Faction          string   `json:"faction"`
	Detected         bool     `json:"detected"`
	Engaged          bool     `json:"engaged"`
	Heading          string   `json:"heading"`
	SectorWidth      int      `json:"sector_width"`
	ReconRadius      int      `json:"recon_radius"`
	ReconDistance    int      `json:"recon_distance"`
	Returning        bool     `json:"returning"`
	TrackingFleetID  string   `json:"tracking_fleet_id"`
	CannotLand       bool     `json:"cannot_land"`
	SectorHexes      []string `json:"sector_hexes"`
	CurrentRingHexes []string `json:"current_ring_hexes"`
	// ReturnTurns 是这支编队"走完任务再落回母舰"还要几个回合（含排队着舰余量）。
	// 前端拿它和 clock.night_in_turns 一比，就能在天黑前提醒玩家"这波回不来了"。
	ReturnTurns int `json:"return_turns"`
	// NightSafe 是显式布尔值：false 正是最要紧的那个值（这波天黑前回不来）。
	// 视图里的字段一律显式下发，不做"零值就省略"（那是补丁/快照对不上的老坑）。
	NightSafe bool `json:"night_safe"`
	// TurnsToDark / NightMargin 是给机器玩家算账用的：距天黑还剩几回合、返航后富余几回合。
	// NightMargin < 0 等于"走完任务再回来就摸黑了"。没有昼夜循环的剧本里 TurnsToDark = -1。
	TurnsToDark int `json:"turns_to_dark"`
	NightMargin int `json:"night_margin"`
	// —— 攻击编队的到点/落舰预估（前端弹窗直接显示，见 strike_timeline.go）——
	// TurnsToTarget：从现在到空袭真正打在目标上的回合数；TurnsToLanded：到最后一架落回来的回合数。
	TurnsToTarget int `json:"turns_to_target"`
	TurnsToLanded int `json:"turns_to_landed"`
	// ETATarget / ETALanded 是同一笔账换算成的当天时刻（"HH:MM"）。没有昼夜循环的剧本里为空串。
	ETATarget string `json:"eta_target"`
	ETALanded string `json:"eta_landed"`
	// LandsAtNight：按预估，落舰那一刻已经在夜间（夜间着舰航母 70%、机场 20% 摔毁）。
	LandsAtNight      bool               `json:"lands_at_night"`
	ReconSectors      []ReconSector      `json:"recon_sectors"`
	DistanceReference *DistanceReference `json:"distance_reference"`
}

type AviationView struct {
	Carriers []CarrierAirStatus `json:"carriers"`
	Tasks    []AirTask          `json:"tasks"`
}

type CarrierAirStatus struct {
	UnitID         string `json:"unit_id"`
	Faction        string `json:"faction"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	HangarUsed     int    `json:"hangar_used"`
	HangarCapacity int    `json:"hangar_capacity"`
	DeckLength     int    `json:"deck_length"`
	DeckCapacity   int    `json:"deck_capacity"`
	DeckUsed       int    `json:"deck_used"`
	LaunchUsed     int    `json:"launch_used"`
	Operation      string `json:"operation"`
	// BlockedReason 说明这艘航空舰当前为什么不能起降（倾斜、起火、甲板摧毁、甲板满……）。
	// 空串表示正常。之前这些状态只体现在"命令被拒"里，玩家看不到原因。
	BlockedReason string `json:"blocked_reason"`
	// PreventedBy 指出挡路的具体来源（例如甲板上占位的无目标攻击编队）。
	PreventedBy string `json:"prevented_by"`
	// SupplyBlocked 是这条船/这个机场不能提供的武器类别，航空面板据此提示"没有鱼雷"。
	SupplyBlocked []Weapon           `json:"supply_blocked,omitempty"`
	Elevators     []AircraftElevator `json:"elevators"`
	Aircraft      []AirUnit          `json:"aircraft"`
}

type FleetContact struct {
	FleetID string `json:"fleet_id"`
	Faction string `json:"faction"`
	Level   int    `json:"level"`
	Coord   string `json:"coord"`
	// DisplayCoord 是玩家在地图上看到的"最后已知位置"（带情报误差）。
	// 情报衰减只降 Level、不改这里，所以标记不会随着等级下降自己漂移（规则书 593 行）。
	DisplayCoord string `json:"display_coord"`
	Heading      string `json:"heading"`
	Count        int    `json:"count"`
	Members      []Unit `json:"members"`
	// StaleTurns 是这条情报已经连续几个回合没更新（0 = 本回合刚刷新）。
	// 事件里早就有"情报已过期N回合"的文字，但 AI 读不到；这里给结构化字段。
	StaleTurns int `json:"stale_turns"`
}

// StrikeTarget 是本次空袭的目标类型：舰船（对舰攻击）或机场（对地攻击）。
type StrikeTarget string

const (
	StrikeTargetShip  StrikeTarget = "ship"
	StrikeTargetGroup StrikeTarget = "group"
	StrikeTargetAir   StrikeTarget = "airfield"
)

// AirHitDetail 记录一次命中落在哪架攻击机上，用来在对舰/对地动画里把损失画到具体飞机上。
// outcome 取值为撤退、损伤、摧毁、无效果，与航空损伤表一致。
type AirHitDetail struct {
	Attacker string `json:"attacker"`
	Model    string `json:"model"`
	Outcome  string `json:"outcome"`
}

// AirShipCard 是被攻击舰队在打击报告里的舰船快照，动画按它画出舰队全部船只。
type AirShipCard struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Strength int    `json:"strength"`
	// Picture 是舰船卡的素材路径：空战动画里的舰船剪影要用同一批资源，
	// 否则战报画面只能画内置手绘剪影，和地图兵牌长得不一样。
	Picture string `json:"picture"`
}

// StrikeAllocation 记录一个攻击算子最终分配到哪艘舰、对它有几次命中。
type StrikeAllocation struct {
	Attacker string `json:"attacker"`
	Model    string `json:"model"`
	ShipID   string `json:"ship_id"`
	ShipName string `json:"ship_name"`
	Hits     int    `json:"hits"`
	// Attempts 是这个算子实际投出的炸弹/鱼雷次数（对舰攻击值）。
	// 分配了目标但还没投弹就被轻防空打掉的算子 Hits 和 Attempts 都是 0，
	// 战报据此能区分"没投弹"和"投了没中"。
	Attempts int `json:"attempts"`
}

type AirCombatReport struct {
	ID              string   `json:"id"`
	Turn            int      `json:"turn"`
	Coord           string   `json:"coord"`
	Target          string   `json:"target"`
	TargetName      string   `json:"target_name"`
	AttackerFaction string   `json:"attacker_faction"`
	DefenderFaction string   `json:"defender_faction"`
	AttackerModels  []string `json:"attacker_models"`
	DefenderModels  []string `json:"defender_models"`
	// AttackerPictures / DefenderPictures 与上面两个机型名一一对应，
	// 前端空战动画直接显示算子图片，缺图时再回退到内置矢量剪影。
	AttackerPictures []string `json:"attacker_pictures"`
	DefenderPictures []string `json:"defender_pictures"`
	AttackerCount    int      `json:"attacker_count"`
	DefenderCount    int      `json:"defender_count"`
	AttackerLosses   int      `json:"attacker_losses"`
	DefenderLosses   int      `json:"defender_losses"`
	// Disabled 是"退出战斗但没被击落"的算子：损伤、撤退、弹药耗尽。
	// 损失数只统计被摧毁的，两者分开，避免战报把驱散说成击落。
	AttackerDisabled int `json:"attacker_disabled"`
	DefenderDisabled int `json:"defender_disabled"`
	// disabledSeen 防止同一架飞机在一次交战中因多次命中被重复计入"退出战斗"。
	disabledSeen map[string]bool `json:"-"`
	AAHits       int             `json:"aa_hits"`
	StrikeHits   int             `json:"strike_hits"`
	// DamageHits 是"命中并且真的打掉强度"的次数：命中之后还要过损管判定，
	// 没过损管的命中只降防空、砸甲板，不掉强度。战报里两个数分开显示，避免看着像一码事。
	DamageHits int `json:"damage_hits"`
	// 对舰：被命中的舰船数量与命中次数；对地：机场损失与地面飞机损失
	ShipsHit int `json:"ships_hit"`
	ShipHits int `json:"ship_hits"`
	// 对舰：命中次数；参与投弹的算子和投弹尝试次数，0 命中也说明"确实投了弹"
	Bombers      []AirHitDetail `json:"bombers"`
	BombAttempts int            `json:"bomb_attempts"`
	// 对地：不挂弹的护航战斗机按固定 1 点强度扫射跑道，命中只掉地面飞机。
	StrafeAttempts int `json:"strafe_attempts"`
	StrafeHits     int `json:"strafe_hits"`
	// 对舰：被攻击舰队的船只清单，以及每个算子的目标分配和命中次数
	Ships             []AirShipCard      `json:"ships"`
	TargetAllocations []StrikeAllocation `json:"target_allocations"`
	// 对地：机场损失与地面飞机损失。空袭命中一次只落在跑道/岸炮/驻机其中之一，
	// 所以 AirfieldHit 是命中总数，真正扣机场生命值的是 AirfieldRunwayHits。
	AirfieldHit        int `json:"airfield_hits"`
	AirfieldRunwayHits int `json:"airfield_runway_hits"`
	AirfieldCoastal    int `json:"airfield_coastal"`
	GroundLoss         int `json:"ground_losses"`
	// Detected 表示本次空袭已被目标方雷达提前发现（被截获时空袭命中的 DCT +1，规则书 408 行）
	Detected              bool           `json:"detected"`
	Airfield              *AirShipCard   `json:"airfield"`
	AirfieldBefore        int            `json:"airfield_before"`
	AirfieldAfter         int            `json:"airfield_after"`
	AirfieldRunwaysBefore int            `json:"airfield_runways_before"`
	AirfieldRunwaysAfter  int            `json:"airfield_runways_after"`
	AttackerHits          []AirHitDetail `json:"attacker_hits"`
	Winner                string         `json:"winner"`
	Summary               string         `json:"summary"`
}

type State struct {
	ScenarioID   string `json:"scenario_id"`
	ScenarioName string `json:"scenario_name"`
	Description  string `json:"description"`
	Year         int    `json:"year"`
	Night        bool   `json:"night"`
	// DayHours / TurnsPerDay：开启昼夜循环时的开局时刻与每天回合数（0 表示固定昼夜）。
	DayHours    float64 `json:"day_hours"`
	TurnsPerDay int     `json:"turns_per_day"`
	// WeatherTurns 是天气刷新间隔（0 = 静态天气）。
	WeatherTurns int               `json:"weather_turns"`
	Doctrines    map[string]string `json:"doctrines"`
	// WithdrawDirections 是剧本给的撤离方向；退避队每回合朝它走一格（马里亚纳：美军 E、日军 W）。
	WithdrawDirections map[string]string                      `json:"withdraw_directions"`
	Map                MapDefinition                          `json:"map"`
	Units              map[string]*Unit                       `json:"units"`
	Aircraft           map[string]*AirUnit                    `json:"aircraft"`
	Airfields          map[string]*Airfield                   `json:"airfields"`
	AirfieldContacts   map[string]map[string]*AirfieldContact `json:"airfield_contacts"`
	// 每个舰队上一回合结束时所在的 hex，用于判断"停留满 1 回合"（岸轰条件）
	FleetStays map[string]string                   `json:"fleet_stays"`
	GroundAir  map[string]*AirfieldAirUnit         `json:"ground_aircraft"`
	Contacts   map[string]map[string]*FleetContact `json:"contacts"`
	AirCombats []AirCombatReport                   `json:"air_combats"`
	// RadioUsed 记录本回合已经发射过无线电的编队（任务 ID）：每个空中编队每回合各只有一次机会，
	// 发射后不论成败都记在这里（规则书 601 行）。键是任务 ID，值是所属阵营。
	RadioUsed map[string]string `json:"radio_used"`
	// DeferCooldown 表示本回合已经用"立即移动"下达过规划航线，结算时不再额外递减冷却
	DeferCooldown bool `json:"defer_cooldown"`
	// Wind 是剧本给定的盛行风向（NE/E/SE/SW/W/NW），决定甲板长度修正（规则书 815–825 行）
	Wind string `json:"wind"`
	// WindField 是每 hex 的风向场（windfield.go）：盛行风 + 三层长波谐波。
	// 有它就用坐标查风，没有（老存档 / 没设风向的剧本）就退回 Wind 的单一风向。
	WindField *WindField `json:"wind_field"`
	// WindNextTurn 是下一次风向场推进的回合号：每 6–12 回合缓慢挪一次，
	// 不是每回合都动（甲板修正每回合抖一下只会让玩家莫名其妙）。
	WindNextTurn int `json:"wind_next_turn"`
	// HeadingChanged 记录本回合已经调整过朝向的舰队（每回合每队一次）
	HeadingChanged map[string]bool `json:"heading_changed"`
	// IntelFresh 记录本回合是否获得过某舰队的新情报（键为 "观察者|舰队"），用于情报过时判定
	IntelFresh map[string]bool `json:"intel_fresh"`
	// IntelRaised 记录本回合哪些"观察者|舰队"已经升过级：同回合目视/雷达/搜索/跟踪
	// 可能同时命中，等级只能 +1，靠它去重。
	IntelRaised map[string]bool `json:"intel_raised"`
	// IntelStaleTurns 记录"脱离接触"后当前等级已经撑了几个回合：
	// 4 级撑 6 回合，之后 3/2/1 级各撑 3 回合，到期降一级并清零重新计时
	IntelStaleTurns map[string]int `json:"intel_stale_turns"`
	// Losses 是战果台账：每方沉没的舰船与按机型聚合的飞机损失（战果页面用）
	Losses map[string]*LossLedger `json:"losses"`
	// AircraftRoster 是回合开始时的在编算子快照，用于回合结束对账出飞机损失
	AircraftRoster map[string]AirUnit `json:"-"`
	// EngageTried 记录本回合已经做过接敌判定的攻击编队（任务 ID），避免同编队重复判定
	EngageTried map[string]bool `json:"engage_tried"`
	// FleetSessions 是舰队在同一 hex 的连续停留回合数：岸轰要 1 回合，运输卸载要 3 回合
	FleetSessions map[string]int `json:"fleet_sessions"`
	// SurpriseLock 是突袭冷却：舰队 ID → 还要等几回合才能重新取得突袭。
	// 打完一轮雷击要重新装填、重整队形，否则快船可以"突进去-退出来"一直白拿先手。
	SurpriseLock map[string]int `json:"surprise_lock"`
	// ShoreSupplies 是已经卸到岸上的物资，键为 "阵营|坐标"
	ShoreSupplies map[string]int `json:"shore_supplies"`
	// 胜利条件（DESIGN.md 49 行）：剧本目标 + 回合上限，结算后统一判定。
	Objectives []Objective `json:"objectives"`
	MaxTurns   int         `json:"max_turns"`
	// FleetMaxShips 是本局一支舰队的舰船上限（来自剧本，0 表示 20）。
	FleetMaxShips int `json:"fleet_max_ships"`
	// Turn 是当前回合号（1 起），由服务端每回合结算后递增；引擎内部的胜负判定要用到它。
	Turn int `json:"turn"`
	// AirContacts 是"某个阵营已经发现的敌方飞行编队"（键：观察者 → 任务 ID）。
	AirContacts map[string]map[string]*AirContact `json:"air_contacts"`
	// 初始舰船数（按阵营），用于"击沉 N 艘"这类目标。
	InitialShipCounts map[string]int `json:"initial_ship_counts"`
	// Phase 是开局阶段标记："deploy" 表示还在部署（双方都在区内摆船，没人打完），
	// "battle"（或老存档的空值）表示正常对局。部署阶段不结算回合，双方都点部署完毕才进第 1 回合。
	Phase string `json:"phase"`
	// DeployZones 是双方的开局部署区（锚点 + 半径），布局验证和地图高亮都靠它。
	DeployZones []DeployZone `json:"deploy_zones"`
	// Outcome 是当前对局结果：为空表示仍在进行。
	Outcome     *GameOutcome      `json:"outcome"`
	RNGState    uint64            `json:"rng_state"`
	Definitions map[string]string `json:"definition_versions"`
	// DeferredSink 为 true 时，舰船沉没先记进 PendingSinks，等这一轮投弹全部落下再真正移除。
	// 对舰攻击按"一轮投弹里所有炸弹都会砸上去"结算：半路打沉不打断这一轮，
	// 沉没只影响它后面的那轮轻防空、以及后面对其他舰的攻击。
	DeferredSink bool      `json:"-"`
	PendingSinks []*Unit   `json:"-"`
	Timeline     *Timeline `json:"timeline"`
}

type Order struct {
	Type         string   `json:"type"`
	UnitID       string   `json:"unit_id"`
	TargetUnitID string   `json:"target_unit_id"`
	AircraftID   string   `json:"aircraft_id"`
	AircraftIDs  []string `json:"aircraft_ids"`
	TaskID       string   `json:"task_id"`
	Coord        string   `json:"coord"`
	// TargetFleetID 用于 SET_STRIKE_TARGET / LAUNCH_STRIKE：同一 hex 同时有敌方机场和舰队时，
	// 显式指定打哪支舰队；为空时保持旧的“机场优先”行为。
	TargetFleetID string            `json:"target_fleet_id"`
	Heading       string            `json:"heading"`
	SectorWidth   int               `json:"sector_width"`
	Assignments   map[string]string `json:"assignments"`
	// TaskKind 只用于无线电改令（RETASK_AIR）：打算换成哪种任务。
	TaskKind string `json:"task_kind"`
	// Loadout 只用于换挂载命令（SET_LOADOUT）。
	Loadout string `json:"loadout"`
	// Lane 只在新建打击/侦察编队时用：前端想把它放进面板的第几栏（1 起）。
	// 为空或那一栏已经被占时由服务端挑最小空位，所以这个字段是可选的偏好，不是约束。
	Lane int `json:"lane"`
	// Disperse 用于攻击编队目标分配（SET_STRIKE_DISPERSION）：true = 分散攻击，
	// 每个算子均匀随机选择编队中的任意一艘船；false = 集中攻击，按目标权重表选择（规则书 624–631 行）。
	Disperse bool `json:"disperse"`
	// Cycle 用于任务循环开关（SET_AIR_CYCLE）：true = 开启自动循环，false = 取消。
	// 语义是"设为"而不是"取反"，重复提交同一条命令不会来回翻转。
	Cycle bool `json:"cycle"`
	// Engagement 用于自动航行的交战设置（SET_AUTO_NAV / SET_ENGAGEMENT）：
	// aggressive=逢敌必战，advantage=优势交战，avoid=避免近战（规则书 362–370 行）。
	Engagement string `json:"engagement"`
}

type EngineEvent struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Audience string `json:"audience"`
}

func ParseCoord(coord string) (int, int, error) {
	if len(coord) != 4 {
		return 0, 0, fmt.Errorf("坐标必须为ABCD格式")
	}
	row, errRow := strconv.Atoi(coord[:2])
	col, errCol := strconv.Atoi(coord[2:])
	if errRow != nil || errCol != nil || row < 1 || col < 1 {
		return 0, 0, fmt.Errorf("无效hex坐标")
	}
	return row, col, nil
}

// Coord 把行列号拼成四字符 hex 坐标（行、列各两位，不足补零）。
//
// 没用 fmt.Sprintf：这是全模拟里调用最密的函数之一，侦察扇面、距离环、航迹展开
// 都在内层循环里按格调它。手写两位拼接比格式化快一个数量级，分配从 3 次降到 1 次。
// 只有两位装不下的越界行列（≥100）才退回格式化，保持原有的十进制输出。
func Coord(row, col int) string {
	if row < 100 && col < 100 && row >= 0 && col >= 0 {
		buf := [4]byte{
			byte('0' + row/10%10), byte('0' + row%10),
			byte('0' + col/10%10), byte('0' + col%10),
		}
		return string(buf[:])
	}
	return fmt.Sprintf("%02d%02d", row, col)
}

func Neighbors(coord string) []string {
	row, col, err := ParseCoord(coord)
	if err != nil {
		return nil
	}
	deltas := [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}
	if row%2 == 0 {
		deltas = append(deltas, [2]int{-1, -1}, [2]int{1, -1})
	} else {
		deltas = append(deltas, [2]int{-1, 1}, [2]int{1, 1})
	}
	result := make([]string, 0, 6)
	for _, delta := range deltas {
		r, c := row+delta[0], col+delta[1]
		if r > 0 && c > 0 {
			result = append(result, Coord(r, c))
		}
	}
	return result
}

func offsetCube(row, col int) (int, int, int) {
	zeroCol := col - 1
	zeroRow := row - 1
	x := zeroCol - (zeroRow+(zeroRow&1))/2
	z := zeroRow
	y := -x - z
	return x, y, z
}

// distanceRC 是 Distance 的整数版：已经拿到行列号时直接用，省掉两次坐标串解析。
// 扇面扫描、距离环、距离参照表都在内层循环里算距离，走这里能省掉每格一次 Atoi。
func distanceRC(ar, ac, br, bc int) int {
	ax, ay, az := offsetCube(ar, ac)
	bx, by, bz := offsetCube(br, bc)
	dx, dy, dz := ax-bx, ay-by, az-bz
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dz < 0 {
		dz = -dz
	}
	if dx > dy && dx > dz {
		return dx
	}
	if dy > dz {
		return dy
	}
	return dz
}

func Distance(a, b string) int {
	ar, ac, errA := ParseCoord(a)
	br, bc, errB := ParseCoord(b)
	if errA != nil || errB != nil {
		return 999
	}
	return distanceRC(ar, ac, br, bc)
}

func BuildDistanceReference(gameMap MapDefinition, origin string, maxDistance int) *DistanceReference {
	originRow, originCol, err := ParseCoord(origin)
	if err != nil || maxDistance < 1 {
		return nil
	}
	reference := &DistanceReference{Origin: origin, MaxDistance: maxDistance, Cells: []DistanceHex{}}
	// 距离用整数算：原来每格都把坐标拼成串再解析回去，一张图就是 2×3328 次 Atoi。
	for row := 1; row <= gameMap.Rows; row++ {
		for col := 1; col <= gameMap.Cols; col++ {
			distance := distanceRC(originRow, originCol, row, col)
			if distance <= maxDistance {
				reference.Cells = append(reference.Cells, DistanceHex{Coord: Coord(row, col), Distance: distance})
			}
		}
	}
	return reference
}

// Timeline 是剧本自带的作战时间线：一回合多少分钟、哪天几点开始整备/放飞/必须回收。
// 不同战役的窗口不一样（珊瑚海 04:00 整备、05:20 升空、19:40 全员回收），所以不能写死在引擎里。
type Timeline struct {
	TurnMinutes  int    `json:"turn_minutes"`
	ServiceStart string `json:"service_start"`
	LaunchStart  string `json:"launch_start"`
	RecoverBy    string `json:"recover_by"`
}
