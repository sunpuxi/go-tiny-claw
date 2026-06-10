package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/tmc/langchaingo/llms/openai"
)

func main() {
	// 1. 初始化 DeepSeek 模型
	llm, err := openai.New(
		openai.WithModel("deepseek-chat"),               // 也可以填 "deepseek-reasoner" (R1模型)
		openai.WithToken(os.Getenv("DEEPSEEK_API_KEY")), // 从环境变量读取 Key
		openai.WithBaseURL("https://api.deepseek.com"),  // DeepSeek API 基础地址
	)
	if err != nil {
		log.Fatal(err)
	}

	// 2. 发起问答请求
	ctx := context.Background()
	resp, err := llm.Call(ctx, "你好，请用一句话介绍你自己")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(resp)
}
