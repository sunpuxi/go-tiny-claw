package context

import (
	"context"
	"encoding/json"
	"github.com/sunpuxi/go-tiny-claw/internal/schema"
	"github.com/tmc/langchaingo/llms/openai"
	"log"
	"os"
)

type LLMCompactor struct {
	llm       *openai.LLM
	maxLength int
}

func NewLLMCompactor(modelName string, maxLength int) *LLMCompactor {
	llm, err := openai.New(
		openai.WithModel(modelName),
		openai.WithToken(os.Getenv("DEEPSEEK_API_KEY")),
		openai.WithBaseURL("https://api.deepseek.com"),
	)
	if err != nil {
		log.Fatal(err)
	}

	return &LLMCompactor{
		llm:       llm,
		maxLength: maxLength,
	}
}

func (l *LLMCompactor) Compact(msgs []schema.Message) []schema.Message {
	// 没有超过设定的阈值上限，则直接返回
	if EstimateLength(msgs) < l.maxLength {
		return msgs
	}

	// 提示词
	prompt := `
现在，你是一个专业的字符压缩专家，我会给你一段文本，这段文本是提供给Agent执行的上下文信息，你需要保留其中的关键信息，
比如工具执行的先后顺序，以及大致的结果（如果工具执行的结果中包含大量的无效信息，这部分信息需要删除，或者替换为“为节省上下文空间，已截断”）
你返回的信息之中，不能包含多余的文本和空格，只在我提供给你的文本信息上进行处理。
输入的文本信息如下：
`

	// json 序列化原始的信息
	jsonStr, err := json.Marshal(msgs)
	if err != nil {
		// 如果压缩失败，则返回原始的文本信息
		return msgs
	}

	// 调用 LLM
	ctx := context.Background()
	resp, err := l.llm.Call(ctx, prompt+string(jsonStr))
	if err != nil {
		log.Fatal(err)
	}

	// 反序列化为上下文信息的格式
	var respMsgs []schema.Message
	err = json.Unmarshal([]byte(resp), &respMsgs)
	if err != nil {
		// 暂时处理为返回原始文本，后续可优化为继续调用模型处理，直至返回正确的结果
		return msgs
	}

	return respMsgs
}
