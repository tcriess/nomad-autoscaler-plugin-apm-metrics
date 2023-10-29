# Nomad Autoscaler APM plugin for "Metrics" endpoints

An APM plugin that regularly collects metrics from a metrics endpoint and provides its values to the Nomad Autoscaler.
It is a standalone solution without the need of an external time series DB to collect the metrics and thus an alternative to using the Prometheus APM plugin or the influx APM plugin.

## Configuration

Part of the Nomad autoscaler configuration:

```hcl
apm "app-metrics" {
  driver = "metrics"
  config = {
    url = "https://app/metrics"
    header_authorization = "Bearer abc"
    refresh = "10"
    retention = "60"
  }
}
```

The refresh value is how often the metrics are collected from the endpoint (in seconds).

The retention value is how long the metrics are kept in memory (in minutes).

## Usage in Nomad job definitions

```hcl
job "webapp" {
  ...
  group "demo" {
    ...
    scaling {
      enabled = true
      min = 1
      max = 2
      ...
      policy {
        check "scalecheck" {
          source = "app-metrics"
          query = "metrics"  # the name of the metric to use, this needs to include the tag if there is one, f.e. metrics{tag="a"}. If there is more than one tag, the order in which the tags are given needs to be alphabetically increasing, f.e. metrics{tag1="a",tag2="b"}

          strategy "threshold" {
            ...
          }
        }
      }
    }
  }
}
```

For now, metrics with tags can only be queried with all tags given, i.e. there are no aggregates across tags.
