package manager

import (
	"fmt"
	"testing"
	"time"
)

func BenchmarkMatchingFirstInstance(b *testing.B) {
	items := make([]ProcessInfo, 1000)
	for i := range items {
		items[i] = ProcessInfo{PID: uint32(i + 1), Name: "demo.exe", StartTime: uint64(len(items) - i)}
	}
	rule := ProcessRule{Name: "DEMO.EXE", MatchMode: "first_instance"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		MatchingProcesses(items, rule)
	}
}

func BenchmarkMonitorCachedScan(b *testing.B) {
	config := ruleConfig()
	config.Processes = nil
	items := make([]ProcessInfo, 1000)
	for i := range items {
		items[i] = ProcessInfo{PID: uint32(i + 1), Name: fmt.Sprintf("process-%d.exe", i), StartTime: 1}
	}
	for i := range 100 {
		rule := ruleConfig().Processes[0]
		rule.Name = items[i].Name
		config.Processes = append(config.Processes, rule)
	}
	m := NewMonitor(config, 8, true, &fakeProcesses{items: items}, func(Event) {})
	now := time.Now()
	m.scan(now)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		m.scan(now)
	}
}

func BenchmarkWindowsEnumeration(b *testing.B) {
	source := WindowsProcesses{}
	for _, scenario := range []struct {
		name  string
		rules []ProcessRule
	}{
		{"NoMatch", []ProcessRule{{Name: "benchmark-no-match.exe", Enabled: true}}},
		{"OneName", []ProcessRule{{Name: "explorer.exe", Enabled: true}}},
		{"OnePath", []ProcessRule{{Name: "explorer.exe", Enabled: true, MatchMode: "specific_path"}}},
	} {
		b.Run(scenario.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := source.ListForRules(scenario.rules); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
	b.Run("Full", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := source.List(); err != nil {
				b.Fatal(err)
			}
		}
	})
}
