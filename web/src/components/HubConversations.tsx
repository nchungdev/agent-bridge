import React, { useEffect, useMemo, useRef, useState } from "react";
import {
  Archive,
  ArchiveRestore,
  ChevronDown,
  ChevronRight,
  Copy,
  Folder,
  FolderInput,
  GitFork,
  MailOpen,
  MoreVertical,
  Pencil,
  Pin,
  PinOff,
  Trash2,
  X,
} from "lucide-react";
import { ConfirmDialog } from "./ConfirmDialog";

export interface HubConv {
  id: string;
  name: string;
  workspace?: string;
  updated_at?: string;
  pinned: boolean;
  archived: boolean;
  group: string;
  unread: boolean;
}

export type ConvAction =
  | { type: "pin" | "unpin" | "unread" | "copylink" | "fork" | "archive" | "unarchive" | "delete"; id: string }
  | { type: "rename" | "group"; id: string; value: string };

interface Props {
  convs: HubConv[];
  activeId: string | null;
  onSelect: (id: string) => void;
  onAction: (a: ConvAction) => void;
}

function relativeTime(s?: string): string {
  if (!s) return "";
  const t = Date.parse(s.includes("T") ? s : s.replace(" ", "T") + "Z");
  if (Number.isNaN(t)) return "";
  const m = Math.max(0, Math.round((Date.now() - t) / 60000));
  if (m < 1) return "now";
  if (m < 60) return `${m}m`;
  if (m < 60 * 24) return `${Math.floor(m / 60)}h`;
  return `${Math.floor(m / 1440)}d`;
}

interface MenuState { id: string; x: number; y: number }

export const HubConversations: React.FC<Props> = ({ convs, activeId, onSelect, onAction }) => {
  const [menu, setMenu] = useState<MenuState | null>(null);
  const [groupMenu, setGroupMenu] = useState(false);
  const [newGroup, setNewGroup] = useState<string | null>(null);
  const [renaming, setRenaming] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);
  const [showArchived, setShowArchived] = useState(false);
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});
  const [toast, setToast] = useState<string | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  const live = convs.filter((c) => !c.archived);
  const pinned = live.filter((c) => c.pinned);
  const groups = useMemo(() => {
    const m = new Map<string, HubConv[]>();
    for (const c of live.filter((x) => !x.pinned)) {
      const g = c.group || "Agent Hub";
      m.set(g, [...(m.get(g) ?? []), c]);
    }
    // "Agent Hub" (ungrouped) first, then named groups alphabetically
    return [...m.entries()].sort(([a], [b]) => (a === "Agent Hub" ? -1 : b === "Agent Hub" ? 1 : a.localeCompare(b)));
  }, [convs]);
  const archived = convs.filter((c) => c.archived);
  const allGroups = useMemo(() => [...new Set(convs.map((c) => c.group).filter(Boolean))].sort(), [convs]);
  const target = menu ? convs.find((c) => c.id === menu.id) : undefined;

  const closeMenu = () => { setMenu(null); setGroupMenu(false); setNewGroup(null); };
  const flash = (m: string) => { setToast(m); window.setTimeout(() => setToast(null), 1600); };

  const run = (type: ConvAction["type"], c: HubConv) => {
    switch (type) {
      case "rename":
        setRenaming(c.id); setRenameValue(c.name); closeMenu(); return;
      case "delete":
        setConfirmDelete(c.id); closeMenu(); return;
      case "copylink":
        onAction({ type: "copylink", id: c.id }); flash("Link copied"); closeMenu(); return;
      case "unread":
        onAction({ type: "unread", id: c.id }); flash("Marked as unread"); closeMenu(); return;
      case "group":
        setGroupMenu(true); return;
      default:
        onAction({ type, id: c.id } as ConvAction); closeMenu();
    }
  };

  useEffect(() => {
    if (!menu) return;
    const onDown = (e: MouseEvent) => { if (menuRef.current && !menuRef.current.contains(e.target as Node)) closeMenu(); };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") { closeMenu(); return; }
      if (newGroup !== null || !target || e.metaKey || e.ctrlKey || e.altKey) return;
      const map: Record<string, ConvAction["type"]> = { p: target.pinned ? "unpin" : "pin", u: "unread", r: "rename", c: "copylink", f: "fork", a: target.archived ? "unarchive" : "archive", d: "delete" };
      const t = map[e.key.toLowerCase()];
      if (t) { e.preventDefault(); run(t, target); }
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => { document.removeEventListener("mousedown", onDown); document.removeEventListener("keydown", onKey); };
  }, [menu, target, newGroup]);

  const openMenu = (e: React.MouseEvent, id: string) => {
    e.preventDefault();
    e.stopPropagation();
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const x = e.type === "contextmenu" ? e.clientX : r.right;
    const y = e.type === "contextmenu" ? e.clientY : r.bottom;
    setGroupMenu(false); setNewGroup(null);
    setMenu({ id, x: Math.min(x, window.innerWidth - 230), y: Math.min(y, window.innerHeight - 330) });
  };

  const Row = ({ c }: { c: HubConv }) => {
    const isActive = c.id === activeId;
    if (renaming === c.id) {
      return (
        <div className="px-2 py-1">
          <input
            autoFocus
            value={renameValue}
            onChange={(e) => setRenameValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && renameValue.trim()) { onAction({ type: "rename", id: c.id, value: renameValue.trim() }); setRenaming(null); }
              if (e.key === "Escape") setRenaming(null);
            }}
            onBlur={() => setRenaming(null)}
            className="w-full rounded-md bg-[#0f1218] border border-sky-600/60 px-2 py-1 text-xs text-slate-100 outline-none"
          />
        </div>
      );
    }
    return (
      <div
        onClick={() => onSelect(c.id)}
        onContextMenu={(e) => openMenu(e, c.id)}
        className={`group flex items-center justify-between pl-2.5 pr-1 h-8 rounded-lg text-xs cursor-pointer transition-colors ${
          isActive ? "bg-[#212733] text-slate-100 font-medium shadow-sm" : "text-slate-400 hover:bg-[#1a1f28] hover:text-slate-200"
        }`}
      >
        <span className="flex items-center gap-1.5 min-w-0">
          {c.unread && !isActive && <span className="w-1.5 h-1.5 rounded-full bg-sky-400 shrink-0" title="Unread" />}
          {c.pinned && <Pin className="w-3 h-3 text-slate-500 shrink-0" />}
          <span className="truncate pr-2">{c.name || "Untitled"}</span>
        </span>
        {/* fixed-size slot: time and ⋮ are stacked, only opacity changes on hover => row height never changes */}
        <span className="relative shrink-0 w-9 h-6 flex items-center justify-end">
          <span className="text-[11px] text-slate-500 font-mono group-hover:opacity-0 transition-opacity">{relativeTime(c.updated_at)}</span>
          <button
            type="button"
            onClick={(e) => openMenu(e, c.id)}
            className="absolute inset-y-0 right-0 w-6 flex items-center justify-center rounded opacity-0 group-hover:opacity-100 hover:bg-[#2a3142] text-slate-400 hover:text-slate-100 cursor-pointer transition-opacity"
            title="More actions"
            aria-label="More actions"
          >
            <MoreVertical className="w-3.5 h-3.5" />
          </button>
        </span>
      </div>
    );
  };

  const Section = ({ title, items, icon }: { title: string; items: HubConv[]; icon?: React.ReactNode }) => {
    const isCollapsed = collapsed[title];
    return (
      <div className="space-y-0.5">
        <div
          className="flex items-center gap-1.5 px-2.5 py-1 text-xs text-slate-400 font-medium cursor-pointer hover:text-slate-200"
          onClick={() => setCollapsed((p) => ({ ...p, [title]: !p[title] }))}
        >
          {icon ?? <Folder className="w-3.5 h-3.5 text-slate-500" />}
          <span className="truncate">{title}</span>
          <ChevronDown className={`w-3 h-3 text-slate-600 transition-transform ${isCollapsed ? "-rotate-90" : ""}`} />
        </div>
        {!isCollapsed && <div className="space-y-0.5 pl-1">{items.map((c) => <Row key={c.id} c={c} />)}</div>}
      </div>
    );
  };

  const Item = ({ icon, label, kbd, onClick, danger, chevron }: { icon: React.ReactNode; label: string; kbd?: string; onClick: () => void; danger?: boolean; chevron?: boolean }) => (
    <div
      onClick={onClick}
      className={`flex items-center justify-between gap-3 px-3 py-1.5 mx-1 rounded-lg text-[13px] cursor-pointer ${danger ? "text-rose-400 hover:bg-rose-950/40" : "text-slate-200 hover:bg-[#262c3a]"}`}
    >
      <span className="flex items-center gap-2.5">{icon}{label}</span>
      {chevron ? <ChevronRight className="w-3.5 h-3.5 text-slate-500" /> : kbd && <span className="text-[11px] font-mono text-slate-500">{kbd}</span>}
    </div>
  );

  return (
    <>
      <div className="space-y-2 pb-1">
        {pinned.length > 0 && <Section title="Pinned" items={pinned} icon={<Pin className="w-3.5 h-3.5 text-slate-500" />} />}
        {groups.map(([g, items]) => <Section key={g} title={g} items={items} />)}
        {archived.length > 0 && (
          <div>
            <div className="flex items-center gap-1.5 px-2.5 py-1 text-xs text-slate-500 cursor-pointer hover:text-slate-300" onClick={() => setShowArchived((s) => !s)}>
              <Archive className="w-3.5 h-3.5" />
              <span>Archived ({archived.length})</span>
              <ChevronDown className={`w-3 h-3 transition-transform ${showArchived ? "" : "-rotate-90"}`} />
            </div>
            {showArchived && <div className="space-y-0.5 pl-1">{archived.map((c) => <Row key={c.id} c={c} />)}</div>}
          </div>
        )}
      </div>

      {menu && target && (
        <div
          ref={menuRef}
          style={{ position: "fixed", left: menu.x, top: menu.y, width: 220 }}
          className="z-50 rounded-xl border border-[#2c3344] bg-[#191c24] py-1.5 shadow-2xl text-slate-200 select-none"
        >
          {!groupMenu ? (
            <>
              <Item icon={target.pinned ? <PinOff className="w-4 h-4" /> : <Pin className="w-4 h-4" />} label={target.pinned ? "Unpin" : "Pin"} kbd="P" onClick={() => run(target.pinned ? "unpin" : "pin", target)} />
              <Item icon={<MailOpen className="w-4 h-4" />} label="Mark as unread" kbd="U" onClick={() => run("unread", target)} />
              <Item icon={<Pencil className="w-4 h-4" />} label="Rename" kbd="R" onClick={() => run("rename", target)} />
              <Item icon={<Copy className="w-4 h-4" />} label="Copy link" kbd="C" onClick={() => run("copylink", target)} />
              <Item icon={<GitFork className="w-4 h-4" />} label="Fork" kbd="F" onClick={() => run("fork", target)} />
              <div className="my-1 border-t border-[#2a2f3c]" />
              <Item icon={<FolderInput className="w-4 h-4" />} label="Move to group" chevron onClick={() => run("group", target)} />
              <div className="my-1 border-t border-[#2a2f3c]" />
              <Item icon={target.archived ? <ArchiveRestore className="w-4 h-4" /> : <Archive className="w-4 h-4" />} label={target.archived ? "Unarchive" : "Archive"} kbd="A" onClick={() => run(target.archived ? "unarchive" : "archive", target)} />
              <Item icon={<Trash2 className="w-4 h-4" />} label="Delete" kbd="D" danger onClick={() => run("delete", target)} />
            </>
          ) : (
            <>
              <div className="flex items-center justify-between px-3 pb-1 text-[11px] uppercase tracking-wider text-slate-500">
                Move to group
                <button onClick={() => { setGroupMenu(false); setNewGroup(null); }} className="text-slate-500 hover:text-slate-200 cursor-pointer" aria-label="Back"><X className="w-3 h-3" /></button>
              </div>
              {target.group && <Item icon={<X className="w-4 h-4" />} label="No group" onClick={() => { onAction({ type: "group", id: target.id, value: "" }); closeMenu(); }} />}
              {allGroups.filter((g) => g !== target.group).map((g) => (
                <Item key={g} icon={<Folder className="w-4 h-4" />} label={g} onClick={() => { onAction({ type: "group", id: target.id, value: g }); closeMenu(); }} />
              ))}
              {newGroup === null ? (
                <Item icon={<FolderInput className="w-4 h-4" />} label="New group…" onClick={() => setNewGroup("")} />
              ) : (
                <div className="px-3 py-1">
                  <input
                    autoFocus
                    value={newGroup}
                    placeholder="Group name"
                    onChange={(e) => setNewGroup(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && newGroup.trim()) { onAction({ type: "group", id: target.id, value: newGroup.trim() }); closeMenu(); }
                      if (e.key === "Escape") setNewGroup(null);
                    }}
                    className="w-full rounded-md bg-[#0f1218] border border-sky-600/60 px-2 py-1 text-xs text-slate-100 outline-none"
                  />
                </div>
              )}
            </>
          )}
        </div>
      )}
      {confirmDelete && (() => {
        const c = convs.find((x) => x.id === confirmDelete);
        return (
          <ConfirmDialog
            danger
            title="Delete conversation?"
            message={<>“{c?.name || "Untitled"}” and its history will be permanently deleted. This cannot be undone.</>}
            confirmLabel="Delete"
            onConfirm={() => { onAction({ type: "delete", id: confirmDelete }); setConfirmDelete(null); }}
            onCancel={() => setConfirmDelete(null)}
          />
        );
      })()}
      {toast && <div className="fixed bottom-4 left-4 z-50 rounded-lg bg-[#222836] border border-[#2c3344] px-3 py-1.5 text-xs text-slate-200 shadow-lg">{toast}</div>}
    </>
  );
};
