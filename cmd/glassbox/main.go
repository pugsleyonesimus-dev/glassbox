// Command glassbox is the CLI entry point for the Glassbox Stellar transaction
// debugger.  It wires together the correlation, network, cache, RPC, and
// diagnostics packages into a single coherent debug session.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/drips/glassbox/internal/cache"
	"github.com/drips/glassbox/internal/correlation"
	"github.com/drips/glassbox/internal/diagnostics"
	"github.com/drips/glassbox/internal/network"
	"github.com/drips/glassbox/internal/rpc"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "glassbox: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("glassbox", flag.ContinueOnError)

	var (
		endpoint   = fs.String("endpoint", "", "Soroban RPC endpoint URL (required)")
		passphrase = fs.String("passphrase", "", "Stellar network passphrase (required)")
		txHash     = fs.String("tx", "", "Transaction hash to debug (required)")
		jsonOut    = fs.Bool("json", false, "Emit diagnostics as JSON instead of text")
		allowXNet  = fs.Bool("allow-cross-network", false, "Allow replay against a different network passphrase")
	)

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}

	if *endpoint == "" || *passphrase == "" || *txHash == "" {
		fs.Usage()
		return fmt.Errorf("-endpoint, -passphrase, and -tx are required")
	}

	// --- Correlation ID ---
	ctx, corrID := correlation.Ensure(context.Background())
	fmt.Fprintf(os.Stderr, "correlation-id: %s\n", corrID)

	// --- Network snapshot ---
	netSnap := network.Snapshot{
		NetworkName: "custom",
		Passphrase:  *passphrase,
		RPCEndpoint: *endpoint,
	}

	// --- Cache ---
	store := cache.New(cache.Options{})

	// --- RPC client ---
	var hooks []rpc.Hook
	hooks = append(hooks, func(e rpc.Event) {
		fmt.Fprintf(os.Stderr, "[rpc] corr=%s kind=%s method=%s\n",
			e.CorrelationID, e.Kind, e.Method)
	})

	client, err := rpc.NewClient(rpc.ClientOptions{
		Endpoint:          *endpoint,
		Cache:             store,
		NetworkPassphrase: *passphrase,
		Hooks:             hooks,
	})
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	// --- Diagnostics builder ---
	var compareOpts []network.CompareOption
	if *allowXNet {
		compareOpts = append(compareOpts, network.AllowPassphraseMismatch())
	}

	builder := diagnostics.NewBuilder(corrID, netSnap)

	// --- Fetch transaction ---
	txResp, err := client.GetTransaction(ctx, *txHash)
	if err != nil {
		builder.AddNote(fmt.Sprintf("GetTransaction error: %v", err))
	} else {
		builder.AddNote(fmt.Sprintf("transaction status: %s", txResp.Status))
	}

	// Apply network compare opts for report.
	if len(compareOpts) > 0 {
		result := network.Compare(netSnap, netSnap, compareOpts...)
		builder.SetNetworkOverrides(result.ActiveOverrides)
	}

	builder.SetCacheStats(store.Diagnostics())

	report := builder.Build()

	// --- Emit report ---
	if *jsonOut {
		return diagnostics.WriteJSON(os.Stdout, report)
	}
	return diagnostics.WriteText(os.Stdout, report)
}
