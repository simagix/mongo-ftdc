// Copyright 2019-present Kuei-chun Chen. All rights reserved.
// ftdc_json.go

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

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
	flag.Parse()
	flagset := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { flagset[f.Name] = true })
	if *ver {
		fullVersion := fmt.Sprintf(`%v %v`, repo, version)
		fmt.Println(fullVersion)
		os.Exit(0)
	}
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
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": 1, "message": "hello mongo-ftdc!"})
}
