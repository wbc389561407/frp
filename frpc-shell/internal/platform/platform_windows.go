//go:build windows

// Package platform 提供 Windows 特定的系统能力：开机自启（HKCU Run 键）、打开浏览器/文本文件。
package platform

import (
	"fmt"
	"os"
	"os/exec"

	"golang.org/x/sys/windows/registry"
)

const (
	runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	runKeyName = "FrpcShell"
)

// SetAutostart 写入或删除当前用户的开机自启项。
func SetAutostart(enable bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("open run key: %w", err)
	}
	defer k.Close()

	if !enable {
		return k.DeleteValue(runKeyName)
	}
	exe, err := execPath()
	if err != nil {
		return err
	}
	return k.SetStringValue(runKeyName, `"`+exe+`"`)
}

func IsAutostart() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(runKeyName)
	return err == nil && v != ""
}

func execPath() (string, error) {
	return os.Executable()
}

func OpenBrowser(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func OpenTextFile(path string) error {
	return exec.Command("notepad.exe", path).Start()
}
