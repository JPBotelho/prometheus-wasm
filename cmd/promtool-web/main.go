// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"

	"fmt"

	"syscall/js"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/tsdb"
)

func main() {
	js.Global().Set("EphemeralQuery", EphemeralQueryWrapper())
	<-make(chan bool)
}

type HeadQueryable struct {
	head *tsdb.Head
}

func (hq *HeadQueryable) Querier(mint, maxt int64) (storage.Querier, error) {
	// Wrap the Head in a block querier
	return tsdb.NewBlockQuerier(hq.head, mint, maxt)
}

type NoopWAL struct{}

func (NoopWAL) Log(...interface{}) error { return nil }
func (NoopWAL) Close() error             { return nil }
func NewInMemoryQueryable() (*tsdb.Head, storage.Queryable, error) {
	head, err := tsdb.NewHead(nil, nil, nil, nil, tsdb.DefaultHeadOptions(), nil)
	if err != nil {
		return nil, nil, err
	}
	return head, &HeadQueryable{head: head}, nil
}

func EphemeralQuery() {
	tstart := time.Now()
	// Create ephemeral in-memory storage
	head, ts, err := NewInMemoryQueryable()
	if err != nil {
		fmt.Println(err.Error())
		fmt.Println("Failed to create storage")
	}

	// Add some sample data: 0, 5, 10 for http_requests_total
	app := head.Appender(context.Background())

	metric := labels.FromStrings("__name__", "http_requests_total")
	now := time.Now()

	samples := []float64{0, 5, 10}
	for i, v := range samples {
		tsMillis := now.Add(time.Duration(i) * time.Second).UnixMilli()
		_, err := app.Append(0, metric, tsMillis, v)
		if err != nil {
			fmt.Println(err)
		}
	}

	if err := app.Commit(); err != nil {
		fmt.Println(err)
	}

	// Create PromQL engine
	engine := promql.NewEngine(promql.EngineOpts{
		MaxSamples:    10000,
		Timeout:       5 * time.Second,
		LookbackDelta: 5 * time.Minute,
	})

	queryStr := `http_requests_total`
	start := now
	end := now.Add(3 * time.Second)

	rangeQry, err := engine.NewRangeQuery(
		context.Background(),
		ts,
		nil,
		queryStr,
		start,
		end,
		time.Second,
	)
	if err != nil {
		fmt.Println("Range query creation error: %v", err)
	}

	res := rangeQry.Exec(context.Background())
	if res.Err != nil {
		fmt.Println("Range query error: %v", res.Err)
	}

	fmt.Println("Range Query result:", res.Value)

	// Clean up
	fmt.Printf("Program execution time: %v\n", time.Since(tstart))
}

// ** wasm **
func EphemeralQueryWrapper() js.Func {
	CheckFunc := js.FuncOf(func(this js.Value, args []js.Value) any {

		EphemeralQuery()
		return nil
	})
	return CheckFunc
}
