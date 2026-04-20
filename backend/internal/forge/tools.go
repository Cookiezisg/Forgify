package forge

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/eino/components/tool"
)

// ─── UpdateToolCode: called by LLM to modify code of the bound tool ───

type UpdateToolCodeParams struct {
	Explanation string `json:"explanation"`
	Code        string `json:"code"`
}

// ToolUpdater is the callback that the tool calls to apply the change.
// It receives toolID, new code, and explanation. Returns an error if the change can't be applied.
type ToolUpdater func(ctx context.Context, code, explanation string) error

type UpdateToolCodeTool struct {
	updater ToolUpdater
}

func NewUpdateToolCodeTool(updater ToolUpdater) *UpdateToolCodeTool {
	return &UpdateToolCodeTool{updater: updater}
}

func (t *UpdateToolCodeTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "update_tool_code",
		Desc: `当用户要求修改当前绑定的工具代码时，调用此函数。
参数 code 必须是完整的 Python 函数代码（不是 diff，不是片段）。
参数 explanation 是对修改内容的简短中文说明（1-2句话）。
只有用户明确要求修改代码时才调用，不要在解释代码时调用。`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"explanation": {
				Type:     schema.String,
				Desc:     "对本次代码修改的简短中文说明",
				Required: true,
			},
			"code": {
				Type:     schema.String,
				Desc:     "修改后的完整 Python 函数代码（必须包含完整的 def 函数定义）",
				Required: true,
			},
		}),
	}, nil
}

func (t *UpdateToolCodeTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var params UpdateToolCodeParams
	if err := json.Unmarshal([]byte(argumentsInJSON), &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if params.Code == "" {
		return "错误：代码不能为空", nil
	}
	if err := t.updater(ctx, params.Code, params.Explanation); err != nil {
		return fmt.Sprintf("更新失败：%s", err.Error()), nil
	}
	return "代码已更新，用户可以在右侧面板查看变更并决定是否接受。", nil
}

// ─── CreateTool: called by LLM to create a new tool in unbound conversations ───

type CreateToolParams struct {
	Explanation string `json:"explanation"`
	Code        string `json:"code"`
}

type ToolCreator func(ctx context.Context, code, explanation string) error

type CreateToolTool struct {
	creator ToolCreator
}

func NewCreateToolTool(creator ToolCreator) *CreateToolTool {
	return &CreateToolTool{creator: creator}
}

func (t *CreateToolTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "create_tool",
		Desc: `当用户要求创建一个新的 Python 工具时，调用此函数。
参数 code 必须是完整的 Python 函数，遵循以下规范：
1. 只有一个顶层函数，使用 snake_case 命名
2. 所有参数必须有类型注解（str/int/float/bool/list/dict）
3. 返回值类型必须是 dict
4. 函数第一行必须是 docstring
参数 explanation 是对工具用途的简短中文说明。`,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"explanation": {
				Type:     schema.String,
				Desc:     "对工具用途的简短中文说明",
				Required: true,
			},
			"code": {
				Type:     schema.String,
				Desc:     "完整的 Python 函数代码",
				Required: true,
			},
		}),
	}, nil
}

func (t *CreateToolTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var params CreateToolParams
	if err := json.Unmarshal([]byte(argumentsInJSON), &params); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if params.Code == "" {
		return "错误：代码不能为空", nil
	}
	if err := t.creator(ctx, params.Code, params.Explanation); err != nil {
		return fmt.Sprintf("创建失败：%s", err.Error()), nil
	}
	return "工具已创建，用户可以在右侧面板查看并测试。", nil
}
