import React, { useEffect, useState } from "react";
import { ArrowUp, FolderPlus, X } from "lucide-react";

interface Props {
  isOpen: boolean;
  onClose: () => void;
  onCreateProject: (path: string, name?: string, autoCreate?: boolean) => Promise<void>;
}

export const NewProjectModal: React.FC<Props> = ({ isOpen, onClose, onCreateProject }) => {
  const [projectPath, setProjectPath] = useState("");
  const [projectName, setProjectName] = useState("");
  const [autoCreateDir, setAutoCreateDir] = useState(true);
  const [fsCurrent, setFsCurrent] = useState("");
  const [fsParent, setFsParent] = useState("");
  const [fsDirs, setFsDirs] = useState<string[]>([]);

  const loadDirectories = async (path?: string) => {
    try {
      const q = path ? `?path=${encodeURIComponent(path)}` : "";
      const res = await fetch(`/api/bridge/fs/directories${q}`);
      if (res.ok) {
        const d = await res.json();
        setFsCurrent(d.current || "");
        setFsParent(d.parent || "");
        setFsDirs(d.directories || []);
      }
    } catch {
      /* ignore */
    }
  };

  useEffect(() => {
    if (isOpen) {
      loadDirectories();
    }
  }, [isOpen]);

  const handleSelectFolder = (path: string) => {
    setProjectPath(path);
    const base = path.split("/").filter(Boolean).pop() || "project";
    if (!projectName) {
      setProjectName(base);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!projectPath.trim()) return;
    await onCreateProject(projectPath.trim(), projectName.trim() || undefined, autoCreateDir);
    setProjectPath("");
    setProjectName("");
    onClose();
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4 animate-in fade-in duration-150">
      <div className="w-full max-w-lg rounded-xl border border-[#2b3548] bg-[#121620] shadow-2xl overflow-hidden flex flex-col text-slate-200">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-[#1f2636] px-5 py-3.5 bg-[#161b27]">
          <div className="flex items-center gap-2">
            <FolderPlus className="h-4 w-4 text-emerald-400" />
            <span className="text-sm font-semibold text-slate-100">Add Project</span>
          </div>
          <button
            onClick={onClose}
            className="cursor-pointer p-1 text-slate-500 hover:text-slate-300 rounded"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Body */}
        <form onSubmit={handleSubmit} className="p-5 space-y-4 text-xs">
          <div>
            <label className="block text-[11px] font-semibold uppercase tracking-wider text-slate-400 mb-1.5">
              Folder Path
            </label>
            <div className="flex items-center gap-2">
              <input
                type="text"
                required
                placeholder="~/my-project or /path/to/project"
                value={projectPath}
                onChange={(e) => handleSelectFolder(e.target.value)}
                className="flex-1 rounded-lg border border-[#2c3549] bg-[#0c0e14] px-3 py-2 text-xs font-mono text-slate-100 placeholder-slate-600 outline-none focus:border-indigo-500"
              />
              <button
                type="button"
                onClick={() => {
                  if (!fsCurrent) loadDirectories(projectPath || undefined);
                }}
                className="cursor-pointer rounded-lg border border-[#2c3549] bg-[#1a202c] px-3 py-2 text-xs font-medium text-slate-300 hover:bg-[#232b3b]"
              >
                Browse
              </button>
            </div>
          </div>

          {/* Directory Browser */}
          {fsCurrent && (
            <div className="rounded-lg border border-[#222a3a] bg-[#0c0f16] p-2.5 space-y-2">
              <div className="flex items-center justify-between text-[11px] text-slate-400 border-b border-[#1c2230] pb-1.5">
                <span className="truncate font-mono" title={fsCurrent}>📂 {fsCurrent}</span>
                {fsParent && (
                  <button
                    type="button"
                    onClick={() => loadDirectories(fsParent)}
                    className="flex cursor-pointer items-center gap-1 rounded bg-[#181f2c] px-2 py-0.5 text-[10.5px] text-indigo-300 hover:bg-indigo-900/40"
                  >
                    <ArrowUp className="h-3 w-3" /> Up
                  </button>
                )}
              </div>
              <div className="max-h-36 overflow-y-auto space-y-0.5 pr-1">
                {fsDirs.length === 0 ? (
                  <div className="py-2 text-center text-[11px] text-slate-500">No subdirectories found.</div>
                ) : (
                  fsDirs.map((dir) => (
                    <div
                      key={dir}
                      className="flex items-center justify-between rounded px-2 py-1 hover:bg-[#181f2d] group cursor-pointer"
                      onClick={() => {
                        const sub = fsCurrent.endsWith("/") ? `${fsCurrent}${dir}` : `${fsCurrent}/${dir}`;
                        handleSelectFolder(sub);
                        loadDirectories(sub);
                      }}
                    >
                      <span className="truncate font-mono text-[11px] text-slate-300 group-hover:text-indigo-200">
                        📁 {dir}
                      </span>
                      <span className="text-[10px] text-slate-500 opacity-0 group-hover:opacity-100">
                        Select
                      </span>
                    </div>
                  ))
                )}
              </div>
            </div>
          )}

          <div>
            <label className="block text-[11px] font-semibold uppercase tracking-wider text-slate-400 mb-1.5">
              Project Name (Optional)
            </label>
            <input
              type="text"
              placeholder="e.g. Backend API, Mobile App..."
              value={projectName}
              onChange={(e) => setProjectName(e.target.value)}
              className="w-full rounded-lg border border-[#2c3549] bg-[#0c0e14] px-3 py-2 text-xs text-slate-100 placeholder-slate-600 outline-none focus:border-indigo-500"
            />
            <span className="mt-1 block text-[10.5px] text-slate-500">
              Display name in sidebar (does not change folder path).
            </span>
          </div>

          <div className="flex items-center gap-2 pt-1">
            <input
              type="checkbox"
              id="auto_create_dir_modal"
              checked={autoCreateDir}
              onChange={(e) => setAutoCreateDir(e.target.checked)}
              className="rounded border-[#2c3549] bg-[#0c0e14] text-indigo-500 focus:ring-0 cursor-pointer"
            />
            <label htmlFor="auto_create_dir_modal" className="text-[11.5px] text-slate-300 cursor-pointer">
              Create folder if it does not exist
            </label>
          </div>

          {/* Footer Actions */}
          <div className="flex items-center justify-end gap-2.5 pt-3 border-t border-[#1c2230]">
            <button
              type="button"
              onClick={onClose}
              className="cursor-pointer rounded-lg border border-[#2c3549] bg-[#161a24] px-4 py-2 text-xs font-medium text-slate-300 hover:bg-[#202634]"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={!projectPath.trim()}
              className="cursor-pointer rounded-lg bg-indigo-600 px-4 py-2 text-xs font-medium text-white hover:bg-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed shadow"
            >
              Add Project
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
