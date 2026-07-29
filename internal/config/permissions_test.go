package config_test

import (
	"fmt"
	"strings"
	"testing"

	delegatev1 "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/delegate/v1"
	fulminev1 "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"github.com/ArkLabsHQ/fulmine/internal/config"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestPermissions(t *testing.T) {
	// Every RPC the daemon serves must have a permission entry: the macaroon
	// service rejects any method it has no permissions for, so a gap here is a
	// silently unreachable endpoint rather than a merely cosmetic problem.
	t.Run("every method has a permission", func(t *testing.T) {
		perms := config.AllPermissionsByMethod()
		for _, d := range allServiceDescs() {
			for _, method := range fullMethods(d) {
				ops, ok := perms[method]
				require.True(t, ok, "no permission registered for %s", method)
				require.Len(t, ops, 1, "unexpected op count for %s", method)
				require.Equal(t, config.ActionAccess, ops[0].Action, "unexpected action for %s", method)
			}
		}
	})

	// The reverse direction, which is what the original tests lacked: a permission
	// entry naming an RPC that does not exist is dead weight at best, and at worst
	// hides the real method having no entry at all — exactly what happened when
	// ListDelegates was registered under DelegateService instead of AdminService.
	// It also catches permissions left behind when an RPC is deleted.
	t.Run("no oprhaned permissions", func(t *testing.T) {
		served := make(map[string]bool)
		for _, d := range allServiceDescs() {
			for _, method := range fullMethods(d) {
				served[method] = true
			}
		}

		for method := range config.AllPermissionsByMethod() {
			// gRPC reflection is served by the grpc runtime, not by our descriptors.
			if strings.HasPrefix(method, "/grpc.reflection.") {
				continue
			}
			require.True(t, served[method], "permission registered for unknown method %s", method)
		}
	})

	// Protected methods must carry the entity matching the service they belong to,
	// so a user macaroon scoped to one entity can't reach another's methods.
	t.Run("protected methods are permissioned", func(t *testing.T) {
		perms := config.ProtectedByMethod()

		for _, method := range fullMethods(&fulminev1.Service_ServiceDesc) {
			if ops, ok := perms[method]; ok {
				require.Equal(t, config.EntityService, ops[0].Entity, "wrong entity for %s", method)
			}
		}
		for _, method := range fullMethods(&fulminev1.NotificationService_ServiceDesc) {
			if ops, ok := perms[method]; ok {
				require.Equal(t, config.EntityNotification, ops[0].Entity, "wrong entity for %s", method)
			}
		}
	})

	// Methods reachable before the wallet is unlocked (setup, auth) and the
	// delegate endpoints a counterparty calls must stay whitelisted — moving one
	// under ProtectedByMethod would lock users out of their own wallet setup.
	t.Run("whitelisted methods are permissionless", func(t *testing.T) {
		whitelisted := config.WhitelistedByMethod()

		mustBeWhitelisted := append(
			fullMethods(&fulminev1.WalletService_ServiceDesc),
			fullMethods(&delegatev1.DelegateService_ServiceDesc)...,
		)
		for _, method := range mustBeWhitelisted {
			_, ok := whitelisted[method]
			require.True(t, ok, "%s must be whitelisted", method)
		}
	})
}

// Every gRPC service the daemon registers. Adding a service here is what makes
// its methods subject to the checks below — an omission is how
// /delegate.v1.AdminService/ListDelegates once shipped with its permission
// registered under the wrong service name, unnoticed.
func allServiceDescs() []*grpc.ServiceDesc {
	return []*grpc.ServiceDesc{
		&fulminev1.Service_ServiceDesc,
		&fulminev1.NotificationService_ServiceDesc,
		&fulminev1.WalletService_ServiceDesc,
		&delegatev1.DelegateService_ServiceDesc,
		&delegatev1.AdminService_ServiceDesc,
	}
}

// fullMethods returns every "/service/method" the descriptor serves, unary and
// streaming alike.
func fullMethods(d *grpc.ServiceDesc) []string {
	out := make([]string, 0, len(d.Methods)+len(d.Streams))
	for _, m := range d.Methods {
		out = append(out, fmt.Sprintf("/%s/%s", d.ServiceName, m.MethodName))
	}
	for _, s := range d.Streams {
		out = append(out, fmt.Sprintf("/%s/%s", d.ServiceName, s.StreamName))
	}
	return out
}
