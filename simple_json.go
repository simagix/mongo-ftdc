// Copyright 2019 Kuei-chun Chen. All rights reserved.

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
	"github.com/simagix/mongo-ftdc/analytics"
	"go.mongodb.org/mongo-driver/bson"
)

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
		fmt.Println("mongo-ftdc", version)
		os.Exit(0)
	}
	addr := fmt.Sprintf(":%d", *port)
	if listener, err := net.Listen("tcp", addr); err != nil {
		log.Fatal(err)
	} else {
		listener.Close()
	}
	metrics := analytics.NewMetrics()
	metrics.SetLatest(*latest)
	metrics.SetVerbose(*verbose)
	if err := metrics.ProcessFiles(flag.Args()); err != nil {
		log.Fatal(err)
	}
	http.HandleFunc("/", gox.Cors(handler))
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(bson.M{"ok": 1, "message": "hello mongo-ftdc!"})
}
