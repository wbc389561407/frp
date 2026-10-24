// FrpcShell —— frpc Windows 外壳（M1：纯托盘程序）。
// 首次运行生成 frpc.toml 基线配置并打开记事本，托盘负责受管启停、
// 崩溃自恢复、打开管理面板与开机自启。架构见 doc/frpc-windows-shell-spec.md。
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"frpcshell/internal/admin"
	"frpcshell/internal/appstate"
	"frpcshell/internal/genconfig"
	"frpcshell/internal/platform"
	"frpcshell/internal/proc"
	"frpcshell/internal/ui"
)

const shellVersion = "0.1.0"

type paths struct {
	bin          string
	cfg          string
	log          string
	launchLog    string
	state        string
	dashboardURL string
}

func main() {
	runtime.LockOSThread() // 托盘事件循环要求主线程

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("resolve executable: %v", err)
	}
	base := filepath.Dir(exe)

	// 目录布局：bin/ logs/ store/ data/
	for _, d := range []string{
		filepath.Join(base, "bin"),
		filepath.Join(base, "logs"),
		filepath.Join(base, "store"),
		filepath.Join(base, "data"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			log.Fatalf("create dir %s: %v", d, err)
		}
	}

	// 外壳自身日志（windowsgui 子系统下无控制台可看）
	if f, err := os.OpenFile(filepath.Join(base, "logs", "shell.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		defer f.Close()
		log.SetOutput(f)
	}

	// 外壳状态
	statePath := filepath.Join(base, "data", "shell.json")
	st, err := appstate.Load(statePath)
	if err != nil {
		log.Printf("load state, reset to default: %v", err)
		st = nil
	}
	if st == nil {
		st = appstate.Default(genconfig.RandomHex(16))
	}

	p := buildPaths(base, st.AdminPort)

	// 首次运行：生成基线配置并打开记事本（M1 向导）
	if _, err := os.Stat(p.cfg); errors.Is(err, fs.ErrNotExist) {
		content := genconfig.TOML("", 7000, "", st.AdminUser, st.AdminPassword, st.AdminPort)
		if err := os.WriteFile(p.cfg, []byte(content), 0o600); err != nil {
			log.Fatalf("write config: %v", err)
		}
		_ = platform.OpenTextFile(p.cfg)
	}
	if err := st.Save(p.state); err != nil {
		log.Printf("save state: %v", err)
	}

	adminC := admin.New(st.AdminPort, st.AdminUser, st.AdminPassword)

	events := make(chan proc.Event, 32)
	mgr := proc.New(proc.Options{
		BinPath: p.bin,
		CfgPath: p.cfg,
		WorkDir: base,
		ErrLog:  p.launchLog,
		Admin:   adminC,
		OnEvent: func(ev proc.Event) {
			select {
			case events <- ev:
			default: // 事件队列满时丢弃旧事件，状态以最新为准
				select {
				case <-events:
				default:
				}
				select {
				case events <- ev:
				default:
				}
			}
		},
	})

	if v := mgr.FrpcVersion(); v != "" {
		st.FrpcVersion = v
		_ = st.Save(p.state)
	}

	// 自动运行：勾选了开机自启时，外壳被拉起后自动启动服务（首次运行 Autostart=false 不会触发）
	if st.Autostart && genconfig.ServerAddr(readFile(p.cfg)) != "" {
		go mgr.Start()
	}
	st.Onboarded = true
	_ = st.Save(p.state)

	tray := &ui.UI{
		Version:   shellVersion,
		Mgr:       mgr,
		State:     st,
		StatePath: p.state,
		Events:    events,
		Paths: ui.Paths{
			BinExists:     fileExists(p.bin),
			CfgPath:       p.cfg,
			LogPath:       p.log,
			LaunchLogPath: p.launchLog,
			DashboardURL:  p.dashboardURL,
		},
	}
	ui.Run(tray)
}

func buildPaths(base string, adminPort int) paths {
	return paths{
		bin:          filepath.Join(base, "bin", "frpc.exe"),
		cfg:          filepath.Join(base, "frpc.toml"),
		log:          filepath.Join(base, "logs", "frpc.log"),
		launchLog:    filepath.Join(base, "logs", "frpc-launch.log"),
		state:        filepath.Join(base, "data", "shell.json"),
		dashboardURL: fmt.Sprintf("http://127.0.0.1:%d/static/", adminPort),
	}
}

func readFile(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(raw)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
