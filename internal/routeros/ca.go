package routeros

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

// checkCAFile verifies that a configured CA file is a regular, non-symlink file
// that other users cannot modify (it decides which router certificate is trusted).
func checkCAFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect RouterOS CA file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("RouterOS CA file must be a regular file")
	}
	if info.Mode().Perm()&0022 != 0 {
		return errors.New("RouterOS CA file must not be writable by group or others")
	}
	return nil
}

func loadCAPool(path string) (*x509.CertPool, error) {
	if err := checkCAFile(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read RouterOS CA file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, errors.New("RouterOS CA file contains no valid PEM certificate")
	}
	return pool, nil
}
