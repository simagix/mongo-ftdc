// Copyright 2019 Kuei-chun Chen. All rights reserved.

package main

import (
	"flag"
	"os"

	"github.com/simagix/mongo-ftdc/analytics"
)

func main() {
	output := flag.Bool("output", false, "output metadata only")
	flag.Parse()
	filenames := append([]string{}, flag.Args()...)
	if *output == true {
		metrics := analytics.NewMetrics(filenames)
		metrics.SetOutputOnly(true)
		metrics.Read()
		os.Exit(0)
	}
	analytics.SingleJSONServer(filenames)
}
