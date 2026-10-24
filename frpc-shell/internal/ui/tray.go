// Package ui 基于系统托盘提供 M1 全部交互：启停、打开管理面板、日志与配置入口、开机自启。
package ui

import (
	_ "embed"
	"log"

	"fyne.io/systray"

	"frpcshell/internal/appstate"
	"frpcshell/internal/platform"
	"frpcshell/internal/proc"
)

//go:embed icon.ico
var iconData []byte

// Paths 汇总托盘菜单需要的外部资源位置。
type Paths struct {
	BinExists     bool
	CfgPath       string
	LogPath       string
	LaunchLogPath string
	DashboardURL  string
}

type UI struct {
	Version   string
	Mgr       *proc.Manager
	State     *appstate.State
	StatePath string
	Events    chan proc.Event
	Paths     Paths

	mStatus *systray.MenuItem
	mStart  *systray.MenuItem
	mStop   *systray.MenuItem
	mDash   *systray.MenuItem
}

func Run(u *UI) {
	systray.Run(u.onReady, func() {})
}

func (u *UI) onReady() {
	systray.SetIcon(iconData)
	systray.SetTooltip("frpc 外壳 v" + u.Version)

	systray.AddMenuItem("frpc 外壳 v"+u.Version, "").Disable()
	u.mStatus = systray.AddMenuItem("状态：已停止", "")
	u.mStatus.Disable()
	systray.AddSeparator()

	u.mStart = systray.AddMenuItem("启动服务", "以受管模式启动 frpc")
	u.mStop = systray.AddMenuItem("停止服务", "优雅停止 frpc")
	mRestart := systray.AddMenuItem("重启服务", "重启 frpc")
	systray.AddSeparator()

	u.mDash = systray.AddMenuItem("打开管理面板", "在浏览器中打开 frpc 内置管理界面")
	mLog := systray.AddMenuItem("打开运行日志", "")
	mLaunchLog := systray.AddMenuItem("打开启动日志", "frpc 启动阶段的输出，排查启动失败用")
	mCfg := systray.AddMenuItem("编辑配置", "使用记事本编辑 frpc.toml")
	systray.AddSeparator()

	mAuto := systray.AddMenuItemCheckbox("开机自启", "登录 Windows 后自动运行外壳", u.State.Autostart)
	mExit := systray.AddMenuItem("退出并停止服务", "")

	// 状态事件串行消费，统一刷新菜单（托盘控件要求单线程更新）
	go u.consumeEvents()

	// 初始可用性（已停止基线：启动可点、停止/面板禁用）
	if !u.Paths.BinExists {
		u.applyText(proc.Event{State: proc.StateFailed, Detail: "未找到 bin/frpc.exe，请将官方 frpc.exe 放入 bin 目录"})
		u.mStart.Disable()
		u.mStop.Disable()
	} else {
		u.mStop.Disable()
		u.mDash.Disable()
	}
	if platform.IsAutostart() != u.State.Autostart {
		// 注册表与状态不一致时以注册表为准
		u.State.Autostart = platform.IsAutostart()
		_ = u.State.Save(u.StatePath)
		if u.State.Autostart {
			mAuto.Check()
		} else {
			mAuto.Uncheck()
		}
	}

	for range u.mStart.ClickedCh {
		go u.Mgr.Start()
	}
	for range u.mStop.ClickedCh {
		go u.Mgr.Stop()
	}
	for range mRestart.ClickedCh {
		go u.Mgr.Restart()
	}
	for range u.mDash.ClickedCh {
		_ = platform.OpenBrowser(u.Paths.DashboardURL)
	}
	for range mLog.ClickedCh {
		_ = platform.OpenTextFile(u.Paths.LogPath)
	}
	for range mLaunchLog.ClickedCh {
		_ = platform.OpenTextFile(u.Paths.LaunchLogPath)
	}
	for range mCfg.ClickedCh {
		_ = platform.OpenTextFile(u.Paths.CfgPath)
	}
	for range mAuto.ClickedCh {
		u.toggleAutostart(mAuto)
	}
	for range mExit.ClickedCh {
		go func() {
			u.Mgr.Stop()
			systray.Quit()
		}()
	}
}

func (u *UI) consumeEvents() {
	for ev := range u.Events {
		u.applyText(ev)
	}
}

func (u *UI) applyText(ev proc.Event) {
	var status, tooltip string
	switch ev.State {
	case proc.StateRunning:
		status = "状态：运行中"
		tooltip = "frpc 外壳 v" + u.Version + " · 运行中"
		u.mDash.Enable()
	case proc.StateStarting:
		status = "状态：" + ev.State.String() + "（" + ev.Detail + "）"
		tooltip = "frpc 外壳 v" + u.Version + " · " + ev.Detail
		u.mDash.Disable()
	case proc.StateFailed:
		status = "状态：启动失败"
		tooltip = "frpc 外壳 v" + u.Version + " · " + ev.Detail
		u.mDash.Disable()
	default:
		status = "状态：已停止"
		tooltip = "frpc 外壳 v" + u.Version + " · 已停止"
		u.mDash.Disable()
	}

	if u.Paths.BinExists {
		switch ev.State {
		case proc.StateRunning:
			u.mStart.Disable()
			u.mStop.Enable()
		case proc.StateStarting:
			u.mStart.Disable()
			u.mStop.Enable()
		default:
			u.mStart.Enable()
			u.mStop.Disable()
		}
	}

	u.mStatus.SetTitle(status)
	systray.SetTooltip(tooltip)
}

func (u *UI) toggleAutostart(item *systray.MenuItem) {
	next := !u.State.Autostart
	if err := platform.SetAutostart(next); err != nil {
		log.Printf("set autostart: %v", err)
		// 回滚勾选状态
		if u.State.Autostart {
			item.Check()
		} else {
			item.Uncheck()
		}
		return
	}
	u.State.Autostart = next
	if err := u.State.Save(u.StatePath); err != nil {
		log.Printf("save state: %v", err)
	}
}
