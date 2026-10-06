package manager

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

type General struct {
	Enabled          bool `json:"enabled" toml:"enabled"`
	IntervalMS       int  `json:"interval_ms" toml:"interval_ms"`
	StartWithWindows bool `json:"start_with_windows" toml:"start_with_windows"`
	MinimizeToTray   bool `json:"minimize_to_tray" toml:"minimize_to_tray"`
	WindowWidth      int  `json:"window_width" toml:"window_width"`
	WindowHeight     int  `json:"window_height" toml:"window_height"`
}

type ProcessRule struct {
	Name      string  `json:"name" toml:"name"`
	Enabled   bool    `json:"enabled" toml:"enabled"`
	Affinity  string  `json:"affinity" toml:"affinity"`
	CPUs      []int   `json:"cpus" toml:"cpus"`
	Priority  string  `json:"priority" toml:"priority"`
	MatchMode string  `json:"match_mode" toml:"match_mode"`
	Path      *string `json:"path" toml:"path,omitempty"`
}

type Config struct {
	General   General       `json:"general" toml:"general"`
	Processes []ProcessRule `json:"processes" toml:"processes"`
}

func DefaultConfig() Config {
	return Config{General: General{Enabled: true, IntervalMS: 1000, MinimizeToTray: true, WindowWidth: 900, WindowHeight: 650}, Processes: []ProcessRule{}}
}

func ConfigPath() string {
	base := os.Getenv("APPDATA")
	if base == "" {
		base, _ = os.UserConfigDir()
	}
	return filepath.Join(base, "ProcessAffinityManager", "config", "config.toml")
}

func LogPath(configPath string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(configPath)), "logs", "app.log")
}

func LoadConfig(path string) (Config, error) {
	config := DefaultConfig()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return config, nil
	}
	if err != nil {
		return config, err
	}
	// 原版缺失的规则字段采用 serde 默认值，显式 false 必须保留。
	var raw struct {
		General   General `toml:"general"`
		Processes []struct {
			Name      string  `toml:"name"`
			Enabled   *bool   `toml:"enabled"`
			Affinity  string  `toml:"affinity"`
			CPUs      []int   `toml:"cpus"`
			Priority  string  `toml:"priority"`
			MatchMode string  `toml:"match_mode"`
			Path      *string `toml:"path"`
		} `toml:"processes"`
	}
	raw.General = config.General
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return config, fmt.Errorf("配置解析失败：%w", err)
	}
	config.General = raw.General
	for _, item := range raw.Processes {
		rule := ProcessRule{Name: item.Name, Enabled: true, Affinity: "all", CPUs: []int{}, Priority: "normal", MatchMode: "all_instances", Path: item.Path}
		if item.Enabled != nil {
			rule.Enabled = *item.Enabled
		}
		if item.Affinity != "" {
			rule.Affinity = item.Affinity
		}
		if item.CPUs != nil {
			rule.CPUs = item.CPUs
		}
		if item.Priority != "" {
			rule.Priority = item.Priority
		}
		if item.MatchMode != "" {
			rule.MatchMode = item.MatchMode
		}
		config.Processes = append(config.Processes, rule)
	}
	config.General.WindowWidth, config.General.WindowHeight = ClampWindowSize(config.General.WindowWidth, config.General.WindowHeight)
	return config, nil
}

func SaveConfig(path string, config Config) error {
	var data bytes.Buffer
	if err := toml.NewEncoder(&data).Encode(config); err != nil {
		return err
	}
	return writeFile(path, data.Bytes())
}

// 同目录临时文件替换，写入失败时保留上一份配置和日志。
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".affinity-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return replaceFile(file.Name(), path)
}

func ClampWindowSize(width, height int) (int, int) {
	return min(max(width, 900), 3840), min(max(height, 650), 2160)
}

func CloneConfig(config Config) Config {
	config.Processes = slices.Clone(config.Processes)
	if config.Processes == nil {
		config.Processes = []ProcessRule{}
	}
	for i := range config.Processes {
		config.Processes[i].CPUs = append([]int{}, config.Processes[i].CPUs...)
		if config.Processes[i].Path != nil {
			value := *config.Processes[i].Path
			config.Processes[i].Path = &value
		}
	}
	return config
}

func ValidateConfig(config Config, cpuCount int) error {
	var problems []string
	if !slices.Contains([]int{250, 500, 1000, 2000, 5000}, config.General.IntervalMS) {
		problems = append(problems, "监控间隔必须是 250、500、1000、2000 或 5000 毫秒")
	}
	for _, rule := range config.Processes {
		if strings.TrimSpace(rule.Name) == "" {
			problems = append(problems, "进程名不能为空")
		}
		if !slices.Contains([]string{"all", "last_cpu", "cpu_list", "none"}, rule.Affinity) {
			problems = append(problems, rule.Name+"：亲和性模式无效")
		}
		if !slices.Contains([]string{"idle", "below_normal", "normal", "above_normal", "high", "realtime"}, rule.Priority) {
			problems = append(problems, rule.Name+"：优先级无效")
		}
		if !slices.Contains([]string{"all_instances", "first_instance", "specific_path"}, rule.MatchMode) {
			problems = append(problems, rule.Name+"：匹配方式无效")
		}
		if rule.Affinity == "cpu_list" {
			if len(rule.CPUs) == 0 {
				problems = append(problems, rule.Name+"：CPU 列表不能为空")
			}
			for _, cpu := range rule.CPUs {
				if cpu < 0 || cpu >= min(cpuCount, 64) {
					problems = append(problems, fmt.Sprintf("%s：CPU %d 不存在或超出支持范围", rule.Name, cpu))
				}
			}
		}
		if rule.MatchMode == "specific_path" && (rule.Path == nil || strings.TrimSpace(*rule.Path) == "") {
			problems = append(problems, rule.Name+"：指定路径不能为空")
		}
	}
	if len(problems) != 0 {
		return fmt.Errorf("%s", strings.Join(problems, "\n"))
	}
	return nil
}
