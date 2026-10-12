package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"non24.app/core/agentskill"
	"non24.app/desktop/internal/localagent"
)

func saveClaudePackageTo(t *testing.T, path string) {
	t.Helper()
	previous := saveClaudePackageDialog
	saveClaudePackageDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) { return path, nil }
	t.Cleanup(func() { saveClaudePackageDialog = previous })
}

func installConnector(t *testing.T, contents []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), localConnectorFile)
	if contents != nil {
		if err := os.WriteFile(path, contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	previous := installedLocalConnector
	installedLocalConnector = func() (string, error) { return path, nil }
	t.Cleanup(func() { installedLocalConnector = previous })
}

func readArchive(t *testing.T, path string) map[string][]byte {
	t.Helper()
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	files := map[string][]byte{}
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[file.Name] = body
	}
	return files
}

func localToolNames() []string {
	var names []string
	for _, tool := range localagent.ToolDefinitions(true) {
		names = append(names, tool.Name)
	}
	return names
}

// The extension holds the installed connector byte for byte, the icon, and a
// manifest that is a valid MCP Bundle manifest for exactly the tools the
// endpoint serves. It asks for nothing.
func TestTheClaudeExtensionCarriesTheConnectorAndItsTools(t *testing.T) {
	app := newTestApp(t)
	connector := []byte("synthetic connector binary")
	installConnector(t, connector)
	path := filepath.Join(t.TempDir(), "ZeitBoard.mcpb")
	saveClaudePackageTo(t, path)

	saved, err := app.SaveClaudeExtension()
	if err != nil || !saved.Saved || saved.FileName != "ZeitBoard.mcpb" {
		t.Fatalf("saved %+v, %v", saved, err)
	}
	files := readArchive(t, path)
	if len(files) != 3 || !bytes.Equal(files["server/"+localConnectorFile], connector) || !bytes.Equal(files["icon.png"], appIcon) {
		t.Fatalf("the extension holds %d files", len(files))
	}

	var manifest map[string]any
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"manifest_version", "name", "version", "description", "author", "server"} {
		if manifest[field] == nil {
			t.Errorf("manifest lacks %s", field)
		}
	}
	if manifest["manifest_version"] != "0.3" || manifest["version"] != localagent.Version ||
		!regexp.MustCompile(`^[a-z0-9-]+$`).MatchString(manifest["name"].(string)) {
		t.Errorf("manifest %v", manifest)
	}
	if _, asks := manifest["user_config"]; asks {
		t.Error("the extension asks for settings it does not need")
	}
	server := manifest["server"].(map[string]any)
	entry := server["entry_point"].(string)
	command := server["mcp_config"].(map[string]any)["command"].(string)
	if server["type"] != "binary" || files[entry] == nil || command != "${__dirname}/"+strings.TrimSuffix(entry, ".exe") {
		t.Errorf("server %v", server)
	}
	if icon := manifest["icon"].(string); files[icon] == nil {
		t.Errorf("the icon %q is not in the extension", icon)
	}

	var listed []string
	for _, tool := range manifest["tools"].([]any) {
		entry := tool.(map[string]any)
		if entry["description"] == "" {
			t.Errorf("%v has no description", entry["name"])
		}
		listed = append(listed, entry["name"].(string))
	}
	if !slices.Equal(listed, localToolNames()) {
		t.Errorf("tools %v, want %v", listed, localToolNames())
	}
	for _, name := range listed {
		if strings.Contains(name, "approve") || strings.Contains(name, "apply") {
			t.Errorf("the extension lists %s", name)
		}
	}
}

func TestWithoutTheConnectorThereIsNoExtension(t *testing.T) {
	app := newTestApp(t)
	installConnector(t, nil)
	saveClaudePackageTo(t, filepath.Join(t.TempDir(), "ZeitBoard.mcpb"))
	if _, err := app.SaveClaudeExtension(); err == nil || !strings.Contains(err.Error(), "not installed beside ZeitBoard") {
		t.Fatalf("an extension without its connector: %v", err)
	}
}

// The skill is one folder holding SKILL.md as ZeitBoard keeps it, and a
// closed dialog saves nothing.
func TestTheSkillIsSavedAsAFolder(t *testing.T) {
	app := newTestApp(t)
	path := filepath.Join(t.TempDir(), "zeitboard-skill.zip")
	saveClaudePackageTo(t, path)
	saved, err := app.SaveClaudeSkill()
	if err != nil || !saved.Saved {
		t.Fatalf("saved %+v, %v", saved, err)
	}
	files := readArchive(t, path)
	if len(files) != 1 || !bytes.Equal(files["zeitboard/SKILL.md"], agentskill.Markdown) {
		t.Fatalf("the skill archive holds %v", files)
	}

	saveClaudePackageTo(t, "")
	if canceled, err := app.SaveClaudeSkill(); err != nil || canceled.Saved {
		t.Fatalf("a closed dialog read as %+v, %v", canceled, err)
	}
}

// The skill explains every tool the connector serves, and names none it does
// not.
func TestTheSkillCoversTheConnectorsTools(t *testing.T) {
	local := localToolNames()
	mentioned := agentskill.Tools()
	for _, name := range local {
		if !slices.Contains(mentioned, name) {
			t.Errorf("SKILL.md does not mention %s", name)
		}
	}
	for _, name := range mentioned {
		if !slices.Contains(local, name) {
			t.Errorf("SKILL.md names %s, which the connector does not serve", name)
		}
	}
}

// The connector reports the product's version, and the extension with it.
func TestTheConnectorVersionIsTheProducts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var product struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &product); err != nil {
		t.Fatal(err)
	}
	if product.Version != localagent.Version {
		t.Fatalf("connector %s, product %s", localagent.Version, product.Version)
	}
}
