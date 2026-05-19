.PHONY: build test vet setup-hooks verify-enforcement check

build:
	go build -o bin/ ./cmd/spex/

test:
	go test ./...

vet:
	go vet ./...

setup-hooks:
	git config core.hooksPath scripts/git-hooks
	chmod +x scripts/git-hooks/* scripts/hooks/*.sh scripts/hooks/log-violation scripts/hooks/test/*.sh 2>/dev/null || true
	@echo "git hooks active at scripts/git-hooks; CC hooks read by .claude/settings.json"

verify-enforcement:
	@scripts/hooks/test/run-all.sh

check: vet test verify-enforcement
