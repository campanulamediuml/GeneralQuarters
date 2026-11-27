// Package config 把运行配置从「默认值 → 配置文件 → 环境变量」叠出来。
//
// 优先级：默认值 < 配置文件 < 环境变量。容器里改环境变量比改挂在卷里的文件方便，
// 所以环境变量放最上面；同时保留纯环境变量启动的方式，老的 docker run 不受影响。
//
// 配置文件路径的查找顺序：命令行 -config > 环境变量 GQ_CONFIG > ./generalquarters.json（存在才读）。
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

const defaultConfigPath = "generalquarters.json"

// Config 是进程的完整配置。字段都用下划线的 JSON 名，和 API 里其它 JSON 保持一致。
type Config struct {
	// DataDir 是对局数据库、备份和 content 副本所在目录。
	DataDir string `json:"data_dir"`
	// DBPath 不给就取 DataDir/generalquarters.db。
	DBPath string `json:"db_path"`
	// Listen 是 HTTP 监听地址，例如 :8080 或 127.0.0.1:8080。
	Listen string `json:"listen"`
	// SecureCookie 打开后会话 Cookie 带 Secure（放在 HTTPS 反代后面才开）。
	SecureCookie bool `json:"secure_cookie"`
	// BackupKeep 是启动备份保留份数。
	BackupKeep int `json:"backup_keep"`
	// WebDir 指到前端产物目录（默认 web/dist）：前端是纯静态资源，一律从磁盘读，
	// 改完 npm run build 刷新即可，不用重新编译、也不用重启服务。目录不存在会拒绝启动。
	WebDir string `json:"web_dir"`
	// Source 记录实际读到的配置文件路径（空表示只用默认值 + 环境变量），仅用于启动日志。
	Source string `json:"-"`
}

// Default 返回不带任何外部输入时的配置。
func Default() Config {
	return Config{
		DataDir:    "./data",
		Listen:     ":8080",
		BackupKeep: 3,
	}
}

// Load 读配置并补齐派生值。path 为空时按 GQ_CONFIG、./generalquarters.json 的顺序找。
func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		path = os.Getenv("GQ_CONFIG")
	}
	if path == "" {
		if _, err := os.Stat(defaultConfigPath); err == nil {
			path = defaultConfigPath
		}
	}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("读取配置文件失败：%w", err)
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		// 严格解码：配置里写错字段名要立刻报错，而不是默默忽略、跑默认值。
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&cfg); err != nil {
			return cfg, fmt.Errorf("配置文件 %s 无效：%w", path, err)
		}
		cfg.Source = path
	}
	applyEnv(&cfg)
	if cfg.DBPath == "" {
		cfg.DBPath = filepath.Join(cfg.DataDir, "generalquarters.db")
	}
	// backup_keep = 0 表示不备份（容器里数据目录本身是卷，可能不需要额外快照）；负数按 0 处理。
	if cfg.BackupKeep < 0 {
		cfg.BackupKeep = 0
	}
	return cfg, nil
}

// ContentDir 是静态内容副本目录：服务端启动时把内嵌 content 同步到这里。
func (c Config) ContentDir() string { return filepath.Join(c.DataDir, "content") }

func applyEnv(cfg *Config) {
	// 数据目录被环境变量改掉时，库文件必须跟着走：否则配置里写死的 db_path 还指着旧目录，
	// 跑起来就是两个库。显式的 GQ_DB_PATH 仍然优先于这个派生。
	dataDirMoved := false
	if value := os.Getenv("GQ_DATA_DIR"); value != "" {
		cfg.DataDir = value
		dataDirMoved = true
	}
	if value := os.Getenv("GQ_DB_PATH"); value != "" {
		cfg.DBPath = value
	} else if dataDirMoved {
		cfg.DBPath = ""
	}
	if value := os.Getenv("GQ_LISTEN"); value != "" {
		cfg.Listen = value
	}
	// PORT 是容器里最常见的写法，保持兼容。
	if port := os.Getenv("PORT"); port != "" {
		cfg.Listen = ":" + port
	}
	if value := os.Getenv("GQ_SECURE_COOKIE"); value != "" {
		cfg.SecureCookie = value == "true" || value == "1"
	}
	if value := os.Getenv("GQ_BACKUP_KEEP"); value != "" {
		if number, err := strconv.Atoi(value); err == nil {
			cfg.BackupKeep = number
		}
	}
	// 开发时用环境变量开"前端从磁盘读"，不用把 web_dir 写进配置文件、免得误提交。
	if value := os.Getenv("GQ_WEB_DIR"); value != "" {
		cfg.WebDir = value
	}
}
