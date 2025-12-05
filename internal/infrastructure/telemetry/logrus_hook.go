package telemetry

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/trace"
)

type OTelHook struct {
}

func NewOTelHook() *OTelHook {
	return &OTelHook{}
}

func (h *OTelHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func mapLevel(l logrus.Level) log.Severity {
	switch l {
	case logrus.PanicLevel:
		return log.SeverityFatal
	case logrus.FatalLevel:
		return log.SeverityFatal
	case logrus.ErrorLevel:
		return log.SeverityError
	case logrus.WarnLevel:
		return log.SeverityWarn
	case logrus.InfoLevel:
		return log.SeverityInfo
	case logrus.DebugLevel:
		return log.SeverityDebug
	case logrus.TraceLevel:
		return log.SeverityTrace
	default:
		return log.SeverityInfo
	}
}

func (h *OTelHook) Fire(e *logrus.Entry) error {
	ctx := e.Context
	if ctx == nil {
		ctx = context.Background()
	}

	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		e.Data["trace_id"] = spanCtx.TraceID().String()
		e.Data["span_id"] = spanCtx.SpanID().String()
	}

	rec := log.Record{}
	rec.SetTimestamp(e.Time)
	rec.SetSeverity(mapLevel(e.Level))
	rec.SetBody(log.StringValue(e.Message))

	rec.AddAttributes(
		log.String("log.kind", "app"),
		log.String("logger", "fulmine"),
		log.String("level", e.Level.String()),
	)

	for k, v := range e.Data {
		rec.AddAttributes(log.String(k, toString(v)))
	}

	rec.SetObservedTimestamp(time.Now())
	logger := global.GetLoggerProvider().Logger("fulmine")
	logger.Emit(ctx, rec)

	return nil
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprintf("%v", v)
	}
}
