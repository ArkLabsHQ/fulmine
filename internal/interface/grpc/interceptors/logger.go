package interceptors

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

var sensitiveRequestFields = map[string]struct{}{
	"macaroon":          {},
	"password":          {},
	"currentpassword":   {},
	"newpassword":       {},
	"privatekey":        {},
	"mnemonic":          {},
	"preimage":          {},
	"message":           {},
	"proof":             {},
	"tx":                {},
	"signedintentproof": {},
	"intentmessage":     {},
	"partialforfeittx":  {},
	"secret":            {},
}

func unaryLogger(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	logUnaryCall(info.FullMethod, req, time.Since(start), err)
	return resp, err
}

func streamLogger(
	srv interface{},
	stream grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	start := time.Now()
	err := handler(srv, stream)
	logStreamCall(info.FullMethod, time.Since(start), err)
	return err
}

func logUnaryCall(method string, req interface{}, dur time.Duration, err error) {
	entry := log.WithFields(log.Fields{
		"method":      method,
		"duration_ms": dur.Milliseconds(),
	})

	if log.IsLevelEnabled(log.DebugLevel) {
		if sanitizedReq, ok := sanitizeRequest(req); ok {
			entry = entry.WithField("request", sanitizedReq)
		}
	}

	if err != nil {
		entry.WithError(err).Warn("gRPC call failed")
		return
	}

	entry.Debug("gRPC call ok")
}

func logStreamCall(method string, dur time.Duration, err error) {
	entry := log.WithFields(log.Fields{
		"method":      method,
		"duration_ms": dur.Milliseconds(),
	})

	if err != nil {
		entry.WithError(err).Warn("gRPC stream failed")
		return
	}

	entry.Debug("gRPC stream ok")
}

func sanitizeRequest(req interface{}) (string, bool) {
	if req == nil {
		return "", false
	}

	raw, err := json.Marshal(req)
	if err != nil {
		return "", false
	}

	var decoded interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", false
	}

	sanitized := redactSensitiveFields(decoded)
	formatted, err := json.Marshal(sanitized)
	if err != nil {
		return "", false
	}

	return string(formatted), true
}

func redactSensitiveFields(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		redacted := make(map[string]interface{}, len(v))
		for key, item := range v {
			if isSensitiveField(key) {
				redacted[key] = "[REDACTED]"
				continue
			}
			redacted[key] = redactSensitiveFields(item)
		}
		return redacted
	case []interface{}:
		redacted := make([]interface{}, len(v))
		for i, item := range v {
			redacted[i] = redactSensitiveFields(item)
		}
		return redacted
	default:
		return v
	}
}

func isSensitiveField(name string) bool {
	normalized := normalizeFieldName(name)
	if _, ok := sensitiveRequestFields[normalized]; ok {
		return true
	}

	for sensitive := range sensitiveRequestFields {
		if strings.Contains(normalized, sensitive) {
			return true
		}
	}

	return false
}

func normalizeFieldName(name string) string {
	var builder strings.Builder
	builder.Grow(len(name))

	for _, r := range name {
		if !reflect.ValueOf(r).IsValid() {
			continue
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		}
	}

	return strings.ToLower(builder.String())
}
