package forge

import (
	"regexp"
	"strings"
)

// ParsedParam represents a function parameter extracted from Python code.
type ParsedParam struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Default  string `json:"default,omitempty"`
}

var (
	reFuncDef   = regexp.MustCompile(`(?m)^def\s+(\w+)\s*\((.*?)\)\s*(?:->\s*\w+)?\s*:`)
	reParam     = regexp.MustCompile(`(\w+)\s*:\s*(\w+)(?:\s*=\s*(.+))?`)
	reImport    = regexp.MustCompile(`(?m)^(?:import|from)\s+(\S+)`)
	reDocstring = regexp.MustCompile(`(?s)(?:"""(.+?)"""|'''(.+?)''')`)
)

var stdlibPackages = map[string]bool{
	"os": true, "sys": true, "json": true, "re": true,
	"datetime": true, "time": true, "math": true, "random": true,
	"collections": true, "itertools": true, "functools": true,
	"pathlib": true, "io": true, "typing": true, "dataclasses": true,
	"enum": true, "abc": true, "copy": true, "hashlib": true,
	"hmac": true, "base64": true, "urllib": true, "http": true,
	"email": true, "smtplib": true, "csv": true, "sqlite3": true,
	"subprocess": true, "threading": true, "multiprocessing": true,
	"logging": true, "unittest": true, "contextlib": true,
	"string": true, "struct": true, "textwrap": true, "unicodedata": true,
	"decimal": true, "fractions": true, "statistics": true,
	"pprint": true, "tempfile": true, "shutil": true, "glob": true,
	"configparser": true, "argparse": true, "warnings": true,
	"traceback": true, "inspect": true, "types": true, "weakref": true,
	"array": true, "heapq": true, "bisect": true, "queue": true,
	"socket": true, "ssl": true, "html": true, "xml": true,
	"uuid": true, "platform": true, "locale": true,
	"pickle": true, "operator": true, "asyncio": true, "concurrent": true,
	"secrets": true, "zipfile": true, "tarfile": true, "gzip": true,
}

// ParseResult holds everything extracted from a Python tool function.
type ParseResult struct {
	FuncName     string
	Docstring    string
	Params       []ParsedParam
	Requirements []string
}

// ParseFunction extracts function metadata from Python source code.
func ParseFunction(code string) *ParseResult {
	result := &ParseResult{}
	lines := strings.Split(code, "\n")

	// Extract function name and parameters
	for _, line := range lines {
		if m := reFuncDef.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			result.FuncName = m[1]
			for _, pm := range reParam.FindAllStringSubmatch(m[2], -1) {
				p := ParsedParam{
					Name:     pm[1],
					Type:     normalizeType(pm[2]),
					Required: true,
				}
				if len(pm) > 3 && strings.TrimSpace(pm[3]) != "" {
					p.Required = false
					p.Default = strings.TrimSpace(pm[3])
				}
				result.Params = append(result.Params, p)
			}
			break
		}
	}

	// Extract docstring
	if m := reDocstring.FindStringSubmatch(code); m != nil {
		if m[1] != "" {
			result.Docstring = strings.TrimSpace(m[1])
		} else if m[2] != "" {
			result.Docstring = strings.TrimSpace(m[2])
		}
	}

	// Extract third-party requirements
	seen := map[string]bool{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if m := reImport.FindStringSubmatch(trimmed); m != nil {
			pkg := strings.Split(m[1], ".")[0]
			if !stdlibPackages[pkg] && !seen[pkg] && pkg != "" {
				result.Requirements = append(result.Requirements, pkg)
				seen[pkg] = true
			}
		}
	}

	return result
}

// ExtractCodeBlock finds the first ```python code block in markdown content.
func ExtractCodeBlock(content string) string {
	start := strings.Index(content, "```python")
	if start == -1 {
		return ""
	}
	start += len("```python")
	rest := content[start:]
	end := strings.Index(rest, "```")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

// CodeMeta holds metadata extracted from @-prefixed comments in Python code.
type CodeMeta struct {
	DisplayName string
	Description string
	Category    string
	Version     string
	RequiresKey string
	IsBuiltin   bool
}

// ParseMeta extracts @display_name, @description, @category etc. from Python code comments.
// Works for both builtin tools (@builtin required) and user-generated tools (@builtin optional).
func ParseMeta(code string) *CodeMeta {
	lines := strings.Split(code, "\n")
	meta := &CodeMeta{}
	found := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			if trimmed != "" {
				break
			}
			continue
		}
		comment := strings.TrimPrefix(trimmed, "#")
		comment = strings.TrimSpace(comment)

		switch {
		case comment == "@builtin":
			meta.IsBuiltin = true
			found = true
		case strings.HasPrefix(comment, "@version "):
			meta.Version = strings.TrimPrefix(comment, "@version ")
			found = true
		case strings.HasPrefix(comment, "@category "):
			meta.Category = strings.TrimPrefix(comment, "@category ")
			found = true
		case strings.HasPrefix(comment, "@display_name "):
			meta.DisplayName = strings.TrimPrefix(comment, "@display_name ")
			found = true
		case strings.HasPrefix(comment, "@description "):
			meta.Description = strings.TrimPrefix(comment, "@description ")
			found = true
		case strings.HasPrefix(comment, "@requires_key "):
			meta.RequiresKey = strings.TrimPrefix(comment, "@requires_key ")
			found = true
		}
	}

	if !found {
		return nil
	}
	if meta.Category == "" {
		meta.Category = "other"
	}
	if meta.Version == "" {
		meta.Version = "1.0"
	}
	return meta
}

func normalizeType(t string) string {
	switch t {
	case "str":
		return "string"
	case "int":
		return "int"
	case "float":
		return "float"
	case "bool":
		return "bool"
	case "list", "List":
		return "list"
	case "dict", "Dict":
		return "dict"
	default:
		return "string"
	}
}
