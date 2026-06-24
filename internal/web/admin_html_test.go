package web

import (
	"strings"
	"testing"
)

func TestAdminOverviewSummaryOnlyRendersInsideOverviewTab(t *testing.T) {
	html := string(AdminHTML())

	overviewStart := strings.Index(html, `<section id="tab-overview"`)
	accountStart := strings.Index(html, `<section id="tab-account"`)
	quickActions := strings.Index(html, `id="quick-actions"`)
	statsGrid := strings.Index(html, `<section class="stats-grid"`)

	if overviewStart == -1 || accountStart == -1 {
		t.Fatalf("admin overview/account tab markers not found")
	}
	for name, idx := range map[string]int{
		"quick-actions": quickActions,
		"stats-grid":   statsGrid,
	} {
		if idx < overviewStart || idx > accountStart {
			t.Fatalf("%s index = %d, want inside overview tab [%d, %d]", name, idx, overviewStart, accountStart)
		}
	}
}
