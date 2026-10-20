package main

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func fakeCMSSite(t *testing.T, root, name, address string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if e := os.MkdirAll(filepath.Join(dir, "application", "extra"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "api.php"), []byte("<?php"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "application", "extra", "maccms.php"), []byte("<?php return ['site_name'=>'Test CMS','site_url'=>'"+address+"','install_dir'=>'/'];"), 0600); e != nil {
		t.Fatal(e)
	}
	return dir
}
func TestDiscoveryConnectsOnlyUniqueHealthySite(t *testing.T) {
	cms := fixtureCMS(t)
	defer cms.Close()
	h := newHarness(t, "")
	root := t.TempDir()
	h.a.config.CMSRoots = []string{root}
	fakeCMSSite(t, root, "one", cms.URL)
	result := h.a.discoverAndConnect(context.Background())
	if result["connected"] != true || h.a.readState().CMSBase == "" {
		t.Fatal(result)
	}
	before := h.a.readState().CMSBase
	cms2 := fixtureCMS(t)
	defer cms2.Close()
	fakeCMSSite(t, root, "two", cms2.URL)
	result = h.a.discoverAndConnect(context.Background())
	if result["multiple"] != true || h.a.readState().CMSBase != before {
		t.Fatal("existing selection overwritten", result)
	}
	h2 := newHarness(t, "")
	h2.a.config.CMSRoots = []string{root}
	result = h2.a.discoverAndConnect(context.Background())
	if result["multiple"] != true || h2.a.readState().CMSBase != "" {
		t.Fatal("ambiguous auto connection", result)
	}
}
func TestDiscoveryPathsAndPublicMetadata(t *testing.T) {
	root := t.TempDir()
	dir := fakeCMSSite(t, root, "cms.example.test", "")
	file := filepath.Join(dir, "application", "extra", "maccms.php")
	os.WriteFile(file, []byte("<?php return ['site_url'=>'https://cms.example.test/cms','install_dir'=>'/cms/'];"), 0600)
	sites, _ := scanCMSSites([]string{root})
	if len(sites) != 1 || str(sites[0]["base"]) != "https://cms.example.test/cms/api.php/provide/vod/" {
		t.Fatal(sites)
	}
	os.WriteFile(file, []byte("<?php return ['site_name'=>'CMS'];"), 0600)
	sites, _ = scanCMSSites([]string{root})
	if len(sites) != 1 || str(sites[0]["base"]) != "https://cms.example.test/api.php/provide/vod/" {
		t.Fatal(sites)
	}
	if withinDirectory(root, filepath.Join(root, "..", "outside")) {
		t.Fatal("escaped scan boundary")
	}
	h := newHarness(t, "")
	h.a.config.CMSRoots = []string{t.TempDir()}
	result := h.a.discoverAndConnect(context.Background())
	if result["connected"] == true || len(result["warnings"].([]string)) == 0 {
		t.Fatal(result)
	}
}
func verifyDirectory(t *testing.T, envelope M) M {
	t.Helper()
	data, e := base64.StdEncoding.DecodeString(str(envelope["payload"]))
	if e != nil {
		t.Fatal(e)
	}
	der, _ := base64.StdEncoding.DecodeString(str(envelope["publicKey"]))
	raw, e := x509.ParsePKIXPublicKey(der)
	if e != nil {
		t.Fatal(e)
	}
	sig, _ := base64.StdEncoding.DecodeString(str(envelope["signature"]))
	hash := sha256.Sum256(data)
	if e = rsa.VerifyPKCS1v15(raw.(*rsa.PublicKey), crypto.SHA256, hash[:], sig); e != nil {
		t.Fatal(e)
	}
	var payload M
	if e = json.Unmarshal(data, &payload); e != nil {
		t.Fatal(e)
	}
	return payload
}
func TestSignedDirectoryPersistsIdentityAndChecksAdmin(t *testing.T) {
	h := newHarness(t, "")
	first := payload(t, h.req("GET", "/api/service-directory", nil, ""))
	verifyDirectory(t, first)
	saved := h.cookie
	h.cookie = ""
	if w := h.req("PUT", "/api/admin/service-addresses", M{"addresses": []string{"https://new.example.test"}}, ""); w.Code != 401 {
		t.Fatal(w.Code, w.Body)
	}
	h.cookie = saved
	for _, address := range []string{"http://bad.test", "https://user:pass@bad.test", "https://bad.test/admin", "https://bad.test?key=a"} {
		if w := h.req("PUT", "/api/admin/service-addresses", M{"addresses": []string{address}}, ""); w.Code != 400 {
			t.Fatal(address, w.Code, w.Body)
		}
	}
	w := h.req("PUT", "/api/admin/service-addresses", M{"addresses": []string{"https://new.example.test", "https://backup.example.test"}, "recoveryUrls": []string{"https://recovery.example.test/service.json"}}, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	next := payload(t, w)
	signed := obj(next["directory"])
	data := verifyDirectory(t, signed)
	if str(signed["publicKey"]) != str(first["publicKey"]) || str(arr(data["addresses"])[0]) != "https://new.example.test" {
		t.Fatal(data)
	}
	revision := data["revision"]
	if e := h.a.change(context.Background(), "test.unrelated", func(next *state) error { return nil }); e != nil {
		t.Fatal(e)
	}
	unchanged := verifyDirectory(t, h.a.directory(h.a.readState()))
	if unchanged["revision"] != revision {
		t.Fatal("unrelated change invalidated recovery directory", unchanged)
	}
	a, e := newApp(context.Background(), h.a.config, h.a.store)
	if e != nil {
		t.Fatal(e)
	}
	if str(a.directory(a.readState())["publicKey"]) != str(first["publicKey"]) {
		t.Fatal("identity changed after restart")
	}
}
