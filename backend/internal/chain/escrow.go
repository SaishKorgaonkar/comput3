package chain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
)

var escrowABI abi.ABI

func init() {
	const abiJSON = `[
		{
			"name": "deposit",
			"type": "function",
			"stateMutability": "payable",
			"inputs": [
				{"name": "sessionId", "type": "bytes32"},
				{"name": "provider",  "type": "address"}
			],
			"outputs": []
		},
		{
			"name": "release",
			"type": "function",
			"stateMutability": "nonpayable",
			"inputs": [{"name": "sessionId", "type": "bytes32"}],
			"outputs": []
		}
	]`
	var err error
	escrowABI, err = abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		panic(fmt.Sprintf("chain: invalid escrow ABI: %v", err))
	}
}

// DepositEscrow calls deposit(sessionId, provider) on the DeploymentEscrow contract,
// locking depositWei in escrow for the session. The agent wallet must hold sufficient ETH.
func DepositEscrow(
	ctx context.Context,
	rpcURL, privateKeyHex, escrowAddress string,
	sessionID [32]byte,
	provider gethcommon.Address,
	depositWei *big.Int,
) error {
	calldata, err := escrowABI.Pack("deposit", sessionID, provider)
	if err != nil {
		return fmt.Errorf("pack deposit: %w", err)
	}
	_, err = sendEscrowTx(ctx, rpcURL, privateKeyHex, escrowAddress, depositWei, calldata)
	return err
}

// ReleaseEscrow calls release(sessionId) on the DeploymentEscrow contract.
// The agent wallet must be the contract's releaseAuthority or owner.
func ReleaseEscrow(
	ctx context.Context,
	rpcURL, privateKeyHex, escrowAddress string,
	sessionID [32]byte,
) error {
	calldata, err := escrowABI.Pack("release", sessionID)
	if err != nil {
		return fmt.Errorf("pack release: %w", err)
	}
	_, err = sendEscrowTx(ctx, rpcURL, privateKeyHex, escrowAddress, big.NewInt(0), calldata)
	return err
}

// sendEscrowTx signs and sends a transaction to the escrow contract.
func sendEscrowTx(ctx context.Context, rpcURL, privateKeyHex, toAddress string, value *big.Int, calldata []byte) (string, error) {
	client := newRPCClient(rpcURL)

	privKeyHex := strings.TrimPrefix(privateKeyHex, "0x")
	privKey, err := gethcrypto.HexToECDSA(privKeyHex)
	if err != nil {
		return "", fmt.Errorf("parse private key: %w", err)
	}
	fromAddr := gethcrypto.PubkeyToAddress(privKey.PublicKey)
	chainID := big.NewInt(11155111) // Ethereum Sepolia

	nonceResult, err := client.call(ctx, "eth_getTransactionCount", fromAddr.Hex(), "latest")
	if err != nil {
		return "", fmt.Errorf("get nonce: %w", err)
	}
	var nonceHex string
	if err := json.Unmarshal(nonceResult, &nonceHex); err != nil {
		return "", fmt.Errorf("decode nonce: %w", err)
	}
	nonce, _ := new(big.Int).SetString(strings.TrimPrefix(nonceHex, "0x"), 16)

	gasPriceResult, err := client.call(ctx, "eth_gasPrice")
	if err != nil {
		return "", fmt.Errorf("get gas price: %w", err)
	}
	var gasPriceHex string
	if err := json.Unmarshal(gasPriceResult, &gasPriceHex); err != nil {
		return "", fmt.Errorf("decode gas price: %w", err)
	}
	gasPrice, _ := new(big.Int).SetString(strings.TrimPrefix(gasPriceHex, "0x"), 16)

	toAddr := gethcommon.HexToAddress(toAddress)
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce.Uint64(),
		To:       &toAddr,
		Value:    value,
		Gas:      150_000,
		GasPrice: gasPrice,
		Data:     calldata,
	})
	signer := types.NewEIP155Signer(chainID)
	signed, err := types.SignTx(tx, signer, privKey)
	if err != nil {
		return "", fmt.Errorf("sign tx: %w", err)
	}
	buf, err := signed.MarshalBinary()
	if err != nil {
		return "", fmt.Errorf("marshal tx: %w", err)
	}
	result, err := client.call(ctx, "eth_sendRawTransaction", "0x"+hex.EncodeToString(buf))
	if err != nil {
		return "", fmt.Errorf("eth_sendRawTransaction: %w", err)
	}
	var txHash string
	_ = json.Unmarshal(result, &txHash)
	return txHash, nil
}
