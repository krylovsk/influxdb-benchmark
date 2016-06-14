package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"strconv"
	"time"

	"github.com/GaryBoone/GoStats/stats"
	influx "github.com/influxdata/influxdb/client/v2"
)

type Client struct {
	ID           int
	MsgCount     int
	BatchSize    int
	Database     string
	influxClient influx.Client
	config       *influx.HTTPConfig
}

func NewClient(url url.URL, user, pass, db string, id, count, bs int) (*Client, error) {
	c := &Client{
		ID:        id,
		Database:  db,
		MsgCount:  count,
		BatchSize: bs,
	}

	c.config = &influx.HTTPConfig{
		Addr:     url.String(),
		Username: user,
		Password: pass,
	}
	var err error
	c.influxClient, err = influx.NewHTTPClient(*c.config)
	return c, err
}

func (c *Client) Run(res chan *RunResults) {
	newMsgs := make(chan *Message)
	subMsgs := make(chan *Message)
	doneGen := make(chan bool)
	donePub := make(chan bool)
	runResults := new(RunResults)

	started := time.Now()
	// start generator
	go c.genMessages(newMsgs, doneGen)
	// start publisher
	go c.pubMessages(newMsgs, subMsgs, doneGen, donePub)

	runResults.ID = c.ID
	times := []float64{}
	for {
		select {
		case m := <-subMsgs:
			if m.Error {
				runResults.Failures++
			} else {
				runResults.Successes++
				times = append(times, m.Delivered.Sub(m.Sent).Seconds()*1000) // in milliseconds
			}
		case <-donePub:
			// calculate results
			duration := time.Now().Sub(started)
			runResults.MsgTimeMin = stats.StatsMin(times)
			runResults.MsgTimeMax = stats.StatsMax(times)
			runResults.MsgTimeMean = stats.StatsMean(times)
			runResults.MsgTimeStd = stats.StatsSampleStandardDeviation(times)
			runResults.MgsTimePs25 = percentile(times, 25.0)
			runResults.MgsTimePs50 = percentile(times, 50.0)
			runResults.MgsTimePs95 = percentile(times, 95.0)
			runResults.RunTime = duration.Seconds()
			runResults.MsgsPerSec = float64(runResults.Successes) / duration.Seconds()

			// report results and exit
			res <- runResults
			return
		}
	}
}

func (c *Client) genMessages(ch chan *Message, done chan bool) {
	bpsConf := influx.BatchPointsConfig{
		Database: c.Database,
	}
	bps, err := influx.NewBatchPoints(bpsConf)
	if err != nil {
		fmt.Println("Error creating batch points: ", err.Error())
		return
	}

	bi := 0 // batch index
	for i := 0; i < c.MsgCount; i++ {
		tags := map[string]string{
			"client_tag": strconv.Itoa(rand.Intn(c.ID + 1)),
		}
		fields := map[string]interface{}{
			"value": rand.Float64(),
		}
		p, err := influx.NewPoint(fmt.Sprintf("influxdb-benchmark-%d", c.ID), tags, fields, time.Now())
		if err != nil {
			fmt.Println("Error creating data point: ", err.Error())
			continue
		}
		bps.AddPoint(p)

		bi++
		// submit the batch and reset index
		if bi == c.BatchSize {
			ch <- &Message{
				Batch: bps,
			}
			bi = 0
		}
	}
	done <- true
	// log.Printf("CLIENT %v is done generating messages\n", c.ID)
	return
}

func (c *Client) pubMessages(in, out chan *Message, doneGen, donePub chan bool) {
	for {
		ctr := 0
		select {
		case m := <-in:
			m.Sent = time.Now()
			err := c.influxClient.Write(m.Batch)
			if err != nil {
				log.Printf("CLIENT %v Error submitting data: %v\n", c.ID, err.Error())
				m.Error = true
			} else {
				m.Delivered = time.Now()
				m.Error = false
			}

			out <- m

			if ctr > 0 && ctr%100 == 0 {
				log.Printf("CLIENT %v submitted %v messages and keeps going...\n", c.ID, ctr)
			}
			ctr++
		case <-doneGen:
			donePub <- true
			log.Printf("CLIENT %v is done submitting data\n", c.ID)
			return
		}
	}
}
