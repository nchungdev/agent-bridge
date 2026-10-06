import React, { useEffect, useState } from "react";

export const AppearanceTab: React.FC = () => {
  const [fontSize, setFontSize] = useState("12.5");
  const [fontFamily, setFontFamily] = useState(
    'ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace'
  );

  useEffect(() => {
    const fs = localStorage.getItem("bridge_term_fontsize") || "12.5";
    const ff =
      localStorage.getItem("bridge_term_fontfamily") ||
      'ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace';
    setFontSize(fs);
    setFontFamily(ff);
  }, []);

  const handleFontSizeChange = (val: string) => {
    setFontSize(val);
    localStorage.setItem("bridge_term_fontsize", val);
    window.dispatchEvent(new Event("bridge_term_font_changed"));
  };

  const handleFontFamilyChange = (val: string) => {
    setFontFamily(val);
    localStorage.setItem("bridge_term_fontfamily", val);
    window.dispatchEvent(new Event("bridge_term_font_changed"));
  };

  return (
    <div className="space-y-6 max-w-2xl">
      <div>
        <h1 className="text-xl font-bold text-slate-100">Appearance</h1>
        <p className="text-xs text-slate-400 mt-1">
          Tuỳ chỉnh hiển thị giao diện, kích thước và font chữ terminal.
        </p>
      </div>

      <div className="space-y-4">
        <div className="rounded-xl border border-[#202738] bg-[#131722] p-4 space-y-4">
          <div className="flex items-center justify-between gap-4">
            <div>
              <div className="text-xs font-medium text-slate-200">Cỡ chữ Terminal</div>
              <div className="text-[11.5px] text-slate-400 mt-0.5">
                Font size hiển thị cho các cửa sổ terminal console.
              </div>
            </div>
            <select
              value={fontSize}
              onChange={(e) => handleFontSizeChange(e.target.value)}
              className="rounded-lg border border-[#2c3549] bg-[#1a202e] px-3 py-1.5 text-xs text-slate-200 focus:outline-none focus:border-indigo-500 cursor-pointer"
            >
              <option value="11">11 px</option>
              <option value="12">12 px</option>
              <option value="12.5">12.5 px (Default)</option>
              <option value="13">13 px</option>
              <option value="14">14 px</option>
              <option value="15">15 px</option>
              <option value="16">16 px</option>
            </select>
          </div>

          <div className="border-t border-[#1c2230] pt-3 flex items-center justify-between gap-4">
            <div>
              <div className="text-xs font-medium text-slate-200">Font chữ Terminal</div>
              <div className="text-[11.5px] text-slate-400 mt-0.5">
                Font đơn khoảng (Monospace) sử dụng cho xterm.
              </div>
            </div>
            <select
              value={fontFamily}
              onChange={(e) => handleFontFamilyChange(e.target.value)}
              className="rounded-lg border border-[#2c3549] bg-[#1a202e] px-3 py-1.5 text-xs text-slate-200 focus:outline-none focus:border-indigo-500 cursor-pointer font-mono"
            >
              <option value='ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace'>
                System Default
              </option>
              <option value='"JetBrains Mono", ui-monospace, monospace'>JetBrains Mono</option>
              <option value='"Fira Code", ui-monospace, monospace'>Fira Code</option>
              <option value='"Cascadia Code", Consolas, monospace'>Cascadia Code</option>
            </select>
          </div>
        </div>
      </div>
    </div>
  );
};
