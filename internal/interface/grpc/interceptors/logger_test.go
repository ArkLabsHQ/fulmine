package interceptors

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"
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

		resp, err := unaryLogger(t.Context(), req, info, handler)
		require.NoError(t, err)
		require.Equal(t, "ok", resp)
		require.Len(t, hook.Entries, 1)

		entry := hook.Entries[0]
		require.Equal(t, log.DebugLevel, entry.Level)
		require.Contains(t, entry.Message, "method=/test.Service/TestMethod")
		require.Contains(t, entry.Message, "duration=")
		require.Contains(t, entry.Message, "request=")

		splts := strings.Split(entry.Message, "request=")
		require.Len(t, splts, 2)

		var requestData map[string]interface{}
		err = json.Unmarshal([]byte(splts[1]), &requestData)
		require.NoError(t, err)
		require.Equal(t, float64(42), requestData["amount"])
		require.Equal(t, "swap-123", requestData["swapId"])
		require.Equal(t, "******", requestData["password"])
		require.Equal(t, "******", requestData["preimage"])
	})

	t.Run("failed call logs warn with error", func(t *testing.T) {
		hook.Reset()

		info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/FailMethod"}
		expectedErr := fmt.Errorf("rpc failed")
		handler := func(ctx context.Context, req interface{}) (interface{}, error) {
			return nil, expectedErr
		}

		resp, err := unaryLogger(t.Context(), testRequest{}, info, handler)
		require.Error(t, err)
		require.EqualError(t, err, expectedErr.Error())
		require.Nil(t, resp)

		require.Len(t, hook.Entries, 1)
		entry := hook.Entries[0]
		require.Equal(t, log.WarnLevel, entry.Level)
		require.Contains(t, entry.Message, "method=/test.Service/FailMethod")
		require.Contains(t, entry.Message, "duration=")
		require.NotEmpty(t, entry.Data[log.ErrorKey])
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
		require.NoError(t, err)

		require.Len(t, hook.Entries, 1)
		entry := hook.Entries[0]
		require.Equal(t, log.DebugLevel, entry.Level)
		require.Contains(t, entry.Message, "method=/test.Service/TestStream")
		require.Contains(t, entry.Message, "duration=")
	})

	t.Run("failed stream logs warn", func(t *testing.T) {
		hook.Reset()

		info := &grpc.StreamServerInfo{FullMethod: "/test.Service/FailStream"}
		expectedErr := fmt.Errorf("stream failed")
		handler := func(srv interface{}, stream grpc.ServerStream) error {
			return expectedErr
		}

		err := streamLogger(nil, nil, info, handler)
		require.Error(t, err)
		require.EqualError(t, err, expectedErr.Error())

		require.Len(t, hook.Entries, 1)
		entry := hook.Entries[0]
		require.Equal(t, log.WarnLevel, entry.Level)
		require.Contains(t, entry.Message, "method=/test.Service/FailStream")
		require.Contains(t, entry.Message, "duration=")
		require.NotEmpty(t, entry.Data[log.ErrorKey])
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
			map[string]interface{}{"secret": "sensitive"},
			map[string]interface{}{"amount": 10},
		},
	}

	sanitized, ok := sanitizeRequest(request)
	require.True(t, ok)
	require.NotEmpty(t, sanitized)

	var decoded map[string]interface{}
	err := json.Unmarshal([]byte(sanitized), &decoded)
	require.NoError(t, err)

	require.Equal(t, "******", decoded["password"])

	nested := decoded["nested"].(map[string]interface{})
	require.Equal(t, "******", nested["preimage"])
	require.Equal(t, "swap-1", nested["swapId"])

	items := decoded["items"].([]interface{})
	firstItem := items[0].(map[string]interface{})
	secondItem := items[1].(map[string]interface{})
	require.Equal(t, "******", firstItem["secret"])
	require.Equal(t, float64(10), secondItem["amount"])
}
