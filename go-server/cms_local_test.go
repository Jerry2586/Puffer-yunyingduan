package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestCMSStaticPHPConfiguration(t *testing.T) {
	data := []byte(`<?php /* credential fixture */ return array('type'=>'mysql', 'password'=>'pa\\ss\'word', 'hostname'=>'127.0.0.1', 'hostport'=>3306, 'prefix'=>'mac_', 'flag'=>false, 'other'=>['a'=>1,],);`)
	m, e := parseCMSDatabase(data)
	if e != nil || m["password"] != "pa\\ss'word" || m["hostport"] != 3306 {
		t.Fatal("literal parsing failed", e)
	}
	for _, bad := range []string{`<?php return ['password'=>getenv('SECRET')];`, `<?php return ['password'=>"$SECRET"];`, `<?php return ['password'=>'x']; file_put_contents('/tmp/executed','1');`, `<?php return include 'secret.php';`, `<?php return ['password'=>'SECRET' . 'suffix'];`} {
		if _, e := parseCMSDatabase([]byte(bad)); e == nil || strings.Contains(e.Error(), "SECRET") {
			t.Fatal("unsafe configuration accepted or leaked")
		}
	}
}
func TestCMSLocalConfigurationBoundaries(t *testing.T) {
	root := t.TempDir()
	site := fakeCMSSite(t, root, "cms", "https://cms.example.test")
	file := filepath.Join(site, "application", "database.php")
	a := &app{config: config{CMSRoots: []string{root}}}
	for _, tc := range []struct {
		source string
		valid  bool
	}{
		{`<?php return ['type'=>'mysql','hostname'=>'127.0.0.1','hostport'=>'3306','database'=>'cms','username'=>'reader','password'=>'secret','prefix'=>'mac_'];`, true},
		{`<?php return ['type'=>'mysql','hostname'=>'external.example.test','database'=>'cms','username'=>'reader','prefix'=>'mac_'];`, false},
		{`<?php return ['type'=>'mysql','hostname'=>'localhost','database'=>'cms','username'=>'reader','prefix'=>'mac_;DROP'];`, false},
		{`<?php return ['type'=>'mysql','hostname'=>'localhost','database'=>'cms','username'=>'reader','prefix'=>'mac_','dsn'=>'mysql:host=external'];`, false},
	} {
		os.WriteFile(file, []byte(tc.source), 0600)
		_, _, e := a.localCMSConfig(site)
		if (e == nil) != tc.valid {
			t.Fatal("configuration validation", e)
		}
	}
	if _, _, e := a.localCMSConfig(t.TempDir()); e == nil {
		t.Fatal("outside root accepted")
	}
	os.Remove(file)
	outside := filepath.Join(t.TempDir(), "outside.php")
	os.WriteFile(outside, []byte("<?php return [];"), 0600)
	if e := os.Symlink(outside, file); e == nil {
		if _, _, e := a.localCMSConfig(site); e == nil {
			t.Fatal("outside symlink accepted")
		}
	}
}

func TestCMSLocalReadWithPublicAPIOff(t *testing.T) {
	dsn := os.Getenv("HETUN_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("requires isolated MySQL")
	}
	cfg, e := mysql.ParseDSN(dsn)
	if e != nil || cfg.DBName != "hetun_test" {
		t.Fatal("only isolated hetun_test database is permitted")
	}
	db, e := sql.Open("mysql", dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	prefix := "local_" + strings.ToLower(randomToken()[:12])
	prefix = strings.NewReplacer("-", "a", "_", "b").Replace(prefix) + "_"
	if !cmsIdentifier.MatchString(prefix) {
		t.Fatal("invalid fixture prefix")
	}
	typ := "`" + prefix + "type`"
	vod := "`" + prefix + "vod`"
	if _, e = db.Exec("CREATE TABLE " + typ + " (type_id INT PRIMARY KEY,type_pid INT,type_name VARCHAR(80),type_mid INT,type_status INT,type_sort INT)"); e != nil {
		t.Fatal(e)
	}
	defer db.Exec("DROP TABLE " + typ)
	if _, e = db.Exec("CREATE TABLE " + vod + " (vod_id INT PRIMARY KEY,type_id INT,type_id_1 INT,vod_status INT,vod_time INT,vod_name VARCHAR(80),vod_pic TEXT,vod_year TEXT,vod_area TEXT,vod_score TEXT,vod_actor TEXT,vod_director TEXT,vod_remarks TEXT,vod_state TEXT,vod_content TEXT,vod_blurb TEXT,vod_play_from TEXT,vod_play_url TEXT)"); e != nil {
		t.Fatal(e)
	}
	defer db.Exec("DROP TABLE " + vod)
	if _, e = db.Exec("INSERT INTO " + typ + " VALUES (1,0,'电影',1,1,0),(2,0,'隐藏分类',1,0,0)"); e != nil {
		t.Fatal(e)
	}
	for i := 1; i <= 25; i++ {
		status, category := 1, 1
		if i == 24 {
			status = 0
		}
		if i == 25 {
			category = 2
		}
		if _, e = db.Exec("INSERT INTO "+vod+" VALUES (?,?,1,?,?,?,'','','','9','','','','','','','line','正片$https://private.invalid/video')", i, category, status, i, fmt.Sprintf("电影%d", i)); e != nil {
			t.Fatal(e)
		}
	}
	root := t.TempDir()
	var requests atomic.Int32
	cms := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); fmt.Fprint(w, "closed") }))
	defer cms.Close()
	site := fakeCMSSite(t, root, "cms", cms.URL)
	host, port, e := net.SplitHostPort(cfg.Addr)
	if e != nil {
		t.Fatal(e)
	}
	quote := func(s string) string { return "'" + strings.NewReplacer("\\", "\\\\", "'", "\\'").Replace(s) + "'" }
	source := fmt.Sprintf("<?php return ['type'=>'mysql','hostname'=>%s,'hostport'=>%s,'database'=>%s,'username'=>%s,'password'=>%s,'prefix'=>%s];", quote(host), quote(port), quote(cfg.DBName), quote(cfg.User), quote(cfg.Passwd), quote(prefix))
	if e = os.WriteFile(filepath.Join(site, "application", "database.php"), []byte(source), 0600); e != nil {
		t.Fatal(e)
	}
	h := newHarness(t, "")
	h.a.config.CMSRoots = []string{root}
	report := h.a.discoverAndConnect(context.Background())
	if report["connected"] != true || h.a.readState().CMSLocalRoot != site || obj(report["currentReport"])["transport"] != "local" {
		t.Fatal("local discovery failed", report)
	}
	if requests.Load() != 0 {
		t.Fatal("public API was requested")
	}
	result, e := h.a.catalog(context.Background(), cms.URL, url.Values{})
	if e != nil || result["total"] != 23 || len(arr(result["items"])) != 20 {
		t.Fatal("catalog read", result, e)
	}
	episode := obj(arr(obj(arr(obj(arr(result["items"])[0])["sources"])[0])["episodes"])[0])
	if episode["url"] != "" {
		t.Fatal("play URL leaked")
	}
	result, e = h.a.catalog(context.Background(), cms.URL, url.Values{"page": {"2"}})
	if e != nil || len(arr(result["items"])) != 3 {
		t.Fatal("pagination", result, e)
	}
	result, e = h.a.catalog(context.Background(), cms.URL, url.Values{"ids": {"2,1"}})
	if e != nil || obj(arr(result["items"])[0])["id"] != "2" {
		t.Fatal("manual ids", result, e)
	}
	for _, keyword := range []string{"%", "' OR 1=1 --"} {
		result, e = h.a.catalog(context.Background(), cms.URL, url.Values{"keyword": {keyword}})
		if e != nil || len(arr(result["items"])) != 0 {
			t.Fatal("search escaped", e)
		}
	}
	if _, e = h.a.localCatalog(context.Background(), site, url.Values{"ids": {"1) OR 1=1"}}); e == nil {
		t.Fatal("injection accepted")
	}
	var count int
	if e = db.QueryRow("SELECT COUNT(*) FROM " + vod).Scan(&count); e != nil || count != 25 {
		t.Fatal("CMS modified")
	}
	saved, e := h.a.store.Load(context.Background())
	if e != nil || saved.CMSLocalRoot != site {
		t.Fatal("local selection not persisted", e)
	}
	raw := fmt.Sprint(report)
	if strings.Contains(raw, cfg.Passwd) && cfg.Passwd != "" {
		t.Fatal("password leaked")
	}
	t.Run("saved directory alias upgrades to local despite stale metadata", func(t *testing.T) {
		aliasRoot := t.TempDir()
		aliasSite := fakeCMSSite(t, aliasRoot, "cms.example.test", "https://stale.example.invalid")
		if err := os.WriteFile(filepath.Join(aliasSite, "application", "database.php"), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		aliasBase := "https://cms.example.test/api.php/provide/vod/"
		ah := newHarness(t, "")
		ah.a.config.CMSRoots = []string{aliasRoot}
		if err := ah.a.change(context.Background(), "test", func(next *state) error { next.CMSBase = aliasBase; return nil }); err != nil {
			t.Fatal(err)
		}
		found := ah.a.discoverAndConnect(context.Background())
		if found["multiple"] != false || found["connectedBase"] != aliasBase || obj(found["currentReport"])["transport"] != "local" || ah.a.readState().CMSLocalRoot != aliasSite {
			t.Fatal("alias upgrade failed", found)
		}
		candidates := found["candidates"].([]M)
		if len(candidates) != 1 {
			t.Fatal("same site duplicated", found)
		}
		chosen, selectedRoot, err := ah.a.selectedCMS(context.Background(), M{"base": aliasBase, "siteId": candidates[0]["id"]})
		if err != nil || chosen["base"] != aliasBase || chosen["transport"] != "local" || selectedRoot != aliasSite {
			t.Fatal("selection reverted to stale domain", chosen, err)
		}
	})

	if requests.Load() != 0 {
		t.Fatal("local reads depended on public API")
	}
}
