package manager

import (
	"reflect"
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

func TestIndexedMonitorMatchesLinearRulesAndRebuilds(t *testing.T) {
	path := `C:\Apps\demo.exe`
	source := &fakeProcesses{items: []ProcessInfo{
		{PID: 5, Name: "demo.exe", StartTime: 2, Path: &path},
		{PID: 4, Name: "DEMO.EXE", StartTime: 1},
		{PID: 3, Name: "K.exe", StartTime: 1},
		{PID: 2, Name: "other.exe", StartTime: 1},
	}}
	config := ruleConfig()
	for _, rule := range []ProcessRule{
		{Name: "demo.exe", Enabled: true, MatchMode: "first_instance"},
		{Name: "demo.exe", Enabled: true, MatchMode: "specific_path", Path: &path},
		{Name: "k.exe", Enabled: true, MatchMode: "all_instances"},
		{Name: "other.exe", Enabled: false},
	} {
		config.Processes = append(config.Processes, rule)
	}
	m := NewMonitor(config, 8, true, source, func(Event) {})
	check := func() {
		t.Helper()
		m.scan(time.Now())
		var want []RuleSnapshot
		for i, rule := range config.Processes {
			if !rule.Enabled {
				continue
			}
			matches := MatchingProcesses(source.items, rule)
			snapshot := RuleSnapshot{Index: i, Name: rule.Name, Running: len(matches) > 0, PIDs: []uint32{}}
			for _, item := range matches {
				snapshot.PIDs = append(snapshot.PIDs, item.PID)
			}
			want = append(want, snapshot)
		}
		if !reflect.DeepEqual(m.Snapshots(), want) {
			t.Fatal("索引匹配与线性匹配不一致", m.Snapshots(), want)
		}
	}
	check()
	source.items = source.items[2:]
	check()
	config.Processes = []ProcessRule{{Name: "OTHER.EXE", Enabled: true}}
	m.UpdateConfig(config)
	check()
	if len(m.processIndex) != 1 {
		t.Fatal("旧规则索引未释放")
	}
}

type filteredProcesses struct {
	fakeProcesses
	filteredCalls int
}

func (f *filteredProcesses) ListForRules([]ProcessRule) ([]ProcessInfo, error) {
	f.filteredCalls++
	return f.items, nil
}

func TestMonitorUsesRuleFilteredSource(t *testing.T) {
	source := &filteredProcesses{}
	config := ruleConfig()
	m := NewMonitor(config, 8, true, source, func(Event) {})
	m.scan(time.Now())
	if source.filteredCalls != 1 || source.listed.Load() != 0 {
		t.Fatal("未使用规则筛选枚举")
	}
	config.Processes[0].Enabled = false
	m.UpdateConfig(config)
	m.scan(time.Now())
	if source.filteredCalls != 1 {
		t.Fatal("没有启用的规则时仍枚举进程")
	}
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
