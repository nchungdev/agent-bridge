# Agent Bridge v2 — Plan chi tiết

Mục tiêu: điều khiển Claude Code / Codex / AGY (và engine tương lai) từ web GUI như app GUI gốc: hội thoại liên tục, approve/đổi quyền thật, đổi engine giữa chừng trong cùng một conversation mà không đứt mạch.

Trạng thái gốc (NAS `duinch`, `/home/chungnh/AI Workspace/agent-bridge`, cổng 127.0.0.1:8088): Go backend + React/Vite frontend, SQLite (`sessions`, `messages`, `file_diffs`, `usage_log`), mô hình 1 tiến trình CLI mỗi tin nhắn, approve gửi `y\n` vào stdin (không hiệu quả), AGY chạy `--dangerously-skip-permissions`, Codex lỗi cờ `--sandbox` + `--approve-for-me`. NAS: 15 GB RAM (~10 GB available), Go không có trong PATH của ssh non-interactive.

## Kiến trúc đích

```
Web GUI ─WS─▶ transport ─▶ app/ ConversationService ─▶ SessionManager ─▶ Engine adapters
                              │  PolicyEngine           (pool, idle,       claude | codex | agy | pty
                              │  ContextStore            resume)               │
                              └──────── SQLite ◀── events ◀── Translator ◀── Transport (stdio NDJSON | JSON-RPC | PTY)
```

- `core/`: `Engine`, `Session`, `Event`, `Capabilities` (chỉ stdlib, không import adapters).
- `app/`: `ConversationService`, `SessionManager`, `PolicyEngine`, `ContextStore`.
- `adapters/`: mỗi engine một thư mục; dịch output riêng → event chuẩn (anti-corruption layer).
- `runner/`: cách spawn (host user riêng → bwrap/container sau).
- Conversation (hiện ở left panel) 1—n `engine_bindings`; chỉ 1 engine active tại một thời điểm.

## Quyết định thiết kế (đã chốt trong thảo luận)

1. Process dài hạn mỗi (conversation, engine) binding; hub là nguồn sự thật của lịch sử.
2. Approve thật qua giao thức của CLI, không đoán text.
3. Quyền (mode, allowlist, workspace) thuộc về conversation, PolicyEngine dịch sang cờ từng engine.
4. Shared ContextStore: event log append-only + working state + artifacts; đưa vào engine mới bằng push (prompt nền) và pull (MCP) .
5. Mặc định không full quyền; bypass chỉ theo phiên, do người dùng bật, tự tắt.

## Phase 0 — Hotfix & nền an toàn (0.5 ngày)

| # | Việc | Chi tiết | Xong khi |
|---|------|----------|----------|
| 0.1 | Sửa Codex | `agents/codex.go`: bỏ `--approve-for-me` (dùng `--sandbox workspace-write` + `--ask-for-approval` phù hợp hoặc `--full-auto`) | gửi prompt Codex từ GUI trả lời được |
| 0.2 | Bỏ bypass của AGY | gỡ `--dangerously-skip-permissions`; tạm dùng `--mode accept-edits` | AGY không còn tự duyệt mọi tool |
| 0.3 | Claude resume tạm | thêm `--model`, `--session-id/--resume` cho lượt `-p` | hội thoại Claude giữ ngữ cảnh giữa các lượt |
| 0.4 | Backup + test | backup binary theo kiểu `agent-bridge.backup-YYYYMMDD-*`; Go test + smoke WS | service restart sạch |

## Phase 1 — Core domain + ContextStore (event log) (2–3 ngày)

Không chạm hub đang chạy (package mới, chạy song song).

| # | Việc | Chi tiết |
|---|------|----------|
| 1.1 | `core/` types | `Event` (text_delta, tool_call, tool_result, approval_request, diff, usage, error{kind}, turn_done, engine_switch), `Capabilities`, `Engine`/`Session` interface, `Decision` |
| 1.2 | Migration SQLite | bảng mới: `conversations` (map từ `sessions`), `engine_bindings(conv_id, engine, engine_session_id, state, last_synced_event, updated)`, `events(id, conv_id, seq, engine, model, type, payload_json, ts)`, `approvals(id, conv_id, engine, tool, args_json, risk, decision, decided_by, ts)`, `working_state(conv_id, json, rev, ts)`; migrate dữ liệu `messages` cũ → `events` (giữ bảng cũ để rollback) |
| 1.3 | `ContextStore` | `Append`, `Tail(conv,n)`, `Since(conv, seq)`, `State/SetState`, `Search`; lọc bí mật (regex token/key) trước khi ghi |
| 1.4 | Working state theo luật | cập nhật không cần LLM: engine active, mode, file đã sửa (từ diff), approval đã duyệt, lỗi gần nhất, git checkpoint |
| 1.5 | State machine | `created→starting→idle⇄running⇄awaiting_approval→suspended→resuming→failed/stopped`, một nơi duy nhất chuyển trạng thái, phát event |
| 1.6 | Contract test + FakeEngine | CLI giả phát event mẫu; bộ test chung mọi adapter phải qua |

Xong khi: unit test pass, migration chạy được trên bản sao DB thật, không đổi hành vi hub hiện tại.

## Phase 2 — SessionManager (2 ngày)

| # | Việc | Chi tiết |
|---|------|----------|
| 2.1 | Pool giới hạn | max N process sống (mặc định 4, cấu hình), LRU suspend khi đầy |
| 2.2 | Idle timeout | suspend sau M phút (mặc định 20), lần sau resume bằng engine_session_id |
| 2.3 | Hàng đợi theo phiên | tin nhắn đến khi đang `running`: xếp hàng / cancel turn (người dùng chọn) |
| 2.4 | Phục hồi | khởi động: mọi binding running/idle → suspended; watchdog phát hiện process chết → failed + cho resume |
| 2.5 | cancel vs stop | cancel = ngắt turn, giữ process; stop = kết thúc binding; SIGTERM → chờ → SIGKILL process group |
| 2.6 | Chạy thử với FakeEngine | test đồng thời, restart, race (`go test -race`) |

## Phase 3 — Adapter Claude + Approve thật (3 ngày)

1. Chạy `claude -p --input-format stream-json --output-format stream-json --verbose --permission-mode <m> --permission-prompts <…> --permission-prompt-tool mcp__hub__approve --mcp-config <hub.json> --session-id/--resume`. (Cờ chính xác xác nhận bằng spike 3.0.)
2. **3.0 Spike (0.5 ngày)**: thử tay trên NAS: gửi 2 lượt qua stdin, kích hoạt tool cần quyền, xác nhận kênh approve và định dạng sự kiện.
3. Hub tự mở một MCP server nội bộ (`approve` tool); khi Claude gọi, hub tạo `approval_request`, chờ `Decide`, trả allow/deny (timeout mặc định deny).
4. Translator: stream-json → Event chuẩn (text_delta, tool_call/result, diff, usage).
5. PolicyEngine v1: chain allowlist (Read/Grep/Glob, lệnh đọc, `auditctl verify`) → luật theo mode (plan / ask / accept-edits / bypass-theo-phiên) → hỏi GUI.
6. Ghi `approvals` + audit log.

Xong khi: từ web GUI gửi 3 lượt liên tục trong 1 process, bấm Approve/Deny một lệnh ghi file và Claude phản ứng đúng; Deny thật sự chặn; đổi mode giữa phiên có hiệu lực.

## Phase 4 — Frontend (song song Phase 3, 3 ngày)

React/Vite (TypeScript) hiện có:
- Left panel: conversation, badge engine active, trạng thái (idle/running/awaiting).
- Thanh đầu chat: chọn engine/model (từ `Capabilities`/`Models()`), chọn permission mode, nút "Full quyền phiên này" (cảnh báo, tự tắt).
- Thẻ approval: tool, args, diff/preview, risk, nút Allow once / Allow for session / Deny; khôi phục khi reload.
- Dòng thời gian gắn nhãn engine ("Claude → Codex"), banner chuyển engine.
- Chỉ hiện tính năng engine hỗ trợ (theo `Capabilities`).
- Hủy lượt / dừng phiên; nhiều tab cùng xem, 1 client điều khiển.

## Phase 5 — Chuyển engine + handoff (2 ngày)

1. API `switch_engine(conv, to)`: chỉ khi binding hiện tại `idle` (hoặc sau cancel).
2. Tạo git checkpoint/commit trong workspace trước khi chuyển (nếu workspace là repo; không tự commit lên repo người dùng ngoài nhánh/stash quy ước).
3. Handoff push: working state + đuôi event (từ `last_synced_event` của engine đích) → tin nhắn nền cho engine mới. Replay tùy chọn.
4. Engine cũ quay lại: resume binding cũ + chỉ nạp phần chênh lệch.
5. Failover quota (thay logic hiện tại): tự chuyển + handoff + event `engine_switch`.
6. Cảnh báo capability mismatch (vd plan mode) trước khi chuyển.

## Phase 6 — Adapter Codex (app-server) & AGY (3–4 ngày)

- **6.0 Spike**: `codex app-server` giao thức JSON-RPC (thread, turn, approval), xác nhận trên 0.160.0 (experimental). Fallback: `codex exec resume` + `--ask-for-approval` nếu app-server không ổn.
- **Codex adapter**: JSON-RPC client, ánh xạ approval/diff/usage → event chuẩn; 1 daemon dùng chung nhiều thread (cân nhắc, tiết kiệm RAM).
- **6.1 Spike AGY**: bỏ bypass, xem `permission_request` trong stream-json và cách trả lời. Nếu không trả lời được → đánh dấu `PermissionPrompts=false`, ép mode `plan`/`accept-edits`, hoặc chạy qua PTY adapter.
- **AGY adapter**: `--input-format stream-json --output-format stream-json`, `--conversation`, `--mode`.
- **PTY adapter** dự phòng cho engine không có giao thức (approve bằng regex, mặc định chỉ đọc).
- Models lấy từ CLI (`agy models`…), bỏ hard-code.

## Phase 7 — Cô lập & bảo mật (2 ngày, bắt buộc trước khi mở ra ngoài localhost)

1. `Runner`: chạy CLI bằng user riêng không sudo; mount/permission read-only cho thư viện media, ghi chỉ trong workspace được chỉ định; không để `~/.ssh`, `*.env` token trong tầm.
2. Auth ở tầng hub (token/session) + CSRF/Origin check cho WebSocket; rà `terminal_handler.go` (web shell) — tắt mặc định hoặc đặt sau xác thực + quyền admin.
3. Redaction bí mật khi ghi `events` và log.
4. Giới hạn tài nguyên (cgroup/ulimit) mỗi process; quota số phiên.
5. Bỏ hard-code `/home/chungnh/AI Workspace` làm workdir mặc định → cấu hình theo conversation.

## Phase 8 — Context nâng cao (sau, tùy chọn)

- LLM chắt lọc `working_state` (mục tiêu, kế hoạch, quyết định + lý do, câu hỏi mở) bằng model rẻ, chỉ chạy khi có thay đổi đáng kể.
- MCP server `hub-context` (`get_state`, `search_history`, `get_decisions`) cấp cho mọi engine (pull).
- Gắn `AGENTS.md`/`PROJECT_MEMORY.md` làm project knowledge dùng chung.
- Usage thật theo nhà cung cấp (khi có nguồn xác thực).

## Thứ tự & mốc

| Mốc | Nội dung | Ước lượng |
|-----|----------|-----------|
| M0 | Phase 0 hotfix | 0.5 ngày |
| M1 | Phase 1–2 (core, store, session manager) với FakeEngine | ~5 ngày |
| M2 | Phase 3–4 (Claude + approve thật + UI) | ~4–5 ngày |
| M3 | Phase 5 (đổi engine + handoff) | 2 ngày |
| M4 | Phase 6 (Codex, AGY) | 3–4 ngày |
| M5 | Phase 7 (cô lập, auth) | 2 ngày |
| Sau | Phase 8 | tùy |

M2 là mốc có giá trị lớn nhất cho người dùng; M5 bắt buộc trước khi mở hub ra ngoài.

## Chiến lược chuyển đổi & rollback

- Code mới trong package mới (`core/ app/ adapters/`), bật bằng feature flag `AGENT_BRIDGE_V2=1`; luồng cũ (`Dispatcher`, 1-process-per-turn) giữ làm dự phòng đến hết M4.
- Migration chỉ thêm bảng/cột; không xóa `messages`/`sessions`. Backup `~/.agent-bridge/*.db` trước mỗi migration.
- Mỗi lần deploy: backup binary + `go build` + `go test` + restart `agent-bridge.service` + smoke test WS (theo cách đã làm ở các lần trước).
- Ghi tiến độ vào `PROJECT_MEMORY.md` theo AGENTS.md; không sửa file đang `[IN_PROGRESS]` của instance khác.

## Rủi ro chính

| Rủi ro | Giảm thiểu |
|--------|-----------|
| Giao thức approve của CLI khác tài liệu / thay đổi theo phiên bản | Spike 3.0, 6.0, 6.1 trước khi viết adapter; ghim phiên bản CLI, contract test |
| `codex app-server` còn experimental | Fallback `exec resume`; adapter ẩn sau interface |
| AGY không cho trả lời approve | `Capabilities.PermissionPrompts=false` + mode giới hạn / PTY |
| RAM NAS khi nhiều process dài hạn | Pool + LRU + idle timeout; theo dõi thực tế (hiện ~10 GB available) |
| Tóm tắt/handoff sai lan sang engine khác | Event log gốc luôn giữ; working state chỉ là chỉ mục; replay tùy chọn |
| Hai engine cùng sửa workspace | Chỉ một active; checkpoint git khi chuyển; hàng đợi theo phiên |
| Rò bí mật qua log/store | Redaction, user cô lập, auth, tắt web terminal mặc định |
| Go không có trong PATH khi ssh non-interactive | Dùng đường dẫn Go đầy đủ đã dùng ở lần deploy trước, hoặc build trong session login |

## Việc cần người dùng quyết định

1. Handoff mặc định: summary hay replay (đề xuất: summary + tham chiếu log, replay tùy chọn).
2. Số process sống tối đa và idle timeout mặc định (đề xuất 4 / 20 phút).
3. Có mở hub ra ngoài mạng nội bộ không (quyết định mức ưu tiên Phase 7).
4. Có cho phép chế độ "Full quyền phiên này" hay bỏ hẳn (đề xuất: cho phép, có cảnh báo, tự tắt).
