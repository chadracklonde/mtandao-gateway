package cloud

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAndReadSecretFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	path := filepath.Join(dir, "gateway.token")
	if err := WriteSecretFile(path, testToken); err != nil { t.Fatal(err) }
	got, err := ReadSecretFile(path)
	if err != nil { t.Fatal(err) }
	if got != testToken { t.Fatal("stored token does not match") }
	info, err := os.Stat(path)
	if err != nil { t.Fatal(err) }
	if info.Mode().Perm() != 0600 { t.Fatalf("expected mode 0600, got %o", info.Mode().Perm()) }
	dirInfo, err := os.Stat(dir)
	if err != nil { t.Fatal(err) }
	if dirInfo.Mode().Perm() != 0700 { t.Fatalf("expected directory mode 0700, got %o", dirInfo.Mode().Perm()) }
}

func TestWriteSecretFileDoesNotOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gateway.token")
	if err := WriteSecretFile(path, testToken); err != nil { t.Fatal(err) }
	if err := WriteSecretFile(path, strings.Repeat("a", 64)); err == nil { t.Fatal("expected existing token to be protected") }
	got, err := ReadSecretFile(path)
	if err != nil { t.Fatal(err) }
	if got != testToken { t.Fatal("existing token was changed") }
}

func TestReadSecretFileRejectsBroadPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unsafe.token")
	if err := os.WriteFile(path, []byte(testToken), 0644); err != nil { t.Fatal(err) }
	if _, err := ReadSecretFile(path); err == nil { t.Fatal("expected broad permissions to be rejected") }
}

func TestReadSecretFileRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte(testToken), 0600); err != nil { t.Fatal(err) }
	if err := os.Symlink(target, link); err != nil { t.Skipf("symlink unavailable: %v", err) }
	if _, err := ReadSecretFile(link); err == nil { t.Fatal("expected symlink to be rejected") }
}
