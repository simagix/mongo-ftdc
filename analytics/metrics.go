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
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/simagix/gox"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// Metrics stores metrics from FTDC data
type Metrics struct {
	sync.RWMutex
	endpoint  string
	ftdcStats FTDCStats
	verbose   bool
}

// FTDCStats FTDC stats
type FTDCStats struct {
	DiskStats         map[string]DiskStats
	ReplicationLags   map[string]TimeSeriesDoc
	ReplSetLegends    []string
	ReplSetStatusList []ReplSetStatusDoc
	ServerInfo        interface{}
	ServerStatusList  []ServerStatusDoc
	SystemMetricsList []SystemMetricsDoc
	TimeSeriesData    map[string]TimeSeriesDoc
}

// DiskStats -
type DiskStats struct {
	Utilization  TimeSeriesDoc
	IOPS         TimeSeriesDoc
	IOInProgress TimeSeriesDoc
}

type directoryReq struct {
	Dir  string `json:"dir"`
	Span int    `json:"span"`
}

// NewMetrics returns &Metrics
func NewMetrics() *Metrics {
	gob.Register(primitive.DateTime(1))
	gob.Register(primitive.A{})
	gob.Register(primitive.D{})
	gob.Register(primitive.M{})
	m := Metrics{}
	http.HandleFunc("/grafana", gox.Cors(m.Handler))
	http.HandleFunc("/grafana/", gox.Cors(m.Handler))
	return &m
}

const endpointTemplate = `/d/simagix-grafana/mongodb-mongo-ftdc?orgId=1&from=%v&to=%v`

// SetVerbose sets verbose mode
func (m *Metrics) SetVerbose(verbose bool) { m.verbose = verbose }

// ProcessFiles reads metrics files/data
func (m *Metrics) ProcessFiles(filenames []string) error {
	hostname, _ := os.Hostname()
	port := 3000
	span := 1
	if len(filenames) == 0 {
		t := time.Now().Unix() * 1000
		minute := int64(60) * 1000
		endpoint := fmt.Sprintf(endpointTemplate, t, t+(10*minute))
		log.Println(fmt.Sprintf("http://localhost:%d%v", port, endpoint))
		return errors.New("no available data files found")
	}
	if hostname == "ftdc" { // from docker-compose
		port = 3030
		span = (len(filenames)-1)/5 + 1
	}
	diag := NewDiagnosticData(span)
	if err := diag.DecodeDiagnosticData(filenames); err != nil { // get summary
		return err
	}
	m.endpoint = diag.endpoint
	m.AddFTDCDetailStats(diag)
	log.Println(fmt.Sprintf("http://localhost:%d%v", port, diag.endpoint))
	return nil
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
	endpoint := fmt.Sprintf(endpointTemplate, tm1, tm2)
	log.Printf("http://localhost:3000%v\n", endpoint)
	log.Printf("http://localhost:3030%v\n", endpoint)
	return err
}

// Handler handle HTTP requests
func (m *Metrics) Handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path[1:] == "grafana/query" {
		m.query(w, r)
	} else if r.URL.Path[1:] == "grafana/search" {
		m.search(w, r)
	} else if r.URL.Path[1:] == "grafana/dir" {
		m.readDirectory(w, r)
	} else {
		json.NewEncoder(w).Encode(bson.M{"ok": 1, "message": "hello ftdc!"})
	}
}

func (m *Metrics) readDirectory(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodOptions:
	case http.MethodPost:
		decoder := json.NewDecoder(r.Body)
		var dr directoryReq
		if err := decoder.Decode(&dr); err != nil {
			json.NewEncoder(w).Encode(bson.M{"ok": 0, "err": err.Error()})
			return
		}
		filenames := getFilenames([]string{dr.Dir})
		if err := m.ProcessFiles(filenames); err != nil {
			json.NewEncoder(w).Encode(bson.M{"ok": 0, "err": err.Error()})
		} else {
			json.NewEncoder(w).Encode(bson.M{"ok": 1, "endpoint": m.endpoint})
		}
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
	var tsData []interface{}
	decoder := json.NewDecoder(r.Body)
	var qr QueryRequest
	if err := decoder.Decode(&qr); err != nil {
		json.NewEncoder(w).Encode(tsData)
		return
	}
	ftdc := m.ftdcStats
	for _, target := range qr.Targets {
		if target.Type == "timeserie" {
			if target.Target == "replication_lags" && len(ftdc.ReplicationLags) > 0 { // replaced with actual hostname
				for k, v := range ftdc.ReplicationLags {
					data := v
					data.Target = k
					tsData = append(tsData, filterTimeSeriesData(data, qr.Range.From, qr.Range.To))
				}
			} else if target.Target == "disks_utils" && len(ftdc.DiskStats) > 0 {
				for k, v := range ftdc.DiskStats {
					data := v.Utilization
					data.Target = k
					tsData = append(tsData, filterTimeSeriesData(data, qr.Range.From, qr.Range.To))
				}
			} else if target.Target == "disks_iops" && len(ftdc.DiskStats) > 0 {
				for k, v := range ftdc.DiskStats {
					data := v.IOPS
					data.Target = k
					tsData = append(tsData, filterTimeSeriesData(data, qr.Range.From, qr.Range.To))
				}
			} else if target.Target == "disks_queue_length" {
				for k, v := range ftdc.DiskStats {
					data := v.IOInProgress
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
				p := message.NewPrinter(language.English)
				rowList = append(rowList, []string{"Memory (MB)", p.Sprintf("%d", si.HostInfo.System.MemSizeMB)})
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

// AddFTDCDetailStats assign FTDC values
func (m *Metrics) AddFTDCDetailStats(diag *DiagnosticData) {
	m.RLock()
	defer m.RUnlock()
	ftdc := &m.ftdcStats
	if len(ftdc.ReplSetStatusList) == 0 {
		ftdc.ReplSetStatusList = diag.ReplSetStatusList
	} else {
		lastOne := ftdc.ReplSetStatusList[len(ftdc.ReplSetStatusList)-1]
		for _, v := range diag.ReplSetStatusList {
			if v.Date.After(lastOne.Date) {
				ftdc.ReplSetStatusList = append(ftdc.ReplSetStatusList, v)
			}
		}
	}
	sort.Slice(ftdc.ReplSetStatusList, func(i int, j int) bool {
		return ftdc.ReplSetStatusList[i].Date.Before(ftdc.ReplSetStatusList[j].Date)
	})
	if len(ftdc.ServerStatusList) == 0 {
		ftdc.ServerStatusList = diag.ServerStatusList
	} else {
		lastOne := ftdc.ServerStatusList[len(ftdc.ServerStatusList)-1]
		for _, v := range diag.ServerStatusList {
			if v.LocalTime.After(lastOne.LocalTime) {
				ftdc.ServerStatusList = append(ftdc.ServerStatusList, v)
			}
		}
	}
	sort.Slice(ftdc.ServerStatusList, func(i int, j int) bool {
		return ftdc.ServerStatusList[i].LocalTime.Before(ftdc.ServerStatusList[j].LocalTime)
	})
	if len(ftdc.SystemMetricsList) == 0 {
		ftdc.SystemMetricsList = diag.SystemMetricsList
	} else {
		lastOne := ftdc.SystemMetricsList[len(ftdc.SystemMetricsList)-1]
		for _, v := range diag.SystemMetricsList {
			if v.Start.After(lastOne.Start) {
				ftdc.SystemMetricsList = append(ftdc.SystemMetricsList, v)
			}
		}
	}
	sort.Slice(ftdc.SystemMetricsList, func(i int, j int) bool {
		return ftdc.SystemMetricsList[i].Start.Before(ftdc.SystemMetricsList[j].Start)
	})
	ftdc.ServerInfo = diag.ServerInfo
	btm := time.Now()
	var wiredTigerTSD map[string]TimeSeriesDoc
	var replicationTSD map[string]TimeSeriesDoc
	var systemMetricsTSD map[string]TimeSeriesDoc

	var wg = gox.NewWaitGroup(4) // use 4 threads to read
	wg.Add(1)
	go func() {
		defer wg.Done()
		replicationTSD, ftdc.ReplicationLags = getReplSetGetStatusTimeSeriesDoc(ftdc.ReplSetStatusList, &ftdc.ReplSetLegends) // replSetGetStatus
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		systemMetricsTSD, ftdc.DiskStats = getSystemMetricsTimeSeriesDoc(ftdc.SystemMetricsList) // SystemMetrics
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		ftdc.TimeSeriesData = getServerStatusTimeSeriesDoc(ftdc.ServerStatusList) // ServerStatus
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		wiredTigerTSD = getWiredTigerTimeSeriesDoc(ftdc.ServerStatusList) // ServerStatus
	}()
	wg.Wait()

	// merge
	for k, v := range wiredTigerTSD {
		ftdc.TimeSeriesData[k] = v
	}
	for k, v := range replicationTSD {
		ftdc.TimeSeriesData[k] = v
	}
	for k, v := range systemMetricsTSD {
		ftdc.TimeSeriesData[k] = v
	}
	etm := time.Now()
	var doc ServerInfoDoc
	b, _ := json.Marshal(ftdc.ServerInfo)
	json.Unmarshal(b, &doc)
	if m.verbose == true {
		log.Println("data points added for", doc.HostInfo.System.Hostname, ", time spent:", etm.Sub(btm).String())
	}
}
