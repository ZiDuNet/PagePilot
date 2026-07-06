package deploy

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/yourorg/hostctl/internal/config"
)

var errFileNotFound = errors.New("file not found")

type ossStorage struct {
	cfg    config.Config
	client *http.Client
}

type ossListBucketResult struct {
	IsTruncated bool   `xml:"IsTruncated"`
	NextMarker  string `xml:"NextMarker"`
	Contents    []struct {
		Key string `xml:"Key"`
	} `xml:"Contents"`
}

func newOSSStorage(cfg config.Config) *ossStorage {
	return &ossStorage{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (o *ossStorage) objectKey(parts ...string) string {
	items := make([]string, 0, len(parts)+1)
	if prefix := strings.Trim(strings.TrimSpace(o.cfg.OSSPrefix), "/"); prefix != "" {
		items = append(items, prefix)
	}
	for _, part := range parts {
		part = strings.Trim(strings.ReplaceAll(part, "\\", "/"), "/")
		if part != "" {
			items = append(items, part)
		}
	}
	return path.Clean(strings.Join(items, "/"))
}

func (o *ossStorage) versionPrefix(code string, version int64) string {
	return o.objectKey(code, "versions", fmt.Sprintf("%d", version)) + "/"
}

func (o *ossStorage) versionObjectKey(code string, version int64, rel string) string {
	return o.objectKey(code, "versions", fmt.Sprintf("%d", version), rel)
}

func (o *ossStorage) endpointURL(key string, query url.Values) (string, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(o.cfg.OSSEndpoint), "/")
	if endpoint == "" || o.cfg.OSSBucket == "" {
		return "", fmt.Errorf("oss endpoint and bucket are required")
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(u.Host, o.cfg.OSSBucket+".") {
		u.Host = o.cfg.OSSBucket + "." + u.Host
	}
	u.Path = "/" + strings.TrimLeft(key, "/")
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func (o *ossStorage) signedRequest(ctx context.Context, method, key string, body []byte, contentType string, query url.Values) (*http.Request, error) {
	u, err := o.endpointURL(key, query)
	if err != nil {
		return nil, err
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return nil, err
	}
	date := time.Now().UTC().Format(http.TimeFormat)
	req.Header.Set("Date", date)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resource := o.canonicalizedResource(key, query)
	stringToSign := method + "\n\n" + contentType + "\n" + date + "\n" + resource
	mac := hmac.New(sha1.New, []byte(o.cfg.OSSAccessKeySecret))
	_, _ = mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	req.Header.Set("Authorization", "OSS "+o.cfg.OSSAccessKeyID+":"+signature)
	return req, nil
}

func (o *ossStorage) canonicalizedResource(key string, query url.Values) string {
	resource := "/" + o.cfg.OSSBucket + "/" + strings.TrimLeft(key, "/")
	if len(query) == 0 {
		return resource
	}
	keys := make([]string, 0, len(query))
	for name := range query {
		if name != "" {
			keys = append(keys, name)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, name := range keys {
		value := query.Get(name)
		if value == "" {
			parts = append(parts, name)
			continue
		}
		parts = append(parts, name+"="+value)
	}
	if len(parts) == 0 {
		return resource
	}
	return resource + "?" + strings.Join(parts, "&")
}

func (o *ossStorage) put(ctx context.Context, key string, body []byte, contentType string) error {
	req, err := o.signedRequest(ctx, http.MethodPut, key, body, contentType, nil)
	if err != nil {
		return err
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("oss put %s failed: %s %s", key, resp.Status, strings.TrimSpace(string(data)))
	}
	return nil
}

func (o *ossStorage) get(ctx context.Context, key string) ([]byte, time.Time, error) {
	req, err := o.signedRequest(ctx, http.MethodGet, key, nil, "", nil)
	if err != nil {
		return nil, time.Time{}, err
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, time.Time{}, errFileNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, time.Time{}, fmt.Errorf("oss get %s failed: %s %s", key, resp.Status, strings.TrimSpace(string(data)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, time.Time{}, err
	}
	modTime := time.Time{}
	if raw := resp.Header.Get("Last-Modified"); raw != "" {
		if parsed, parseErr := http.ParseTime(raw); parseErr == nil {
			modTime = parsed
		}
	}
	return data, modTime, nil
}

func (o *ossStorage) delete(ctx context.Context, key string) error {
	req, err := o.signedRequest(ctx, http.MethodDelete, key, nil, "", nil)
	if err != nil {
		return err
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("oss delete %s failed: %s %s", key, resp.Status, strings.TrimSpace(string(data)))
	}
	return nil
}

func (o *ossStorage) listPrefix(ctx context.Context, prefix string) ([]string, error) {
	keys := make([]string, 0)
	marker := ""
	for {
		query := url.Values{
			"prefix":   []string{prefix},
			"max-keys": []string{"1000"},
		}
		if marker != "" {
			query.Set("marker", marker)
		}
		req, err := o.signedRequest(ctx, http.MethodGet, "", nil, "", query)
		if err != nil {
			return nil, err
		}
		resp, err := o.client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			return nil, fmt.Errorf("oss list %s failed: %s %s", prefix, resp.Status, strings.TrimSpace(string(data)))
		}
		var out ossListBucketResult
		if err := xml.NewDecoder(resp.Body).Decode(&out); err != nil {
			_ = resp.Body.Close()
			return nil, err
		}
		_ = resp.Body.Close()
		for _, item := range out.Contents {
			if item.Key != "" {
				keys = append(keys, item.Key)
			}
		}
		if !out.IsTruncated {
			return keys, nil
		}
		next := strings.TrimSpace(out.NextMarker)
		if next == "" && len(out.Contents) > 0 {
			next = out.Contents[len(out.Contents)-1].Key
		}
		if next == "" || next == marker {
			return keys, nil
		}
		marker = next
	}
}

func (o *ossStorage) deletePrefix(ctx context.Context, prefix string) error {
	keys, err := o.listPrefix(ctx, prefix)
	if err != nil {
		return err
	}
	for _, key := range keys {
		if err := o.delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}
