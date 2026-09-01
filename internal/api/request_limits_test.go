package api

import (
	"math"
	"testing"

	"github.com/yourorg/hostctl/internal/config"
)

func TestMaxRequestBodyBytesClampsWithoutOverflow(t *testing.T) {
	tests := []struct {
		name  string
		total int64
		want  int64
	}{
		{name: "small total uses minimum", total: 1, want: 8 << 20},
		{name: "normal total includes overhead", total: 10 << 20, want: 11 << 20},
		{name: "near cap saturates", total: (256 << 20) - (1 << 20) + 1, want: 256 << 20},
		{name: "large total saturates", total: math.MaxInt64, want: 256 << 20},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := &Server{cfg: config.Config{MaxSiteTotalBytes: tc.total}}
			if got := srv.maxRequestBodyBytes(); got != tc.want {
				t.Fatalf("maxRequestBodyBytes() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestPageOffsetSaturates(t *testing.T) {
	if got := pageOffset(1, 50); got != 0 {
		t.Fatalf("first page offset = %d, want 0", got)
	}
	if got := pageOffset(2, 50); got != 50 {
		t.Fatalf("second page offset = %d, want 50", got)
	}
	maxInt := int(^uint(0) >> 1)
	if got := pageOffset(maxInt, 200); got != maxInt {
		t.Fatalf("overflowing page offset = %d, want %d", got, maxInt)
	}
}
