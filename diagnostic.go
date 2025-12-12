// Copyright 2019-present Kuei-chun Chen. All rights reserved.
// diagnostic.go

package ftdc

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/simagix/gox"
	"github.com/simagix/mongo-ftdc/decoder"
	"go.mongodb.org/mongo-driver/bson"
)

// OSDoc -
type OSDoc struct {
	Name    string `json:"name" bson:"name"`
	Type    string `json:"type" bson:"type"`
	Version string `json:"version" bson:"version"`
}

// SystemDoc -
type SystemDoc struct {
	CPUArch   string `json:"cpuArch" bson:"cpuArch"`
	Hostname  string `json:"hostname" bson:"hostname"`
	NumCores  int    `json:"numCores" bson:"numCores"`
	MemSizeMB int    `json:"memSizeMB" bson:"memSizeMB"`
}

// HostInfo -
type HostInfo struct {
	OS     OSDoc     `json:"os" bson:"os"`
	System SystemDoc `json:"system" bson:"system"`
}

// BuildInfo -
type BuildInfo struct {
	Version string `json:"version" bson:"version"`
}

// ServerInfoDoc -
type ServerInfoDoc struct {
	HostInfo  HostInfo  `json:"hostInfo" bson:"hostInfo"`
	BuildInfo BuildInfo `json:"buildInfo" bson:"buildInfo"`
}

// DiagnosticData -
type DiagnosticData struct {
	ServerInfo        interface{}
	ServerStatusList  []ServerStatusDoc
	ReplSetStatusList []ReplSetStatusDoc
	SystemMetricsList []SystemMetricsDoc
	endpoints         []string
}

// DiagnosticDoc -
type DiagnosticDoc struct {
	Start            time.Time        `json:"start" bson:"start"`
	ServerStatus     ServerStatusDoc  `json:"serverStatus" bson:"serverStatus"`
	ReplSetGetStatus ReplSetStatusDoc `json:"replSetGetStatus" bson:"replSetGetStatus"`
	SystemMetrics    SystemMetricsDoc `json:"systemMetrics" bson:"systemMetrics"`
	End              time.Time        `json:"end" bson:"end"`
}

// NewDiagnosticData -
func NewDiagnosticData() *DiagnosticData {
	return &DiagnosticData{ServerStatusList: []ServerStatusDoc{}, ReplSetStatusList: []ReplSetStatusDoc{}}
}

// GetEndPoints gets grafana uri
func (d *DiagnosticData) GetEndPoints() []string {
	return d.endpoints
}

// DecodeDiagnosticData decodes FTDC data files
func (d *DiagnosticData) DecodeDiagnosticData(filenames []string) error {
	var err error
	fnames := GetMetricsFilenames(filenames)
	if err = d.readDiagnosticFiles(fnames); err != nil {
		return err
	}

	if len(d.ServerStatusList) == 0 {
		log.Println("no server status found")
		t := time.Now().Unix() * 1000
		minute := int64(60) * 1000
		d.endpoints = append(d.endpoints, fmt.Sprintf(analyticsEndpoint, t, t+(10*minute)))
	} else {
		fmt.Printf("\nStats from %v to %v\n", d.ServerStatusList[0].LocalTime.Format("2006-01-02T15:04:05Z"),
			d.ServerStatusList[len(d.ServerStatusList)-1].LocalTime.Format("2006-01-02T15:04:05Z"))
		d.endpoints = append(d.endpoints, fmt.Sprintf(analyticsEndpoint,
			d.ServerStatusList[0].LocalTime.Unix()*1000,
			d.ServerStatusList[len(d.ServerStatusList)-1].LocalTime.Unix()*1000))
	}
	return nil
}

func (d *DiagnosticData) readDiagnosticDir(dirname string) error {
	var err error
	var files []os.DirEntry
	var filenames []string

	if files, err = os.ReadDir(dirname); err != nil {
		return err
	}

	for _, f := range files {
		if strings.Index(f.Name(), "metrics.") != 0 && strings.Index(f.Name(), "keyhole_stats.") != 0 {
			continue
		}
		filename := dirname + "/" + f.Name()
		filenames = append(filenames, filename)
	}

	if len(filenames) == 0 {
		return errors.New("No metrics file found under " + dirname)
	}
	return d.readDiagnosticFiles(filenames)
}

// readDiagnosticFiles reads multiple files
func (d *DiagnosticData) readDiagnosticFiles(filenames []string) error {
	var err error
	if len(filenames) == 0 {
		return errors.New("no valid data file found")
	}
	sort.Strings(filenames)
	if strings.Contains(filenames[0], "keyhole_stats.") {
		for _, filename := range filenames {
			if err = d.analyzeServerStatusFromFile(filename); err != nil {
				return err
			}
		}
		return err
	}

	btime := time.Now()
	log.Printf("reading %d files with %d second(s) interval\n", len(filenames), 1)

	// Use a slice instead of map to avoid race condition, protected by mutex
	type fileResult struct {
		filename string
		data     DiagnosticData
		err      error
	}
	results := make([]fileResult, 0, len(filenames))
	var mu sync.Mutex

	nThreads := runtime.NumCPU() - 1
	if nThreads < 1 {
		nThreads = 1
	}
	var wg = gox.NewWaitGroup(nThreads)
	for threadNum := 0; threadNum < len(filenames); threadNum++ {
		filename := filenames[threadNum]
		if !strings.Contains(filename, "metrics.") {
			continue
		}
		wg.Add(1)
		go func(filename string) {
			defer wg.Done()
			diagData, ferr := d.readDiagnosticFile(filename)
			mu.Lock()
			results = append(results, fileResult{filename: filename, data: diagData, err: ferr})
			mu.Unlock()
		}(filename)
	}
	wg.Wait()

	// Sort by filename to ensure consistent ordering
	sort.Slice(results, func(i, j int) bool {
		return results[i].filename < results[j].filename
	})

	// Pre-calculate total sizes for pre-allocation
	totalServerStatus := 0
	totalSystemMetrics := 0
	totalReplSetStatus := 0
	for _, r := range results {
		if r.err != nil {
			continue
		}
		totalServerStatus += len(r.data.ServerStatusList)
		totalSystemMetrics += len(r.data.SystemMetricsList)
		totalReplSetStatus += len(r.data.ReplSetStatusList)
	}

	// Pre-allocate slices
	d.ServerStatusList = make([]ServerStatusDoc, 0, totalServerStatus)
	d.SystemMetricsList = make([]SystemMetricsDoc, 0, totalSystemMetrics)
	d.ReplSetStatusList = make([]ReplSetStatusDoc, 0, totalReplSetStatus)

	// Merge results
	for _, r := range results {
		if r.err != nil {
			err = r.err
			continue
		}
		if r.data.ServerInfo != nil {
			d.ServerInfo = r.data.ServerInfo
		}
		d.ServerStatusList = append(d.ServerStatusList, r.data.ServerStatusList...)
		d.SystemMetricsList = append(d.SystemMetricsList, r.data.SystemMetricsList...)
		d.ReplSetStatusList = append(d.ReplSetStatusList, r.data.ReplSetStatusList...)
	}
	log.Println(len(filenames), "files loaded, time spent:", time.Since(btime))
	return err
}

// readDiagnosticFile reads diagnostic.data from a file
func (d *DiagnosticData) readDiagnosticFile(filename string) (DiagnosticData, error) {
	btm := time.Now()
	var diagData = DiagnosticData{}
	var buffer []byte
	var err error
	var r *bufio.Reader

	if r, err = gox.NewFileReader(filename); err != nil {
		return diagData, err
	}
	if buffer, err = io.ReadAll(r); err != nil {
		return diagData, err
	}

	metrics := decoder.NewMetrics()
	metrics.ReadAllMetrics(&buffer)
	diagData.ServerInfo = metrics.Doc

	// Pre-calculate total capacity needed
	totalDeltas := 0
	for _, v := range metrics.Data {
		totalDeltas += int(v.NumDeltas)
	}

	// Pre-allocate slices
	diagData.ServerStatusList = make([]ServerStatusDoc, 0, totalDeltas)
	diagData.SystemMetricsList = make([]SystemMetricsDoc, 0, totalDeltas)
	diagData.ReplSetStatusList = make([]ReplSetStatusDoc, 0, len(metrics.Data))

	for _, v := range metrics.Data {
		var doc DiagnosticDoc
		bson.Unmarshal(v.Block, &doc) // first document
		diagData.ReplSetStatusList = append(diagData.ReplSetStatusList, doc.ReplSetGetStatus)
		attrib := NewAttribs(&v.DataPointsMap)
		for i := 0; i < int(v.NumDeltas); i++ {
			ss := attrib.GetServerStatusDataPoints(i)
			diagData.ServerStatusList = append(diagData.ServerStatusList, ss)
			sm := attrib.GetSystemMetricsDataPoints(i)
			diagData.SystemMetricsList = append(diagData.SystemMetricsList, sm)
		}
	}

	filename = filepath.Base(filename)
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	mem := fmt.Sprintf("Memory Alloc = %v MiB, TotalAlloc = %v MiB", m.Alloc/(1024*1024), m.TotalAlloc/(1024*1024))
	log.Println(filename, "blocks:", len(metrics.Data), ", time:", time.Since(btm), mem)
	return diagData, err
}

// analyzeServerStatus analyzes serverStatus from a file
func (d *DiagnosticData) analyzeServerStatusFromFile(filename string) error {
	var err error
	var reader *bufio.Reader

	if reader, err = gox.NewFileReader(filename); err != nil {
		return err
	}
	return d.AnalyzeServerStatus(reader)
}

// AnalyzeServerStatus -
func (d *DiagnosticData) AnalyzeServerStatus(reader *bufio.Reader) error {
	var err error
	var allDocs = []ServerStatusDoc{}
	var docs = []ServerStatusDoc{}
	var allRepls = []ReplSetStatusDoc{}
	var repls = []ReplSetStatusDoc{}

	cnt := 0
	for {
		line, ferr := reader.ReadBytes('\n')
		if ferr == io.EOF {
			break
		}
		cnt++
		if cnt%3 == 1 { // serverStatus
			if err = json.Unmarshal(line, &docs); err != nil {
				return err
			}
			allDocs = append(allDocs, docs...)
		} else if cnt%3 == 2 { // replica
			if err = json.Unmarshal(line, &repls); err != nil {
				return err
			}
			allRepls = append(allRepls, repls...)
		} else if cnt == 3 { // serverInfo
			d.ServerInfo = bson.M{}
			if err = json.Unmarshal(line, &d.ServerInfo); err != nil {
				return err
			}
		}
	}

	if len(allDocs) == 0 && len(allRepls) == 0 {
		return errors.New("no doc found")
	}

	d.ServerStatusList = append(d.ServerStatusList, allDocs...)
	if cnt < 3 && len(d.ServerStatusList) > 0 { // shortcut hack
		d.ServerInfo = bson.M{"BuildInfo": bson.M{"Version": d.ServerStatusList[0].Version}}
	}
	d.ReplSetStatusList = append(d.ReplSetStatusList, allRepls...)
	return err
}
