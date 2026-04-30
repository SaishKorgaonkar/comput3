package chain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Provider mirrors the on-chain ProviderRegistry.Provider struct.
type Provider struct {
	Wallet        gethcommon.Address
	Endpoint      string
	PricePerHour  *big.Int
	StakedAmount  *big.Int
	SlashCount    *big.Int
	JobsCompleted *big.Int
	Active        bool
}

var providerABI abi.ABI

func init() {
	const abiJSON = `[{
		"name": "getActiveProviders",
		"type": "function",
		"stateMutability": "view",
		"inputs": [],
		"outputs": [{
			"name": "",
			"type": "tuple[]",
			"components": [
				{"name": "wallet",        "type": "address"},
				{"name": "endpoint",      "type": "string"},
				{"name": "pricePerHour",  "type": "uint256"},
				{"name": "stakedAmount",  "type": "uint256"},
				{"name": "slashCount",    "type": "uint256"},
				{"name": "jobsCompleted", "type": "uint256"},
				{"name": "active",        "type": "bool"}
			]
		}]
	}]`
	var err error
	providerABI, err = abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		panic(fmt.Sprintf("chain: invalid provider ABI: %v", err))
	}
}

// GetActiveProviders returns all active providers from the ProviderRegistry.
func GetActiveProviders(ctx context.Context, rpcURL, registryAddress string) ([]Provider, error) {
	client := newRPCClient(rpcURL)
	selector := crypto.Keccak256([]byte("getActiveProviders()"))[:4]
	calldata := "0x" + hex.EncodeToString(selector)

	result, err := client.call(ctx, "eth_call", map[string]string{
		"to":   registryAddress,
		"data": calldata,
	}, "latest")
	if err != nil {
		return nil, fmt.Errorf("eth_call getActiveProviders: %w", err)
	}

	var hexStr string
	if err := json.Unmarshal(result, &hexStr); err != nil {
		return nil, fmt.Errorf("decode eth_call result: %w", err)
	}
	hexStr = strings.TrimPrefix(hexStr, "0x")
	rawBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("decode hex: %w", err)
	}

	out, err := providerABI.Methods["getActiveProviders"].Outputs.Unpack(rawBytes)
	if err != nil {
		return nil, fmt.Errorf("abi unpack providers: %w", err)
	}
	if len(out) == 0 {
		return []Provider{}, nil
	}

	raw, err := json.Marshal(out[0])
	if err != nil {
		return nil, err
	}

	type onChainProvider struct {
		Wallet        gethcommon.Address `json:"wallet"`
		Endpoint      string             `json:"endpoint"`
		PricePerHour  *big.Int           `json:"pricePerHour"`
		StakedAmount  *big.Int           `json:"stakedAmount"`
		SlashCount    *big.Int           `json:"slashCount"`
		JobsCompleted *big.Int           `json:"jobsCompleted"`
		Active        bool               `json:"active"`
	}
	var providers []onChainProvider
	if err := json.Unmarshal(raw, &providers); err != nil {
		return nil, fmt.Errorf("unmarshal providers: %w", err)
	}

	out2 := make([]Provider, len(providers))
	for i, p := range providers {
		out2[i] = Provider{
			Wallet:        p.Wallet,
			Endpoint:      p.Endpoint,
			PricePerHour:  p.PricePerHour,
			StakedAmount:  p.StakedAmount,
			SlashCount:    p.SlashCount,
			JobsCompleted: p.JobsCompleted,
			Active:        p.Active,
		}
	}
	return out2, nil
}

// SelectCheapestProvider queries the registry and returns the cheapest active provider.
// Falls back to a local COMPUT3 node sentinel if none are available.
func SelectCheapestProvider(ctx context.Context, rpcURL, registryAddress string) (*Provider, error) {
	providers, err := GetActiveProviders(ctx, rpcURL, registryAddress)
	if err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		// Fallback: COMPUT3 local node
		return &Provider{
			Endpoint:     "http://localhost:8081",
			PricePerHour: big.NewInt(0),
			Active:       true,
		}, nil
	}
	sort.Slice(providers, func(i, j int) bool {
		cmp := providers[i].PricePerHour.Cmp(providers[j].PricePerHour)
		if cmp != 0 {
			return cmp < 0
		}
		return providers[i].JobsCompleted.Cmp(providers[j].JobsCompleted) > 0
	})
	return &providers[0], nil
}
