// useEscrow.ts — deposit ETH into DeploymentEscrow before a session starts.
import { useWriteContract, useWaitForTransactionReceipt } from "wagmi";
import { parseEther, keccak256, toBytes } from "viem";
import { DeploymentEscrowABI, deployments } from "./contracts/typechain";

// Convert a session ID string to bytes32 via keccak256.
function sessionIdToBytes32(sessionId: string): `0x${string}` {
  return keccak256(toBytes(sessionId));
}

export function useEscrow() {
  const { writeContractAsync, isPending } = useWriteContract();

  async function deposit(sessionId: string, providerAddress: `0x${string}`, ethAmount: string): Promise<`0x${string}`> {
    const contractAddress = deployments.ethSepolia.DeploymentEscrow;
    if (!contractAddress) throw new Error("DeploymentEscrow address not configured");

    const hash = await writeContractAsync({
      address: contractAddress,
      abi: DeploymentEscrowABI.abi,
      functionName: "deposit",
      args: [sessionIdToBytes32(sessionId), providerAddress],
      value: parseEther(ethAmount),
    });
    return hash;
  }

  return { deposit, isPending };
}
