// Copyright 2019 Kuei-chun Chen. All rights reserved.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/simagix/gox"
	"github.com/simagix/mongo-ftdc/analytics"
	"go.mongodb.org/mongo-driver/bson"
)

func main() {
	latest := flag.Int("latest", 10, "latest n files")
	port := flag.Int("port", 5408, "port number")
	verbose := flag.Bool("v", false, "verbose")
	flag.Parse()
	flagset := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { flagset[f.Name] = true })
	metrics := analytics.NewMetrics()
	metrics.SetLatest(*latest)
	metrics.SetVerbose(*verbose)
	metrics.ProcessFiles(flag.Args())
	addr := fmt.Sprintf(":%d", *port)
	http.HandleFunc("/", gox.Cors(handler))
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(bson.M{"ok": 1, "message": "hello ftdc!"})
}
