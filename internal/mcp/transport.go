// internal/mcp/transport.go
// stdio 传输层：管理 MCP Server 子进程，通过 stdin/stdout 进行 JSON-RPC 通信
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// StdioTransport 管理与一个 MCP Server 子进程的 stdio 通信
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	scanner *bufio.Scanner
	stderr bytes.Buffer

	mu      sync.Mutex   // 保护 stdin 写入，防止并发交错
	nextID  atomic.Int64 // 自增请求 ID
	pending map[int64]chan *jsonrpcResponse
	pendMu  sync.RWMutex // 保护 pending map
	done    chan struct{} // 关闭表示子进程已退出

	healthy atomic.Bool   // 连接健康状态
	name    string         // 服务器名称，用于日志
}

// NewStdioTransport 创建并启动一个 MCP Server 子进程
// command: 启动命令（如 "npx"、"python"）
// args: 命令参数
// env: 额外的环境变量（key=value）
func NewStdioTransport(name, command string, args []string, env map[string]string) (*StdioTransport, error) {
	cmd := exec.Command(command, args...)

	// 设置环境变量：继承当前进程环境 + 额外注入
	cmd.Env = cmd.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// 获取 stdin pipe
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("获取 stdin pipe 失败: %w", err)
	}

	// 获取 stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("获取 stdout pipe 失败: %w", err)
	}

	// stderr 重定向到 buffer，后台日志输出
	cmd.Stderr = &stderrBuf{name: name}

	t := &StdioTransport{
		cmd:     cmd,
		stdin:   stdin,
		scanner: bufio.NewScanner(stdout),
		pending: make(map[int64]chan *jsonrpcResponse),
		done:    make(chan struct{}),
		name:    name,
	}

	// 增大 Scanner Buffer 到 10MB，防止大响应截断
	t.scanner.Buffer(make([]byte, 0, 10<<20), 10<<20)

	// 启动子进程
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 MCP Server 进程失败: %w", err)
	}

	log.Printf("[MCP-%s] 子进程已启动 (PID: %d)", name, cmd.Process.Pid)

	// 启动后台读取 goroutine
	go t.readLoop()

	return t, nil
}

// readLoop 持续从 stdout 读取 JSON-RPC 响应并路由到对应的等待者
func (t *StdioTransport) readLoop() {
	for t.scanner.Scan() {
		line := t.scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var resp jsonrpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			log.Printf("[MCP-%s] 解析 JSON-RPC 响应失败: %v\n原始数据: %s", t.name, err, string(line))
			continue
		}

		// 通知消息（id 为 0）直接忽略
		if resp.ID == 0 {
			log.Printf("[MCP-%s] 收到通知消息，忽略", t.name)
			continue
		}

		// 路由到对应的等待者
		t.pendMu.RLock()
		ch := t.pending[resp.ID]
		t.pendMu.RUnlock()
		if ch != nil {
			ch <- &resp
		} else {
			log.Printf("[MCP-%s] 收到未匹配的响应 (id=%d)", t.name, resp.ID)
		}
	}

	// Scanner 退出 = 子进程断开或 stdout 关闭
	t.healthy.Store(false)
	select {
	case <-t.done:
		// 已经关闭（由 Close() 触发），不再重复关闭
	default:
		close(t.done)
	}

	if err := t.scanner.Err(); err != nil {
		log.Printf("[MCP-%s] stdout 读取异常: %v", t.name, err)
	}
	log.Printf("[MCP-%s] 子进程已断开连接", t.name)
}

// SendRequest 发送 JSON-RPC 请求并等待响应
func (t *StdioTransport) SendRequest(ctx context.Context, method string, params interface{}) (*jsonrpcResponse, error) {
	if !t.healthy.Load() {
		return nil, fmt.Errorf("MCP Server '%s' 已断开连接", t.name)
	}

	id := t.nextID.Add(1)
	ch := make(chan *jsonrpcResponse, 1)

	t.pendMu.Lock()
	t.pending[id] = ch
	t.pendMu.Unlock()

	defer func() {
		t.pendMu.Lock()
		delete(t.pending, id)
		t.pendMu.Unlock()
	}()

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	// 并发安全写入 stdin（newline-delimited JSON）
	t.mu.Lock()
	_, err = fmt.Fprintf(t.stdin, "%s\n", data)
	t.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("写入 stdin 失败: %w", err)
	}

	// 等待响应、断开或超时
	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("JSON-RPC 错误 (code=%d): %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	case <-t.done:
		return nil, fmt.Errorf("MCP Server '%s' 在处理请求前断开", t.name)
	case <-ctx.Done():
		return nil, fmt.Errorf("请求超时: %w", ctx.Err())
	}
}

// SendNotification 发送 JSON-RPC 通知（无需响应）
func (t *StdioTransport) SendNotification(method string, params interface{}) error {
	if !t.healthy.Load() {
		return fmt.Errorf("MCP Server '%s' 已断开连接", t.name)
	}

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("序列化通知失败: %w", err)
	}

	t.mu.Lock()
	_, err = fmt.Fprintf(t.stdin, "%s\n", data)
	t.mu.Unlock()
	if err != nil {
		return fmt.Errorf("写入 stdin 失败: %w", err)
	}

	return nil
}

// IsHealthy 返回连接是否健康
func (t *StdioTransport) IsHealthy() bool {
	return t.healthy.Load()
}

// Close 关闭传输层，杀死子进程并回收资源
func (t *StdioTransport) Close() error {
	t.healthy.Store(false)

	// 关闭 done channel（仅首次有效）
	select {
	case <-t.done:
	default:
		close(t.done)
	}

	// 先关闭 stdin，让子进程收到 EOF 后自行退出
	if t.stdin != nil {
		t.stdin.Close()
	}

	// 杀子进程（如果还没退出）
	if t.cmd.Process != nil {
		// 给进程一点时间优雅退出
		done := make(chan struct{})
		go func() {
			t.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
			// 进程已自行退出
		case <-time.After(2 * time.Second):
			// 超时，强制杀死
			t.cmd.Process.Kill()
			<-done
		}
	}

	log.Printf("[MCP-%s] 传输层已关闭", t.name)
	return nil
}

// stderrBuf 将 stderr 输出转发到日志
type stderrBuf struct {
	name string
}

func (s *stderrBuf) Write(p []byte) (n int, err error) {
	log.Printf("[MCP-%s:stderr] %s", s.name, string(bytes.TrimRight(p, "\r\n")))
	return len(p), nil
}
