import React, { useState } from "react";
import { Globe, X, RotateCw, ExternalLink } from "lucide-react";

interface BrowserModalProps {
  isOpen: boolean;
  onClose: () => void;
  defaultUrl?: string;
}

export const BrowserModal: React.FC<BrowserModalProps> = ({
  isOpen,
  onClose,
  defaultUrl = "http://localhost:8088",
}) => {
  const [url, setUrl] = useState(defaultUrl);
  const [activeUrl, setActiveUrl] = useState(defaultUrl);
  const [refreshKey, setRefreshKey] = useState(0);

  if (!isOpen) return null;

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
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm animate-in fade-in duration-150">
      <div className="w-[1000px] h-[650px] max-w-full max-h-[92vh] bg-[#12151c] border border-[#232836] rounded-2xl shadow-2xl flex flex-col overflow-hidden">
        {/* Navigation Bar */}
        <div className="h-12 px-3 bg-[#151922] border-b border-[#232836] flex items-center gap-2 select-none">
          <div className="flex items-center gap-1">
            <button
              type="button"
              onClick={() => setRefreshKey((k) => k + 1)}
              className="p-1.5 rounded-lg text-slate-400 hover:text-slate-100 hover:bg-[#202532] transition-colors"
              title="Reload"
            >
              <RotateCw className="w-3.5 h-3.5" />
            </button>
          </div>

          <form onSubmit={handleNavigate} className="flex-1 flex items-center">
            <div className="w-full flex items-center gap-2 px-3 py-1.5 rounded-xl bg-[#0d0f15] border border-[#232733] text-xs text-slate-200 focus-within:border-sky-500/50 transition-colors">
              <Globe className="w-3.5 h-3.5 text-sky-400 shrink-0" />
              <input
                type="text"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="Enter URL (e.g. http://localhost:8088 or https://google.com)"
                className="flex-1 bg-transparent border-none outline-none text-slate-100 placeholder-slate-500 font-mono text-[11.5px]"
              />
              <span className="text-[10px] text-slate-500 bg-[#161a22] px-1.5 py-0.5 rounded border border-[#232833]">
                Browser
              </span>
            </div>
          </form>

          <div className="flex items-center gap-1.5">
            <a
              href={activeUrl}
              target="_blank"
              rel="noreferrer"
              className="p-1.5 rounded-lg text-slate-400 hover:text-slate-100 hover:bg-[#202532] transition-colors"
              title="Open in external browser"
            >
              <ExternalLink className="w-3.5 h-3.5" />
            </a>
            <button
              type="button"
              onClick={onClose}
              className="p-1.5 rounded-lg text-slate-400 hover:text-rose-400 hover:bg-[#202532] transition-colors"
              title="Close browser"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>

        {/* Browser Content (Iframe with sandbox) */}
        <div className="flex-1 w-full h-full bg-white relative">
          <iframe
            key={refreshKey}
            src={activeUrl}
            title="Embedded Browser"
            className="w-full h-full border-none"
            sandbox="allow-same-origin allow-scripts allow-popups allow-forms allow-downloads"
          />
        </div>
      </div>
    </div>
  );
};
