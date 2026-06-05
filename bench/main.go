// cmd/bench/main.go
package main

import (
	"context"
	"log"
	"os"

	"github.com/sunpuxi/go-tiny-claw/internal/eval"
)

func main() {
	if os.Getenv("ZHIPU_API_KEY") == "" {
		log.Fatal("请先导出 ZHIPU_API_KEY 环境变量进行跑分测试")
	}

	// 构建一套微型评测集
	testcases := []eval.TestCase{
		{
			ID:   "test_001_edit",
			Name: "测试模糊替换工具的准确性",
			// 准备靶机：生成一个有错误的 json 文件
			SetupScript: `echo '{"name": "tiny-claw", "version": "v1.0.0"}' > config.json`,
			// Windows PowerShell 版本：Out-File 确保 UTF-8 编码，避免 > 重定向默认输出 UTF-16 LE
			SetupScriptWin: `echo '{"name": "tiny-claw", "version": "v1.0.0"}' | Out-File -FilePath config.json -Encoding utf8`,
			// 考题：要求修改版本号
			TaskPrompt: `当前目录下有一个 config.json。请你使用 edit_file 工具，将其中的 version 从 v1.0.0 改为 v2.0.0。不要做其他多余操作。`,
			// 判卷脚本：使用 grep 检查文件是否包含 v2.0.0
			ValidateScript: `grep '"version": "v2.0.0"' config.json`,
			// Windows PowerShell 版本：用 Select-String 替代 grep，-Quiet 返回布尔值便于 if 判断退出码
			ValidateScriptWin: `if (Select-String -Quiet -Path config.json -Pattern '"version": "v2.0.0"') { exit 0 } else { exit 1 }`,
		},
		{
			ID:   "test_002_code_gen",
			Name: "测试代码阅读与创建新文件的综合能力",
			// 准备靶机：生成一个简单的乘法函数
			SetupScript: `echo 'package math\n\nfunc Multiply(a, b int) int {\n\treturn a * b\n}' > math.go`,
			// Windows PowerShell 版本：here-string 支持多行，把 \n \t 替换为真正的换行和缩进
			SetupScriptWin: "@\"\r\npackage math\r\n\r\nfunc Multiply(a, b int) int {\r\n\treturn a * b\r\n}\r\n\"@ | Out-File -FilePath math.go -Encoding utf8",
			// 考题：要求 Agent 根据刚才的代码，自己去写一份单元测试
			TaskPrompt: `当前目录下有一个 math.go。请你仔细阅读它，然后在同级目录下，帮我写一个规范的单元测试文件 math_test.go，用来测试 Multiply 函数。请务必包含正常的测试用例。`,
			// 判卷脚本：直接运行 go test！如果不通过则直接 0 分。
			ValidateScript: `go mod init bench && go test -v ./...`,
			// Windows PowerShell 版本：用 ; 替代 &&
			ValidateScriptWin: `go mod init bench; go test -v ./...`,
		},
	}

	// 启动跑分执行器！
	// 我们选用国内极其廉价但能力不错的 glm-4.5-air 跑分，省点钱。
	runner := eval.NewBenchmarkRunner("glm-4.5-air")
	runner.RunSuite(context.Background(), testcases)
}
