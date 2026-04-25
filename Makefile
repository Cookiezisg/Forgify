BACKEND_DATA_DIR ?= /tmp/forgify-dev
PORT             ?= 8742

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
	    --integration-dir $(shell pwd)/testend &
	@echo "→ Waiting for backend..."
	@while ! curl -sf http://localhost:$(PORT)/api/v1/health > /dev/null 2>&1; do sleep 0.5; done
	@echo "→ Opening http://localhost:$(PORT)/dev/"
	@open http://localhost:$(PORT)/dev/

stop:
	@lsof -ti :$(PORT) | xargs kill 2>/dev/null || true
	@echo "→ Backend stopped"

.PHONY: testend stop
