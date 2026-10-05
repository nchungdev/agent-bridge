# Project Conventions & Agent Guide: agent-bridge

> **Universal Agent Operating Instructions**  
> Target Platforms: Google Antigravity, Claude Code, Gemini CLI, OpenAI Codex, Cursor, Windsurf.  
> Single Source of Truth: `AGENTS.md` (all platform instruction files are symlinked to this document).

---

## 🏛️ 1. Project Overview & Architecture

* **Project Name**: `agent-bridge`
* **Purpose**: Core application repository.
* **Core Languages / Stack**: Go
* **Frameworks & Subsystems**: Standard Architecture
* **Package / Dependency Manager**: `go mod`
* **Runtime & Infrastructure**: `Docker`
* **Primary Git Branch**: `main`

### Key Directory Layout
* `internal/`: Core subsystem directory.
* `web/`: Core subsystem directory.
* `web_tmp/`: Core subsystem directory.

### 🩺 Repository Health & Stabilization Guidance
* 💡 Tạo ngay `.gitignore` phù hợp với stack công nghệ.
* 💡 Bổ sung các lệnh `npm run <cmd>` hoặc Makefile targets cho: test, lint.

---

## ⚡ 2. Standard Development Workflows

Run commands directly in the root workspace using standard tools. Always prefer existing project scripts over generic commands.

### Core Commands & Canonical Scripts
* `make all`: Run Makefile target 'all'
* `make build`: Run Makefile target 'build'
* `make build-go`: Run Makefile target 'build-go'
* `make build-web`: Run Makefile target 'build-web'
* `make clean`: Run Makefile target 'clean'
* `make docker-build`: Run Makefile target 'docker-build'
* `make run`: Run Makefile target 'run'

### Testing & Verification
* **Test Runner**: `go test`
* Always run automated tests and linters before reporting tasks as completed.
* Do not bypass, fake mock, or disable failing assertions without clear rationale.

### Formatting & Linting
* **Linters & Formatters**: `golangci-lint`
* Maintain existing code formatting and comment integrity.

---

## 🛠️ 3. Coding & Implementation Protocol

1. **Toolchain Discipline**:
   * Strictly use `go mod`. Never perform unauthorized global package installs or switch package managers without approval.
2. **Zero Scope Creep**:
   * Modify ONLY files directly relevant to the assigned task. Never perform speculative refactoring on unrelated code.
3. **Preserve Documentation & Comments**:
   * Keep existing comments, type signatures, and docstrings intact. Do not truncate code blocks.
4. **Strict Typing / No Lazy Workarounds**:
   * No untyped `any` or broad catch blocks (`except Exception: pass`) without documented justification.
5. **Verification Gate**:
   * Never declare a task complete without executing the relevant linters and test suites (`go test`) and verifying exit code 0.

---

## 🔎 4. Code Review Protocol

When tasked with reviewing code or pull requests, evaluate across 5 rigorous dimensions:
1. **Scope & Architecture**: Does the diff strictly solve the requirement without architectural drift or violation of layer boundaries?
2. **Edge Cases & Concurrency**: Check null/undefined boundaries, empty collections, timeouts, race conditions, and unclosed resources (streams, connections).
3. **Security & Sanitization**: Inspect for unvalidated inputs, SQLi, XSS, exposed tokens, and insecure defaults.
4. **Performance**: Look for $O(n^2)$ loops, N+1 database queries, and memory leaks.
5. **Test Assertiveness**: Ensure tests assert actual business logic rather than shallow mocks.

**Output Classification (Never provide generic praise)**:
* 🔴 `[Blocker]`: Critical defects, security vulnerabilities, crash hazards (must include remediation code).
* 🟡 `[Warning]`: Performance issues, missing edge-case handling, or test gaps.
* 🟢 `[Nit]`: Minor formatting, naming suggestions, or cosmetic improvements.

---

## 🛡️ 5. Safety, Secrets & Environment Rules

1. **Environment Variables**:
   * Never commit `.env` or sensitive credentials to Git. Document new configurations in `.env.example`.
2. **Secret Redaction**:
   * Mask API tokens, Bearer keys, passwords, and private hashes in transcripts and logs.
3. **Data Loss Prevention**:
   * Never execute destructive commands (`rm -rf`, `DROP TABLE`, uncommitted hard git resets) without explicit user confirmation.

---

## 🎯 6. Git & Contribution Conventions

* **Commit Message Format**: Follow Conventional Commits (`feat: ...`, `fix: ...`, `refactor: ...`, `docs: ...`, `chore: ...`).
* Keep commits atomic and focused on single logical units.
* Verify clean git status before proposing branches or PRs.

---

## 🤖 7. Agent Behavior & Communication Style

* **Concise & Direct**: Deliver clear, actionable answers. Avoid unnecessary conversational fluff.
* **Proactive Verification**: When modifying code, always verify the build or tests pass before wrapping up.
* **Modular Codebase**: Preserve separation of concerns. Do not cram business logic into entrypoints.
* **Symlink Integrity**: Do not overwrite or delete root symlinks (`CLAUDE.md`, `GEMINI.md`, `CODEX.md`, `.cursorrules`).
