package web

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestSkillPackageContainsPagepSkill(t *testing.T) {
	data, err := SkillPackage()
	if err != nil {
		t.Fatalf("read built-in skill package: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open built-in skill package: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == "pagep/SKILL.md" {
			return
		}
	}
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	t.Logf("built-in skill package entries: %v", names)
	t.Fatalf("built-in skill package missing pagep/SKILL.md")
}
