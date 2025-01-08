package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	cln_grpc "github.com/ArkLabsHQ/ark-node/internal/infrastructure/cln-grpc"
)

const datadir = "path/to//Nigiri/volumes/lightningd/regtest"
const invoice = "lnbcrtpicyourinvoice"

func main() {
	rootCert := filepath.Join(datadir, "ca.pem")
	privateKey := filepath.Join(datadir, "client-key.pem")
	certChain := filepath.Join(datadir, "client.pem")
	lnSvc, err := cln_grpc.NewService(rootCert, privateKey, certChain)
	if err != nil {
		log.Fatalf("failed to init cln client: %s", err)
	}
	// lnSvc := cln.NewService()
	if err := lnSvc.Connect(context.Background(), "localhost:9936"); err != nil {
		log.Fatalf("failed to connect to cln: %s", err)
	}

	fmt.Println(lnSvc.IsConnected())

	fmt.Println(lnSvc.GetInfo(context.Background()))

	fmt.Println(lnSvc.PayInvoice(context.Background(), invoice))
}
