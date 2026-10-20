package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCMSAddressNormalization(t *testing.T) {
	for input, want := range map[string]string{
		"cms.example.test":       "https://cms.example.test/api.php/provide/vod/",
		" cms.example.test/cms ": "https://cms.example.test/cms/api.php/provide/vod/",
		"http://cms.example.test:8080/api.php/provide/vod/?ac=detail": "http://cms.example.test:8080/api.php/provide/vod/",
	} {
		u, err := cmsEndpoint(input)
		if err != nil || u.String() != want {
			t.Fatalf("%s: %v %v", input, u, err)
		}
	}
	for _, input := range []string{"ftp://cms.example.test", "https://user:secret@cms.example.test", "/etc/passwd", "https://cms.example.test/#frag"} {
		if _, err := cmsEndpoint(input); err == nil {
			t.Fatalf("invalid accepted: %s", input)
		}
	}
}

func TestCMSBindingsRecoverStaleMetadataWithoutWritingCMS(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	parsed, _ := url.Parse(cms.URL)
	root := t.TempDir()
	dir := fakeCMSSite(t, root, "site", "https://old.example.invalid")
	configFile := filepath.Join(dir, "application", "extra", "maccms.php")
	before, _ := os.ReadFile(configFile)
	bindings := t.TempDir()
	configuration := fmt.Sprintf(`# outside root /incorrect;
server { listen %s; server_name 127.0.0.1; root "%s"; location / { root /ignore; } }
server { listen 443 ssl; server_name unrelated.example.test; root /unrelated; }
`, parsed.Port(), filepath.ToSlash(dir))
	if err := os.WriteFile(filepath.Join(bindings, "site.conf"), []byte(configuration), 0600); err != nil {
		t.Fatal(err)
	}
	sites, _ := scanCMSSites([]string{root})
	warnings := addCMSBindings([]string{bindings}, sites)
	if len(warnings) != 0 || len(sites) != 1 || str(sites[0]["base"]) != cms.URL+"/api.php/provide/vod/" {
		t.Fatal(sites, warnings)
	}
	h := newHarness(t, "")
	h.a.probeCMSCandidate(context.Background(), sites[0])
	if obj(sites[0]["report"])["ok"] != true || sites[0]["source"] != "宝塔 Nginx 站点绑定" {
		t.Fatal(sites)
	}
	after, _ := os.ReadFile(configFile)
	if string(before) != string(after) {
		t.Fatal("discovery modified CMS")
	}
}

func TestCMSBindingsSubdirectoryAndIsolation(t *testing.T) {
	root := t.TempDir()
	site := fakeCMSSite(t, root, "cms", "")
	// A sibling with a similar prefix must never inherit this server's domain.
	other := fakeCMSSite(t, t.TempDir(), "cms", "")
	bindings := t.TempDir()
	configuration := fmt.Sprintf(`server { listen 8443 ssl; server_name media.example.test; root "%s"; location / { set $text "} # ;"; root /ignore; } }`, filepath.ToSlash(root))
	os.WriteFile(filepath.Join(bindings, "site.conf"), []byte(configuration), 0600)
	candidates := []M{{"root": site, "addresses": []M{}}, {"root": other, "addresses": []M{}}}
	addCMSBindings([]string{bindings}, candidates)
	if candidates[0]["base"] != "https://media.example.test:8443/cms/api.php/provide/vod/" {
		t.Fatal(candidates)
	}
	if candidates[1]["base"] != nil {
		t.Fatal("unrelated root was matched", candidates)
	}
}

func TestCMSCandidateFallsBackToHealthyAddress(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "closed") }))
	defer bad.Close()
	good := fixtureCMS(t)
	defer good.Close()
	candidate := M{"base": bad.URL, "addresses": []M{}}
	addCMSAddress(candidate, bad.URL, "旧配置")
	addCMSAddress(candidate, good.URL, "站点绑定")
	h := newHarness(t, "")
	h.a.probeCMSCandidate(context.Background(), candidate)
	if obj(candidate["report"])["ok"] != true || candidate["base"] != good.URL+"/api.php/provide/vod/" || len(candidate["attempts"].([]M)) != 2 {
		t.Fatal(candidate)
	}
	if obj(candidate["attempts"].([]M)[0]["report"])["ok"] != false {
		t.Fatal("failed attempt lost", candidate)
	}
}

func TestDiscoveryRepairsBrokenSelectionAndReportsCurrentHealth(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "closed") }))
	defer bad.Close()
	good := fixtureCMS(t)
	defer good.Close()
	h := newHarness(t, "")
	root := t.TempDir()
	h.a.config.CMSRoots = []string{root}
	fakeCMSSite(t, root, "one", good.URL)
	if err := h.a.change(context.Background(), "test", func(next *state) error { next.CMSBase = bad.URL; return nil }); err != nil {
		t.Fatal(err)
	}
	result := h.a.discoverAndConnect(context.Background())
	if result["connected"] != true || result["connectedBase"] != good.URL+"/api.php/provide/vod/" || obj(result["currentReport"])["ok"] != true {
		t.Fatal(result)
	}
	again := h.a.discoverAndConnect(context.Background())
	if again["connected"] != true {
		t.Fatal("saved healthy connection lost", again)
	}
}

func TestDiscoveryDoesNotOverwriteConcurrentManualSelection(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	cms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(entered) })
		<-release
		jsonResponse(w, 200, M{"class": []any{}, "list": []any{}})
	}))
	defer cms.Close()
	h := newHarness(t, "")
	root := t.TempDir()
	h.a.config.CMSRoots = []string{root}
	fakeCMSSite(t, root, "one", cms.URL)
	result := make(chan M, 1)
	go func() { result <- h.a.discoverAndConnect(context.Background()) }()
	<-entered
	err := h.a.change(context.Background(), "test", func(next *state) error { next.CMSBase = "https://manual.example.test/api.php/provide/vod/"; return nil })
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	got := <-result
	if h.a.readState().CMSBase != "https://manual.example.test/api.php/provide/vod/" || got["connected"] != false || !strings.Contains(strings.Join(got["warnings"].([]string), " "), "其他操作") {
		t.Fatal(got)
	}
}

func TestDiscoveryRetainsHealthyUnnormalizedSelectionAmongMultipleSites(t *testing.T) {
	first := fixtureCMS(t)
	defer first.Close()
	second := fixtureCMS(t)
	defer second.Close()
	h := newHarness(t, "")
	sitesRoot := t.TempDir()
	h.a.config.CMSRoots = []string{sitesRoot}
	fakeCMSSite(t, sitesRoot, "one", first.URL)
	fakeCMSSite(t, sitesRoot, "two", second.URL)
	if err := h.a.change(context.Background(), "test", func(next *state) error { next.CMSBase = first.URL; return nil }); err != nil {
		t.Fatal(err)
	}
	result := h.a.discoverAndConnect(context.Background())
	if result["connected"] != true || result["multiple"] != true || result["connectedBase"] != first.URL || obj(result["currentReport"])["ok"] != true {
		t.Fatal(result)
	}
	if h.a.readState().CMSBase != first.URL {
		t.Fatal("healthy selection was overwritten")
	}
}
