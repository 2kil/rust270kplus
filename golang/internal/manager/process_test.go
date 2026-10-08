package manager

import (
	"reflect"
	"strings"
	"testing"
	"unicode"
)

func TestAffinityMasksAndBoundaries(t *testing.T) {
	for _, test := range []struct {
		rule    ProcessRule
		count   int
		mask    uint64
		invalid bool
	}{
		{ProcessRule{Affinity: "all"}, 64, ^uint64(0), false},
		{ProcessRule{Affinity: "last_cpu"}, 8, 128, false},
		{ProcessRule{Affinity: "cpu_list", CPUs: []int{0, 2, 4, 6}}, 8, 0b01010101, false},
		{ProcessRule{Affinity: "cpu_list", CPUs: []int{63}}, 64, uint64(1) << 63, false},
		{ProcessRule{Affinity: "all"}, 65, 0, true},
		{ProcessRule{Affinity: "last_cpu"}, 0, 0, true},
		{ProcessRule{Affinity: "cpu_list", CPUs: []int{}}, 8, 0, true},
		{ProcessRule{Affinity: "cpu_list", CPUs: []int{-1}}, 8, 0, true},
		{ProcessRule{Affinity: "cpu_list", CPUs: []int{8}}, 8, 0, true},
	} {
		mask, err := AffinityMask(test.rule, test.count)
		if (err != nil) != test.invalid || mask != test.mask {
			t.Fatalf("%+v：mask=%d err=%v", test, mask, err)
		}
	}
}

func TestProcessNameKeyPreservesUnicodeFold(t *testing.T) {
	for r := rune(0); r <= unicode.MaxRune; r++ {
		next := unicode.SimpleFold(r)
		if next != r && processNameKey(string(r)) != processNameKey(string(next)) {
			t.Fatalf("Unicode fold mismatch: %U and %U", r, next)
		}
	}
	for _, pair := range [][2]string{{"DEMO.EXE", "demo.exe"}, {"K.exe", "k.exe"}, {"ſ.exe", "S.exe"}, {"Σ.exe", "ς.exe"}, {"İ.exe", "i.exe"}} {
		if (processNameKey(pair[0]) == processNameKey(pair[1])) != strings.EqualFold(pair[0], pair[1]) {
			t.Fatal("名称索引改变匹配语义", pair)
		}
	}
}

func TestFirstInstanceDoesNotMutateInput(t *testing.T) {
	items := []ProcessInfo{{PID: 5, Name: "demo.exe", StartTime: 3}, {PID: 4, Name: "demo.exe", StartTime: 0}, {PID: 2, Name: "DEMO.EXE", StartTime: 0}}
	before := append([]ProcessInfo(nil), items...)
	matches := MatchingProcesses(items, ProcessRule{Name: "demo.exe", MatchMode: "first_instance"})
	if len(matches) != 1 || matches[0].PID != 2 || !reflect.DeepEqual(items, before) {
		t.Fatal("首个实例边界或输入数据发生变化", matches, items)
	}
}

func TestProcessMatchingByNamePathAndStartTime(t *testing.T) {
	pathA, pathB := `C:\Apps\DEMO.exe`, `D:\Apps\demo.exe`
	items := []ProcessInfo{{PID: 10, Name: "demo.exe", Path: &pathA, StartTime: 2}, {PID: 9, Name: "DEMO.EXE", Path: &pathB, StartTime: 1}, {PID: 8, Name: "demo.exe", StartTime: 1}, {PID: 7, Name: "other.exe"}}
	rule := ProcessRule{Name: "DEMO.exe", MatchMode: "all_instances"}
	if len(MatchingProcesses(items, rule)) != 3 {
		t.Fatal("名称匹配错误")
	}
	rule.MatchMode = "first_instance"
	if matches := MatchingProcesses(items, rule); len(matches) != 1 || matches[0].PID != 8 {
		t.Fatal(matches)
	}
	rule.MatchMode, rule.Path = "specific_path", &pathA
	if matches := MatchingProcesses(items, rule); len(matches) != 1 || !reflect.DeepEqual(matches[0], items[0]) {
		t.Fatal(matches)
	}
	rule.Path = nil
	if len(MatchingProcesses(items, rule)) != 0 {
		t.Fatal("路径缺失时错误匹配进程")
	}
}
