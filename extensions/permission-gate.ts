/**
 * Factory permission gate.
 *
 * Blocks destructive shell patterns and writes outside the agent's cwd.
 * Designed for headless (RPC) operation: when there's no UI to ask, we
 * BLOCK, never allow. Loosen this only with explicit operator consent
 * upstream in the Go orchestrator.
 *
 * Patterns based on examples/extensions/{permission-gate,protected-paths}.ts
 * from pi-mono, but hard-coded to deny in non-interactive mode.
 */

import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import * as path from "node:path";

export default function (pi: ExtensionAPI) {
	const dangerousBash: RegExp[] = [
		/\brm\s+(-[a-z]*r[a-z]*f?|-[a-z]*f[a-z]*r|--recursive)/i,
		/\bsudo\b/i,
		/\b(chmod|chown)\b.*\b(777|666)\b/i,
		/\bdd\s+if=.*\bof=\/dev\//i,
		/\bmkfs\b/i,
		/:\(\)\s*\{\s*:\|:&\s*\};:/, // fork bomb
		// destructive git
		/\bgit\s+push\s+(--force|-f)\b.*\b(main|master|production|release)\b/i,
		/\bgit\s+(reset\s+--hard|checkout\s+--|clean\s+-fd|branch\s+-D)\b/i,
		/\bgit\s+commit\s+--no-verify\b/i,
		// credential exfil sniff
		/\b(curl|wget)\b[^|]*\|\s*sh\b/i,
		/\.ssh\/id_(rsa|ed25519|ecdsa)\b/i,
	];

	const protectedPathFragments = [
		".env",
		".env.local",
		".env.production",
		"/secrets/",
		".aws/credentials",
		".ssh/",
		".npmrc",
		".pypirc",
		".netrc",
	];

	const protectedPathSuffixes = [".pem", ".key", ".p12", ".pfx"];

	function isProtectedPath(p: string): boolean {
		if (protectedPathFragments.some((frag) => p.includes(frag))) return true;
		if (protectedPathSuffixes.some((suf) => p.endsWith(suf))) return true;
		return false;
	}

	function isOutsideCwd(p: string, cwd: string): boolean {
		const abs = path.resolve(cwd, p);
		const root = path.resolve(cwd);
		const rel = path.relative(root, abs);
		return rel.startsWith("..") || path.isAbsolute(rel);
	}

	pi.on("tool_call", async (event, _ctx) => {
		if (event.toolName === "bash") {
			const command = String(event.input.command ?? "");
			const offender = dangerousBash.find((p) => p.test(command));
			if (offender) {
				return {
					block: true,
					reason: `permission-gate: bash command matches denylist ${offender}: ${command}`,
				};
			}
			return undefined;
		}

		if (event.toolName === "write" || event.toolName === "edit") {
			const targetPath = String(event.input.path ?? "");
			if (isProtectedPath(targetPath)) {
				return {
					block: true,
					reason: `permission-gate: ${event.toolName} of protected path ${targetPath}`,
				};
			}
			if (isOutsideCwd(targetPath, process.cwd())) {
				return {
					block: true,
					reason: `permission-gate: ${event.toolName} outside repo root: ${targetPath}`,
				};
			}
			return undefined;
		}

		return undefined;
	});
}
