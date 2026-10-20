//go:build linux

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
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Runs a real old process, accepts an authenticated panel update, then execs the
// actual newly compiled service. Never connects to production or GitHub.
func TestUpdateLinuxExecEndToEnd(t *testing.T) {
	if os.Getenv("HETUN_UPDATE_EXEC_HELPER") != "" {
		return
	}
	dir := t.TempDir()
	candidate := filepath.Join(dir, "candidate")
	cmd := exec.Command("go", "build", "-ldflags", "-X main.buildVersion=99.0.0", "-o", candidate, ".")
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("build candidate: %v %s", e, b)
	}
	binary, e := os.ReadFile(candidate)
	if e != nil {
		t.Fatal(e)
	}
	var archive bytes.Buffer
	z := gzip.NewWriter(&archive)
	tr := tar.NewWriter(z)
	tr.WriteHeader(&tar.Header{Name: "hetun-operations", Mode: 0700, Size: int64(len(binary))})
	tr.Write(binary)
	tr.Close()
	z.Close()
	os.WriteFile(filepath.Join(dir, "update.tar.gz"), archive.Bytes(), 0600)
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	addr := listener.Addr().String()
	listener.Close()
	c := config{Listen: addr, PublicOrigin: "http://" + addr, AdminPath: "/abc123", Storage: "file", DataDir: filepath.Join(dir, "data")}
	b, _ := json.Marshal(c)
	configPath := filepath.Join(dir, "config.json")
	os.WriteFile(configPath, b, 0600)
	self, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	selfBytes, e := os.ReadFile(self)
	if e != nil {
		t.Fatal(e)
	}
	target := filepath.Join(dir, "hetun-operations")
	os.WriteFile(target, selfBytes, 0700)
	child := exec.Command(target, "-test.run=^TestUpdateExecHelper$")
	child.Env = append(os.Environ(), "HETUN_UPDATE_EXEC_HELPER="+dir)
	var logs bytes.Buffer
	child.Stdout = &logs
	child.Stderr = &logs
	if e = child.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() {
		child.Process.Signal(syscall.SIGTERM)
		child.Wait()
		if t.Failed() {
			t.Log(logs.String())
		}
	}()
	client := &http.Client{Timeout: time.Second}
	base := "http://" + addr
	request := func(path, body string) (int, []byte) {
		r, _ := http.NewRequest("POST", base+path, strings.NewReader(body))
		r.Header.Set("Origin", base)
		r.Header.Set("X-Hetun-Client", "studio")
		r.Header.Set("Content-Type", "application/json")
		resp, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, data
	}
	deadline := time.Now().Add(15 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		resp, e := client.Get(base + "/api/health")
		if e == nil {
			resp.Body.Close()
			ready = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		t.Fatal("old service not ready")
	}
	// Preserve the session returned by the real login handler.
	req, _ := http.NewRequest("POST", base+"/api/admin/login", strings.NewReader("{\"username\":\"admin\",\"password\":\"UpdateTest123!\"}"))
	req.Header.Set("Origin", base)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hetun-Client", "studio")
	resp, e := client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	cookies := resp.Cookies()
	resp.Body.Close()
	if resp.StatusCode != 200 || len(cookies) == 0 {
		t.Fatal("login failed", resp.StatusCode)
	}
	client.Transport = updateTransport(func(r *http.Request) (*http.Response, error) {
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		return http.DefaultTransport.RoundTrip(r)
	})
	configBefore, _ := os.ReadFile(configPath)
	stateBefore, _ := os.ReadFile(filepath.Join(c.DataDir, "state.json"))
	keyBefore, _ := os.ReadFile(filepath.Join(c.DataDir, "service-key.pem"))
	if code, data := request("/api/admin/system/update/check", "{}"); code != 200 {
		t.Fatalf("check %d %s", code, data)
	}
	if code, data := request("/api/admin/system/update/install", "{\"version\":\"99.0.0\"}"); code != 200 {
		t.Fatalf("install %d %s", code, data)
	}
	deadline = time.Now().Add(25 * time.Second)
	updated := false
	for time.Now().Before(deadline) {
		resp, e := client.Get(base + "/api/health")
		if e == nil {
			var v M
			json.NewDecoder(resp.Body).Decode(&v)
			resp.Body.Close()
			if v["version"] == "99.0.0" {
				updated = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !updated {
		t.Fatal("new service did not recover")
	}
	for file, want := range map[string][]byte{configPath: configBefore, filepath.Join(c.DataDir, "state.json"): stateBefore, filepath.Join(c.DataDir, "service-key.pem"): keyBefore} {
		got, e := os.ReadFile(file)
		if e != nil || !bytes.Equal(got, want) {
			t.Fatalf("persistent data changed %s %v", file, e)
		}
	}
	if _, e := os.Stat(target + ".previous"); e != nil {
		t.Fatal("previous binary missing")
	}
	if code, _ := request("/api/admin/system/update/check", "{}"); code != 401 {
		t.Fatal("old session survived restart", code)
	}
}
func TestUpdateExecHelper(t *testing.T) {
	dir := os.Getenv("HETUN_UPDATE_EXEC_HELPER")
	if dir == "" {
		return
	}
	configPath := filepath.Join(dir, "config.json")
	c, e := loadConfig(configPath)
	if e != nil {
		t.Fatal(e)
	}
	os.MkdirAll(c.DataDir, 0700)
	s := &fileStore{filepath.Join(c.DataDir, "state.json")}
	a, e := newApp(context.Background(), c, s)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.change(context.Background(), "setup.test", func(v *state) error {
		v.Credential, e = credentials("admin", "UpdateTest123!")
		v.Setup = M{"completedAt": now()}
		return e
	}); e != nil {
		t.Fatal(e)
	}
	target := filepath.Join(dir, "hetun-operations")
	u := newSystemUpdater(target, configPath)
	a.updater = u
	archive, _ := os.ReadFile(filepath.Join(dir, "update.tar.gz"))
	hash := sha256.Sum256(archive)
	asset := releaseAsset{Name: "Hetun-Operations-99.0.0-linux-" + u.arch + ".tar.gz", Size: int64(len(archive)), Digest: "sha256:" + hex.EncodeToString(hash[:])}
	asset.URL = "https://github.com/" + updateRepo + "/releases/download/v99.0.0/" + asset.Name
	g := githubRelease{Tag: "v99.0.0", URL: "https://github.com/" + updateRepo + "/releases/tag/v99.0.0", Assets: []releaseAsset{asset}}
	u.client = &http.Client{Transport: updateTransport(func(r *http.Request) (*http.Response, error) {
		data := archive
		if r.URL.String() == releaseAPI {
			data, _ = json.Marshal(g)
		} else if r.URL.String() != asset.URL {
			return nil, fmt.Errorf("unexpected URL")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}, nil
	})}
	e = serveWithUpdates(c.Listen, a, u.restart)
	a.cache.close()
	s.Close()
	p, ok := e.(*preparedUpdate)
	if !ok {
		t.Fatal(e)
	}
	os.Args = []string{target, "--config", configPath}
	if e = activateUpdate(p); e != nil {
		t.Fatal(e)
	}
}
