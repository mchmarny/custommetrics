package custommetrics

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	meta "cloud.google.com/go/compute/metadata"
	monitoring "cloud.google.com/go/monitoring/apiv3"
	googlepb "github.com/golang/protobuf/ptypes/timestamp"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	monitoredrespb "google.golang.org/genproto/googleapis/api/monitoredres"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"

	"gopkg.in/thedevsaddam/gojsonq.v2"
)

const (
	metricTypeToken         = "METRIC_TYPE"
	metricSrcIDPathToken    = "SRC_ID_PATH"
	metricValuePathToken    = "VALUE_PATH"
	metricTsPathToken       = "TIME_PATH"
	metricTsPathTokenNotSet = "now"
)

var (
	logger = log.New(os.Stdout, "[CM] ", 0)

	metricType = mustEnvVar(metricTypeToken, "custom.googleapis.com/metric/default")
	// its value must be a string
	metricSrcIDPath = mustEnvVar(metricSrcIDPathToken, "")
	// its value must be an int or a float
	metricValuePath = mustEnvVar(metricValuePathToken, "")
	// Optional, if specified must be in RFC3339 format
	metricTimePath = mustEnvVar(metricTsPathToken, metricTsPathTokenNotSet)

	projectID     string
	once          sync.Once
	monitorClient *monitoring.MetricClient
)

// PubSubMessage is the payload of a Pub/Sub event
type PubSubMessage struct {
	Data []byte `json:"data"`
}

// ProcessorMetric processes pubsub topic events
func ProcessorMetric(ctx context.Context, m PubSubMessage) error {

	// setup
	once.Do(func() {

		// create metadata client
		projectID = os.Getenv("GCP_PROJECT") // for local execution
		if projectID == "" {
			mc := meta.NewClient(&http.Client{Transport: userAgentTransport{
				userAgent: "custommetrics",
				base:      http.DefaultTransport,
			}})
			p, err := mc.ProjectID()
			if err != nil {
				logger.Fatalf("Error creating metadata client: %v", err)
			}
			projectID = p
		}

		// create metric client
		mo, err := monitoring.NewMetricClient(ctx)
		if err != nil {
			logger.Fatalf("Error creating monitor client: %v", err)
		}
		monitorClient = mo
	})

	json := string(m.Data)
	logger.Printf("JSON: %s", json)

	jq := gojsonq.New().JSONString(json)

	metricSrcID := jq.Find(metricSrcIDPath).(string)
	logger.Printf("%s: %s", metricSrcIDPathToken, metricSrcID)

	jq.Reset() // reset before each read
	metricValue := jq.Find(metricValuePath)
	logger.Printf("%s: %v", metricValuePathToken, metricValue)

	ts := time.Now()
	if metricTimePath != metricTsPathTokenNotSet {
		jq.Reset()
		metricTime := jq.Find(metricTimePath).(string)
		logger.Printf("%s: %v", metricTsPathToken, metricTime)
		mts, err := time.Parse(time.RFC3339, metricTime)
		if err != nil {
			return fmt.Errorf("Error parsing event time from %s: %v", metricTime, err)
		}
		ts = mts
	}

	return publishMetric(ctx, metricSrcID, ts, metricValue)

}

func publishMetric(ctx context.Context, sourceID string, ts time.Time, metric interface{}) error {

	// derive typed value from passed interface
	var val *monitoringpb.TypedValue
	switch v := metric.(type) {
	default:
		return fmt.Errorf("Unsupported metric type: %T", v)
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
					Type: metricType,
					Labels: map[string]string{
						"source_id": sourceID,
						// random label to work around SD complaining
						// about multiple events for same time window
						"rnd_label": fmt.Sprint(rand.Intn(100)),
					},
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

// GCP Metadata
// https://godoc.org/cloud.google.com/go/compute/metadata#example-NewClient
type userAgentTransport struct {
	userAgent string
	base      http.RoundTripper
}

func (t userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", t.userAgent)
	return t.base.RoundTrip(req)
}
