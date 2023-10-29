package plugin

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/nomad-autoscaler/plugins"
	"github.com/hashicorp/nomad-autoscaler/plugins/apm"
	"github.com/hashicorp/nomad-autoscaler/plugins/base"
	"github.com/hashicorp/nomad-autoscaler/sdk"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

const (
	// pluginName is the unique name of the this plugin amongst apm
	// plugins.
	pluginName = "metrics"

	// These are the keys read from the RunRequest.Config map.
	configKeyUrl          = "url"
	configKeyTimeout      = "timeout"
	configKeyHeaderPrefix = "header_"
	configKeyRefresh      = "refresh"
	configKeyRetention    = "retention"
)

var (
	PluginID = plugins.PluginID{
		Name:       pluginName,
		PluginType: sdk.PluginTypeAPM,
	}

	PluginConfig = &plugins.InternalPluginConfig{
		Factory: func(l hclog.Logger) interface{} { return NewMetricsPlugin(l) },
	}

	pluginInfo = &base.PluginInfo{
		Name:       pluginName,
		PluginType: sdk.PluginTypeAPM,
	}
)

// Assert that APMPlugin meets the  interface.
var _ apm.APM = (*APMPlugin)(nil)

// APMPlugin is the implementation of the apm.APM
// interface.
type APMPlugin struct {
	logger    hclog.Logger
	running   bool
	done      chan struct{}
	url       string
	timeout   int
	headers   map[string]string
	refresh   time.Duration
	retention time.Duration
	config    map[string]string
	series    map[string]sdk.TimestampedMetrics
}

// NewMetricsPlugin returns the Periods implementation of the
// strategy.Strategy interface.
func NewMetricsPlugin(log hclog.Logger) apm.APM {
	return &APMPlugin{
		logger: log,
	}
}

// PluginInfo satisfies the PluginInfo function on the base.Base interface.
func (a *APMPlugin) PluginInfo() (*base.PluginInfo, error) {
	return pluginInfo, nil
}

func (a *APMPlugin) doRequest() (map[string]*dto.MetricFamily, error) {
	client := retryablehttp.NewClient()
	client.RetryMax = 3
	client.Logger = a.logger.StandardLogger(&hclog.StandardLoggerOptions{InferLevels: true})
	client.HTTPClient.Timeout = time.Second * time.Duration(a.timeout)
	client.RetryWaitMin = 1 * time.Second
	client.RetryWaitMax = 5 * time.Second
	client.Backoff = retryablehttp.LinearJitterBackoff
	req, err := retryablehttp.NewRequest(http.MethodGet, a.url, nil)
	if err != nil {
		a.logger.Error("could not create request", "error", err)
		return nil, err
	}

	a.logger.Info("headers", "headers", a.headers)
	for headerName, headerValue := range a.headers {
		req.Header.Set(headerName, headerValue)
	}

	resp, err := client.Do(req)
	if err != nil {
		a.logger.Error("could not perform request", "error", err)
		return nil, err
	}
	defer resp.Body.Close()
	defer io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		a.logger.Error("unexpected http status", "status code", resp.StatusCode)
		return nil, err
	}
	var parser expfmt.TextParser
	mf, err := parser.TextToMetricFamilies(resp.Body)
	if err != nil {
		a.logger.Warn("could not parse response", "error", err)
		return nil, err
	}

	return mf, nil
}

// fetchMetrics is called once when setting the configuration, and it fetches the metrics every "refresh".
func (a *APMPlugin) fetchMetrics(done chan struct{}) {
	for {
		select {
		case <-time.After(a.refresh):
			now := time.Now()
			mf, err := a.doRequest()
			if err != nil {
				a.logger.Warn("could not parse response", "error", err)
				break
			}

			for metricFamilyName, metricFamily := range mf {
				metricType := metricFamily.GetType()
				tags := make([]string, 0)
			metricLoop:
				for _, metric := range metricFamily.Metric {
					metricName := metricFamilyName
					for _, labelPair := range metric.GetLabel() {
						tags = append(tags, fmt.Sprintf(`%s="%s"`, labelPair.GetName(), labelPair.GetValue()))
					}
					if len(tags) > 0 {
						sort.Strings(tags)
						metricName = fmt.Sprintf("%s{%s}", metricFamilyName, strings.Join(tags, `,`))
					}
					var value float64
					switch metricType {
					case dto.MetricType_COUNTER:
						if counter := metric.GetCounter(); counter != nil {
							value = counter.GetValue()
						}

					case dto.MetricType_GAUGE:
						if gauge := metric.GetGauge(); gauge != nil {
							value = gauge.GetValue()
						}

					default:
						continue metricLoop
					}
					if _, ok := a.series[metricName]; !ok {
						a.series[metricName] = make(sdk.TimestampedMetrics, 0)
					}
					a.series[metricName] = append(a.series[metricName], sdk.TimestampedMetric{
						Timestamp: now,
						Value:     value,
					})

					checkBefore := now.Add(-a.retention)
					deleteTo := -1
					for i, el := range a.series[metricName] {
						if el.Timestamp.After(checkBefore) {
							deleteTo = i - 1
							break
						}
					}
					if deleteTo >= 0 {
						for i := 0; i <= deleteTo; i++ {
							a.series[metricName][i] = sdk.TimestampedMetric{}
						}
						a.series[metricName] = a.series[metricName][deleteTo+1:]
					}
				}

			}

		case <-done:
			return
		}
	}
}

// SetConfig satisfies the SetConfig function on the base.Base interface.
func (a *APMPlugin) SetConfig(config map[string]string) error {
	a.config = config
	url, ok := a.config[configKeyUrl]
	if !ok || url == "" {
		return fmt.Errorf("%q config value cannot be empty", configKeyUrl)
	}
	a.url = url

	headers := make(map[string]string)
	for k, element := range config {
		if strings.HasPrefix(k, configKeyHeaderPrefix) && len(k) > len(configKeyHeaderPrefix) {
			headerName := http.CanonicalHeaderKey(strings.Replace(k[len(configKeyHeaderPrefix):], "_", "-", -1))
			headers[headerName] = element
		}
	}
	a.headers = headers

	timeout := 10
	if t, ok := config[configKeyTimeout]; ok {
		if t, err := strconv.Atoi(t); err == nil && t > 0 {
			timeout = t
		}
	}
	a.timeout = timeout

	a.series = make(map[string]sdk.TimestampedMetrics)

	refresh := 10 * time.Second
	if r, ok := config[configKeyRefresh]; ok {
		if r, err := strconv.Atoi(r); err == nil && r > 0 {
			refresh = time.Duration(r) * time.Second
		}
	}
	a.refresh = refresh

	retention := 60 * time.Minute
	if r, ok := config[configKeyRetention]; ok {
		if r, err := strconv.Atoi(r); err == nil && r > 0 {
			retention = time.Duration(r) * time.Minute
		}
	}
	a.retention = retention

	if a.done == nil {
		a.done = make(chan struct{})
	}
	if !a.running {
		go a.fetchMetrics(a.done)
		a.running = true
	} else {
		close(a.done)
		a.done = make(chan struct{})
		go a.fetchMetrics(a.done)
	}

	return nil
}

// Query satisfies the Query function on the apm.APM interface.
func (a *APMPlugin) Query(q string, r sdk.TimeRange) (sdk.TimestampedMetrics, error) {
	m, err := a.QueryMultiple(q, r)
	if err != nil {
		return nil, err
	}

	switch len(m) {
	case 0:
		return sdk.TimestampedMetrics{}, nil

	case 1:
		return m[0], nil

	default:
		return nil, fmt.Errorf("query returned %d metric streams, only 1 is expected", len(m))
	}
}

// QueryMultiple satisfies the QueryMultiple function on the apm.APM interface
func (a *APMPlugin) QueryMultiple(q string, r sdk.TimeRange) ([]sdk.TimestampedMetrics, error) {
	allSeries := make([]sdk.TimestampedMetrics, 0)
	if m, ok := a.series[q]; ok {
		serie := make(sdk.TimestampedMetrics, 0)
		idxFrom, _ := sort.Find(len(m), func(i int) int {
			if m[i].Timestamp.After(r.From) {
				return -1
			}
			return 1
		})
		if idxFrom < len(m) {
			idxTo, _ := sort.Find(len(m), func(i int) int {
				if m[i].Timestamp.After(r.To) {
					return -1
				}
				return 1
			})
			if idxTo >= idxFrom {
				serie = m[idxFrom:idxTo]
			}
		}

		allSeries = append(allSeries, serie)
	}

	return allSeries, nil
}
