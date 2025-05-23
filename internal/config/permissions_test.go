package config_test

import (
	"fmt"
	"testing"

	fulminev1 "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/internal/config"
	"github.com/stretchr/testify/require"
)

func TestProtectedMethods(t *testing.T) {
	allMethods := make([]string, 0)
	// Service.proto
	for _, m := range fulminev1.Service_ServiceDesc.Methods {
		allMethods = append(allMethods, fmt.Sprintf("/%s/%s", fulminev1.Service_ServiceDesc.ServiceName, m.MethodName))
	}
	// Notification.proto
	for _, m := range fulminev1.NotificationService_ServiceDesc.Methods {
		allMethods = append(allMethods, fmt.Sprintf("/%s/%s", fulminev1.NotificationService_ServiceDesc.ServiceName, m.MethodName))
	}
	perms := config.ProtectedByMethod()
	for _, method := range allMethods {
		ops, ok := perms[method]
		require.True(t, ok, "missing permission for %s", method)
		require.Len(t, ops, 1)
		require.Equal(t, config.ActionAccess, ops[0].Action)
		// Check correct entity
		if m := method; m[:len("/Service/")] == "/Service/" || m[:len("/fulmine.v1.Service/")] == "/fulmine.v1.Service/" {
			require.Equal(t, config.EntityService, ops[0].Entity)
		} else if m[:len("/NotificationService/")] == "/NotificationService/" || m[:len("/fulmine.v1.NotificationService/")] == "/fulmine.v1.NotificationService/" {
			require.Equal(t, config.EntityNotification, ops[0].Entity)
		}
	}
}

func TestWhitelistedMethods(t *testing.T) {
	allMethods := make([]string, 0)
	// WalletService methods
	for _, m := range fulminev1.WalletService_ServiceDesc.Methods {
		allMethods = append(allMethods, fmt.Sprintf("/%s/%s", fulminev1.WalletService_ServiceDesc.ServiceName, m.MethodName))
	}
	perms := config.WhitelistedByMethod()
	for _, method := range allMethods {
		ops, ok := perms[method]
		require.True(t, ok, "missing permission for %s", method)
		require.Len(t, ops, 1)
		require.Equal(t, config.EntityWallet, ops[0].Entity)
		require.Equal(t, config.ActionAccess, ops[0].Action)
	}
}
