import React, { useState, useRef, useEffect, useMemo } from "react";
import {
  ArrowUp,
  Square,
  Plus,
  Image as ImageIcon,
  AtSign,
  SquareSlash,
  Globe,
  X,
  Loader2,
  Mic,
  Check,
  ChevronDown,
  ChevronRight,
  Sparkles,
  Terminal,
  Code2,
  Zap,
  AlertTriangle,
  Cpu,
  Settings,
} from "lucide-react";
import { ModelSelector, type SelectedModelConfig } from "./ModelSelector";
import type { ModelDefinition } from "../lib/models";
import type { MessageItem } from "./ChatStream";
import { AddAgentModal, type CustomAgentConfig } from "./AddAgentModal";

export interface SlashCommand {
  name: string;
  description?: string;
  arg_hint?: string;
  kind?: string;
}

export interface AttachedMedia {
  uri: string;
  mime_type: string;
  url: string;
  filename?: string;
}

interface ChatInputProps {
  onSendMessage: (text: string, media?: AttachedMedia[]) => void;
  onCancelTask: () => void;
  currentConfig: SelectedModelConfig;
  onSelectConfig: (config: SelectedModelConfig) => void;
  isRunning: boolean;
  initialText?: string;
  onTextConsumed?: () => void;
  onOpenBrowser?: () => void;
  onOpenChanges?: () => void;
  onSelectAgent?: (agentId: string) => void;
  toolAliases?: Record<string, string>;
  messages?: MessageItem[];
  permissionMode?: string;
  onChangePermissionMode?: (mode: string) => void;
  allowedModes?: string[];
  onShellCommand?: (command: string) => void;
  liveModels?: Record<string, ModelDefinition[]>;
  modelsFetchedAt?: string;
  onRefreshModels?: () => Promise<void> | void;
}

const AGENT_TOOLS = [
  {
    id: "agy" as const,
    name: "Antigravity",
    desc: "Gemini CLI Engine",
    icon: Sparkles,
    color: "text-sky-400",
  },
  {
    id: "claude" as const,
    name: "Claude Code",
    desc: "Anthropic Claude CLI",
    icon: Terminal,
    color: "text-purple-400",
  },
  {
    id: "codex" as const,
    name: "OpenAI Codex",
    desc: "Codex CLI Subprocess",
    icon: Code2,
    color: "text-emerald-400",
  },
];

const AgentToolSelector: React.FC<{
  activeAgent: string;
  onSelectAgent: (agentId: "agy" | "claude" | "codex" | string) => void;
  aliases?: Record<string, string>;
}> = ({ activeAgent, onSelectAgent, aliases = {} }) => {
  const [isOpen, setIsOpen] = useState(false);
  const [isAddModalOpen, setIsAddModalOpen] = useState(false);
  const [availableAgents, setAvailableAgents] = useState<Record<string, boolean>>({
    agy: true,
    claude: false,
    codex: false,
  });
  const [customAgents, setCustomAgents] = useState<CustomAgentConfig[]>(() => {
    try {
      const saved = localStorage.getItem("clara_custom_agents");
      return saved ? JSON.parse(saved) : [];
    } catch {
      return [];
    }
  });
  const dropdownRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    fetch("/api/agents")
      .then((res) => res.json())
      .then((data: any[]) => {
        if (Array.isArray(data)) {
          const map: Record<string, boolean> = {};
          data.forEach((a) => {
            map[a.id] = !!a.available;
          });
          setAvailableAgents(map);
        }
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setIsOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const allAgentTools = useMemo(() => {
    // Collect all custom agents first
    const customMap = new Map(customAgents.map((ca) => [ca.id, ca]));

    // Start with built-ins, but if customAgents has an entry with same ID, override description/color/etc.
    const builtIns = AGENT_TOOLS.map((bt) => {
      const customOverride = customMap.get(bt.id);
      if (customOverride) {
        return {
          id: bt.id,
          name: customOverride.name || bt.name,
          desc: customOverride.desc || bt.desc,
          icon: bt.icon,
          color: customOverride.color || bt.color,
          isCustom: true,
        };
      }
      return {
        ...bt,
        isCustom: false,
      };
    });

    // Add extra purely-custom agents (not matching built-in IDs)
    const extraCustom = customAgents
      .filter((ca) => !AGENT_TOOLS.some((bt) => bt.id === ca.id))
      .map((ca) => ({
        id: ca.id,
        name: ca.name,
        desc: ca.desc,
        icon: Cpu,
        color: ca.color || "text-sky-400",
        isCustom: true,
      }));

    return [...builtIns, ...extraCustom];
  }, [customAgents]);

  const currentTool =
    allAgentTools.find((t) => t.id === activeAgent) || allAgentTools[0];
  const Icon = currentTool.icon;
  const displayName = aliases[currentTool.id] || currentTool.name;

  const handleAddNewAgent = (newAgent: CustomAgentConfig) => {
    const updated = [...customAgents.filter((a) => a.id !== newAgent.id), newAgent];
    setCustomAgents(updated);
    try {
      localStorage.setItem("clara_custom_agents", JSON.stringify(updated));
    } catch (e) {
      console.error(e);
    }
    setAvailableAgents((prev) => ({ ...prev, [newAgent.id]: newAgent.isAvailable ?? true }));
    onSelectAgent(newAgent.id);
  };

  return (
    <>
      <div className="relative inline-flex items-center" ref={dropdownRef}>
        <button
          type="button"
          onClick={() => setIsOpen(!isOpen)}
          className="flex items-center gap-1.5 px-2 py-0.5 rounded-lg bg-[#181d28] hover:bg-[#202736] border border-[#273042] text-xs text-slate-200 transition-colors cursor-pointer select-none"
          title="Switch AI CLI Tool"
        >
          <Icon className={`w-3.5 h-3.5 ${currentTool.color}`} />
          <span className="font-medium text-[12px]">{displayName}</span>
          <ChevronDown className="w-3 h-3 text-slate-500" />
        </button>

        {isOpen && (
          <div className="absolute bottom-full right-0 mb-2 w-56 bg-[#161a22] border border-[#272e3d] rounded-xl shadow-2xl py-1 z-50 animate-in fade-in zoom-in-95 duration-100">
            {/* Header with Title and Settings button at top right */}
            <div className="flex items-center justify-between px-3 py-1.5 border-b border-[#222836] mb-1 select-none">
              <span className="text-[10.5px] font-semibold text-slate-500 uppercase tracking-wider">
                AI Engine
              </span>
              <button
                type="button"
                onClick={() => {
                  setIsOpen(false);
                  setIsAddModalOpen(true);
                }}
                className="p-1 rounded-md text-slate-500 hover:text-slate-300 hover:bg-[#1f2533] transition-colors cursor-pointer"
                title="AI Engine Settings & Accounts"
              >
                <Settings className="w-3.5 h-3.5" />
              </button>
            </div>

            {/* List of Agents */}
            <div className="max-h-60 overflow-y-auto">
              {allAgentTools.map((tool) => {
                const isSelected = tool.id === activeAgent;
                const isAvailable = availableAgents[tool.id] !== false;
                const ToolIcon = tool.icon;
                const name = aliases[tool.id] || tool.name;

                return (
                  <div
                    key={tool.id}
                    onClick={() => {
                      if (!isAvailable) return;
                      onSelectAgent(tool.id);
                      setIsOpen(false);
                    }}
                    className={`group flex items-center justify-between px-3 py-2 text-xs transition-colors select-none ${
                      !isAvailable
                        ? "opacity-40 cursor-not-allowed bg-transparent"
                        : isSelected
                        ? "bg-[#212734] text-slate-100 font-medium cursor-pointer"
                        : "hover:bg-[#1a202c] text-slate-300 cursor-pointer"
                    }`}
                    title={!isAvailable ? `${name} is not installed or available` : undefined}
                  >
                    <div className="flex items-center gap-2.5 min-w-0 pr-2">
                      <ToolIcon className={`w-3.5 h-3.5 shrink-0 ${isAvailable ? tool.color : "text-slate-500"}`} />
                      <div className="min-w-0 truncate">
                        <div className="font-medium truncate">{name}</div>
                        <div className="text-[10px] text-slate-500 truncate">{tool.desc}</div>
                      </div>
                    </div>

                    <div className="flex items-center gap-1.5 shrink-0">
                      {!isAvailable && (
                        <span className="text-[9.5px] text-slate-500 bg-[#1e2330] px-1.5 py-0.5 rounded border border-[#2b3346]/80 font-medium">
                          Inactive
                        </span>
                      )}
                      {isSelected && isAvailable && <Check className="w-3.5 h-3.5 text-[#38bdf8]" />}
                    </div>
                  </div>
                );
              })}
            </div>

            {/* Add AI Engine Action Row */}
            <div
              onClick={() => {
                setIsOpen(false);
                setIsAddModalOpen(true);
              }}
              className="flex items-center gap-2 px-3 py-2 mt-1 border-t border-[#222836] text-xs text-sky-400 hover:text-sky-300 hover:bg-[#1a202c] cursor-pointer transition-colors select-none"
            >
              <Plus className="w-3.5 h-3.5" />
              <span className="font-medium text-[11.5px]">Add AI engine...</span>
            </div>
          </div>
        )}
      </div>

      <AddAgentModal
        isOpen={isAddModalOpen}
        onClose={() => setIsAddModalOpen(false)}
        onAddAgent={handleAddNewAgent}
      />
    </>
  );
};

// Permission modes enforced by the hub (not cosmetic): what the agent may do without asking.
export const PERMISSION_MODES = [
  { id: "ask", label: "Ask", desc: "Ask before running commands or editing files" },
  { id: "plan", label: "Plan", desc: "Read-only: explore and propose, no changes" },
  { id: "accept-edits", label: "Auto-edit", desc: "Edit files freely; ask before commands" },
  { id: "bypass", label: "Full access", desc: "Run everything without asking (this chat)" },
] as const;

function formatTokens(num: number): string {
  if (num >= 1000000) {
    const val = num / 1000000;
    return val % 1 === 0 ? `${val}M` : `${val.toFixed(1)}M`;
  }
  if (num >= 1000) {
    const val = num / 1000;
    return val >= 100
      ? `${Math.round(val * 10) / 10}k`
      : val % 1 === 0
      ? `${val}k`
      : `${val.toFixed(1)}k`;
  }
  return num.toString();
}

export const ChatInput: React.FC<ChatInputProps> = ({
  onSendMessage,
  onCancelTask,
  currentConfig,
  onSelectConfig,
  isRunning,
  initialText = "",
  onTextConsumed,
  onOpenBrowser,
  onOpenChanges,
  onSelectAgent,
  toolAliases = {},
  messages = [],
  permissionMode = "ask",
  onChangePermissionMode,
  allowedModes,
  onShellCommand,
  liveModels,
  modelsFetchedAt,
  onRefreshModels,
}) => {
  const [text, setText] = useState("");
  const [attachments, setAttachments] = useState<AttachedMedia[]>([]);
  const [isUploading, setIsUploading] = useState(false);
  const [isMenuOpen, setIsMenuOpen] = useState(false);
  const [isModeOpen, setIsModeOpen] = useState(false);
  const [isUsageOpen, setIsUsageOpen] = useState(false);
  const [isListening, setIsListening] = useState(false);

  // "/" menu: the active engine's slash commands and skills (fetched once per engine)
  const [commandsByAgent, setCommandsByAgent] = useState<Record<string, SlashCommand[]>>({});
  const [slashIdx, setSlashIdx] = useState(0);
  const [slashDismissed, setSlashDismissed] = useState(false);
  const slashAgent = currentConfig.model.agent;
  const slashMatch = /^\/(\S*)$/.exec(text);
  const slashActive = !!slashMatch;
  useEffect(() => {
    if (!slashActive || commandsByAgent[slashAgent]) return;
    fetch(`/api/v2/engines/${slashAgent}/commands`)
      .then((r) => (r.ok ? r.json() : []))
      .then((l) => setCommandsByAgent((p) => ({ ...p, [slashAgent]: Array.isArray(l) ? l : [] })))
      .catch(() => setCommandsByAgent((p) => ({ ...p, [slashAgent]: [] })));
  }, [slashActive, slashAgent, commandsByAgent]);
  const slashQuery = slashMatch ? slashMatch[1].toLowerCase() : "";
  const slashItems = useMemo(() => {
    if (!slashActive) return [] as SlashCommand[];
    const list = commandsByAgent[slashAgent] ?? [];
    const starts = list.filter((c) => c.name.toLowerCase().startsWith(slashQuery));
    const rest = list.filter(
      (c) => !c.name.toLowerCase().startsWith(slashQuery) && (c.name.toLowerCase().includes(slashQuery) || (c.description ?? "").toLowerCase().includes(slashQuery))
    );
    return [...starts, ...rest].slice(0, 40);
  }, [slashActive, slashQuery, commandsByAgent, slashAgent]);
  const slashLoading = slashActive && !commandsByAgent[slashAgent];
  const isShellMode = text.startsWith("!");
  const showSlash = slashActive && !slashDismissed && (slashLoading || slashItems.length > 0);
  useEffect(() => setSlashIdx(0), [slashQuery, slashAgent]);
  useEffect(() => {
    document.querySelector(`[data-slash-idx="${slashIdx}"]`)?.scrollIntoView({ block: "nearest" });
  }, [slashIdx, showSlash]);
  const pickSlash = (c: SlashCommand) => {
    setText(`/${c.name} `);
    setSlashDismissed(false);
    requestAnimationFrame(() => textareaRef.current?.focus());
  };

  // Git diff & changes banner
  const [gitStatus, setGitStatus] = useState<{
    repo_name?: string;
    branch?: string;
    files?: string[];
    additions?: number;
    deletions?: number;
    has_diff?: boolean;
  } | null>(null);
  const [showGitChip, setShowGitChip] = useState(true);

  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const modeRef = useRef<HTMLDivElement>(null);
  const usageRef = useRef<HTMLDivElement>(null);
  const isComposingRef = useRef(false);
  const lastSubmitTimeRef = useRef(0);
  const recognitionRef = useRef<any>(null);

  // Poll git status periodically
  useEffect(() => {
    let isMounted = true;
    const fetchGit = async () => {
      try {
        const res = await fetch("/api/git/diff");
        if (res.ok && isMounted) {
          const data = await res.json();
          setGitStatus(data);
        }
      } catch {
        // quiet error
      }
    };
    fetchGit();
    const interval = setInterval(fetchGit, 6000);
    return () => {
      isMounted = false;
      clearInterval(interval);
    };
  }, []);

  // Close menus when clicking outside
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setIsMenuOpen(false);
      }
      if (modeRef.current && !modeRef.current.contains(e.target as Node)) {
        setIsModeOpen(false);
      }
      if (usageRef.current && !usageRef.current.contains(e.target as Node)) {
        setIsUsageOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  // Handle Edit from queue
  useEffect(() => {
    if (initialText) {
      setText(initialText);
      onTextConsumed?.();
      textareaRef.current?.focus();
    }
  }, [initialText, onTextConsumed]);

  // Auto-resize textarea
  useEffect(() => {
    if (textareaRef.current) {
      textareaRef.current.style.height = "auto";
      textareaRef.current.style.height = `${Math.min(textareaRef.current.scrollHeight, 220)}px`;
    }
  }, [text]);

  // Context & token limits calculation for the CURRENT active window.
  // In long agent conversations, only recent turns (typically last 20 messages)
  // reside in the active model prompt context window; older turns are compacted.
  const { contextTokens, maxTokens, contextPercent } = useMemo(() => {
    let inTok = 0;
    let outTok = 0;
    if (messages && messages.length > 0) {
      // Focus on active window (last 20 messages) for context window gauge
      const activeSlice = messages.slice(-20);
      activeSlice.forEach((m) => {
        const charTok = Math.round((m.content || "").length / 3.8);
        let stepTok = 0;
        if (m.steps) {
          // Cap long step tool output representations to prevent blowing up the active window estimate
          m.steps.forEach((s) => {
            const outLen = Math.min((s.output || "").length, 4000);
            stepTok += Math.round(((s.command || "").length + outLen) / 4);
          });
        }
        const totalMsgTok = m.token_count && m.token_count > 0 ? m.token_count : charTok + stepTok;
        if (m.role === "user") {
          inTok += totalMsgTok;
        } else {
          outTok += totalMsgTok;
        }
      });
    }
    const total = Math.max(inTok + outTok, 2400);

    const modelId = (currentConfig.model.id || "").toLowerCase();
    let max = 1000000;
    if (modelId.includes("gemini")) {
      max = 1000000;
    } else if (modelId.includes("codex") || modelId.includes("gpt-4o")) {
      max = 128000;
    } else if (modelId.includes("opus") || modelId.includes("sonnet") || modelId.includes("claude")) {
      max = 200000;
    }

    const percent = Math.min(100, Math.max(0.8, (total / max) * 100));
    return {
      contextTokens: total,
      maxTokens: max,
      contextPercent: percent,
    };
  }, [messages, currentConfig.model.id]);

  // Context expansion state
  const [isContextExpanded, setIsContextExpanded] = useState<boolean>(() => {
    try {
      const saved = localStorage.getItem("clara_context_expanded");
      return saved === "true";
    } catch {
      return false;
    }
  });

  const handleCompact = () => {
    onSendMessage("/compact");
    setIsUsageOpen(false);
  };

  const contextDetails = useMemo(() => {
    const isGemini = (currentConfig.model.id || "").toLowerCase().includes("gemini");
    const systemTools = isGemini ? 16000 : 8000;
    const mcpTools = isGemini ? 14200 : 9600;
    const skills = 10000;
    const memoryFiles = 8400;
    const systemPrompt = 4400;
    const autocompactBuffer = 33000;

    // Use actual contextTokens — no artificial floor
    const messagesCount = contextTokens;
    const overhead = systemTools + mcpTools + skills + memoryFiles + systemPrompt;
    const totalUsed = messagesCount + overhead;
    const freeSpace = Math.max(0, maxTokens - totalUsed - autocompactBuffer);
    const overallPercent = Math.min(100, Math.max(0.5, (totalUsed / maxTokens) * 100));

    return {
      messages: messagesCount,
      overhead,
      systemTools,
      mcpTools,
      skills,
      memoryFiles,
      systemPrompt,
      autocompactBuffer,
      freeSpace,
      totalUsed,
      overallPercent,
    };
  }, [contextTokens, maxTokens, currentConfig.model.id]);

  // Context is an estimate based on persisted conversation text, not provider telemetry.
  const isContextNearLimit = contextPercent >= 85 || contextDetails.overallPercent >= 85;
  const isAnyLimitExceeded = contextPercent >= 95;
  const isAnyLimitWarning = isContextNearLimit;

  // Upload file helper
  const uploadFile = async (file: File) => {
    setIsUploading(true);
    try {
      const formData = new FormData();
      formData.append("file", file);

      const res = await fetch("/api/upload", {
        method: "POST",
        body: formData,
      });

      if (!res.ok) {
        throw new Error("Upload failed: " + res.statusText);
      }

      const data = await res.json();
      setAttachments((prev) => [
        ...prev,
        {
          uri: data.uri,
          mime_type: data.mime_type,
          url: data.url,
          filename: data.filename || file.name,
        },
      ]);
    } catch (err) {
      console.error("Upload error:", err);
    } finally {
      setIsUploading(false);
    }
  };

  const handlePaste = (e: React.ClipboardEvent) => {
    if (e.clipboardData.files && e.clipboardData.files.length > 0) {
      e.preventDefault();
      Array.from(e.clipboardData.files).forEach((file) => {
        uploadFile(file);
      });
    }
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files) {
      Array.from(e.target.files).forEach((file) => {
        uploadFile(file);
      });
      e.target.value = "";
    }
  };

  const handleRemoveAttachment = (uri: string) => {
    setAttachments((prev) => prev.filter((a) => a.uri !== uri));
  };

  const handleSubmit = (e?: React.FormEvent) => {
    e?.preventDefault();
    const now = Date.now();
    if (now - lastSubmitTimeRef.current < 400) return;

    // "!cmd" runs a shell command directly on the server (no model), like the Claude CLI
    const trimmedText = text.trim();
    if (onShellCommand && trimmedText.startsWith("!") && trimmedText.length > 1) {
      lastSubmitTimeRef.current = now;
      onShellCommand(trimmedText.slice(1).trim());
      setText("");
      if (textareaRef.current) textareaRef.current.style.height = "auto";
      return;
    }

    if ((text.trim() || attachments.length > 0) && !isUploading) {
      lastSubmitTimeRef.current = now;
      onSendMessage(text.trim(), attachments);
      setText("");
      setAttachments([]);
      if (textareaRef.current) {
        textareaRef.current.style.height = "auto";
      }
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (isComposingRef.current || e.nativeEvent.isComposing) return;

    if (showSlash && slashItems.length > 0) {
      if (e.key === "ArrowDown") { e.preventDefault(); setSlashIdx((i) => (i + 1) % slashItems.length); return; }
      if (e.key === "ArrowUp") { e.preventDefault(); setSlashIdx((i) => (i - 1 + slashItems.length) % slashItems.length); return; }
      if (e.key === "Enter" || e.key === "Tab") { e.preventDefault(); pickSlash(slashItems[Math.min(slashIdx, slashItems.length - 1)]); return; }
    }
    if (showSlash && e.key === "Escape") { e.preventDefault(); setSlashDismissed(true); return; }

    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    }
  };

  // Voice speech-to-text dictation
  const toggleListening = () => {
    const SpeechRecognition =
      (window as any).SpeechRecognition || (window as any).webkitSpeechRecognition;
    if (!SpeechRecognition) {
      alert("Speech recognition is not supported in this browser.");
      return;
    }

    if (isListening) {
      recognitionRef.current?.stop();
      setIsListening(false);
      return;
    }

    try {
      const recognition = new SpeechRecognition();
      recognition.continuous = true;
      recognition.interimResults = true;

      recognition.onresult = (event: any) => {
        let transcript = "";
        for (let i = event.resultIndex; i < event.results.length; ++i) {
          if (event.results[i].isFinal) {
            transcript += event.results[i][0].transcript;
          }
        }
        if (transcript) {
          setText((prev) => (prev ? prev + " " + transcript : transcript));
        }
      };

      recognition.onerror = () => setIsListening(false);
      recognition.onend = () => setIsListening(false);

      recognition.start();
      recognitionRef.current = recognition;
      setIsListening(true);
    } catch {
      setIsListening(false);
    }
  };

  const canSubmit = text.trim().length > 0 || attachments.length > 0;

  return (
    <div className="w-full max-w-4xl mx-auto p-4 z-20">
      <input
        type="file"
        ref={fileInputRef}
        onChange={handleFileChange}
        className="hidden"
        multiple
      />

      {/* 1. TOP PILL: Git changes / Branch badge (Style Claude) */}
      {showGitChip && gitStatus && gitStatus.has_diff && (
        <div className="flex items-center justify-between px-3.5 py-2 mb-2 rounded-2xl bg-[#141720] border border-[#252c3c] text-xs shadow-sm transition-all animate-in fade-in slide-in-from-bottom-1 duration-150">
          <div
            onClick={onOpenChanges}
            className="flex items-center gap-2 font-mono text-slate-300 hover:text-white cursor-pointer select-none truncate flex-1 min-w-0 mr-3"
            title="Click to view workspace diff"
          >
            <span className="font-semibold text-slate-200">
              {gitStatus.repo_name || "agent-hub"}
            </span>
            <span className="text-slate-400 truncate text-[11.5px]">
              {gitStatus.branch || "main"}
            </span>
          </div>

          <div className="flex items-center gap-3 shrink-0">
            <div
              onClick={onOpenChanges}
              className="flex items-center gap-1.5 font-mono text-[11.5px] select-none cursor-pointer hover:bg-[#202534] px-1.5 py-0.5 rounded transition-colors group/diff"
              title="Click to open Git changes panel"
            >
              <span className="text-emerald-400 font-semibold group-hover/diff:underline">
                +{gitStatus.additions ? gitStatus.additions.toLocaleString() : (gitStatus.files?.length || 1) * 8}
              </span>
              <span className="text-rose-400 font-semibold group-hover/diff:underline">
                -{gitStatus.deletions ? gitStatus.deletions.toLocaleString() : 0}
              </span>
            </div>

            <button
              type="button"
              onClick={onOpenChanges}
              className="px-2.5 py-1 rounded-lg bg-[#212634] hover:bg-[#2b3244] text-slate-200 hover:text-white font-medium text-[11.5px] transition-colors cursor-pointer border border-[#30384a]"
            >
              Commit changes
            </button>

            <button
              type="button"
              onClick={() => setShowGitChip(false)}
              className="p-1 rounded-full text-slate-500 hover:text-slate-300 hover:bg-[#202534] transition-colors cursor-pointer"
              title="Dismiss"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
      )}

      {/* 2. MIDDLE INPUT BOX: Khung nhập liệu style Claude */}
      <div className="relative rounded-2xl bg-[#141720] border border-[#252c3c] focus-within:border-slate-500 shadow-sm transition-all px-4 py-3">
        {/* Attachment preview chips */}
        {attachments.length > 0 && (
          <div className="flex flex-wrap gap-2 pb-2.5 mb-1 border-b border-[#202636]">
            {attachments.map((att) => {
              const isImage = att.mime_type.startsWith("image/");
              return (
                <div
                  key={att.uri}
                  className="flex items-center gap-2 pl-1.5 pr-2 py-1 rounded-xl bg-[#202531] border border-[#2e3547] text-slate-200 text-xs shadow-sm group"
                >
                  {isImage ? (
                    <img
                      src={att.url}
                      alt={att.filename || "attachment"}
                      className="w-7 h-7 object-cover rounded-lg border border-slate-700/60 shrink-0"
                    />
                  ) : (
                    <div className="w-7 h-7 rounded-lg bg-sky-950/60 border border-sky-800/40 flex items-center justify-center shrink-0 text-sky-400">
                      <ImageIcon className="w-3.5 h-3.5" />
                    </div>
                  )}
                  <span className="max-w-[120px] truncate font-medium text-[12px]">
                    {att.filename || att.uri.split("/").pop()}
                  </span>
                  <button
                    type="button"
                    onClick={() => handleRemoveAttachment(att.uri)}
                    className="p-0.5 rounded-full hover:bg-slate-700/60 text-slate-400 hover:text-slate-100 transition-colors cursor-pointer"
                  >
                    <X className="w-3.5 h-3.5" />
                  </button>
                </div>
              );
            })}
            {isUploading && (
              <div className="flex items-center gap-2 px-3 py-1.5 rounded-xl bg-[#202531]/70 border border-[#2e3547] text-slate-400 text-xs animate-pulse">
                <Loader2 className="w-3.5 h-3.5 animate-spin text-sky-400" />
                <span>Uploading...</span>
              </div>
            )}
          </div>
        )}

        {/* Textarea + Right Action icon */}
        <div className="flex items-center gap-3">
          <div className="relative flex-1 min-w-0">
          {showSlash && (
            <div className="absolute bottom-full left-0 mb-3 w-[min(680px,92vw)] max-h-72 overflow-y-auto rounded-xl bg-[#161920] border border-[#282e3c] shadow-2xl py-1 z-50 select-none">
              <div className="px-3 py-1 text-[10.5px] font-semibold text-slate-500 uppercase tracking-wider">
                {slashAgent} commands &amp; skills
              </div>
              {slashLoading && <div className="px-3 py-2 text-xs text-slate-500">Loading…</div>}
              {slashItems.map((c, i) => (
                <div
                  key={c.name}
                  data-slash-idx={i}
                  onMouseDown={(e) => { e.preventDefault(); pickSlash(c); }}
                  onMouseEnter={() => setSlashIdx(i)}
                  className={`flex items-baseline gap-2 px-3 py-1.5 text-[12.5px] cursor-pointer ${i === slashIdx ? "bg-[#212734] text-white" : "text-slate-300 hover:bg-[#1a202c]"}`}
                >
                  <span className="font-mono text-emerald-300 shrink-0">/{c.name}</span>
                  {c.arg_hint && <span className="font-mono text-[11px] text-slate-500 shrink-0">{c.arg_hint}</span>}
                  <span className="truncate text-[11.5px] text-slate-500">{c.description}</span>
                </div>
              ))}
            </div>
          )}
          {isShellMode && (
            <div className="mb-1 flex items-center gap-1.5 text-[10.5px] text-emerald-400/90 select-none">
              <span className="px-1 rounded bg-emerald-950/70 border border-emerald-800/50 font-mono font-bold">$</span>
              <span>Shell command · runs on the server, no model · output is passed to the agent · Enter to run</span>
            </div>
          )}
          <textarea
            ref={textareaRef}
            value={text}
            onChange={(e) => { setText(e.target.value); setSlashDismissed(false); }}
            onKeyDown={handleKeyDown}
            onPaste={handlePaste}
            onCompositionStart={() => {
              isComposingRef.current = true;
            }}
            onCompositionEnd={() => {
              isComposingRef.current = false;
            }}
            placeholder="Type / for commands, ! for a shell command"
            rows={1}
            className={`w-full bg-transparent text-[14px] placeholder-[#717b90] focus:outline-none resize-none leading-relaxed min-h-[24px] max-h-[220px] ${isShellMode ? "font-mono text-amber-200 caret-emerald-400" : "text-slate-100"}`}
          />
          </div>

          {/* Right action button */}
          <div className="shrink-0 flex items-center">
            {isRunning ? (
              <button
                type="button"
                onClick={onCancelTask}
                className="w-7 h-7 rounded-full bg-rose-950/70 border border-rose-700/60 hover:bg-rose-900 text-rose-300 flex items-center justify-center transition-all cursor-pointer shadow-sm"
                title="Stop running task"
              >
                <Square className="w-3 h-3 fill-current text-rose-400" />
              </button>
            ) : canSubmit ? (
              <button
                type="button"
                onClick={() => handleSubmit()}
                disabled={isUploading}
                className="w-7 h-7 rounded-full bg-slate-100 hover:bg-white text-slate-900 flex items-center justify-center transition-all cursor-pointer shadow-sm"
                title="Send prompt"
              >
                <ArrowUp className="w-3.5 h-3.5 stroke-[2.4]" />
              </button>
            ) : (
              <button
                type="button"
                onClick={() => {
                  setText((prev) => (prev ? prev : "/"));
                  textareaRef.current?.focus();
                }}
                className="w-7 h-7 rounded-full flex items-center justify-center text-slate-500 hover:text-slate-300 transition-colors cursor-pointer"
                title="Type / for commands"
              >
                {/* Subtle command target circle ⊚ chuẩn Claude */}
                <div className="w-4.5 h-4.5 rounded-full border border-slate-600 flex items-center justify-center">
                  <div className="w-1.5 h-1.5 rounded-full bg-slate-400" />
                </div>
              </button>
            )}
          </div>
        </div>
      </div>

      {/* 3. FOOTER ROW: + 🎙 ⌄ Auto                      Opus 5.5 High ◔ */}
      <div className="flex items-center justify-between px-2 pt-2.5 text-xs select-none">
        {/* Left Side: + 🎙 ⌄ Auto */}
        <div className="flex items-center gap-2.5 text-slate-400">
          {/* Plus Add Context Button */}
          <div className="relative" ref={menuRef}>
            <button
              type="button"
              onClick={() => setIsMenuOpen(!isMenuOpen)}
              className="p-1 rounded-full text-slate-400 hover:text-slate-200 hover:bg-[#202532] transition-colors cursor-pointer"
              title="Add Context"
            >
              <Plus className="w-4 h-4 stroke-[2.2]" />
            </button>

            {isMenuOpen && (
              <div className="absolute bottom-full left-0 mb-2 w-52 rounded-xl bg-[#161920] border border-[#282e3c] shadow-2xl py-1.5 z-50 text-slate-300 backdrop-blur-md">
                <div className="px-3 py-1 text-[11px] font-semibold text-slate-500 tracking-wide uppercase">
                  Add Context
                </div>
                <button
                  type="button"
                  onClick={() => {
                    setIsMenuOpen(false);
                    fileInputRef.current?.click();
                  }}
                  className="w-full flex items-center gap-2.5 px-3 py-2 text-[13px] hover:bg-[#202532] hover:text-white transition-colors text-left cursor-pointer"
                >
                  <ImageIcon className="w-4 h-4 text-sky-400 shrink-0" />
                  <span>Media</span>
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setIsMenuOpen(false);
                    setText((prev) => prev + "@");
                    textareaRef.current?.focus();
                  }}
                  className="w-full flex items-center gap-2.5 px-3 py-2 text-[13px] hover:bg-[#202532] hover:text-white transition-colors text-left cursor-pointer"
                >
                  <AtSign className="w-4 h-4 text-indigo-400 shrink-0" />
                  <span>@ Mentions</span>
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setIsMenuOpen(false);
                    setText((prev) => (prev.startsWith("/") ? prev : "/" + prev));
                    textareaRef.current?.focus();
                  }}
                  className="w-full flex items-center gap-2.5 px-3 py-2 text-[13px] hover:bg-[#202532] hover:text-white transition-colors text-left cursor-pointer"
                >
                  <SquareSlash className="w-4 h-4 text-emerald-400 shrink-0" />
                  <span>Actions</span>
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setIsMenuOpen(false);
                    onOpenBrowser?.();
                  }}
                  className="w-full flex items-center gap-2.5 px-3 py-2 text-[13px] hover:bg-[#202532] hover:text-white transition-colors text-left cursor-pointer"
                >
                  <Globe className="w-4 h-4 text-amber-400 shrink-0" />
                  <span>Browser</span>
                </button>
              </div>
            )}
          </div>

          {/* Microphone Voice Button */}
          <button
            type="button"
            onClick={toggleListening}
            className={`p-1 rounded-full transition-all cursor-pointer ${
              isListening
                ? "bg-rose-500/20 text-rose-400 ring-2 ring-rose-500/50 animate-pulse"
                : "text-slate-400 hover:text-slate-200 hover:bg-[#202532]"
            }`}
            title={isListening ? "Listening... Click to stop" : "Voice input"}
          >
            <Mic className="w-3.5 h-3.5" />
          </button>

          {/* ⌄ Auto (Workflow Mode Selector) */}
          <div className="relative" ref={modeRef}>
            <button
              type="button"
              onClick={() => setIsModeOpen(!isModeOpen)}
              className="flex items-center gap-1 px-1.5 py-0.5 rounded-lg text-slate-300 hover:text-white hover:bg-[#202532] transition-colors cursor-pointer text-[12.5px] font-medium"
            >
              <ChevronDown className="w-3.5 h-3.5 text-slate-500" />
              <span className={permissionMode === "bypass" ? "text-rose-300" : ""}>{PERMISSION_MODES.find((m) => m.id === permissionMode)?.label ?? permissionMode}</span>
            </button>

            {isModeOpen && (
              <div className="absolute bottom-full left-0 mb-2 w-48 rounded-xl bg-[#161920] border border-[#282e3c] shadow-2xl py-1.5 z-50 text-slate-300 backdrop-blur-md">
                <div className="px-3 py-1 text-[10.5px] font-semibold text-slate-500 uppercase tracking-wider">
                  Permissions
                </div>
                {PERMISSION_MODES.map((m) => {
                  const disabled = !!allowedModes && !allowedModes.includes(m.id);
                  return (
                  <div
                    key={m.id}
                    title={disabled ? "Not supported by this engine" : undefined}
                    onClick={() => {
                      if (disabled) return;
                      if (m.id === "bypass" && !window.confirm("Full access lets the agent run any command and edit any file in this workspace without asking. Enable for this chat?")) return;
                      onChangePermissionMode?.(m.id);
                      setIsModeOpen(false);
                    }}
                    className={`flex items-center justify-between px-3 py-1.5 text-xs transition-colors ${
                      disabled ? "opacity-35 cursor-not-allowed" : "cursor-pointer"
                    } ${
                      permissionMode === m.id
                        ? "bg-[#212734] text-slate-100 font-medium"
                        : "hover:bg-[#1a202c] text-slate-300"
                    }`}
                  >
                    <div>
                      <div className="font-medium">{m.label}</div>
                      <div className="text-[10px] text-slate-500">{m.desc}</div>
                    </div>
                    {permissionMode === m.id && (
                      <Check className="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                    )}
                  </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>

        {/* Right Side: [ Agent ] [ Model ] [ Effort ] [ Usage Ring ◔ ] */}
        <div className="flex items-center gap-2.5">
          {/* Agent CLI Tool Selector: Antigravity | Claude Code | Codex */}
          {onSelectAgent && (
            <AgentToolSelector
              activeAgent={currentConfig.model.agent}
              onSelectAgent={onSelectAgent}
              aliases={toolAliases}
            />
          )}

          {/* Model & Effort Selector */}
          <ModelSelector
            currentConfig={currentConfig}
            onSelectConfig={onSelectConfig}
            onOpenUsage={() => setIsUsageOpen(true)}
            liveModels={liveModels}
            modelsFetchedAt={modelsFetchedAt}
            onRefreshModels={onRefreshModels}
          />

          {/* Circular Progress Gauge ◔ for Context Window & Usage */}
          <div className="relative flex items-center gap-1.5" ref={usageRef}>
            {/* Nút Compact khi context sắp đầy */}
            {isContextNearLimit && (
              <button
                type="button"
                onClick={handleCompact}
                className="flex items-center gap-1 px-2.5 py-1 rounded-full text-[11px] font-semibold bg-amber-500/20 text-amber-300 border border-amber-500/40 hover:bg-amber-500/30 transition-all cursor-pointer animate-pulse shadow-sm"
                title="Context window is nearly full. Click to run /compact."
              >
                <Zap className="w-3 h-3 text-amber-400" />
                <span>Compact</span>
              </button>
            )}

            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                setIsUsageOpen((prev) => !prev);
              }}
              className="flex items-center gap-2 px-2 py-1 rounded-lg bg-[#181d27] hover:bg-[#202736] border border-[#262f40] hover:border-[#333e54] transition-all cursor-pointer select-none group"
              title={`Local context estimate: ${contextPercent.toFixed(0)}%. Provider quota is unavailable.`}
            >
              {/* Local context estimate only. Provider quota must not be fabricated. */}
              <div className="flex items-center gap-1.5">
                {/* Context bar (fills upward with usage) */}
                <div className="flex flex-col items-center gap-0.5">
                  <div className="w-1 h-3 rounded-full bg-[#232b3b] overflow-hidden flex flex-col justify-end">
                    <div
                      style={{ height: `${contextPercent}%` }}
                      className={`w-full rounded-full transition-all ${
                        contextPercent >= 90 ? "bg-rose-500" : contextPercent >= 70 ? "bg-amber-400" : "bg-sky-400"
                      }`}
                    />
                  </div>
                </div>

              </div>

              <span className="text-[10px] font-medium leading-none text-slate-400">local</span>
            </button>

            {/* Context & Usage Limits Popover */}
            {isUsageOpen && (
              <div
                style={{ width: isContextExpanded ? "360px" : "310px" }}
                className="absolute bottom-full right-0 mb-3 max-h-[580px] overflow-y-auto rounded-xl bg-[#161920] border border-[#272e3d] shadow-2xl p-3.5 z-50 text-slate-200 animate-in fade-in zoom-in-95 duration-150 space-y-3.5"
              >
                {/* Warning Alert if any limit nearly full */}
                {isAnyLimitWarning && (
                  <div
                    className={`p-2 rounded-lg border flex items-center justify-between animate-in fade-in duration-200 ${
                      isAnyLimitExceeded
                        ? "bg-rose-500/10 border-rose-500/25 text-rose-200"
                        : "bg-amber-500/10 border-amber-500/25 text-amber-200"
                    }`}
                  >
                    <div className="flex items-center gap-1.5">
                      <AlertTriangle
                        className={`w-3.5 h-3.5 shrink-0 ${
                          isAnyLimitExceeded ? "text-rose-400" : "text-amber-400"
                        }`}
                      />
                      <div className="text-[11px] font-medium leading-tight">
                        {`Local context estimate gần đầy (${contextPercent.toFixed(1)}%)`}
                      </div>
                    </div>
                  </div>
                )}

                {/* 1. Context Window Section */}
                <div className="space-y-1.5 text-[12px]">
                  {/* Header Row: Context window                     602.3k / 1M (60%) > */}
                  <div
                    onClick={() => {
                      const next = !isContextExpanded;
                      setIsContextExpanded(next);
                      localStorage.setItem("clara_context_expanded", String(next));
                    }}
                    className="flex items-center justify-between cursor-pointer hover:text-white transition-colors select-none group"
                    title={isContextExpanded ? "Collapse breakdown" : "Expand breakdown"}
                  >
                    <span className="text-slate-400 group-hover:text-slate-200 text-[11.5px]">
                      Local context estimate
                    </span>
                    <div className="flex items-center gap-1 font-mono text-[11.5px] text-slate-300 group-hover:text-white">
                      <span>
                        {formatTokens(contextTokens)} / {formatTokens(maxTokens)} ({contextPercent.toFixed(0)}%)
                      </span>
                      {isContextExpanded ? (
                        <ChevronDown className="w-3.5 h-3.5 text-slate-500" />
                      ) : (
                        <ChevronRight className="w-3.5 h-3.5 text-slate-500" />
                      )}
                    </div>
                  </div>

                  {/* Context Bar */}
                  <div className="w-full h-1.5 rounded-full bg-[#202532] overflow-hidden flex select-none">
                    <div
                      className="h-full bg-blue-500 transition-all duration-300"
                      style={{ width: `${contextPercent}%` }}
                    />
                    {isContextExpanded && (
                      <>
                        <div style={{ width: "3%" }} className="h-full bg-orange-500" />
                        <div style={{ width: "2%" }} className="h-full bg-emerald-500" />
                        <div style={{ width: "2%" }} className="h-full bg-amber-400" />
                      </>
                    )}
                  </div>

                  {/* Sub-row: 360k until auto-compact                [ Compact session ] */}
                  <div className="flex items-center justify-between text-[11px] pt-1">
                    <span className="text-slate-400 font-normal">
                      {formatTokens(Math.max(0, maxTokens - contextTokens))} estimated space
                    </span>
                    <button
                      type="button"
                      onClick={handleCompact}
                      className="px-2.5 py-0.5 rounded-md bg-[#252a36] hover:bg-[#303746] text-slate-200 hover:text-white text-[11px] font-medium transition-colors cursor-pointer border border-[#333b4c]"
                      title="Ask the active agent to compact its session"
                    >
                      Compact session
                    </button>
                  </div>

                  {/* Expanded Breakdown Table & Accordions */}
                  {isContextExpanded && (
                    <div className="space-y-1 pt-2 border-t border-[#222836] select-none animate-in fade-in duration-150">
                      <div className="space-y-1">
                        <div className="flex items-center justify-between text-[11px] py-0.5">
                          <div className="flex items-center gap-1.5">
                            <span className="w-2 h-2 rounded-sm bg-blue-500 shrink-0" />
                            <span className="text-slate-300">Messages</span>
                          </div>
                          <span className="text-slate-400 font-mono text-[10.5px]">
                            {formatTokens(contextDetails.messages)} ({((contextDetails.messages / maxTokens) * 100).toFixed(1)}%)
                          </span>
                        </div>

                        <div className="flex items-center justify-between text-[11px] py-0.5">
                          <div className="flex items-center gap-1.5">
                            <span className="w-2 h-2 rounded-sm bg-orange-500 shrink-0" />
                            <span className="text-slate-300">System tools</span>
                          </div>
                          <span className="text-slate-400 font-mono text-[10.5px]">
                            {formatTokens(contextDetails.systemTools)} ({((contextDetails.systemTools / maxTokens) * 100).toFixed(1)}%)
                          </span>
                        </div>

                        <div className="flex items-center justify-between text-[11px] py-0.5">
                          <div className="flex items-center gap-1.5">
                            <span className="w-2 h-2 rounded-sm bg-emerald-500 shrink-0" />
                            <span className="text-slate-300">MCP tools</span>
                          </div>
                          <span className="text-slate-400 font-mono text-[10.5px]">
                            {formatTokens(contextDetails.mcpTools)} ({((contextDetails.mcpTools / maxTokens) * 100).toFixed(1)}%)
                          </span>
                        </div>

                        <div className="flex items-center justify-between text-[11px] py-0.5">
                          <div className="flex items-center gap-1.5">
                            <span className="w-2 h-2 rounded-sm bg-amber-400 shrink-0" />
                            <span className="text-slate-300">Skills</span>
                          </div>
                          <span className="text-slate-400 font-mono text-[10.5px]">
                            {formatTokens(contextDetails.skills)} ({((contextDetails.skills / maxTokens) * 100).toFixed(1)}%)
                          </span>
                        </div>

                        <div className="flex items-center justify-between text-[11px] py-0.5">
                          <div className="flex items-center gap-1.5">
                            <span className="w-2 h-2 rounded-sm bg-slate-500 shrink-0" />
                            <span className="text-slate-300">Free space</span>
                          </div>
                          <span className="text-slate-400 font-mono text-[10.5px]">
                            {formatTokens(contextDetails.freeSpace)} ({((contextDetails.freeSpace / maxTokens) * 100).toFixed(1)}%)
                          </span>
                        </div>
                      </div>
                    </div>
                  )}
                </div>

                {/* 2. Provider usage */}
                <div className="pt-2 border-t border-[#222836] space-y-3">
                  <div className="flex items-center justify-between text-[11px] text-slate-400 font-medium tracking-wide">
                    <span>Provider usage</span>
                    <span className="text-[10px] text-slate-500 bg-[#161a24] px-1.5 py-0.5 rounded border border-[#2b3346]">
                      {currentConfig.model.name}
                    </span>
                  </div>

                  <div className="rounded-lg border border-[#232a3b] bg-[#141824]/60 p-2.5">
                    <div className="flex items-center gap-1.5 text-[11px] font-semibold text-slate-200">
                      <span className="w-1.5 h-1.5 rounded-full bg-slate-400" />
                      Quota unavailable
                    </div>
                    <p className="mt-1 text-[10.5px] leading-relaxed text-slate-400">
                      Agent Hub has no verified provider quota source for this engine. Remaining percentage and reset time are intentionally not shown.
                    </p>
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
