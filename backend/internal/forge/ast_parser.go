package forge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"

	_ "embed"
)

//go:embed parse_function.py
var parseFunctionScript string

type astResult struct {
	Error        string     `json:"error"`
	FunctionName string     `json:"function_name"`
	Docstring    string     `json:"docstring"`
	Parameters   []astParam `json:"parameters"`
	Requirements []string   `json:"requirements"`
	DisplayName  string     `json:"display_name"`
	Description  string     `json:"description"`
	Category     string     `json:"category"`
	IsBuiltin    bool       `json:"is_builtin"`
	Version      string     `json:"version"`
	RequiresKey  string     `json:"requires_key"`
}

type astParam struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	BaseType string `json:"base_type"`
	Required bool   `json:"required"`
	Default  string `json:"default"`
}

// ParseFunctionAST uses Python's ast module to parse a function. 100% accurate.
func ParseFunctionAST(code string) (*ParseResult, *CodeMeta, error) {
	// Find python3
	pythonBin, err := findPython()
	if err != nil {
		return nil, nil, err
	}

	cmd := exec.Command(pythonBin, "-c", parseFunctionScript)
	cmd.Stdin = bytes.NewBufferString(code)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, nil, fmt.Errorf("python parse failed: %w\n%s", err, stderr.String())
	}

	var result astResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, nil, fmt.Errorf("parse JSON: %w", err)
	}

	if result.Error != "" {
		return nil, nil, fmt.Errorf("%s", result.Error)
	}

	// Convert to ParseResult
	params := make([]ParsedParam, len(result.Parameters))
	for i, p := range result.Parameters {
		params[i] = ParsedParam{
			Name:     p.Name,
			Type:     p.BaseType, // Use base type for compatibility
			FullType: p.Type,     // Full type for display
			Required: p.Required,
			Default:  p.Default,
		}
	}

	parseResult := &ParseResult{
		FuncName:     result.FunctionName,
		Docstring:    result.Docstring,
		Params:       params,
		Requirements: result.Requirements,
	}

	var meta *CodeMeta
	if result.DisplayName != "" || result.Description != "" || result.Category != "other" || result.IsBuiltin {
		meta = &CodeMeta{
			DisplayName: result.DisplayName,
			Description: result.Description,
			Category:    result.Category,
			Version:     result.Version,
			RequiresKey: result.RequiresKey,
			IsBuiltin:   result.IsBuiltin,
		}
	}

	return parseResult, meta, nil
}

func findPython() (string, error) {
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("python3 not found in PATH")
}
