package manager

import (
	"bufio"
	"fmt"
	"math/bits"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestProcessHelper(t *testing.T) {
	if os.Getenv("AFFINITY_TEST_HELPER") != "1" {
		return
	}
	fmt.Println("就绪")
	for {
		time.Sleep(time.Hour)
	}
}

func processMask(handle windows.Handle) (uintptr, error) {
	var process, system uintptr
	ok, _, err := kernel32.NewProc("GetProcessAffinityMask").Call(uintptr(handle), uintptr(unsafe.Pointer(&process)), uintptr(unsafe.Pointer(&system)))
	if ok == 0 {
		return 0, err
	}
	return process, nil
}

func TestWindowsApplyToIsolatedChild(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(exe, "-test.run=^TestProcessHelper$")
	child.Env = append(os.Environ(), "AFFINITY_TEST_HELPER=1")
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "就绪\n" {
		t.Fatal("子进程启动失败", line, err)
	}
	source := WindowsProcesses{}
	items, err := source.List()
	if err != nil {
		t.Fatal(err)
	}
	var info ProcessInfo
	for _, item := range items {
		if item.PID == uint32(child.Process.Pid) {
			info = item
			break
		}
	}
	if info.PID == 0 || info.Path == nil || info.StartTime == 0 {
		t.Fatalf("真实进程信息缺失：%+v", info)
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, info.PID)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	count, singleGroup := ProcessorInfo()
	rule := ProcessRule{Name: info.Name, Affinity: "none", Priority: "below_normal"}
	if result := source.Apply(info, rule, count, singleGroup); !result.OK {
		t.Fatal(result.Message)
	}
	if class, err := windows.GetPriorityClass(handle); err != nil || class != 0x4000 {
		t.Fatal("真实优先级未改变", class, err)
	}
	if singleGroup {
		mask, err := processMask(handle)
		if err != nil || mask == 0 {
			t.Fatal(mask, err)
		}
		cpu := bits.TrailingZeros64(uint64(mask))
		rule.Affinity, rule.CPUs = "cpu_list", []int{cpu}
		if result := source.Apply(info, rule, count, true); !result.OK {
			t.Fatal(result.Message)
		}
		if mask, err := processMask(handle); err != nil || uint64(mask) != uint64(1)<<cpu {
			t.Fatal("真实亲和性未改变", mask, err)
		}
	}
	stale := info
	stale.StartTime++
	if source.Apply(stale, rule, count, singleGroup).OK {
		t.Fatal("PID 重用检查未拦截过期进程")
	}
	if source.Apply(ProcessInfo{PID: uint32(os.Getpid()), Name: "self.exe"}, rule, count, singleGroup).OK {
		t.Fatal("工具自身保护失效")
	}
}

func TestStartupCommandQuotesWindowsPath(t *testing.T) {
	exe := `C:\Program Files\测试\工具.exe`
	if !reflect.DeepEqual(startupCommand(exe), `"C:\Program Files\测试\工具.exe"`) {
		t.Fatal("含空格的启动路径未正确引用")
	}
}
