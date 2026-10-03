import React, { useState } from "react";
import { Globe, X, RotateCw, ExternalLink } from "lucide-react";

interface BrowserPanelProps {
  defaultUrl?: string;
  onClose: () => void;
}

export const BrowserPanel: React.FC<BrowserPanelProps> = ({
  defaultUrl = "http://localhost:8088",
  onClose,
}) => {
  const [url, setUrl] = useState(defaultUrl);
  const [activeUrl, setActiveUrl] = useState(defaultUrl);
  const [refreshKey, setRefreshKey] = useState(0);

  const handleNavigate = (e: React.FormEvent) => {
    e.preventDefault();
    let target = url.trim();
    if (!target) return;
    if (!target.startsWith("http://") && !target.startsWith("https://")) {
      target = "http://" + target;
    }
    setActiveUrl(target);
    setUrl(target);
  };

  return (
    <div className="flex flex-col h-full w-full bg-[#11141a] select-none overflow-hidden">
      {/* Navigation Bar */}
      <div className="h-11 px-2.5 bg-[#14171e] border-b border-[#1d222b] flex items-center gap-1.5 shrink-0 text-xs">
        <button
          type="button"
          onClick={() => setRefreshKey((k) => k + 1)}
          className="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-[#1f2533] transition-colors"
          title="Reload"
        >
          <RotateCw className="w-3.5 h-3.5" />
        </button>

        <form onSubmit={handleNavigate} className="flex-1 flex items-center min-w-0">
          <div className="w-full flex items-center gap-1.5 px-2 py-1 rounded-lg bg-[#0d0f15] border border-[#232733] text-xs text-slate-200 focus-within:border-sky-500/50 transition-colors">
            <Globe className="w-3 h-3 text-amber-400 shrink-0" />
            <input
              type="text"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="Enter URL..."
              className="flex-1 bg-transparent border-none outline-none text-slate-100 placeholder-slate-500 font-mono text-[11px] truncate"
            />
          </div>
        </form>

        <div className="flex items-center gap-0.5 shrink-0">
          <a
            href={activeUrl}
            target="_blank"
            rel="noreferrer"
            className="p-1 rounded text-slate-400 hover:text-slate-100 hover:bg-[#1f2533] transition-colors"
            title="Open in external browser"
          >
            <ExternalLink className="w-3.5 h-3.5" />
          </a>
          <button
            type="button"
            onClick={onClose}
            className="p-1 rounded text-slate-400 hover:text-rose-400 hover:bg-[#1f2533] transition-colors"
            title="Close browser panel"
          >
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {/* Embedded Iframe */}
      <div className="flex-1 w-full h-full bg-white relative">
        <iframe
          key={refreshKey}
          src={activeUrl}
          title="Embedded Browser Panel"
          className="w-full h-full border-none"
          sandbox="allow-same-origin allow-scripts allow-popups allow-forms allow-downloads"
        />
      </div>
    </div>
  );
};
