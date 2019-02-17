verify:
	@echo "===> Checking code formatting"
	@go fmt $(go list ./... | grep -v /vendor/)
#	@echo "===> Checking imports"
#	@goimports -l $(go list -f '{{.Dir}}' ./... | grep -v /vendor/)
	@echo "===> Linting"
	@go list ./... | xargs -n 1 golint -set_exit_status
