/*
Copyright © 2024 Masayuki Yamai <twsnmp@gmail.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"time"

	"github.com/mattn/go-sixel"
	chart "github.com/wcharczuk/go-chart/v2"
)

func SaveCountTimeChart(path string) {
	x := []time.Time{}
	y := []float64{}
	for _, e := range countList {
		if t, err := time.ParseInLocation("2006/01/02 15:04", e.Key, time.Local); err == nil {
			x = append(x, t)
			y = append(y, float64(e.Count))
		}
	}
	graph := chart.Chart{
		XAxis: chart.XAxis{
			Name:           "Time",
			ValueFormatter: chart.TimeValueFormatterWithFormat("01/02 15:04"),
		},
		YAxis: chart.YAxis{
			Name: "Count",
			ValueFormatter: func(v interface{}) string {
				if vf, isFloat := v.(float64); isFloat {
					return fmt.Sprintf("%d", int64(vf))
				}
				return ""
			},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				XValues: x,
				YValues: y,
			},
		},
	}

	if f, err := os.Create(path); err == nil {
		graph.Render(chart.PNG, f)
	}
}

func SaveCountChart(path string) {
	sort.Slice(countList, func(i, j int) bool {
		return countList[i].Count > countList[j].Count
	})
	value := []chart.Value{}
	other := 0
	for i, e := range countList {
		if i < 10 {
			value = append(value, chart.Value{
				Value: float64(e.Count),
				Label: fmt.Sprintf("%s(%d)", e.Key, e.Count),
			})
		} else {
			other += e.Count
		}
	}
	if other > 0 {
		value = append(value, chart.Value{
			Value: float64(other),
			Label: fmt.Sprintf("Other(%d)", other),
		})
	}
	graph := chart.PieChart{
		Height:     512,
		Width:      512,
		Values:     value,
		SliceStyle: chart.Style{FontSize: 8.0},
	}

	if f, err := os.Create(path); err == nil {
		graph.Render(chart.PNG, f)
	}
}

func SaveRelationChart(path string) {
	sort.Slice(relationList, func(i, j int) bool {
		return relationList[i].Count > relationList[j].Count
	})
	value := []chart.Value{}
	other := 0
	for i, e := range relationList {
		if i < 10 {
			value = append(value, chart.Value{
				Value: float64(e.Count),
				Label: fmt.Sprintf("%s(%d)", e.Key, e.Count),
			})
		} else {
			other += e.Count
		}
	}
	if other > 0 {
		value = append(value, chart.Value{
			Value: float64(other),
			Label: fmt.Sprintf("Other(%d)", other),
		})
	}
	graph := chart.PieChart{
		Height:     512,
		Width:      512,
		Values:     value,
		SliceStyle: chart.Style{FontSize: 8.0},
	}

	if f, err := os.Create(path); err == nil {
		graph.Render(chart.PNG, f)
	}
}

func SaveExtractChart(path string) {
	x := []time.Time{}
	y := []float64{}
	i := 1.0
	cat := make(map[string]float64)
	for _, e := range extractList {
		v, err := strconv.ParseFloat(e.Value, 64)
		if err != nil {
			var ok bool
			v, ok = cat[e.Value]
			if !ok {
				v = i
				cat[e.Value] = i
				i += 1.0
			}
		}
		x = append(x, time.Unix(0, e.Time))
		y = append(y, v)
	}
	graph := chart.Chart{
		XAxis: chart.XAxis{
			Name:           "Time",
			ValueFormatter: chart.TimeValueFormatterWithFormat("01/02 15:04"),
		},
		YAxis: chart.YAxis{
			Name: name,
		},
		Series: []chart.Series{
			chart.TimeSeries{
				XValues: x,
				YValues: y,
			},
		},
	}

	if f, err := os.Create(path); err == nil {
		graph.Render(chart.PNG, f)
	}
}

func SaveDelayTimeChart(path string) {
	x := []time.Time{}
	y := []float64{}
	for _, e := range delayList {
		t := time.Unix(0, e.Time)
		x = append(x, t)
		y = append(y, float64(e.Delay))
	}
	graph := chart.Chart{
		XAxis: chart.XAxis{
			Name:           "Time",
			ValueFormatter: chart.TimeValueFormatterWithFormat("01/02 15:04"),
		},
		YAxis: chart.YAxis{
			Name: "Delay",
			ValueFormatter: func(v interface{}) string {
				if vf, isFloat := v.(float64); isFloat {
					return fmt.Sprintf("%d", int64(vf))
				}
				return ""
			},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				XValues: x,
				YValues: y,
			},
		},
	}

	if f, err := os.Create(path); err == nil {
		graph.Render(chart.PNG, f)
	}
}

func SaveDeltaTimeChart(path string) {
	x := []time.Time{}
	y := []float64{}
	for _, e := range timeList {
		t := time.Unix(0, e.Time)
		x = append(x, t)
		y = append(y, float64(e.Delta))
	}
	graph := chart.Chart{
		XAxis: chart.XAxis{
			Name: "Time",
			NameStyle: chart.Style{
				FontSize: 8,
			},
			ValueFormatter: chart.TimeValueFormatterWithFormat("01/02 15:04"),
			Style:          chart.Style{FontSize: 6},
		},
		YAxis: chart.YAxis{
			Name: "Delta",
			NameStyle: chart.Style{
				FontSize: 8,
			},
			ValueFormatter: func(v interface{}) string {
				if vf, isFloat := v.(float64); isFloat {
					return (time.Duration(int64(vf*1000)) * time.Millisecond).String()
				}
				return ""
			},
			Style: chart.Style{FontSize: 6, Padding: chart.Box{Top: 0, Left: 10, IsSet: false}},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				YAxis:   chart.YAxisPrimary,
				XValues: x,
				YValues: y,
			},
		},
	}
	if f, err := os.Create(path); err == nil {
		graph.Render(chart.PNG, f)
	}
}

func openChart(path string) error {
	var err error
	url := "file:///" + path
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	return err
}

func openChartSixel(path string) string {
	fp, err := os.Open(path)
	if err != nil {
		return ""
	}
	img, _, err := image.Decode(fp)
	if err != nil {
		return ""
	}
	var buf bytes.Buffer
	sixel.NewEncoder(&buf).Encode(img)
	return buf.String()
}
