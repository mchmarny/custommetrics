package custommetrics

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3"
	googlepb "github.com/golang/protobuf/ptypes/timestamp"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	monitoredrespb "google.golang.org/genproto/googleapis/api/monitoredres"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

const (
	customMetricType = "custom.googleapis.com/partner/mocked"
)

var (
	logger    = log.New(os.Stdout, "[METRICS] ", 0)
	projectID = mustEnvVar("PID", "")

	once          sync.Once
	monitorClient *monitoring.MetricClient
)

// PubSubMessage is the payload of a Pub/Sub event
type PubSubMessage struct {
	Data []byte `json:"data"`
}

// TextContent represents generic text event
type TextContent struct {
	Symbol    string    `json:"symbol"`
	ID        string    `json:"cid"`
	CreatedAt time.Time `json:"created"`
	Author    string    `json:"author"`
	Lang      string    `json:"lang"`
	Source    string    `json:"source"`
	Content   string    `json:"content"`
	Magnitude float32   `json:"magnitude"`
	Score     float32   `json:"score"`
	IsRetweet bool      `json:"retweet"`
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

	var c TextContent
	if err := json.Unmarshal(m.Data, &c); err != nil {
		logger.Fatalf("Error converting data: %v", err)
	}

	content, err := json.Marshal(c)
	if err != nil {
		logger.Fatalf("Error marshaling content: %v", err)
	}

	logger.Printf("Payload: %v", content)

	//extract metrics and send them to publishMetric
	publishMetric(ctx, "123", 1.2345)

	return err

}

func publishMetric(ctx context.Context, sourceID string, metric float64) {

	dataPoint := &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{
			EndTime: &googlepb.Timestamp{Seconds: time.Now().Unix()},
		},
		Value: &monitoringpb.TypedValue{
			Value: &monitoringpb.TypedValue_DoubleValue{
				DoubleValue: metric,
			},
		},
	}

	tsRequest := &monitoringpb.CreateTimeSeriesRequest{
		Name: monitoring.MetricProjectPath(projectID),
		TimeSeries: []*monitoringpb.TimeSeries{
			{
				Metric: &metricpb.Metric{
					Type:   customMetricType,
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

	if err := monitorClient.CreateTimeSeries(ctx, tsRequest); err != nil {
		log.Printf("Failed to write time series data: %v", err)
	}

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
