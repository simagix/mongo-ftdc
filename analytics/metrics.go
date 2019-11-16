// Copyright 2019 Kuei-chun Chen. All rights reserved.

package analytics

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/simagix/gox"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Metrics stores metrics from FTDC data
type Metrics struct {
	sync.RWMutex
	ftdcStats   FTDCStats
	filenames   []string
	isProcessed bool
	outputOnly  bool
}

// NewMetrics returns &Metrics
func NewMetrics(filenames []string) *Metrics {
	gob.Register(primitive.DateTime(1))
	gob.Register(primitive.A{})
	gob.Register(primitive.D{})
	gob.Register(primitive.M{})
	metrics := &Metrics{filenames: getFilenames(filenames), isProcessed: false}
	if len(metrics.filenames) == 0 {
		return metrics
	}
	infile := filenames[0]
	if len(filenames) == 1 && strings.HasSuffix(infile, ".ftdc-enc.gz") {
		metrics.isProcessed = true
	}
	return metrics
}

// SetOutputOnly set output flag to process FTDC in foreground
func (m *Metrics) SetOutputOnly(outputOnly bool) {
	m.outputOnly = outputOnly
}

// Read reads metrics files/data
func (m *Metrics) Read() {
	infile := m.filenames[0]
	if m.isProcessed == true {
		if err := m.readProcessedFTDC(infile); err != nil {
			log.Println(err)
		}
	} else {
		m.parse()
	}
}

func (m *Metrics) readProcessedFTDC(infile string) error {
	log.Println("Reading from processed FTDC data", infile)
	var err error
	var data []byte
	var file *os.File
	var reader *bufio.Reader

	if file, err = os.Open(infile); err != nil {
		return err
	}
	if reader, err = gox.NewReader(file); err != nil {
		return err
	}
	if data, err = ioutil.ReadAll(reader); err != nil {
		return err
	}
	buffer := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buffer)
	if err = dec.Decode(&m.ftdcStats); err != nil {
		return err
	}

	points := m.ftdcStats.TimeSeriesData["wt_cache_used"].DataPoints
	tm1 := time.Unix(0, int64(points[0][1])*int64(time.Millisecond)).Unix() * 1000
	tm2 := time.Unix(0, int64(points[len(points)-1][1])*int64(time.Millisecond)).Unix() * 1000
	log.Println(tm1, tm2)
	endpoint := fmt.Sprintf("/d/simagix-grafana/mongodb-mongo-ftdc?orgId=1&from=%v&to=%v", tm1, tm2)
	log.Printf("http://localhost:3000%v\n", endpoint)
	log.Printf("http://localhost:3030%v\n", endpoint)
	return err
}

func (m *Metrics) parse() string {
	if m.outputOnly == true {
		diag := decode(m.filenames, 1)
		m.SetFTDCDetailStats(diag)
		m.outputProcessedFTDC()
		return diag.endpoint
	}
	diag := decode(m.filenames, 300)
	hostname, _ := os.Hostname()
	port := 3000
	span := 1
	if hostname == "ftdc" { // from docker-compose
		port = 3030
		span = len(m.filenames)/4 + 1
	}
	uri := fmt.Sprintf("http://localhost:%d%v", port, diag.endpoint)
	m.SetFTDCDetailStats(diag)
	log.Println(uri)
	go func(filenames []string, span int) {
		diag := decode(m.filenames, span)
		m.SetFTDCDetailStats(diag)
		log.Println(uri)
	}(m.filenames, span)
	return diag.endpoint
}

func decode(filenames []string, span int) *DiagnosticData {
	diag := NewDiagnosticData(span)
	if err := diag.DecodeDiagnosticData(filenames); err != nil { // get summary
		log.Fatal(err)
	}
	return diag
}

// outputProcessedFTDC outputs processed FTDC data
func (m *Metrics) outputProcessedFTDC() {
	var data bytes.Buffer
	enc := gob.NewEncoder(&data)
	if err := enc.Encode(m.ftdcStats); err != nil {
		log.Println("encode error:", err)
	}
	ofile := filepath.Base(m.filenames[0]) + ".ftdc-enc.gz"
	gox.OutputGzipped(data.Bytes(), ofile)
	log.Println("FTDC metadata written to", ofile)
}

func getFilenames(filenames []string) []string {
	var err error
	var fi os.FileInfo
	fnames := []string{}
	for _, filename := range filenames {
		if fi, err = os.Stat(filename); err != nil {
			continue
		}
		switch mode := fi.Mode(); {
		case mode.IsDir():
			files, _ := ioutil.ReadDir(filename)
			for _, file := range files {
				if file.IsDir() == false &&
					(strings.HasPrefix(file.Name(), "metrics.") || strings.HasPrefix(file.Name(), "keyhole_stats.")) {
					fnames = append(fnames, filename+"/"+file.Name())
				}
			}
		case mode.IsRegular():
			fnames = append(fnames, filename)
		}
	}
	return fnames
}

// Handler handle HTTP requests
func (m *Metrics) Handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path[1:] == "grafana/" {
		fmt.Fprintf(w, "ok\n")
	} else if r.URL.Path[1:] == "grafana/query" {
		m.query(w, r)
	} else if r.URL.Path[1:] == "grafana/search" {
		m.search(w, r)
	} else if r.URL.Path[1:] == "grafana/dir" {
		m.readDirectory(w, r)
	}
}

// SetFTDCDetailStats populate FTDC details, 1 second span
func (m *Metrics) SetFTDCDetailStats(diag *DiagnosticData) {
	m.RLock()
	defer m.RUnlock()
	setFTDCStats(diag, &m.ftdcStats)
}

type directoryReq struct {
	Dir  string `json:"dir"`
	Span int    `json:"span"`
}

func (m *Metrics) readDirectory(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodOptions:
	case http.MethodPost:
		var err error
		decoder := json.NewDecoder(r.Body)
		var dr directoryReq
		if err = decoder.Decode(&dr); err != nil {
			json.NewEncoder(w).Encode(bson.M{"ok": 0, "err": err.Error()})
			return
		}
		m.filenames = getFilenames([]string{dr.Dir})
		ep := m.parse()
		json.NewEncoder(w).Encode(bson.M{"ok": 1, "endpoint": ep})

	default:
		http.Error(w, "bad method; supported OPTIONS, POST", http.StatusBadRequest)
		return
	}
}

func (m *Metrics) search(w http.ResponseWriter, r *http.Request) {
	var list []string
	for _, doc := range m.ftdcStats.TimeSeriesData {
		list = append(list, doc.Target)
	}

	list = append(list, "host_info")
	json.NewEncoder(w).Encode(list)
}

func (m *Metrics) query(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var qr QueryRequest
	if err := decoder.Decode(&qr); err != nil {
		return
	}
	ftdc := m.ftdcStats
	var tsData []interface{}
	for _, target := range qr.Targets {
		if target.Type == "timeserie" {
			if target.Target == "replication_lags" { // replaced with actual hostname
				for k, v := range ftdc.ReplicationLags {
					data := v
					data.Target = k
					tsData = append(tsData, filterTimeSeriesData(data, qr.Range.From, qr.Range.To))
				}
			} else if target.Target == "disks_utils" {
				for k, v := range ftdc.DiskStats {
					data := v.Utilization
					data.Target = k
					tsData = append(tsData, filterTimeSeriesData(data, qr.Range.From, qr.Range.To))
				}
			} else if target.Target == "disks_iops" {
				for k, v := range ftdc.DiskStats {
					data := v.IOPS
					data.Target = k
					tsData = append(tsData, filterTimeSeriesData(data, qr.Range.From, qr.Range.To))
				}
			} else {
				tsData = append(tsData, filterTimeSeriesData(ftdc.TimeSeriesData[target.Target], qr.Range.From, qr.Range.To))
			}
		} else if target.Type == "table" {
			if target.Target == "host_info" {
				headerList := []bson.M{}
				headerList = append(headerList, bson.M{"text": "Info", "type": "string"})
				headerList = append(headerList, bson.M{"text": "Value", "type": "string"})
				var si ServerInfoDoc
				b, _ := json.Marshal(ftdc.ServerInfo)
				if err := json.Unmarshal(b, &si); err != nil {
					rowList := [][]string{[]string{"Error", err.Error()}}
					doc1 := bson.M{"columns": headerList, "type": "table", "rows": rowList}
					tsData = append(tsData, doc1)
					continue
				}
				rowList := [][]string{}

				rowList = append(rowList, []string{"CPU", strconv.Itoa(si.HostInfo.System.NumCores) + " cores (" + si.HostInfo.System.CPUArch + ")"})
				// rowList = append(rowList, []string{"Hostname", si.HostInfo.System.Hostname})
				rowList = append(rowList, []string{"Memory (MB)", strconv.Itoa(si.HostInfo.System.MemSizeMB)})
				rowList = append(rowList, []string{"MongoDB Version", si.BuildInfo.Version})
				rowList = append(rowList, []string{"OS", si.HostInfo.OS.Name})
				rowList = append(rowList, []string{"OS Type", si.HostInfo.OS.Type + " (" + si.HostInfo.OS.Version + ")"})
				doc1 := bson.M{"columns": headerList, "type": "table", "rows": rowList}
				tsData = append(tsData, doc1)
			}
		}
	}
	json.NewEncoder(w).Encode(tsData)
}

func parseTime(filename string) (time.Time, error) {
	layout := "2006-01-02T15-04-05Z"
	x := strings.Index(filename, "metrics.")
	y := strings.LastIndex(filename, "-")
	if x < 0 || y < 0 || y < x {
		return time.Now(), errors.New("not valid")
	}
	t, err := time.Parse(layout, filename[x+8:y])
	if err != nil {
		return time.Now(), err
	}
	return t, nil
}

func filterTimeSeriesData(tsData TimeSeriesDoc, from time.Time, to time.Time) TimeSeriesDoc {
	var data = TimeSeriesDoc{DataPoints: [][]float64{}}
	hours := int(to.Sub(from).Hours()) + 1
	data.Target = tsData.Target
	for i, v := range tsData.DataPoints {
		tm := time.Unix(0, int64(v[1])*int64(time.Millisecond))
		if (i%hours) != 0 || tm.After(to) || tm.Before(from) {
			continue
		}
		data.DataPoints = append(data.DataPoints, v)
	}
	return data
}
