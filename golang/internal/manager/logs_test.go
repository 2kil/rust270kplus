package manager

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLogsKeepLatestHundredAcrossRestarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "app.log")
	store := NewLogStore(path)
	if lines, err := store.Read(); err != nil || len(lines) != 0 {
		t.Fatal(lines, err)
	}
	for i := range 105 {
		if err := store.Append(fmt.Sprintf("日志 %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	lines, err := NewLogStore(path).Read()
	if err != nil || len(lines) != 100 || !strings.HasSuffix(lines[0], "日志 5") || !strings.HasSuffix(lines[99], "日志 104") {
		t.Fatal(lines, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || len(strings.Split(strings.TrimSpace(string(data)), "\n")) != 100 {
		t.Fatal("持久日志超过上限", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatal(err)
	}
	if lines, err := store.Read(); err != nil || len(lines) != 0 {
		t.Fatal(lines, err)
	}
}

func TestLegacyLogsWithoutFinalNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("[12:34:56] 已有日志"), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewLogStore(path)
	if err := store.Append("新日志"); err != nil {
		t.Fatal(err)
	}
	lines, err := store.Read()
	if err != nil || len(lines) != 2 || lines[0] != "[12:34:56] 已有日志" || !strings.HasSuffix(lines[1], "新日志") {
		t.Fatal(lines, err)
	}
}

func TestConcurrentLogWritersStayBounded(t *testing.T) {
	store := NewLogStore(filepath.Join(t.TempDir(), "app.log"))
	var group sync.WaitGroup
	for worker := range 4 {
		group.Add(1)
		go func() {
			defer group.Done()
			for i := range 30 {
				if err := store.Append(fmt.Sprintf("%d-%d", worker, i)); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	group.Wait()
	if lines, err := store.Read(); err != nil || len(lines) != 100 {
		t.Fatal(lines, err)
	}
}
