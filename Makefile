.PHONY: build build-backend build-frontend build-datamanagementd test test-backend test-frontend test-frontend-critical test-datamanagementd secret-scan

PNPM ?= npx --yes pnpm@9.15.9

FRONTEND_CRITICAL_VITEST := \
	src/views/auth/__tests__/LinuxDoCallbackView.spec.ts \
	src/views/auth/__tests__/WechatCallbackView.spec.ts \
	src/views/user/__tests__/PaymentView.spec.ts \
	src/views/user/__tests__/PaymentResultView.spec.ts \
	src/components/payment/__tests__/paymentFlow.spec.ts \
	src/components/user/profile/__tests__/ProfileInfoCard.spec.ts \
	src/views/admin/__tests__/SettingsView.spec.ts \
	src/views/admin/ops/components/__tests__/OpsAlertEventsCard.spec.ts \
	src/views/auth/__tests__/WechatPaymentCallbackView.spec.ts

# 一键编译前后端
build: build-backend build-frontend

# 编译后端（复用 backend/Makefile）
build-backend:
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
