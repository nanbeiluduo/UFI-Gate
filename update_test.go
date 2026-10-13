package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionDowngradeAndRepair(t *testing.T) {
	for _, v := range []struct {
		target, local string
		repair, ok    bool
	}{{"1.0.10", "1.0.9", false, true}, {"1.0.9", "1.0.10", true, false}, {"1.0.10", "1.0.10", false, false}, {"1.0.10", "1.0.10", true, true}, {"1.0.10", "", false, true}, {"1.01.0", "", false, false}, {"1.0.10-beta", "", false, false}} {
		e := allowInstall(v.target, VersionInfo{Version: v.local}, v.repair)
		if (e == nil) != v.ok {
			t.Fatalf("%+v: %v", v, e)
		}
	}
}
func signedFixture(t *testing.T) ([]byte, []byte, string) {
	t.Helper()
	pub, priv, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	m := UpdateManifest{Version: "1.0.11", Protocol: 1, MinVersion: "1.0.9"}
	for _, arch := range []string{"arm32", "arm64"} {
		m.Assets = append(m.Assets, UpdateAsset{Arch: arch, Name: "UFI-Gate-1.0.11-" + arch + "-update.tar.gz", SHA256: strings.Repeat("ab", 32), Size: 123})
	}
	raw, _ := json.Marshal(m)
	return raw, []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, raw))), base64.StdEncoding.EncodeToString(pub)
}
func TestManifestTrust(t *testing.T) {
	raw, sig, key := signedFixture(t)
	if _, e := verifyManifest(raw, sig, key); e != nil {
		t.Fatal(e)
	}
	for _, v := range []struct {
		raw, sig []byte
		key      string
	}{{append(append([]byte{}, raw...), byte(' ')), sig, key}, {raw, sig, ""}, {raw, []byte("invalid"), key}} {
		if _, e := verifyManifest(v.raw, v.sig, v.key); e == nil {
			t.Fatal("untrusted manifest accepted")
		}
	}
	for _, u := range []string{"http://github.com/a", "https://github.com.evil/a", "https://user@github.com/a", "https://github.com:444/a"} {
		if officialURL(u) {
			t.Fatal(u)
		}
	}
}
func updateTar(t *testing.T, names []string, link bool) []byte {
	t.Helper()
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	w := tar.NewWriter(z)
	for _, name := range names {
		h := &tar.Header{Name: name, Mode: 0700, Size: 1, Typeflag: tar.TypeReg}
		if link {
			h.Typeflag = tar.TypeSymlink
			h.Size = 0
			h.Linkname = "/etc/passwd"
		}
		if e := w.WriteHeader(h); e != nil {
			t.Fatal(e)
		}
		if !link {
			_, _ = w.Write([]byte("x"))
		}
	}
	_ = w.Close()
	_ = z.Close()
	return b.Bytes()
}
func TestUpdateArchiveBoundaries(t *testing.T) {
	for _, tc := range []struct {
		names    []string
		link, ok bool
	}{{serviceFiles, false, true}, {[]string{"../state.json"}, false, false}, {[]string{"ufi-gate", "ufi-gate"}, false, false}, {serviceFiles, true, false}, {[]string{"ufi-gate"}, false, false}, {append(append([]string{}, serviceFiles...), "state.json"), false, false}} {
		if e := extractUpdate(updateTar(t, tc.names, tc.link), t.TempDir()); (e == nil) != tc.ok {
			t.Fatalf("%+v: %v", tc, e)
		}
	}
}
func TestInstallTransaction(t *testing.T) {
	for _, failHealth := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "rollback"}[failHealth], func(t *testing.T) {
			oldDir, oldDemo := dir, demo
			dir = t.TempDir()
			demo = false
			oldCommand, oldHealth, oldBinary, oldSpace := updateServiceCommand, updateWaitHealth, updateBinaryVersion, updateCheckSpace
			t.Cleanup(func() {
				dir, demo = oldDir, oldDemo
				updateServiceCommand, updateWaitHealth, updateBinaryVersion, updateCheckSpace = oldCommand, oldHealth, oldBinary, oldSpace
			})
			stage := filepath.Join(dir, "update-stage-test")
			_ = os.Mkdir(stage, 0700)
			for _, n := range serviceFiles {
				_ = os.WriteFile(filepath.Join(dir, n), []byte("old:"+n), 0700)
				_ = os.WriteFile(filepath.Join(stage, n), []byte("new:"+n), 0700)
			}
			original := []byte(`{"Config":{"Enabled":false},"traffic":123,"admin":"keep"}`)
			_ = os.WriteFile(filepath.Join(dir, "state.json"), original, 0600)
			_ = os.WriteFile(filepath.Join(dir, "custom-logo.png"), []byte("logo"), 0600)
			updateBinaryVersion = func(path string) (VersionInfo, error) {
				b, e := os.ReadFile(path)
				v := "1.0.9"
				if strings.HasPrefix(string(b), "new:") {
					v = "1.0.10"
				}
				return VersionInfo{Version: v, Arch: buildArch(), Protocol: 1}, e
			}
			updateCheckSpace = func(string) error { return nil }
			var actions []string
			updateServiceCommand = func(script, action string) error { actions = append(actions, action); return nil }
			updateWaitHealth = func(v string) error {
				if failHealth && v == "1.0.10" {
					_ = os.WriteFile(filepath.Join(dir, "state.json"), []byte("candidate migration"), 0600)
					return errors.New("test startup failure")
				}
				return nil
			}
			e := runInstall(stage, "1.0.10", "test", false, false)
			if (e != nil) != failHealth {
				t.Fatal(e)
			}
			want := "new:"
			phase := "success"
			if failHealth {
				want = "old:"
				phase = "rolled_back"
			}
			for _, n := range serviceFiles {
				b, _ := os.ReadFile(filepath.Join(dir, n))
				if string(b) != want+n {
					t.Fatalf("wrong %s: %q", n, b)
				}
			}
			b, _ := os.ReadFile(filepath.Join(dir, "state.json"))
			if !bytes.Equal(b, original) {
				t.Fatalf("state lost: %s", b)
			}
			b, _ = os.ReadFile(filepath.Join(dir, "custom-logo.png"))
			if string(b) != "logo" {
				t.Fatal("custom logo changed")
			}
			if s := readUpdateStatus(); s.Phase != phase {
				t.Fatalf("status %+v", s)
			}
			if _, e := os.Stat(filepath.Join(dir, "update-journal.json")); !os.IsNotExist(e) {
				t.Fatal("journal not cleared")
			}
			if len(actions) < 2 || actions[0] != "stop-update" {
				t.Fatal(actions)
			}
		})
	}
}
