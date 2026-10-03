export interface ModelDefinition {
  id: string;
  name: string;
  tier: "Fast" | "Medium" | "Smart" | "Thinking";
  badge?: string;
  isCreditRequired?: boolean;
  isWarning?: boolean;
  shortcut?: string;
  agent: "agy" | "claude" | "codex" | string;
  provider?: string;
}

export interface ProviderGroup {
  id: string;
  name: string;
  icon: string;
  models: ModelDefinition[];
}

export const PROVIDER_GROUPS: ProviderGroup[] = [
  {
    id: "anthropic",
    name: "Anthropic Claude",
    icon: "claude",
    models: [
      {
        id: "claude-sonnet-5-5",
        name: "Claude Sonnet 5.5",
        tier: "Smart",
        badge: "Coding SOTA",
        agent: "claude",
        provider: "anthropic",
        shortcut: "1",
      },
      {
        id: "claude-opus-5-5",
        name: "Claude Opus 5.5",
        tier: "Thinking",
        badge: "Agentic Flagship",
        agent: "claude",
        provider: "anthropic",
        shortcut: "2",
      },
      {
        id: "claude-fable-5-1",
        name: "Claude Fable 5.1",
        tier: "Thinking",
        badge: "Long-Horizon",
        agent: "claude",
        provider: "anthropic",
      },
      {
        id: "claude-haiku-4-5",
        name: "Claude Haiku 4.5",
        tier: "Fast",
        badge: "Fast & Efficient",
        agent: "claude",
        provider: "anthropic",
        shortcut: "3",
      },
      {
        id: "claude-3-7-sonnet",
        name: "Claude 3.7 Sonnet",
        tier: "Thinking",
        badge: "Extended Thinking",
        agent: "claude",
        provider: "anthropic",
      },
    ],
  },
  {
    id: "openai",
    name: "OpenAI",
    icon: "openai",
    models: [
      {
        id: "gpt-6-astra",
        name: "GPT-6 Astra",
        tier: "Smart",
        badge: "Agentic Flagship",
        agent: "codex",
        provider: "openai",
      },
      {
        id: "gpt-6.1-sol",
        name: "GPT-6.1 Sol",
        tier: "Medium",
        badge: "Balanced SOTA",
        agent: "codex",
        provider: "openai",
      },
      {
        id: "gpt-6-luna",
        name: "GPT-6 Luna",
        tier: "Fast",
        badge: "High-Volume",
        agent: "codex",
        provider: "openai",
      },
      {
        id: "gpt-5.5",
        name: "GPT-5.5",
        tier: "Smart",
        badge: "Workhorse",
        agent: "codex",
        provider: "openai",
      },
      {
        id: "o3-pro",
        name: "o3-pro",
        tier: "Thinking",
        badge: "Deep Reasoning",
        agent: "codex",
        provider: "openai",
      },
      {
        id: "o4-mini",
        name: "o4-mini",
        tier: "Smart",
        badge: "Fast Reasoning",
        agent: "codex",
        provider: "openai",
      },
      {
        id: "gpt-oss-120b",
        name: "GPT-OSS 120B",
        tier: "Medium",
        badge: "Open Weights",
        agent: "codex",
        provider: "openai",
      },
    ],
  },
  {
    id: "gemini",
    name: "Google Gemini",
    icon: "gemini",
    models: [
      {
        id: "gemini-3.8-flash",
        name: "Gemini 3.8 Flash",
        tier: "Medium",
        badge: "Agentic Workhorse",
        agent: "agy",
        provider: "google",
        shortcut: "1",
      },
      {
        id: "gemini-3.5-flash",
        name: "Gemini 3.5 Flash",
        tier: "Fast",
        badge: "Next-Gen Fast",
        agent: "agy",
        provider: "google",
        shortcut: "2",
      },
      {
        id: "gemini-3.5-flash-lite",
        name: "Gemini 3.5 Flash-Lite",
        tier: "Fast",
        badge: "High-Speed",
        agent: "agy",
        provider: "google",
        shortcut: "3",
      },
      {
        id: "gemini-3.1-pro",
        name: "Gemini 3.1 Pro",
        tier: "Smart",
        badge: "Multimodal Reasoning",
        agent: "agy",
        provider: "google",
        shortcut: "4",
      },
    ],
  },
  {
    id: "deepseek",
    name: "DeepSeek",
    icon: "deepseek",
    models: [
      {
        id: "deepseek-v4-pro",
        name: "DeepSeek V4 Pro",
        tier: "Thinking",
        badge: "1M Context SOTA",
        agent: "custom",
        provider: "deepseek",
      },
      {
        id: "deepseek-v4-flash",
        name: "DeepSeek V4 Flash",
        tier: "Fast",
        badge: "1M Fast",
        agent: "custom",
        provider: "deepseek",
      },
      {
        id: "deepseek-reasoner",
        name: "DeepSeek R1",
        tier: "Thinking",
        badge: "Reasoning SOTA",
        agent: "custom",
        provider: "deepseek",
      },
      {
        id: "deepseek-chat",
        name: "DeepSeek V3",
        tier: "Fast",
        badge: "MoE 671B",
        agent: "custom",
        provider: "deepseek",
      },
    ],
  },
  {
    id: "xai",
    name: "xAI (Grok)",
    icon: "sparkles",
    models: [
      {
        id: "grok-3",
        name: "Grok 3",
        tier: "Thinking",
        badge: "Reasoning Flagship",
        agent: "custom",
        provider: "xai",
      },
      {
        id: "grok-3-mini",
        name: "Grok 3 Mini",
        tier: "Smart",
        badge: "Fast Reasoning",
        agent: "custom",
        provider: "xai",
      },
      {
        id: "grok-2-1212",
        name: "Grok 2",
        tier: "Smart",
        badge: "Vision & Code",
        agent: "custom",
        provider: "xai",
      },
    ],
  },
  {
    id: "ollama",
    name: "Ollama / Local",
    icon: "cpu",
    models: [
      {
        id: "gpt-oss-120b",
        name: "GPT-OSS 120B",
        tier: "Smart",
        badge: "Open Weights",
        agent: "custom",
        provider: "ollama",
      },
      {
        id: "qwen2.5-coder:32b",
        name: "Qwen 2.5 Coder 32B",
        tier: "Smart",
        badge: "SOTA Code",
        agent: "custom",
        provider: "ollama",
      },
      {
        id: "deepseek-r1:32b",
        name: "DeepSeek R1 32B",
        tier: "Thinking",
        badge: "Local R1",
        agent: "custom",
        provider: "ollama",
      },
      {
        id: "llama3.3:70b",
        name: "Llama 3.3 70B",
        tier: "Smart",
        badge: "Flagship 70B",
        agent: "custom",
        provider: "ollama",
      },
      {
        id: "codestral:22b",
        name: "Codestral 22B",
        tier: "Smart",
        badge: "Mistral Code",
        agent: "custom",
        provider: "ollama",
      },
    ],
  },
];

export const ALL_MODELS: ModelDefinition[] = PROVIDER_GROUPS.flatMap((g) => g.models);

export function getModelById(id: string): ModelDefinition {
  return ALL_MODELS.find((m) => m.id === id) || ALL_MODELS[0];
}
