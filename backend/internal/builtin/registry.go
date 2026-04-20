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

		meta := forge.ParseMeta(code)
		if meta == nil || !meta.IsBuiltin {
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

// parseMeta is now in forge.ParseMeta — shared between builtin and user-generated tools.
