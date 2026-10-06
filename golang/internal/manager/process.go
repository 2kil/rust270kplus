package manager

import (
	"fmt"
	"slices"
	"strings"
)

type ProcessInfo struct {
	PID       uint32  `json:"pid"`
	Name      string  `json:"name"`
	Path      *string `json:"path"`
	StartTime uint64  `json:"start_time"`
}

type ApplyResult struct {
	PID     uint32 `json:"pid"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type RuleSnapshot struct {
	Index   int      `json:"index"`
	Name    string   `json:"name"`
	Running bool     `json:"running"`
	PIDs    []uint32 `json:"pids"`
}

func MatchingProcesses(processes []ProcessInfo, rule ProcessRule) []ProcessInfo {
	items := []ProcessInfo{}
	for _, item := range processes {
		if !strings.EqualFold(item.Name, rule.Name) {
			continue
		}
		if rule.MatchMode == "specific_path" && (rule.Path == nil || item.Path == nil || !strings.EqualFold(*rule.Path, *item.Path)) {
			continue
		}
		items = append(items, item)
	}
	if rule.MatchMode == "first_instance" {
		slices.SortFunc(items, func(a, b ProcessInfo) int {
			if a.StartTime < b.StartTime {
				return -1
			}
			if a.StartTime > b.StartTime {
				return 1
			}
			if a.PID < b.PID {
				return -1
			}
			if a.PID > b.PID {
				return 1
			}
			return 0
		})
		if len(items) > 1 {
			items = items[:1]
		}
	}
	return items
}

func AffinityMask(rule ProcessRule, cpuCount int) (uint64, error) {
	if cpuCount < 1 || cpuCount > 64 {
		return 0, fmt.Errorf("当前版本仅支持单组且最多 64 个逻辑 CPU")
	}
	switch rule.Affinity {
	case "all":
		return ^uint64(0) >> (64 - cpuCount), nil
	case "last_cpu":
		return uint64(1) << (cpuCount - 1), nil
	case "cpu_list":
		var mask uint64
		for _, cpu := range rule.CPUs {
			if cpu < 0 || cpu >= cpuCount {
				return 0, fmt.Errorf("CPU %d 不存在", cpu)
			}
			mask |= uint64(1) << cpu
		}
		if mask == 0 {
			return 0, fmt.Errorf("CPU 列表不能为空")
		}
		return mask, nil
	default:
		return 0, fmt.Errorf("亲和性模式无效")
	}
}
