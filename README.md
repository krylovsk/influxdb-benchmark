InfluxDB benchmarking tool
=========
A simple InfluxDB (>=0.13) benchmarking tool.

Installation:

```
go get github.com/krylovsk/influxdb-benchmark
```

All dependencies are vendored with [manul](https://github.com/kovetskiy/manul).

The tool supports multiple concurrent clients and configurable batch size:
```
> influxdb-benchmark --help
Usage of influxdb-benchmark:
  -batch=1: Number of data points to submit at once (1 means no batching)
  -clean=true: Whether to clean (create a new) DB before starting
  -clients=10: Number of clients to start
  -count=100: Number of messages to send per client
  -database="benchmarking": InfluxDB database (will be created/cleaned if --clean)
  -format="text": Output format: text|json
  -password="": InfluxDB password (empty if auth disabled)
  -server="http://localhost:8086": InfluxDB server endpoint as scheme://host:port
  -username="": InfluxDB username (empty if auth disabled)
```

Two output formats supported: human-readable plain text and JSON.

Example use and output:

```
> influxdb-benchmark --count 100 --clients 10 --batch 10 --clean --format=text
....
======= CLIENT 7 =======
Ratio:                 1.000 (10/10)
Runtime (s):           1.092
Msg time min (ms):     1.287
Msg time max (ms):     1011.427
Msg time mean (ms):    109.009
Msg time std (ms):     317.138
Msg time ps 25p (ms):  2.676
Msg time ps 50p (ms):  6.931
Msg time ps 95p (ms):  1011.427
Bandwidth (msg/sec):   9.153

========= TOTAL (10) =========
Total Ratio:                 1.000 (100/100)
Total Runtime (sec):         1.093
Average Runtime (sec):       1.086
Msg time min (ms):           1.175
Msg time max (ms):           1017.344
Msg time mean mean (ms):     108.419
Msg time mean std (ms):      0.863
Msg time mean ps 25p (ms):   108.402
Msg time mean ps 50p (ms):   108.658
Msg time mean ps 95p (ms):   109.062
Average Bandwidth (msg/sec): 9.211
Total Bandwidth (msg/sec):   92.113
```

Similarly, in JSON:

```
> influxdb-benchmark --count 100 --clients 10 --batch 10 --clean --format=json
{
    runs: [
      ...
      {
        "id": 6,
        "successes": 10,
        "failures": 0,
        "run_time": 1.116949391,
        "msg_time_min": 1.798685,
        "msg_time_max": 1011.316071,
        "msg_time_mean": 111.41071449999997,
        "msg_time_ps25": 8.087942,
        "msg_time_ps50": 11.621392000000002,
        "msg_time_ps95": 1011.316071,
        "msg_time_std": 316.23294862522374,
        "msgs_per_sec": 8.952957117463527
      }
    ],
    "totals": {
      "ratio": 1,
      "successes": 100,
      "failures": 0,
      "total_run_time": 1.117257643,
      "avg_run_time": 1.1079248436000002,
      "msg_time_min": 1.356745,
      "msg_time_max": 1013.3505410000001,
      "msg_time_mean_avg": 110.55490952999999,
      "msg_time_mean_std": 1.1110282011556343,
      "msg_time_mean_ps25": 109.78193140000005,
      "msg_time_mean_ps50": 111.20193829999997,
      "msg_time_mean_ps95": 111.56316000000004,
      "total_msgs_per_sec": 90.2661002587504,
      "avg_msgs_per_sec": 9.02661002587504,
      "ps25_msgs_per_sec": 8.965260251266766,
      "ps50_msgs_per_sec": 8.965838528802154,
      "ps95_msgs_per_sec": 9.157642857417652
    }
}
```