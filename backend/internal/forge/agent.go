package forge

// ForgeSystemPrompt is injected into conversations when forge mode is active.
const ForgeSystemPrompt = `你是 Forgify 的工具锻造助手。你的任务是帮助用户创建可运行的 Python 工具。

**工具代码规范**（必须严格遵守）：
1. 代码顶部必须有元数据注释（见下方格式）
2. 只有一个顶层函数，使用 snake_case 命名
3. 所有参数必须有类型注解（str/int/float/bool/list/dict）
4. 返回值类型必须是 dict
5. 函数第一行必须是 docstring
6. 可以使用 import（依赖会自动安装）

**必须使用以下格式**（包含元数据注释）：
` + "```python" + `
# @display_name 发送邮件
# @description 通过 SMTP 发送邮件到指定地址
# @category email

def send_email(to: str, subject: str, body: str) -> dict:
    """通过 SMTP 发送邮件到指定地址"""
    import smtplib
    # ... 实现
    return {"success": True, "message": "邮件已发送"}
` + "```" + `

元数据注释说明：
- @display_name：工具的中文显示名称（简洁，不超过10个字）
- @description：一句话描述工具功能（不超过30个字）
- @category：分类，必须是以下之一：email, data, web, file, system, other

生成代码后，在代码块下方简短说明用法。`

// DetectResult is returned when a code block is found in an AI response.
type DetectResult struct {
	Code         string
	FuncName     string
	Docstring    string
	DisplayName  string
	Description  string
	Category     string
	Params       []ParsedParam
	Requirements []string
}

// DetectCodeInResponse checks if an AI response contains a Python code block
// and parses it if found. Extracts metadata from @-comments.
func DetectCodeInResponse(content string) *DetectResult {
	code := ExtractCodeBlock(content)
	if code == "" {
		return nil
	}

	parsed := ParseFunction(code)
	if parsed.FuncName == "" {
		return nil
	}

	// Extract metadata from @-comments (same format as builtin tools)
	meta := ParseMeta(code)

	result := &DetectResult{
		Code:         code,
		FuncName:     parsed.FuncName,
		Docstring:    parsed.Docstring,
		Params:       parsed.Params,
		Requirements: parsed.Requirements,
	}

	if meta != nil {
		result.DisplayName = meta.DisplayName
		result.Description = meta.Description
		result.Category = meta.Category
	}

	// Fallback: use docstring/funcName if metadata is missing
	if result.DisplayName == "" {
		if parsed.Docstring != "" {
			result.DisplayName = parsed.Docstring
		} else {
			result.DisplayName = parsed.FuncName
		}
	}
	if result.Description == "" {
		result.Description = parsed.Docstring
	}
	if result.Category == "" {
		result.Category = "other"
	}

	return result
}
