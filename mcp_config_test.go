package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// mcpConfig mirrors the expected structure of .mcp.json.
type mcpConfig struct {
	MCPServers map[string]mcpServer `json:"mcpServers"`
}

type mcpServer struct {
	Type    string            `json:"type"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

func readMCPConfig(t *testing.T) mcpConfig {
	t.Helper()
	data, err := os.ReadFile(".mcp.json")
	if err != nil {
		t.Fatalf("failed to read .mcp.json: %v", err)
	}
	var cfg mcpConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to parse .mcp.json as JSON: %v", err)
	}
	return cfg
}

// --- .mcp.json tests ---

func TestMCPConfigFileExists(t *testing.T) {
	if _, err := os.Stat(".mcp.json"); os.IsNotExist(err) {
		t.Fatal(".mcp.json does not exist")
	}
}

func TestMCPConfigValidJSON(t *testing.T) {
	data, err := os.ReadFile(".mcp.json")
	if err != nil {
		t.Fatalf("failed to read .mcp.json: %v", err)
	}
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf(".mcp.json contains invalid JSON: %v", err)
	}
}

func TestMCPConfigHasMCPServersKey(t *testing.T) {
	cfg := readMCPConfig(t)
	if cfg.MCPServers == nil {
		t.Fatal(".mcp.json is missing required top-level key \"mcpServers\"")
	}
}

func TestMCPConfigLinearServerExists(t *testing.T) {
	cfg := readMCPConfig(t)
	if _, ok := cfg.MCPServers["linear"]; !ok {
		t.Fatal(".mcp.json mcpServers is missing \"linear\" entry")
	}
}

func TestMCPConfigLinearServerType(t *testing.T) {
	cfg := readMCPConfig(t)
	linear := cfg.MCPServers["linear"]
	if linear.Type != "http" {
		t.Fatalf("linear server type: want \"http\", got %q", linear.Type)
	}
}

func TestMCPConfigLinearServerURL(t *testing.T) {
	cfg := readMCPConfig(t)
	linear := cfg.MCPServers["linear"]
	const wantURL = "https://mcp.linear.app/mcp"
	if linear.URL != wantURL {
		t.Fatalf("linear server url: want %q, got %q", wantURL, linear.URL)
	}
}

func TestMCPConfigLinearAuthorizationHeaderExists(t *testing.T) {
	cfg := readMCPConfig(t)
	linear := cfg.MCPServers["linear"]
	if linear.Headers == nil {
		t.Fatal("linear server has no headers configured")
	}
	if _, ok := linear.Headers["Authorization"]; !ok {
		t.Fatal("linear server headers missing \"Authorization\" key")
	}
}

func TestMCPConfigLinearAuthorizationHeaderFormat(t *testing.T) {
	cfg := readMCPConfig(t)
	linear := cfg.MCPServers["linear"]
	authValue := linear.Headers["Authorization"]
	if !strings.HasPrefix(authValue, "Bearer ") {
		t.Fatalf("Authorization header should start with \"Bearer \", got %q", authValue)
	}
}

func TestMCPConfigLinearAuthorizationUsesEnvVar(t *testing.T) {
	cfg := readMCPConfig(t)
	linear := cfg.MCPServers["linear"]
	authValue := linear.Headers["Authorization"]
	if !strings.Contains(authValue, "${LINEAR_API_KEY}") {
		t.Fatalf("Authorization header should reference ${LINEAR_API_KEY}, got %q", authValue)
	}
}

func TestMCPConfigLinearAuthorizationExactValue(t *testing.T) {
	cfg := readMCPConfig(t)
	linear := cfg.MCPServers["linear"]
	const want = "Bearer ${LINEAR_API_KEY}"
	got := linear.Headers["Authorization"]
	if got != want {
		t.Fatalf("Authorization header: want %q, got %q", want, got)
	}
}

// TestMCPConfigOnlyKnownServers ensures no unexpected server entries have been added.
func TestMCPConfigOnlyKnownServers(t *testing.T) {
	cfg := readMCPConfig(t)
	known := map[string]bool{"linear": true}
	for name := range cfg.MCPServers {
		if !known[name] {
			t.Errorf("unexpected server %q found in .mcp.json mcpServers", name)
		}
	}
}

// TestMCPConfigLinearServerHasNoExtraFields ensures the linear server block
// contains exactly the expected keys (type, url, headers) and nothing else.
func TestMCPConfigLinearServerNoExtraTopLevelKeys(t *testing.T) {
	data, err := os.ReadFile(".mcp.json")
	if err != nil {
		t.Fatalf("failed to read .mcp.json: %v", err)
	}
	// Unmarshal into a raw map to inspect actual keys.
	var raw map[string]map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to parse .mcp.json: %v", err)
	}
	linearRaw, ok := raw["mcpServers"]["linear"].(map[string]interface{})
	if !ok {
		t.Fatal("linear server entry is not an object")
	}
	allowed := map[string]bool{"type": true, "url": true, "headers": true}
	for k := range linearRaw {
		if !allowed[k] {
			t.Errorf("unexpected key %q in linear server config", k)
		}
	}
}

// --- README.md tests (for the added "Development with Claude Code" section) ---

func readREADME(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}
	return string(data)
}

func TestReadmeHasClaudeCodeSection(t *testing.T) {
	content := readREADME(t)
	if !strings.Contains(content, "Development with Claude Code") {
		t.Fatal("README.md missing \"Development with Claude Code\" section")
	}
}

func TestReadmeMentionsMCPJsonFile(t *testing.T) {
	content := readREADME(t)
	if !strings.Contains(content, ".mcp.json") {
		t.Fatal("README.md should reference the .mcp.json configuration file")
	}
}

func TestReadmeDocumentsLinearAPIKeyEnvVar(t *testing.T) {
	content := readREADME(t)
	if !strings.Contains(content, "LINEAR_API_KEY") {
		t.Fatal("README.md should document the LINEAR_API_KEY environment variable")
	}
}

func TestReadmeMentionsMCPLinearEndpoint(t *testing.T) {
	content := readREADME(t)
	if !strings.Contains(content, "linear.app") {
		t.Fatal("README.md should mention the Linear MCP endpoint (linear.app)")
	}
}

func TestReadmeClaudeCodeSectionDocumentsInstallStep(t *testing.T) {
	content := readREADME(t)
	if !strings.Contains(content, "npm install") || !strings.Contains(content, "@anthropic-ai/claude-code") {
		t.Fatal("README.md Claude Code section should document the npm install command for @anthropic-ai/claude-code")
	}
}

func TestReadmeClaudeCodeSectionMentionsNetworkNote(t *testing.T) {
	content := readREADME(t)
	if !strings.Contains(content, "internet access") && !strings.Contains(content, "network") {
		t.Fatal("README.md Claude Code section should note network access requirements")
	}
}

// --- CLAUDE.md tests ---

func readCLAUDEMD(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("CLAUDE.md")
	if err != nil {
		t.Fatalf("failed to read CLAUDE.md: %v", err)
	}
	return string(data)
}

func TestCLAUDEMDExists(t *testing.T) {
	if _, err := os.Stat("CLAUDE.md"); os.IsNotExist(err) {
		t.Fatal("CLAUDE.md does not exist")
	}
}

func TestCLAUDEMDHasProjectOverviewSection(t *testing.T) {
	content := readCLAUDEMD(t)
	if !strings.Contains(content, "Project Overview") {
		t.Fatal("CLAUDE.md is missing \"Project Overview\" section")
	}
}

func TestCLAUDEMDHasRepositoryLayoutSection(t *testing.T) {
	content := readCLAUDEMD(t)
	if !strings.Contains(content, "Repository Layout") {
		t.Fatal("CLAUDE.md is missing \"Repository Layout\" section")
	}
}

func TestCLAUDEMDHasArchitectureSection(t *testing.T) {
	content := readCLAUDEMD(t)
	if !strings.Contains(content, "Architecture") {
		t.Fatal("CLAUDE.md is missing \"Architecture\" section")
	}
}

func TestCLAUDEMDHasBuildCommandsSection(t *testing.T) {
	content := readCLAUDEMD(t)
	if !strings.Contains(content, "Build") {
		t.Fatal("CLAUDE.md is missing build commands documentation")
	}
}

func TestCLAUDEMDDocumentsProtocolPackage(t *testing.T) {
	content := readCLAUDEMD(t)
	if !strings.Contains(content, "pkg/protocol") {
		t.Fatal("CLAUDE.md should reference pkg/protocol as the public protocol package")
	}
}

func TestCLAUDEMDDocumentsKeyFilePaths(t *testing.T) {
	content := readCLAUDEMD(t)
	requiredPaths := []string{
		"internal/server/control/handler.go",
		"internal/server/registry/registry.go",
		"internal/server/auth/auth.go",
		"internal/server/config/config.go",
	}
	for _, path := range requiredPaths {
		if !strings.Contains(content, path) {
			t.Errorf("CLAUDE.md should document key file path %q", path)
		}
	}
}

func TestCLAUDEMDHasCodeConventionsSection(t *testing.T) {
	content := readCLAUDEMD(t)
	if !strings.Contains(content, "Code Conventions") {
		t.Fatal("CLAUDE.md is missing \"Code Conventions\" section")
	}
}

func TestCLAUDEMDHasCommonPitfallsSection(t *testing.T) {
	content := readCLAUDEMD(t)
	if !strings.Contains(content, "Common Pitfalls") {
		t.Fatal("CLAUDE.md is missing \"Common Pitfalls\" section")
	}
}

// TestCLAUDEMDNotEmpty guards against an accidentally empty file.
func TestCLAUDEMDNotEmpty(t *testing.T) {
	info, err := os.Stat("CLAUDE.md")
	if err != nil {
		t.Fatalf("cannot stat CLAUDE.md: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("CLAUDE.md must not be empty")
	}
}