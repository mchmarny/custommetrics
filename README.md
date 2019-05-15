# custommetrics

This demo illustrates how to use a generic Cloud Function to trigger on already existing PubSub topic, extract data from its payload, and publish it to Google Stackdriver as a custom metric without altering your original data pipeline.

## Context

If you have done any distributed development on GCP you've probably used PubSub to connect two or more data/event processing components (e.g. IoT Gateway to BigQuery or GCE-based batch process to Dataflow processing etc.). When I build these kind of integrations I often find myself wishing I could "take a pick" at one of the metrics in the data flowing through the system.

```json
{
  "source_id":"device-1",
  "event_id":"63404713-76c8-412b-a8fc-49f35409a977",
  "event_ts":"2019-05-14T05:48:13.132652Z",
  "metric": {
    "label":"friction",
    "value":0.2789,
  },
}
```

Assuming for example that your data published to PubSub topic has the above shape, in this demo I will illustrate how you can easily monitor the deviation of `friction` per each `device` over time without the need to manage additional infrastructure.

![Chart](./img/sd.png "Stackdriver Chart")

Additionally, I will show you how you can create monitoring policy to alert you when the monitored metric falls outside of the pre-configured range.

> Note, this will only work on JSON PubSub payloads

## Configuration

Assuming the above JSON payload shape on your PubSub topic, there are few variables we need to define first:

```shell
FTOPIC="name-of-data-topic"

FVAR="PID=${GCP_PROJECT}"
FVAR="${FVAR},METRIC_SRC_ID_PATH=source_id"
FVAR="${FVAR},METRIC_TIME_PATH=event_ts"
FVAR="${FVAR},METRIC_VALUE_PATH=metric.value"
FVAR="${FVAR},METRIC_TYPE=custom.googleapis.com/metric/friction"
```

* `FTOPIC` is the name of the PubSub topic on which you want to trigger
* `FVAR` defines the "selects" for data to extract from each one fo the PubSub topic payloads
  * `METRIC_SRC_ID_PATH` uniquely identity of the source of this event
  * `METRIC_TIME_PATH` (optional) time stamp of this event (must be RFC3339 format, processing time if not defined)
  * `METRIC_TYPE` the type of this metric that will distinguish it from other metrics you track in Stackdriver

## Deployment

Once you have these metrics defined, you can deploy the Cloud Function

```shell
gcloud functions deploy custommetrics-maker \
  --entry-point ProcessorMetric \
  --set-env-vars $FVARS \
  --memory 256MB \
  --region us-central1 \
  --runtime go112 \
  --trigger-topic $FTOPIC \
  --timeout 540s
```

If everything goes well you will see a confirmation

```shell
$: bin/deploy
Deploying function (may take a while - up to 2 minutes)...done.
...
status: ACTIVE
versionId: '3'
```

## Monitoring

In Stackdriver now you can use Metric Explorer to build a time-series chart of the published data. First, use the  resource type and metric finder to paste your metric type. Using the above deployment as an example that would be `custom.googleapis.com/metric/friction`.

![Metric](./img/metric.png "Stackdriver Metric")

Now you can use the `Group By` to display the time series per `source_id` and optionally specify the Aggregator in case you want to display `mean` for more volatile metrics.

## Alerts

Once you have custom metric monitored in stack driver you can easily create an alert policy. Many options there with regards to the duration, single or multiple sources, and percentages vs single values.

![Alert](./img/alert.png "Stackdriver Alert Policy")

Once configured though, when the monitored metrics violates that policy you can get notifications about an incident

![Alert](./img/incident.png "Stackdriver Incident")

Obviously this approach is meant to replace a proper monitoring solution. Still, I hope you found it helpful.

## Disclaimer

This is my personal project and it does not represent my employer. I take no responsibility for issues caused by this code. I do my best to ensure that everything works, but if something goes wrong, my apologies is all you will get.