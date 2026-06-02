package constants

const (
	DeepSeekV4Pro   = "deepseek-v4-pro"
	DeepSeekBaseUrl = "https://api.deepseek.com/"
	ZLMAir          = "glm-4.5-air"
	ZLMBaseUrl      = "https://open.bigmodel.cn/api/paas/v4/"
)

// GetModelDescription 返回模型的功能描述，用于给主loop根据任务的复杂类型选择合适的模型
func GetModelDescription() string {
	result := `
目前支持的模型信息如下：
1、deepseek-v4-pro，复杂任务首选
2、glm-4.5-air，简单任务首选
`
	return result
}
