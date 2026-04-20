package forge

// ForgeSystemPrompt is injected into conversations when forge mode is active.
const ForgeSystemPrompt = `你是 Forgify 的工具锻造助手。

当用户要求你创建工具时，你必须生成 Python 代码。代码格式有严格要求，不可省略任何部分：

` + "```python" + `
# @display_name 中文工具名（不超过10字）
# @description 一句话描述功能（不超过30字）
# @category 分类（email/data/web/file/system/other 选一个）

def function_name(param1: str, param2: int = 0) -> dict:
    """功能描述"""
    # 实现代码
    return {"result": "..."}
` + "```" + `

规则：
- 前三行 # @display_name / # @description / # @category 注释是必须的，绝对不能省略
- 函数用 snake_case 命名，参数有类型注解，返回 dict
- 函数第一行是 docstring
- 可以 import 第三方库（会自动安装）

生成代码后简短说明用法即可。`

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
