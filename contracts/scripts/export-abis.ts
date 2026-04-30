/**
 * export-abis.ts
 *
 * Copies compiled ABI JSON files and deployments.json into
 * ../frontend/lib/contracts/ for use in the Next.js frontend.
 *
 * Run after every compile:
 *   npm run export:abis
 */

import * as fs from "fs";
import * as path from "path";

const CONTRACTS    = ["ProviderRegistry", "DeploymentEscrow", "JobAuction"];
const ARTIFACTS_DIR = path.join(__dirname, "../artifacts/contracts");
const OUT_DIR       = path.join(__dirname, "../../frontend/lib/contracts");
const DEPLOYMENTS   = path.join(__dirname, "../deployments.json");

function ensureDir(dir: string) {
  if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
}

async function main() {
  ensureDir(OUT_DIR);

  for (const name of CONTRACTS) {
    const artifactPath = path.join(ARTIFACTS_DIR, `${name}.sol`, `${name}.json`);
    if (!fs.existsSync(artifactPath)) {
      console.warn(`Artifact not found for ${name} — run: npm run compile`);
      continue;
    }
    const artifact = JSON.parse(fs.readFileSync(artifactPath, "utf-8"));
    const out = { abi: artifact.abi, contractName: artifact.contractName };
    const dest = path.join(OUT_DIR, `${name}.json`);
    fs.writeFileSync(dest, JSON.stringify(out, null, 2));
    console.log(`✅  ABI exported: ${dest}`);
  }

  if (fs.existsSync(DEPLOYMENTS)) {
    const dest = path.join(OUT_DIR, "deployments.json");
    fs.copyFileSync(DEPLOYMENTS, dest);
    console.log(`✅  Deployments copied: ${dest}`);
  } else {
    console.warn("deployments.json not found — deploy first, then re-run.");
  }

  console.log("\nDone. Import ABIs in the frontend:");
  for (const name of CONTRACTS) {
    console.log(`  import ${name}ABI from '@/lib/contracts/${name}.json'`);
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
