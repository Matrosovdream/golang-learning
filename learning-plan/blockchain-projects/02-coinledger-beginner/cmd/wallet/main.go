// Command wallet manages keys and sends transactions to a coinledger node.
//
//	wallet new                                   create a key (encrypted keystore file)
//	wallet list                                  addresses in the keystore
//	wallet balance -address 0x…                  balance and nonces
//	wallet send -from 0x… -to 0x… -amount 5      sign locally and broadcast
//	wallet send … -nonce 3 -offline              sign only; print the raw tx
//	wallet submit -raw 0x…                       broadcast a raw tx signed elsewhere
//	wallet tx -hash 0x…                          status of a tx
//	wallet verify -hash 0x…                      light-client check of a mined tx
//
// Every command accepts -node (default $COINLEDGER_NODE or http://localhost:8080)
// and -keystore (default ./keystore). The password comes from $WALLET_PASSWORD
// or is read from stdin.
package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"coinledger/internal/domain"
	"coinledger/internal/wallet"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmd, args := os.Args[1], os.Args[2:]
	commands := map[string]func([]string) error{
		"new": cmdNew, "list": cmdList, "balance": cmdBalance, "send": cmdSend,
		"submit": cmdSubmit, "tx": cmdTx, "verify": cmdVerify,
	}
	run, ok := commands[cmd]
	if !ok {
		usage()
	}
	if err := run(args); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: wallet new|list|balance|send|submit|tx|verify [flags]  (-h on any command for its flags)")
	os.Exit(2)
}

// common holds the flags every command shares.
type common struct {
	node     *string
	keystore *string
}

func newFlags(name string) (*flag.FlagSet, common) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	node := os.Getenv("COINLEDGER_NODE")
	if node == "" {
		node = "http://localhost:8080"
	}
	return fs, common{
		node:     fs.String("node", node, "node URL"),
		keystore: fs.String("keystore", "./keystore", "keystore directory"),
	}
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Minute)
}

func cmdNew(args []string) error {
	fs, c := newFlags("new")
	fs.Parse(args)
	password, err := readPassword("password for the new key: ")
	if err != nil {
		return err
	}
	acct, err := wallet.OpenKeystore(*c.keystore).NewAccount(password)
	if err != nil {
		return err
	}
	fmt.Printf("address: %s\nkey file: %s\n", acct.Address.Hex(), acct.URL.Path)
	return nil
}

func cmdList(args []string) error {
	fs, c := newFlags("list")
	fs.Parse(args)
	for _, acct := range wallet.OpenKeystore(*c.keystore).Accounts() {
		fmt.Println(acct.Address.Hex())
	}
	return nil
}

func cmdBalance(args []string) error {
	fs, c := newFlags("balance")
	address := fs.String("address", "", "account address")
	fs.Parse(args)
	ctx, cancel := ctx()
	defer cancel()
	acct, err := wallet.NewClient(*c.node).Account(ctx, *address)
	if err != nil {
		return err
	}
	fmt.Printf("address:       %s\nbalance:       %s\nnonce:         %d  (transactions mined)\npending nonce: %d  (next nonce to use)\nat height:     %d\n",
		acct.Address, acct.Balance, acct.Nonce, acct.PendingNonce, acct.AtHeight)
	return nil
}

func cmdSend(args []string) error {
	fs, c := newFlags("send")
	from := fs.String("from", "", "sender address (must be in the keystore)")
	to := fs.String("to", "", "recipient address")
	amount := fs.Uint64("amount", 0, "coins to send")
	fee := fs.Uint64("fee", 1, "fee for the miner")
	nonce := fs.Int64("nonce", -1, "nonce to use (default: ask the node for the pending nonce)")
	chainID := fs.Uint64("chain-id", 1337, "chain id to sign for")
	offline := fs.Bool("offline", false, "sign only and print the raw tx (requires -nonce)")
	wait := fs.Bool("wait", false, "wait until the tx is mined")
	fs.Parse(args)

	fromAddr, err := domain.ParseAddress(*from)
	if err != nil {
		return fmt.Errorf("-from: %w", err)
	}
	toAddr, err := domain.ParseAddress(*to)
	if err != nil {
		return fmt.Errorf("-to: %w", err)
	}
	ctx, cancel := ctx()
	defer cancel()
	client := wallet.NewClient(*c.node)

	// The nonce is the one thing a wallet must learn from the chain: the next
	// number in this account's sequence. Offline signing needs it given by hand.
	if *nonce < 0 {
		if *offline {
			return errors.New("-offline needs an explicit -nonce: there is no node to ask")
		}
		acct, err := client.Account(ctx, fromAddr.Hex())
		if err != nil {
			return err
		}
		*nonce = int64(acct.PendingNonce)
	}

	ks := wallet.OpenKeystore(*c.keystore)
	signer, err := wallet.FindAccount(ks, fromAddr)
	if err != nil {
		return err
	}
	password, err := readPassword("password for " + fromAddr.Hex() + ": ")
	if err != nil {
		return err
	}
	tx, err := wallet.SignTx(ks, signer, password, domain.Tx{
		ChainID: *chainID, Nonce: uint64(*nonce), To: toAddr, Amount: *amount, Fee: *fee,
	})
	if err != nil {
		return err
	}
	raw := tx.Encode()
	fmt.Printf("signed tx %s (nonce %d, %d bytes)\n", tx.Hash().Hex(), tx.Nonce, len(raw))

	if *offline {
		fmt.Printf("raw: 0x%s\nbroadcast later with: wallet submit -raw 0x%s\n", hex.EncodeToString(raw), hex.EncodeToString(raw))
		return nil
	}
	return broadcast(ctx, client, raw, *wait)
}

func cmdSubmit(args []string) error {
	fs, c := newFlags("submit")
	rawHex := fs.String("raw", "", "0x-prefixed raw transaction")
	wait := fs.Bool("wait", false, "wait until the tx is mined")
	fs.Parse(args)
	raw, err := hex.DecodeString(strings.TrimPrefix(*rawHex, "0x"))
	if err != nil {
		return fmt.Errorf("-raw: %w", err)
	}
	ctx, cancel := ctx()
	defer cancel()
	return broadcast(ctx, wallet.NewClient(*c.node), raw, *wait)
}

func broadcast(ctx context.Context, client *wallet.Client, raw []byte, wait bool) error {
	res, err := client.Submit(ctx, raw)
	if err != nil {
		return err
	}
	fmt.Printf("accepted by node: %s from %s (%s)\n", res.Hash, res.From, res.Status)
	if !wait {
		return nil
	}
	for {
		info, err := client.Tx(ctx, res.Hash)
		if err != nil {
			return err
		}
		if info.Status == "included" {
			fmt.Printf("mined in block %d (confirmations: %d)\n", *info.BlockHeight, info.Confirmations)
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("still %s: %w", info.Status, ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func cmdTx(args []string) error {
	fs, c := newFlags("tx")
	hash := fs.String("hash", "", "transaction hash")
	fs.Parse(args)
	ctx, cancel := ctx()
	defer cancel()
	info, err := wallet.NewClient(*c.node).Tx(ctx, *hash)
	if err != nil {
		return err
	}
	fmt.Printf("hash:   %s\nstatus: %s\nfrom:   %s\nto:     %s\namount: %s  fee: %s  nonce: %d\n",
		info.Hash, info.Status, info.From, info.To, info.Amount, info.Fee, info.Nonce)
	if info.BlockHeight != nil {
		fmt.Printf("block:  %d (confirmations: %d)\n", *info.BlockHeight, info.Confirmations)
	}
	return nil
}

func cmdVerify(args []string) error {
	fs, c := newFlags("verify")
	hashStr := fs.String("hash", "", "transaction hash")
	minDifficulty := fs.Uint("min-difficulty", 18, "reject headers claiming less work than this")
	fs.Parse(args)
	txHash, err := domain.ParseHash(*hashStr)
	if err != nil {
		return err
	}
	ctx, cancel := ctx()
	defer cancel()
	client := wallet.NewClient(*c.node)

	info, err := client.Tx(ctx, txHash.Hex())
	if err != nil {
		return err
	}
	if info.BlockHeight == nil {
		return fmt.Errorf("tx is %s, not mined yet", info.Status)
	}
	proof, err := client.Proof(ctx, *info.BlockHeight, txHash.Hex())
	if err != nil {
		return err
	}
	v, err := wallet.VerifyInclusion(proof, txHash, uint8(*minDifficulty))
	if err != nil {
		return err
	}

	mark := map[bool]string{true: "✔", false: "✘"}
	fmt.Printf("header (%d bytes) for block %d\n", domain.HeaderSize, v.Header.Height)
	fmt.Printf("  %s block hash computed locally: %s\n", mark[v.HeightOK], v.BlockHash.Hex())
	fmt.Printf("  %s proof-of-work: %d leading zero bits, header difficulty %d, minimum %d\n",
		mark[v.WorkOK], v.LeadingZeros, v.Header.Difficulty, *minDifficulty)
	fmt.Printf("merkle path (%d steps, index %d):\n", len(proof.Steps), proof.Index)
	for i, s := range proof.Steps {
		fmt.Printf("  %d. sibling on the %-5s %s\n", i+1, s.Position, s.Sibling)
	}
	fmt.Printf("  %s recomputed root matches header tx_root %s\n", mark[v.ProofOK], v.Header.TxRoot.Hex())
	if !v.Valid() {
		return errors.New("verification FAILED")
	}
	fmt.Println("verified: the transaction is committed to by a header with valid proof-of-work")
	return nil
}

// readPassword takes $WALLET_PASSWORD, or reads a line from stdin.
// (Stdin echoes the password; a real wallet disables terminal echo.)
func readPassword(prompt string) (string, error) {
	if p := os.Getenv("WALLET_PASSWORD"); p != "" {
		return p, nil
	}
	fmt.Fprint(os.Stderr, prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read password: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}
