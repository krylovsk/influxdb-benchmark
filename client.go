package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"strconv"
	"time"

	"github.com/GaryBoone/GoStats/stats"
	influx "github.com/influxdb/influxdb/client"
)

type Client struct {
	ID           int
	MsgCount     int
	BatchSize    int
	Database     string
	influxClient *influx.Client
	config       *influx.Config
}

func NewClient(url url.URL, user, pass, db string, id, count, bs int) (*Client, error) {
	c := &Client{
		ID:        id,
		Database:  db,
		MsgCount:  count,
		BatchSize: bs,
	}

	c.config = &influx.Config{
		URL:      url,
		Username: user,
		Password: pass,
	}
	var err error
	c.influxClient, err = influx.NewClient(*c.config)
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
	bps := &influx.BatchPoints{
		Database:        c.Database,
		RetentionPolicy: "default",
		Points:          make([]influx.Point, c.BatchSize),
	}

	bi := 0 // batch index
	for i := 0; i < c.MsgCount; i++ {
		bps.Points[bi] = influx.Point{
			Measurement: fmt.Sprintf("influxdb-benchmark-%d", c.ID),
			Tags: map[string]string{
				"client_tag": strconv.Itoa(rand.Intn(c.ID + 1)),
			},
			Fields: map[string]interface{}{
				"int_value":   rand.Int(),
				"float_value": rand.Float64(),
				"bool_balue":  rand.Intn(2) == 1,
			},
			Time:      time.Now(),
			Precision: "u",
		}
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
			_, err := c.influxClient.Write(*m.Batch)
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
