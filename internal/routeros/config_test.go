package routeros

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadConfigPrivateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "router.json")
	data := `{"address":"192.168.88.1:8729","username":"gateway","password":"secret","timeout_seconds":10}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Address != "192.168.88.1:8729" || got.TimeoutSeconds != 10 {
		t.Fatalf("unexpected config: %+v", got)
	}
}

func TestReadConfigRejectsWeakPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "router.json")
	if err := os.WriteFile(path, []byte(`{"address":"192.168.88.1:8729","username":"u","password":"p"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadConfig(path); err == nil {
		t.Fatal("expected permission error")
	}
}

func TestReadConfigRejectsPublicAndDNSTargets(t *testing.T) {
	for _, address := range []string{"8.8.8.8:8729", "router.local:8729"} {
		path := filepath.Join(t.TempDir(), "router.json")
		data := []byte(`{"address":"` + address + `","username":"u","password":"p"}`)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadConfig(path); err == nil {
			t.Fatalf("accepted %s", address)
		}
	}
}

func TestReadConfigRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	if err := os.WriteFile(target, []byte(`{"address":"192.168.88.1:8729","username":"u","password":"p"}`), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadConfig(link); err == nil {
		t.Fatal("expected symlink rejection")
	}
}
