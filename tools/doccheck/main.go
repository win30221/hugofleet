// doccheck validates local documentation links, shell syntax and typed lesson examples.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	paths, err := documentationPaths(".")
	if err != nil {
		return err
	}
	links := regexp.MustCompile(`\]\(([^)]+)\)`)
	blocks := regexp.MustCompile("(?ms)^```(?:bash|sh)\\r?\\n(.*?)^```[ \\t]*\\r?$")
	linkCount, blockCount := 0, 0
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range links.FindAllSubmatch(raw, -1) {
			checked, err := checkLocalLink(p, string(m[1]))
			if err != nil {
				return err
			}
			if checked {
				linkCount++
			}
		}
		for i, m := range blocks.FindAllSubmatch(raw, -1) {
			cmd := exec.Command("bash", "-n")
			cmd.Stdin = bytes.NewReader(m[1])
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("shell example %s #%d: %s %w", p, i+1, out, err)
			}
			blockCount++
		}
	}
	for _, p := range []string{"stages/m2/create-app.json", "stages/m3/deploy-a.json", "stages/m3/deploy-b.json"} {
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		var req domain.DeployRequest
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if err = d.Decode(&req); err != nil {
			return err
		}
		if req.ExpectedDesiredRevision == nil || *req.ExpectedDesiredRevision != 0 || domain.ValidateName(req.Name) != nil {
			return fmt.Errorf("invalid create example %s", p)
		}
		if _, err = domain.DefaultPolicy().Spec(req); err != nil {
			return fmt.Errorf("invalid application spec %s: %w", p, err)
		}
	}
	fmt.Printf("PASS %d Markdown files, %d local file/section links, %d shell example blocks, three typed deployment examples\n", len(paths), linkCount, blockCount)
	return nil
}

func documentationPaths(root string) ([]string, error) {
	paths := []string{filepath.Join(root, "README.md"), filepath.Join(root, "AGENTS.md"), filepath.Join(root, "HUGOFLEET_SPEC.md")}
	for _, dir := range []string{"docs", "stages", "scripts", "tools", "deploy"} {
		dir = filepath.Join(root, dir)
		if err := filepath.WalkDir(dir, func(p string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if !d.IsDir() && strings.HasSuffix(p, ".md") {
				paths = append(paths, p)
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

func checkLocalLink(source, target string) (bool, error) {
	if strings.Contains(target, "://") || strings.HasPrefix(target, "/") || strings.HasPrefix(target, "mailto:") {
		return false, nil
	}
	parts := strings.SplitN(target, "#", 2)
	path := source
	if parts[0] != "" {
		path = filepath.Join(filepath.Dir(source), parts[0])
	}
	if _, err := os.Stat(path); err != nil {
		return false, fmt.Errorf("broken local link %s -> %s: %w", source, target, err)
	}
	if len(parts) == 2 && parts[1] != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		if !headingAnchors(raw)[parts[1]] {
			return false, fmt.Errorf("broken section link %s -> %s", source, target)
		}
	}
	return true, nil
}

// These docs use ATX headings. Ignore fenced examples and preserve Unicode
// letters, numbers, marks, hyphens and underscores in their section anchors.
func headingAnchors(raw []byte) map[string]bool {
	anchors := map[string]bool{}
	counts := map[string]int{}
	fence := ""
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			marker := line[:3]
			if fence == "" {
				fence = marker
			} else if fence == marker {
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		heading := strings.TrimLeft(line, "#")
		level := len(line) - len(heading)
		if level < 1 || level > 6 || !strings.HasPrefix(heading, " ") {
			continue
		}
		heading = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(heading), "#"))
		anchor := strings.Map(func(r rune) rune {
			switch {
			case unicode.IsLetter(r), unicode.IsNumber(r), unicode.IsMark(r), r == '-', r == '_':
				return unicode.ToLower(r)
			case unicode.IsSpace(r):
				return '-'
			default:
				return -1
			}
		}, heading)
		base := anchor
		if counts[base] > 0 {
			anchor = fmt.Sprintf("%s-%d", base, counts[base])
		}
		counts[base]++
		anchors[anchor] = true
	}
	return anchors
}
