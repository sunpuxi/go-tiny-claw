// internal/mcp/testserver/echo_server.go
// 最小化的 MCP Server 实现，用于集成测试
// 暴露一个 echo 工具：接收任意 JSON 参数，原样返回
//
// 使用方法：
//
//	go run ./internal/mcp/testserver/echo_server.go
//
// 配置 config/mcp_servers.yaml：
//
//	mcp_servers:
//	  - name: "echo"
//	    command: "go"
//	    args: ["run", "./internal/mcp/testserver/echo_server.go"]
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
)

// ---- 简化的 JSON-RPC / MCP 类型 ----

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type toolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	// 增大 buffer
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)

	log.SetOutput(os.Stderr) // 日志写入 stderr，避免污染 stdout 的 JSON-RPC 通信
	log.SetPrefix("[echo-mcp] ")

	writer := bufio.NewWriter(os.Stdout)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			log.Printf("解析请求失败: %v", err)
			continue
		}

		resp := handleRequest(req)
		if resp == nil {
			continue // 通知消息，不回复
		}

		data, _ := json.Marshal(resp)
		fmt.Fprintf(writer, "%s\n", data)
		writer.Flush()
	}

	if err := scanner.Err(); err != nil {
		log.Printf("stdin 读取错误: %v", err)
	}
}

func handleRequest(req request) *response {
	switch req.Method {
	case "initialize":
		return handleInitialize(req)
	case "tools/list":
		return handleListTools(req)
	case "tools/call":
		return handleCallTool(req)
	default:
		// 通知消息（notifications/initialized 等），忽略
		log.Printf("收到非请求方法: %s", req.Method)
		return nil
	}
}

func handleInitialize(req request) *response {
	result, _ := json.Marshal(map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"capabilities": map[string]interface{}{
			"tools": struct{}{},
		},
		"serverInfo": serverInfo{
			Name:    "echo-test-server",
			Version: "1.0.0",
		},
	})
	return &response{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func handleListTools(req request) *response {
	result, _ := json.Marshal(map[string]interface{}{
		"tools": []toolDef{
			{
				Name:        "echo",
				Description: "回声工具：接收任意 JSON 参数，将其原样返回。用于测试 MCP 通信链路。",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"message": map[string]interface{}{
							"type":        "string",
							"description": "要回显的消息内容",
						},
					},
					"required": []string{"message"},
				},
			},
			{
				Name:        "get_time",
				Description: "返回服务器当前时间",
				InputSchema: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
	})
	return &response{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func handleCallTool(req request) *response {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	json.Unmarshal(req.Params, &params)

	var resultText string
	switch params.Name {
	case "echo":
		argsJSON, _ := json.MarshalIndent(params.Arguments, "", "  ")
		resultText = fmt.Sprintf("Echo 回显:\n%s", string(argsJSON))
	case "get_time":
		resultText = fmt.Sprintf("服务器当前时间: (MCP echo server 在 stdio 模式下运行)")
	default:
		resultText = fmt.Sprintf("未知工具: %s", params.Name)
	}

	result, _ := json.Marshal(map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": resultText,
			},
		},
		"isError": false,
	})
	return &response{JSONRPC: "2.0", ID: req.ID, Result: result}
}
