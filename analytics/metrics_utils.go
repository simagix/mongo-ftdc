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
	if len(points) > 720 {
		denom := len(points) / 720
		sum := 0.0
		num := 0.0
		for i := 0; i < len(points); i++ {
			sum += points[i][0]
			num++
			if i%denom == 0 {
				data.DataPoints = append(data.DataPoints, []float64{sum / num, points[i][1]})
				sum = 0
				num = 0
			}
		}
		if num > 0 {
			data.DataPoints = append(data.DataPoints, []float64{sum / num, points[len(points)-1][1]})
		}
	} else {
		data.DataPoints = points
	}
	return data
}
