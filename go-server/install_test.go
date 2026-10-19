package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	mysql "github.com/go-sql-driver/mysql"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func installRequest(i *installer, method, route string, v any, origin string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(v)
	r := httptest.NewRequest(method, "http://127.0.0.1:8787"+route, bytes.NewReader(b))
	r.Header.Set("Origin", origin)
	r.Header.Set("X-Hetun-Install", "1")
	w := httptest.NewRecorder()
	i.ServeHTTP(w, r)
	return w
}
func TestInstallerLifecycleAndGuards(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.json")
	i, e := newInstaller(file, "127.0.0.1:8787")
	if e != nil {
		t.Fatal(e)
	}
	defer i.Close()
	same, e := newInstaller(file, "127.0.0.1:8787")
	if e != nil || same.route != i.route {
		t.Fatal("install token must survive restart", e)
	}
	storage := &fileStore{filepath.Join(t.TempDir(), "state.json")}
	i.connect = func(string) (store, error) { return storage, nil }
	i.probe = func(context.Context, string) error { return nil }
	v := installInput{PublicOrigin: "http://127.0.0.1:8787", Host: "127.0.0.1", Port: 3306, Database: "isolated", Username: "owner", Password: "db@pass:/?#", Admin: "owner", AdminPassword: "StrongLocal12345"}
	c, e := i.configuration(v)
	if e != nil {
		t.Fatal(e)
	}
	parsed, e := mysql.ParseDSN(c.MySQLDSN)
	if e != nil || parsed.Passwd != v.Password || parsed.DBName != v.Database {
		t.Fatal("database credentials changed", e)
	}
	for _, route := range []string{"/", "/install/wrong", "/config.json", "/data/install-token.txt", "/api/admin/state"} {
		if w := installRequest(i, "GET", route, nil, ""); w.Code != 404 {
			t.Fatal(route, w.Code)
		}
	}
	w := installRequest(i, "GET", i.route, nil, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "测试数据库连接") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "nonce-") {
		t.Fatal(w.Code)
	}
	for _, origin := range []string{"", "https://foreign.test"} {
		if w = installRequest(i, "POST", i.route+"/complete", v, origin); w.Code != 403 {
			t.Fatal("cross origin accepted")
		}
	}
	if w = installRequest(i, "POST", i.route+"/test", v, v.PublicOrigin); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if _, e = os.Stat(file); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("test wrote config")
	}
	if _, e = storage.Load(context.Background()); !errors.Is(e, errNoState) {
		t.Fatal("test wrote database")
	}
	bad := v
	bad.AdminPassword = "short"
	if w = installRequest(i, "POST", i.route+"/complete", bad, v.PublicOrigin); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = installRequest(i, "POST", i.route+"/complete", v, v.PublicOrigin); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	loaded, e := loadConfig(file)
	if e != nil || loaded.Storage != "mysql" || loaded.PublicOrigin != v.PublicOrigin {
		t.Fatal(e)
	}
	state, e := storage.Load(context.Background())
	if e != nil || !matches(state.Credential, v.AdminPassword) || str(state.Setup["completedAt"]) == "" {
		t.Fatal("admin/setup not persisted", e)
	}
	if _, e = os.Stat(filepath.Join(i.dataDir, "install-token.txt")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("temporary entry not removed")
	}
	if w = installRequest(i, "POST", i.route+"/complete", v, v.PublicOrigin); w.Code != 404 {
		t.Fatal("reinstall still open", w.Code)
	}
	if w = installRequest(i, "GET", "/api/admin/state", nil, ""); w.Code != 401 {
		t.Fatal("admin unprotected", w.Code)
	}
	// Reopen from persisted config/state; chosen credentials work after restart.
	restarted, e := newApp(context.Background(), loaded, storage)
	if e != nil {
		t.Fatal(e)
	}
	for _, route := range []string{"/install", i.route, i.route + "/complete"} {
		w := httptest.NewRecorder()
		restarted.ServeHTTP(w, httptest.NewRequest("GET", v.PublicOrigin+route, nil))
		if w.Code != 404 {
			t.Fatal("restarted installer route must stay closed", route, w.Code)
		}
	}
	r := httptest.NewRequest("POST", v.PublicOrigin+"/api/admin/login", strings.NewReader(`{"username":"owner","password":"StrongLocal12345"}`))
	r.Header.Set("Origin", v.PublicOrigin)
	r.Header.Set("X-Hetun-Client", "studio")
	w = httptest.NewRecorder()
	restarted.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
}
func TestInstallerRejectsExistingStateAndConnectionFailures(t *testing.T) {
	i, e := newInstaller(filepath.Join(t.TempDir(), "config.json"), "127.0.0.1:8787")
	if e != nil {
		t.Fatal(e)
	}
	defer i.Close()
	v := installInput{PublicOrigin: "http://127.0.0.1:8787", Host: "127.0.0.1", Port: 3306, Database: "isolated", Username: "owner", Admin: "owner", AdminPassword: "StrongLocal12345"}
	i.probe = func(context.Context, string) error { return errors.New("do-not-leak-password") }
	if w := installRequest(i, "POST", i.route+"/test", v, v.PublicOrigin); w.Code != 400 || strings.Contains(w.Body.String(), "do-not-leak") {
		t.Fatal(w.Code, w.Body)
	}
	s := &fileStore{filepath.Join(t.TempDir(), "state.json")}
	old := initialState()
	old.Credential, _ = credentials("existing", "ExistingPass12345")
	if e = s.Save(context.Background(), old, 0); e != nil {
		t.Fatal(e)
	}
	i.connect = func(string) (store, error) { return s, nil }
	if w := installRequest(i, "POST", i.route+"/complete", v, v.PublicOrigin); w.Code != 409 {
		t.Fatal(w.Code, w.Body)
	}
	after, _ := s.Load(context.Background())
	if !matches(after.Credential, "ExistingPass12345") {
		t.Fatal("existing account overwritten")
	}
	// Simulate retry after the database was saved but the config write failed.
	old.Setup = M{"completedAt": now(), "installationID": i.installationID}
	old.Credential, _ = credentials(v.Admin, v.AdminPassword)
	old.Revision = 2
	if e = s.Save(context.Background(), old, 1); e != nil {
		t.Fatal(e)
	}
	if w := installRequest(i, "POST", i.route+"/complete", v, v.PublicOrigin); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
}
func TestInstallerMySQL(t *testing.T) {
	dsn := os.Getenv("HETUN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("requires isolated MySQL test database")
	}
	db, e := mysql.ParseDSN(dsn)
	if e != nil {
		t.Fatal("invalid test DSN")
	}
	host, portText, ok := strings.Cut(db.Addr, ":")
	if !ok {
		t.Fatal("test DB needs host:port")
	}
	var port int
	if port, e = strconv.Atoi(portText); e != nil {
		t.Fatal(e)
	}
	i, e := newInstaller(filepath.Join(t.TempDir(), "config.json"), "127.0.0.1:8787")
	if e != nil {
		t.Fatal(e)
	}
	defer i.Close()
	t.Cleanup(func() {
		cleanup, err := sql.Open("mysql", dsn)
		if err != nil {
			t.Error(err)
			return
		}
		defer cleanup.Close()
		if _, err = cleanup.Exec("DELETE FROM hetun_operations_state WHERE id=1 AND JSON_UNQUOTE(JSON_EXTRACT(payload,'$.setup.installationID'))=?", i.installationID); err != nil {
			t.Error(err)
		}
	})
	v := installInput{PublicOrigin: "http://127.0.0.1:8787", Host: host, Port: port, Database: db.DBName, Username: db.User, Password: db.Passwd, Admin: "owner", AdminPassword: "StrongLocal12345"}
	if w := installRequest(i, "POST", i.route+"/test", v, v.PublicOrigin); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w := installRequest(i, "POST", i.route+"/complete", v, v.PublicOrigin); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if !matches(i.active.readState().Credential, v.AdminPassword) {
		t.Fatal("mysql lost chosen credentials")
	}
	if e = i.active.change(context.Background(), "test.persist", func(s *state) error { s.UpdatedAt = "mysql-persisted"; return nil }); e != nil {
		t.Fatal(e)
	}
	loaded, e := i.active.store.Load(context.Background())
	if e != nil || loaded.UpdatedAt != "mysql-persisted" {
		t.Fatal("mysql save failed", e)
	}
	// MySQL compare-and-swap rejects a stale write.
	if e = i.active.store.Save(context.Background(), loaded, loaded.Revision-1); e == nil {
		t.Fatal("stale mysql write accepted")
	}
	c, e := loadConfig(i.configPath)
	if e != nil {
		t.Fatal(e)
	}
	s, e := openMySQL(c.MySQLDSN)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	a, e := newApp(context.Background(), c, s)
	if e != nil || a.readState().UpdatedAt != "mysql-persisted" {
		t.Fatal("mysql restart failed", e)
	}
	// Confirm the installation probe has a deadline even when a DB is unavailable.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if e = probeMySQL(ctx, "bad:bad@tcp(127.0.0.1:1)/missing?timeout=1s"); e == nil {
		t.Fatal("invalid DB accepted")
	}
}
