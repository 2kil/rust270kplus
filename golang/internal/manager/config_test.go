package manager

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLegacyConfigDefaultsAndExplicitFalse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	text := "[general]\nenabled = false\n\n[[processes]]\nname = 'one.exe'\n\n[[processes]]\nname = 'two.exe'\nenabled = false\n"
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.General.Enabled || config.General.IntervalMS != 1000 || config.General.WindowWidth != 900 || config.General.WindowHeight != 650 || !config.General.MinimizeToTray {
		t.Fatalf("旧配置默认值错误：%+v", config.General)
	}
	if !config.Processes[0].Enabled || config.Processes[1].Enabled || config.Processes[0].Affinity != "all" || config.Processes[0].Priority != "normal" || config.Processes[0].MatchMode != "all_instances" {
		t.Fatalf("规则默认值错误：%+v", config.Processes)
	}
}

func TestConfigRoundTripAndClone(t *testing.T) {
	config := DefaultConfig()
	pathValue := `C:\Program Files\测试\demo.exe`
	config.Processes = []ProcessRule{{Name: "demo.exe", Enabled: true, Affinity: "cpu_list", CPUs: []int{0, 2}, Priority: "below_normal", MatchMode: "specific_path", Path: &pathValue}}
	path := filepath.Join(t.TempDir(), "config", "config.toml")
	for range 2 {
		if err := SaveConfig(path, config); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := LoadConfig(path)
	if err != nil || !reflect.DeepEqual(config, loaded) {
		t.Fatalf("配置读写不一致：%+v %v", loaded, err)
	}
	clone := CloneConfig(config)
	clone.Processes[0].CPUs[0] = 7
	*clone.Processes[0].Path = "changed"
	if config.Processes[0].CPUs[0] != 0 || *config.Processes[0].Path != pathValue {
		t.Fatal("克隆修改了原配置")
	}
}

func TestInvalidConfigIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.toml")
	if err := os.WriteFile(path, []byte("[general"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("损坏的 TOML 未报告错误")
	}
	config := DefaultConfig()
	config.Processes = []ProcessRule{{Name: "demo.exe", Enabled: true, Affinity: "cpu_list", CPUs: []int{-1}, Priority: "normal", MatchMode: "all_instances"}}
	if err := ValidateConfig(config, 8); err == nil {
		t.Fatal("负数 CPU 被接受")
	}
	config.Processes[0].CPUs = []int{8}
	if err := ValidateConfig(config, 8); err == nil {
		t.Fatal("越界 CPU 被接受")
	}
	config.Processes[0].CPUs = []int{7}
	if err := ValidateConfig(config, 8); err != nil {
		t.Fatal(err)
	}
	config.Processes[0].MatchMode = "specific_path"
	if err := ValidateConfig(config, 8); err == nil {
		t.Fatal("指定路径缺失被接受")
	}
	config.Processes[0].MatchMode = "all_instances"
	config.Processes[0].Priority = "invalid"
	if err := ValidateConfig(config, 8); err == nil {
		t.Fatal("未知优先级被接受")
	}
}

func TestWindowSizeClampsToSupportedRange(t *testing.T) {
	if w, h := ClampWindowSize(1, 1); w != 900 || h != 650 {
		t.Fatal(w, h)
	}
	if w, h := ClampWindowSize(5000, 5000); w != 3840 || h != 2160 {
		t.Fatal(w, h)
	}
}
