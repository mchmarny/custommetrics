package custommetrics

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3"
	googlepb "github.com/golang/protobuf/ptypes/timestamp"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	monitoredrespb "google.golang.org/genproto/googleapis/api/monitoredres"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"

	"gopkg.in/thedevsaddam/gojsonq.v2"
)

var (
	logger = log.New(os.Stdout, "[CM] ", 0)

	projectID       = mustEnvVar("PID", "")
	metricType      = mustEnvVar("METRIC_TYPE", "custom.googleapis.com/metric/default")
	metricSrcIDPath = mustEnvVar("METRIC_SRC_ID_PATH", "")  // its value must be a string
	metricValuePath = mustEnvVar("METRIC_VALUE_PATH", "")   // its value must be an int or a float
	metricTimePath  = mustEnvVar("METRIC_TIME_PATH", "now") // Optional, if specified must be in RFC3339 format

	once          sync.Once
	monitorClient *monitoring.MetricClient
)

// PubSubMessage is the payload of a Pub/Sub event
type PubSubMessage struct {
	Data []byte `json:"data"`
}

// ProcessorMetric processes pubsub topic events
func ProcessorMetric(ctx context.Context, m PubSubMessage) error {

	once.Do(func() {

		// create metric client
		mc, err := monitoring.NewMetricClient(ctx)
		if err != nil {
			logger.Fatalf("Error creating monitor client: %v", err)
		}
		monitorClient = mc
	})

	json := string(m.Data)
	logger.Printf("JSON: %s", json)

	jq := gojsonq.New().JSONString(json)

	metricSrcID := jq.Find(metricSrcIDPath).(string)
	logger.Printf("METRIC_SRC_ID: %s", metricSrcID)

	jq.Reset() // reset after each read

	metricValue := jq.Find(metricValuePath)
	logger.Printf("METRIC_VALUE: %v", metricValue)

	ts := time.Now()

	if metricTimePath != "now" {
		jq.Reset()
		metricTime := jq.Find(metricTimePath).(string)
		logger.Printf("METRIC_TIME: %v", metricTime)
		mts, err := time.Parse(time.RFC3339, metricTime)
		if err != nil {
			return fmt.Errorf("Error parsing event time from %s: %v", metricTime, err)
		}
		ts = mts
	}

	return publishMetric(ctx, metricSrcID, ts, metricValue)

}

func publishMetric(ctx context.Context, sourceID string, ts time.Time, metric interface{}) error {

	// derive typed vlaue from passed interface
	var val *monitoringpb.TypedValue
	switch v := metric.(type) {
	default:
		return fmt.Errorf("Unsuported metric type: %T", v)
	case float64:
		val = &monitoringpb.TypedValue{
			Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: metric.(float64)},
		}
	case int64:
		val = &monitoringpb.TypedValue{
			Value: &monitoringpb.TypedValue_Int64Value{Int64Value: metric.(int64)},
		}
	}

	// create data point
	ptTs := &googlepb.Timestamp{Seconds: ts.Unix()}
	dataPoint := &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{StartTime: ptTs, EndTime: ptTs},
		Value:    val,
	}

	// create time series request with the data point
	tsRequest := &monitoringpb.CreateTimeSeriesRequest{
		Name: monitoring.MetricProjectPath(projectID),
		TimeSeries: []*monitoringpb.TimeSeries{
			{
				Metric: &metricpb.Metric{
					Type:   metricType,
					Labels: map[string]string{"source_id": sourceID},
				},
				Resource: &monitoredrespb.MonitoredResource{
					Type:   "global",
					Labels: map[string]string{"project_id": projectID},
				},
				Points: []*monitoringpb.Point{dataPoint},
			},
		},
	}

	// publish series
	return monitorClient.CreateTimeSeries(ctx, tsRequest)

}

func mustEnvVar(key, fallbackValue string) string {

	if val, ok := os.LookupEnv(key); ok {
		logger.Printf("%s: %s", key, val)
		return val
	}

	if fallbackValue == "" {
		logger.Fatalf("Required envvar not set: %s", key)
	}

	logger.Printf("%s: %s (not set, using default)", key, fallbackValue)
	return fallbackValue
}
