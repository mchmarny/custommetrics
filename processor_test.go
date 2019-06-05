package custommetrics

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestProcessorSentiment(t *testing.T) {

	ctx := context.Background()

	json := `{
		"source_id":"test-client",
		"event_id":"eid-4d29eab6-a11d-4313-a514-19750f339c3c",
		"event_ts":"%s",
		"label":"comp-stats",
		"mem_used":45.30436197916667,
		"cpu_used":20.5,
		"load_1":3.45,
		"load_5":2.84,
		"load_15":2.6,
		"random_metric":1.086788711467383
	}`

	// Can't hardcode event_ts.
	// Data points cannot be written more than 25h in the past
	json = fmt.Sprintf(json, time.Now().Format(time.RFC3339))

	msg := PubSubMessage{Data: []byte(json)}

	err := EventProcessor(ctx, msg)
	if err != nil {
		t.Errorf("Failed to publish metric: %v", err)
	}
}
