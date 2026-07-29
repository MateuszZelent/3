all:
	@COMMIT_HASH=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown"); \
	if [ "$$COMMIT_HASH" = "unknown" ]; then \
		echo "Warning: Could not determine Git commit hash. Using 'unknown'."; \
	fi; \
	BUILD_VERSION=$$(git describe --tags --always --dirty 2>/dev/null || printf development); \
	BUILD_DATE=$$(date -u +%Y-%m-%dT%H:%M:%SZ); \
	go install -ldflags "-X main.commitHash=$$COMMIT_HASH -X main.buildVersion=$$BUILD_VERSION -X main.buildDate=$$BUILD_DATE" -v
