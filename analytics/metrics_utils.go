// Copyright 2019 Kuei-chun Chen. All rights reserved.

package analytics

import (
	"errors"
	"io/ioutil"
	"os"
	"strings"
	"time"
)

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
	var data = TimeSeriesDoc{Target: tsData.Target, DataPoints: [][]float64{}}
	hours := int(to.Sub(from).Hours()) + 1
	points := [][]float64{}
	for i, v := range tsData.DataPoints {
		tm := time.Unix(0, int64(v[1])*int64(time.Millisecond))
		if (i%hours) != 0 || tm.After(to) || tm.Before(from) {
			continue
		}
		points = append(points, v)
	}
	sample := 1000
	denom := len(points) / sample
	if len(points) > sample {
		max := []float64{-1, 0}
		min := []float64{10000000, 0}
		num := 0.0
		for i, point := range points {
			num++
			if point[0] >= max[0] {
				max = point
			}
			if point[0] <= min[0] {
				min = point
			}
			if i%denom == 0 {
				if data.Target == "ticket_avail_read" || data.Target == "ticket_avail_write" {
					data.DataPoints = append(data.DataPoints, min)
				} else {
					data.DataPoints = append(data.DataPoints, max)
				}
				max = []float64{-1, point[1]}
				min = []float64{1000000000, point[1]}
				num = 0
			}
		}
		if num > 0 {
			if data.Target == "ticket_avail_read" || data.Target == "ticket_avail_write" {
				data.DataPoints = append(data.DataPoints, min)
			} else {
				data.DataPoints = append(data.DataPoints, max)
			}
		}
	} else {
		data.DataPoints = points
	}
	return data
}
