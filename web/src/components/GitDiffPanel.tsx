import React, { useState, useEffect } from "react";
import { GitBranch, RefreshCw, X, Check } from "lucide-react";

interface GitStatusData {
  branch: string;
  files: string[] | null;
  diff: string;
  has_diff: boolean;
}

interface GitDiffPanelProps {
  currentPath?: string;
  onClose: () => void;
}

export const GitDiffPanel: React.FC<GitDiffPanelProps> = ({
  currentPath = "",
  onClose,
}) => {
  const [data, setData] = useState<GitStatusData | null>(null);
  const [loading, setLoading] = useState(false);
  const [activeFile, setActiveFile] = useState<string | null>(null);

  const loadGitStatus = () => {
    if (!currentPath) return;
    setLoading(true);
    fetch(`/api/git/diff?path=${encodeURIComponent(currentPath)}`)
      .then((res) => res.json())
      .then((resData: GitStatusData) => {
        setData(resData);
        if (resData.files && resData.files.length > 0 && !activeFile) {
          setActiveFile(resData.files[0]);
        }
      })
      .catch((err) => console.error("Error loading git diff:", err))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    loadGitStatus();
  }, [currentPath]);

  return (
    <div className="flex flex-col h-full w-full bg-[#11141a] select-none overflow-hidden text-xs">
      {/* Header */}
      <div className="h-11 px-3 border-b border-[#1d222b] flex items-center justify-between bg-[#14171e]">
        <div className="flex items-center gap-2">
          <GitBranch className="w-3.5 h-3.5 text-emerald-400" />
          <span className="font-semibold text-slate-200">Git Changes</span>
          {data?.branch && (
            <span className="px-1.5 py-0.2 rounded bg-[#1e2723] text-emerald-300 font-mono text-[10px] border border-emerald-500/20">
              {data.branch}
            </span>
          )}
        </div>
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={loadGitStatus}
            className="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-[#1f2533] transition-colors"
            title="Refresh Git status"
          >
            <RefreshCw className={`w-3 h-3 ${loading ? "animate-spin" : ""}`} />
          </button>
          <button
            type="button"
            onClick={onClose}
            className="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-[#1f2533] transition-colors"
            title="Close Git panel"
          >
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {/* Changed Files List */}
      <div className="p-2 border-b border-[#1d222b] bg-[#141822]">
        <div className="text-[11px] font-medium text-slate-400 mb-1.5 flex items-center justify-between">
          <span>Modified Files</span>
          <span className="text-[10px] font-mono px-1.5 py-0.2 bg-[#202636] rounded text-slate-300">
            {data?.files ? data.files.length : 0}
          </span>
        </div>

        {data?.files && data.files.length > 0 ? (
          <div className="space-y-0.5 max-h-36 overflow-y-auto">
            {data.files.map((f, i) => (
              <div
                key={i}
                onClick={() => setActiveFile(f)}
                className={`flex items-center justify-between py-1 px-1.5 rounded cursor-pointer transition-colors ${
                  activeFile === f
                    ? "bg-[#1c2436] text-sky-300 font-medium"
                    : "text-slate-300 hover:bg-[#181d28]"
                }`}
              >
                <span className="truncate max-w-[200px]">{f}</span>
                <span className="text-[10px] font-mono text-amber-400">M</span>
              </div>
            ))}
          </div>
        ) : (
          <div className="py-2 text-center text-[11px] text-slate-500 flex items-center justify-center gap-1.5">
            <Check className="w-3 h-3 text-emerald-400" />
            <span>Working tree clean</span>
          </div>
        )}
      </div>

      {/* Diff View */}
      <div className="flex-1 overflow-y-auto overflow-x-auto p-2 font-mono text-[11px] leading-relaxed bg-[#0e1117]">
        {data?.diff ? (
          <div className="space-y-0.5">
            {data.diff.split("\n").map((line, idx) => {
              const isAdd = line.startsWith("+") && !line.startsWith("+++");
              const isDel = line.startsWith("-") && !line.startsWith("---");
              const isHdr = line.startsWith("@@");

              return (
                <div
                  key={idx}
                  className={`px-1 rounded ${
                    isAdd
                      ? "bg-emerald-950/40 text-emerald-300"
                      : isDel
                      ? "bg-rose-950/40 text-rose-300"
                      : isHdr
                      ? "bg-sky-950/40 text-sky-300 font-semibold"
                      : "text-slate-400"
                  }`}
                >
                  {line}
                </div>
              );
            })}
          </div>
        ) : (
          <div className="text-center py-10 text-slate-500">No diff changes</div>
        )}
      </div>
    </div>
  );
};
