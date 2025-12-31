// Copyright 2020-present Kuei-chun Chen. All rights reserved.
// obfuscate.go

package ftdc

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/simagix/gox"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Obfuscator handles PII obfuscation for FTDC files with consistent mappings
// Uses deterministic hashing so the same input always produces the same output
type Obfuscator struct {
	*gox.Obfuscator
}

// NewObfuscator creates a new Obfuscator configured for FTDC obfuscation
func NewObfuscator() *Obfuscator {
	o := &Obfuscator{
		Obfuscator: gox.NewObfuscator(),
	}
	// Configure for infrastructure-style obfuscation
	o.Obfuscator.IPStyle = gox.IPStylePrivate   // 10.x.x.x range
	o.Obfuscator.NameStyle = gox.NameStyleHash  // host-xxx.local, rs-xxx
	return o
}

// ObfuscateFile reads an FTDC file, obfuscates PII, and writes to output file
func (o *Obfuscator) ObfuscateFile(inputPath, outputPath string) error {
	var err error
	var reader *bufio.Reader
	var buffer []byte

	if reader, err = gox.NewFileReader(inputPath); err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}

	if buffer, err = io.ReadAll(reader); err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}

	obfuscated, err := o.obfuscateBuffer(buffer)
	if err != nil {
		return fmt.Errorf("failed to obfuscate: %w", err)
	}

	if err = os.WriteFile(outputPath, obfuscated, 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	return nil
}

// obfuscateBuffer processes the entire FTDC buffer
func (o *Obfuscator) obfuscateBuffer(buffer []byte) ([]byte, error) {
	var result bytes.Buffer
	var pos uint32

	for pos < uint32(len(buffer)) {
		// Read document length
		if pos+4 > uint32(len(buffer)) {
			break
		}
		length := getUint32(buffer[pos : pos+4])
		if pos+length > uint32(len(buffer)) {
			break
		}

		docBytes := buffer[pos : pos+length]
		pos += length

		// Parse the document
		var doc bson.M
		if err := bson.Unmarshal(docBytes, &doc); err != nil {
			return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
		}

		docType, _ := doc["type"].(int32)

		if docType == 0 {
			// Type 0: metadata document - obfuscate the "doc" field
			if innerDoc, ok := doc["doc"]; ok {
				doc["doc"] = o.obfuscateValue(innerDoc)
			}
		} else if docType == 1 {
			// Type 1: metrics data - need to decompress, obfuscate, recompress
			if data, ok := doc["data"].(primitive.Binary); ok {
				obfuscatedData, err := o.obfuscateCompressedData(data.Data)
				if err != nil {
					return nil, err
				}
				doc["data"] = primitive.Binary{Subtype: data.Subtype, Data: obfuscatedData}
			}
		}

		// Re-marshal the document
		newDocBytes, err := bson.Marshal(doc)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal BSON: %w", err)
		}

		result.Write(newDocBytes)
	}

	return result.Bytes(), nil
}

// obfuscateCompressedData handles compressed FTDC data blocks
func (o *Obfuscator) obfuscateCompressedData(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return data, nil
	}

	// First 4 bytes are uncompressed size (skip for decompression)
	compressedData := data[4:]

	// Decompress
	r, err := zlib.NewReader(bytes.NewReader(compressedData))
	if err != nil {
		return nil, fmt.Errorf("failed to create zlib reader: %w", err)
	}
	decompressed, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to decompress: %w", err)
	}

	// The decompressed data starts with a BSON document (the first sample)
	// followed by delta-encoded metrics
	if len(decompressed) < 4 {
		return data, nil
	}

	docSize := getUint32(decompressed[:4])
	if docSize > uint32(len(decompressed)) {
		return data, nil
	}

	// Parse and obfuscate the BSON document
	var doc bson.D
	if err := bson.Unmarshal(decompressed[:docSize], &doc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metrics doc: %w", err)
	}

	obfuscatedDoc := o.obfuscateBsonD(doc)

	newDocBytes, err := bson.Marshal(obfuscatedDoc)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metrics doc: %w", err)
	}

	// Rebuild the decompressed data with obfuscated doc
	var newDecompressed bytes.Buffer
	newDecompressed.Write(newDocBytes)
	newDecompressed.Write(decompressed[docSize:]) // keep the delta data as-is

	// Recompress
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	w.Write(newDecompressed.Bytes())
	w.Close()

	// Rebuild with size prefix
	// Update the uncompressed size
	newSize := uint32(newDecompressed.Len())
	newSizeBytes := []byte{
		byte(newSize),
		byte(newSize >> 8),
		byte(newSize >> 16),
		byte(newSize >> 24),
	}

	var result bytes.Buffer
	result.Write(newSizeBytes)
	result.Write(compressed.Bytes())

	return result.Bytes(), nil
}

// obfuscateBsonD recursively obfuscates a bson.D document
func (o *Obfuscator) obfuscateBsonD(doc bson.D) bson.D {
	result := make(bson.D, len(doc))
	for i, elem := range doc {
		result[i] = bson.E{
			Key:   elem.Key,
			Value: o.obfuscateValueWithKey(elem.Key, elem.Value),
		}
	}
	return result
}

// obfuscateValueWithKey obfuscates based on key name
func (o *Obfuscator) obfuscateValueWithKey(key string, value interface{}) interface{} {
	// Check if this is a PII field
	keyLower := strings.ToLower(key)

	switch v := value.(type) {
	case string:
		if o.isPIIKey(keyLower) {
			return o.obfuscateString(v, keyLower)
		}
		// Also check if value looks like IP/CIDR or path with cluster name
		if o.looksLikeIP(v) {
			return o.Obfuscator.ObfuscateIP(v)
		}
		if o.looksLikePathWithClusterName(v) {
			return o.obfuscatePath(v)
		}
		return v
	case bson.D:
		return o.obfuscateBsonD(v)
	case bson.A:
		result := make(bson.A, len(v))
		for i, item := range v {
			result[i] = o.obfuscateValueWithKey(key, item)
		}
		return result
	case bson.M:
		result := bson.M{}
		for k, val := range v {
			result[k] = o.obfuscateValueWithKey(k, val)
		}
		return result
	default:
		return value
	}
}

// obfuscateValue obfuscates without key context
func (o *Obfuscator) obfuscateValue(value interface{}) interface{} {
	switch v := value.(type) {
	case bson.D:
		return o.obfuscateBsonD(v)
	case bson.A:
		result := make(bson.A, len(v))
		for i, item := range v {
			result[i] = o.obfuscateValue(item)
		}
		return result
	case bson.M:
		result := bson.M{}
		for k, val := range v {
			result[k] = o.obfuscateValueWithKey(k, val)
		}
		return result
	default:
		return value
	}
}

// isPIIKey checks if a key name indicates PII content
func (o *Obfuscator) isPIIKey(key string) bool {
	piiKeys := []string{
		"hostname",
		"host",
		"name",           // for replica set member names (will check value format)
		"setname",        // replica set name
		"set",            // replica set name
		"replsetname",    // replica set name in config
		"me",             // self hostname in replset
		"primary",        // primary hostname
		"syncingto",      // syncing target hostname
		"syncsourcehost", // sync source hostname in replset status
		// Path fields that may contain cluster/shard names
		"dbpath",
		"path",
		"config",
		"keyfile",
		"cafile",
		"pemkeyfile",
		"clusterfile",
		"clustercafile",
	}

	for _, pii := range piiKeys {
		if key == pii {
			return true
		}
	}
	return false
}

// obfuscateString obfuscates a string value based on key hint
func (o *Obfuscator) obfuscateString(value, keyHint string) string {
	if value == "" {
		return value
	}

	// Check if it looks like a hostname:port
	if gox.LooksLikeHostPort(value) {
		return o.Obfuscator.ObfuscateHostPort(value)
	}

	// Check if it looks like an IP address
	if o.looksLikeIP(value) {
		return o.Obfuscator.ObfuscateIP(value)
	}

	// For other PII fields (like hostname, setname), use consistent mapping
	switch keyHint {
	case "hostname", "host", "me", "primary", "syncingto", "syncsourcehost":
		return o.Obfuscator.ObfuscateHostname(value)
	case "setname", "set", "replsetname":
		return o.Obfuscator.ObfuscateReplSet(value)
	case "name":
		// "name" field: only obfuscate if it looks like a hostname (for replset members)
		// Don't obfuscate OS names like "CentOS Linux"
		if gox.LooksLikeHostname(value) {
			return o.Obfuscator.ObfuscateHostname(value)
		}
		return value // Keep as-is (e.g., OS name)
	case "dbpath", "path", "config", "keyfile", "cafile", "pemkeyfile", "clusterfile", "clustercafile":
		// Obfuscate paths that may contain cluster/shard names
		return o.obfuscatePath(value)
	default:
		// Don't obfuscate unknown patterns - safer to preserve
		return value
	}
}

// looksLikeIP checks if string looks like an IP address or CIDR subnet
func (o *Obfuscator) looksLikeIP(s string) bool {
	return gox.ReIPCIDR.MatchString(s)
}

// obfuscatePath obfuscates file paths that may contain cluster/shard names
func (o *Obfuscator) obfuscatePath(path string) string {
	if path == "" {
		return path
	}

	// Split path into segments
	segments := strings.Split(path, "/")
	lastIdx := len(segments) - 1

	for i, seg := range segments {
		// Skip empty segments
		if seg == "" {
			continue
		}

		// Skip the last segment if it looks like a file name (has extension)
		if i == lastIdx && strings.Contains(seg, ".") {
			continue
		}

		// Skip common system directories that don't reveal identity
		segLower := strings.ToLower(seg)
		if segLower == "srv" || segLower == "var" || segLower == "lib" || segLower == "etc" ||
			segLower == "pki" || segLower == "tls" || segLower == "certs" || segLower == "private" ||
			segLower == "mongodb" || segLower == "data" || segLower == "home" ||
			segLower == "diagnostic.data" {
			continue
		}

		// Check if segment looks like a cluster/shard/node identifier
		if o.looksLikeClusterName(seg) {
			segments[i] = o.getObfuscatedPathSegment(seg)
		}
	}

	return strings.Join(segments, "/")
}

// looksLikeClusterName checks if a path segment looks like a cluster/shard identifier
func (o *Obfuscator) looksLikeClusterName(seg string) bool {
	segLower := strings.ToLower(seg)

	// Must contain identifying patterns AND look like a custom name (has hyphens or numbers)
	hasIdentifyingPattern := strings.Contains(segLower, "shard") ||
		strings.Contains(segLower, "-node") ||
		strings.Contains(segLower, "node-") ||
		strings.Contains(segLower, "cluster") ||
		strings.Contains(segLower, "repl") ||
		strings.Contains(segLower, "rs0") ||
		strings.Contains(segLower, "rs1") ||
		strings.Contains(segLower, "rs2")

	// Also catch automation directories that reveal cloud provider
	isAutomationDir := segLower == "mongodb-mms-automation"

	return hasIdentifyingPattern || isAutomationDir
}

// looksLikePathWithClusterName checks if a string looks like a file path containing cluster identifiers
func (o *Obfuscator) looksLikePathWithClusterName(s string) bool {
	// Must start with / to be a path
	if !strings.HasPrefix(s, "/") {
		return false
	}

	// Check if any segment looks like a cluster name
	segments := strings.Split(s, "/")
	for _, seg := range segments {
		if seg != "" && o.looksLikeClusterName(seg) {
			return true
		}
	}
	return false
}

// getObfuscatedPathSegment returns consistent obfuscated path segment
func (o *Obfuscator) getObfuscatedPathSegment(segment string) string {
	// Reuse hostname map for path segments to maintain consistency
	if obfuscated, exists := o.HostnameMap[segment]; exists {
		return obfuscated
	}

	// Deterministic: same input always produces same output
	obfuscated := fmt.Sprintf("server-%s", gox.HashString(segment, 8))
	o.HostnameMap[segment] = obfuscated
	return obfuscated
}

// GetMappings returns the obfuscation mappings (for reference/debugging)
func (o *Obfuscator) GetMappings() map[string]map[string]string {
	return map[string]map[string]string{
		"hostnames": o.HostnameMap,
		"ips":       o.IPMap,
		"replSets":  o.ReplSetMap,
	}
}

// helper function
func getUint32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}
