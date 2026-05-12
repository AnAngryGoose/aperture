// Package compose wraps docker compose CLI operations for both the local hub
// and remote agents. The Local type runs commands via exec; the hub wires
// remote agents via agentComposeProvider (in internal/hub/agentws.go) which
// uses the same interface over WebSocket frames.
package compose

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aperture/aperture/internal/types"
)

// Local runs docker compose operations on the local host via exec.
type Local struct {
	bin string // "docker" (compose v2 plugin) or "docker-compose" (v1 standalone)
}

// NewLocal detects which compose binary is available and returns a Local.
// Returns an error only if neither docker compose nor docker-compose is found.
func NewLocal() (*Local, error) {
	if exec.Command("docker", "compose", "version").Run() == nil {
		return &Local{bin: "docker"}, nil
	}
	if exec.Command("docker-compose", "version").Run() == nil {
		return &Local{bin: "docker-compose"}, nil
	}
	return nil, fmt.Errorf("neither 'docker compose' nor 'docker-compose' found in PATH")
}

// DiscoverStacks lists all compose projects (including stopped) via
// `docker compose ls --all --format json`.
func (l *Local) DiscoverStacks(ctx context.Context) ([]types.ComposeStack, error) {
	out, err := l.run(ctx, "", "ls", "--all", "--format", "json")
	if err != nil {
		// ls may exit non-zero when there are no stacks; treat as empty.
		return nil, nil
	}
	return ParseLS(out)
}

// GetStack returns one stack with its full service list.
func (l *Local) GetStack(ctx context.Context, project string) (*types.ComposeStack, error) {
	// Discover for working dir + config files.
	stacks, _ := l.DiscoverStacks(ctx)
	var base types.ComposeStack
	for _, st := range stacks {
		if st.Project == project {
			base = st
			break
		}
	}
	base.Project = project // ensure set even if not found in ls

	// Service detail from `docker compose ps`.
	psArgs := []string{"--project-name", project, "ps", "--all", "--format", "json"}
	if base.WorkingDir != "" {
		psArgs = append([]string{"--project-directory", base.WorkingDir}, psArgs...)
	}
	psOut, _ := l.run(ctx, base.WorkingDir, psArgs...)
	svcs, _ := ParsePS(psOut)
	base.Services = svcs

	base.TotalCount = len(svcs)
	base.RunningCount = 0
	for _, s := range svcs {
		if s.State == "running" {
			base.RunningCount++
		}
	}
	base.Status = stackStatus(base.RunningCount, base.TotalCount)
	return &base, nil
}

// StackAction runs a lifecycle subcommand (up, down, restart, pull, stop, start).
// service is optional; empty targets all services. Returns combined stdout+stderr.
func (l *Local) StackAction(ctx context.Context, project, workingDir, action, service string, extraArgs ...string) (string, error) {
	var args []string
	if project != "" {
		args = append(args, "--project-name", project)
	}
	if workingDir != "" {
		args = append(args, "--project-directory", workingDir)
	}
	args = append(args, action)
	switch action {
	case "up":
		args = append(args, "-d", "--remove-orphans")
	case "pull":
		args = append(args, "--quiet")
	}
	args = append(args, extraArgs...)
	if service != "" {
		args = append(args, service)
	}
	return l.run(ctx, workingDir, args...)
}

// Logs fetches compose logs for the stack (or a specific service).
func (l *Local) Logs(ctx context.Context, project, workingDir, service string, tail int) (string, error) {
	if tail <= 0 {
		tail = 200
	}
	var args []string
	if project != "" {
		args = append(args, "--project-name", project)
	}
	if workingDir != "" {
		args = append(args, "--project-directory", workingDir)
	}
	args = append(args, "logs", fmt.Sprintf("--tail=%d", tail), "--no-color")
	if service != "" {
		args = append(args, service)
	}
	out, err := l.run(ctx, workingDir, args...)
	// logs exits non-zero when there are no containers — still return output.
	if err != nil && out != "" {
		return out, nil
	}
	return out, err
}

// ReadFile reads the compose YAML from workingDir.
func (l *Local) ReadFile(_ context.Context, workingDir string) (string, error) {
	path := FindComposeFile(workingDir)
	if path == "" {
		return "", fmt.Errorf("no compose file found in %s", workingDir)
	}
	b, err := os.ReadFile(path)
	return string(b), err
}

// WriteFile writes compose YAML to workingDir (creates dir and file if needed).
func (l *Local) WriteFile(_ context.Context, workingDir, content string) error {
	if err := os.MkdirAll(workingDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", workingDir, err)
	}
	path := FindComposeFile(workingDir)
	if path == "" {
		path = filepath.Join(workingDir, "compose.yml")
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// run executes a compose subcommand (args must NOT include the "compose" verb
// when using the v2 plugin — it is prepended automatically).
func (l *Local) run(ctx context.Context, dir string, args ...string) (string, error) {
	var cmdArgs []string
	if l.bin == "docker" {
		cmdArgs = append([]string{"compose"}, args...)
	} else {
		cmdArgs = args
	}
	cmd := exec.CommandContext(ctx, l.bin, cmdArgs...)
	if dir != "" {
		cmd.Dir = dir
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return buf.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(buf.String()))
	}
	return buf.String(), nil
}

// ── parsing (exported so agentComposeProvider can reuse on the hub side) ───

// lsEntry matches one item from `docker compose ls --all --format json`.
type lsEntry struct {
	Name        string `json:"Name"`
	Status      string `json:"Status"`
	ConfigFiles string `json:"ConfigFiles"`
}

// psEntry matches one line of `docker compose ps --all --format json`.
type psEntry struct {
	ID         string `json:"ID"`
	Service    string `json:"Service"`
	State      string `json:"State"`
	Status     string `json:"Status"`
	Health     string `json:"Health"`
	ExitCode   int    `json:"ExitCode"`
	Publishers []struct {
		URL           string `json:"URL"`
		TargetPort    int    `json:"TargetPort"`
		PublishedPort int    `json:"PublishedPort"`
		Protocol      string `json:"Protocol"`
	} `json:"Publishers"`
}

// ParseLS parses `docker compose ls --all --format json` stdout.
func ParseLS(stdout string) ([]types.ComposeStack, error) {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" || stdout == "null" || stdout == "[]" {
		return nil, nil
	}
	var entries []lsEntry
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		return nil, fmt.Errorf("parse compose ls: %w", err)
	}
	out := make([]types.ComposeStack, 0, len(entries))
	for _, e := range entries {
		st := types.ComposeStack{
			Project:     e.Name,
			ConfigFiles: e.ConfigFiles,
		}
		if e.ConfigFiles != "" {
			first := strings.SplitN(e.ConfigFiles, ",", 2)[0]
			st.WorkingDir = filepath.Dir(strings.TrimSpace(first))
		}
		lower := strings.ToLower(e.Status)
		switch {
		case strings.Contains(lower, "running"):
			st.Status = "running"
		case strings.Contains(lower, "exit"), lower == "created", lower == "stopped":
			st.Status = "stopped"
		default:
			st.Status = lower
		}
		out = append(out, st)
	}
	return out, nil
}

// ParsePS parses `docker compose ps --all --format json` stdout.
// Handles both JSON-array format (newer Docker Compose) and NDJSON.
func ParsePS(stdout string) ([]types.ComposeService, error) {
	stdout = strings.TrimSpace(stdout)
	if stdout == "" || stdout == "null" || stdout == "[]" {
		return nil, nil
	}
	var entries []psEntry
	if strings.HasPrefix(stdout, "[") {
		if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
			return nil, fmt.Errorf("parse compose ps: %w", err)
		}
	} else {
		scanner := bufio.NewScanner(strings.NewReader(stdout))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var e psEntry
			if json.Unmarshal([]byte(line), &e) == nil {
				entries = append(entries, e)
			}
		}
	}

	svcs := make([]types.ComposeService, 0, len(entries))
	for _, e := range entries {
		svc := types.ComposeService{
			Name:     e.Service,
			State:    strings.ToLower(e.State),
			Status:   e.Status,
			Health:   strings.ToLower(e.Health),
			ExitCode: e.ExitCode,
		}
		if id := e.ID; len(id) >= 12 {
			svc.ContainerID = id[:12]
		} else {
			svc.ContainerID = id
		}
		for _, pub := range e.Publishers {
			if pub.PublishedPort > 0 {
				svc.Ports = append(svc.Ports, types.PortMapping{
					PrivatePort: uint16(pub.TargetPort),
					PublicPort:  uint16(pub.PublishedPort),
					Type:        pub.Protocol,
				})
			}
		}
		svcs = append(svcs, svc)
	}
	sort.Slice(svcs, func(i, j int) bool { return svcs[i].Name < svcs[j].Name })
	return svcs, nil
}

// FindComposeFile returns the path to the compose file in dir, or "" if none.
func FindComposeFile(dir string) string {
	for _, name := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func stackStatus(running, total int) string {
	if total == 0 {
		return "stopped"
	}
	if running == total {
		return "running"
	}
	if running == 0 {
		return "stopped"
	}
	return "partial"
}
