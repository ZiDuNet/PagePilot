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
