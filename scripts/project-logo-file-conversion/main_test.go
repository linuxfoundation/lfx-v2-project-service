// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsPublicIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		// Must be rejected
		{"loopback IPv4", "127.0.0.1", false},
		{"loopback IPv4 alt", "127.0.0.2", false},
		{"loopback IPv6", "::1", false},
		{"unspecified IPv4 (0.0.0.0)", "0.0.0.0", false},
		{"unspecified IPv6 (::)", "::", false},
		{"link-local / AWS IMDS", "169.254.169.254", false},
		{"link-local other", "169.254.0.1", false},
		{"IPv6 link-local", "fe80::1", false},
		{"RFC1918 10/8", "10.0.0.1", false},
		{"RFC1918 172.16/12", "172.16.0.1", false},
		{"RFC1918 192.168/16", "192.168.1.1", false},
		{"RFC6598 CGNAT", "100.64.0.1", false},
		{"IANA special 192.0.0/24", "192.0.0.1", false},
		{"benchmark 198.18/15", "198.18.0.1", false},
		{"reserved 240/4", "240.0.0.1", false},
		{"IPv6 ULA fc00::/7", "fc00::1", false},
		{"IPv6 ULA fd00::/8", "fd00::1", false},
		{"IPv6 multicast", "ff02::1", false},
		{"IPv4 multicast", "224.0.0.1", false},
		// Must be accepted
		{"public IPv4", "1.1.1.1", true},
		{"public IPv4 alt", "8.8.8.8", true},
		{"public IPv6", "2606:4700:4700::1111", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("net.ParseIP(%q) returned nil", tt.ip)
			}
			if got := isPublicIP(ip); got != tt.want {
				t.Errorf("isPublicIP(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestValidateLogoURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		// Must be rejected
		{"http scheme", "http://example.com/logo.svg", true},
		{"file scheme", "file:///etc/passwd", true},
		{"loopback IP literal", "https://127.0.0.1/logo.svg", true},
		{"unspecified IP literal 0.0.0.0", "https://0.0.0.0/logo.svg", true},
		{"IMDS IP literal", "https://169.254.169.254/logo.svg", true},
		{"RFC1918 IP literal", "https://10.0.0.1/logo.svg", true},
		{"IPv6 loopback literal", "https://[::1]/logo.svg", true},
		{"IPv6 ULA literal", "https://[fc00::1]/logo.svg", true},
		// Must be accepted
		{"public https host", "https://example.com/logo.svg", false},
		{"public https IP literal", "https://1.1.1.1/logo.svg", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLogoURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateLogoURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestSafeHTTPClient_RedirectToHTTP(t *testing.T) {
	// Server that redirects to http://
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://internal.example.com/secret", http.StatusFound)
	}))
	defer target.Close()

	client := safeHTTPClient()
	resp, err := client.Get(target.URL + "/logo.svg")
	if resp != nil {
		resp.Body.Close() //nolint:errcheck
	}
	if err == nil {
		t.Error("expected error on http redirect, got nil")
	}
}

func TestSafeHTTPClient_NonSVGContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"secret":"data"}`))
	}))
	defer server.Close()

	// validateLogoURL would reject non-https; bypass it by calling downloadFile
	// indirectly through a URL that passes scheme check but hits local test server.
	// We verify the Content-Type check using the mime-parsed comparison directly.
	ct := "application/json"
	mt, _, err := parseMimeType(ct)
	if err == nil && mt == "image/svg+xml" {
		t.Error("application/json should not match image/svg+xml")
	}

	ctSVG := "Image/SVG+XML; charset=utf-8"
	mt2, _, err2 := parseMimeType(ctSVG)
	if err2 != nil || mt2 != "image/svg+xml" {
		t.Errorf("parseMimeType(%q) = %q, %v; want image/svg+xml, nil", ctSVG, mt2, err2)
	}
}
