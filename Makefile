.PHONY: help
help: ## list targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "%-18s %s\n", $$1, $$2}'

.PHONY: lint
lint: ## run all pre-commit hooks on every file, and scan all commits for secrets
	pre-commit run --all-files --show-diff-on-failure
	pre-commit run --all-files --hook-stage manual gitleaks-history

.PHONY: install-hooks
install-hooks: ## install pre-commit hooks (run once after cloning)
	pre-commit install

# Pinned so local runs and CI agree; bump deliberately.
STATICCHECK := honnef.co/go/tools/cmd/staticcheck@v0.8.2
LAB := go run ./cmd/lab

.PHONY: check
check: ## build, vet, staticcheck and test the Go code
	go build ./...
	go vet ./...
	go run $(STATICCHECK) ./...
	go test ./...

# bootc dev VM on GCE. See bootc-vm/README.md.
.PHONY: bootc-vm-create bootc-vm-provision bootc-vm-start bootc-vm-stop bootc-vm-ssh bootc-vm-status bootc-vm-destroy
bootc-vm-create: ## create the bootc dev VM if missing, start it, and provision it
	$(LAB) bootc-vm create

bootc-vm-provision: ## re-run the guest setup on the running bootc dev VM
	$(LAB) bootc-vm provision

bootc-vm-start: ## start the bootc dev VM and wait for SSH
	$(LAB) bootc-vm start

bootc-vm-stop: ## stop the bootc dev VM; its disk and caches are kept
	$(LAB) bootc-vm stop

bootc-vm-ssh: ## open a shell on the bootc dev VM (OS Login, through IAP)
	$(LAB) bootc-vm ssh

bootc-vm-status: ## print the bootc dev VM's status
	@$(LAB) bootc-vm status

bootc-vm-destroy: ## delete the bootc dev VM and its disk (asks first)
	$(LAB) bootc-vm destroy
