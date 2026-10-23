/**
 * The way out of the box, as pi sees it.
 *
 * pi reaches a model through a provider it knows. The box holds no credential,
 * so this registers one pointing at the proxy the sandbox gave it: the address
 * in GUSTO_MODEL_URL, and the per-job token in GUSTO_MODEL_TOKEN where a key
 * would go. The proxy trades that token for the real credential, and counts
 * what goes.
 *
 * The model list is asked for rather than written down, because which models a
 * credential serves is the deployment's to say and changes without pi.
 */
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

type Listed = { id: string; name?: string; context_window?: number; max_tokens?: number };

export default async function (pi: ExtensionAPI) {
	const baseUrl = process.env.GUSTO_MODEL_URL;
	const token = process.env.GUSTO_MODEL_TOKEN;
	if (!baseUrl) {
		return;
	}

	let models: Listed[] = [];
	try {
		const answer = await fetch(`${baseUrl.replace(/\/$/, "")}/models`, {
			headers: token ? { Authorization: `Bearer ${token}` } : {},
		});
		if (answer.ok) {
			models = ((await answer.json()) as { data?: Listed[] }).data ?? [];
		}
	} catch {
		// A proxy that cannot be asked leaves the list empty, and the model the
		// job named is registered on its own below.
	}

	// The job named one model, and it must be registered whether or not the
	// listing came back, or pi has nothing to be pointed at.
	const named = process.env.GUSTO_MODEL_ID;
	if (named && !models.some((model) => model.id === named)) {
		models.push({ id: named });
	}

	pi.registerProvider("gusto", {
		name: "gusto",
		baseUrl,
		apiKey: "$GUSTO_MODEL_TOKEN",
		api: "openai-completions",
		models: models.map((model) => ({
			id: model.id,
			name: model.name ?? model.id,
			reasoning: false,
			input: ["text"],
			// The proxy prices a call, not pi: a price written here would be a
			// second answer to what a run cost.
			cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
			contextWindow: model.context_window ?? 128000,
			maxTokens: model.max_tokens ?? 8192,
		})),
	});
}
