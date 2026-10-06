package manager

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	setProcessAffinityMask       = kernel32.NewProc("SetProcessAffinityMask")
	getActiveProcessorGroupCount = kernel32.NewProc("GetActiveProcessorGroupCount")
)

type WindowsProcesses struct{}

func ProcessorInfo() (int, bool) {
	count := int(windows.GetActiveProcessorCount(0xffff))
	groups, _, _ := getActiveProcessorGroupCount.Call()
	return max(count, 1), groups == 1 && count > 0 && count <= 64
}

func (WindowsProcesses) List() ([]ProcessInfo, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	items := []ProcessInfo{}
	buffer := make([]uint16, 32768)
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		item := ProcessInfo{PID: entry.ProcessID, Name: windows.UTF16ToString(entry.ExeFile[:])}
		if handle, openErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, item.PID); openErr == nil {
			var created, exited, kernel, user windows.Filetime
			if windows.GetProcessTimes(handle, &created, &exited, &kernel, &user) == nil {
				item.StartTime = uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
			}
			size := uint32(len(buffer))
			if windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size) == nil {
				value := windows.UTF16ToString(buffer[:size])
				item.Path = &value
			}
			windows.CloseHandle(handle)
		}
		items = append(items, item)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, err
	}
	return items, nil
}

func windowsError(action string, err error) string {
	var code syscall.Errno
	if errors.As(err, &code) {
		return fmt.Sprintf("%s：%s（错误码 %d）", action, err, code)
	}
	return fmt.Sprintf("%s：%s", action, err)
}

func (WindowsProcesses) Apply(info ProcessInfo, rule ProcessRule, cpuCount int, singleGroup bool) ApplyResult {
	result := ApplyResult{PID: info.PID}
	if info.PID == uint32(os.Getpid()) || strings.EqualFold(info.Name, "ProcessAffinityManager.exe") || strings.EqualFold(info.Name, "process-affinity-manager.exe") || strings.EqualFold(info.Name, "process-affinity-manager-go.exe") {
		result.Message = "拒绝修改工具自身"
		return result
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, info.PID)
	if err != nil {
		result.Message = windowsError("无法打开进程", err)
		return result
	}
	defer windows.CloseHandle(handle)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
		result.Message = windowsError("读取进程启动时间失败", err)
		return result
	}
	if info.StartTime != 0 && info.StartTime != uint64(created.HighDateTime)<<32|uint64(created.LowDateTime) {
		result.Message = "进程已重启，将在下一次扫描重新匹配"
		return result
	}
	if rule.Affinity != "none" {
		if !singleGroup {
			result.Message = "CPU 亲和性仅支持单 Processor Group、最多 64 个逻辑 CPU"
			return result
		}
		mask, err := AffinityMask(rule, cpuCount)
		if err != nil {
			result.Message = err.Error()
			return result
		}
		ok, _, callErr := setProcessAffinityMask.Call(uintptr(handle), uintptr(mask))
		if ok == 0 {
			result.Message = windowsError("设置 CPU 亲和性失败", callErr)
			return result
		}
	}
	classes := map[string]uint32{"idle": 0x40, "below_normal": 0x4000, "normal": 0x20, "above_normal": 0x8000, "high": 0x80, "realtime": 0x100}
	class, valid := classes[rule.Priority]
	if !valid {
		result.Message = "优先级无效"
		return result
	}
	if err = windows.SetPriorityClass(handle, class); err != nil {
		result.Message = windowsError("设置进程优先级失败", err)
		return result
	}
	result.OK = true
	result.Message = "应用成功"
	return result
}

func EnableDebugPrivilege() error {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()
	var luid windows.LUID
	name, _ := windows.UTF16PtrFromString("SeDebugPrivilege")
	if err := windows.LookupPrivilegeValue(nil, name, &luid); err != nil {
		return err
	}
	privileges := windows.Tokenprivileges{PrivilegeCount: 1}
	privileges.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
	adjust := windows.NewLazySystemDLL("advapi32.dll").NewProc("AdjustTokenPrivileges")
	ok, _, callErr := adjust.Call(uintptr(token), 0, uintptr(unsafe.Pointer(&privileges)), 0, 0, 0)
	if ok == 0 || errors.Is(callErr, windows.ERROR_NOT_ALL_ASSIGNED) {
		return callErr
	}
	return nil
}

func replaceFile(from, to string) error {
	fromPtr, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	toPtr, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(fromPtr, toPtr, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
