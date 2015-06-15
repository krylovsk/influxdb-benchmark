package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/url"
	"sort"
	"time"

	"github.com/GaryBoone/GoStats/stats"
	influx "github.com/influxdb/influxdb/client"
)

// Message describes a data point
type Message struct {
	Batch     *influx.BatchPoints
	Sent      time.Time
	Delivered time.Time
	Error     bool
}

// RunResults describes results of a single client / run
type RunResults struct {
	ID          int     `json:"id"`
	Successes   int64   `json:"successes"`
	Failures    int64   `json:"failures"`
	RunTime     float64 `json:"run_time"`
	MsgTimeMin  float64 `json:"msg_time_min"`
	MsgTimeMax  float64 `json:"msg_time_max"`
	MsgTimeMean float64 `json:"msg_time_mean"`
	MgsTimePs25 float64 `json:"msg_time_ps25"`
	MgsTimePs50 float64 `json:"msg_time_ps50"`
	MgsTimePs95 float64 `json:"msg_time_ps95"`
	MsgTimeStd  float64 `json:"msg_time_std"`
	MsgsPerSec  float64 `json:"msgs_per_sec"`
}

// TotalResults describes results of all clients / runs
type TotalResults struct {
	Ratio           float64 `json:"ratio"`
	Successes       int64   `json:"successes"`
	Failures        int64   `json:"failures"`
	TotalRunTime    float64 `json:"total_run_time"`
	AvgRunTime      float64 `json:"avg_run_time"`
	MsgTimeMin      float64 `json:"msg_time_min"`
	MsgTimeMax      float64 `json:"msg_time_max"`
	MsgTimeMeanAvg  float64 `json:"msg_time_mean_avg"`
	MsgTimeMeanStd  float64 `json:"msg_time_mean_std"`
	MgsTimeMeanPs25 float64 `json:"msg_time_mean_ps25"`
	MgsTimeMeanPs50 float64 `json:"msg_time_mean_ps50"`
	MgsTimeMeanPs95 float64 `json:"msg_time_mean_ps95"`
	TotalMsgsPerSec float64 `json:"total_msgs_per_sec"`
	AvgMsgsPerSec   float64 `json:"avg_msgs_per_sec"`
	Ps25MgssPerSec  float64 `json:"ps25_msgs_per_sec"`
	Ps50MgssPerSec  float64 `json:"ps50_msgs_per_sec"`
	Ps95MgssPerSec  float64 `json:"ps95_msgs_per_sec"`
}

// JSONResults are used to export results as a JSON document
type JSONResults struct {
	Runs   []*RunResults `json:"runs"`
	Totals *TotalResults `json:"totals"`
}

func main() {
	var (
		server    = flag.String("server", "http://localhost:8086", "InfluxDB server endpoint as scheme://host:port")
		username  = flag.String("username", "", "InfluxDB username (empty if auth disabled)")
		password  = flag.String("password", "", "InfluxDB password (empty if auth disabled)")
		database  = flag.String("database", "benchmarking", "InfluxDB database (will be created/cleaned if --clean)")
		clean     = flag.Bool("clean", true, "Whether to clean (create a new) DB before starting")
		count     = flag.Int("count", 100, "Number of messages to send per client")
		batchsize = flag.Int("batch", 1, "Number of data points to submit at once (1 means no batching)")
		clients   = flag.Int("clients", 10, "Number of clients to start")
		format    = flag.String("format", "text", "Output format: text|json")
	)

	flag.Parse()
	if *clients < 1 {
		log.Fatal("Number of clients should be >= 1")
	}
	url, err := url.Parse(*server)
	if err != nil {
		log.Fatal("Invalid server URL")
	}
	if *batchsize < 1 || *batchsize > *count {
		log.Fatal("Batch size should be >= 1 and <= count")
	}
	if *database == "" {
		log.Fatal("Database should be provided")
	}

	if *clean {
		cleanData(url, *username, *password, *database)
	}

	resCh := make(chan *RunResults)
	start := time.Now()
	for i := 0; i < *clients; i++ {
		log.Println("Starting client ", i)
		c, err := NewClient(*url, *username, *password, *database, i, *count, *batchsize)
		if err != nil {
			log.Fatalf("Error connecting to server %v: %v", url.String(), err)
		}
		go c.Run(resCh)
	}

	// collect the results
	results := make([]*RunResults, *clients)
	for i := 0; i < *clients; i++ {
		results[i] = <-resCh
	}
	totalTime := time.Now().Sub(start)
	totals := calculateTotalResults(results, totalTime)

	// print stats
	printResults(results, totals, *format)
}

func calculateTotalResults(results []*RunResults, totalTime time.Duration) *TotalResults {
	totals := new(TotalResults)
	totals.TotalRunTime = totalTime.Seconds()

	msgTimeMeans := make([]float64, len(results))
	msgsPerSecs := make([]float64, len(results))
	runTimes := make([]float64, len(results))
	bws := make([]float64, len(results))

	totals.MsgTimeMin = results[0].MsgTimeMin
	for i, res := range results {
		totals.Successes += res.Successes
		totals.Failures += res.Failures
		totals.TotalMsgsPerSec += res.MsgsPerSec

		if res.MsgTimeMin < totals.MsgTimeMin {
			totals.MsgTimeMin = res.MsgTimeMin
		}

		if res.MsgTimeMax > totals.MsgTimeMax {
			totals.MsgTimeMax = res.MsgTimeMax
		}

		msgTimeMeans[i] = res.MsgTimeMean
		msgsPerSecs[i] = res.MsgsPerSec
		runTimes[i] = res.RunTime
		bws[i] = res.MsgsPerSec
	}
	totals.Ratio = float64(totals.Successes) / float64(totals.Successes+totals.Failures)
	totals.AvgMsgsPerSec = stats.StatsMean(msgsPerSecs)
	totals.AvgRunTime = stats.StatsMean(runTimes)
	totals.MsgTimeMeanAvg = stats.StatsMean(msgTimeMeans)
	totals.MsgTimeMeanStd = stats.StatsSampleStandardDeviation(msgTimeMeans)
	totals.MgsTimeMeanPs25 = percentile(msgTimeMeans, 25.0)
	totals.MgsTimeMeanPs50 = percentile(msgTimeMeans, 50.0)
	totals.MgsTimeMeanPs95 = percentile(msgTimeMeans, 95.0)
	totals.Ps25MgssPerSec = percentile(msgsPerSecs, 25.0)
	totals.Ps50MgssPerSec = percentile(msgsPerSecs, 50.0)
	totals.Ps95MgssPerSec = percentile(msgsPerSecs, 95.0)
	return totals
}

func printResults(results []*RunResults, totals *TotalResults, format string) {
	switch format {
	case "json":
		jr := JSONResults{
			Runs:   results,
			Totals: totals,
		}
		data, _ := json.Marshal(jr)
		var out bytes.Buffer
		json.Indent(&out, data, "", "\t")

		fmt.Println(string(out.Bytes()))
	default:
		for _, res := range results {
			fmt.Printf("======= CLIENT %d =======\n", res.ID)
			fmt.Printf("Ratio:                 %.3f (%d/%d)\n", float64(res.Successes)/float64(res.Successes+res.Failures), res.Successes, res.Successes+res.Failures)
			fmt.Printf("Runtime (s):           %.3f\n", res.RunTime)
			fmt.Printf("Msg time min (ms):     %.3f\n", res.MsgTimeMin)
			fmt.Printf("Msg time max (ms):     %.3f\n", res.MsgTimeMax)
			fmt.Printf("Msg time mean (ms):    %.3f\n", res.MsgTimeMean)
			fmt.Printf("Msg time std (ms):     %.3f\n", res.MsgTimeStd)
			fmt.Printf("Msg time ps 25p (ms):  %.3f\n", res.MgsTimePs25)
			fmt.Printf("Msg time ps 50p (ms):  %.3f\n", res.MgsTimePs50)
			fmt.Printf("Msg time ps 95p (ms):  %.3f\n", res.MgsTimePs95)
			fmt.Printf("Bandwidth (msg/sec):   %.3f\n\n", res.MsgsPerSec)
		}
		fmt.Printf("========= TOTAL (%d) =========\n", len(results))
		fmt.Printf("Total Ratio:                 %.3f (%d/%d)\n", totals.Ratio, totals.Successes, totals.Successes+totals.Failures)
		fmt.Printf("Total Runtime (sec):         %.3f\n", totals.TotalRunTime)
		fmt.Printf("Average Runtime (sec):       %.3f\n", totals.AvgRunTime)
		fmt.Printf("Msg time min (ms):           %.3f\n", totals.MsgTimeMin)
		fmt.Printf("Msg time max (ms):           %.3f\n", totals.MsgTimeMax)
		fmt.Printf("Msg time mean mean (ms):     %.3f\n", totals.MsgTimeMeanAvg)
		fmt.Printf("Msg time mean std (ms):      %.3f\n", totals.MsgTimeMeanStd)
		fmt.Printf("Msg time mean ps 25p (ms):   %.3f\n", totals.MgsTimeMeanPs25)
		fmt.Printf("Msg time mean ps 50p (ms):   %.3f\n", totals.MgsTimeMeanPs50)
		fmt.Printf("Msg time mean ps 95p (ms):   %.3f\n", totals.MgsTimeMeanPs95)
		fmt.Printf("Average Bandwidth (msg/sec): %.3f\n", totals.AvgMsgsPerSec)
		fmt.Printf("Total Bandwidth (msg/sec):   %.3f\n", totals.TotalMsgsPerSec)
	}
	return
}

func cleanData(url *url.URL, user, pass, db string) {
	fmt.Println("Cleaning benchmarking data on server ", url)
	clientConfig := influx.Config{
		URL:      *url,
		Username: user,
		Password: pass,
	}
	c, err := influx.NewClient(clientConfig)
	if err != nil {
		log.Fatalf("Error connecting to server %v: %v", url.String(), err)
	}

	c.Query(influx.Query{
		Command:  fmt.Sprintf("drop database %s", db),
		Database: db,
	})
	c.Query(influx.Query{
		Command:  fmt.Sprintf("create database %s", db),
		Database: db,
	})
}

// https://github.com/influxdb/influxdb/blob/master/influxql/functions.go#L1070
func percentile(values []float64, ps float64) float64 {
	sort.Float64s(values)
	length := len(values)
	index := int(math.Floor(float64(length)*ps/100.0+0.5)) - 1

	if index < 0 || index >= len(values) {
		return 0
	}

	return values[index]
}
