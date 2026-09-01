package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yourorg/hostctl/internal/store"
)

type templateSourceAuthStub struct {
	DeployerPort
	site store.Site
	err  error
}

func (s templateSourceAuthStub) GetSite(context.Context, string) (store.Site, error) {
	if s.err != nil {
		return store.Site{}, s.err
	}
	return s.site, nil
}

func TestAuthorizeTemplateSourceUsesOwnershipAndPolicy(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/deploy", nil)
	cases := []struct {
		name     string
		req      DeployRequest
		owner    string
		admin    bool
		site     store.Site
		wantCode ErrorCode
		wantNil  bool
	}{
		{name: "no source", req: DeployRequest{}, owner: "anon:s1", wantNil: true},
		{name: "version without code", req: DeployRequest{TemplateSourceVersion: 1}, owner: "user:u1", wantCode: CodeInvalidInput},
		{
			name:    "public policy allows registered user",
			req:     DeployRequest{TemplateSourceCode: "source", TemplateSourceVersion: 1},
			owner:   "user:u1",
			site:    store.Site{Code: "source", Visibility: "public", Status: "active"},
			wantNil: true,
		},
		{
			name:     "anonymous is denied even when owner",
			req:      DeployRequest{TemplateSourceCode: "source", TemplateSourceVersion: 1},
			owner:    "anon:s1",
			site:     store.Site{Code: "source", OwnerTokenID: "anon:s1", Visibility: "public", Status: "active"},
			wantCode: CodeForbidden,
		},
		{
			name:     "protected non-owner denied",
			req:      DeployRequest{TemplateSourceCode: "source", TemplateSourceVersion: 1},
			owner:    "user:u1",
			site:     store.Site{Code: "source", OwnerTokenID: "user:u2", Visibility: "public", Status: "active", AccessPasswordHash: "hash"},
			wantCode: CodeForbidden,
		},
		{
			name:    "owner can reuse protected source",
			req:     DeployRequest{TemplateSourceCode: "source", TemplateSourceVersion: 1},
			owner:   "user:u2",
			site:    store.Site{Code: "source", OwnerTokenID: "user:u2", Visibility: "unlisted", Status: "inactive", AccessPasswordHash: "hash"},
			wantNil: true,
		},
		{
			name:    "admin can reuse protected source",
			req:     DeployRequest{TemplateSourceCode: "source", TemplateSourceVersion: 1},
			owner:   "user:admin",
			admin:   true,
			site:    store.Site{Code: "source", OwnerTokenID: "user:u2", Visibility: "unlisted", Status: "inactive", AccessPasswordHash: "hash"},
			wantNil: true,
		},
		{name: "missing source", req: DeployRequest{TemplateSourceCode: "source"}, owner: "user:u1", site: store.Site{}, wantCode: CodeInvalidInput},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := templateSourceAuthStub{site: tc.site}
			if tc.name == "missing source" {
				stub.err = store.ErrNotFound
			}
			srv := &Server{deployer: stub}
			got := srv.authorizeTemplateSource(req, tc.req, tc.owner, tc.admin)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("authorizeTemplateSource() = %v, want nil", got)
				}
				return
			}
			if got == nil || got.ErrorCode != tc.wantCode {
				t.Fatalf("authorizeTemplateSource() = %+v, want code %s", got, tc.wantCode)
			}
		})
	}
}

func TestAuthorizeTemplateSourcePropagatesStoreErrors(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/deploy", nil)
	srv := &Server{deployer: templateSourceAuthStub{err: errors.New("database unavailable")}}
	got := srv.authorizeTemplateSource(req, DeployRequest{TemplateSourceCode: "source"}, "user:u1", false)
	if got == nil || got.ErrorCode != CodeInternal {
		t.Fatalf("authorizeTemplateSource() = %+v, want internal error", got)
	}
}

func TestSiteReuseDecisionPolicyMatrix(t *testing.T) {
	cases := []struct {
		name         string
		site         store.Site
		wantDownload bool
		wantReuse    bool
	}{
		{
			name:         "public defaults",
			site:         store.Site{Status: "active", Visibility: "public", ReusePolicy: "auto", SourceDownloadPolicy: "auto"},
			wantDownload: true,
			wantReuse:    true,
		},
		{
			name:         "explicit source allow enables unlisted reuse",
			site:         store.Site{Status: "active", Visibility: "unlisted", ReusePolicy: "auto", SourceDownloadPolicy: "allow"},
			wantDownload: true,
			wantReuse:    true,
		},
		{
			name:         "unlisted defaults remain private",
			site:         store.Site{Status: "active", Visibility: "unlisted", ReusePolicy: "auto", SourceDownloadPolicy: "auto"},
			wantDownload: false,
			wantReuse:    false,
		},
		{
			name:         "source deny wins",
			site:         store.Site{Status: "active", Visibility: "public", ReusePolicy: "allow", SourceDownloadPolicy: "deny"},
			wantDownload: false,
			wantReuse:    false,
		},
		{
			name:         "reuse deny leaves download",
			site:         store.Site{Status: "active", Visibility: "public", ReusePolicy: "deny", SourceDownloadPolicy: "allow"},
			wantDownload: true,
			wantReuse:    false,
		},
		{
			name:         "protected site",
			site:         store.Site{Status: "active", Visibility: "public", AccessPasswordHash: "hash", ReusePolicy: "allow", SourceDownloadPolicy: "allow"},
			wantDownload: false,
			wantReuse:    false,
		},
		{
			name:         "inactive site",
			site:         store.Site{Status: "inactive", Visibility: "public", ReusePolicy: "allow", SourceDownloadPolicy: "allow"},
			wantDownload: false,
			wantReuse:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotDownload, gotReuse := SiteReuseDecision(tc.site)
			if gotDownload != tc.wantDownload || gotReuse != tc.wantReuse {
				t.Fatalf("SiteReuseDecision() = (%v, %v), want (%v, %v)", gotDownload, gotReuse, tc.wantDownload, tc.wantReuse)
			}
		})
	}
}
