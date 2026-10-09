package zalo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const maxImageBytes = 10 << 20

var imageExt = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp"}

// photoURL extracts the image address from the "photo" field, which the docs describe as a URL but may
// arrive as a string, an object with a url, or a list of either.
func photoURL(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var o struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(raw, &o) == nil && o.URL != "" {
		return o.URL
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil && len(list) > 0 {
		return photoURL(list[len(list)-1])
	}
	return ""
}

// FileImageStore saves images into a folder. It implements ImageStore.
type FileImageStore struct{ dir string }

// NewImageStore returns a store that writes into dir.
func NewImageStore(dir string) *FileImageStore { return &FileImageStore{dir: dir} }

// Save downloads an image that Zalo sent and returns the path of the saved file. The address comes from
// outside, so it must be https, may not point at this machine or a private network (also after redirects),
// and the file must really be an image of a sane size.
func (f *FileImageStore) Save(ctx context.Context, raw string) (string, error) {
	dir := f.dir
	if dir == "" {
		return "", errors.New("chưa cấu hình thư mục lưu ảnh")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", errors.New("địa chỉ ảnh không hợp lệ")
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		if ip := net.ParseIP(host); ip == nil || !publicIP(ip) {
			return errors.New("địa chỉ ảnh trỏ vào mạng nội bộ")
		}
		return nil
	}}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{DialContext: dialer.DialContext, Proxy: nil},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || req.URL.Scheme != "https" {
				return errors.New("chuyển hướng không hợp lệ")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("không tải được: " + scrubURL(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("máy chủ ảnh trả về %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return "", errors.New("lỗi khi tải ảnh")
	}
	if len(data) > maxImageBytes {
		return "", errors.New("ảnh lớn hơn 10MB")
	}
	ext, ok := imageExt[strings.SplitN(http.DetectContentType(data), ";", 2)[0]]
	if !ok {
		return "", errors.New("tệp tải về không phải ảnh được hỗ trợ (jpg, png, gif, webp)")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "zalo-"+hex.EncodeToString(id[:])+ext)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// publicIP is true for addresses that are not loopback, private, link-local, multicast or unspecified.
func publicIP(ip net.IP) bool {
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified())
}

// scrubURL keeps the reason of a network error but drops the (signed) address that net/http puts in it.
func scrubURL(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err.Error()
	}
	return err.Error()
}
