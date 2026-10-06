package manager

import (
	"errors"
	"os"

	"golang.org/x/sys/windows/registry"
)

func SetStartWithWindows(enabled bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if enabled {
		return key.SetStringValue("ProcessAffinityManager", startupCommand(exe))
	}
	err = key.DeleteValue("ProcessAffinityManager")
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}

func startupCommand(exe string) string { return `"` + exe + `"` }
