package cln

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/ArkLabsHQ/ark-node/internal/core/ports"
	"github.com/google/uuid"
)

const (
	clnGetInfoCmd       = "getinfo"
	clnCreateInvoiceCmd = "invoice"
	clnPayInvoiceCmd    = "pay"
)

type service struct {
}

func NewService() ports.LnService {
	return &service{}
}

func (s service) Connect(ctx context.Context, _ string) error {
	_, _, err := s.GetInfo(context.Background())
	return err
}

func (s service) IsConnected() bool {
	_, _, err := s.GetInfo(context.Background())
	return err == nil
}

func (s service) GetInfo(ctx context.Context) (version string, pubkey string, err error) {
	resp, err := runCommand(clnGetInfoCmd)
	if err != nil {
		return "", "", err
	}

	info := GetInfoResponse{}
	if err := json.Unmarshal([]byte(resp), &info); err != nil {
		return "", "", err
	}

	return info.Version, info.Id, nil
}

func (s service) GetInvoice(ctx context.Context, value uint64, note string) (invoice string, preimageHash string, err error) {
	label := uuid.New()
	amountMsats := value * 1000
	resp, err := runCommand(
		clnCreateInvoiceCmd,
		"-k",
		fmt.Sprintf("%v=%v", "amount_msat", strconv.Itoa(int(amountMsats))),
		fmt.Sprintf("%v=%v", "label", label.String()),
		fmt.Sprintf("%v=%v", "description", note),
	)
	if err != nil {
		return "", "", err
	}

	invoiceResp := CreateInvoiceResponse{}
	if err := json.Unmarshal([]byte(resp), &invoiceResp); err != nil {
		return "", "", err
	}

	return invoiceResp.Bolt11, invoiceResp.PaymentHash, nil
}

func (s service) PayInvoice(ctx context.Context, invoice string) (preimage string, err error) {
	resp, err := runCommand(clnPayInvoiceCmd, invoice)
	if err != nil {
		return "", err
	}

	payInvResp := PayInvoiceResponse{}
	if err := json.Unmarshal([]byte(resp), &payInvResp); err != nil {
		return "", err
	}

	return payInvResp.PaymentPreimage, nil
}

func (s service) Disconnect() {}
