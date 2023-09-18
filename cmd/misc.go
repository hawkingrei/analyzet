package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hawkingrei/analyzet/pkg/workload"
)

func checkPrepare(ctx context.Context, w workload.Workloader) {
	// skip preparation check in csv case
	if w.Name() == "tpcc-csv" {
		fmt.Println("Skip preparing checking. Please load CSV data into database and check later.")
		return
	}
	if w.Name() == "tpcc" && tpccConfig.NoCheck {
		return
	}

	var wg sync.WaitGroup
	wg.Add(threads)
	for i := 0; i < threads; i++ {
		go func(index int) {
			defer wg.Done()

			ctx = w.InitThread(ctx, index)
			defer w.CleanupThread(ctx, index)

			if err := w.CheckPrepare(ctx, index); err != nil {
				fmt.Printf("check prepare failed, err %v\n", err)
				return
			}
		}(i)
	}
	wg.Wait()
}

func execute(timeoutCtx context.Context, w workload.Workloader, action string, threads, index int) error {
	count := totalCount / threads

	ctx := w.InitThread(context.Background(), index)
	defer w.CleanupThread(ctx, index)

	switch action {
	case "prepare":
		// Do cleanup only if dropData is set and not generate csv data.
		if dropData {
			if err := w.Cleanup(ctx, index); err != nil {
				return err
			}
		}
		return w.Prepare(ctx, index)
	case "cleanup":
		return w.Cleanup(ctx, index)
	case "check":
		return w.Check(ctx, index)
	}

	for i := 0; i < count || count <= 0; i++ {
		err := w.Run(ctx, index)
		if err != nil {
			if !silence {
				fmt.Printf("[%s] execute %s failed, err %v\n", time.Now().Format("2006-01-02 15:04:05"), action, err)
			}
			if !ignoreError {
				return err
			}
		}
		select {
		case <-timeoutCtx.Done():
			return nil
		default:
		}
	}

	return nil
}

func executeWorkload(ctx context.Context, w workload.Workloader, threads int) {
	var wg sync.WaitGroup
	wg.Add(threads * 2)

	outputCtx, outputCancel := context.WithCancel(ctx)
	ch := make(chan struct{}, 1)
	go func() {
		ticker := time.NewTicker(outputInterval)
		defer ticker.Stop()

		for {
			select {
			case <-outputCtx.Done():
				ch <- struct{}{}
				return
			case <-ticker.C:
				w.OutputStats(false)
			}
		}
	}()
	if w.Name() == "tpch" {
		err := w.Exec(`create or replace view revenue0 (supplier_no, total_revenue) as
	select
		l_suppkey,
		sum(l_extendedprice * (1 - l_discount))
	from
		lineitem
	where
		l_shipdate >= '1997-07-01'
		and l_shipdate < date_add('1997-07-01', interval '3' month)
	group by
		l_suppkey;`)
		if err != nil {
			panic(fmt.Sprintf("a fatal occurred when preparing view data: %v", err))
		}
	}
	enabledDumpPlanReplayer := w.IsPlanReplayerDumpEnabled()
	if enabledDumpPlanReplayer {
		err := w.PreparePlanReplayerDump()
		if err != nil {
			fmt.Printf("[%s] prepare plan replayer failed, err%v\n",
				time.Now().Format("2006-01-02 15:04:05"), err)
		}
		defer func() {
			err = w.FinishPlanReplayerDump()
			if err != nil {
				fmt.Printf("[%s] dump plan replayer failed, err%v\n",
					time.Now().Format("2006-01-02 15:04:05"), err)
			}
		}()
	}

	for i := 0; i < threads; i++ {
		go func(index int) {
			defer wg.Done()
			if err := execute(ctx, w, "ready", threads, index); err != nil {
				panic(fmt.Sprintf("a fatal occurred when preparing data: %v", err))
				return
			}
		}(i)
	}
	time.Sleep(10 * time.Second)
	for i := 0; i < threads; i++ {
		go func(index int) {
			defer wg.Done()
			if err := execute(ctx, w, "run", threads, index); err != nil {
				fmt.Printf("execute %s failed, err %v\n", "run", err)
				return
			}
		}(i)
	}

	wg.Wait()
	checkPrepare(ctx, w)
	outputCancel()

	<-ch
}
