// Copyright 2019 Kuei-chun Chen. All rights reserved.

package analytics

import (
	"encoding/json"
	"fmt"
	"log"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// OptimeDoc -
type OptimeDoc struct {
	T  int64 `json:"t" bson:"t"`
	TS int64 `json:"ts" bson:"ts"`
}

// GetOptime -
func GetOptime(optime interface{}) int64 {
	var ts int64
	switch optime.(type) {
	case primitive.D:
		doc := optime.(primitive.D)
		for _, elem := range doc {
			if elem.Key == "ts" {
				b, _ := json.Marshal(elem.Value)
				var optm OptimeDoc
				json.Unmarshal(b, &optm)
				ts = int64(optm.T)
				break
			}
		}
	case primitive.Timestamp:
		b, _ := json.Marshal(optime.(primitive.Timestamp))
		var optm OptimeDoc
		json.Unmarshal(b, &optm)
		ts = int64(optm.T)
	default:
		log.Println(fmt.Sprintf("default =>%T\n", optime))
	}

	return ts
}
