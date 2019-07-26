// Copyright 2019 Kuei-chun Chen. All rights reserved.

package main

import (
	"flag"

	"github.com/simagix/mongo-ftdc/analytics"
)

func main() {
	flag.Parse()
	filenames := append([]string{}, flag.Args()...)
	analytics.SingleJSONServer(filenames)
}
