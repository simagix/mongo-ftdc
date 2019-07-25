// Copyright 2019 Kuei-chun Chen. All rights reserved.

package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"

	ana "github.com/simagix/mongo-ftdc/analytics"
)

func main() {
	flag.Parse()
	filenames := append([]string{}, flag.Args()...)
	metrics := ana.NewMetrics(filenames)
	http.HandleFunc("/grafana", cors(metrics.Handler))
	http.HandleFunc("/grafana/", cors(metrics.Handler))
	hostname, _ := os.Hostname()
	log.Println("HTTP server ready, URL: http://" + hostname + ":5408/")
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(5408), nil))
}

func cors(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Headers", "accept, content-type")
		w.Header().Set("Access-Control-Allow-Methods", "POST")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		f(w, r)
	}
}
