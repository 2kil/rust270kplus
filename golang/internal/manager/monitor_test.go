package manager

import (
	"sync/atomic"
	"testing"
	"time"
)

type fakeProcesses struct {
	items  []ProcessInfo
	failed bool
	calls  atomic.Int32
	listed atomic.Int32
}

func (f *fakeProcesses) List() ([]ProcessInfo, error) { f.listed.Add(1); return f.items, nil }
func (f *fakeProcesses) Apply(info ProcessInfo, _ ProcessRule, _ int, _ bool) ApplyResult {
	f.calls.Add(1)
	return ApplyResult{PID: info.PID, OK: !f.failed, Message: "测试结果"}
}

func ruleConfig() Config {
	config := DefaultConfig()
	config.Processes = []ProcessRule{{Name: "demo.exe", Enabled: true, Affinity: "none", CPUs: []int{}, Priority: "normal", MatchMode: "all_instances"}}
	return config
}

func TestMonitorRetriesFailuresAndHandlesPIDReuse(t *testing.T) {
	source := &fakeProcesses{items: []ProcessInfo{{PID: 42, Name: "demo.exe", StartTime: 1}}, failed: true}
	m := NewMonitor(ruleConfig(), 8, true, source, func(Event) {})
	now := time.Now()
	m.scan(now)
	m.scan(now.Add(4 * time.Second))
	if source.calls.Load() != 1 {
		t.Fatal("失败后立即重试")
	}
	m.scan(now.Add(5 * time.Second))
	if source.calls.Load() != 2 {
		t.Fatal("失败后未在五秒重试")
	}
	source.failed = false
	m.scan(now.Add(10 * time.Second))
	m.scan(now.Add(11 * time.Second))
	if source.calls.Load() != 3 {
		t.Fatal("成功应用后未缓存")
	}
	source.items[0].StartTime = 2
	m.scan(now.Add(12 * time.Second))
	if source.calls.Load() != 4 {
		t.Fatal("PID 复用导致新进程被跳过")
	}
	source.items = nil
	m.scan(now.Add(13 * time.Second))
	if len(m.cache) != 0 || len(m.retryAfter) != 0 {
		t.Fatal("已退出进程未清理")
	}
}

func TestSameNameRulesRemainIndependent(t *testing.T) {
	config := ruleConfig()
	disabled := config.Processes[0]
	disabled.Enabled = false
	config.Processes = append(config.Processes, disabled, config.Processes[0])
	source := &fakeProcesses{items: []ProcessInfo{{PID: 42, Name: "demo.exe", StartTime: 1}}}
	m := NewMonitor(config, 8, true, source, func(Event) {})
	m.scan(time.Now())
	if source.calls.Load() != 2 || len(m.snapshots) != 2 || m.snapshots[1].Index != 2 {
		t.Fatal("同名规则或禁用规则的索引处理错误", m.snapshots)
	}
	m.UpdateConfig(config)
	m.scan(time.Now())
	if source.calls.Load() != 4 {
		t.Fatal("更新配置后未重新应用规则")
	}
}

func TestDisabledMonitoringDoesNotScanOnStartup(t *testing.T) {
	config := ruleConfig()
	config.General.Enabled = false
	source := &fakeProcesses{}
	m := NewMonitor(config, 8, true, source, func(Event) {})
	m.Start()
	m.ScanNow()
	m.Stop()
	if source.listed.Load() != 0 {
		t.Fatal("监控关闭时仍进行首次扫描")
	}
}

func TestPausedMonitoringWaitsUntilResume(t *testing.T) {
	source := &fakeProcesses{items: []ProcessInfo{{PID: 42, Name: "demo.exe", StartTime: 1}}}
	applied := make(chan struct{}, 4)
	m := NewMonitor(ruleConfig(), 8, true, source, func(event Event) {
		if event.Type == "Applied" {
			applied <- struct{}{}
		}
	})
	m.SetPaused(true)
	m.Start()
	m.ScanNow()
	if !m.Status().Paused || source.calls.Load() != 0 {
		t.Fatal("暂停状态不一致")
	}
	m.SetPaused(false)
	select {
	case <-applied:
	case <-time.After(time.Second):
		t.Fatal("恢复后未应用规则")
	}
	m.Stop()
}
