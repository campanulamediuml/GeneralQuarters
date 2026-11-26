package server

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"generalquarters/internal/store"
)

// 观战快照的性能基准。需要一份真实对局的数据库副本：
//
//	mkdir -p ../GeneralQuarterData/scratch
//	cp ../GeneralQuarterData/generalquarters.db* ../GeneralQuarterData/scratch/
//	GQ_BENCH_DB=../GeneralQuarterData/scratch/generalquarters.db \
//	GQ_BENCH_ROOM=<roomID> go test ./internal/server -bench Spectate -benchtime 20x \
//		-cpuprofile ../GeneralQuarterData/scratch/spectate.prof
//
// 之所以从副本起来，是因为基准要在库上做迁移（Open 会补列），不能对着在跑的服。
//
// 落盘约定：对局数据、库副本、profile、截图都放仓库外的 ../GeneralQuarterData
// （.gitignore 里已经忽略它），用完自己清；唯一例外是 Go 构建缓存 .gq/gocache，
// 它每次编译都在写，必须留在工作区内。不要写 /tmp——那里没人清理，
// 一次性能测试就能留下几十 GB，而且事后根本不知道是谁的。
func benchFixture(b *testing.B) (*store.Store, string) {
	b.Helper()
	path := os.Getenv("GQ_BENCH_DB")
	roomID := os.Getenv("GQ_BENCH_ROOM")
	if path == "" || roomID == "" {
		b.Skip("未设置 GQ_BENCH_DB / GQ_BENCH_ROOM，跳过观战快照基准")
	}
	st, err := store.Open(path)
	if err != nil {
		b.Fatalf("打开基准库失败: %v", err)
	}
	b.Cleanup(func() { _ = st.Close() })
	return st, roomID
}

func BenchmarkSpectateSnapshot(b *testing.B) {
	st, roomID := benchFixture(b)
	ctx := context.Background()
	room, err := st.Room(ctx, roomID)
	if err != nil {
		b.Fatalf("读取房间失败: %v", err)
	}
	players, err := st.Players(ctx, roomID)
	if err != nil {
		b.Fatalf("读取玩家失败: %v", err)
	}
	events, err := st.AllEvents(ctx, roomID)
	if err != nil {
		b.Fatalf("读取战报失败: %v", err)
	}

	// 1) 投影：把两边的合法视图合并成一份观战视图。
	b.Run("project", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if view := buildSpectateView(room, players, events); view == nil {
				b.Fatal("空视图")
			}
		}
	})

	// 2) 序列化：视图 + 状态一起编码成 JSON 字节。
	view := buildSpectateView(room, players, events)
	b.Run("marshal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			data, err := json.Marshal(view)
			if err != nil {
				b.Fatal(err)
			}
			if len(data) == 0 {
				b.Fatal("空输出")
			}
		}
	})

	// 3) 从库里读房间（含 state_json 反序列化）。
	b.Run("load_room", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := st.Room(ctx, roomID); err != nil {
				b.Fatal(err)
			}
		}
	})

	// 4) 完整一次请求：读库 + 投影 + 编码。
	b.Run("full", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			r, err := st.Room(ctx, roomID)
			if err != nil {
				b.Fatal(err)
			}
			p, err := st.Players(ctx, roomID)
			if err != nil {
				b.Fatal(err)
			}
			e, err := st.AllEvents(ctx, roomID)
			if err != nil {
				b.Fatal(err)
			}
			data, err := json.Marshal(buildSpectateView(r, p, e))
			if err != nil {
				b.Fatal(err)
			}
			if len(data) == 0 {
				b.Fatal("空输出")
			}
		}
	})
}

// 一次"推补丁"的成本拆开看：投影（含读库）、diff、编码。
// diff 这一项曾经是 11ms——两万条战报的列表里每个元素都在跑 FieldByNameFunc，
// 修完类型元数据缓存后是 0.4ms 量级；这条基准就是防它再爬回去。
func BenchmarkPrivateViewPush(b *testing.B) {
	st, roomID := benchFixture(b)
	ctx := context.Background()
	server := &Server{store: st}
	room, err := st.Room(ctx, roomID)
	if err != nil {
		b.Fatalf("读取房间失败: %v", err)
	}
	players, err := st.Players(ctx, roomID)
	if err != nil {
		b.Fatalf("读取玩家失败: %v", err)
	}
	if len(players) == 0 {
		b.Skip("房间没有玩家")
	}
	player := players[0]

	b.Run("view", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := server.privateView(ctx, room, players, player); err != nil {
				b.Fatal(err)
			}
		}
	})

	previous, err := server.privateView(ctx, room, players, player)
	if err != nil {
		b.Fatal(err)
	}
	next, err := server.privateView(ctx, room, players, player)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("diff", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if diffView(previous, next) != any(patchNone) {
				b.Fatal("同一份状态不该算出补丁")
			}
		}
	})

	b.Run("marshal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if data, err := json.Marshal(next); err != nil || len(data) == 0 {
				b.Fatal("编码失败")
			}
		}
	})
}
