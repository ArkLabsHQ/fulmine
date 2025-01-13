package main

import (
	"context"
	"fmt"
	"log"

	cln_grpc "github.com/ArkLabsHQ/ark-node/internal/infrastructure/cln-grpc"
)

const (
	datadir = "path/to//Nigiri/volumes/lightningd/regtest"
	invoice = "lnbcrtpicyourinvoice"
	clnUrl  = "localhost:9936"
)

func main() {
	lnSvc := cln_grpc.NewService()
	if err := lnSvc.Connect(context.Background(), clnUrl); err != nil {
		log.Fatalf("failed to connect to cln: %s", err)
	}

	fmt.Println(lnSvc.IsConnected())

	fmt.Println(lnSvc.GetInfo(context.Background()))

	fmt.Println(lnSvc.PayInvoice(context.Background(), invoice))
}
