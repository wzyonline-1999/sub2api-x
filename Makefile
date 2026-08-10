.PHONY: build build-backend build-frontend build-datamanagementd test test-backend test-frontend test-frontend-critical test-datamanagementd secret-scan

PNPM ?= npx --yes pnpm@9.15.9

FRONTEND_CRITICAL_VITEST := \
	src/api/__tests__/client.spec.ts \
	src/api/__tests__/tokenRefresh.spec.ts \
	src/api/__tests__/channelMonitorV2.spec.ts \
	src/views/auth/__tests__/LinuxDoCallbackView.spec.ts \
	src/views/auth/__tests__/WechatCallbackView.spec.ts \
	src/views/user/__tests__/PaymentView.spec.ts \
	src/views/user/__tests__/PaymentResultView.spec.ts \
	src/components/payment/__tests__/paymentFlow.spec.ts \
	src/components/user/profile/__tests__/ProfileInfoCard.spec.ts \
	src/views/admin/__tests__/SettingsView.spec.ts \
	src/views/admin/ops/components/__tests__/OpsAlertEventsCard.spec.ts \
	src/views/auth/__tests__/WechatPaymentCallbackView.spec.ts \
	src/views/user/__tests__/ChannelStatusView.mode.spec.ts \
	src/features/channel-monitor-v2/__tests__/designSystem.structure.spec.ts \
	src/features/channel-monitor-v2/__tests__/monitorFormat.spec.ts \
	src/features/channel-monitor-v2/__tests__/monitorZoom.spec.ts

# 一键编译可发布二进制。后端嵌入 frontend dist，因此构建顺序必须严格为前端后后端。
build: build-backend

# 编译后端（复用 backend/Makefile）。直接调用该目标也会先刷新前端产物。
build-backend: build-frontend
	@$(MAKE) -C backend build

# 编译前端（需要已安装依赖）
build-frontend:
	@$(PNPM) --dir frontend run build

# 编译 datamanagementd（宿主机数据管理进程）
build-datamanagementd:
	@cd datamanagement && go build -o datamanagementd ./cmd/datamanagementd

# 运行测试（后端 + 前端）
test: test-backend test-frontend

test-backend:
	@$(MAKE) -C backend test

test-frontend:
	@$(PNPM) --dir frontend run lint:check
	@$(PNPM) --dir frontend run typecheck
	@$(MAKE) test-frontend-critical

test-frontend-critical:
	@$(PNPM) --dir frontend run test:run -- $(FRONTEND_CRITICAL_VITEST)

test-datamanagementd:
	@cd datamanagement && go test ./...

secret-scan:
	@if git diff --cached -U0 | rg -n '^\+.*(sk-[A-Za-z0-9_-]{32,}|BEGIN (RSA |OPENSSH |EC |)PRIVATE KEY)'; then \
		echo "Potential secret found in staged diff" >&2; \
		exit 1; \
	fi
