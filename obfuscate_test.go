// Copyright 2020-present Kuei-chun Chen. All rights reserved.
// obfuscate_test.go

package ftdc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/simagix/gox"
)

func TestObfuscator_ObfuscateHostname(t *testing.T) {
	o := NewObfuscator()

	// Test hostname obfuscation is consistent
	original := "mongodb-prod-server1.company.com"
	obfuscated1 := o.Obfuscator.ObfuscateHostname(original)
	obfuscated2 := o.Obfuscator.ObfuscateHostname(original)

	if obfuscated1 != obfuscated2 {
		t.Errorf("Expected consistent obfuscation, got %s and %s", obfuscated1, obfuscated2)
	}

	if obfuscated1 == original {
		t.Errorf("Expected obfuscation to change hostname, got same value")
	}

	// Different hostnames should get different obfuscated values
	different := "mongodb-prod-server2.company.com"
	obfuscatedDiff := o.Obfuscator.ObfuscateHostname(different)
	if obfuscated1 == obfuscatedDiff {
		t.Errorf("Expected different hostnames to get different obfuscation")
	}
}

func TestObfuscator_ObfuscateIP(t *testing.T) {
	o := NewObfuscator()

	// Test IP obfuscation
	original := "192.168.1.100"
	obfuscated := o.Obfuscator.ObfuscateIP(original)

	if obfuscated == original {
		t.Errorf("Expected IP to be obfuscated")
	}

	// Should be consistent
	obfuscated2 := o.Obfuscator.ObfuscateIP(original)
	if obfuscated != obfuscated2 {
		t.Errorf("Expected consistent IP obfuscation")
	}

	// Should be in 10.x.x.x range (IPStylePrivate)
	if obfuscated[:3] != "10." {
		t.Errorf("Expected IP in 10.x.x.x range, got %s", obfuscated)
	}
}

func TestObfuscator_ObfuscateHostPort(t *testing.T) {
	o := NewObfuscator()

	// Test hostname:port
	original := "mongodb-server.company.com:27017"
	obfuscated := o.Obfuscator.ObfuscateHostPort(original)

	if obfuscated == original {
		t.Errorf("Expected host:port to be obfuscated")
	}

	// Port should be preserved
	if len(obfuscated) < 6 || obfuscated[len(obfuscated)-5:] != "27017" {
		t.Errorf("Expected port to be preserved, got %s", obfuscated)
	}

	// Test IP:port
	ipOriginal := "192.168.1.100:27017"
	ipObfuscated := o.Obfuscator.ObfuscateHostPort(ipOriginal)

	if ipObfuscated == ipOriginal {
		t.Errorf("Expected IP:port to be obfuscated")
	}
}

func TestObfuscator_LooksLikeIP(t *testing.T) {
	o := NewObfuscator()

	tests := []struct {
		input    string
		expected bool
	}{
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"255.255.255.255", true},
		{"hostname.com", false},
		{"192.168.1", false},
		{"192.168.1.1.1", false},
	}

	for _, tt := range tests {
		result := o.looksLikeIP(tt.input)
		if result != tt.expected {
			t.Errorf("looksLikeIP(%s) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}

func TestObfuscator_LooksLikeHostPort(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"hostname:27017", true},
		{"mongodb.company.com:27017", true},
		{"192.168.1.1:27017", true},
		{"hostname", false},
		{":27017", false},
		{"hostname:", false},
	}

	for _, tt := range tests {
		result := gox.LooksLikeHostPort(tt.input)
		if result != tt.expected {
			t.Errorf("LooksLikeHostPort(%s) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}

func TestObfuscator_GetMappings(t *testing.T) {
	o := NewObfuscator()

	// Obfuscate some values
	o.Obfuscator.ObfuscateHostname("host1.example.com")
	o.Obfuscator.ObfuscateIP("192.168.1.1")
	o.Obfuscator.ObfuscateReplSet("production-rs")

	mappings := o.GetMappings()

	if len(mappings["hostnames"]) != 1 {
		t.Errorf("Expected 1 hostname mapping, got %d", len(mappings["hostnames"]))
	}

	if len(mappings["ips"]) != 1 {
		t.Errorf("Expected 1 IP mapping, got %d", len(mappings["ips"]))
	}

	if len(mappings["replSets"]) != 1 {
		t.Errorf("Expected 1 replSet mapping, got %d", len(mappings["replSets"]))
	}
}

func TestObfuscator_DeterministicAcrossRuns(t *testing.T) {
	// Create two separate Obfuscator instances (simulating two separate runs)
	o1 := NewObfuscator()
	o2 := NewObfuscator()

	// Test hostnames
	hostname := "prod-mongodb-server1.company.com"
	h1 := o1.Obfuscator.ObfuscateHostname(hostname)
	h2 := o2.Obfuscator.ObfuscateHostname(hostname)
	if h1 != h2 {
		t.Errorf("Hostname obfuscation not deterministic: %s vs %s", h1, h2)
	}
	t.Logf("Hostname: %s -> %s", hostname, h1)

	// Test IPs
	ip := "192.168.1.100"
	ip1 := o1.Obfuscator.ObfuscateIP(ip)
	ip2 := o2.Obfuscator.ObfuscateIP(ip)
	if ip1 != ip2 {
		t.Errorf("IP obfuscation not deterministic: %s vs %s", ip1, ip2)
	}
	t.Logf("IP: %s -> %s", ip, ip1)

	// Test replica sets
	rs := "production-replica-set"
	rs1 := o1.Obfuscator.ObfuscateReplSet(rs)
	rs2 := o2.Obfuscator.ObfuscateReplSet(rs)
	if rs1 != rs2 {
		t.Errorf("ReplSet obfuscation not deterministic: %s vs %s", rs1, rs2)
	}
	t.Logf("ReplSet: %s -> %s", rs, rs1)

	// Test host:port
	hostPort := "mongodb-server.company.com:27017"
	hp1 := o1.Obfuscator.ObfuscateHostPort(hostPort)
	hp2 := o2.Obfuscator.ObfuscateHostPort(hostPort)
	if hp1 != hp2 {
		t.Errorf("HostPort obfuscation not deterministic: %s vs %s", hp1, hp2)
	}
	t.Logf("HostPort: %s -> %s", hostPort, hp1)

	// Test path segments
	pathSeg := "atlas-prod-shard-00-01"
	ps1 := o1.getObfuscatedPathSegment(pathSeg)
	ps2 := o2.getObfuscatedPathSegment(pathSeg)
	if ps1 != ps2 {
		t.Errorf("PathSegment obfuscation not deterministic: %s vs %s", ps1, ps2)
	}
	t.Logf("PathSegment: %s -> %s", pathSeg, ps1)
}

func TestObfuscator_ObfuscateFile(t *testing.T) {
	// Test with actual FTDC file if available
	testFiles := []string{
		"testdata/metrics.2020-05-13T14-20-19Z-00000",
		"diagnostic.data/metrics.2025-12-10T01-55-12Z-00000",
	}

	var testFile string
	for _, f := range testFiles {
		if _, err := os.Stat(f); err == nil {
			testFile = f
			break
		}
	}

	if testFile == "" {
		t.Skip("No test FTDC file available")
	}

	o := NewObfuscator()

	// Create temp output file
	outputFile := filepath.Join(os.TempDir(), "obfuscated-test-metrics")
	defer os.Remove(outputFile)

	err := o.ObfuscateFile(testFile, outputFile)
	if err != nil {
		t.Fatalf("ObfuscateFile failed: %v", err)
	}

	// Verify output file exists and is not empty
	info, err := os.Stat(outputFile)
	if err != nil {
		t.Fatalf("Output file not created: %v", err)
	}

	if info.Size() == 0 {
		t.Errorf("Output file is empty")
	}

	// Print mappings for verification
	mappings := o.GetMappings()
	t.Logf("Obfuscation mappings: %+v", mappings)
}
