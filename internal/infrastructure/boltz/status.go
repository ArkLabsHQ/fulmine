package boltz

type SwapUpdateEvent int

const (
	SwapCreated SwapUpdateEvent = iota
	SwapExpired

	InvoiceSet
	InvoicePaid
	InvoicePending
	InvoiceSettled
	InvoiceFailedToPay

	ChannelCreated

	TransactionFailed
	TransactionMempool
	TransactionClaimed
	TransactionRefunded
	TransactionConfirmed
	TransactionLockupFailed
	TransactionClaimPending

	TransactionServerMempoool
	TransactionServerConfirmed

	TransactionDirect
	TransactionDirectMempool
)

var swapUpdateEventStrings = map[string]SwapUpdateEvent{
	"swap.created": SwapCreated,
	"swap.expired": SwapExpired,

	"invoice.set":         InvoiceSet,
	"invoice.paid":        InvoicePaid,
	"invoice.pending":     InvoicePending,
	"invoice.settled":     InvoiceSettled,
	"invoice.failedToPay": InvoiceFailedToPay,

	"channel.created": ChannelCreated,

	"transaction.failed":           TransactionFailed,
	"transaction.mempool":          TransactionMempool,
	"transaction.claimed":          TransactionClaimed,
	"transaction.refunded":         TransactionRefunded,
	"transaction.confirmed":        TransactionConfirmed,
	"transaction.lockupFailed":     TransactionLockupFailed,
	"transaction.claim.pending":    TransactionClaimPending,
	"transaction.server.mempool":   TransactionServerMempoool,
	"transaction.server.confirmed": TransactionServerConfirmed,

	"transaction.direct":         TransactionDirect,
	"transaction.direct.mempool": TransactionDirectMempool,
}

func ParseEvent(event string) SwapUpdateEvent {
	return swapUpdateEventStrings[event]
}
