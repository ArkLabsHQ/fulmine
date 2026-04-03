package interceptors

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"google.golang.org/grpc"
)

type testRequest struct {
	Amount   uint64 `json:"amount"`
	Password string `json:"password"`
	Preimage string `json:"preimage"`
	SwapID   string `json:"swapId"`
}

func TestUnaryLogger(t *testing.T) {
	logger, hook := logtest.NewNullLogger()
	previousLogger := log.StandardLogger()
	previousLevel := previousLogger.GetLevel()
	log.SetOutput(logger.Out)
	log.SetFormatter(logger.Formatter)
	log.StandardLogger().ReplaceHooks(logger.Hooks)
	defer log.StandardLogger().ReplaceHooks(previousLogger.Hooks)
	defer log.SetFormatter(previousLogger.Formatter)
	defer log.SetOutput(previousLogger.Out)
	defer log.SetLevel(previousLevel)

	log.SetLevel(log.DebugLevel)

	t.Run("successful call logs debug with sanitized request", func(t *testing.T) {
		hook.Reset()

		info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/TestMethod"}
		req := testRequest{
			Amount:   42,
			Password: "super-secret",
			Preimage: "very-secret",
			SwapID:   "swap-123",
		}
		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			return "ok", nil
		}

		resp, err := unaryLogger(context.Background(), req, info, handler)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if resp != "ok" {
			t.Fatalf("expected resp 'ok', got: %v", resp)
		}

		if len(hook.Entries) != 1 {
			t.Fatalf("expected 1 log entry, got %d", len(hook.Entries))
		}

		entry := hook.Entries[0]
		if entry.Level != log.DebugLevel {
			t.Fatalf("expected Debug level, got %s", entry.Level)
		}
		if entry.Message != "gRPC call ok" {
			t.Fatalf("expected 'gRPC call ok', got %q", entry.Message)
		}
		if entry.Data["method"] != "/test.Service/TestMethod" {
			t.Fatalf("expected method field, got: %v", entry.Data["method"])
		}
		if _, ok := entry.Data["duration_ms"]; !ok {
			t.Fatal("expected duration_ms field")
		}

		requestValue, ok := entry.Data["request"].(string)
		if !ok {
			t.Fatalf("expected request field to be a string, got %T", entry.Data["request"])
		}

		var requestData map[string]interface{}
		if err := json.Unmarshal([]byte(requestValue), &requestData); err != nil {
			t.Fatalf("failed to decode request log field: %v", err)
		}

		if requestData["amount"] != float64(42) {
			t.Fatalf("expected amount to be preserved, got %v", requestData["amount"])
		}
		if requestData["swapId"] != "swap-123" {
			t.Fatalf("expected swapId to be preserved, got %v", requestData["swapId"])
		}
		if requestData["password"] != "[REDACTED]" {
			t.Fatalf("expected password to be redacted, got %v", requestData["password"])
		}
		if requestData["preimage"] != "[REDACTED]" {
			t.Fatalf("expected preimage to be redacted, got %v", requestData["preimage"])
		}
	})

	t.Run("failed call logs warn with error", func(t *testing.T) {
		hook.Reset()

		info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/FailMethod"}
		expectedErr := fmt.Errorf("rpc failed")
		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			return nil, expectedErr
		}

		resp, err := unaryLogger(context.Background(), testRequest{}, info, handler)
		if err != expectedErr {
			t.Fatalf("expected error %v, got: %v", expectedErr, err)
		}
		if resp != nil {
			t.Fatalf("expected nil resp, got: %v", resp)
		}

		if len(hook.Entries) != 1 {
			t.Fatalf("expected 1 log entry, got %d", len(hook.Entries))
		}

		entry := hook.Entries[0]
		if entry.Level != log.WarnLevel {
			t.Fatalf("expected Warn level, got %s", entry.Level)
		}
		if entry.Message != "gRPC call failed" {
			t.Fatalf("expected 'gRPC call failed', got %q", entry.Message)
		}
		if entry.Data["method"] != "/test.Service/FailMethod" {
			t.Fatalf("expected method field, got: %v", entry.Data["method"])
		}
		if _, ok := entry.Data["duration_ms"]; !ok {
			t.Fatal("expected duration_ms field")
		}
		if entry.Data[log.ErrorKey] != expectedErr {
			t.Fatalf("expected error in log, got: %v", entry.Data[log.ErrorKey])
		}
	})
}

func TestStreamLogger(t *testing.T) {
	logger, hook := logtest.NewNullLogger()
	previousLogger := log.StandardLogger()
	previousLevel := previousLogger.GetLevel()
	log.SetOutput(logger.Out)
	log.SetFormatter(logger.Formatter)
	log.StandardLogger().ReplaceHooks(logger.Hooks)
	defer log.StandardLogger().ReplaceHooks(previousLogger.Hooks)
	defer log.SetFormatter(previousLogger.Formatter)
	defer log.SetOutput(previousLogger.Out)
	defer log.SetLevel(previousLevel)

	log.SetLevel(log.DebugLevel)

	t.Run("successful stream logs debug", func(t *testing.T) {
		hook.Reset()

		info := &grpc.StreamServerInfo{FullMethod: "/test.Service/TestStream"}
		handler := func(srv interface{}, stream grpc.ServerStream) error {
			return nil
		}

		err := streamLogger(nil, nil, info, handler)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if len(hook.Entries) != 1 {
			t.Fatalf("expected 1 log entry, got %d", len(hook.Entries))
		}

		entry := hook.Entries[0]
		if entry.Level != log.DebugLevel {
			t.Fatalf("expected Debug level, got %s", entry.Level)
		}
		if entry.Message != "gRPC stream ok" {
			t.Fatalf("expected 'gRPC stream ok', got %q", entry.Message)
		}
	})

	t.Run("failed stream logs warn", func(t *testing.T) {
		hook.Reset()

		info := &grpc.StreamServerInfo{FullMethod: "/test.Service/FailStream"}
		expectedErr := fmt.Errorf("stream failed")
		handler := func(srv interface{}, stream grpc.ServerStream) error {
			return expectedErr
		}

		err := streamLogger(nil, nil, info, handler)
		if err != expectedErr {
			t.Fatalf("expected error %v, got: %v", expectedErr, err)
		}

		if len(hook.Entries) != 1 {
			t.Fatalf("expected 1 log entry, got %d", len(hook.Entries))
		}

		entry := hook.Entries[0]
		if entry.Level != log.WarnLevel {
			t.Fatalf("expected Warn level, got %s", entry.Level)
		}
		if entry.Message != "gRPC stream failed" {
			t.Fatalf("expected 'gRPC stream failed', got %q", entry.Message)
		}
	})
}

func TestSanitizeRequest(t *testing.T) {
	request := map[string]interface{}{
		"password": "secret",
		"nested": map[string]interface{}{
			"preimage": "another-secret",
			"swapId":   "swap-1",
		},
		"items": []interface{}{
			map[string]interface{}{"token": "sensitive"},
			map[string]interface{}{"amount": 10},
		},
	}

	sanitized, ok := sanitizeRequest(request)
	if !ok {
		t.Fatal("expected sanitizeRequest to succeed")
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(sanitized), &decoded); err != nil {
		t.Fatalf("failed to decode sanitized request: %v", err)
	}

	if decoded["password"] != "[REDACTED]" {
		t.Fatalf("expected password to be redacted, got %v", decoded["password"])
	}

	nested := decoded["nested"].(map[string]interface{})
	if nested["preimage"] != "[REDACTED]" {
		t.Fatalf("expected nested preimage to be redacted, got %v", nested["preimage"])
	}
	if nested["swapId"] != "swap-1" {
		t.Fatalf("expected nested swapId to be preserved, got %v", nested["swapId"])
	}

	items := decoded["items"].([]interface{})
	firstItem := items[0].(map[string]interface{})
	secondItem := items[1].(map[string]interface{})
	if firstItem["token"] != "[REDACTED]" {
		t.Fatalf("expected token to be redacted, got %v", firstItem["token"])
	}
	if secondItem["amount"] != float64(10) {
		t.Fatalf("expected amount to be preserved, got %v", secondItem["amount"])
	}
}
