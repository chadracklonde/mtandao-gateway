package routeros

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func selfSignedIPCert(t *testing.T) (tls.Certificate, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "router"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, BasicConstraintsValid: true, IsCA: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pemBytes
}

// startFakeRouter accepts TLS connections and closes them after the handshake.
func startFakeRouter(t *testing.T, cert tls.Certificate) string {
	t.Helper()
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				_ = conn.(*tls.Conn).Handshake()
				_ = conn.Close()
			}()
		}
	}()
	return listener.Addr().String()
}

func TestSelfSignedRouterCertificateIsRejectedUnlessTrusted(t *testing.T) {
	cert, pemBytes := selfSignedIPCert(t)
	addr := startFakeRouter(t, cert)
	base := TLSConfig{Address: addr, Username: "u", Password: "p", Timeout: 3 * time.Second}

	_, err := DialTLS(context.Background(), base)
	if err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("untrusted certificate must fail verification, got %v", err)
	}

	caPath := filepath.Join(t.TempDir(), "router.pem")
	if err := os.WriteFile(caPath, pemBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	trusted := base
	trusted.CAFile = caPath
	_, err = DialTLS(context.Background(), trusted)
	if err == nil {
		t.Fatal("fake router cannot complete the RouterOS login")
	}
	if strings.Contains(err.Error(), "certificate") {
		t.Fatalf("trusted certificate was still rejected: %v", err)
	}

	wrongName := trusted
	wrongName.ServerName = "other.example"
	if _, err = DialTLS(context.Background(), wrongName); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("mismatching server name must fail verification, got %v", err)
	}
}

func TestCAFileValidation(t *testing.T) {
	dir := t.TempDir()
	if _, err := loadCAPool(filepath.Join(dir, "missing.pem")); err == nil {
		t.Fatal("missing CA file accepted")
	}
	bad := filepath.Join(dir, "bad.pem")
	_ = os.WriteFile(bad, []byte("not a certificate"), 0o600)
	if _, err := loadCAPool(bad); err == nil {
		t.Fatal("garbage CA file accepted")
	}
	_, pemBytes := selfSignedIPCert(t)
	writable := filepath.Join(dir, "writable.pem")
	_ = os.WriteFile(writable, pemBytes, 0o666)
	_ = os.Chmod(writable, 0o666)
	if _, err := loadCAPool(writable); err == nil {
		t.Fatal("world-writable CA file accepted")
	}
	link := filepath.Join(dir, "link.pem")
	good := filepath.Join(dir, "good.pem")
	_ = os.WriteFile(good, pemBytes, 0o644)
	_ = os.Symlink(good, link)
	if _, err := loadCAPool(link); err == nil {
		t.Fatal("symlinked CA file accepted")
	}
	if _, err := loadCAPool(good); err != nil {
		t.Fatalf("valid CA file rejected: %v", err)
	}
}
