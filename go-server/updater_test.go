package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func updateArchive(t *testing.T, name string, kind byte, extra bool) []byte {
	t.Helper()
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	tr := tar.NewWriter(z)
	if e := tr.WriteHeader(&tar.Header{Name: name, Mode: 0700, Typeflag: kind, Size: 4}); e != nil {
		t.Fatal(e)
	}
	tr.Write([]byte("test"))
	if extra {
		tr.WriteHeader(&tar.Header{Name: "config.json", Mode: 0600, Size: 2})
		tr.Write([]byte("{}"))
	}
	tr.Close()
	z.Close()
	return b.Bytes()
}
func fixtureUpdater(t *testing.T) (*systemUpdater, *githubRelease, []byte) {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, "hetun-operations")
	os.WriteFile(exe, []byte("old-program"), 0700)
	u := newSystemUpdater(exe, filepath.Join(dir, "config.json"))
	u.goos = "linux"
	u.arch = "amd64"
	archive := updateArchive(t, "hetun-operations", tar.TypeReg, false)
	sum := sha256.Sum256(archive)
	tag := "v99.0.0"
	name := "Hetun-Operations-99.0.0-linux-amd64.tar.gz"
	g := &githubRelease{Tag: tag, URL: "https://github.com/" + updateRepo + "/releases/tag/" + tag, Assets: []releaseAsset{{Name: name, URL: "https://github.com/" + updateRepo + "/releases/download/" + tag + "/" + name, Size: int64(len(archive)), Digest: "sha256:" + hex.EncodeToString(sum[:])}}}
	u.client = &http.Client{Transport: updateTransport(func(r *http.Request) (*http.Response, error) {
		var data []byte
		if r.URL.String() == releaseAPI {
			data, _ = json.Marshal(g)
		} else if r.URL.String() == g.Assets[0].URL {
			data = archive
		} else if strings.HasSuffix(r.URL.Path, "SHA256SUMS.txt") {
			data = []byte(hex.EncodeToString(sum[:]) + "  " + name + "\n")
		} else {
			return nil, fmt.Errorf("unexpected request")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}, nil
	})}
	u.preflight = func(context.Context, string, string, string) error { return nil }
	return u, g, archive
}
func waitUpdate(t *testing.T, u *systemUpdater) updateView {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s := u.snapshot()
		if s.Phase == "failed" || s.Phase == "restarting" {
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("update task timed out")
	return updateView{}
}
func TestSystemUpdateAuthAndMethods(t *testing.T) {
	h := systemHarness(t)
	h.a.updater, _, _ = fixtureUpdater(t)
	for _, item := range []struct {
		method, path string
		data         M
	}{{"GET", "/api/admin/system/update", nil}, {"POST", "/api/admin/system/update/check", M{}}, {"POST", "/api/admin/system/update/install", M{"version": "99.0.0"}}} {
		cookie := h.cookie
		h.cookie = ""
		w := h.req(item.method, item.path, item.data, "")
		h.cookie = cookie
		if w.Code != 401 {
			t.Fatal(w.Code, w.Body)
		}
	}
	for _, path := range []string{"/api/admin/system/update/check", "/api/admin/system/update/install"} {
		if w := h.req("GET", path, nil, ""); w.Code != 405 {
			t.Fatal(w.Code)
		}
	}
	if w := h.req("POST", "/api/admin/system/update/install", M{"version": "99.0.0"}, ""); w.Code != 409 {
		t.Fatal(w.Code)
	}
	if w := h.req("POST", "/api/admin/system/update/check", M{}, ""); w.Code != 200 || payload(t, w)["available"] != true {
		t.Fatal(w.Code, w.Body)
	}
	if w := h.req("POST", "/api/admin/system/update/install", M{"version": "99.0.0", "url": "https://evil.example"}, ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := h.req("POST", "/api/admin/system/update/install", M{"version": "98.0.0"}, ""); w.Code != 409 {
		t.Fatal(w.Code)
	}
	r := httptest.NewRequest("POST", h.origin+"/api/admin/system/update/install", strings.NewReader("{\"version\":\"99.0.0\"}"))
	r.Header.Set("Origin", "https://evil.example")
	r.Header.Set("X-Hetun-Client", "studio")
	r.AddCookie(&http.Cookie{Name: "hetun_session", Value: h.cookie})
	w := httptest.NewRecorder()
	h.a.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestUpdateDownloadVerifyPreflightSwap(t *testing.T) {
	u, _, _ := fixtureUpdater(t)
	if e := u.check(context.Background()); e != nil {
		t.Fatal(e)
	}
	checked := false
	u.preflight = func(_ context.Context, candidate, config, version string) error {
		b, e := os.ReadFile(candidate)
		if e != nil || string(b) != "test" || config != u.configPath || version != "99.0.0" {
			t.Fatal("preflight arguments")
		}
		checked = true
		return nil
	}
	os.WriteFile(u.configPath, []byte("preserved-config"), 0600)
	asset := filepath.Join(filepath.Dir(u.executable), "assets")
	os.Mkdir(asset, 0700)
	os.WriteFile(filepath.Join(asset, "poster"), []byte("preserved-asset"), 0600)
	if e := u.start("99.0.0"); e != nil {
		t.Fatal(e)
	}
	s := waitUpdate(t, u)
	if s.Phase != "restarting" {
		t.Fatal(s.Message)
	}
	p := <-u.restart
	if !checked {
		t.Fatal("preflight skipped")
	}
	if e := u.start("99.0.0"); e == nil {
		t.Fatal("duplicate install allowed")
	}
	if e := p.swap(); e != nil {
		t.Fatal(e)
	}
	for file, want := range map[string]string{u.executable: "test", p.backup: "old-program", u.configPath: "preserved-config", filepath.Join(asset, "poster"): "preserved-asset"} {
		b, e := os.ReadFile(file)
		if e != nil || string(b) != want {
			t.Fatalf("changed %s: %v", file, e)
		}
	}
}
func TestUpdateRejectsBadReleaseHashAndConfig(t *testing.T) {
	for _, kind := range []string{"hash", "digest", "tag", "draft", "assetURL", "preflight", "movedRelease"} {
		t.Run(kind, func(t *testing.T) {
			u, g, _ := fixtureUpdater(t)
			u.check(context.Background())
			switch kind {
			case "hash":
				g.Assets[0].Digest = "sha256:" + strings.Repeat("0", 64)
			case "digest":
				g.Assets[0].Digest = "sha256:bad"
			case "tag":
				g.Tag = "v99.0.0-rc1"
			case "draft":
				g.Draft = true
			case "assetURL":
				g.Assets[0].URL = "https://evil.example/download"
			case "preflight":
				u.preflight = func(context.Context, string, string, string) error { return fmt.Errorf("incompatible configuration") }
			case "movedRelease":
				g.Tag = "v100.0.0"
				g.URL = "https://github.com/" + updateRepo + "/releases/tag/" + g.Tag
			}
			if e := u.start("99.0.0"); e != nil {
				t.Fatal(e)
			}
			s := waitUpdate(t, u)
			if s.Phase != "failed" {
				t.Fatal("bad release accepted", s)
			}
			b, _ := os.ReadFile(u.executable)
			if string(b) != "old-program" {
				t.Fatal("old executable changed")
			}
			files, _ := filepath.Glob(filepath.Join(filepath.Dir(u.executable), ".hetun-update-*"))
			if len(files) != 0 {
				t.Fatal("candidate leaked")
			}
		})
	}
}
func TestUpdateChecksumsFallbackAndNoDowngrade(t *testing.T) {
	u, g, _ := fixtureUpdater(t)
	g.Assets[0].Digest = ""
	g.Assets = append(g.Assets, releaseAsset{Name: "SHA256SUMS.txt", URL: "https://github.com/" + updateRepo + "/releases/download/" + g.Tag + "/SHA256SUMS.txt"})
	if _, e := u.checksum(context.Background(), g.Assets[0], *g); e != nil {
		t.Fatal(e)
	}
	g.Tag = "v" + buildVersion
	g.URL = "https://github.com/" + updateRepo + "/releases/tag/" + g.Tag
	g.Assets = nil
	u.check(context.Background())
	if u.snapshot().Available {
		t.Fatal("same version offered")
	}
	u.goos = "windows"
	if u.snapshot().CanInstall {
		t.Fatal("windows install offered")
	}
}
func TestUpdateArchiveRejectsUnsafeContent(t *testing.T) {
	for _, v := range []struct {
		name  string
		kind  byte
		extra bool
	}{{"../hetun-operations", tar.TypeReg, false}, {"hetun-operations", tar.TypeSymlink, false}, {"hetun-operations", tar.TypeReg, true}, {"config.json", tar.TypeReg, false}} {
		b := updateArchive(t, v.name, v.kind, v.extra)
		if file, e := unpackUpdate(b, t.TempDir()); e == nil {
			os.Remove(file)
			t.Fatal("unsafe archive accepted", v)
		}
	}
	b := updateArchive(t, "hetun-operations", tar.TypeReg, false)
	b[len(b)-5] ^= 0xff
	if _, e := unpackUpdate(b, t.TempDir()); e == nil {
		t.Fatal("bad gzip CRC accepted")
	}
}
func TestUpdateURLAllowlist(t *testing.T) {
	for _, raw := range []string{"http://github.com/file", "https://github.com.evil.example/file", "https://user@github.com/file", "https://github.com:444/file", "file:///tmp/test", "https://127.0.0.1/file"} {
		if updateURLAllowed(raw) {
			t.Fatal(raw)
		}
	}
}
func TestUpdateCheckFailureDoesNotClaimLatest(t *testing.T) {
	u, _, _ := fixtureUpdater(t)
	u.check(context.Background())
	u.client = &http.Client{Transport: updateTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader("limit")), Header: make(http.Header)}, nil
	})}
	u.check(context.Background())
	s := u.snapshot()
	if s.Phase != "failed" || s.Latest != nil || s.Available {
		t.Fatal(s)
	}
}
