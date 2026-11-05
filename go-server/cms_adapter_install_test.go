package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

func previousPlaybackFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/Hetun-v0.3.21.php")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != previousPlaybackAdapterSHA256 {
		t.Fatal("previous release fixture digest changed")
	}
	return b
}

func TestPlaybackAdapterUpgradeAndRollback(t *testing.T) {
	for _, release := range []string{"0.3.21", "0.3.22"} {
		for _, failPersistence := range []bool{false, true} {
			t.Run(release+"/"+strconv.FormatBool(failPersistence), func(t *testing.T) {
				h, root := managementLocalFixture(t)
				request := M{"source": managementBase(h.a.readState().CMSBase)}
				if w := h.req("POST", "/api/admin/manage/install", request, ""); w.Code != 200 {
					t.Fatal(w.Code)
				}
				config := filepath.Join(root, "application", "extra", "hetun_operations.php")
				keyBefore, err := os.ReadFile(config)
				if err != nil {
					t.Fatal(err)
				}
				playback := filepath.Join(root, "application", "index", "controller", "Hetun.php")
				previous := previousPlaybackFixture(t)
				if release == "0.3.22" {
					previous, err = os.ReadFile("testdata/Hetun-v0.3.22.php")
					if err != nil {
						t.Fatal(err)
					}
					sum := sha256.Sum256(previous)
					if hex.EncodeToString(sum[:]) != previousPlaybackAdapterV0322SHA256 {
						t.Fatal("previous release fixture digest changed")
					}
				}
				if err := os.WriteFile(playback, previous, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(playback, 0640); err != nil {
					t.Fatal(err)
				}
				if failPersistence {
					h.a.store = &applyFailStore{h.a.store}
				}
				w := h.req("POST", "/api/admin/manage/install", request, "")
				want := playbackAdapter
				if failPersistence {
					if w.Code == 200 {
						t.Fatal("upgrade ignored persistence failure")
					}
					want = previous
				} else if w.Code != 200 {
					t.Fatalf("upgrade failed: %d %s", w.Code, w.Body.String())
				}
				actual, err := os.ReadFile(playback)
				if err != nil || !bytes.Equal(actual, want) {
					t.Fatal("upgrade or rollback bytes incorrect")
				}
				keyAfter, err := os.ReadFile(config)
				if err != nil || !bytes.Equal(keyBefore, keyAfter) {
					t.Fatal("upgrade changed the existing management key")
				}
				info, err := os.Stat(playback)
				if err != nil {
					t.Fatal(err)
				}
				if runtime.GOOS != "windows" && info.Mode().Perm() != 0640 {
					t.Fatal("replacement changed CMS file permissions")
				}
				temp, _ := filepath.Glob(filepath.Join(filepath.Dir(playback), ".hetun-adapter-*"))
				if len(temp) != 0 {
					t.Fatal("staged files left behind")
				}
				if !failPersistence {
					if w := h.req("POST", "/api/admin/manage/install", request, ""); w.Code != 200 {
						t.Fatal("upgrade was not idempotent")
					}
				}
			})
		}
	}

}

func TestPlaybackAdapterUpgradePreflightConflict(t *testing.T) {
	h, root := managementLocalFixture(t)
	playback := filepath.Join(root, "application", "index", "controller", "Hetun.php")
	previous := previousPlaybackFixture(t)
	if err := os.WriteFile(playback, previous, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "application", "extra", "hetun_operations.php"), []byte("custom-key-file"), 0600); err != nil {
		t.Fatal(err)
	}
	w := h.req("POST", "/api/admin/manage/install", M{"source": managementBase(h.a.readState().CMSBase)}, "")
	if w.Code != 409 {
		t.Fatal("preflight accepted custom key")
	}
	actual, _ := os.ReadFile(playback)
	if !bytes.Equal(actual, previous) {
		t.Fatal("playback was upgraded before preflight completed")
	}
	if _, err := os.Stat(filepath.Join(root, "application", "index", "controller", "Hetunops.php")); !os.IsNotExist(err) {
		t.Fatal("preflight created a controller")
	}
}

func TestReplaceManagedAdapterRejectsChangedFile(t *testing.T) {
	for _, replaceIdentity := range []bool{false, true} {
		t.Run(strconv.FormatBool(replaceIdentity), func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "Hetun.php")
			original := []byte("original")
			if err := os.WriteFile(file, original, 0600); err != nil {
				t.Fatal(err)
			}
			opened, err := os.Open(file)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := opened.Stat()
			_ = opened.Close()
			if err != nil {
				t.Fatal(err)
			}
			changed := []byte("custom-change")
			if replaceIdentity {
				if err := os.Rename(file, file+".old"); err != nil {
					t.Fatal(err)
				}
				changed = original
			}
			if err := os.WriteFile(file, changed, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := replaceManagedAdapter(file, expected, original, []byte("replacement")); err == nil {
				t.Fatal("concurrent change was overwritten")
			}
			actual, _ := os.ReadFile(file)
			if !bytes.Equal(actual, changed) {
				t.Fatal("custom content changed")
			}
		})
	}
}
