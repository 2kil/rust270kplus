package main

import (
	"path/filepath"
	"testing"

	"process-affinity-manager/internal/manager"
)

func testApp(t *testing.T) *App {
	config := manager.DefaultConfig()
	return &App{config: config, configPath: filepath.Join(t.TempDir(), "config.toml"), monitor: manager.NewMonitor(config, 8, true, manager.WindowsProcesses{}, func(manager.Event) {})}
}

func TestSaveConfigPreservesDesktopWindowSize(t *testing.T) {
	app := testApp(t)
	app.config.General.WindowWidth = 1200
	app.config.General.WindowHeight = 800
	config := manager.DefaultConfig()
	config.Processes = []manager.ProcessRule{{Name: "demo.exe", Enabled: true, Affinity: "none", CPUs: []int{}, Priority: "normal", MatchMode: "all_instances"}}
	if err := app.SaveConfig(config); err != nil {
		t.Fatal(err)
	}
	loaded, err := manager.LoadConfig(app.configPath)
	if err != nil || loaded.General.WindowWidth != 1200 || loaded.General.WindowHeight != 800 || len(loaded.Processes) != 1 {
		t.Fatal(loaded, err)
	}
	config.Processes[0].Name = "changed.exe"
	if app.GetConfig().Processes[0].Name != "demo.exe" {
		t.Fatal("调用者修改了已保存的规则")
	}
}

func TestInvalidSaveAndDeleteLeaveConfigIntact(t *testing.T) {
	app := testApp(t)
	config := app.GetConfig()
	config.General.IntervalMS = 1
	if app.SaveConfig(config) == nil || app.GetConfig().General.IntervalMS != 1000 {
		t.Fatal("无效配置破坏了原设置")
	}
	if app.DeleteProcessRule(-1) == nil || app.DeleteProcessRule(0) == nil {
		t.Fatal("无效索引未报告错误")
	}
}
