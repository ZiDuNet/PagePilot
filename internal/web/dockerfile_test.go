package web

import (
	"os"
	"strings"
	"testing"
)

func TestDockerfileRefreshesEmbeddedSkillZipBeforeGoBuild(t *testing.T) {
	data, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatalf("read Dockerfile: %v", err)
	}
	dockerfile := string(data)
	pythonIdx := strings.Index(dockerfile, "python3 scripts/build_skill_zip.py")
	buildIdx := strings.Index(dockerfile, "go build -trimpath")
	if pythonIdx < 0 {
		t.Fatalf("Dockerfile must rebuild the embedded Skill ZIP")
	}
	if buildIdx < 0 {
		t.Fatalf("Dockerfile missing Go build step")
	}
	if pythonIdx > buildIdx {
		t.Fatalf("Dockerfile rebuilds Skill ZIP after Go build, so embed may be stale")
	}
	if !strings.Contains(dockerfile, "apk add --no-cache git ca-certificates python3") {
		t.Fatalf("Dockerfile must install python3 in the Go builder stage")
	}
}
