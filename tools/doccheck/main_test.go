package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDocumentationPathsIncludesAllREADMESections(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"docs", "stages", "scripts", "tools", "deploy"} {
		path := filepath.Join(root, dir)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "README.md"), []byte("# Guide\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := documentationPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, path := range paths {
		seen[path] = true
	}
	for _, dir := range []string{"scripts", "tools", "deploy"} {
		if !seen[filepath.Join(root, dir, "README.md")] {
			t.Errorf("missing %s README", dir)
		}
	}
}

func TestLocalLinksValidateUnicodeSectionsAndRejectMissingTargets(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "README.md")
	target := filepath.Join(root, "guide.md")
	for path, content := range map[string]string{
		source: "# Local\n",
		target: "## 斷線、修復與重連練習\n## HTTP `API`\n## HTTP `API`\n```bash\n# Fake\n```\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, link := range []string{"#local", "guide.md#斷線修復與重連練習", "guide.md#http-api", "guide.md#http-api-1"} {
		if checked, err := checkLocalLink(source, link); !checked || err != nil {
			t.Errorf("valid %s: %v", link, err)
		}
	}
	for _, link := range []string{"missing.md", "guide.md#missing", "guide.md#fake"} {
		if _, err := checkLocalLink(source, link); err == nil {
			t.Errorf("accepted broken link %s", link)
		}
	}
	if checked, err := checkLocalLink(source, "https://example.com/guide#heading"); checked || err != nil {
		t.Fatal("external URL must remain outside local checks")
	}
}
