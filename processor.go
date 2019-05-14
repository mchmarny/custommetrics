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
	logger = log.New(os.Stdout, "[METRICS] ", 0)

	projectID       = mustEnvVar("PID", "")
	metricType      = mustEnvVar("METRIC_TYPE", "custom.googleapis.com/metric/default")
	metricSrcIDPath = mustEnvVar("METRIC_SRC_ID_PATH", "")
	metricValuePath = mustEnvVar("METRIC_VALUE_PATH", "")

	once          sync.Once
	monitorClient *monitoring.MetricClient
)

// PubSubMessage is the payload of a Pub/Sub event
type PubSubMessage struct {
	Data []byte `json:"data"`
}

// ProcessorSentiment processes pubsub topic events
func ProcessorSentiment(ctx context.Context, m PubSubMessage) error {

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

	metricSrcID := jq.Find(metricSrcIDPath)
	logger.Printf("METRIC_SRC_ID: %v", metricSrcID)

	metricValue := jq.Find(metricValuePath)
	logger.Printf("METRIC_VALUE: %v", metricValue)

	//TODO: implement value conversion extract metrics and send them to publishMetric
	return publishMetric(ctx, "123", 1.2345)

}

func publishMetric(ctx context.Context, sourceID string, metric interface{}) error {

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
	dataPoint := &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{
			EndTime: &googlepb.Timestamp{Seconds: time.Now().Unix()},
		},
		Value: val,
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
		return val
	}

	if fallbackValue == "" {
		logger.Fatalf("Required envvar not set: %s", key)
	}

	return fallbackValue
}
