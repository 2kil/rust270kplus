package main

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlantern/systray"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"process-affinity-manager/internal/manager"
)

type App struct {
	ctx              context.Context
	mu               sync.Mutex
	config           manager.Config
	configError      *string
	configPath       string
	logs             *manager.LogStore
	monitor          *manager.Monitor
	permissionDenied atomic.Bool
	trayReady        atomic.Bool
	quitting         atomic.Bool
	trayDone         chan struct{}
	trayStarted      chan struct{}
}

func NewApp() *App {
	path := manager.ConfigPath()
	config, err := manager.LoadConfig(path)
	app := &App{config: config, configPath: path, logs: manager.NewLogStore(manager.LogPath(path)), trayDone: make(chan struct{}), trayStarted: make(chan struct{})}
	if err == nil {
		cpuCount, _ := manager.ProcessorInfo()
		err = manager.ValidateConfig(config, cpuCount)
	}
	if err != nil {
		message := err.Error()
		app.configError = &message
		app.config = manager.DefaultConfig()
	}
	cpuCount, singleGroup := manager.ProcessorInfo()
	app.monitor = manager.NewMonitor(app.config, cpuCount, singleGroup, manager.WindowsProcesses{}, app.onMonitorEvent)
	return app
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if a.configError != nil {
		_ = a.logs.Append("配置加载失败，已使用默认配置：" + *a.configError)
	}
	if err := manager.EnableDebugPrivilege(); err != nil {
		_ = a.logs.Append("无法启用调试权限：" + err.Error())
	}
	a.monitor.Start()
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(a.trayDone)
		systray.Run(a.setupTray, func() { a.trayReady.Store(false) })
	}()
}

func (a *App) onMonitorEvent(event manager.Event) {
	switch data := event.Data.(type) {
	case manager.AppliedEvent:
		if err := a.logs.Append(fmt.Sprintf("%s PID=%d %s", data.RuleName, data.Result.PID, data.Result.Message)); err != nil {
			wailsruntime.LogError(a.ctx, "写入日志失败："+err.Error())
		}
		if !data.Result.OK && strings.Contains(data.Result.Message, "错误码 5") {
			a.permissionDenied.Store(true)
		}
	case manager.StatusEvent:
		if err := a.logs.Append(data.Message); err != nil {
			wailsruntime.LogError(a.ctx, "写入日志失败："+err.Error())
		}
	}
	wailsruntime.EventsEmit(a.ctx, "monitor-event", event)
}

func (a *App) GetConfig() manager.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return manager.CloneConfig(a.config)
}

func (a *App) saveConfigLocked(config manager.Config) error {
	if err := manager.ValidateConfig(config, a.monitor.Status().LogicalCPUs); err != nil {
		return err
	}
	config = manager.CloneConfig(config)
	config.General.WindowWidth, config.General.WindowHeight = manager.ClampWindowSize(config.General.WindowWidth, config.General.WindowHeight)
	if err := manager.SaveConfig(a.configPath, config); err != nil {
		return err
	}
	a.config = config
	a.configError = nil
	a.monitor.UpdateConfig(config)
	return nil
}

func (a *App) SaveConfig(config manager.Config) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	// 窗口大小由桌面窗口维护，避免前端旧数据覆盖最近的尺寸。
	config.General.WindowWidth = a.config.General.WindowWidth
	config.General.WindowHeight = a.config.General.WindowHeight
	if config.General.StartWithWindows != a.config.General.StartWithWindows {
		return fmt.Errorf("请通过开机启动开关修改启动设置")
	}
	return a.saveConfigLocked(config)
}

func (a *App) DeleteProcessRule(index int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if index < 0 || index >= len(a.config.Processes) {
		return fmt.Errorf("规则索引无效")
	}
	config := manager.CloneConfig(a.config)
	config.Processes = append(config.Processes[:index], config.Processes[index+1:]...)
	return a.saveConfigLocked(config)
}

func (a *App) SetStartWithWindows(enabled bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := manager.SetStartWithWindows(enabled); err != nil {
		return err
	}
	config := manager.CloneConfig(a.config)
	config.General.StartWithWindows = enabled
	if err := a.saveConfigLocked(config); err != nil {
		if rollbackErr := manager.SetStartWithWindows(a.config.General.StartWithWindows); rollbackErr != nil {
			return fmt.Errorf("保存配置失败：%v；恢复开机启动失败：%v", err, rollbackErr)
		}
		return err
	}
	return nil
}

func (a *App) GetRuntimeStatus() manager.RuntimeStatus {
	status := a.monitor.Status()
	a.mu.Lock()
	defer a.mu.Unlock()
	status.ConfigError = a.configError
	status.PermissionDenied = a.permissionDenied.Load()
	return status
}

func (a *App) ListRunningProcesses() ([]manager.ProcessInfo, error) {
	return (manager.WindowsProcesses{}).List()
}
func (a *App) GetRuleSnapshots() []manager.RuleSnapshot { return a.monitor.Snapshots() }
func (a *App) GetLogs() ([]string, error)               { return a.logs.Read() }
func (a *App) ClearLogs() error                         { return a.logs.Clear() }
func (a *App) ScanNow()                                 { a.monitor.ScanNow() }
func (a *App) ApplyAll()                                { a.monitor.ScanNow() }
func (a *App) SetMonitorPaused(paused bool)             { a.monitor.SetPaused(paused) }

func (a *App) SaveWindowSize() error {
	if wailsruntime.WindowIsMaximised(a.ctx) || wailsruntime.WindowIsMinimised(a.ctx) {
		return nil
	}
	width, height := wailsruntime.WindowGetSize(a.ctx)
	width, height = manager.ClampWindowSize(width, height)
	a.mu.Lock()
	defer a.mu.Unlock()
	if width == a.config.General.WindowWidth && height == a.config.General.WindowHeight {
		return nil
	}
	config := manager.CloneConfig(a.config)
	config.General.WindowWidth, config.General.WindowHeight = width, height
	if err := manager.SaveConfig(a.configPath, config); err != nil {
		return err
	}
	a.config = config
	return nil
}

func (a *App) beforeClose(ctx context.Context) bool {
	if a.quitting.Load() {
		return false
	}
	if err := a.SaveWindowSize(); err != nil {
		wailsruntime.LogError(ctx, "保存窗口尺寸失败："+err.Error())
	}
	if a.GetConfig().General.MinimizeToTray && a.trayReady.Load() {
		wailsruntime.WindowHide(ctx)
		return true
	}
	return false
}

func (a *App) shutdown(context.Context) {
	a.monitor.Stop()
	// 启动后立即退出时，等待托盘窗口建立，避免发送退出消息给空句柄。
	select {
	case <-a.trayStarted:
		systray.Quit()
	case <-a.trayDone:
		return
	case <-time.After(time.Second):
		return
	}
	select {
	case <-a.trayDone:
	case <-time.After(time.Second):
	}
}

func (a *App) setupTray() {
	systray.SetIcon(trayIcon)
	systray.SetTooltip("进程亲和性管理器")
	show := systray.AddMenuItem("显示主界面", "恢复主窗口")
	apply := systray.AddMenuItem("立即应用", "重新扫描并应用全部规则")
	pause := systray.AddMenuItem("暂停监控", "暂停自动应用")
	resume := systray.AddMenuItem("恢复监控", "恢复自动应用")
	startup := systray.AddMenuItem("切换开机启动", "修改当前用户的启动项")
	systray.AddSeparator()
	quit := systray.AddMenuItem("退出", "退出程序")
	a.trayReady.Store(true)
	close(a.trayStarted)
	for {
		select {
		case <-show.ClickedCh:
			wailsruntime.WindowShow(a.ctx)
			wailsruntime.WindowUnminimise(a.ctx)
		case <-apply.ClickedCh:
			a.ScanNow()
		case <-pause.ClickedCh:
			a.SetMonitorPaused(true)
		case <-resume.ClickedCh:
			a.SetMonitorPaused(false)
		case <-startup.ClickedCh:
			if err := a.SetStartWithWindows(!a.GetConfig().General.StartWithWindows); err != nil {
				_ = a.logs.Append("切换开机启动失败：" + err.Error())
				wailsruntime.EventsEmit(a.ctx, "app-error", err.Error())
			} else {
				wailsruntime.EventsEmit(a.ctx, "config-changed", a.GetConfig())
			}
		case <-quit.ClickedCh:
			a.quitting.Store(true)
			wailsruntime.Quit(a.ctx)
			return
		case <-a.trayDone:
			return
		}
	}
}
