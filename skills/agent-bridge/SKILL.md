---
name: agent-bridge
description: "Universal Agent Bridge client: Bàn giao ca (Context Handoff) và đồng bộ trạng thái giữa Google Antigravity, Claude Code, và OpenAI Codex qua Agent Bridge Daemon (localhost:8088)."
---

# Agent Bridge Universal Skill

Kỹ năng này kết nối Agent hiện tại với **Agent Bridge Daemon** chạy nền trên máy tại `http://localhost:8088`.

## 1. Khi Người Dùng Yêu Cầu Switch Agent hoặc Bàn Giao Ca (Handoff)
Ví dụ các câu lệnh của người dùng:
- `"Chuyển sang claude code"` / `"switch to claude"`
- `"Hết quota rồi, chuyển tiếp sang agy"`
- `"Bàn giao ca cho codex"`

### Các bước thực hiện tự động:
1. Xác định thư mục dự án hiện tại (`cwd`).
2. Tóm tắt ngắn gọn:
   - **Task Goal**: Mục tiêu bạn đang thực hiện.
   - **Trạng thái & File đã sửa**: Những file vừa được chỉnh sửa, các bước đã hoàn tất.
   - **Next Steps**: Bước tiếp theo mà Agent nhận ca cần làm ngay.
3. Gửi lệnh Handoff về Daemon bằng `curl`:
```bash
curl -s -X POST http://localhost:8088/api/bridge/handoff \
  -H "Content-Type: application/json" \
  -d '{
    "workspace_path": "'"$PWD"'",
    "from_agent": "agy",
    "to_agent": "claude",
    "task_goal": "<TÓM_TẮT_MỤC_TIÊU>",
    "extra_context": "<CÁC_BƯỚC_TIẾP_THEO>"
  }'
```
4. Daemon sẽ tự động ghi nội dung vào file local `.agent/handoff.md` trong project folder và cập nhật checkpoint vào SQLite database.
5. Báo lại cho người dùng:
   > *"✅ Đã lưu checkpoint bàn giao ca vào `.agent/handoff.md` và đồng bộ lên Agent Bridge. Bạn có thể mở Claude Code (hoặc bấm Switch trên Dashboard http://localhost:8088) để tiếp tục ngay lập tức!"*

---

## 2. Khi Nhận Ca Mới (Resume / Tiếp Quản Dự Án)
Khi bắt đầu một phiên làm việc mới trong workspace:
1. Kiểm tra xem file `.agent/handoff.md` có tồn tại trong thư mục hiện tại không:
```bash
cat .agent/handoff.md 2>/dev/null
```
2. Nếu có, đọc nội dung và chủ động chào người dùng:
   > *"Tôi đã nắm được bối cảnh bàn giao từ Agent trước: Task đang làm là `<Task Goal>`. Tôi sẽ tiếp tục từ bước `<Next Steps>` nhé?"*
