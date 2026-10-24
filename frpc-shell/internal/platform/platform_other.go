//go:build !windows

package platform

import (
	"errors"
	"os/exec"
)

// 非 Windows 平台仅用于开发编译，功能为空实现或通用替代。

func SetAutostart(enable bool) error { return errors.New("autostart only supported on windows") }

func IsAutostart() bool { return false }

func OpenBrowser(url string) error {
	return exec.Command("xdg-open", url).Start()
}

func OpenTextFile(path string) error {
	return exec.Command("xdg-open", path).Start()
}
