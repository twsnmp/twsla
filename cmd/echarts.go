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
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
)

// SaveCountTimeECharts is save counter time chart by go-echarts
func SaveCountTimeECharts(path string) {
	items := []opts.LineData{}
	for _, e := range countList {
		if t, err := time.ParseInLocation("2006/01/02 15:04", e.Key, time.Local); err == nil {
			items = append(items, opts.LineData{Value: []interface{}{t.UnixMilli(), e.Count}})
		}
	}
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithXAxisOpts(opts.XAxis{Name: "time", Type: "time"}, 0),
		charts.WithTitleOpts(opts.Title{Title: "TWSLA Count:" + nameCount}),
		charts.WithDataZoomOpts(opts.DataZoom{}),
	)
	line.SetXAxis(nil).
		AddSeries(nameCount, items)

	if f, err := os.Create(path); err == nil {
		line.Render(f)
	}
}

func SaveCountECharts(path string) {
	sort.Slice(countList, func(i, j int) bool {
		return countList[i].Count > countList[j].Count
	})
	items := make([]opts.PieData, 0)
	other := 0
	for i, e := range countList {
		if i < 10 {
			items = append(items, opts.PieData{Name: e.Key, Value: e.Count})
		} else {
			other += e.Count
		}
	}
	if other > 0 {
		items = append(items, opts.PieData{Name: "Other", Value: other})
	}

	pie := charts.NewPie()
	pie.SetGlobalOptions(
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(false)}),
		charts.WithTitleOpts(opts.Title{
			Title: "TWSLA Log count:" + nameCount,
		}),
	)
	pie.AddSeries(nameCount, items)
	if f, err := os.Create(path); err == nil {
		pie.Render(f)
	}
}

func SaveRelationECharts(path string) {
	nodeMap := make(map[string]bool)
	nodes := []opts.GraphNode{}
	links := []opts.GraphLink{}
	graph := charts.NewGraph()
	for _, e := range relationList {
		for i, v := range e.Values {
			if _, ok := nodeMap[v]; !ok {
				nodeMap[v] = true
				nodes = append(nodes, opts.GraphNode{
					Name:      v,
					ItemStyle: &opts.ItemStyle{Color: graph.Colors[i%10]},
				})
			}
			if i > 0 {
				links = append(links, opts.GraphLink{
					Source: e.Values[i-1],
					Target: e.Values[i],
				})
			}
		}
	}
	graph.SetGlobalOptions(
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(false)}),
		charts.WithTitleOpts(opts.Title{Title: "TWSLA relation graph"}),
	)
	graph.AddSeries("graph", nodes, links,
		charts.WithGraphChartOpts(opts.GraphChart{
			Layout:             "circular",
			Roam:               opts.Bool(true),
			FocusNodeAdjacency: opts.Bool(true),
		}),
		charts.WithLabelOpts(opts.Label{Show: opts.Bool(true), Position: "right"}),
	)
	if f, err := os.Create(path); err == nil {
		graph.Render(f)
	}
}

func SaveExtractECharts(path string) {
	items := []opts.LineData{}
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithXAxisOpts(opts.XAxis{Name: "time", Type: "time"}, 0),
		charts.WithTitleOpts(opts.Title{Title: "TWSLA Extract:" + nameExtract}),
		charts.WithDataZoomOpts(opts.DataZoom{}),
	)

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
		items = append(items, opts.LineData{Value: []interface{}{e.Time / (1000 * 1000), v}})
	}
	line.SetXAxis(nil).AddSeries(nameExtract, items)
	if f, err := os.Create(path); err == nil {
		line.Render(f)
	}
}

func SaveDelayTimeECharts(path string) {
	items := []opts.LineData{}
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithXAxisOpts(opts.XAxis{Name: "time", Type: "time"}, 0),
		charts.WithTitleOpts(opts.Title{Title: "TWSLA Delay"}),
		charts.WithDataZoomOpts(opts.DataZoom{}),
	)
	for _, e := range delayList {
		items = append(items, opts.LineData{Value: []interface{}{e.Time / (1000 * 1000), e.Delay}})
	}
	line.SetXAxis(nil).AddSeries("Delay", items)
	if f, err := os.Create(path); err == nil {
		line.Render(f)
	}
}

var weekDays = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

var dayHrs = [...]string{
	"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11",
	"12", "13", "14", "15", "16", "17", "18", "19", "20", "21", "22", "23",
}

func saveHeatmapECharts(path string) {
	items := make([]opts.HeatMapData, 0)
	max := 10
	for _, r := range heatmapList {
		items = append(items, opts.HeatMapData{Value: [3]interface{}{r.X, r.Y, r.Count}})
		if max < r.Count {
			max = r.Count
		}
	}
	xAxis := dateList
	if week {
		xAxis = weekDays
	}
	hm := charts.NewHeatMap()
	hm.SetGlobalOptions(
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(false)}),
		charts.WithTitleOpts(opts.Title{
			Title: "TWSLA Heatmap",
		}),
		charts.WithXAxisOpts(opts.XAxis{
			Type:      "category",
			Data:      xAxis,
			SplitArea: &opts.SplitArea{Show: opts.Bool(true)},
		}),
		charts.WithYAxisOpts(opts.YAxis{
			Type:      "category",
			Data:      dayHrs,
			SplitArea: &opts.SplitArea{Show: opts.Bool(true)},
		}),
		charts.WithVisualMapOpts(opts.VisualMap{
			Calculable: opts.Bool(true),
			Min:        0,
			Max:        float32(max),
			InRange: &opts.VisualMapInRange{
				Color: []string{"#50a3ba", "#eac736", "#d94e5d"},
			},
		}),
	)
	hm.AddSeries("heatmap", items)
	if f, err := os.Create(path); err == nil {
		hm.Render(f)
	}
}

func SaveDeltaTimeECharts(path string) {
	y := []opts.LineData{}
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithXAxisOpts(opts.XAxis{Name: "time", Type: "time"}, 0),
		charts.WithTitleOpts(opts.Title{Title: "TWSLA Delta Time"}),
		charts.WithDataZoomOpts(opts.DataZoom{}),
		charts.WithYAxisOpts(opts.YAxis{Name: "Delta(Sec)", Type: "value"}, 0),
	)
	for _, e := range timeList {
		y = append(y, opts.LineData{Value: []interface{}{e.Time / (1000 * 1000), e.Delta}, YAxisIndex: 1})
	}
	line.SetXAxis(nil)
	line.AddSeries("Delta", y)
	if f, err := os.Create(path); err == nil {
		line.Render(f)
	}
}
