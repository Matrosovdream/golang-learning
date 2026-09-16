// Command chainctl drives a p2pchain network: inspect nodes, send transactions,
// generate traffic, and stage a double spend across a partition.
//
//	chainctl keys                                   list the dev accounts
//	chainctl status -nodes a,b,c                    compare nodes side by side
//	chainctl send -node a -from 0 -to 1 -amount 5    sign with a dev key and submit
//	chainctl spam -node a -txs 20                    generate traffic
//	chainctl double-spend -a urlA -b urlB -from 0    the same coins, twice, on two nodes
//	chainctl watch -node a                          follow a node's event stream
//
// The dev keys are PUBLIC (see domain/devkeys.go): local demos only.
package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"p2pchain/internal/domain"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmds := map[string]func([]string) error{
		"keys": cmdKeys, "status": cmdStatus, "send": cmdSend,
		"spam": cmdSpam, "double-spend": cmdDoubleSpend, "watch": cmdWatch,
	}
	run, ok := cmds[os.Args[1]]
	if !ok {
		usage()
	}
	if err := run(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: chainctl keys|status|send|spam|double-spend|watch [flags]")
	os.Exit(2)
}

func cmdKeys(args []string) error {
	fs := flag.NewFlagSet("keys", flag.ExitOnError)
	n := fs.Int("n", 5, "how many dev accounts to print")
	fs.Parse(args)
	for i := range *n {
		addr, err := domain.DevAddress(i)
		if err != nil {
			return err
		}
		fmt.Printf("dev account %d: %s\n", i, addr.Hex())
	}
	return nil
}

type nodeStatus struct {
	PeerID    string `json:"peer_id"`
	Height    uint64 `json:"height"`
	TipHash   string `json:"tip_hash"`
	TotalWork string `json:"total_work"`
	PeerCount int    `json:"peer_count"`
	Pending   int    `json:"mempool_pending"`
	Queued    int    `json:"mempool_queued"`
	Threads   int    `json:"miner_threads"`
	Mined     uint64 `json:"blocks_mined"`
	Syncing   bool   `json:"syncing"`
	Orphans   int    `json:"orphans"`
}

// cmdStatus is the tool you watch during every experiment: it shows at a
// glance whether the nodes agree (same tip hash) or have forked.
func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	nodes := fs.String("nodes", "http://localhost:8081,http://localhost:8082", "comma-separated node URLs")
	watch := fs.Duration("every", 0, "repeat every interval (0 = once)")
	fs.Parse(args)

	for {
		fmt.Printf("%-28s %7s %14s %6s %7s %7s %6s %s\n", "node", "height", "tip", "peers", "pending", "mined", "thr", "flags")
		tips := map[string]int{}
		for _, url := range strings.Split(*nodes, ",") {
			url = strings.TrimSpace(url)
			var st nodeStatus
			if err := getJSON(url+"/node/status", &st); err != nil {
				fmt.Printf("%-28s %s\n", url, err)
				continue
			}
			tips[st.TipHash]++
			flags := ""
			if st.Syncing {
				flags += "syncing "
			}
			if st.Orphans > 0 {
				flags += fmt.Sprintf("orphans=%d ", st.Orphans)
			}
			fmt.Printf("%-28s %7d %14s %6d %7d %7d %6d %s\n",
				url, st.Height, short(st.TipHash), st.PeerCount, st.Pending+st.Queued, st.Mined, st.Threads, flags)
		}
		switch len(tips) {
		case 0:
		case 1:
			fmt.Println("→ all nodes agree on the same tip")
		default:
			fmt.Printf("→ %d different tips: the network has forked\n", len(tips))
		}
		if *watch == 0 {
			return nil
		}
		time.Sleep(*watch)
		fmt.Println()
	}
}

func cmdSend(args []string) error {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	node := fs.String("node", "http://localhost:8081", "node URL")
	from := fs.Int("from", 0, "sender dev account index")
	to := fs.Int("to", 1, "recipient dev account index")
	amount := fs.Uint64("amount", 10, "coins to send")
	fee := fs.Uint64("fee", 1, "fee")
	nonce := fs.Int64("nonce", -1, "nonce (default: ask the node)")
	chainID := fs.Uint64("chain-id", 1337, "chain id")
	fs.Parse(args)

	tx, err := buildTx(*node, *from, *to, *amount, *fee, *nonce, *chainID)
	if err != nil {
		return err
	}
	res, err := submit(*node, tx)
	if err != nil {
		return err
	}
	fmt.Printf("%s → %s: %s (%s)\n", res["from"], tx.To.Hex(), res["hash"], res["status"])
	return nil
}

func cmdSpam(args []string) error {
	fs := flag.NewFlagSet("spam", flag.ExitOnError)
	node := fs.String("node", "http://localhost:8081", "node URL")
	count := fs.Int("txs", 10, "how many transactions")
	accounts := fs.Int("accounts", 3, "how many dev accounts to send from")
	amount := fs.Uint64("amount", 1, "coins per transaction")
	chainID := fs.Uint64("chain-id", 1337, "chain id")
	fs.Parse(args)

	sent := 0
	for i := range *count {
		from := i % *accounts
		to := (from + 1) % (*accounts + 1)
		tx, err := buildTx(*node, from, to, *amount, uint64(1+i%3), -1, *chainID)
		if err != nil {
			return err
		}
		if _, err := submit(*node, tx); err != nil {
			fmt.Fprintf(os.Stderr, "tx %d: %v\n", i, err)
			continue
		}
		sent++
	}
	fmt.Printf("submitted %d/%d transactions to %s\n", sent, *count, *node)
	return nil
}

// cmdDoubleSpend signs two transactions with the SAME nonce and sends one to
// each node. While the nodes can't talk to each other, both will happily mine
// "their" transaction — and when the partition heals, only one survives.
func cmdDoubleSpend(args []string) error {
	fs := flag.NewFlagSet("double-spend", flag.ExitOnError)
	a := fs.String("a", "http://localhost:8081", "node on one side of the partition")
	b := fs.String("b", "http://localhost:8083", "node on the other side")
	from := fs.Int("from", 0, "sender dev account index")
	toA := fs.Int("to-a", 1, "recipient on side A")
	toB := fs.Int("to-b", 2, "recipient on side B")
	amount := fs.Uint64("amount", 1000, "coins to send twice")
	chainID := fs.Uint64("chain-id", 1337, "chain id")
	fs.Parse(args)

	nonce, err := pendingNonce(*a, *from)
	if err != nil {
		return err
	}
	txA, err := signTx(*from, *toA, *amount, 1, nonce, *chainID)
	if err != nil {
		return err
	}
	txB, err := signTx(*from, *toB, *amount, 1, nonce, *chainID) // same nonce, other recipient
	if err != nil {
		return err
	}
	resA, err := submit(*a, txA)
	if err != nil {
		return fmt.Errorf("side A: %w", err)
	}
	resB, err := submit(*b, txB)
	if err != nil {
		return fmt.Errorf("side B: %w", err)
	}
	fmt.Printf("nonce %d spent twice:\n  A %s → dev %d : %s\n  B %s → dev %d : %s\nboth are valid on their own side; only one can survive the merge\n",
		nonce, *a, *toA, resA["hash"], *b, *toB, resB["hash"])
	return nil
}

func cmdWatch(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	node := fs.String("node", "http://localhost:8081", "node URL")
	fs.Parse(args)

	resp, err := http.Get(*node + "/events")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	fmt.Printf("watching %s (ctrl-c to stop)\n", *node)

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev struct {
			Kind   string         `json:"kind"`
			Time   time.Time      `json:"time"`
			Fields map[string]any `json:"fields"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			continue
		}
		fmt.Printf("%s %-16s %v\n", ev.Time.Format("15:04:05.000"), ev.Kind, ev.Fields)
	}
	return scanner.Err()
}

// --- helpers ----------------------------------------------------------------------

func signTx(from, to int, amount, fee uint64, nonce, chainID uint64) (domain.Tx, error) {
	key, err := domain.DevKey(from)
	if err != nil {
		return domain.Tx{}, err
	}
	toAddr, err := domain.DevAddress(to)
	if err != nil {
		return domain.Tx{}, err
	}
	return domain.SignTx(domain.Tx{ChainID: chainID, Nonce: nonce, To: toAddr, Amount: amount, Fee: fee}, key)
}

func buildTx(node string, from, to int, amount, fee uint64, nonce int64, chainID uint64) (domain.Tx, error) {
	n := uint64(nonce)
	if nonce < 0 {
		var err error
		if n, err = pendingNonce(node, from); err != nil {
			return domain.Tx{}, err
		}
	}
	return signTx(from, to, amount, fee, n, chainID)
}

func pendingNonce(node string, dev int) (uint64, error) {
	addr, err := domain.DevAddress(dev)
	if err != nil {
		return 0, err
	}
	var acct struct {
		PendingNonce uint64 `json:"pending_nonce"`
	}
	if err := getJSON(node+"/accounts/"+addr.Hex(), &acct); err != nil {
		return 0, err
	}
	return acct.PendingNonce, nil
}

func submit(node string, tx domain.Tx) (map[string]string, error) {
	body, _ := json.Marshal(map[string]string{"raw": "0x" + hex.EncodeToString(tx.Encode())})
	resp, err := http.Post(node+"/txs", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: %s (%s)", resp.Status, out["error"], out["code"])
	}
	return out, nil
}

func getJSON(url string, dst any) error {
	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

func short(h string) string {
	if len(h) > 14 {
		return h[:14]
	}
	return h
}
