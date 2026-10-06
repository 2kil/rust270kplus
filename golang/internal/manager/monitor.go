package manager

import (
	"reflect"
	"sync"
	"time"
)

type ProcessSource interface {
	List() ([]ProcessInfo, error)
	Apply(ProcessInfo, ProcessRule, int, bool) ApplyResult
}

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type AppliedEvent struct {
	RuleName string      `json:"rule_name"`
	Result   ApplyResult `json:"result"`
}

type StatusEvent struct {
	Message string `json:"message"`
	Paused  bool   `json:"paused"`
}

type RuntimeStatus struct {
	Enabled                 bool    `json:"enabled"`
	Paused                  bool    `json:"paused"`
	LogicalCPUs             int     `json:"logical_cpus"`
	ProcessorGroupSupported bool    `json:"processor_group_supported"`
	PermissionDenied        bool    `json:"permission_denied"`
	ConfigError             *string `json:"config_error"`
}

type processKey struct {
	rule    int
	pid     uint32
	name    string
	started uint64
}

type Monitor struct {
	mu         sync.RWMutex
	config     Config
	status     RuntimeStatus
	snapshots  []RuleSnapshot
	source     ProcessSource
	emit       func(Event)
	cache      map[processKey]bool
	retryAfter map[processKey]time.Time
	wake       chan struct{}
	stop       chan struct{}
	done       chan struct{}
}

func NewMonitor(config Config, cpuCount int, singleGroup bool, source ProcessSource, emit func(Event)) *Monitor {
	return &Monitor{config: CloneConfig(config), status: RuntimeStatus{Enabled: config.General.Enabled, LogicalCPUs: cpuCount, ProcessorGroupSupported: singleGroup}, snapshots: []RuleSnapshot{}, source: source, emit: emit, cache: map[processKey]bool{}, retryAfter: map[processKey]time.Time{}, wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
}

func (m *Monitor) Start() { go m.run() }

func (m *Monitor) run() {
	defer close(m.done)
	for {
		m.mu.Lock()
		if m.config.General.Enabled && !m.status.Paused {
			m.scan(time.Now())
		}
		interval := time.Duration(min(max(m.config.General.IntervalMS, 250), 5000)) * time.Millisecond
		m.mu.Unlock()
		timer := time.NewTimer(interval)
		select {
		case <-m.stop:
			timer.Stop()
			return
		case <-m.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (m *Monitor) scan(now time.Time) {
	items := []ProcessInfo{}
	for _, rule := range m.config.Processes {
		if rule.Enabled {
			var err error
			items, err = m.source.List()
			if err != nil {
				m.emit(Event{Type: "Status", Data: StatusEvent{Message: "读取进程失败：" + err.Error(), Paused: m.status.Paused}})
				return
			}
			break
		}
	}
	snapshots := []RuleSnapshot{}
	live := map[processKey]bool{}
	for index, rule := range m.config.Processes {
		if !rule.Enabled {
			continue
		}
		matches := MatchingProcesses(items, rule)
		snapshot := RuleSnapshot{Index: index, Name: rule.Name, Running: len(matches) > 0, PIDs: []uint32{}}
		for _, item := range matches {
			snapshot.PIDs = append(snapshot.PIDs, item.PID)
			key := processKey{index, item.PID, rule.Name, item.StartTime}
			live[key] = true
			if m.cache[key] || now.Before(m.retryAfter[key]) {
				continue
			}
			result := m.source.Apply(item, rule, m.status.LogicalCPUs, m.status.ProcessorGroupSupported)
			if result.OK {
				// 无法读取启动时间时不缓存，避免 PID 复用跳过新进程。
				if item.StartTime != 0 {
					m.cache[key] = true
				}
				delete(m.retryAfter, key)
			} else {
				m.retryAfter[key] = now.Add(5 * time.Second)
			}
			m.emit(Event{Type: "Applied", Data: AppliedEvent{RuleName: rule.Name, Result: result}})
		}
		snapshots = append(snapshots, snapshot)
	}
	for key := range m.cache {
		if !live[key] {
			delete(m.cache, key)
		}
	}
	for key := range m.retryAfter {
		if !live[key] {
			delete(m.retryAfter, key)
		}
	}
	if !reflect.DeepEqual(snapshots, m.snapshots) {
		m.snapshots = snapshots
		m.emit(Event{Type: "Snapshot", Data: struct {
			Rules []RuleSnapshot `json:"rules"`
		}{Rules: snapshots}})
	}
}

func (m *Monitor) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Monitor) UpdateConfig(config Config) {
	m.mu.Lock()
	m.config = CloneConfig(config)
	m.status.Enabled = config.General.Enabled
	m.cache = map[processKey]bool{}
	m.retryAfter = map[processKey]time.Time{}
	m.mu.Unlock()
	m.signal()
}

func (m *Monitor) ScanNow() {
	m.mu.Lock()
	m.cache = map[processKey]bool{}
	m.retryAfter = map[processKey]time.Time{}
	m.mu.Unlock()
	m.signal()
}

func (m *Monitor) SetPaused(paused bool) {
	m.mu.Lock()
	m.status.Paused = paused
	m.cache = map[processKey]bool{}
	m.retryAfter = map[processKey]time.Time{}
	message := "监控已恢复"
	if paused {
		message = "监控已暂停"
	}
	m.emit(Event{Type: "Status", Data: StatusEvent{Message: message, Paused: paused}})
	m.mu.Unlock()
	m.signal()
}

func (m *Monitor) Status() RuntimeStatus { m.mu.RLock(); defer m.mu.RUnlock(); return m.status }

func (m *Monitor) Snapshots() []RuleSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := append([]RuleSnapshot{}, m.snapshots...)
	for i := range result {
		result[i].PIDs = append([]uint32{}, result[i].PIDs...)
	}
	return result
}

func (m *Monitor) Stop() { close(m.stop); <-m.done }
