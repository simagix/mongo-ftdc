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

	if len(dp.DataPointsMap) != int(numAttribs) || len(attribsList) != int(numAttribs) {
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
		x := make([]uint64, 1, sliceCap) // pre-allocate for all deltas
		x[0] = v
		(*attribsMap)[parentPath] = x
		(*attribsList) = append((*attribsList), parentPath)
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
		tSlice := make([]uint64, 1, sliceCap)
		tSlice[0] = uint64(0)
		(*attribsMap)[tKey] = tSlice
		(*attribsList) = append((*attribsList), tKey)
		iKey := parentPath + "/i"
		iSlice := make([]uint64, 1, sliceCap)
		iSlice[0] = uint64(0)
		(*attribsMap)[iKey] = iSlice
		(*attribsList) = append((*attribsList), iKey)
	case primitive.ObjectID: // ignore it
	case string: // ignore it
	case float64:
		s := make([]uint64, 1, sliceCap)
		s[0] = uint64(value)
		(*attribsMap)[parentPath] = s
		(*attribsList) = append((*attribsList), parentPath)
	case int:
		s := make([]uint64, 1, sliceCap)
		s[0] = uint64(value)
		(*attribsMap)[parentPath] = s
		(*attribsList) = append((*attribsList), parentPath)
	case int32:
		s := make([]uint64, 1, sliceCap)
		s[0] = uint64(value)
		(*attribsMap)[parentPath] = s
		(*attribsList) = append((*attribsList), parentPath)
	case int64:
		s := make([]uint64, 1, sliceCap)
		s[0] = uint64(value)
		(*attribsMap)[parentPath] = s
		(*attribsList) = append((*attribsList), parentPath)
	case primitive.DateTime: // ignore it
		s := make([]uint64, 1, sliceCap)
		s[0] = uint64(value)
		(*attribsMap)[parentPath] = s
		(*attribsList) = append((*attribsList), parentPath)
	default:
		// log.Fatalf("'%s' ==> %T\n", parentPath, value)
	}
}
