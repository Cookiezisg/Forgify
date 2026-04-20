package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sunweilin/forgify/internal/storage"
)

type ToolParameter struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Default  string `json:"default,omitempty"`
	Doc      string `json:"doc,omitempty"`
}

type Tool struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	DisplayName    string          `json:"displayName"`
	Description    string          `json:"description"`
	Code           string          `json:"code"`
	Requirements   []string        `json:"requirements"`
	Parameters     []ToolParameter `json:"parameters"`
	Category       string          `json:"category"`
	Status         string          `json:"status"`
	Builtin        bool            `json:"builtin"`
	Version        string          `json:"version"`
	RequiresKey    string          `json:"requiresKey,omitempty"`
	LastTestAt     *time.Time      `json:"lastTestAt,omitempty"`
	LastTestPassed *bool           `json:"lastTestPassed,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

type ToolTestRecord struct {
	ID         string    `json:"id"`
	ToolID     string    `json:"toolId"`
	Passed     bool      `json:"passed"`
	DurationMs int64     `json:"durationMs"`
	InputJSON  string    `json:"inputJson,omitempty"`
	OutputJSON string    `json:"outputJson,omitempty"`
	ErrorMsg   string    `json:"errorMsg,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type ToolService struct{}

func NewToolService() *ToolService {
	return &ToolService{}
}

func (s *ToolService) Save(t *Tool) error {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	now := time.Now()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	if t.Requirements == nil {
		t.Requirements = []string{}
	}
	if t.Parameters == nil {
		t.Parameters = []ToolParameter{}
	}

	// Auto-version: if updating and code changed, save the old version
	if t.ID != "" {
		var oldCode string
		if err := storage.DB().QueryRow(`SELECT code FROM tools WHERE id=?`, t.ID).Scan(&oldCode); err == nil {
			if oldCode != "" && oldCode != t.Code {
				oldLines := strings.Split(oldCode, "\n")
				newLines := strings.Split(t.Code, "\n")
				summary := fmt.Sprintf("+%d / -%d 行", countDiff(oldLines, newLines), countDiff(newLines, oldLines))
				s.SaveVersion(t.ID, oldCode, summary)
			}
		}
	}

	reqJSON, _ := json.Marshal(t.Requirements)
	paramsJSON, _ := json.Marshal(t.Parameters)

	_, err := storage.DB().Exec(`
		INSERT INTO tools (id, name, display_name, description, code, requirements,
		                   parameters, category, status, builtin, version, requires_key,
		                   created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, display_name=excluded.display_name,
			description=excluded.description, code=excluded.code,
			requirements=excluded.requirements, parameters=excluded.parameters,
			category=excluded.category, status=excluded.status,
			version=excluded.version, requires_key=excluded.requires_key,
			updated_at=excluded.updated_at
	`, t.ID, t.Name, t.DisplayName, t.Description, t.Code,
		string(reqJSON), string(paramsJSON), t.Category, t.Status,
		t.Builtin, t.Version, nullStr(t.RequiresKey),
		t.CreatedAt.Format(time.DateTime), now.Format(time.DateTime))
	return err
}

func (s *ToolService) Get(id string) (*Tool, error) {
	tools, err := s.scan(`
		SELECT id, name, display_name, description, code, requirements,
		       parameters, category, status, builtin, version, requires_key,
		       last_test_at, last_test_passed, created_at, updated_at
		FROM tools WHERE id = ?`, id)
	if err != nil || len(tools) == 0 {
		return nil, err
	}
	return tools[0], nil
}

func (s *ToolService) GetByName(name string) (*Tool, error) {
	tools, err := s.scan(`
		SELECT id, name, display_name, description, code, requirements,
		       parameters, category, status, builtin, version, requires_key,
		       last_test_at, last_test_passed, created_at, updated_at
		FROM tools WHERE name = ?`, name)
	if err != nil || len(tools) == 0 {
		return nil, err
	}
	return tools[0], nil
}

func (s *ToolService) List(category, query string) ([]*Tool, error) {
	q := `SELECT id, name, display_name, description, code, requirements,
	             parameters, category, status, builtin, version, requires_key,
	             last_test_at, last_test_passed, created_at, updated_at
	      FROM tools WHERE 1=1`
	var args []any

	if category != "" && category != "all" {
		q += " AND category = ?"
		args = append(args, category)
	}
	if query != "" {
		q += " AND (name LIKE ? OR display_name LIKE ? OR description LIKE ?)"
		like := "%" + query + "%"
		args = append(args, like, like, like)
	}
	q += " ORDER BY builtin DESC, updated_at DESC"
	return s.scan(q, args...)
}

func (s *ToolService) Delete(id string) error {
	// Don't allow deleting built-in tools
	var builtin bool
	storage.DB().QueryRow("SELECT builtin FROM tools WHERE id=?", id).Scan(&builtin)
	if builtin {
		return nil
	}
	_, err := storage.DB().Exec("DELETE FROM tools WHERE id = ?", id)
	return err
}

func (s *ToolService) UpdateTestResult(id string, passed bool) error {
	now := time.Now().Format(time.DateTime)
	status := "tested"
	if !passed {
		status = "failed"
	}
	_, err := storage.DB().Exec(`
		UPDATE tools SET status=?, last_test_at=?, last_test_passed=?, updated_at=?
		WHERE id=?`, status, now, passed, now, id)
	return err
}

func (s *ToolService) SaveTestRecord(rec *ToolTestRecord) error {
	if rec.ID == "" {
		rec.ID = uuid.NewString()
	}
	_, err := storage.DB().Exec(`
		INSERT INTO tool_test_history (id, tool_id, passed, duration_ms, input_json, output_json, error_msg)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rec.ID, rec.ToolID, rec.Passed, rec.DurationMs,
		rec.InputJSON, rec.OutputJSON, rec.ErrorMsg)

	// Trim to 20 most recent records
	storage.DB().Exec(`
		DELETE FROM tool_test_history WHERE tool_id=? AND id NOT IN (
			SELECT id FROM tool_test_history WHERE tool_id=? ORDER BY created_at DESC LIMIT 20
		)`, rec.ToolID, rec.ToolID)

	return err
}

func (s *ToolService) ListTestHistory(toolID string) ([]*ToolTestRecord, error) {
	rows, err := storage.DB().Query(`
		SELECT id, tool_id, passed, duration_ms,
		       COALESCE(input_json,''), COALESCE(output_json,''), COALESCE(error_msg,''),
		       created_at
		FROM tool_test_history
		WHERE tool_id = ? ORDER BY created_at DESC LIMIT 20`, toolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*ToolTestRecord
	for rows.Next() {
		r := &ToolTestRecord{}
		var created sql.NullString
		if err := rows.Scan(&r.ID, &r.ToolID, &r.Passed, &r.DurationMs,
			&r.InputJSON, &r.OutputJSON, &r.ErrorMsg, &created); err != nil {
			return nil, err
		}
		r.CreatedAt = parseSQLTime(created)
		records = append(records, r)
	}
	if records == nil {
		records = []*ToolTestRecord{}
	}
	return records, rows.Err()
}

// ─── Pending Changes (review-before-apply) ───

func (s *ToolService) SetPendingChange(id, code, summary string) error {
	_, err := storage.DB().Exec(`
		UPDATE tools SET pending_code=?, pending_summary=?, updated_at=datetime('now')
		WHERE id=?`, code, summary, id)
	return err
}

func (s *ToolService) AcceptPendingChange(id string) error {
	tool, err := s.Get(id)
	if err != nil || tool == nil {
		return fmt.Errorf("tool not found")
	}

	var pendingCode, pendingSummary sql.NullString
	storage.DB().QueryRow(`SELECT pending_code, pending_summary FROM tools WHERE id=?`, id).Scan(&pendingCode, &pendingSummary)
	if !pendingCode.Valid || pendingCode.String == "" {
		return fmt.Errorf("no pending change")
	}

	// Save current code as version before overwriting
	if tool.Code != "" {
		summary := pendingSummary.String
		if summary == "" {
			summary = "accepted change"
		}
		s.SaveVersion(id, tool.Code, summary)
	}

	// Apply pending code
	_, err = storage.DB().Exec(`
		UPDATE tools SET code=pending_code, pending_code=NULL, pending_summary=NULL, updated_at=datetime('now')
		WHERE id=?`, id)
	return err
}

func (s *ToolService) RejectPendingChange(id string) error {
	_, err := storage.DB().Exec(`
		UPDATE tools SET pending_code=NULL, pending_summary=NULL, updated_at=datetime('now')
		WHERE id=?`, id)
	return err
}

func (s *ToolService) GetPendingChange(id string) (code string, summary string, hasPending bool) {
	var pc, ps sql.NullString
	storage.DB().QueryRow(`SELECT pending_code, pending_summary FROM tools WHERE id=?`, id).Scan(&pc, &ps)
	if pc.Valid && pc.String != "" {
		return pc.String, ps.String, true
	}
	return "", "", false
}

// ─── Metadata editing ───

func (s *ToolService) UpdateMeta(id string, displayName, description, category *string) error {
	sets := []string{"updated_at=datetime('now')"}
	args := []any{}
	if displayName != nil {
		sets = append(sets, "display_name=?")
		args = append(args, *displayName)
	}
	if description != nil {
		sets = append(sets, "description=?")
		args = append(args, *description)
	}
	if category != nil {
		sets = append(sets, "category=?")
		args = append(args, *category)
	}
	args = append(args, id)
	q := "UPDATE tools SET " + strings.Join(sets, ", ") + " WHERE id=?"
	_, err := storage.DB().Exec(q, args...)
	return err
}

// ─── Tags ───

func (s *ToolService) AddTag(id, tag string) error {
	_, err := storage.DB().Exec(`INSERT OR IGNORE INTO tool_tags (tool_id, tag) VALUES (?, ?)`, id, tag)
	return err
}

func (s *ToolService) RemoveTag(id, tag string) error {
	_, err := storage.DB().Exec(`DELETE FROM tool_tags WHERE tool_id=? AND tag=?`, id, tag)
	return err
}

func (s *ToolService) ListTags(id string) ([]string, error) {
	rows, err := storage.DB().Query(`SELECT tag FROM tool_tags WHERE tool_id=? ORDER BY tag`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var tag string
		rows.Scan(&tag)
		tags = append(tags, tag)
	}
	if tags == nil {
		tags = []string{}
	}
	return tags, nil
}

// ─── Versions ───

type ToolVersion struct {
	ID            string    `json:"id"`
	ToolID        string    `json:"toolId"`
	Version       int       `json:"version"`
	Code          string    `json:"code"`
	ChangeSummary string    `json:"changeSummary"`
	CreatedAt     time.Time `json:"createdAt"`
}

func (s *ToolService) SaveVersion(toolID, code, summary string) error {
	// Get next version number
	var maxVer int
	storage.DB().QueryRow(`SELECT COALESCE(MAX(version), 0) FROM tool_versions WHERE tool_id=?`, toolID).Scan(&maxVer)
	_, err := storage.DB().Exec(`
		INSERT INTO tool_versions (id, tool_id, version, code, change_summary)
		VALUES (?, ?, ?, ?, ?)`,
		uuid.NewString(), toolID, maxVer+1, code, summary)
	return err
}

func (s *ToolService) ListVersions(toolID string) ([]*ToolVersion, error) {
	rows, err := storage.DB().Query(`
		SELECT id, tool_id, version, code, change_summary, created_at
		FROM tool_versions WHERE tool_id=? ORDER BY version DESC LIMIT 50`, toolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var versions []*ToolVersion
	for rows.Next() {
		v := &ToolVersion{}
		var created sql.NullString
		if err := rows.Scan(&v.ID, &v.ToolID, &v.Version, &v.Code, &v.ChangeSummary, &created); err != nil {
			return nil, err
		}
		v.CreatedAt = parseSQLTime(created)
		versions = append(versions, v)
	}
	if versions == nil {
		versions = []*ToolVersion{}
	}
	return versions, nil
}

func (s *ToolService) RestoreVersion(toolID string, version int) error {
	var code string
	err := storage.DB().QueryRow(`SELECT code FROM tool_versions WHERE tool_id=? AND version=?`,
		toolID, version).Scan(&code)
	if err != nil {
		return fmt.Errorf("version %d not found", version)
	}
	tool, err := s.Get(toolID)
	if err != nil || tool == nil {
		return fmt.Errorf("tool not found")
	}
	tool.Code = code
	return s.Save(tool)
}

// ─── Test Cases ───

type ToolTestCase struct {
	ID         string    `json:"id"`
	ToolID     string    `json:"toolId"`
	Name       string    `json:"name"`
	ParamsJSON string    `json:"paramsJson"`
	CreatedAt  time.Time `json:"createdAt"`
}

func (s *ToolService) SaveTestCase(toolID, name, paramsJSON string) error {
	_, err := storage.DB().Exec(`
		INSERT INTO tool_test_cases (id, tool_id, name, params_json) VALUES (?, ?, ?, ?)`,
		uuid.NewString(), toolID, name, paramsJSON)
	return err
}

func (s *ToolService) ListTestCases(toolID string) ([]*ToolTestCase, error) {
	rows, err := storage.DB().Query(`
		SELECT id, tool_id, name, params_json, created_at
		FROM tool_test_cases WHERE tool_id=? ORDER BY created_at DESC`, toolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cases []*ToolTestCase
	for rows.Next() {
		c := &ToolTestCase{}
		var created sql.NullString
		if err := rows.Scan(&c.ID, &c.ToolID, &c.Name, &c.ParamsJSON, &created); err != nil {
			return nil, err
		}
		c.CreatedAt = parseSQLTime(created)
		cases = append(cases, c)
	}
	if cases == nil {
		cases = []*ToolTestCase{}
	}
	return cases, nil
}

func (s *ToolService) DeleteTestCase(id string) error {
	_, err := storage.DB().Exec(`DELETE FROM tool_test_cases WHERE id=?`, id)
	return err
}

func (s *ToolService) scan(query string, args ...any) ([]*Tool, error) {
	rows, err := storage.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tools []*Tool
	for rows.Next() {
		t := &Tool{}
		var reqJSON, paramsJSON string
		var requiresKey sql.NullString
		var lastTestAt sql.NullString
		var lastTestPassed sql.NullBool
		var created, updated sql.NullString

		if err := rows.Scan(&t.ID, &t.Name, &t.DisplayName, &t.Description, &t.Code,
			&reqJSON, &paramsJSON, &t.Category, &t.Status,
			&t.Builtin, &t.Version, &requiresKey,
			&lastTestAt, &lastTestPassed, &created, &updated); err != nil {
			return nil, err
		}

		json.Unmarshal([]byte(reqJSON), &t.Requirements)
		json.Unmarshal([]byte(paramsJSON), &t.Parameters)
		if t.Requirements == nil {
			t.Requirements = []string{}
		}
		if t.Parameters == nil {
			t.Parameters = []ToolParameter{}
		}
		if requiresKey.Valid {
			t.RequiresKey = requiresKey.String
		}
		if lastTestPassed.Valid {
			v := lastTestPassed.Bool
			t.LastTestPassed = &v
		}
		t.CreatedAt = parseSQLTime(created)
		t.UpdatedAt = parseSQLTime(updated)
		if lastTestAt.Valid && lastTestAt.String != "" {
			lt := parseSQLTime(lastTestAt)
			t.LastTestAt = &lt
		}

		tools = append(tools, t)
	}
	if tools == nil {
		tools = []*Tool{}
	}
	return tools, rows.Err()
}

// ─── Import / Export ───

type ToolPackage struct {
	Version    string     `json:"version"`
	ExportedAt time.Time  `json:"exported_at"`
	Tool       ToolExport `json:"tool"`
}

type ToolExport struct {
	Name         string   `json:"name"`
	DisplayName  string   `json:"display_name"`
	Description  string   `json:"description"`
	Category     string   `json:"category"`
	Code         string   `json:"code"`
	Requirements []string `json:"requirements"`
}

type ImportResult struct {
	Tool         *Tool  `json:"tool"`
	ConflictName string `json:"conflictName,omitempty"`
	ConflictID   string `json:"conflictId,omitempty"`
}

func (s *ToolService) Export(toolID string) ([]byte, error) {
	tool, err := s.Get(toolID)
	if err != nil || tool == nil {
		return nil, err
	}
	pkg := ToolPackage{
		Version:    "1.0",
		ExportedAt: time.Now(),
		Tool: ToolExport{
			Name:         tool.Name,
			DisplayName:  tool.DisplayName,
			Description:  tool.Description,
			Category:     tool.Category,
			Code:         tool.Code,
			Requirements: tool.Requirements,
		},
	}
	return json.MarshalIndent(pkg, "", "  ")
}

func (s *ToolService) ParseImport(data []byte) (*ImportResult, error) {
	var pkg ToolPackage
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("无效的工具文件格式")
	}
	if pkg.Tool.Code == "" {
		return nil, fmt.Errorf("工具文件缺少必要字段")
	}
	result := &ImportResult{
		Tool: &Tool{
			Name:         pkg.Tool.Name,
			DisplayName:  pkg.Tool.DisplayName,
			Description:  pkg.Tool.Description,
			Category:     pkg.Tool.Category,
			Code:         pkg.Tool.Code,
			Requirements: pkg.Tool.Requirements,
			Status:       "draft",
		},
	}
	// Check name conflict
	existing, _ := s.GetByName(pkg.Tool.Name)
	if existing != nil {
		result.ConflictName = existing.DisplayName
		result.ConflictID = existing.ID
	}
	return result, nil
}

func (s *ToolService) ConfirmImport(data []byte, action, replaceID string) (*Tool, error) {
	result, err := s.ParseImport(data)
	if err != nil {
		return nil, err
	}
	tool := result.Tool
	switch action {
	case "replace":
		if replaceID != "" {
			tool.ID = replaceID
		}
	case "rename":
		tool.Name = tool.Name + "_imported"
		tool.DisplayName = tool.DisplayName + " (导入)"
	default: // "new"
	}
	if err := s.Save(tool); err != nil {
		return nil, err
	}
	return tool, nil
}

// parseSQLTime tries multiple formats for SQLite datetime strings.
func parseSQLTime(s sql.NullString) time.Time {
	if !s.Valid || s.String == "" {
		return time.Now()
	}
	for _, layout := range []string{
		time.DateTime,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, s.String); err == nil {
			return t
		}
	}
	return time.Now()
}

// countDiff counts lines in a that are not in b (simple line diff).
func countDiff(a, b []string) int {
	bSet := make(map[string]bool, len(b))
	for _, line := range b {
		bSet[line] = true
	}
	count := 0
	for _, line := range a {
		if !bSet[line] {
			count++
		}
	}
	return count
}

// nullStr converts empty string to SQL NULL.
func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
