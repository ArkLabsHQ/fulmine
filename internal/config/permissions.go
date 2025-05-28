package config

import (
	"fmt"

	fulminev1 "github.com/ArkLabsHQ/fulmine/api-spec/protobuf/gen/go/fulmine/v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
)

const (
	EntityWallet       = "wallet"
	EntityService      = "service"
	EntityNotification = "notification"

	ActionAccess = "access"

	adminMacaroonFile = "admin.macaroon"
)

var (
	macFiles = map[string][]bakery.Op{
		adminMacaroonFile: AdminPermissions(),
	}
)

// AdminPermissions grants access to all protected methods for all entities
func AdminPermissions() []bakery.Op {
	return []bakery.Op{
		{Entity: EntityWallet, Action: ActionAccess},
		{Entity: EntityService, Action: ActionAccess},
		{Entity: EntityNotification, Action: ActionAccess},
	}
}

// WhitelistedByMethod methods accessible without macaroon (public)
func WhitelistedByMethod() map[string][]bakery.Op {
	return map[string][]bakery.Op{
		fmt.Sprintf("/%s/GenSeed", fulminev1.WalletService_ServiceDesc.ServiceName):        {{Entity: EntityWallet, Action: ActionAccess}},
		fmt.Sprintf("/%s/CreateWallet", fulminev1.WalletService_ServiceDesc.ServiceName):   {{Entity: EntityWallet, Action: ActionAccess}},
		fmt.Sprintf("/%s/Unlock", fulminev1.WalletService_ServiceDesc.ServiceName):         {{Entity: EntityWallet, Action: ActionAccess}},
		fmt.Sprintf("/%s/Lock", fulminev1.WalletService_ServiceDesc.ServiceName):           {{Entity: EntityWallet, Action: ActionAccess}},
		fmt.Sprintf("/%s/ChangePassword", fulminev1.WalletService_ServiceDesc.ServiceName): {{Entity: EntityWallet, Action: ActionAccess}},
		fmt.Sprintf("/%s/RestoreWallet", fulminev1.WalletService_ServiceDesc.ServiceName):  {{Entity: EntityWallet, Action: ActionAccess}},
		fmt.Sprintf("/%s/Status", fulminev1.WalletService_ServiceDesc.ServiceName):         {{Entity: EntityWallet, Action: ActionAccess}},
		fmt.Sprintf("/%s/Auth", fulminev1.WalletService_ServiceDesc.ServiceName):           {{Entity: EntityWallet, Action: ActionAccess}},
	}
}

// ProtectedByMethod methods requiring user macaroon
func ProtectedByMethod() map[string][]bakery.Op {
	return map[string][]bakery.Op{
		// Service.proto methods
		fmt.Sprintf("/%s/GetAddress", fulminev1.Service_ServiceDesc.ServiceName):                 {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/GetBalance", fulminev1.Service_ServiceDesc.ServiceName):                 {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/GetInfo", fulminev1.Service_ServiceDesc.ServiceName):                    {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/GetOnboardAddress", fulminev1.Service_ServiceDesc.ServiceName):          {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/GetRoundInfo", fulminev1.Service_ServiceDesc.ServiceName):               {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/GetTransactionHistory", fulminev1.Service_ServiceDesc.ServiceName):      {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/RedeemNote", fulminev1.Service_ServiceDesc.ServiceName):                 {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/Settle", fulminev1.Service_ServiceDesc.ServiceName):                     {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/SendOffChain", fulminev1.Service_ServiceDesc.ServiceName):               {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/SendOnChain", fulminev1.Service_ServiceDesc.ServiceName):                {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/SignTransaction", fulminev1.Service_ServiceDesc.ServiceName):            {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/CreateVHTLC", fulminev1.Service_ServiceDesc.ServiceName):                {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/ClaimVHTLC", fulminev1.Service_ServiceDesc.ServiceName):                 {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/RefundVHTLCWithoutReceiver", fulminev1.Service_ServiceDesc.ServiceName): {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/ListVHTLC", fulminev1.Service_ServiceDesc.ServiceName):                  {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/GetInvoice", fulminev1.Service_ServiceDesc.ServiceName):                 {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/PayInvoice", fulminev1.Service_ServiceDesc.ServiceName):                 {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/IsInvoiceSettled", fulminev1.Service_ServiceDesc.ServiceName):           {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/GetDelegatePublicKey", fulminev1.Service_ServiceDesc.ServiceName):       {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/WatchAddressForRollover", fulminev1.Service_ServiceDesc.ServiceName):    {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/UnwatchAddress", fulminev1.Service_ServiceDesc.ServiceName):             {{Entity: EntityService, Action: ActionAccess}},
		fmt.Sprintf("/%s/ListWatchedAddresses", fulminev1.Service_ServiceDesc.ServiceName):       {{Entity: EntityService, Action: ActionAccess}},
		// Notification.proto methods
		fmt.Sprintf("/%s/SubscribeForAddresses", fulminev1.NotificationService_ServiceDesc.ServiceName):   {{Entity: EntityNotification, Action: ActionAccess}},
		fmt.Sprintf("/%s/UnsubscribeForAddresses", fulminev1.NotificationService_ServiceDesc.ServiceName): {{Entity: EntityNotification, Action: ActionAccess}},
		fmt.Sprintf("/%s/GetVtxoNotifications", fulminev1.NotificationService_ServiceDesc.ServiceName):    {{Entity: EntityNotification, Action: ActionAccess}},
		fmt.Sprintf("/%s/RoundNotifications", fulminev1.NotificationService_ServiceDesc.ServiceName):      {{Entity: EntityNotification, Action: ActionAccess}},
		fmt.Sprintf("/%s/AddWebhook", fulminev1.NotificationService_ServiceDesc.ServiceName):              {{Entity: EntityNotification, Action: ActionAccess}},
		fmt.Sprintf("/%s/RemoveWebhook", fulminev1.NotificationService_ServiceDesc.ServiceName):           {{Entity: EntityNotification, Action: ActionAccess}},
		fmt.Sprintf("/%s/ListWebhooks", fulminev1.NotificationService_ServiceDesc.ServiceName):            {{Entity: EntityNotification, Action: ActionAccess}},
	}
}

// AllPermissionsByMethod combines whitelisted and protected methods
func AllPermissionsByMethod() map[string][]bakery.Op {
	all := make(map[string][]bakery.Op)
	for k, v := range WhitelistedByMethod() {
		all[k] = v
	}
	for k, v := range ProtectedByMethod() {
		all[k] = v
	}
	return all
}
