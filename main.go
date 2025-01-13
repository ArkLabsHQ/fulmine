package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ArkLabsHQ/ark-node/internal/core/ports"
	cln "github.com/ArkLabsHQ/ark-node/internal/infrastructure/cln-grpc"
	"github.com/ArkLabsHQ/ark-node/internal/infrastructure/lnd"
)

const clnUrl = "clnconnect://localhost:9936?rootCert=MIIBfTCCASOgAwIBAgIULHjEUPAi_9LppwVZSrNzIWuzxPAwCgYIKoZIzj0EAwIwFjEUMBIGA1UEAwwLY2xuIFJvb3QgQ0EwIBcNNzUwMTAxMDAwMDAwWhgPNDA5NjAxMDEwMDAwMDBaMBYxFDASBgNVBAMMC2NsbiBSb290IENBMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEeIxODYi1-lYTk_PHC3OQvM57UiXSYI-X_8gAVcI92ndw7tJSZk_atfRZ4wa8Uq0c4eRD1eUIIGAwoUp4yWdze6NNMEswGQYDVR0RBBIwEIIDY2xugglsb2NhbGhvc3QwHQYDVR0OBBYEFAKHK9Upgnu8X6yI8JSbGhWyvgFpMA8GA1UdEwEB_wQFMAMBAf8wCgYIKoZIzj0EAwIDSAAwRQIgbDH-dox70a5Wcr6bam76ubxZYJczEXzLhvCK0ReLKyoCIQDDt8LHmftXkYojuj2-agF9qjTBk4wwc2V6URcRAxgD3Q&privateKey=MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgKLbb89-I3OQ_UFXrNUsm5RMiRfG3LGn2ohOlNVUMP4qhRANCAARoC4y-zWcTUZVXcMQR5nIIrU5mkLe1lGp2x1RygKCmdm_LWw7mt0yEzFhfJC8P7_ufaJgYp8ceDIuWg5KwZUrQ&certChain=MIIBUTCB96ADAgECAhRhkqP1qfoXnEFiv9lBJfTQk29ugDAKBggqhkjOPQQDAjAWMRQwEgYDVQQDDAtjbG4gUm9vdCBDQTAgFw03NTAxMDEwMDAwMDBaGA80MDk2MDEwMTAwMDAwMFowGjEYMBYGA1UEAwwPY2xuIGdycGMgQ2xpZW50MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEaAuMvs1nE1GVV3DEEeZyCK1OZpC3tZRqdsdUcoCgpnZvy1sO5rdMhMxYXyQvD-_7n2iYGKfHHgyLloOSsGVK0KMdMBswGQYDVR0RBBIwEIIDY2xugglsb2NhbGhvc3QwCgYIKoZIzj0EAwIDSQAwRgIhAOeS-E7gzN4Htf1CLhf6ekQFVmMKPlbm_CnGOp4fKPG4AiEAz38mVoplsl1N648yxHSV7SfVwDHpGZhHJredCE7Xhes"

const lndUrl = "lndconnect://localhost:10009?macaroon=AgEDbG5kAvgBAwoQew1ylL1w9HE4_xm5gzIvaBIBMBoWCgdhZGRyZXNzEgRyZWFkEgV3cml0ZRoTCgRpbmZvEgRyZWFkEgV3cml0ZRoXCghpbnZvaWNlcxIEcmVhZBIFd3JpdGUaIQoIbWFjYXJvb24SCGdlbmVyYXRlEgRyZWFkEgV3cml0ZRoWCgdtZXNzYWdlEgRyZWFkEgV3cml0ZRoXCghvZmZjaGFpbhIEcmVhZBIFd3JpdGUaFgoHb25jaGFpbhIEcmVhZBIFd3JpdGUaFAoFcGVlcnMSBHJlYWQSBXdyaXRlGhgKBnNpZ25lchIIZ2VuZXJhdGUSBHJlYWQAAAYgFy5zZLlEBkHkdKjc6a8nLHGX8N1AoQ1p0B21GyV_Us8&cert=MIICKjCCAdCgAwIBAgIRAN3eyvJgpC9gBJmrN_UjeUswCgYIKoZIzj0EAwIwODEfMB0GA1UEChMWbG5kIGF1dG9nZW5lcmF0ZWQgY2VydDEVMBMGA1UEAxMMMzYxMDNiNjVlNGJjMB4XDTI1MDEwODE1MDEyMFoXDTI2MDMwNTE1MDEyMFowODEfMB0GA1UEChMWbG5kIGF1dG9nZW5lcmF0ZWQgY2VydDEVMBMGA1UEAxMMMzYxMDNiNjVlNGJjMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE3DULnOLF4nM9KWm2YRAvCneic3hhmoyNQdMaeDIB-uRrc6ZIEY8LATN4UDHM14BBgOeRbwiNRyEBog-_-cg0sKOBujCBtzAOBgNVHQ8BAf8EBAMCAqQwEwYDVR0lBAwwCgYIKwYBBQUHAwEwDwYDVR0TAQH_BAUwAwEB_zAdBgNVHQ4EFgQUn9-dBQ0z1PFgxZ4ERMqfgw0sQ0cwYAYDVR0RBFkwV4IMMzYxMDNiNjVlNGJjgglsb2NhbGhvc3SCA2xuZIIEdW5peIIKdW5peHBhY2tldIIHYnVmY29ubocEfwAAAYcQAAAAAAAAAAAAAAAAAAAAAYcEwKhhBDAKBggqhkjOPQQDAgNIADBFAiEAqk5evAHc0oL19xzMpH9ZakxHxYs7NdQ0yTJgsJAMfwQCIFdC1EV2X43aCnlj63vzoJ4p3FLz46mu7oJTEzkwk7_-"

func connectCln() ports.LnService {
	svc := cln.NewService()
	if err := svc.Connect(context.Background(), clnUrl); err != nil {
		log.Fatalf("failed to connect to cln: %s", err)
	}
	return svc
}

func connectLnd() ports.LnService {
	svc := lnd.NewService()
	if err := svc.Connect(context.Background(), lndUrl); err != nil {
		log.Fatalf("failed to connect to lnd: %s", err)
	}
	return svc
}

func main() {
	fmt.Println("---- LND ----")
	lndSvc := connectLnd()
	fmt.Println("Is Connected:", lndSvc.IsConnected())
	fmt.Println(lndSvc.GetInfo(context.Background()))

	fmt.Println("---- CLN ----")
	clnSvc := connectCln()
	fmt.Println("Is Connected:", clnSvc.IsConnected())
	fmt.Println(clnSvc.GetInfo(context.Background()))

	// fmt.Println(lnSvc.PayInvoice(context.Background(), invoice))
}
