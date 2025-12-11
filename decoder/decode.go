// Copyright 2018-present Kuei-chun Chen. All rights reserved.
// decode.go

package decoder

import (
	"bytes"
	"errors"
	"io"
	"strconv"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// PathSeparator -
const PathSeparator = "/" // path separator

// Decode decodes MongoDB FTDC data
func (m *Metrics) decode(buffer []byte) (MetricsData, error) {
	var err error
	var dp = MetricsData{DataPointsMap: map[string][]uint64{}}

	r := bytes.NewReader(buffer)
	docSize := GetUint32(r) // first bson document length
	r.Seek(int64(docSize), io.SeekStart)
	numAttribs := GetUint32(r)  // 4 bytes # of keys
	dp.NumDeltas = GetUint32(r) // 4 bytes # of deltas
	ptr, _ := r.Seek(0, io.SeekCurrent)
	r = bytes.NewReader(buffer[ptr:]) // reset reader to where deltas begin

	// Pre-calculate total capacity needed: 1 (initial value) + NumDeltas
	sliceCap := int(dp.NumDeltas) + 1

	// use DOM (bson.D) to ensure orders
	var attribsList = make([]string, 0, numAttribs) // pre-allocate for known number of attributes
	// systemMetrics
	// end
	// start
	// serverStatus
	// replSetGetStatus
	// local.oplog.rs.stats
	var docElem = bson.D{}
	bson.Unmarshal(buffer[:docSize], &docElem) // first document
	traverseDocElem(&attribsList, &dp.DataPointsMap, docElem, "", sliceCap)

	// Verify list matches header (map may have fewer due to duplicate BSON keys)
	if len(attribsList) != int(numAttribs) {
		return dp, errors.New("inconsistent FTDC data")
	}

	// deltas
	// d where d > 0, return d
	// 0d -> there are d number of zeros
	var delta uint64
	var zerosLeft uint64
	for _, attr := range attribsList {
		list := dp.DataPointsMap[attr]
		v := list[0]
		for j := uint32(0); j < dp.NumDeltas; j++ {
			if zerosLeft != 0 {
				delta = 0
				zerosLeft--
			} else {
				delta = Uvarint(r)
				if delta == 0 {
					zerosLeft = Uvarint(r)
				}
			}
			v += delta
			list = append(list, v) // no reallocation due to pre-allocated capacity
		}
		dp.DataPointsMap[attr] = list
	}
	dp.Block = buffer[:docSize]
	return dp, err
}

// addAttribute adds an attribute, handling BSON duplicate keys by only adding to map once
func addAttribute(attribsList *[]string, attribsMap *map[string][]uint64, key string, val uint64, sliceCap int) {
	// Always add to list (needed for delta processing order)
	(*attribsList) = append((*attribsList), key)
	// Only add to map if key is new (duplicates share the same slice)
	if _, exists := (*attribsMap)[key]; !exists {
		s := make([]uint64, 1, sliceCap)
		s[0] = val
		(*attribsMap)[key] = s
	}
}

func traverseDocElem(attribsList *[]string, attribsMap *map[string][]uint64, docElem interface{}, parentPath string, sliceCap int) {
	switch value := docElem.(type) {
	case bson.A:
		for i, v := range value {
			fld := parentPath + PathSeparator + strconv.Itoa(i)
			traverseDocElem(attribsList, attribsMap, v, fld, sliceCap)
		}
	case bool:
		v := uint64(0)
		if value {
			v = 1
		}
		addAttribute(attribsList, attribsMap, parentPath, v, sliceCap)
	case bson.D:
		elem := docElem.(bson.D)
		for _, elem := range elem {
			name := elem.Key
			if parentPath != "" {
				name = parentPath + PathSeparator + name
			}
			traverseDocElem(attribsList, attribsMap, elem.Value, name, sliceCap)
		}
	case primitive.Timestamp:
		tKey := parentPath + "/t"
		addAttribute(attribsList, attribsMap, tKey, uint64(0), sliceCap)
		iKey := parentPath + "/i"
		addAttribute(attribsList, attribsMap, iKey, uint64(0), sliceCap)
	case primitive.ObjectID: // ignore it
	case string: // ignore it
	case float64:
		addAttribute(attribsList, attribsMap, parentPath, uint64(value), sliceCap)
	case int:
		addAttribute(attribsList, attribsMap, parentPath, uint64(value), sliceCap)
	case int32:
		addAttribute(attribsList, attribsMap, parentPath, uint64(value), sliceCap)
	case int64:
		addAttribute(attribsList, attribsMap, parentPath, uint64(value), sliceCap)
	case primitive.DateTime:
		addAttribute(attribsList, attribsMap, parentPath, uint64(value), sliceCap)
	default:
		// log.Fatalf("'%s' ==> %T\n", parentPath, value)
	}
}
