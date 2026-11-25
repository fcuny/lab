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

# bootc dev VM on GCE. See bootc-vm/README.md.
.PHONY: bootc-vm-create bootc-vm-provision bootc-vm-start bootc-vm-stop bootc-vm-ssh bootc-vm-status bootc-vm-destroy
bootc-vm-create: ## create the bootc dev VM if missing, start it, and provision it
	bootc-vm/vm.sh create

bootc-vm-provision: ## re-run the guest setup on the running bootc dev VM
	bootc-vm/vm.sh provision

bootc-vm-start: ## start the bootc dev VM and wait for SSH
	bootc-vm/vm.sh start

bootc-vm-stop: ## stop the bootc dev VM; its disk and caches are kept
	bootc-vm/vm.sh stop

bootc-vm-ssh: ## open a shell on the bootc dev VM (OS Login, through IAP)
	bootc-vm/vm.sh ssh

bootc-vm-status: ## print the bootc dev VM's status
	@bootc-vm/vm.sh status

bootc-vm-destroy: ## delete the bootc dev VM and its disk (asks first)
	bootc-vm/vm.sh destroy
