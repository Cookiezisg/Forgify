package builtin

import (
	"embed"
	"io/fs"
	"strings"

	"github.com/sunweilin/forgify/internal/forge"
	"github.com/sunweilin/forgify/internal/service"
)

//go:embed tools/**/*.py
var toolsFS embed.FS

// builtinMeta holds metadata parsed from @-prefixed comments in Python files.
type builtinMeta struct {
	Version     string
	Category    string
	DisplayName string
	Description string
	RequiresKey string
}

// Register scans all embedded Python tool files and upserts them into the tools table.
// Existing tools with the same version are skipped (cache hit).
func Register(toolSvc *service.ToolService) error {
	return fs.WalkDir(toolsFS, "tools", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".py") {
			return err
		}

		data, err := toolsFS.ReadFile(path)
		if err != nil {
			return nil // skip unreadable files
		}
		code := string(data)

		meta := parseMeta(code)
		if meta == nil {
			return nil // not a @builtin file
		}

		// Parse function signature
		parsed := forge.ParseFunction(code)
		if parsed.FuncName == "" {
			return nil
		}

		// Check if same version already registered
		existing, _ := toolSvc.GetByName(parsed.FuncName)
		if existing != nil && existing.Version == meta.Version && existing.Builtin {
			return nil // same version, skip
		}

		// Convert params
		params := make([]service.ToolParameter, len(parsed.Params))
		for i, p := range parsed.Params {
			params[i] = service.ToolParameter{
				Name:     p.Name,
				Type:     p.Type,
				Required: p.Required,
				Default:  p.Default,
			}
		}

		tool := &service.Tool{
			Name:        parsed.FuncName,
			DisplayName: meta.DisplayName,
			Description: meta.Description,
			Code:        code,
			Requirements: parsed.Requirements,
			Parameters:  params,
			Category:    meta.Category,
			Builtin:     true,
			Version:     meta.Version,
			RequiresKey: meta.RequiresKey,
			Status:      "tested", // built-in tools are pre-tested
		}

		// Preserve existing ID if updating
		if existing != nil {
			tool.ID = existing.ID
		}

		return toolSvc.Save(tool)
	})
}

// parseMeta extracts @builtin metadata from Python file comments.
func parseMeta(code string) *builtinMeta {
	lines := strings.Split(code, "\n")
	meta := &builtinMeta{}
	isBuiltin := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			if trimmed != "" {
				break // stop at first non-comment non-empty line
			}
			continue
		}
		comment := strings.TrimPrefix(trimmed, "#")
		comment = strings.TrimSpace(comment)

		switch {
		case comment == "@builtin":
			isBuiltin = true
		case strings.HasPrefix(comment, "@version "):
			meta.Version = strings.TrimPrefix(comment, "@version ")
		case strings.HasPrefix(comment, "@category "):
			meta.Category = strings.TrimPrefix(comment, "@category ")
		case strings.HasPrefix(comment, "@display_name "):
			meta.DisplayName = strings.TrimPrefix(comment, "@display_name ")
		case strings.HasPrefix(comment, "@description "):
			meta.Description = strings.TrimPrefix(comment, "@description ")
		case strings.HasPrefix(comment, "@requires_key "):
			meta.RequiresKey = strings.TrimPrefix(comment, "@requires_key ")
		}
	}

	if !isBuiltin {
		return nil
	}
	if meta.Version == "" {
		meta.Version = "1.0"
	}
	if meta.Category == "" {
		meta.Category = "other"
	}
	return meta
}
