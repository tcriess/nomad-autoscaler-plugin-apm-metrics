package main

import (
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-autoscaler/plugins"
	"github.com/tcriess/nomad-autoscaler-plugin-apm-metrics/plugin"
)

func main() {
	plugins.Serve(factory)
}

// factory returns a new instance of the Cron Strategy plugin.
func factory(log hclog.Logger) interface{} {
	return plugin.NewMetricsPlugin(log)
}
