package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/alicebob/miniredis/v2"
	mysql "github.com/go-sql-driver/mysql"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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
	v := installInput{Redis: installerTestRedis(t), PublicOrigin: "http://127.0.0.1:8787", Host: "127.0.0.1", Port: 3306, Database: "isolated", Username: "owner", Password: "db@pass:/?#", Admin: "owner", AdminPassword: "1"}
	c, e := i.configuration(v)
	if e != nil {
		t.Fatal(e)
	}
	parsed, e := mysql.ParseDSN(c.MySQLDSN)
	if e != nil || parsed.Passwd != v.Password || parsed.DBName != v.Database {
		t.Fatal("database credentials changed", e)
	}
	for _, route := range []string{"/install/wrong", "/config.json", "/data/install-token.txt", "/api/admin/state"} {
		if w := installRequest(i, "GET", route, nil, ""); w.Code != 404 {
			t.Fatal(route, w.Code)
		}
	}
	if w := installRequest(i, "GET", "/", nil, ""); w.Code != 307 || w.Header().Get("Location") != i.adminPath() {
		t.Fatal("root must lead to installation", w.Code)
	}
	if w := installRequest(i, "GET", i.adminPath(), nil, ""); w.Code != 200 || !strings.Contains(w.Body.String(), i.route+"/complete") && !strings.Contains(w.Body.String(), i.route+"/") {
		t.Fatal("random entry must serve working installer", w.Code)
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
	bad.AdminPassword = ""
	if w = installRequest(i, "POST", i.route+"/complete", bad, v.PublicOrigin); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w = installRequest(i, "POST", i.route+"/complete", v, v.PublicOrigin); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	// The first installed session must expose updates without a process restart.
	if i.active.updater == nil || i.active.updater != i.updater || i.active.updater.configPath != i.configPath {
		t.Fatal("installer did not hand off its updater and restart channel")
	}
	login := httptest.NewRequest("POST", v.PublicOrigin+"/api/admin/login", strings.NewReader(`{"username":"owner","password":"1"}`))
	login.Header.Set("Origin", v.PublicOrigin)
	login.Header.Set("X-Hetun-Client", "studio")
	loginResponse := httptest.NewRecorder()
	i.ServeHTTP(loginResponse, login)
	if loginResponse.Code != 200 || len(loginResponse.Result().Cookies()) == 0 {
		t.Fatal("first-session login failed", loginResponse.Code)
	}
	fixture, _, _ := fixtureUpdater(t)
	i.updater.client = fixture.client
	for _, endpoint := range []struct{ method, path string }{{"GET", "/api/admin/system/update"}, {"POST", "/api/admin/system/update/check"}} {
		req := httptest.NewRequest(endpoint.method, v.PublicOrigin+endpoint.path, strings.NewReader("{}"))
		req.Header.Set("Origin", v.PublicOrigin)
		req.Header.Set("X-Hetun-Client", "studio")
		req.AddCookie(loginResponse.Result().Cookies()[0])
		response := httptest.NewRecorder()
		i.ServeHTTP(response, req)
		var view updateView
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &view) != nil || view.Current != buildVersion {
			t.Fatal("first-session update API unavailable", endpoint.path, response.Code, response.Body.String())
		}
		if endpoint.method == "POST" && (view.Latest == nil || view.Latest.Version != "99.0.0") {
			t.Fatal("first-session release check failed", response.Body.String())
		}
	}
	loaded, e := loadConfig(file)
	if e != nil || loaded.Storage != "mysql" || loaded.PublicOrigin != v.PublicOrigin || loaded.AdminPath != i.adminPath() {
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
	for _, route := range []string{"/", "/index.html", "/anything", "/install", i.route, i.route + "/complete"} {
		w := httptest.NewRecorder()
		restarted.ServeHTTP(w, httptest.NewRequest("GET", v.PublicOrigin+route, nil))
		if w.Code != 404 {
			t.Fatal("restarted installer route must stay closed", route, w.Code)
		}
	}
	wEntry := httptest.NewRecorder()
	restarted.ServeHTTP(wEntry, httptest.NewRequest("GET", v.PublicOrigin+loaded.AdminPath, nil))
	if wEntry.Code != 200 {
		t.Fatal("saved admin entry unavailable", wEntry.Code)
	}
	r := httptest.NewRequest("POST", v.PublicOrigin+"/api/admin/login", strings.NewReader(`{"username":"owner","password":"1"}`))
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
	v := installInput{Redis: installerTestRedis(t), PublicOrigin: "http://127.0.0.1:8787", Host: "127.0.0.1", Port: 3306, Database: "isolated", Username: "owner", Admin: "owner", AdminPassword: "StrongLocal12345"}
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
	v := installInput{Redis: installerTestRedis(t), PublicOrigin: "http://127.0.0.1:8787", Host: host, Port: port, Database: db.DBName, Username: db.User, Password: db.Passwd, Admin: "owner", AdminPassword: "StrongLocal12345"}
	// Validate real server error classification before creating any installation state.
	for _, tc := range []struct {
		name, code string
		mutate     func(*installInput)
	}{
		{"missing-database", "database_not_found", func(x *installInput) { x.Database = "hetun_missing_" + randomToken()[:16] }},
		{"invalid-login", "authentication_failed", func(x *installInput) { x.Username = "hetun_missing_" + randomToken()[:12]; x.Password = randomToken() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := v
			tc.mutate(&bad)
			w := installRequest(i, "POST", i.route+"/test", bad, v.PublicOrigin)
			var result struct{ Code string }
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if w.Code != 400 || result.Code != tc.code {
				t.Fatalf("real MySQL diagnostic: %d %s", w.Code, w.Body)
			}
		})
	}
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

func TestInstallerDatabaseDiagnostics(t *testing.T) {
	i, err := newInstaller(filepath.Join(t.TempDir(), "config.json"), "127.0.0.1:8787")
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()
	v := installInput{Redis: installerTestRedis(t), PublicOrigin: "http://127.0.0.1:8787", Host: "127.0.0.1", Port: 3306, Database: "isolated", Username: "owner", Admin: "owner", AdminPassword: "StrongLocal12345"}
	tests := []struct {
		name   string
		err    error
		code   string
		number uint16
	}{
		{"auth", &mysql.MySQLError{Number: 1045, Message: "SECRET_PASSWORD"}, "authentication_failed", 1045},
		{"missing", fmt.Errorf("wrapped: %w", &mysql.MySQLError{Number: 1049, Message: "SECRET_PASSWORD"}), "database_not_found", 1049},
		{"access", &mysql.MySQLError{Number: 1044, Message: "SECRET_PASSWORD"}, "permission_denied", 1044},
		{"table", &mysql.MySQLError{Number: 1142, Message: "SECRET_PASSWORD"}, "permission_denied", 1142},
		{"host", &mysql.MySQLError{Number: 1130, Message: "SECRET_PASSWORD"}, "host_denied", 1130},
		{"limit", &mysql.MySQLError{Number: 1040, Message: "SECRET_PASSWORD"}, "connection_limit", 1040},
		{"timeout", fmt.Errorf("SECRET_PASSWORD: %w", context.DeadlineExceeded), "connection_timeout", 0},
		{"dns", &net.DNSError{Err: "SECRET_PASSWORD", Name: "private-host", IsNotFound: true}, "host_not_found", 0},
		{"refused", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, "connection_refused", 0},
		{"unknown", errors.New("SECRET_PASSWORD"), "database_error", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			i.probe = func(context.Context, string) error { return tc.err }
			i.connect = func(string) (store, error) { return nil, tc.err }
			for _, action := range []string{"test", "complete"} {
				w := installRequest(i, "POST", i.route+"/"+action, v, v.PublicOrigin)
				var result struct {
					Error     string
					Code      string
					Stage     string
					MySQLCode uint16
				}
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				stage := "connect"
				if action == "complete" {
					stage = "initialize"
				}
				if w.Code != 400 || result.Code != tc.code || result.MySQLCode != tc.number || result.Stage != stage || result.Error == "" {
					t.Fatalf("unexpected diagnostic: %d %s", w.Code, w.Body)
				}
				if strings.Contains(w.Body.String(), "SECRET_PASSWORD") || strings.Contains(w.Body.String(), "private-host") {
					t.Fatal("driver details leaked")
				}
			}
		})
	}
}

func installerTestRedis(t *testing.T) *redisConfig {
	t.Helper()
	server := miniredis.RunT(t)
	return &redisConfig{Address: server.Addr()}
}
