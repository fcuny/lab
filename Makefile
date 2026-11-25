.PHONY: help
help: ## list targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "%-15s %s\n", $$1, $$2}'

.PHONY: lint
lint: ## run all pre-commit hooks on every file, and scan all commits for secrets
	pre-commit run --all-files --show-diff-on-failure
	pre-commit run --all-files --hook-stage manual gitleaks-history

.PHONY: install-hooks
install-hooks: ## install pre-commit hooks (run once after cloning)
	pre-commit install
