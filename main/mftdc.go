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

	// Server mode (default)
	addr := fmt.Sprintf(":%d", *port)
	if listener, err := net.Listen("tcp", addr); err != nil {
		log.Fatal(err)
	} else {
		listener.Close()
	}
	metrics := ftdc.NewMetrics()
	metrics.SetLatest(*latest)
	metrics.SetVerbose(*verbose)
	if err := metrics.ProcessFiles(flag.Args()); err != nil {
		log.Fatal(err)
	}
	http.HandleFunc("/", gox.Cors(handler))

	// Print endpoints right before server starts (so it's visible at the bottom)
	log.Printf("FTDC API server starting on port %d\n", *port)
	log.Println("=== FTDC Ready ===")
	for _, endpoint := range metrics.GetEndPoints() {
		log.Printf("Grafana: http://localhost:3030%s\n", endpoint)
	}

	log.Fatal(http.ListenAndServe(addr, nil))
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
