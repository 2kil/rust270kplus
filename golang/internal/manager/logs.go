package manager

import (
	"os"
	"strings"
	"sync"
	"time"
)

const LogLimit = 100

type LogStore struct {
	mu   sync.Mutex
	path string
}

func NewLogStore(path string) *LogStore { return &LogStore{path: path} }

func recentLogs(text string) []string {
	text = strings.TrimSuffix(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if text == "" {
		return []string{}
	}
	lines := strings.Split(text, "\n")
	if len(lines) > LogLimit {
		lines = lines[len(lines)-LogLimit:]
	}
	for i, line := range lines {
		if len(line) < 10 || line[0] != '[' || line[9] != ']' {
			lines[i] = "[" + time.Now().Format("15:04:05") + "] " + line
		}
	}
	return lines
}

func (store *LogStore) Read() ([]string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	data, err := os.ReadFile(store.path)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	return recentLogs(string(data)), nil
}

func (store *LogStore) Append(message string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	data, err := os.ReadFile(store.path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	text := string(data)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += "[" + time.Now().Format("15:04:05") + "] " + message + "\n"
	return writeFile(store.path, []byte(strings.Join(recentLogs(text), "\n")+"\n"))
}

func (store *LogStore) Clear() error {
	store.mu.Lock()
	defer store.mu.Unlock()
	return writeFile(store.path, nil)
}
