// Package proc 以受管模式运行 frpc：拉起子进程、轮询就绪、崩溃自动重启（指数退避）、
// 优先经 Admin API 优雅停止，超时强杀。
package proc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"frpcshell/internal/admin"
)

type State int

const (
	StateStopped State = iota
	StateStarting
	StateRunning
	StateFailed
)

func (s State) String() string {
	switch s {
	case StateStarting:
		return "启动中"
	case StateRunning:
		return "运行中"
	case StateFailed:
		return "启动失败"
	default:
		return "已停止"
	}
}

type Event struct {
	State  State
	Detail string
}

type Options struct {
	BinPath string // bin/frpc.exe
	CfgPath string // frpc.toml
	WorkDir string // 安装目录
	ErrLog  string // 启动 stderr/stdout 捕获文件
	Admin   *admin.Client
	OnEvent func(Event)
}

const (
	readyTimeout   = 10 * time.Second
	probeInterval  = 300 * time.Millisecond
	probeTimeout   = 800 * time.Millisecond
	stopGrace      = 5 * time.Second
	maxFailStreak  = 5
	initialBackoff = time.Second
	maxBackoff     = 60 * time.Second
)

type Manager struct {
	opts Options

	mu         sync.Mutex
	cmd        *exec.Cmd
	exited     chan struct{}
	exitErr    error
	gen        int
	manualStop bool
	loopCancel context.CancelFunc
	state      State
	detail     string
}

func New(opts Options) *Manager {
	return &Manager{opts: opts, state: StateStopped}
}

// Start 受管启动。已运行时返回错误。
func (m *Manager) Start() error {
	m.mu.Lock()
	if m.cmd != nil {
		m.mu.Unlock()
		return errors.New("service is already running")
	}
	m.gen++
	gen := m.gen
	m.manualStop = false
	if m.loopCancel != nil {
		m.loopCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.loopCancel = cancel
	m.mu.Unlock()

	if err := m.spawn(); err != nil {
		cancel()
		m.setState(StateFailed, err.Error())
		return err
	}
	m.setState(StateStarting, "")
	go m.loop(ctx, gen)
	return nil
}

// Stop 停止服务：Admin API 优雅停止 → 超时强杀。
func (m *Manager) Stop() {
	m.mu.Lock()
	cmd := m.cmd
	exited := m.exited
	if m.loopCancel != nil {
		m.loopCancel()
	}
	m.gen++
	m.manualStop = true
	m.mu.Unlock()

	if cmd != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = m.opts.Admin.Stop(ctx)
		cancel()

		select {
		case <-exited:
		case <-time.After(stopGrace):
			_ = cmd.Process.Kill()
			select {
			case <-exited:
			case <-time.After(3 * time.Second):
			}
		}
	}

	m.mu.Lock()
	m.cmd = nil
	m.exited = nil
	m.mu.Unlock()
	m.setState(StateStopped, "")
}

// Restart 等价于 Stop + Start。
func (m *Manager) Restart() {
	m.Stop()
	_ = m.Start()
}

func (m *Manager) CurrentState() (State, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state, m.detail
}

// FrpcVersion 返回受管 frpc 二进制的版本串。
func (m *Manager) FrpcVersion() string {
	out, err := exec.Command(m.opts.BinPath, "-v").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// clearCmd 清理已退出进程的句柄，保证 Start 的"已运行"判定准确。
func (m *Manager) clearCmd() {
	m.mu.Lock()
	m.cmd = nil
	m.exited = nil
	m.mu.Unlock()
}

// spawn 拉起子进程并注册退出监听（退出信号用 close(channel) 广播，避免多接收方竞争）。
func (m *Manager) spawn() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cmd = nil
	m.exited = nil

	f, err := os.Create(m.opts.ErrLog)
	if err != nil {
		return fmt.Errorf("create launch log: %w", err)
	}
	cmd := exec.Command(m.opts.BinPath, "-c", m.opts.CfgPath)
	cmd.Dir = m.opts.WorkDir
	cmd.Stdout = f
	cmd.Stderr = f
	if err := cmd.Start(); err != nil {
		f.Close()
		return err
	}

	exited := make(chan struct{})
	m.cmd = cmd
	m.exited = exited
	m.exitErr = nil

	go func() {
		err := cmd.Wait()
		f.Close()
		m.mu.Lock()
		m.exitErr = err
		m.mu.Unlock()
		close(exited)
	}()
	return nil
}

// loop 重启监督循环，每个 Start 一个，gen 失配即退出。
func (m *Manager) loop(ctx context.Context, gen int) {
	backoff := initialBackoff
	streak := 0

	for {
		if m.changed(ctx, gen) {
			return
		}

		ready, exitErr := m.waitReady(ctx, gen)
		if m.changed(ctx, gen) {
			return
		}

		if ready {
			streak = 0
			backoff = initialBackoff
			m.setState(StateRunning, "")

			exitErr := m.waitExit(ctx)
			if m.changed(ctx, gen) {
				return
			}
			m.clearCmd()
			// 崩溃：走重启路径
			m.setState(StateStarting, fmt.Sprintf("进程退出(%v)，%s 后自动重启", exitErr, backoff))
		} else {
			// 启动失败：杀掉残留进程，清理句柄，计数重试
			m.killIfAlive()
			m.clearCmd()
			streak++
			detail := "就绪探测超时"
			if exitErr != nil {
				detail = "启动即退出: " + exitErr.Error()
			}
			if streak >= maxFailStreak {
				m.setState(StateFailed, "连续启动失败("+detail+")，已停止重试。请查看启动日志")
				return
			}
			m.setState(StateStarting, fmt.Sprintf("%s，%s 后重试(%d/%d)", detail, backoff, streak, maxFailStreak))
		}

		if m.sleep(ctx, backoff) {
			return
		}
		backoff = nextBackoff(backoff)
		if err := m.spawn(); err != nil {
			// spawn 失败（如二进制被删、配置被删）：进入失败计数
			streak++
			if streak >= maxFailStreak {
				m.setState(StateFailed, "连续启动失败("+err.Error()+")，已停止重试")
				return
			}
			m.setState(StateStarting, fmt.Sprintf("拉起失败(%v)，%s 后重试(%d/%d)", err, backoff, streak, maxFailStreak))
			if m.sleep(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
		}
	}
}

// waitReady 阻塞直至 healthz 通过（true, nil）、进程退出（false, exitErr）或 ctx 取消（false, nil）。
func (m *Manager) waitReady(ctx context.Context, gen int) (bool, error) {
	deadline := time.Now().Add(readyTimeout)
	for {
		pctx, cancel := context.WithTimeout(ctx, probeTimeout)
		err := m.opts.Admin.Healthz(pctx)
		cancel()
		if err == nil {
			return true, nil
		}
		if ctx.Err() != nil {
			return false, nil
		}

		m.mu.Lock()
		exited := m.exited
		m.mu.Unlock()
		if exited != nil {
			select {
			case <-exited:
				return false, m.takeExitErr()
			default:
			}
		}

		if time.Now().After(deadline) {
			return false, nil
		}
		if m.sleep(ctx, probeInterval) {
			return false, nil
		}
	}
}

// waitExit 等待当前代进程退出（Stop 会取消 ctx 使本函数提前返回）。
func (m *Manager) waitExit(ctx context.Context) error {
	m.mu.Lock()
	exited := m.exited
	m.mu.Unlock()
	if exited == nil {
		return nil
	}
	select {
	case <-exited:
		return m.takeExitErr()
	case <-ctx.Done():
		return nil
	}
}

func (m *Manager) takeExitErr() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.exitErr
}

func (m *Manager) killIfAlive() {
	m.mu.Lock()
	cmd := m.cmd
	m.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	m.clearCmd()
}

func (m *Manager) changed(ctx context.Context, gen int) bool {
	if ctx.Err() != nil {
		return true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return gen != m.gen
}

func (m *Manager) sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return true
	case <-time.After(d):
		return false
	}
}

func (m *Manager) setState(s State, detail string) {
	m.mu.Lock()
	m.state = s
	m.detail = detail
	cb := m.opts.OnEvent
	m.mu.Unlock()
	if cb != nil {
		cb(Event{State: s, Detail: detail})
	}
}

func nextBackoff(d time.Duration) time.Duration {
	d *= 2
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}
