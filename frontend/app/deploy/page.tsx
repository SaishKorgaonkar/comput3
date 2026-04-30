"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Sidebar } from "@/components/Sidebar";
import { apiFetch, WS_API } from "@/lib/api";
import { useAuth } from "@/lib/AuthContext";
import { getWallet } from "@/lib/api";

const BG = "#111111";
const CARD = "#1a1a1a";
const BORDER = "rgba(255,255,255,0.06)";
const ACCENT = "#e2f0d9";
const ACCENT_FG = "#111111";

type Phase =
  | "repo"
  | "scanning"
  | "pick"
  | "prompt"
  | "creating"
  | "streaming"
  | "awaiting_confirm"
  | "building"
  | "done"
  | "error";

type DetectedOption = {
  framework: string;
  type: string;
  port: number;
  install_cmd: string;
  build_cmd?: string;
  start_cmd: string;
  sub_dir?: string;
};

type RepoScan = {
  repo_url: string;
  options: DetectedOption[];
};

type PlanContainer = {
  name?: string;
  image?: string;
  ports?: string[];
  reason?: string;
  ram_mb?: number;
  cpu_cores?: number;
};

type Plan = {
  summary: string;
  containers?: PlanContainer[];
  estimated_cost_per_hour?: number;
  estimated_cost_usd?: number;
};

type LiveEvent = {
  type: string;
  ts: number;
  data?: unknown;
  message?: string;
  action?: unknown;
  plan?: Plan;
  deployed_url?: string;
  container_id?: string;
  error?: string;
};

type WsStatus = "connecting" | "connected" | "reconnecting" | "disconnected";

export default function DeployPage() {
  const { isAuthenticated, hydrated, teamId } = useAuth();
  const router = useRouter();

  const [phase, setPhase] = useState<Phase>("repo");
  const [repoURL, setRepoURL] = useState("");
  const [scan, setScan] = useState<RepoScan | null>(null);
  const [selectedOption, setSelectedOption] = useState(0);
  const [deployPrompt, setDeployPrompt] = useState("");
  const [sessionId, setSessionId] = useState("");
  const [liveEvents, setLiveEvents] = useState<LiveEvent[]>([]);
  const [plan, setPlan] = useState<Plan | null>(null);
  const [appURL, setAppURL] = useState("");
  const [errMsg, setErrMsg] = useState("");
  const [confirming, setConfirming] = useState(false);
  const [wsStatus, setWsStatus] = useState<WsStatus>("disconnected");

  const wsRef = useRef<WebSocket | null>(null);
  const eventsEndRef = useRef<HTMLDivElement | null>(null);
  const intentionalCloseRef = useRef(false);
  const reconnectAttemptsRef = useRef(0);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (hydrated && !isAuthenticated) router.replace("/signin");
  }, [hydrated, isAuthenticated, router]);

  useEffect(() => {
    eventsEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [liveEvents]);

  useEffect(() => {
    return () => {
      intentionalCloseRef.current = true;
      wsRef.current?.close();
      if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
    };
  }, []);

  function scheduleReconnect(sid: string) {
    if (reconnectAttemptsRef.current >= 5) {
      setWsStatus("disconnected");
      return;
    }
    setWsStatus("reconnecting");
    const delay = Math.min(1000 * 2 ** reconnectAttemptsRef.current, 16000);
    reconnectTimerRef.current = setTimeout(() => {
      reconnectAttemptsRef.current += 1;
      connectWS(sid);
    }, delay);
  }

  function connectWS(sid: string) {
    intentionalCloseRef.current = false;
    setWsStatus("connecting");
    const wallet = getWallet();
    const url = `${WS_API}/sessions/${sid}/stream${wallet ? `?wallet=${encodeURIComponent(wallet)}` : ""}`;
    const ws = new WebSocket(url);
    wsRef.current = ws;

    ws.onopen = () => {
      setWsStatus("connected");
      reconnectAttemptsRef.current = 0;
    };

    ws.onmessage = (e) => {
      try {
        const evt = JSON.parse(e.data) as LiveEvent & { type: string };
        const ts = evt.ts ?? Date.now();
        setLiveEvents((prev) => [...prev, { ...evt, ts }]);

        if (evt.type === "plan") {
          const p = (evt.plan ?? evt.data) as Plan | undefined;
          if (p) { setPlan(p); setPhase("awaiting_confirm"); }
        } else if (evt.type === "done") {
          const url = evt.deployed_url ?? (evt.data as { url?: string })?.url ?? "";
          if (url) setAppURL(url);
          setPhase("done");
          intentionalCloseRef.current = true;
          ws.close();
          setWsStatus("disconnected");
        } else if (evt.type === "error") {
          const msg = evt.error ?? String(evt.data ?? "Unknown error");
          setErrMsg(msg);
          setPhase("error");
          intentionalCloseRef.current = true;
          ws.close();
          setWsStatus("disconnected");
        } else if (evt.type === "building") {
          setPhase("building");
          setPlan(null);
        }

        eventsEndRef.current?.scrollIntoView({ behavior: "smooth" });
      } catch {
        // ignore parse errors
      }
    };

    ws.onclose = () => {
      if (!intentionalCloseRef.current) scheduleReconnect(sid);
    };

    ws.onerror = () => ws.close();
  }

  async function handleScan() {
    if (!repoURL.trim()) return;
    setPhase("scanning");
    setErrMsg("");
    try {
      const res = await apiFetch("/repos/scan", {
        method: "POST",
        body: JSON.stringify({ repo_url: repoURL.trim() }),
      });
      const text = await res.text();
      if (!res.ok) throw new Error(text);
      const data: RepoScan = JSON.parse(text);
      if (!data.options?.length) {
        setDeployPrompt(`Deploy the repo at ${repoURL.trim()} on its default port.`);
        setPhase("prompt");
        return;
      }
      setScan(data);
      setSelectedOption(0);
      setPhase("pick");
    } catch (e) {
      setErrMsg(String(e));
      setPhase("error");
    }
  }

  function handlePickDone() {
    const opt = scan?.options[selectedOption];
    if (opt) {
      const subDirClause = opt.sub_dir
        ? `The source is in the \`${opt.sub_dir}\` subdirectory.`
        : "The source is at the repo root.";
      const buildClause = opt.build_cmd ? ` Build with: ${opt.build_cmd}.` : "";
      setDeployPrompt(
        `Deploy the ${opt.framework} ${opt.type} on port ${opt.port}. ${subDirClause} Run: ${opt.install_cmd}.${buildClause} Start with: ${opt.start_cmd}.`
      );
    } else {
      setDeployPrompt(`Deploy the repo at ${repoURL} and start the application.`);
    }
    setPhase("prompt");
  }

  async function handleDeploy() {
    if (!deployPrompt.trim() || !teamId) return;
    setPhase("creating");
    setErrMsg("");
    try {
      const res = await apiFetch("/sessions", {
        method: "POST",
        body: JSON.stringify({
          team_id: teamId,
          prompt: deployPrompt.trim(),
          repo_url: repoURL.trim() || undefined,
        }),
      });
      if (!res.ok) throw new Error(await res.text());
      const session = await res.json();
      const sid: string = session.id;
      setSessionId(sid);
      setPhase("streaming");
      connectWS(sid);
    } catch (e) {
      setErrMsg(String(e));
      setPhase("error");
    }
  }

  async function handleConfirm(approved: boolean) {
    setConfirming(true);
    try {
      const res = await apiFetch(`/sessions/${sessionId}/confirm`, {
        method: "POST",
        body: JSON.stringify({ approved }),
      });
      if (!res.ok) throw new Error(await res.text());
      if (!approved) {
        setErrMsg("Deployment cancelled.");
        setPhase("error");
        intentionalCloseRef.current = true;
        wsRef.current?.close();
      }
    } catch (e) {
      setErrMsg(String(e));
      setPhase("error");
    } finally {
      setConfirming(false);
    }
  }

  function resetDeploy() {
    intentionalCloseRef.current = true;
    wsRef.current?.close();
    setPhase("repo");
    setScan(null);
    setRepoURL("");
    setLiveEvents([]);
    setPlan(null);
    setAppURL("");
    setSessionId("");
    setErrMsg("");
    setDeployPrompt("");
  }

  const PHASE_ORDER: Phase[] = ["repo","scanning","pick","prompt","creating","streaming","awaiting_confirm","building","done","error"];

  function stageStatus(phases: Phase[]): "active" | "done" | "pending" {
    const currentIdx = PHASE_ORDER.indexOf(phase);
    const stageMaxIdx = Math.max(...phases.map((p) => PHASE_ORDER.indexOf(p)));
    const stageMinIdx = Math.min(...phases.map((p) => PHASE_ORDER.indexOf(p)));
    if (phases.includes(phase)) return "active";
    if (currentIdx > stageMaxIdx) return "done";
    if (currentIdx < stageMinIdx) return "pending";
    return "pending";
  }

  const stages = [
    { id: ["repo","scanning"] as Phase[], label: "Connect Repository", sub: repoURL ? repoURL.split("/").slice(-1)[0] : "Link source code" },
    { id: ["pick"] as Phase[], label: "Detect Stack", sub: scan ? `${scan.options.length} option(s) found` : "Auto-detect framework" },
    { id: ["prompt"] as Phase[], label: "Deployment Prompt", sub: deployPrompt ? deployPrompt.slice(0, 36) + "…" : "Describe what to deploy" },
    { id: ["creating","streaming","awaiting_confirm","building","done"] as Phase[], label: "AI Agent Deploy", sub: phase === "done" ? "Live ✓" : phase === "awaiting_confirm" ? "Awaiting confirmation" : ["creating","streaming","building"].includes(phase) ? "Running…" : "Encrypted container" },
  ];

  const isLivePhase = ["streaming", "awaiting_confirm", "building"].includes(phase);

  if (!hydrated || !isAuthenticated) {
    return (
      <div style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", background: BG }}>
        <span className="animate-spin" style={{ display: "inline-block", width: 32, height: 32, borderRadius: "50%", border: "2px solid rgba(255,255,255,0.1)", borderTopColor: "#fff" }} />
      </div>
    );
  }

  return (
    <div style={{ display: "flex", height: "100vh", background: BG, fontFamily: "Inter, var(--font-inter), sans-serif", color: "#e5e7eb" }}>
      <Sidebar mode="user" />
      <main style={{ flex: 1, display: "flex", flexDirection: "column", overflowY: "auto" }}>
        <div style={{ padding: 32 }}>

          {/* Header */}
          <header style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", flexWrap: "wrap", gap: 16, marginBottom: 24 }}>
            <div>
              <p style={{ fontSize: 28, fontWeight: 900, color: "#f9fafb", lineHeight: 1.2 }}>New Deployment</p>
              <p style={{ fontSize: 13, fontFamily: "monospace", color: "#6b7280", marginTop: 4 }}>
                AI agent · encrypted container · on-chain attestation
              </p>
            </div>
            <div style={{ display: "flex", gap: 10 }}>
              <Link href="/sessions" style={{ display: "flex", alignItems: "center", height: 40, padding: "0 16px", borderRadius: 8, background: "rgba(255,255,255,0.06)", color: "#e5e7eb", fontSize: 13, fontWeight: 700, textDecoration: "none" }}>Sessions</Link>
              <Link href="/" style={{ display: "flex", alignItems: "center", height: 40, padding: "0 16px", borderRadius: 8, background: "rgba(255,255,255,0.06)", color: "#e5e7eb", fontSize: 13, fontWeight: 700, textDecoration: "none" }}>← Dashboard</Link>
            </div>
          </header>

          {/* Stat cards */}
          <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 16, marginBottom: 32 }}>
            {[
              { label: "Status", value: phase === "creating" ? "Creating…" : isLivePhase ? "Agent running" : phase === "awaiting_confirm" ? "Awaiting confirm" : phase === "done" ? "Live ✓" : phase === "error" ? "Failed" : phase === "scanning" ? "Scanning…" : "Ready" },
              { label: "Repository", value: repoURL ? repoURL.split("/").slice(-1)[0] || repoURL : "—" },
              { label: "Framework", value: scan?.options[selectedOption]?.framework ?? "—" },
              { label: "Session ID", value: sessionId ? sessionId.slice(0, 16) + "…" : "—" },
            ].map((c) => (
              <div key={c.label} style={{ background: CARD, border: `1px solid ${BORDER}`, borderRadius: 12, padding: 16 }}>
                <p style={{ fontSize: 12, color: "#9ca3af", marginBottom: 6 }}>{c.label}</p>
                <p style={{ fontSize: 15, fontWeight: 700, color: "#f9fafb" }}>{c.value}</p>
              </div>
            ))}
          </div>

          {/* Main grid */}
          <div style={{ display: "grid", gridTemplateColumns: "1fr 2fr", gap: 32 }}>

            {/* Left: pipeline stages */}
            <div style={{ display: "flex", flexDirection: "column", gap: 0 }}>
              <h2 style={{ fontSize: 15, fontWeight: 700, color: "#f9fafb", marginBottom: 16 }}>Pipeline Stages</h2>
              {stages.map((s, i) => {
                const st = stageStatus(s.id);
                return (
                  <div key={i} style={{ display: "grid", gridTemplateColumns: "auto 1fr", gap: "0 16px" }}>
                    <div style={{ display: "flex", flexDirection: "column", alignItems: "center" }}>
                      <div style={{
                        width: 28, height: 28, borderRadius: "50%", display: "flex", alignItems: "center", justifyContent: "center",
                        background: st === "active" ? "rgba(226,240,217,0.15)" : st === "done" ? "rgba(40,167,69,0.15)" : "#1c1c1e",
                      }}>
                        {st === "done"
                          ? <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="#28a745" strokeWidth="3"><polyline points="20 6 9 17 4 12"/></svg>
                          : st === "active"
                          ? <div style={{ width: 8, height: 8, borderRadius: "50%", background: ACCENT }} />
                          : <div style={{ width: 6, height: 6, borderRadius: "50%", background: "#4b5563" }} />
                        }
                      </div>
                      {i < stages.length - 1 && <div style={{ width: 1, flex: 1, background: "#2c2c2e", minHeight: 24 }} />}
                    </div>
                    <div style={{
                      background: st === "active" ? "rgba(226,240,217,0.08)" : "transparent",
                      border: st === "active" ? "1px solid rgba(226,240,217,0.2)" : "1px solid transparent",
                      borderRadius: 8, padding: st === "active" ? "10px 12px" : "4px 0", marginBottom: st === "active" ? 4 : 0, paddingBottom: 20
                    }}>
                      <p style={{ fontSize: 13, fontWeight: 600, color: st === "active" ? ACCENT : st === "done" ? "#f3f4f6" : "#4b5563" }}>{s.label}</p>
                      <p style={{ fontSize: 11, color: st === "done" ? "#28a745" : "#6b7280", marginTop: 2 }}>{s.sub}</p>
                    </div>
                  </div>
                );
              })}
              {sessionId && (
                <div style={{ marginTop: 16, padding: 12, background: "#161618", borderRadius: 8, border: `1px solid ${BORDER}` }}>
                  <p style={{ fontSize: 11, color: "#6b7280", marginBottom: 4 }}>Active Session</p>
                  <Link href={`/sessions/${sessionId}`} style={{ fontSize: 12, fontFamily: "monospace", color: ACCENT, textDecoration: "none" }}>
                    {sessionId.slice(0, 28)}…
                  </Link>
                </div>
              )}
            </div>

            {/* Right: active panel */}
            <div style={{ background: CARD, border: `1px solid ${BORDER}`, borderRadius: 12, display: "flex", flexDirection: "column", overflow: "hidden" }}>

              {/* Stage 1: Repo URL */}
              {(phase === "repo" || phase === "scanning") && (
                <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 20 }}>
                  <div>
                    <p style={{ fontSize: 15, fontWeight: 700, color: "#f9fafb", marginBottom: 4 }}>Import Git Repository</p>
                    <p style={{ fontSize: 12, color: "#6b7280" }}>Paste a public GitHub URL to scan and deploy.</p>
                  </div>
                  <div style={{ display: "flex", gap: 8 }}>
                    <input
                      type="text"
                      placeholder="https://github.com/owner/repo"
                      value={repoURL}
                      onChange={(e) => setRepoURL(e.target.value)}
                      onKeyDown={(e) => e.key === "Enter" && handleScan()}
                      style={{ flex: 1, padding: "10px 12px", borderRadius: 8, border: `1px solid ${BORDER}`, background: BG, color: "#e5e7eb", fontSize: 13, outline: "none" }}
                    />
                    <button
                      onClick={handleScan}
                      disabled={!repoURL.trim() || phase === "scanning"}
                      style={{ padding: "10px 18px", borderRadius: 8, background: ACCENT, color: ACCENT_FG, fontSize: 13, fontWeight: 700, border: "none", cursor: !repoURL.trim() ? "default" : "pointer", opacity: !repoURL.trim() ? 0.4 : 1, whiteSpace: "nowrap" }}
                    >
                      {phase === "scanning" ? "Scanning…" : "Import"}
                    </button>
                  </div>
                  <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    <span style={{ fontSize: 12, color: "#4b5563" }}>No repo?</span>
                    <button
                      onClick={() => { setDeployPrompt(""); setPhase("prompt"); }}
                      style={{ fontSize: 12, fontWeight: 600, color: ACCENT, background: "transparent", border: "none", cursor: "pointer", textDecoration: "underline" }}
                    >
                      Create from prompt only →
                    </button>
                  </div>
                </div>
              )}

              {/* Stage 2: Pick option */}
              {phase === "pick" && scan && (
                <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 20 }}>
                  <div>
                    <p style={{ fontSize: 15, fontWeight: 700, color: "#f9fafb", marginBottom: 4 }}>Detected Stack</p>
                    <p style={{ fontSize: 12, color: "#6b7280" }}>Select what to deploy from <span style={{ fontFamily: "monospace" }}>{scan.repo_url.split("/").slice(-1)[0]}</span></p>
                  </div>
                  <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                    {scan.options.map((opt, i) => (
                      <button
                        key={i}
                        onClick={() => setSelectedOption(i)}
                        style={{
                          textAlign: "left", padding: 16, borderRadius: 10,
                          border: `1px solid ${selectedOption === i ? ACCENT : "#2c2c2e"}`,
                          background: selectedOption === i ? "rgba(226,240,217,0.08)" : "#0a0a0a",
                          cursor: "pointer",
                        }}
                      >
                        <p style={{ fontSize: 13, fontWeight: 700, color: selectedOption === i ? ACCENT : "#e5e7eb" }}>{opt.framework} — {opt.type}</p>
                        <p style={{ fontSize: 11, color: "#6b7280", marginTop: 4 }}>Port {opt.port} · {opt.install_cmd}</p>
                      </button>
                    ))}
                  </div>
                  <button
                    onClick={handlePickDone}
                    style={{ alignSelf: "flex-end", padding: "10px 24px", borderRadius: 8, background: ACCENT, color: ACCENT_FG, fontSize: 13, fontWeight: 700, border: "none", cursor: "pointer" }}
                  >
                    Configure Prompt →
                  </button>
                </div>
              )}

              {/* Stage 3: Prompt */}
              {phase === "prompt" && (
                <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 20 }}>
                  <div>
                    <p style={{ fontSize: 15, fontWeight: 700, color: "#f9fafb", marginBottom: 4 }}>Deployment Prompt</p>
                    <p style={{ fontSize: 12, color: "#6b7280" }}>Tell the AI agent what to deploy. It will generate a plan for your confirmation.</p>
                  </div>
                  {repoURL && (
                    <div style={{ padding: "10px 12px", borderRadius: 8, background: "#0a0a0a", border: `1px solid ${BORDER}`, display: "flex", alignItems: "center", gap: 8 }}>
                      <span style={{ fontSize: 12, fontFamily: "monospace", color: "#9ca3af" }}>{repoURL}</span>
                    </div>
                  )}
                  <textarea
                    value={deployPrompt}
                    onChange={(e) => setDeployPrompt(e.target.value)}
                    rows={5}
                    placeholder={repoURL ? `e.g. "Deploy on port 3000. Set NODE_ENV=production."` : `e.g. "Create a Node.js Express API with a /health endpoint on port 3000."`}
                    style={{ padding: 12, borderRadius: 8, border: `1px solid ${BORDER}`, background: BG, color: "#e5e7eb", fontSize: 13, resize: "vertical", outline: "none", fontFamily: "inherit" }}
                  />
                  <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
                    {["Deploy on port 3000", "Set NODE_ENV=production", "Run npm install && npm start", "Use Docker"].map((s) => (
                      <button
                        key={s}
                        onClick={() => setDeployPrompt((p) => p ? `${p} ${s}.` : `${s}.`)}
                        style={{ padding: "5px 12px", borderRadius: 999, fontSize: 11, fontWeight: 600, color: "#9ca3af", background: "#161618", border: `1px solid ${BORDER}`, cursor: "pointer" }}
                      >
                        + {s}
                      </button>
                    ))}
                  </div>
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", paddingTop: 8, borderTop: `1px solid ${BORDER}` }}>
                    <button
                      onClick={() => setPhase(scan ? "pick" : "repo")}
                      style={{ padding: "8px 16px", borderRadius: 8, background: "rgba(255,255,255,0.05)", color: "#9ca3af", fontSize: 13, fontWeight: 600, border: `1px solid ${BORDER}`, cursor: "pointer" }}
                    >
                      ← Back
                    </button>
                    <button
                      onClick={handleDeploy}
                      disabled={!deployPrompt.trim()}
                      style={{ padding: "10px 24px", borderRadius: 8, background: ACCENT, color: ACCENT_FG, fontSize: 13, fontWeight: 700, border: "none", cursor: !deployPrompt.trim() ? "default" : "pointer", opacity: !deployPrompt.trim() ? 0.4 : 1 }}
                    >
                      Launch Agent 🚀
                    </button>
                  </div>
                </div>
              )}

              {/* Creating */}
              {phase === "creating" && (
                <div style={{ padding: 40, display: "flex", flexDirection: "column", alignItems: "center", gap: 16 }}>
                  <span className="animate-spin" style={{ display: "inline-block", width: 32, height: 32, borderRadius: "50%", border: `2px solid rgba(226,240,217,0.2)`, borderTopColor: ACCENT }} />
                  <p style={{ fontSize: 14, color: "#9ca3af" }}>Creating deployment session…</p>
                </div>
              )}

              {/* Live streaming / awaiting confirm / building */}
              {isLivePhase && (
                <div style={{ display: "flex", flexDirection: "column", height: "100%", minHeight: 400 }}>
                  <div style={{ padding: "16px 20px", borderBottom: `1px solid ${BORDER}`, display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                    <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                      <div
                        className="animate-pulse"
                        style={{ width: 8, height: 8, borderRadius: "50%", background: wsStatus === "reconnecting" ? "#f97316" : phase === "awaiting_confirm" ? "#eab308" : ACCENT }}
                      />
                      <span style={{ fontSize: 14, fontWeight: 700, color: "#f9fafb" }}>
                        {phase === "awaiting_confirm" ? "Plan Ready — Confirm to Proceed" : phase === "building" ? "Building Deployment…" : "Agent Running"}
                      </span>
                    </div>
                    <Link href={`/sessions/${sessionId}`} style={{ fontSize: 11, color: "#6b7280", textDecoration: "none" }}>View full session →</Link>
                  </div>

                  {/* Plan confirmation */}
                  {phase === "awaiting_confirm" && plan && (
                    <div style={{ padding: 20, borderBottom: `1px solid ${BORDER}`, background: "#0e1117" }}>
                      <p style={{ fontSize: 13, fontWeight: 700, color: "#eab308", marginBottom: 10 }}>📋 Deployment Plan</p>
                      <p style={{ fontSize: 13, color: "#d1d5db", marginBottom: 12, lineHeight: 1.5 }}>{plan.summary}</p>
                      {plan.containers && plan.containers.length > 0 && (
                        <div style={{ display: "flex", flexDirection: "column", gap: 6, marginBottom: 12 }}>
                          {plan.containers.map((c, i) => (
                            <div key={i} style={{ display: "flex", gap: 8, padding: "8px 12px", borderRadius: 6, background: "#161618", border: `1px solid ${BORDER}` }}>
                              {c.name && <span style={{ fontSize: 12, fontFamily: "monospace", color: ACCENT, minWidth: 80 }}>{c.name}</span>}
                              {c.image && <span style={{ fontSize: 12, fontFamily: "monospace", color: "#6b7280" }}>{c.image}</span>}
                              {c.ram_mb != null && <span style={{ marginLeft: "auto", fontSize: 11, color: "#4b5563" }}>{c.ram_mb}MB · {c.cpu_cores} cpu</span>}
                            </div>
                          ))}
                        </div>
                      )}
                      <div style={{ display: "flex", gap: 10 }}>
                        <button
                          onClick={() => handleConfirm(true)}
                          disabled={confirming}
                          style={{ flex: 1, padding: 10, borderRadius: 8, background: ACCENT, color: ACCENT_FG, fontSize: 13, fontWeight: 700, border: "none", cursor: confirming ? "default" : "pointer", opacity: confirming ? 0.6 : 1 }}
                        >
                          {confirming ? "Confirming…" : "✓ Approve & Deploy"}
                        </button>
                        <button
                          onClick={() => handleConfirm(false)}
                          disabled={confirming}
                          style={{ padding: "10px 20px", borderRadius: 8, background: "rgba(220,53,69,0.1)", color: "#dc3545", fontSize: 13, fontWeight: 700, border: "1px solid rgba(220,53,69,0.3)", cursor: confirming ? "default" : "pointer" }}
                        >
                          Cancel
                        </button>
                      </div>
                    </div>
                  )}

                  {/* Live event log */}
                  <div style={{ flex: 1, overflowY: "auto", padding: 16, fontFamily: "monospace", fontSize: 12, display: "flex", flexDirection: "column", gap: 4, background: "#0a0a0a" }}>
                    {liveEvents.length === 0 && <p style={{ color: "#4b5563" }}>Waiting for agent events…</p>}
                    {liveEvents.map((evt, i) => {
                      const color = evt.type === "plan" ? "#eab308" : evt.type === "done" ? "#22c55e" : evt.type === "error" ? "#ef4444" : evt.type === "action" ? "#60a5fa" : "#9ca3af";
                      const prefix = evt.type === "plan" ? "📋 " : evt.type === "done" ? "✓ " : evt.type === "error" ? "✗ " : evt.type === "action" ? "⚡ " : "· ";
                      const text =
                        evt.type === "message" ? (typeof evt.message === "string" ? evt.message : JSON.stringify(evt.message ?? evt.data)) :
                        evt.type === "plan" ? ((evt.plan as Plan)?.summary ?? "Plan received") :
                        evt.type === "action" ? JSON.stringify(evt.action).slice(0, 100) :
                        evt.type === "done" ? "Deployment complete" :
                        evt.type === "error" ? (typeof evt.error === "string" ? evt.error : JSON.stringify(evt.error)) :
                        JSON.stringify(evt.data ?? "");
                      if (!text) return null;
                      return (
                        <div key={i} style={{ color, lineHeight: 1.5 }}>
                          <span style={{ color: "#374151" }}>{new Date(evt.ts).toLocaleTimeString()} </span>
                          <span style={{ fontWeight: 600 }}>{prefix}</span>
                          <span style={{ color: "#d1d5db" }}>{String(text)}</span>
                        </div>
                      );
                    })}
                    <div ref={eventsEndRef} />
                  </div>
                </div>
              )}

              {/* Done */}
              {phase === "done" && (
                <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16 }}>
                  <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="#28a745" strokeWidth="2.5"><polyline points="20 6 9 17 4 12"/></svg>
                    <p style={{ fontSize: 15, fontWeight: 700, color: "#28a745" }}>Deployment Successful</p>
                  </div>
                  <div style={{ background: BG, border: `1px solid ${BORDER}`, borderRadius: 10, padding: 16, display: "flex", flexDirection: "column", gap: 10 }}>
                    <div style={{ display: "flex", justifyContent: "space-between" }}>
                      <span style={{ fontSize: 12, color: "#6b7280" }}>Session ID</span>
                      <span style={{ fontSize: 12, fontFamily: "monospace", color: "#e5e7eb" }}>{sessionId}</span>
                    </div>
                    {appURL && (
                      <div style={{ display: "flex", justifyContent: "space-between" }}>
                        <span style={{ fontSize: 12, color: "#6b7280" }}>App URL</span>
                        <a href={appURL} target="_blank" rel="noreferrer" style={{ fontSize: 12, fontFamily: "monospace", color: ACCENT }}>{appURL}</a>
                      </div>
                    )}
                  </div>
                  <div style={{ display: "flex", gap: 10 }}>
                    {appURL && <a href={appURL} target="_blank" rel="noreferrer" style={{ flex: 1, textAlign: "center", padding: "10px 16px", borderRadius: 8, background: ACCENT, color: ACCENT_FG, fontSize: 13, fontWeight: 700, textDecoration: "none" }}>Open App ↗</a>}
                    <Link href={`/sessions/${sessionId}`} style={{ flex: 1, textAlign: "center", padding: "10px 16px", borderRadius: 8, background: "rgba(255,255,255,0.07)", color: "#e5e7eb", fontSize: 13, fontWeight: 700, textDecoration: "none" }}>View Session Log</Link>
                    <button onClick={resetDeploy} style={{ padding: "10px 16px", borderRadius: 8, background: "rgba(255,255,255,0.04)", color: "#9ca3af", fontSize: 13, fontWeight: 600, border: `1px solid ${BORDER}`, cursor: "pointer" }}>Deploy Again</button>
                  </div>
                </div>
              )}

              {/* Error */}
              {phase === "error" && (
                <div style={{ padding: 24, display: "flex", flexDirection: "column", gap: 12 }}>
                  <p style={{ fontSize: 14, fontWeight: 700, color: "#dc3545" }}>Something went wrong</p>
                  <pre style={{ fontSize: 12, fontFamily: "monospace", color: "#9ca3af", whiteSpace: "pre-wrap", background: BG, border: `1px solid ${BORDER}`, borderRadius: 8, padding: 12 }}>{errMsg}</pre>
                  <div style={{ display: "flex", gap: 8 }}>
                    <button onClick={resetDeploy} style={{ padding: "8px 16px", borderRadius: 8, background: "rgba(255,255,255,0.06)", color: "#e5e7eb", fontSize: 13, fontWeight: 600, border: "none", cursor: "pointer" }}>← Try again</button>
                    {sessionId && <Link href={`/sessions/${sessionId}`} style={{ padding: "8px 16px", borderRadius: 8, background: "rgba(255,255,255,0.04)", color: "#9ca3af", fontSize: 13, fontWeight: 600, textDecoration: "none", border: `1px solid ${BORDER}` }}>View Session</Link>}
                  </div>
                </div>
              )}

            </div>
          </div>
        </div>
      </main>
    </div>
  );
}
