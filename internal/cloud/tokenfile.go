package cloud

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadSecretFile accepts only a regular, non-symlink file inaccessible to
// group and other users.
func ReadSecretFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("inspect secret file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("secret path must be a regular, non-symlink file")
	}
	if info.Mode().Perm()&0077 != 0 {
		return "", errors.New("secret file permissions are too broad; use chmod 600")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read secret file: %w", err)
	}
	secret := strings.TrimSpace(string(data))
	if !validHexToken(secret) {
		return "", errors.New("secret file must contain 64 lowercase hexadecimal characters")
	}
	return secret, nil
}

// WriteSecretFile creates a new secret file atomically and refuses to replace
// an existing credential. The parent directory is created with mode 0700.
func WriteSecretFile(path, secret string) error {
	if !validHexToken(secret) {
		return errors.New("refusing to store an invalid Gateway token")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve token path: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create token directory: %w", err)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("inspect token directory: %w", err)
	}
	if !dirInfo.IsDir() || dirInfo.Mode().Perm()&0077 != 0 {
		return errors.New("token directory must be a directory accessible only to its owner (mode 0700)")
	}

	tmp, err := os.CreateTemp(dir, ".gateway-token-*")
	if err != nil {
		return fmt.Errorf("create temporary token file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secure temporary token file: %w", err)
	}
	if _, err := tmp.WriteString(secret + "\n"); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write Gateway token: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync Gateway token: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close Gateway token: %w", err)
	}
	// Hard-link creation is atomic and fails if the destination already exists.
	if err := os.Link(tmpName, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("Gateway token file already exists; refusing to overwrite it")
		}
		return fmt.Errorf("install Gateway token file: %w", err)
	}
	return nil
}
