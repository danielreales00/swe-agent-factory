/**
 * run_bin_quality — invoke the target repo's blocking quality pipeline.
 *
 * Most target manifests declare a `quality_cmd` (e.g. personal_finance's
 * `bin/quality`). The agent loop wants a single, named tool to call when
 * it thinks it's done editing — that's this. Returns exit code and a tail
 * of combined stdout+stderr.
 */

import { spawn } from "node:child_process";
import { Type } from "@earendil-works/pi-ai";
import { defineTool, type ExtensionAPI } from "@earendil-works/pi-coding-agent";

const DEFAULT_CMD = "bin/quality";
const DEFAULT_TIMEOUT_MS = 5 * 60 * 1000; // 5 min
const TAIL_LINES = 80;

const runBinQuality = defineTool({
	name: "run_bin_quality",
	label: "bin/quality",
	description:
		"Run the target repo's blocking quality pipeline (fmt + lint + tests). " +
		"Call this after edits to confirm the change still passes project gates.",
	promptSnippet: "Run the project's blocking quality pipeline; surfaces exit code + log tail.",
	promptGuidelines: [
		"Use run_bin_quality after edits to confirm the change passes the project's gates before declaring done.",
		"If run_bin_quality returns nonzero, read its tail output, fix the failure, and re-run.",
	],
	parameters: Type.Object({
		cmd: Type.Optional(
			Type.String({
				description: "Quality command relative to cwd. Defaults to 'bin/quality'.",
			}),
		),
		timeoutMs: Type.Optional(
			Type.Number({
				description: "Hard timeout in ms. Default 300000 (5 min).",
			}),
		),
	}),

	async execute(_toolCallId, params, signal, onUpdate, _ctx) {
		const cmd = params.cmd ?? DEFAULT_CMD;
		const timeoutMs = params.timeoutMs ?? DEFAULT_TIMEOUT_MS;

		return await new Promise((resolve) => {
			const child = spawn("/bin/sh", ["-c", cmd], {
				cwd: process.cwd(),
				env: process.env,
				stdio: ["ignore", "pipe", "pipe"],
			});

			let combined = "";
			const append = (chunk: Buffer | string) => {
				combined += typeof chunk === "string" ? chunk : chunk.toString("utf8");
				onUpdate?.({
					content: [{ type: "text", text: lastLines(combined, TAIL_LINES) }],
				});
			};
			child.stdout?.on("data", append);
			child.stderr?.on("data", append);

			const timer = setTimeout(() => {
				child.kill("SIGTERM");
				setTimeout(() => child.kill("SIGKILL"), 2000);
			}, timeoutMs);

			const onAbort = () => {
				clearTimeout(timer);
				child.kill("SIGTERM");
			};
			signal?.addEventListener("abort", onAbort, { once: true });

			child.on("close", (code, sigName) => {
				clearTimeout(timer);
				signal?.removeEventListener("abort", onAbort);
				const exitCode = code ?? -1;
				const tail = lastLines(combined, TAIL_LINES);
				const header = `cmd=${cmd}  exit=${exitCode}${sigName ? `  signal=${sigName}` : ""}`;
				resolve({
					content: [{ type: "text", text: `${header}\n---\n${tail}` }],
					details: { exitCode, signal: sigName ?? null, fullOutputBytes: combined.length },
					isError: exitCode !== 0,
				});
			});
		});
	},
});

function lastLines(s: string, n: number): string {
	const lines = s.split("\n");
	return lines.slice(Math.max(0, lines.length - n)).join("\n");
}

export default function (pi: ExtensionAPI) {
	pi.registerTool(runBinQuality);
}
