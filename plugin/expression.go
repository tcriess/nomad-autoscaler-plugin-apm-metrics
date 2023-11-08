package plugin

import (
	"fmt"

	"github.com/antonmedv/expr"
	"github.com/hashicorp/nomad-autoscaler/sdk"
)

type env struct {
	Metrics     map[string]sdk.TimestampedMetrics
	MetricsSum  func(metrics1, metrics2 sdk.TimestampedMetrics) sdk.TimestampedMetrics
	MetricsDiff func(metrics1, metrics2 sdk.TimestampedMetrics) sdk.TimestampedMetrics
}

func metricsSum(metrics1, metrics2 sdk.TimestampedMetrics) sdk.TimestampedMetrics {
	l := len(metrics1)
	if len(metrics2) > l {
		l = len(metrics2)
	}
	tm2 := metrics2[:]
	res := make(sdk.TimestampedMetrics, 0, l)
	for _, v1 := range metrics1 {
		for i, v2 := range tm2 {
			if v2.Timestamp == v1.Timestamp {
				res = append(res, sdk.TimestampedMetric{
					Timestamp: v1.Timestamp,
					Value:     v1.Value + v2.Value,
				})
				tm2 = tm2[i:]
				break
			}
		}
	}
	return res
}

func metricsDiff(metrics1, metrics2 sdk.TimestampedMetrics) sdk.TimestampedMetrics {
	l := len(metrics1)
	if len(metrics2) > l {
		l = len(metrics2)
	}
	tm2 := metrics2[:]
	res := make(sdk.TimestampedMetrics, 0, l)
	for _, v1 := range metrics1 {
		for i, v2 := range tm2 {
			if v2.Timestamp == v1.Timestamp {
				res = append(res, sdk.TimestampedMetric{
					Timestamp: v1.Timestamp,
					Value:     v1.Value - v2.Value,
				})
				tm2 = tm2[i:]
				break
			}
		}
	}
	return res
}

// evaluateExpression parses the given expression and evaluates the resulting program. The expected result is
// the target value.
func evaluateExpression(expression string, metricsMap map[string]sdk.TimestampedMetrics) (sdk.TimestampedMetrics, error) {
	e := env{
		Metrics:     metricsMap,
		MetricsSum:  metricsSum,
		MetricsDiff: metricsDiff,
	}

	prg, err := expr.Compile(expression, expr.Env(env{}), expr.Operator("+", "MetricsSum"), expr.Operator("-", "MetricsDiff"))
	if err != nil {
		return nil, err
	}

	res, err := expr.Run(prg, e)
	if err != nil {
		return nil, err
	}
	// not quite sure what result type to expect...
	switch res.(type) {
	case sdk.TimestampedMetrics: // already the exact type we want
		return res.(sdk.TimestampedMetrics), nil

	case []sdk.TimestampedMetric: // different type name for the same thing
		slice := res.([]sdk.TimestampedMetric)
		return slice, nil

	case []interface{}: // generic slice of interfaces
		tm := make([]sdk.TimestampedMetric, 0)
		slice := res.([]interface{})
		for _, v := range slice {
			switch v.(type) {
			case sdk.TimestampedMetric:
				tm = append(tm, v.(sdk.TimestampedMetric))
			}
		}
		return tm, nil
	}
	return nil, fmt.Errorf(fmt.Sprintf("could not parse expression result %+v (%T)", res, res))
}
