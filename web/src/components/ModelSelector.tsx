import React, { useState, useRef, useEffect, useMemo } from "react";
import { Check, ChevronRight, AlertTriangle } from "lucide-react";
import { ALL_MODELS, PROVIDER_GROUPS, type ModelDefinition } from "../lib/models";

export type EffortLevel = "Low" | "Medium" | "High";

export interface SelectedModelConfig {
  model: ModelDefinition;
  effort: EffortLevel;
}

interface ModelSelectorProps {
  currentConfig: SelectedModelConfig;
  onSelectConfig: (config: SelectedModelConfig) => void;
  onOpenUsage?: () => void;
}

export const ModelSelector: React.FC<ModelSelectorProps> = ({
  currentConfig,
  onSelectConfig,
  onOpenUsage,
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const [isEffortMenuOpen, setIsEffortMenuOpen] = useState(false);
  const [isMoreModelsOpen, setIsMoreModelsOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const moreItemRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setIsOpen(false);
        setIsEffortMenuOpen(false);
        setIsMoreModelsOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const currentAgent = currentConfig.model.agent || "agy";

  // Lấy các model của Agent hiện tại làm Featured models (ưu tiên hiển thị ở Menu chính)
  const featuredModels = useMemo(() => {
    const agentModels = ALL_MODELS.filter((m) => m.agent === currentAgent);
    return agentModels.slice(0, 4).map((m, idx) => ({
      ...m,
      shortcut: String(idx + 1),
    }));
  }, [currentAgent]);

  // Các model còn lại sẽ nằm trong Flyout "More models"
  const moreModels = useMemo(() => {
    return ALL_MODELS.filter((m) => !featuredModels.some((f) => f.id === m.id));
  }, [featuredModels]);

  // Phân nhóm theo Provider cho Flyout "More models"
  const moreGroups = useMemo(() => {
    return PROVIDER_GROUPS.map((group) => {
      const models = group.models.filter((m) => !featuredModels.some((f) => f.id === m.id));
      return {
        ...group,
        models,
      };
    }).filter((g) => g.models.length > 0);
  }, [featuredModels]);

  // Phím tắt bàn phím 1, 2, 3, 4 khi popup mở
  useEffect(() => {
    if (!isOpen) return;

    function handleKeyDown(e: KeyboardEvent) {
      if (e.key >= "1" && e.key <= "4") {
        const idx = parseInt(e.key, 10) - 1;
        if (featuredModels[idx]) {
          onSelectConfig({
            model: featuredModels[idx],
            effort: currentConfig.effort,
          });
          setIsOpen(false);
          setIsMoreModelsOpen(false);
        }
      } else if (e.key === "Escape") {
        setIsOpen(false);
        setIsMoreModelsOpen(false);
      }
    }

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [isOpen, featuredModels, currentConfig.effort, onSelectConfig]);

  const handleSelectModel = (model: ModelDefinition) => {
    onSelectConfig({ model, effort: currentConfig.effort });
    setIsOpen(false);
    setIsMoreModelsOpen(false);
  };

  const effortOptions: EffortLevel[] = ["Low", "Medium", "High"];

  return (
    <div className="relative inline-flex items-center gap-2" ref={containerRef}>
      {/* Model Name Button */}
      <button
        type="button"
        onClick={() => {
          setIsOpen(!isOpen);
          setIsEffortMenuOpen(false);
        }}
        className="text-[12.5px] font-medium text-slate-300 hover:text-white transition-colors cursor-pointer select-none"
        title="Select model"
      >
        {currentConfig.model.name}
      </button>

      {/* Effort Level Button */}
      <button
        type="button"
        onClick={() => {
          setIsEffortMenuOpen(!isEffortMenuOpen);
          setIsOpen(false);
          setIsMoreModelsOpen(false);
        }}
        className="text-[12.5px] text-slate-400 hover:text-slate-200 transition-colors cursor-pointer select-none"
        title="Select thinking / effort level"
      >
        {currentConfig.effort}
      </button>

      {/* Effort Menu Dropdown */}
      {isEffortMenuOpen && (
        <div className="absolute bottom-full right-0 mb-2 w-32 bg-[#1c202a] border border-[#2c3344] rounded-xl shadow-2xl py-1 z-50 animate-in fade-in zoom-in-95 duration-100">
          <div className="px-3 py-1 text-[10.5px] font-semibold text-slate-500 uppercase tracking-wider select-none">
            Effort Level
          </div>
          {effortOptions.map((effort) => (
            <div
              key={effort}
              onClick={() => {
                onSelectConfig({ model: currentConfig.model, effort });
                setIsEffortMenuOpen(false);
              }}
              className={`flex items-center justify-between px-3 py-1.5 text-xs cursor-pointer transition-colors ${
                currentConfig.effort === effort
                  ? "bg-[#252c3c] text-white font-medium"
                  : "hover:bg-[#232938] text-slate-300"
              }`}
            >
              <span>{effort}</span>
              {currentConfig.effort === effort && (
                <Check className="w-3.5 h-3.5 text-[#38bdf8]" />
              )}
            </div>
          ))}
        </div>
      )}

      {/* Main Model Popup (Style Claude - Hình 2) */}
      {isOpen && (
        <div className="absolute bottom-full right-0 mb-2.5 z-50 animate-in fade-in zoom-in-95 duration-100">
          <div className="relative">
            {/* Main Menu Box */}
            <div className="w-64 bg-[#191c24] border border-[#2c3344] rounded-2xl shadow-2xl p-1.5 text-slate-200">
              {/* Featured Primary Models List */}
              <div className="space-y-0.5">
                {featuredModels.map((model) => {
                  const isSelected = model.id === currentConfig.model.id;
                  return (
                    <div
                      key={model.id}
                      onClick={() => handleSelectModel(model)}
                      className={`flex items-center justify-between px-3 py-2 rounded-xl text-[13px] cursor-pointer transition-colors ${
                        isSelected
                          ? "bg-[#262c3a] text-white font-medium"
                          : "hover:bg-[#222734] text-slate-200"
                      }`}
                    >
                      <div className="flex items-center gap-1.5 flex-1 min-w-0 pr-2">
                        <span className="truncate">{model.name}</span>
                        {model.isWarning && (
                          <AlertTriangle className="w-3 h-3 text-amber-400 shrink-0" />
                        )}
                      </div>

                      <div className="shrink-0 flex items-center justify-end w-5">
                        {isSelected ? (
                          <Check className="w-4 h-4 text-[#38bdf8] stroke-[2.5]" />
                        ) : (
                          model.shortcut && (
                            <span className="text-[12px] font-mono text-slate-500">
                              {model.shortcut}
                            </span>
                          )
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>

              {/* Divider nếu có thêm models */}
              {moreModels.length > 0 && (
                <>
                  <div className="my-1 border-t border-[#252a36]" />

                  {/* "More models" Row with Flyout Trigger */}
                  <div
                    ref={moreItemRef}
                    onMouseEnter={() => setIsMoreModelsOpen(true)}
                    onClick={() => setIsMoreModelsOpen(!isMoreModelsOpen)}
                    className={`flex items-center justify-between px-3 py-2 rounded-xl text-[13px] cursor-pointer transition-colors ${
                      isMoreModelsOpen
                        ? "bg-[#242938] text-white"
                        : "hover:bg-[#222734] text-slate-200"
                    }`}
                  >
                    <span>More models</span>
                    <ChevronRight className="w-4 h-4 text-slate-500" />
                  </div>
                </>
              )}

              {/* Context & Limits Row */}
              {onOpenUsage && (
                <>
                  <div className="my-1 border-t border-[#252a36]" />
                  <div
                    onClick={() => {
                      setIsOpen(false);
                      setIsMoreModelsOpen(false);
                      onOpenUsage();
                    }}
                    className="flex items-center justify-between px-3 py-1.5 rounded-xl text-[12px] text-sky-400 hover:text-sky-300 hover:bg-[#202738] cursor-pointer transition-colors"
                  >
                    <span>Context Window & Limits</span>
                    <span className="text-[10px] text-slate-500 font-mono">◐</span>
                  </div>
                </>
              )}
            </div>

            {/* Flyout Submenu ("More models" - Grouped by Provider) */}
            {isMoreModelsOpen && moreGroups.length > 0 && (
              <div
                onMouseEnter={() => setIsMoreModelsOpen(true)}
                className="absolute right-full bottom-0 mr-1.5 w-72 max-h-[min(500px,80vh)] overflow-y-auto bg-[#191c24] border border-[#2c3344] rounded-2xl shadow-2xl p-1.5 text-slate-200 z-50 animate-in fade-in slide-in-from-right-1 duration-100 space-y-2 scrollbar-thin scrollbar-thumb-slate-700"
              >
                {moreGroups.map((group) => (
                  <div key={group.id} className="space-y-0.5">
                    <div className="px-2.5 pt-1.5 pb-0.5 text-[10px] font-semibold text-slate-500 uppercase tracking-wider">
                      {group.name}
                    </div>
                    {group.models.map((model) => {
                      const isSelected = model.id === currentConfig.model.id;
                      return (
                        <div
                          key={model.id}
                          onClick={() => handleSelectModel(model)}
                          className={`flex items-center justify-between px-3 py-1.5 rounded-xl text-[12.5px] cursor-pointer transition-colors ${
                            isSelected
                              ? "bg-[#262c3a] text-white font-medium"
                              : "hover:bg-[#222734] text-slate-200"
                          }`}
                        >
                          <div className="flex items-center gap-1.5 flex-1 min-w-0 pr-2">
                            <span className="truncate">{model.name}</span>
                            {model.isWarning && (
                              <AlertTriangle className="w-3 h-3 text-amber-400 shrink-0" />
                            )}
                          </div>

                          <div className="shrink-0 flex items-center justify-end w-5">
                            {isSelected && (
                              <Check className="w-4 h-4 text-[#38bdf8] stroke-[2.5]" />
                            )}
                          </div>
                        </div>
                      );
                    })}
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
};
