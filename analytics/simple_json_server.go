// Copyright 2019 Kuei-chun Chen. All rights reserved.

package analytics

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/simagix/gox"
)

// SingleJSONServer spins up a Single JSON web server for Grafana
func SingleJSONServer(filenames []string) {
	metrics := NewMetrics(filenames)
	metrics.Read()
	http.HandleFunc("/grafana", gox.Cors(metrics.Handler))
	http.HandleFunc("/grafana/", gox.Cors(metrics.Handler))
	hostname, _ := os.Hostname()
	log.Println("HTTP server ready, URL: http://" + hostname + ":5408/")
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(5408), nil))
}
