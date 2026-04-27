BACKEND_DATA_DIR ?= /tmp/forgify-dev
LOG_FILE         := /tmp/forgify-dev.log
PORT             ?= 8742

# 新机器：make setup（全新 Mac 可能需要跑两次：第一次装 CLT，装完再跑一次）
# 日常开发：make testend
setup:
	@echo "━━━ Step 1/4: Xcode Command Line Tools ━━━"
	@if ! xcode-select -p > /dev/null 2>&1; then \
	  echo "→ Installing... a popup will appear. Click Install, wait for it to finish,"; \
	  echo "   then run 'make setup' again."; \
	  xcode-select --install; \
	  exit 1; \
	fi
	@echo "   ✓ Xcode CLT found"
	@echo "━━━ Step 2/4: Homebrew ━━━"
	@if ! command -v brew > /dev/null 2>&1; then \
	  echo "→ Installing Homebrew..."; \
	  /bin/bash -c "$$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"; \
	fi
	@echo "   ✓ Homebrew ready"
	@echo "━━━ Step 3/4: Go + Python3 ━━━"
	@brew bundle --no-lock
	@echo "   ✓ Go $$(go version | cut -d' ' -f3)"
	@echo "   ✓ Python $$(python3 --version)"
	@echo "━━━ Step 4/4: Go modules ━━━"
	@cd backend && go mod download
	@echo "   ✓ Modules cached"
	@echo ""
	@echo "✅ Done. Run: make testend"

testend:
	@lsof -ti :$(PORT) | xargs kill 2>/dev/null || true
	@sleep 0.3
	@cd backend && \
	  CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" \
	  go run ./cmd/server \
	    --dev \
	    --port $(PORT) \
	    --data-dir $(BACKEND_DATA_DIR) \
	    --collections-dir $(shell pwd)/testend/collections \
	    --integration-dir $(shell pwd)/testend \
	  > $(LOG_FILE) 2>&1 &
	@echo "→ Waiting for backend..."
	@while ! curl -sf http://localhost:$(PORT)/api/v1/health > /dev/null 2>&1; do sleep 0.5; done
	@echo "→ http://localhost:$(PORT)/dev/  (logs → $(LOG_FILE))"
	@open http://localhost:$(PORT)/dev/

stop:
	@lsof -ti :$(PORT) | xargs kill 2>/dev/null || true
	@echo "→ Stopped"

logs:
	@tail -f $(LOG_FILE)

clear:
	@lsof -ti :$(PORT) | xargs kill 2>/dev/null || true
	@rm -rf $(BACKEND_DATA_DIR)
	@rm -f  $(LOG_FILE)
	@echo "→ Cleared (db + attachments + log)"

.PHONY: setup testend stop logs clear
