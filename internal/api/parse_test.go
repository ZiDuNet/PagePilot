package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEnsureSingleJSONValueRejectsTrailingValues(t *testing.T) {
	dec := json.NewDecoder(strings.NewReader(`{"value":1} {"extra":2}`))
	var first map[string]int
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("decode first JSON value: %v", err)
	}
	if err := ensureSingleJSONValue(dec); err == nil {
		t.Fatal("ensureSingleJSONValue accepted a trailing JSON value")
	}

	dec = json.NewDecoder(strings.NewReader("{\"value\":1} \n\t"))
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("decode JSON with whitespace: %v", err)
	}
	if err := ensureSingleJSONValue(dec); err != nil {
		t.Fatalf("ensureSingleJSONValue rejected trailing whitespace: %v", err)
	}
}

func TestParseInt64RejectsEmptyInvalidAndOverflow(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int64
		ok    bool
	}{
		{name: "positive", value: "42", want: 42, ok: true},
		{name: "max", value: "9223372036854775807", want: 9223372036854775807, ok: true},
		{name: "empty", value: "", ok: false},
		{name: "negative", value: "-1", ok: false},
		{name: "decimal", value: "1.0", ok: false},
		{name: "overflow", value: "9223372036854775808", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseInt64(tt.value)
			if tt.ok {
				if err != nil || got != tt.want {
					t.Fatalf("parseInt64(%q) = %d, %v; want %d, nil", tt.value, got, err, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("parseInt64(%q) = %d, nil; want an error", tt.value, got)
			}
		})
	}
}

func TestParseInt64FormDistinguishesMissingAndInvalid(t *testing.T) {
	if got, err := parseInt64Form(""); err != nil || got != 0 {
		t.Fatalf("parseInt64Form(empty) = %d, %v; want 0, nil", got, err)
	}
	if got, err := parseInt64Form(" 42 "); err != nil || got != 42 {
		t.Fatalf("parseInt64Form(42) = %d, %v; want 42, nil", got, err)
	}
	for _, value := range []string{"-1", "not-a-number", "9223372036854775808"} {
		if got, err := parseInt64Form(value); err == nil {
			t.Fatalf("parseInt64Form(%q) = %d, nil; want an error", value, got)
		}
	}
}

func TestParseTokenExpiresAtRejectsDurationOverflow(t *testing.T) {
	tooLarge := int64((1<<63-1)/int64(time.Second)) + 1
	if _, err := parseTokenExpiresAt(TokenCreateRequest{TTLSeconds: &tooLarge}); err == nil {
		t.Fatal("parseTokenExpiresAt() accepted TTL that overflows time.Duration")
	}
	valid := int64(60)
	if expires, err := parseTokenExpiresAt(TokenCreateRequest{TTLSeconds: &valid}); err != nil || expires == nil || !expires.After(time.Now()) {
		t.Fatalf("parseTokenExpiresAt(valid) = %v, %v; want future expiry", expires, err)
	}
}

func TestParseBoolParamRejectsUnknownValue(t *testing.T) {
	for _, value := range []string{"maybe", "2", "truthy"} {
		if got, err := parseBoolParam(value); err == nil || got {
			t.Fatalf("parseBoolParam(%q) = %v, %v; want an error", value, got, err)
		}
	}
}
