// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Render the benchmark output from make bench-ci as a GitHub job summary.
package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

func main() {
	const samples = 10
	names := []string{
		"SPSCSessionHandoff/Queue", "SPSCSessionHandoff/Channel", "SPSCSessionFullRetry",
		"SPSCIndirect_SingleOp", "MPSCIndirect_SingleOp", "SPMCIndirect_SingleOp", "MPMCIndirect_SingleOp",
		"SPMCIndirectSeq_ParallelConsumers",
	}
	results := make(map[string]map[string][]float64)
	for _, name := range names {
		results[name] = make(map[string][]float64)
	}

	var host []string
	goos, goarch := runtime.GOOS, runtime.GOARCH
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		line := in.Text()
		switch {
		case strings.HasPrefix(line, "goos: "):
			goos = strings.TrimPrefix(line, "goos: ")
		case strings.HasPrefix(line, "goarch: "):
			goarch = strings.TrimPrefix(line, "goarch: ")
		case strings.HasPrefix(line, "Linux "), strings.HasPrefix(line, "go version "), strings.HasPrefix(line, "cpu: "):
			host = append(host, line)
		case strings.HasPrefix(line, "Benchmark"):
			fields := strings.Fields(line)
			name := strings.TrimSuffix(strings.TrimPrefix(fields[0], "Benchmark"), "-2")
			metrics, ok := results[name]
			if !ok {
				continue
			}
			for i := 2; i+1 < len(fields); i += 2 {
				value, err := strconv.ParseFloat(fields[i], 64)
				if err != nil {
					log.Fatal(err)
				}
				unit := fields[i+1]
				metrics[unit] = append(metrics[unit], value)
			}
		}
	}
	if err := in.Err(); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("### Queue benchmarks (%s/%s)\n\n", goos, goarch)
	for _, line := range host {
		fmt.Printf("- `%s`\n", line)
	}
	fmt.Printf("\nMedian of %d samples with `-benchtime=200ms` and `GOMAXPROCS=2`. ", samples)
	fmt.Println("`SingleOp` measures one enqueue/dequeue pair. `waits/op` counts backoff waits on both sides of a handoff.")
	fmt.Println()
	fmt.Println("| Benchmark | ns/op | B/op | allocs/op | waits/op |")
	fmt.Println("| --- | ---: | ---: | ---: | ---: |")
	complete := true
	for _, name := range names {
		fmt.Printf("| `%s` |", name)
		for _, unit := range []string{"ns/op", "B/op", "allocs/op", "waits/op"} {
			if unit == "waits/op" && !strings.HasPrefix(name, "SPSCSessionHandoff/") {
				fmt.Print(" — |")
				continue
			}
			values := results[name][unit]
			if len(values) != samples {
				fmt.Printf(" %d/%d samples |", len(values), samples)
				complete = false
				continue
			}
			slices.Sort(values)
			fmt.Printf(" %.6g |", (values[(samples-1)/2]+values[samples/2])/2)
		}
		fmt.Println()
	}
	if !complete {
		fmt.Println("\nIncomplete measurements. See the raw benchmark output in the workflow artifacts.")
		os.Exit(1)
	}
}
