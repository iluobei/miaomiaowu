package handler

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

func certPEMSha256(certPEM string) (string, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return "", errors.New("certificate PEM not found")
	}
	if _, err := x509.ParseCertificate(block.Bytes); err != nil {
		return "", fmt.Errorf("parse certificate: %w", err)
	}
	sum := sha256.Sum256(block.Bytes)
	return hex.EncodeToString(sum[:]), nil
}

func fetchPeerCertSha256(ctx context.Context, address string, port int, sni, alpn string) (string, error) {
	certs, err := fetchPeerCertChain(ctx, address, port, sni, alpn)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(certs[0].Raw)
	return hex.EncodeToString(sum[:]), nil
}

// fetchPeerCertChain 握手一次,取回对端发来的整条证书链(叶子在前)。不校验证书 —— 调用方自己决定怎么验。
func fetchPeerCertChain(ctx context.Context, address string, port int, sni, alpn string) ([]*x509.Certificate, error) {
	address = strings.TrimSpace(address)
	if address == "" || port <= 0 || port > 65535 {
		return nil, errors.New("invalid TLS target")
	}
	if sni == "" {
		sni = address
	}
	timeout := 10 * time.Second
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < timeout {
		timeout = time.Until(deadline)
	}
	cfg := &tls.Config{InsecureSkipVerify: true, ServerName: sni} // #nosec G402 -- fingerprint retrieval intentionally accepts untrusted certificates
	if alpn = strings.TrimSpace(alpn); alpn != "" {
		for _, value := range strings.Split(alpn, ",") {
			if value = strings.TrimSpace(value); value != "" {
				cfg.NextProtos = append(cfg.NextProtos, value)
			}
		}
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: timeout}, "tcp", net.JoinHostPort(address, strconv.Itoa(port)), cfg)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, errors.New("no peer certificate received")
	}
	return certs, nil
}
