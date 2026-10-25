package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"math"
	"os"
	"path/filepath"
	"time"
)

type M = map[string]any

type record struct {
	Version          int        `json:"version"`
	PublishedVersion int        `json:"publishedVersion"`
	Draft            M          `json:"draft"`
	Published        M          `json:"published"`
	PublishedAt      string     `json:"publishedAt"`
	History          []snapshot `json:"history"`
}
type snapshot struct {
	Version     int    `json:"version"`
	PublishedAt string `json:"publishedAt"`
	Config      M      `json:"config"`
}
type credential struct {
	Username  string `json:"username"`
	Salt      string `json:"salt"`
	Hash      string `json:"hash"`
	Algorithm string `json:"algorithm"`
}
type state struct {
	PlaybackEngines       *engineSettings       `json:"playbackEngines,omitempty"`
	Playback              *playbackSettings     `json:"playbackResolvers,omitempty"`
	Advertising           map[string]*adAccount `json:"advertising,omitempty"`
	AppReleases           []appRelease          `json:"appReleases,omitempty"`
	ManagedReleaseTargets map[string]bool       `json:"managedReleaseTargets,omitempty"`
	ServiceRouting        M                     `json:"serviceRouting,omitempty"`
	CMSLocalRoot          string                `json:"cmsLocalRoot,omitempty"`
	CMSBase               string                `json:"cmsBase"`
	UpdatedAt             string                `json:"updatedAt"`
	Setup                 M                     `json:"setup"`
	Operations            map[string]*record    `json:"operations"`
	Credential            credential            `json:"admin"`
	Revision              int64                 `json:"revision"`
}

// All state changes use a durable store and compare-and-swap revision. One service process per instance.
type store interface {
	Load(context.Context) (state, error)
	Save(context.Context, state, int64) error
	Close() error
}
type fileStore struct{ file string }

func (s *fileStore) Load(_ context.Context) (state, error) {
	var v state
	b, e := os.ReadFile(s.file)
	if errors.Is(e, os.ErrNotExist) {
		return v, sql.ErrNoRows
	}
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(b, &v)
	return v, e
}
func atomicFile(name string, b []byte) error {
	if e := os.MkdirAll(filepath.Dir(name), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(name), ".hetun-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if e = os.Rename(tmp, name); e != nil {
		return e
	}
	return nil
}
func (s *fileStore) Save(ctx context.Context, v state, old int64) error {
	previous, e := s.Load(ctx)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	if e == nil && previous.Revision != old {
		return problem(409, "数据已改变，请重载服务")
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return atomicFile(s.file, b)
}
func (s *fileStore) Close() error { return nil }

type mysqlStore struct{ db *sql.DB }

func openMySQL(dsn string, options ...*databaseSettings) (*mysqlStore, error) {
	var option *databaseSettings
	if len(options) > 0 {
		option = options[0]
	}
	settings := effectiveDatabase(option)
	if err := settings.validate(); err != nil {
		return nil, err
	}
	prepared, e := databaseDSN(dsn, settings)
	if e != nil {
		return nil, e
	}
	db, e := sql.Open("mysql", prepared)
	if e != nil {
		return nil, e
	}
	configureDatabase(db, settings)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if e = db.PingContext(ctx); e != nil {
		db.Close()
		return nil, fmt.Errorf("MySQL 连接失败: %w", e)
	}
	_, e = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS hetun_operations_state (id TINYINT PRIMARY KEY, revision BIGINT NOT NULL, payload LONGTEXT NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if e != nil {
		db.Close()
		return nil, e
	}
	return &mysqlStore{db}, nil
}
func (s *mysqlStore) Load(ctx context.Context) (state, error) {
	var v state
	var b []byte
	e := s.db.QueryRowContext(ctx, "SELECT payload FROM hetun_operations_state WHERE id=1").Scan(&b)
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(b, &v)
	return v, e
}
func (s *mysqlStore) Save(ctx context.Context, v state, old int64) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	var result sql.Result
	if old == 0 {
		result, e = s.db.ExecContext(ctx, "INSERT INTO hetun_operations_state (id, revision, payload) VALUES (1,?,?)", v.Revision, b)
	} else {
		result, e = s.db.ExecContext(ctx, "UPDATE hetun_operations_state SET revision=?,payload=? WHERE id=1 AND revision=?", v.Revision, b, old)
	}
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return problem(409, "数据库版本冲突，请重载服务")
	}
	return nil
}
func (s *mysqlStore) Close() error { return s.db.Close() }
func clone[T any](v T) T {
	b, e := json.Marshal(v)
	if e != nil {
		panic(e)
	}
	var x T
	if e = json.Unmarshal(b, &x); e != nil {
		panic(e)
	}
	return x
}
func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func obj(v any) M {
	m, _ := v.(map[string]any)
	if m == nil {
		return M{}
	}
	return m
}
func arr(v any) []any {
	x, _ := v.([]any)
	if x == nil {
		return []any{}
	}
	return x
}
func str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}
func number(v any) int {
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) || x != math.Trunc(x) || x > 2147483647 || x < -2147483648 {
			return 0
		}
		return int(x)
	case int:
		return x
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	}
	return 0
}
func pick(m M, keys ...string) M {
	v := M{}
	for _, k := range keys {
		if x, ok := m[k]; ok {
			v[k] = clone(x)
		}
	}
	return v
}
