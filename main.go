package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/ArkLabsHQ/ark-node/internal/core/ports"
	"github.com/ArkLabsHQ/ark-node/internal/infrastructure/cln"
	"github.com/ArkLabsHQ/ark-node/internal/infrastructure/lnd"
)

func runScript(path string) string {
	url, err := exec.Command(path).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(url))
}

func connect(kind string) ports.LnService {
	svc := lnd.NewService()
	url := runScript("./scripts/lndconnect.sh")
	if kind == "cln" {
		svc = cln.NewService()
		url = runScript("./scripts/clnconnect.sh")
	}
	if url == "" {
		log.Fatalf("failed to get %s connect url", kind)
	}
	if err := svc.Connect(context.Background(), url); err != nil {
		log.Fatalf("failed to connect to %s: %s", kind, err)
	}
	return svc
}

func main() {
	fmt.Println("---- LND ----")
	lndSvc := connect("lnd")
	fmt.Println("Is Connected:", lndSvc.IsConnected())
	fmt.Println(lndSvc.GetInfo(context.Background()))
	fmt.Println(lndSvc.GetInvoice(context.Background(), 21000, "a note", ""))

	fmt.Println("---- CLN ----")
	clnSvc := connect("cln")
	fmt.Println("Is Connected:", clnSvc.IsConnected())
	fmt.Println(clnSvc.GetInfo(context.Background()))
	fmt.Println(clnSvc.GetInvoice(context.Background(), 21000, "a note", ""))

}
