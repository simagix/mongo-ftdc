// Copyright 2019-present Kuei-chun Chen. All rights reserved.
// mftdc.go

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/simagix/gox"
	ftdc "github.com/simagix/mongo-ftdc"
)

var repo = "simagix/mongo-ftdc"
var version = "self-built"

func main() {
	latest := flag.Int("latest", 10, "latest n files")
	port := flag.Int("port", 5408, "port number")
	ver := flag.Bool("version", false, "print version number")
	verbose := flag.Bool("v", false, "verbose")
	obfuscate := flag.Bool("obfuscate", false, "obfuscate PII and save to output directory")
	outputDir := flag.String("output", "obfuscated", "output directory for obfuscated files")
	showMappings := flag.Bool("show-mappings", false, "show obfuscation mappings (with -obfuscate)")
	server := flag.Bool("server", false, "start API server for Grafana (default: diagnosis only)")
	flag.Parse()

	if *ver {
		fullVersion := fmt.Sprintf(`%v %v`, repo, version)
		fmt.Println(fullVersion)
		os.Exit(0)
	}

	// Obfuscate mode
	if *obfuscate {
		if err := runObfuscate(flag.Args(), *outputDir, *showMappings); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}

	metrics := ftdc.NewMetrics()
	metrics.SetLatest(*latest)
	metrics.SetVerbose(*verbose)
	if err := metrics.ProcessFiles(flag.Args()); err != nil {
		log.Fatal(err)
	}

	// Run diagnosis (always)
	runDiagnosis(metrics, flag.Args())

	// Default: diagnosis only, exit
	if !*server {
		os.Exit(0)
	}

	// Server mode (with -server flag)
	addr := fmt.Sprintf(":%d", *port)
	http.HandleFunc("/", gox.Cors(handler))

	// Create listener first, then print ready message
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	// Auto-detect Grafana port: Docker (hostname=ftdc) uses 3030, native uses 3000
	grafanaPort := 3000
	if hostname, _ := os.Hostname(); hostname == "ftdc" {
		grafanaPort = 3030
	}
	log.Printf("FTDC API server starting on port %d\n", *port)
	log.Println("=== FTDC Ready ===")
	for _, endpoint := range metrics.GetEndPoints() {
		log.Printf("Grafana: http://localhost:%d%s\n", grafanaPort, endpoint)
	}

	log.Fatal(http.Serve(listener, nil))
}

func handler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": 1, "message": "hello mongo-ftdc!"})
}

func runObfuscate(args []string, outputDir string, showMappings bool) error {
	if len(args) == 0 {
		fmt.Println("Usage: mftdc -obfuscate [-output <dir>] <file_or_directory>...")
		fmt.Println()
		fmt.Println("Obfuscates PII (hostnames, IPs, replica set names) in FTDC files.")
		fmt.Println("Output files are saved to 'obfuscated/' directory by default.")
		return nil
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	obfuscator := ftdc.NewObfuscator()

	for _, inputPath := range args {
		info, err := os.Stat(inputPath)
		if err != nil {
			log.Printf("Warning: cannot access %s: %v", inputPath, err)
			continue
		}

		if info.IsDir() {
			if err := processDirectory(obfuscator, inputPath, outputDir); err != nil {
				log.Printf("Warning: failed to process directory %s: %v", inputPath, err)
			}
		} else {
			outputPath := filepath.Join(outputDir, filepath.Base(inputPath))
			if err := obfuscator.ObfuscateFile(inputPath, outputPath); err != nil {
				log.Printf("Warning: failed to obfuscate %s: %v", inputPath, err)
				continue
			}
			fmt.Printf("Obfuscated: %s -> %s\n", inputPath, outputPath)
		}
	}

	if showMappings {
		mappings := obfuscator.GetMappings()
		jsonMappings, _ := json.MarshalIndent(mappings, "", "  ")
		fmt.Println("\nObfuscation Mappings:")
		fmt.Println(string(jsonMappings))
	}

	fmt.Println("\nDone!")
	return nil
}

func processDirectory(obfuscator *ftdc.Obfuscator, inputDir, outputDir string) error {
	return filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		name := info.Name()
		if !isMetricsFile(name) {
			return nil
		}

		relPath, err := filepath.Rel(inputDir, path)
		if err != nil {
			return err
		}

		outputPath := filepath.Join(outputDir, relPath)
		outputFileDir := filepath.Dir(outputPath)
		if err := os.MkdirAll(outputFileDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", outputFileDir, err)
		}

		if err := obfuscator.ObfuscateFile(path, outputPath); err != nil {
			log.Printf("Warning: failed to obfuscate %s: %v", path, err)
			return nil
		}
		fmt.Printf("Obfuscated: %s -> %s\n", path, outputPath)
		return nil
	})
}

func isMetricsFile(name string) bool {
	if len(name) >= 8 && name[:8] == "metrics." {
		return true
	}
	if len(name) >= 14 && name[:14] == "keyhole_stats." {
		return true
	}
	return false
}

func runDiagnosis(metrics *ftdc.Metrics, args []string) {
	if len(args) == 0 {
		return
	}

	// Get time range and stats
	from, to := metrics.GetTimeRange()
	if from.IsZero() || to.IsZero() {
		return
	}

	stats := metrics.GetFTDCStats()

	// Run diagnosis
	diagnosis := ftdc.NewDiagnosis(stats, from, to)
	diagnosis.Run()

	// Print to stdout
	diagnosis.PrintReport()

	// Generate HTML report
	htmlOutput := determineHTMLPath(args[0])
	if err := diagnosis.GenerateHTML(htmlOutput); err != nil {
		log.Printf("Warning: failed to generate HTML report: %v", err)
		return
	}
	fmt.Printf("📄 HTML report saved to: %s\n", htmlOutput)
}

func determineHTMLPath(inputPath string) string {
	info, err := os.Stat(inputPath)
	if err != nil {
		// Default to current directory if can't stat
		return "ftdc_diagnosis.html"
	}

	if info.IsDir() {
		// Input is a directory - save HTML in that directory
		return filepath.Join(inputPath, "ftdc_diagnosis.html")
	}

	// Input is a file - replace "metrics." with "ftdc-diag." and add .html
	// Example: metrics.2025-12-10T11-40-06Z-00000 -> ftdc-diag.2025-12-10T11-40-06Z.html
	dir := filepath.Dir(inputPath)
	name := filepath.Base(inputPath)

	if len(name) > 8 && name[:8] == "metrics." {
		// Extract timestamp part (remove trailing -00000 suffix if present)
		suffix := name[8:] // e.g., "2025-12-10T11-40-06Z-00000"
		// Remove the trailing -NNNNN part
		if idx := len(suffix) - 6; idx > 0 && suffix[idx] == '-' {
			suffix = suffix[:idx] // e.g., "2025-12-10T11-40-06Z"
		}
		return filepath.Join(dir, "ftdc-diag."+suffix+".html")
	}

	// Fallback for other file types
	return filepath.Join(dir, "ftdc_diagnosis.html")
}
