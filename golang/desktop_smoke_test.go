//go:build desktop_smoke

package main

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"process-affinity-manager/internal/manager"
)

func TestDesktopSmoke(t *testing.T) {
	base := t.TempDir()
	config := manager.DefaultConfig()
	config.General.Enabled = false
	a := &App{config: config, configPath: filepath.Join(base, "config", "config.toml"), logs: manager.NewLogStore(filepath.Join(base, "logs", "app.log")), trayDone: make(chan struct{}), trayStarted: make(chan struct{})}
	var reported atomic.Bool
	webAssets := fstest.MapFS{}
	err := fs.WalkDir(assets, "frontend/dist", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			data, err := fs.ReadFile(assets, path)
			if err != nil {
				return err
			}
			webAssets[strings.TrimPrefix(path, "frontend/dist/")] = &fstest.MapFile{Data: data}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	index := webAssets["index.html"]
	if index == nil {
		t.Fatal("请先运行 pnpm --dir frontend build，生成桌面测试所需页面")
	}
	index.Data = []byte(strings.Replace(string(index.Data), "</body>", "<script>"+smokeScript+"</script></body>", 1))
	count, singleGroup := manager.ProcessorInfo()
	a.monitor = manager.NewMonitor(config, count, singleGroup, manager.WindowsProcesses{}, a.onMonitorEvent)
	err = wails.Run(&options.App{
		Title: "迁移验证", Width: 900, Height: 650, MinWidth: 900, MinHeight: 650, StartHidden: true,
		AssetServer: &assetserver.Options{Assets: webAssets}, Bind: []interface{}{a},
		Windows: &windows.Options{Theme: windows.Light, WebviewUserDataPath: filepath.Join(base, "webview")},
		OnStartup: func(ctx context.Context) {
			a.startup(ctx)
			wailsruntime.EventsOn(ctx, "smoke-seed", func(...interface{}) {
				for i := 0; i < 105; i++ {
					a.onMonitorEvent(manager.Event{Type: "Status", Data: manager.StatusEvent{Message: fmt.Sprintf("桌面日志 %d", i), Paused: false}})
				}
			})
			wailsruntime.EventsOn(ctx, "smoke-result", func(data ...interface{}) {
				reported.Store(true)
				if len(data) != 1 || data[0] != "通过" {
					t.Errorf("前端验证失败：%v", data)
				}
				if !a.beforeClose(ctx) {
					t.Error("关闭窗口未隐藏到托盘")
				}
				a.quitting.Store(true)
				wailsruntime.Quit(ctx)
			})
			wailsruntime.EventsOn(ctx, "smoke-close", func(...interface{}) { a.quitting.Store(true); wailsruntime.Quit(ctx) })
			wailsruntime.EventsOn(ctx, "smoke-hide", func(...interface{}) {
				if !a.beforeClose(ctx) {
					t.Error("关闭窗口未隐藏到托盘")
				}
			})
			go func() { time.Sleep(45 * time.Second); a.quitting.Store(true); wailsruntime.Quit(ctx) }()
		},
		OnDomReady: func(ctx context.Context) {
			select {
			case <-a.trayStarted:
				t.Log("WebView2 与托盘已就绪")
			case <-time.After(5 * time.Second):
				t.Error("托盘未完成初始化")
				a.quitting.Store(true)
				wailsruntime.Quit(ctx)
			}
		},
		OnBeforeClose: a.beforeClose, OnShutdown: a.shutdown,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reported.Load() {
		t.Fatal("前端未完成调用和事件验证")
	}
}

const smokeScript = `
(async () => {
  const assert = (value, message) => { if (!value) throw new Error(message); };
  const wait = async (check) => { for (let i=0; i<100; i++) { if (await check()) return; await new Promise(resolve => setTimeout(resolve, 100)); } throw new Error("等待超时"); };
  try {
    await wait(() => document.querySelector("#save-rule"));
    const api = window.go.main.App;
    const config = await api.GetConfig();
    assert(!config.general.enabled, "测试自动监控未关闭");
    const status = await api.GetRuntimeStatus();
    assert(status.logical_cpus > 0, "逻辑 CPU 数错误");
    assert(Array.isArray(await api.ListRunningProcesses()), "进程枚举调用失败");
    document.querySelector('[data-tab="add"]').click();
    document.querySelector('#save-rule').click();
    await wait(() => document.querySelector('#form-error').textContent === "请输入进程名称");
    const name = document.querySelector('#field-name');
    name.value = "migration-ui-validation-20261004.exe";
    name.dispatchEvent(new Event('input', {bubbles:true}));
    document.querySelector('input[name="affinity"][value="cpu_list"]').click();
    document.querySelector('#select-all-cpu-toggle').click();
    const cpuCount = Math.min(status.logical_cpus, 64);
    assert(document.querySelectorAll('#cpu-grid input:checked').length === cpuCount, "全选 CPU 失败");
    const editor = document.querySelector('.editor-scroll');
    assert(editor.scrollHeight <= editor.clientHeight, "添加页面出现纵向溢出");
    const save = document.querySelector('#save-rule').getBoundingClientRect();
    assert(save.bottom <= innerHeight && save.right <= innerWidth, "保存按钮超出窗口");
    document.querySelector('#save-rule').click();
    await wait(async () => (await api.GetConfig()).processes.length === 1);
    const rule = (await api.GetConfig()).processes[0];
    assert(rule.name === name.value && rule.cpus.length === cpuCount && rule.priority === "normal", "规则保存内容错误");
    document.querySelector('[data-tab="logs"]').click();
    window.runtime.EventsEmit("smoke-seed");
    await wait(() => document.querySelector('#log-output').textContent.endsWith("桌面日志 104"));
    const lines = document.querySelector('#log-output').textContent.split('\n');
    assert(lines.length === 100 && lines[0].endsWith("桌面日志 5"), "实时日志上限或顺序错误");
    assert((await api.GetLogs()).length === 100, "持久日志上限错误");
    document.querySelector('#clear-log-btn').click();
    await wait(async () => (await api.GetLogs()).length === 0);
    await api.DeleteProcessRule(0);
    assert((await api.GetConfig()).processes.length === 0, "规则删除失败");
    window.runtime.EventsEmit("smoke-result", "通过");
  } catch (error) { window.runtime.EventsEmit("smoke-result", String(error)); }
})();
`
