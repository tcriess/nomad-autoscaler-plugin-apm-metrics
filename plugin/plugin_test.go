package plugin

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-autoscaler/plugins/base"
	"github.com/hashicorp/nomad-autoscaler/sdk"
	"github.com/stretchr/testify/assert"
)

func TestAPMPlugin_PluginInfo(t *testing.T) {
	s := &APMPlugin{logger: hclog.NewNullLogger()}
	expectedOutput := &base.PluginInfo{Name: "metrics", PluginType: "apm"}
	actualOutput, err := s.PluginInfo()
	assert.Nil(t, err)
	assert.Equal(t, expectedOutput, actualOutput)
}

func TestAPMPlugin_SetConfig(t *testing.T) {
	testCases := []struct {
		config      map[string]string
		expectedUrl string
	}{
		{config: map[string]string{configKeyUrl: "http://example.invalid"}, expectedUrl: "http://example.invalid"},
	}

	for _, tc := range testCases {
		a := &APMPlugin{logger: hclog.NewNullLogger()}
		err := a.SetConfig(tc.config)
		assert.Nil(t, err)
		assert.Equal(t, tc.expectedUrl, a.url)
	}
}

func TestAPMPlugin_GetValue(t *testing.T) {
	url := os.Getenv("TEST_URL")
	metric := os.Getenv("TEST_METRIC")
	header := os.Getenv("TEST_HEADER")
	if url == "" || metric == "" {
		return
	}
	fmt.Printf("url: %s, metric: %s, header: %s\n", url, metric, header)
	headerName := ""
	headerValue := ""
	if header != "" {
		parts := strings.Split(header, ":")
		if len(parts) != 2 {
			return
		}
		headerName = parts[0]
		headerValue = parts[1]
	}
	testCases := []struct {
		config map[string]string
	}{
		{config: map[string]string{configKeyUrl: url, configKeyHeaderPrefix + headerName: headerValue, configKeyRefresh: "10"}},
	}
	for _, tc := range testCases {
		a := &APMPlugin{logger: hclog.New(&hclog.LoggerOptions{Level: hclog.Debug})}
		err := a.SetConfig(tc.config)
		assert.Nil(t, err)
		r := sdk.TimeRange{
			From: time.Now().Add(-time.Hour),
			To:   time.Now(),
		}
		res, err := a.Query(metric, r)
		assert.Nil(t, err)
		fmt.Printf("%+v", res)

		time.Sleep(20 * time.Second)

		r = sdk.TimeRange{
			From: time.Now().Add(-time.Hour),
			To:   time.Now(),
		}
		res, err = a.Query(metric, r)
		assert.Nil(t, err)
		fmt.Printf("%+v", res)
	}
}
