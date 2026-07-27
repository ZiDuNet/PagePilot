package deploy

import (
	"net/url"
	"testing"

	"github.com/yourorg/hostctl/internal/config"
)

func TestOSSEndpointURLAddsDefaultScheme(t *testing.T) {
	oss := newOSSStorage(config.Config{
		OSSEndpoint: "oss-cn-hangzhou.aliyuncs.com",
		OSSBucket:   "pagepilot-assets",
	})

	got, err := oss.endpointURL("pagepilot/demo/index.html", nil)
	if err != nil {
		t.Fatalf("endpointURL returned error: %v", err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse endpoint URL %q: %v", got, err)
	}
	if u.Scheme != "https" {
		t.Fatalf("scheme = %q; want https", u.Scheme)
	}
	if u.Host != "pagepilot-assets.oss-cn-hangzhou.aliyuncs.com" {
		t.Fatalf("host = %q; want bucket endpoint host", u.Host)
	}
	if u.Path != "/pagepilot/demo/index.html" {
		t.Fatalf("path = %q; want object key path", u.Path)
	}
}

func TestOSSCanonicalizedResourceIgnoresListQuery(t *testing.T) {
	oss := newOSSStorage(config.Config{OSSBucket: "wushuo"})
	query := url.Values{
		"prefix":   []string{"pagepilot/6m828j/versions/1/"},
		"max-keys": []string{"1000"},
		"marker":   []string{"pagepilot/old"},
	}

	got := oss.canonicalizedResource("", query)
	if got != "/wushuo/" {
		t.Fatalf("canonicalized resource = %q; want /wushuo/", got)
	}
}

func TestOSSCanonicalizedResourceKeepsSignedSubresources(t *testing.T) {
	oss := newOSSStorage(config.Config{OSSBucket: "wushuo"})
	query := url.Values{
		"prefix":         []string{"ignored"},
		"uploadId":       []string{"upload-1"},
		"partNumber":     []string{"2"},
		"security-token": []string{"token-1"},
	}

	got := oss.canonicalizedResource("pagepilot/demo/index.html", query)
	want := "/wushuo/pagepilot/demo/index.html?partNumber=2&security-token=token-1&uploadId=upload-1"
	if got != want {
		t.Fatalf("canonicalized resource = %q; want %q", got, want)
	}
}
