package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ArkLabsHQ/fulmine/examples"
	"github.com/ArkLabsHQ/fulmine/pkg/swap"
)

const (
	cmdGetInvoice = "get-invoice"
	cmdPayInvoice = "pay-invoice"
	cmdGetAddress = "get-address"
	cmdBalance    = "balance"
	cmdHelp       = "help"
	cmdExit       = "exit"
)

type cliConfig struct {
	ServerURL      string
	ExplorerURL    string
	BoltzURL       string
	BoltzWSURL     string
	SwapTimeout    uint
	CommandTimeout time.Duration
}

func main() {
	cfg, err := parseConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(2)
	}

	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func parseConfig() (cliConfig, error) {
	var cfg cliConfig

	fs := flag.NewFlagSet("exampleswap", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	fs.StringVar(&cfg.ServerURL, "server-url", "http://localhost:7070", "ARK gRPC server URL")
	fs.StringVar(&cfg.ExplorerURL, "explorer-url", "http://localhost:3000", "explorer URL")
	fs.StringVar(&cfg.BoltzURL, "boltz-url", "http://localhost:9001", "Boltz REST API URL")
	fs.StringVar(&cfg.BoltzWSURL, "boltz-ws-url", "ws://localhost:9004", "Boltz websocket URL")
	fs.UintVar(&cfg.SwapTimeout, "swap-timeout", 3600, "Swap timeout in seconds")
	fs.DurationVar(&cfg.CommandTimeout, "command-timeout", 2*time.Minute, "Per-command timeout")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return cliConfig{}, err
	}

	return cfg, cfg.validate()
}

func (c cliConfig) validate() error {
	switch {
	case c.ServerURL == "":
		return errors.New("server-url is required")
	case c.ExplorerURL == "":
		return errors.New("explorer-url is required")
	case c.BoltzURL == "":
		return errors.New("boltz-url is required")
	case c.BoltzWSURL == "":
		return errors.New("boltz-ws-url is required")
	case c.CommandTimeout <= 0:
		return errors.New("command-timeout must be positive")
	default:
		return nil
	}
}

func run(cfg cliConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.CommandTimeout)
	defer cancel()

	client, err := examples.NewSwapExampleClient(
		ctx,
		cfg.ServerURL,
		cfg.ExplorerURL,
		cfg.BoltzURL,
		cfg.BoltzWSURL,
		uint32(cfg.SwapTimeout),
	)
	if err != nil {
		return fmt.Errorf("init swap client: %w", err)
	}

	fmt.Println("Interactive Fulmine swap REPL.")
	fmt.Println("Type 'help' to list commands or 'exit' to quit.")

	return replLoop(client, cfg.CommandTimeout)
}

func replLoop(client *examples.SwapExampleClient, timeout time.Duration) error {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("exampleswap> ")

		if !scanner.Scan() {
			fmt.Println()
			return scanner.Err()
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		cmd, args := parseLine(line)
		if cmd == cmdExit || cmd == "quit" {
			return nil
		}

		if cmd == cmdHelp {
			printHelp()
			continue
		}

		if err := executeCommand(client, timeout, cmd, args); err != nil {
			fmt.Fprintf(os.Stderr, "command error: %v\n", err)
		}
	}
}

func parseLine(line string) (string, []string) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", nil
	}

	cmd := strings.ToLower(fields[0])
	args := fields[1:]
	return cmd, args
}

func executeCommand(client *examples.SwapExampleClient, timeout time.Duration, cmd string, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	switch cmd {
	case cmdGetInvoice:
		if len(args) != 1 {
			return errors.New("usage: get-invoice <amount sats>")
		}

		amount, err := strconv.ParseUint(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid amount: %w", err)
		}

		invoice, err := client.GetSwapInvoice(ctx, amount)
		if err != nil {
			return fmt.Errorf("get swap invoice: %w", err)
		}

		fmt.Printf("Invoice (%d sats): %s\n", amount, invoice)
	case cmdPayInvoice:
		if len(args) != 1 {
			return errors.New("usage: pay-invoice <invoice>")
		}

		swapDetails, err := client.PayInvoice(ctx, args[0])
		if err != nil {
			return fmt.Errorf("pay invoice: %w", err)
		}

		switch swapDetails.Status {
		case swap.SwapSuccess:
			fmt.Printf("Swap %s succeeded!\n", swapDetails.Id)
		case swap.SwapFailed:
			fmt.Printf("Swap %s failed!\n", swapDetails.Id)
		default:
			fmt.Printf("Swap %s status: %d\n", swapDetails.Id, int(swapDetails.Status))
		}

	case cmdGetAddress:
		if len(args) != 0 {
			return errors.New("usage: get-address")
		}

		onchain, offchain, err := client.GetAddress(ctx)
		if err != nil {
			return fmt.Errorf("get address: %w", err)
		}

		fmt.Printf("On-chain address:  %s\n", onchain)
		fmt.Printf("Off-chain address: %s\n", offchain)
	case cmdBalance:
		if len(args) != 0 {
			return errors.New("usage: balance")
		}

		total, err := client.Balance(ctx)
		if err != nil {
			return fmt.Errorf("get balance: %w", err)
		}

		fmt.Printf("Off-chain balance: %d sats\n", total)
	default:
		return fmt.Errorf("unknown command %q (type 'help' for options)", cmd)
	}

	return nil
}

func printHelp() {
	fmt.Println("Commands:")
	fmt.Println("  get-invoice <sats>  - request a swap invoice for the given amount")
	fmt.Println("  pay-invoice <bolt11>- pay a Lightning invoice via a swap")
	fmt.Println("  get-address         - show fresh on-chain and off-chain receive addresses")
	fmt.Println("  balance             - show total off-chain balance (after settlement)")
	fmt.Println("  help                - show this help text")
	fmt.Println("  exit|quit           - leave the REPL")
}
