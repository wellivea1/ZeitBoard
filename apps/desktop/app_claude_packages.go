package main

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"non24.app/core/agentskill"
	"non24.app/desktop/internal/localagent"
)

// ZeitBoard packaged for Claude (ADR-0061). The desktop writes two files where
// the owner chooses: a Claude Desktop extension holding the installed local
// connector, and the skill that tells the model how to use it. Neither holds
// data, passwords or tokens.

//go:embed build/appicon.png
var appIcon []byte

var saveClaudePackageDialog = runtime.SaveFileDialog

// installedLocalConnector is the bridge the installer puts beside ZeitBoard.
var installedLocalConnector = func() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(executable), localConnectorFile), nil
}

const (
	localConnectorFile = "zeitboard-local-mcp.exe"

	// claudeExtensionManifestVersion is the MCP Bundle manifest version the
	// extension is written to.
	claudeExtensionManifestVersion = "0.3"
)

type ClaudePackageDTO struct {
	FileName string `json:"fileName"`
	Saved    bool   `json:"saved"`
}

// SaveClaudeExtension writes ZeitBoard.mcpb: the installed connector and a
// manifest listing the tools it serves. Opening it adds ZeitBoard to Claude
// Desktop.
func (a *App) SaveClaudeExtension() (ClaudePackageDTO, error) {
	connector, err := installedLocalConnector()
	if err != nil {
		return ClaudePackageDTO{}, errors.New("ZeitBoard could not find where it is installed.")
	}
	if info, err := os.Stat(connector); err != nil || !info.Mode().IsRegular() {
		return ClaudePackageDTO{}, errors.New("The local connector is not installed beside ZeitBoard. Install ZeitBoard again with its installer, which includes it.")
	}
	return a.saveClaudePackage("Save ZeitBoard for Claude Desktop", "ZeitBoard.mcpb",
		runtime.FileFilter{DisplayName: "Claude Desktop extension (*.mcpb)", Pattern: "*.mcpb"},
		func(out io.Writer) error { return writeClaudeExtension(out, connector) })
}

// SaveClaudeSkill writes zeitboard-skill.zip, the skill to upload in Claude's
// settings or to unpack into Claude Code's skills folder.
func (a *App) SaveClaudeSkill() (ClaudePackageDTO, error) {
	return a.saveClaudePackage("Save the ZeitBoard skill", "zeitboard-skill.zip",
		runtime.FileFilter{DisplayName: "Zip files (*.zip)", Pattern: "*.zip"}, writeClaudeSkill)
}

func (a *App) saveClaudePackage(title, name string, filter runtime.FileFilter, write func(io.Writer) error) (ClaudePackageDTO, error) {
	path, err := saveClaudePackageDialog(a.applicationContext(), runtime.SaveDialogOptions{
		Title: title, DefaultFilename: name, CanCreateDirectories: true, Filters: []runtime.FileFilter{filter},
	})
	if err != nil {
		return ClaudePackageDTO{}, errors.New("The place to save it could not be chosen.")
	}
	if path == "" {
		return ClaudePackageDTO{}, nil
	}
	if err := writePrivateStreamAtomic(path, write); err != nil {
		return ClaudePackageDTO{}, errors.New("It could not be saved there. Nothing was replaced.")
	}
	return ClaudePackageDTO{FileName: filepath.Base(path), Saved: true}, nil
}

// extensionManifest is an MCP Bundle manifest, as far as ZeitBoard needs one.
type extensionManifest struct {
	ManifestVersion string                 `json:"manifest_version"`
	Name            string                 `json:"name"`
	DisplayName     string                 `json:"display_name"`
	Version         string                 `json:"version"`
	Description     string                 `json:"description"`
	LongDescription string                 `json:"long_description"`
	Author          extensionAuthor        `json:"author"`
	Icon            string                 `json:"icon"`
	Server          extensionServer        `json:"server"`
	Tools           []extensionTool        `json:"tools"`
	Keywords        []string               `json:"keywords"`
	Compatibility   extensionCompatibility `json:"compatibility"`
}

type extensionAuthor struct {
	Name string `json:"name"`
}

type extensionServer struct {
	Type       string             `json:"type"`
	EntryPoint string             `json:"entry_point"`
	MCPConfig  extensionMCPConfig `json:"mcp_config"`
}

type extensionMCPConfig struct {
	// Command names the binary without .exe: Claude Desktop, and Windows,
	// find it either way.
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

type extensionTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type extensionCompatibility struct {
	Platforms []string `json:"platforms"`
}

// claudeExtensionManifest describes the connector and every tool it can
// serve, from the registry the endpoint serves them from.
func claudeExtensionManifest() extensionManifest {
	var tools []extensionTool
	for _, tool := range localagent.ToolDefinitions(true) {
		tools = append(tools, extensionTool{Name: tool.Name, Description: tool.Description})
	}
	return extensionManifest{
		ManifestVersion: claudeExtensionManifestVersion,
		Name:            "zeitboard",
		DisplayName:     "ZeitBoard",
		Version:         localagent.Version,
		Description:     "Ask about your sleep-wake rhythm and plans in ZeitBoard, and propose changes you accept there.",
		LongDescription: "Connects Claude to ZeitBoard running on this computer. Claude can read what ZeitBoard lets an " +
			"assistant see: when sleep and waking are likely and how sure that is, reachable hours, commitments and what " +
			"waits for you. Titles, labels, notes and raw records are never included. Claude can propose a task, a " +
			"schedule change or a dose, and nothing changes until you accept it in ZeitBoard. What the tools return goes " +
			"to the model Claude uses, under your account with its maker. Medical questions get ZeitBoard's one answer.",
		Author: extensionAuthor{Name: "ZeitBoard"},
		Icon:   "icon.png",
		Server: extensionServer{
			Type:       "binary",
			EntryPoint: "server/" + localConnectorFile,
			MCPConfig: extensionMCPConfig{
				Command: "${__dirname}/server/zeitboard-local-mcp",
				Args:    []string{},
				Env:     map[string]string{},
			},
		},
		Tools:         tools,
		Keywords:      []string{"sleep", "non-24", "circadian", "planning"},
		Compatibility: extensionCompatibility{Platforms: []string{"win32"}},
	}
}

func writeClaudeExtension(out io.Writer, connectorPath string) error {
	connector, err := os.Open(connectorPath)
	if err != nil {
		return err
	}
	defer connector.Close()
	manifest, err := json.MarshalIndent(claudeExtensionManifest(), "", "  ")
	if err != nil {
		return err
	}
	archive := zip.NewWriter(out)
	for _, file := range []struct {
		name string
		body io.Reader
	}{
		{"manifest.json", bytes.NewReader(manifest)},
		{"icon.png", bytes.NewReader(appIcon)},
		{"server/" + localConnectorFile, connector},
	} {
		writer, err := archive.CreateHeader(&zip.FileHeader{Name: file.name, Method: zip.Deflate})
		if err != nil {
			return err
		}
		if _, err := io.Copy(writer, file.body); err != nil {
			return fmt.Errorf("write %s: %w", file.name, err)
		}
	}
	return archive.Close()
}

func writeClaudeSkill(out io.Writer) error {
	archive := zip.NewWriter(out)
	writer, err := archive.CreateHeader(&zip.FileHeader{Name: agentskill.Name + "/SKILL.md", Method: zip.Deflate})
	if err != nil {
		return err
	}
	if _, err := writer.Write(agentskill.Markdown); err != nil {
		return err
	}
	return archive.Close()
}
