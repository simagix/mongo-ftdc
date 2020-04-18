// Copyright 2019 Kuei-chun Chen. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/simagix/gox"
	"github.com/simagix/mongo-ftdc/analytics"
	"go.mongodb.org/mongo-driver/bson"
)

func main() {
	var err error
	var hostname string
	var port = 5408
	metrics := analytics.NewMetrics()
	metrics.SetVerbose(true)
	metrics.ProcessFiles(os.Args)
	if hostname, err = os.Hostname(); err != nil {
		hostname = "127.0.0.1"
	}
	log.Printf("HTTP server ready, URL: http://%s:%d/\n", hostname, port)
	addr := fmt.Sprintf(":%d", port)
	http.HandleFunc("/", gox.Cors(handler))
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handler(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(bson.M{"ok": 1, "message": "hello ftdc!"})
}
