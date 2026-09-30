// Copyright The Linux Foundation and each contributor to LFX.
// SPDX-License-Identifier: MIT

package main

import (
	"net"
	"net/http"
	"net/url"
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
		{"this-network 0.0.0.0/8", "0.1.2.3", false},
		{"link-local / AWS IMDS", "169.254.169.254", false},
		{"link-local other", "169.254.0.1", false},
		{"IPv6 link-local", "fe80::1", false},
		{"RFC1918 10/8", "10.0.0.1", false},
		{"RFC1918 172.16/12", "172.16.0.1", false},
		{"RFC1918 192.168/16", "192.168.1.1", false},
		{"RFC6598 CGNAT", "100.64.0.1", false},
		{"IANA special 192.0.0/24", "192.0.0.1", false},
		{"TEST-NET-1 192.0.2/24", "192.0.2.1", false},
		{"benchmark 198.18/15", "198.18.0.1", false},
		{"TEST-NET-2 198.51.100/24", "198.51.100.1", false},
		{"TEST-NET-3 203.0.113/24", "203.0.113.1", false},
		{"reserved 240/4", "240.0.0.1", false},
		{"IPv6 ULA fc00::/7", "fc00::1", false},
		{"IPv6 ULA fd00::/8", "fd00::1", false},
		{"IPv6 documentation 2001:db8::/32", "2001:db8::1", false},
		{"IPv6 multicast", "ff02::1", false},
		{"IPv4 multicast", "224.0.0.1", false},
		{"NAT64 well-known 64:ff9b::/96", "64:ff9b::1", false},
		{"NAT64 local 64:ff9b:1::/48", "64:ff9b:1::1", false},
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
		{"this-network IP literal 0.1.2.3", "https://0.1.2.3/logo.svg", true},
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

func TestSafeHTTPClient_CheckRedirect(t *testing.T) {
	client := safeHTTPClient()

	// Non-https redirect must be rejected.
	httpReq := &http.Request{URL: &url.URL{Scheme: "http", Host: "example.com", Path: "/logo.svg"}}
	if err := client.CheckRedirect(httpReq, nil); err == nil {
		t.Error("CheckRedirect: expected error for http redirect, got nil")
	}

	// https redirect must be allowed.
	httpsReq := &http.Request{URL: &url.URL{Scheme: "https", Host: "example.com", Path: "/logo.svg"}}
	if err := client.CheckRedirect(httpsReq, nil); err != nil {
		t.Errorf("CheckRedirect: unexpected error for https redirect: %v", err)
	}
}

func TestIsSVGContentType(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"image/svg+xml", true},
		{"Image/SVG+XML", true},                // case-insensitive
		{"image/svg+xml; charset=utf-8", true}, // with parameter
		{"IMAGE/SVG+XML; CHARSET=UTF-8", true}, // all-caps with parameter
		{"application/json", false},
		{"text/plain", false},
		{"image/png", false},
		{"", false},
		{"not-a-media-type", false},
	}

	for _, tt := range tests {
		if got := isSVGContentType(tt.ct); got != tt.want {
			t.Errorf("isSVGContentType(%q) = %v, want %v", tt.ct, got, tt.want)
		}
	}
}
