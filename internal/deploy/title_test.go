package deploy

import "testing"

func TestSanitizeSiteTitle(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "blank", in: "   ", want: ""},
		{name: "filename html", in: "index.html", want: ""},
		{name: "filename htm", in: "INDEX.HTM", want: ""},
		{name: "regular title", in: "My App", want: "My App"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeSiteTitle(tt.in); got != tt.want {
				t.Fatalf("sanitizeSiteTitle(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
