package forge

// ForgeSystemPrompt is injected into conversations when forge mode is active.
const ForgeSystemPrompt = `你是 Forgify 的工具锻造助手。你的任务是帮助用户创建可运行的 Python 工具。

**工具代码规范**（必须严格遵守）：
1. 只有一个顶层函数，使用 snake_case 命名
2. 所有参数必须有类型注解（str/int/float/bool/list/dict）
3. 返回值类型必须是 dict
4. 函数第一行必须是 docstring，说明功能
5. 可以使用 import（依赖会自动安装）

**示例格式**：
` + "```python" + `
def send_email(to: str, subject: str, body: str) -> dict:
    """通过 SMTP 发送邮件到指定地址"""
    import smtplib
    # ... 实现
    return {"success": True, "message": "邮件已发送"}
` + "```" + `

生成代码后，在代码块下方简短说明用法，不需要解释每一行代码。`

// DetectResult is returned when a code block is found in an AI response.
type DetectResult struct {
	Code         string
	FuncName     string
	Docstring    string
	Params       []ParsedParam
	Requirements []string
}

// DetectCodeInResponse checks if an AI response contains a Python code block
// and parses it if found.
func DetectCodeInResponse(content string) *DetectResult {
	code := ExtractCodeBlock(content)
	if code == "" {
		return nil
	}

	parsed := ParseFunction(code)
	if parsed.FuncName == "" {
		return nil
	}

	return &DetectResult{
		Code:         code,
		FuncName:     parsed.FuncName,
		Docstring:    parsed.Docstring,
		Params:       parsed.Params,
		Requirements: parsed.Requirements,
	}
}
