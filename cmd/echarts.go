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
		charts.WithTitleOpts(opts.Title{Title: "TWSLA Count"}),
	)
	line.SetXAxis(nil).
		AddSeries("Count", items)

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
	var f = false
	pie.SetGlobalOptions(
		charts.WithLegendOpts(opts.Legend{Show: &f}),
		charts.WithTitleOpts(opts.Title{
			Title: "TWSLA Log count",
		}),
	)
	pie.AddSeries("Count", items)
	if f, err := os.Create(path); err == nil {
		pie.Render(f)
	}
}
