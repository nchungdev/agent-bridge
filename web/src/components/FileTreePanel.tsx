import React, { useState, useEffect } from "react";
import {
  Folder,
  FolderOpen,
  FileText,
  FileCode,
  FileJson,
  ChevronRight,
  ChevronDown,
  RefreshCw,
  PanelLeftClose,
} from "lucide-react";

export interface FileNode {
  name: string;
  path: string;
  is_dir: boolean;
  size?: number;
  children?: FileNode[];
}

interface FileTreePanelProps {
  currentPath?: string;
  onSelectFile?: (filePath: string) => void;
  onClose?: () => void;
}

export const FileTreePanel: React.FC<FileTreePanelProps> = ({
  currentPath = "/home/chungnh/AI Workspace",
  onSelectFile,
  onClose,
}) => {
  const [rootNode, setRootNode] = useState<FileNode | null>(null);
  const [loading, setLoading] = useState(false);
  const [expandedPaths, setExpandedPaths] = useState<Set<string>>(new Set());

  const loadTree = () => {
    setLoading(true);
    fetch(`/api/fs/tree?path=${encodeURIComponent(currentPath)}`)
      .then((res) => res.json())
      .then((data: FileNode) => {
        setRootNode(data);
        // Tự động mở thư mục gốc
        if (data && data.path) {
          setExpandedPaths((prev) => new Set(prev).add(data.path));
        }
      })
      .catch((err) => console.error("Error loading file tree:", err))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    loadTree();
  }, [currentPath]);

  const toggleExpand = (path: string) => {
    setExpandedPaths((prev) => {
      const next = new Set(prev);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return next;
    });
  };

  return (
    <div className="flex flex-col h-full bg-[#11141a] border-r border-[#1d222b] w-64 select-none shrink-0 overflow-hidden text-xs">
      {/* Header */}
      <div className="h-11 px-3 border-b border-[#1d222b] flex items-center justify-between bg-[#14171e]">
        <div className="flex items-center gap-2">
          <Folder className="w-3.5 h-3.5 text-sky-400" />
          <span className="font-semibold text-slate-200">Files</span>
        </div>
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={loadTree}
            className="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-[#1f2533] transition-colors"
            title="Refresh files"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? "animate-spin" : ""}`} />
          </button>
          {onClose && (
            <button
              type="button"
              onClick={onClose}
              className="p-1 rounded text-slate-400 hover:text-slate-200 hover:bg-[#1f2533] transition-colors"
              title="Collapse files panel"
            >
              <PanelLeftClose className="w-3.5 h-3.5" />
            </button>
          )}
        </div>
      </div>

      {/* Tree Content */}
      <div className="flex-1 overflow-y-auto overflow-x-hidden p-2 space-y-0.5">
        {rootNode ? (
          <TreeNode
            node={rootNode}
            level={0}
            expandedPaths={expandedPaths}
            onToggleExpand={toggleExpand}
            onSelectFile={onSelectFile}
          />
        ) : (
          <div className="text-center py-8 text-slate-500">Loading files...</div>
        )}
      </div>
    </div>
  );
};

interface TreeNodeProps {
  node: FileNode;
  level: number;
  expandedPaths: Set<string>;
  onToggleExpand: (path: string) => void;
  onSelectFile?: (filePath: string) => void;
}

const TreeNode: React.FC<TreeNodeProps> = ({
  node,
  level,
  expandedPaths,
  onToggleExpand,
  onSelectFile,
}) => {
  const isExpanded = expandedPaths.has(node.path);

  const getFileIcon = (fileName: string) => {
    if (fileName.endsWith(".ts") || fileName.endsWith(".tsx") || fileName.endsWith(".js") || fileName.endsWith(".jsx")) {
      return <FileCode className="w-3.5 h-3.5 text-sky-400 shrink-0" />;
    }
    if (fileName.endsWith(".go")) {
      return <FileCode className="w-3.5 h-3.5 text-cyan-400 shrink-0" />;
    }
    if (fileName.endsWith(".json") || fileName.endsWith(".yaml") || fileName.endsWith(".yml")) {
      return <FileJson className="w-3.5 h-3.5 text-amber-400 shrink-0" />;
    }
    if (fileName.endsWith(".md")) {
      return <FileText className="w-3.5 h-3.5 text-purple-400 shrink-0" />;
    }
    return <FileText className="w-3.5 h-3.5 text-slate-400 shrink-0" />;
  };

  if (node.is_dir) {
    return (
      <div>
        <div
          onClick={() => onToggleExpand(node.path)}
          className="flex items-center gap-1.5 py-1 px-1.5 rounded hover:bg-[#1a1f2b] cursor-pointer text-slate-300 hover:text-slate-100"
          style={{ paddingLeft: `${Math.max(level * 12, 6)}px` }}
        >
          {isExpanded ? (
            <ChevronDown className="w-3 h-3 text-slate-500 shrink-0" />
          ) : (
            <ChevronRight className="w-3 h-3 text-slate-500 shrink-0" />
          )}
          {isExpanded ? (
            <FolderOpen className="w-3.5 h-3.5 text-amber-400 shrink-0" />
          ) : (
            <Folder className="w-3.5 h-3.5 text-amber-400 shrink-0" />
          )}
          <span className="truncate font-medium">{node.name}</span>
        </div>

        {isExpanded && node.children && (
          <div>
            {node.children.map((child) => (
              <TreeNode
                key={child.path}
                node={child}
                level={level + 1}
                expandedPaths={expandedPaths}
                onToggleExpand={onToggleExpand}
                onSelectFile={onSelectFile}
              />
            ))}
          </div>
        )}
      </div>
    );
  }

  return (
    <div
      onClick={() => onSelectFile && onSelectFile(node.path)}
      className="flex items-center gap-1.5 py-1 px-1.5 rounded hover:bg-[#1a1f2b] cursor-pointer text-slate-400 hover:text-slate-200 transition-colors"
      style={{ paddingLeft: `${Math.max(level * 12 + 14, 20)}px` }}
    >
      {getFileIcon(node.name)}
      <span className="truncate">{node.name}</span>
    </div>
  );
};
