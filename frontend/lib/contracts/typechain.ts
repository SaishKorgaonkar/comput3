// Minimal ABI + deployments for frontend contract interactions.
// Addresses are filled from .env after running scripts/deploy-contracts.sh

export const ProviderRegistryABI = {
  abi: [
    {
      inputs: [
        { internalType: "string",  name: "endpoint",     type: "string"  },
        { internalType: "uint256", name: "pricePerHour", type: "uint256" },
      ],
      name: "register",
      outputs: [],
      stateMutability: "payable",
      type: "function",
    },
    {
      inputs: [{ internalType: "address", name: "provider", type: "address" }],
      name: "isActive",
      outputs: [{ internalType: "bool", name: "", type: "bool" }],
      stateMutability: "view",
      type: "function",
    },
    {
      inputs: [{ internalType: "address", name: "", type: "address" }],
      name: "providers",
      outputs: [
        { internalType: "string",  name: "endpoint",      type: "string"  },
        { internalType: "uint256", name: "pricePerHour",  type: "uint256" },
        { internalType: "uint256", name: "stakedAmount",  type: "uint256" },
        { internalType: "uint256", name: "jobsCompleted", type: "uint256" },
        { internalType: "bool",    name: "active",        type: "bool"    },
      ],
      stateMutability: "view",
      type: "function",
    },
  ],
} as const;

// DeploymentEscrow: simple escrow deposit so user locks funds before agent runs.
export const DeploymentEscrowABI = {
  abi: [
    {
      inputs: [
        { internalType: "bytes32", name: "sessionId", type: "bytes32" },
        { internalType: "address", name: "provider",  type: "address" },
      ],
      name: "deposit",
      outputs: [],
      stateMutability: "payable",
      type: "function",
    },
    {
      inputs: [{ internalType: "bytes32", name: "sessionId", type: "bytes32" }],
      name: "refund",
      outputs: [],
      stateMutability: "nonpayable",
      type: "function",
    },
    {
      inputs: [{ internalType: "bytes32", name: "", type: "bytes32" }],
      name: "escrows",
      outputs: [
        { internalType: "address",       name: "user",        type: "address"  },
        { internalType: "address",       name: "provider",    type: "address"  },
        { internalType: "uint256",       name: "amount",      type: "uint256"  },
        { internalType: "uint256",       name: "depositedAt", type: "uint256"  },
        { internalType: "bytes32",       name: "sessionId",   type: "bytes32"  },
        { internalType: "uint8",         name: "status",      type: "uint8"    },
      ],
      stateMutability: "view",
      type: "function",
    },
  ],
} as const;

export const deployments = {
  ethSepolia: {
    ProviderRegistry: (process.env.NEXT_PUBLIC_PROVIDER_REGISTRY_ADDRESS ?? "") as `0x${string}`,
    DeploymentEscrow: (process.env.NEXT_PUBLIC_DEPLOYMENT_ESCROW_ADDRESS ?? "") as `0x${string}`,
    JobAuction:       (process.env.NEXT_PUBLIC_JOB_AUCTION_ADDRESS ?? "") as `0x${string}`,
  },
};
