"use client";

import { useState } from "react";
import { Sidebar } from "@/components/Sidebar";
import { WalletButton } from "@/components/WalletButton";
import { useAccount, useWriteContract, useWaitForTransactionReceipt } from "wagmi";
import { sepolia } from "viem/chains";
import { ProviderRegistryABI, deployments } from "@/lib/contracts/typechain";

const ACCENT = "#e2f0d9";
const REGISTRY_ADDRESS = deployments.ethSepolia.ProviderRegistry;

type SettingsForm = {
  endpoint: string;
  pricePerHour: string;
};

export default function ProviderSettingsPage() {
  const { isConnected } = useAccount();
  const [form, setForm] = useState<SettingsForm>({ endpoint: "", pricePerHour: "0.08" });
  const [error, setError] = useState("");

  const { writeContract, data: txHash, isPending } = useWriteContract();
  const { isSuccess, isLoading: isConfirming } = useWaitForTransactionReceipt({ hash: txHash, chainId: sepolia.id });

  function handleChange(key: keyof SettingsForm, value: string) {
    setForm((f) => ({ ...f, [key]: value }));
  }

  async function handleSubmit() {
    if (!isConnected) return;
    setError("");
    try {
      const pricePerHourMicro = BigInt(Math.round(parseFloat(form.pricePerHour) * 1e6));
      writeContract({
        address: REGISTRY_ADDRESS,
        abi: ProviderRegistryABI.abi,
        functionName: "update",
        args: [form.endpoint, pricePerHourMicro],
        chainId: sepolia.id,
      });
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Transaction failed");
    }
  }

  return (
    <div style={{ display: "flex", height: "100vh", background: "#111111", fontFamily: "Inter, var(--font-inter), sans-serif", color: "#e5e7eb" }}>
      <Sidebar mode="provider" />
      <main style={{ flex: 1, overflowY: "auto" }}>
        <div style={{ padding: 32, maxWidth: 560 }}>
          <header style={{ marginBottom: 24 }}>
            <p style={{ fontSize: 28, fontWeight: 900, letterSpacing: -0.5, color: "#f9fafb" }}>Provider Settings</p>
            <p style={{ fontSize: 13, fontFamily: "monospace", marginTop: 4, color: "#6b7280" }}>
              Update your endpoint URL and pricing on-chain
            </p>
          </header>

          {!isConnected ? (
            <div style={{ borderRadius: 12, padding: 24, display: "flex", flexDirection: "column", alignItems: "center", gap: 16, background: "#1a1a1a", border: `1px solid ${ACCENT}` }}>
              <p style={{ fontSize: 13, color: "#9ca3af" }}>Connect your wallet to update provider settings</p>
              <WalletButton />
            </div>
          ) : isSuccess ? (
            <div style={{ borderRadius: 12, padding: 24, display: "flex", flexDirection: "column", gap: 12, background: "rgba(34,197,94,0.06)", border: "1px solid rgba(34,197,94,0.25)" }}>
              <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#22c55e" strokeWidth="2.5"><polyline points="20 6 9 17 4 12"/></svg>
                <p style={{ fontSize: 13, fontWeight: 700, color: "#22c55e" }}>Provider settings updated on-chain!</p>
              </div>
              <p style={{ fontSize: 11, fontFamily: "monospace", wordBreak: "break-all", color: "#6b7280" }}>Tx: {txHash}</p>
              <a href={`https://sepolia.etherscan.io/tx/${txHash}`} target="_blank" rel="noreferrer" style={{ fontSize: 12, color: ACCENT }}>View on Etherscan →</a>
            </div>
          ) : (
            <div style={{ display: "flex", flexDirection: "column", gap: 24 }}>
              {error && (
                <div style={{ borderRadius: 8, padding: 12, fontSize: 12, fontFamily: "monospace", background: "rgba(239,68,68,0.08)", border: "1px solid rgba(239,68,68,0.25)", color: "#ef4444" }}>{error}</div>
              )}

              {/* Endpoint */}
              <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                <label style={{ fontSize: 13, fontWeight: 600, color: "#9ca3af" }}>New Endpoint URL</label>
                <input
                  type="text"
                  placeholder="https://my-provider.example.com"
                  value={form.endpoint}
                  onChange={(e) => handleChange("endpoint", e.target.value)}
                  style={{ fontSize: 13, padding: "10px 12px", borderRadius: 8, outline: "none", background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.08)", color: "#e5e7eb" }}
                  onFocus={(e) => (e.currentTarget.style.borderColor = ACCENT)}
                  onBlur={(e) => (e.currentTarget.style.borderColor = "rgba(255,255,255,0.08)")}
                />
              </div>

              {/* Price */}
              <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                <label style={{ fontSize: 13, fontWeight: 600, color: "#9ca3af" }}>New Price / hour (USDC)</label>
                <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                  <input
                    type="number" step="0.01" min="0.01"
                    value={form.pricePerHour}
                    onChange={(e) => handleChange("pricePerHour", e.target.value)}
                    style={{ width: 160, fontSize: 13, padding: "10px 12px", borderRadius: 8, outline: "none", background: "#1a1a1a", border: "1px solid rgba(255,255,255,0.08)", color: "#e5e7eb" }}
                    onFocus={(e) => (e.currentTarget.style.borderColor = ACCENT)}
                    onBlur={(e) => (e.currentTarget.style.borderColor = "rgba(255,255,255,0.08)")}
                  />
                  <span style={{ fontSize: 13, fontFamily: "monospace", color: "#6b7280" }}>USDC/hr</span>
                </div>
              </div>

              <p style={{ fontSize: 12, color: "#6b7280" }}>Updating settings calls <code style={{ fontFamily: "monospace", color: ACCENT }}>update()</code> on the ProviderRegistry contract. No additional stake required.</p>

              <button
                onClick={handleSubmit}
                disabled={!form.endpoint.trim() || isPending || isConfirming}
                style={{ height: 44, borderRadius: 8, fontSize: 13, fontWeight: 900, border: "none", cursor: !form.endpoint.trim() || isPending || isConfirming ? "default" : "pointer", background: ACCENT, color: "#111111", opacity: !form.endpoint.trim() || isPending || isConfirming ? 0.3 : 1, display: "flex", alignItems: "center", justifyContent: "center", gap: 8 }}
              >
                {(isPending || isConfirming) && (
                  <span className="animate-spin" style={{ display: "inline-block", width: 14, height: 14, borderRadius: "50%", border: "2px solid currentColor", borderTopColor: "transparent" }} />
                )}
                {isPending ? "Confirm in wallet…" : isConfirming ? "Confirming…" : "Update Provider Settings"}
              </button>
            </div>
          )}
        </div>
      </main>
    </div>
  );
}
