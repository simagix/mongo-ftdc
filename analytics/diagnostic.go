// Copyright 2019 Kuei-chun Chen. All rights reserved.

package analytics

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/simagix/gox"
	"github.com/simagix/mongo-ftdc/ftdc"
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
	span              int
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
func NewDiagnosticData(span int) *DiagnosticData {
	return &DiagnosticData{ServerStatusList: []ServerStatusDoc{}, ReplSetStatusList: []ReplSetStatusDoc{}, span: span}
}

// DecodeDiagnosticData decodes FTDC data files
func (d *DiagnosticData) DecodeDiagnosticData(filenames []string) error {
	var err error
	var fi os.FileInfo
	fnames := []string{}

	for _, filename := range filenames {
		if fi, err = os.Stat(filename); err != nil {
			return err
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

	if err = d.readDiagnosticFiles(fnames); err != nil {
		return err
	}

	if len(d.ServerStatusList) == 0 {
		return errors.New("no FTDC data found")
	}

	log.Printf("FTDC data from %v to %v\n", d.ServerStatusList[0].LocalTime.Format("2006-01-02T15:04:05Z"),
		d.ServerStatusList[len(d.ServerStatusList)-1].LocalTime.Format("2006-01-02T15:04:05Z"))
	return nil
}

// PrintDiagnosticData prints diagnostic data of MongoD
func (d *DiagnosticData) PrintDiagnosticData(filenames []string) (string, error) {
	if err := d.DecodeDiagnosticData(filenames); err != nil {
		return "", err
	}
	strs := []string{}
	if d.ServerInfo != nil {
		b, _ := json.MarshalIndent(d.ServerInfo, "", "  ")
		strs = append(strs, string(b))
	}
	strs = append(strs, PrintAllStats(d.ServerStatusList, -1))
	return strings.Join(strs, "\n"), nil
}

// readDiagnosticDir reads diagnotics.data from a directory
func (d *DiagnosticData) readDiagnosticDir(dirname string) error {
	var err error
	var files []os.FileInfo
	var filenames []string

	if files, err = ioutil.ReadDir(dirname); err != nil {
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
	sort.Strings(filenames)
	if strings.Index(filenames[0], "keyhole_stats.") >= 0 {
		for _, filename := range filenames {
			d.analyzeServerStatus(filename)
		}
		return err
	}

	btime := time.Now()
	log.Printf("reading %d files with %d second(s) interval\n", len(filenames), d.span)
	var diagDataMap = map[string]DiagnosticData{}
	nThreads := runtime.NumCPU()
	if nThreads < 4 {
		nThreads = 4
	}
	var wg = gox.NewWaitGroup(nThreads) // use 4 threads to read
	for threadNum := 0; threadNum < len(filenames); threadNum++ {
		filename := filenames[threadNum]
		if strings.Index(filename, "metrics.") < 0 {
			continue
		}
		wg.Add(1)
		go func(filename string) {
			defer wg.Done()
			var diagData DiagnosticData
			if diagData, err = d.readDiagnosticFile(filename); err == nil {
				diagDataMap[filename] = diagData
			}
		}(filename)
	}
	wg.Wait()

	keys := []string{}
	for k := range diagDataMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if diagDataMap[key].ServerInfo != nil {
			d.ServerInfo = diagDataMap[key].ServerInfo
		}
		d.ServerStatusList = append(d.ServerStatusList, diagDataMap[key].ServerStatusList...)
		d.SystemMetricsList = append(d.SystemMetricsList, diagDataMap[key].SystemMetricsList...)
		d.ReplSetStatusList = append(d.ReplSetStatusList, diagDataMap[key].ReplSetStatusList...)
	}
	log.Println(len(filenames), "files loaded, time spent:", time.Now().Sub(btime))
	return err
}

// readDiagnosticFile reads diagnostic.data from a file
func (d *DiagnosticData) readDiagnosticFile(filename string) (DiagnosticData, error) {
	btm := time.Now()
	var diagData = DiagnosticData{}
	var buffer []byte
	var err error

	if buffer, err = ioutil.ReadFile(filename); err != nil {
		return diagData, err
	}

	metrics := ftdc.NewMetrics()
	if d.span >= 300 {
		metrics.ReadMetricsSummary(buffer)
		diagData.ServerInfo = metrics.Doc
		for _, v := range metrics.Data {
			block := v.Buffer
			var doc DiagnosticDoc
			bson.Unmarshal(block[:v.DocSize], &doc) // first document
			diagData.ServerStatusList = append(diagData.ServerStatusList, doc.ServerStatus)
			diagData.SystemMetricsList = append(diagData.SystemMetricsList, doc.SystemMetrics)
			diagData.ReplSetStatusList = append(diagData.ReplSetStatusList, doc.ReplSetGetStatus)
		}
	} else {
		metrics.ReadAllMetrics(buffer)
		diagData.ServerInfo = metrics.Doc
		for _, v := range metrics.Data {
			block := v.Buffer
			var doc DiagnosticDoc
			bson.Unmarshal(block[:v.DocSize], &doc) // first document
			diagData.ReplSetStatusList = append(diagData.ReplSetStatusList, doc.ReplSetGetStatus)
		}
		for _, v := range metrics.Data {
			for i := uint32(0); i < v.NumDeltas; i += uint32(d.span) {
				ss := getServerStatusDataPoints(v.DataPointsMap, i)
				diagData.ServerStatusList = append(diagData.ServerStatusList, ss)
				sm := getSystemMetricsDataPoints(v.DataPointsMap, i)
				diagData.SystemMetricsList = append(diagData.SystemMetricsList, sm)
			}
		}
	}

	filename = strings.TrimRight(filename, "/")
	i := strings.LastIndex(filename, "/")
	if i >= 0 {
		filename = filename[i+1:]
	}
	log.Println(filename, "blocks:", len(metrics.Data), ", time:", time.Now().Sub(btm))
	return diagData, err
}

// analyzeServerStatus -
func (d *DiagnosticData) analyzeServerStatus(filename string) error {
	var err error
	var file *os.File
	var reader *bufio.Reader
	var allDocs = []ServerStatusDoc{}
	var docs = []ServerStatusDoc{}
	var allRepls = []ReplSetStatusDoc{}
	var repls = []ReplSetStatusDoc{}

	if file, err = os.Open(filename); err != nil {
		return err
	}
	defer file.Close()

	if reader, err = gox.NewReader(file); err != nil {
		return err
	}

	for {
		line, ferr := reader.ReadBytes('\n')
		if ferr == io.EOF {
			break
		}

		docs = []ServerStatusDoc{}
		if err = json.Unmarshal(line, &docs); err == nil {
			if len(docs) > 0 && docs[0].Host != "" {
				allDocs = append(allDocs, docs...)
			} else if err = json.Unmarshal(line, &repls); err == nil { // ReplSetStatusDoc
				allRepls = append(allRepls, repls...)
			} else {
				log.Println(err)
			}
		} else {
			log.Println(err)
		}
	}

	if len(allDocs) == 0 && len(allRepls) == 0 {
		return errors.New("No doc found")
	}

	d.ServerStatusList = append(d.ServerStatusList, allDocs...)
	d.ReplSetStatusList = append(d.ReplSetStatusList, allRepls...)
	return err
}
