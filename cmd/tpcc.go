package main

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"time"

	"github.com/hawkingrei/analyzet/pkg/measurement"
	"github.com/hawkingrei/analyzet/pkg/workload"
	"github.com/hawkingrei/analyzet/tpcc"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
)

var tpccConfig tpcc.Config

func executeTpcc() {
	if pprofAddr != "" {
		go func() {
			if err := http.ListenAndServe(pprofAddr, http.DefaultServeMux); err != nil {
				fmt.Printf("Failed to listen pprofAddr: %v\n", err)
				os.Exit(1)
			}
		}()
	}
	if metricsAddr != "" {
		go func() {
			s := http.Server{
				Addr:    metricsAddr,
				Handler: promhttp.Handler(),
			}
			if err := s.ListenAndServe(); err != nil {
				fmt.Printf("Failed to listen metricsAddr: %v\n", err)
				os.Exit(1)
			}
		}()
	}
	if maxProcs != 0 {
		runtime.GOMAXPROCS(maxProcs)
	}

	openDB()
	defer closeDB()

	tpccConfig.OutputStyle = outputStyle
	tpccConfig.Driver = driver
	tpccConfig.DBName = dbName
	tpccConfig.Threads = threads
	tpccConfig.Isolation = isolationLevel
	var (
		w   workload.Workloader
		err error
	)
	w, err = tpcc.NewWorkloader(globalDB, &tpccConfig)

	if err != nil {
		fmt.Printf("Failed to init work loader: %v\n", err)
		os.Exit(1)
	}

	timeoutCtx, cancel := context.WithTimeout(globalCtx, totalTime)
	defer cancel()

	executeWorkload(timeoutCtx, w, threads)

	fmt.Println("Finished")
	w.OutputStats(true)
}

func registerTpcc(root *cobra.Command) {
	cmd := &cobra.Command{
		Use: "tpcc",
		Run: func(cmd *cobra.Command, args []string) {
			executeTpcc()
		},
	}

	cmd.PersistentFlags().IntVar(&tpccConfig.Parts, "parts", 1, "Number to partition warehouses")
	cmd.PersistentFlags().IntVar(&tpccConfig.PartitionType, "partition-type", 1, "Partition type (1 - HASH, 2 - RANGE, 3 - LIST (like HASH), 4 - LIST (like RANGE)")
	cmd.PersistentFlags().IntVar(&tpccConfig.Warehouses, "warehouses", 10, "Number of warehouses")
	cmd.PersistentFlags().BoolVar(&tpccConfig.CheckAll, "check-all", false, "Run all consistency checks")
	cmd.PersistentFlags().BoolVar(&tpccConfig.NoCheck, "no-check", false, "TPCC prepare check, default false")
	cmd.PersistentFlags().StringVar(&tpccConfig.OutputType, "output-type", "", "Output file type."+
		" If empty, then load data to db. Current only support csv")
	cmd.PersistentFlags().StringVar(&tpccConfig.OutputDir, "output-dir", "", "Output directory for generating file if specified")
	cmd.PersistentFlags().StringVar(&tpccConfig.SpecifiedTables, "tables", "", "Specified tables for "+
		"generating file, separated by ','. Valid only if output is set. If this flag is not set, generate all tables by default")
	cmd.PersistentFlags().IntVar(&tpccConfig.PrepareRetryCount, "retry-count", 50, "Retry count when errors occur")
	cmd.PersistentFlags().DurationVar(&tpccConfig.PrepareRetryInterval, "retry-interval", 10*time.Second, "The interval for each retry")

	cmd.PersistentFlags().BoolVar(&tpccConfig.Wait, "wait", false, "including keying & thinking time described on TPC-C Standard Specification")
	cmd.PersistentFlags().DurationVar(&tpccConfig.MaxMeasureLatency, "max-measure-latency", measurement.DefaultMaxLatency, "max measure latency in millisecond")
	cmd.PersistentFlags().IntSliceVar(&tpccConfig.Weight, "weight", []int{45, 43, 4, 4, 4}, "Weight for NewOrder, Payment, OrderStatus, Delivery, StockLevel")

	root.AddCommand(cmd)
}
