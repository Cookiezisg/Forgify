.PHONY: install dev build build-go build-fe build-el clean

# 安装所有依赖
install:
	npm install
	npm --prefix frontend install

# 开发模式：编译 Go 后端 + 启动 Vite + Electron
dev: build-go
	npm run dev

# 完整生产构建
build: build-go build-fe build-el

# 仅编译 Go 后端
build-go:
	cd backend && go build -o ../dist-electron/forgify-backend .

# 仅构建前端
build-fe:
	npm --prefix frontend run build

# 仅编译 Electron TS
build-el:
	npx tsc -p electron/tsconfig.json

# 清理构建产物
clean:
	rm -rf dist-electron/main.js dist-electron/preload.js dist-electron/forgify-backend
	rm -rf frontend/dist
