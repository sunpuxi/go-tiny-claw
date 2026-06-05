package eval

import (
	"context"
	"fmt"
	ctxpkg "github.com/sunpuxi/go-tiny-claw/internal/context"
	"github.com/sunpuxi/go-tiny-claw/internal/engine"
	"github.com/sunpuxi/go-tiny-claw/internal/observability"
	"github.com/sunpuxi/go-tiny-claw/internal/provider"
	"github.com/sunpuxi/go-tiny-claw/internal/schema"
	"github.com/sunpuxi/go-tiny-claw/internal/tools"
	"log"
	"os"
	"os/exec"
	"runtime"
	"time"
)

type TestCase struct {
	ID                string // 用例的唯一表示
	Name              string // 用例的名称
	SetupScript       string // 在 Agent 运行之前的脚本（Linux/macOS: bash 语法）
	SetupScriptWin    string // 在 Agent 运行之前的脚本（Windows: PowerShell 语法），为空时回退到 SetupScript
	TaskPrompt        string // 发送给Agent执行任务的提示词
	ValidateScript    string // 验证 Agent 执行结果的脚本（Linux/macOS: bash 语法）
	ValidateScriptWin string // 验证 Agent 执行结果的脚本（Windows: PowerShell 语法），为空时回退到 ValidateScript
	MaxTurns          int    // 允许Agent执行任务的最大轮次
}

type TestResult struct {
	TestCaseID   string  // 测试用例的ID
	Passed       bool    // 是否通过测试
	TotalCostCNY float64 // 总共消耗的金额
	DurationMs   int64   // 持续时间
	ErrorMsg     string  // 错误信息
}

type BenchmarkRunner struct {
	modelName string
}

func NewBenchmarkRunner(modelName string) *BenchmarkRunner {
	return &BenchmarkRunner{modelName: modelName}
}

func (b *BenchmarkRunner) RunSuite(ctx context.Context, tcs []TestCase) {
	log.Printf("==================")
	log.Printf("启动Benchmark评估...|模型：%s|", b.modelName)
	log.Printf("==================")

	var results []TestResult
	passedCount := 0
	totalCost := 0.0

	for _, tc := range tcs {
		log.Printf("\n>>> 正在执行测试用例:[%s]:%s\n", tc.ID, tc.Name)
		res := b.RunSingleTest(ctx, tc)
		results = append(results, res)

		if res.Passed {
			passedCount++
			log.Printf(">>> ✔ 用例 [%s] 测试通过，耗时：%dms, 总消耗：%f元\n", res.TestCaseID, res.DurationMs, res.TotalCostCNY)
		} else {
			log.Printf(">>> ❌ 用例 [%s] 测试失败，耗时：%dms, 总消耗：%f元,错误信息：%s\n", res.TestCaseID, res.DurationMs, res.TotalCostCNY, res.ErrorMsg)
		}
		totalCost += res.TotalCostCNY
	}

	log.Printf("======== 跑分报告信息 ==========")
	log.Printf("总用例数：%d，成功数：%d，成功率：%.2f%%", len(tcs), passedCount, float64(passedCount)/float64(len(tcs))*100)
	log.Printf("==================")
}

func (b *BenchmarkRunner) RunSingleTest(ctx context.Context, tc TestCase) TestResult {
	// 开始时间
	start := time.Now()

	// 给每一个测试用例创建一个干净的互不干扰的工作目录
	workDir, _ := os.Getwd()
	workDir += fmt.Sprintf("/workspace/%s_%d", tc.ID, time.Now().Unix())
	_ = os.MkdirAll(workDir, 0755)

	// 执行SetUp 脚本代码【可选】
	setupScript := tc.SetupScript
	if runtime.GOOS == "windows" && tc.SetupScriptWin != "" {
		setupScript = tc.SetupScriptWin
	}
	if setupScript != "" {
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			// Windows 环境：使用 powershell 执行
			// 加上 -NoProfile 可以避免加载冗长的用户脚本，防止命令干扰
			cmd = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", setupScript)
		} else {
			// macOS/Linux 环境：保持使用 bash
			cmd = exec.Command("bash", "-c", setupScript)
		}
		cmd.Dir = workDir
		if err := cmd.Run(); err != nil {
			return TestResult{TestCaseID: tc.ID, ErrorMsg: "setup script run failed", Passed: false}
		}
	}

	// 创建provider
	realProvider := provider.NewZhipuOpenAIProvider(b.modelName)
	session := ctxpkg.NewSession(tc.ID, workDir)
	trackerProvider := observability.NewCostTracker(realProvider, b.modelName, session)

	// 工具集
	registry := tools.NewRegistry()
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))

	// 创建引擎
	eng := engine.NewAgentEngine(trackerProvider, registry, false, false)

	// 将当前测试用例的提示词注入
	session.Append(schema.Message{Role: schema.RoleUser, Content: tc.TaskPrompt})

	// 启动引擎执行任务
	err := eng.Run(ctx, session, nil)
	if err != nil {
		return TestResult{TestCaseID: tc.ID, ErrorMsg: "engine run failed", Passed: false}
	}

	// 结果断言，判断 Agent 执行结果是否可验证
	validateScript := tc.ValidateScript
	if runtime.GOOS == "windows" && tc.ValidateScriptWin != "" {
		validateScript = tc.ValidateScriptWin
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// Windows 环境：使用 powershell 执行
		// 加上 -NoProfile 可以避免加载冗长的用户脚本，防止命令干扰
		cmd = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", validateScript)
	} else {
		// macOS/Linux 环境：保持使用 bash
		cmd = exec.Command("bash", "-c", validateScript)
	}
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return TestResult{
			TestCaseID:   tc.ID,
			ErrorMsg:     fmt.Sprintf("execute faild,output is %s", out),
			Passed:       false,
			DurationMs:   time.Since(start).Milliseconds(),
			TotalCostCNY: session.TotalCostCNY,
		}
	}

	return TestResult{
		TestCaseID:   tc.ID,
		ErrorMsg:     fmt.Sprintf("seccess,output is %s", out),
		Passed:       false,
		DurationMs:   time.Since(start).Milliseconds(),
		TotalCostCNY: session.TotalCostCNY,
	}
}
