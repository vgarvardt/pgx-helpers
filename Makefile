verify:
	@echo "===> Checking code formatting"
	@go fmt $(go list ./... | grep -v /vendor/)
	@echo "===> Linting"
	@go list ./... | xargs -n 1 golint -set_exit_status

test:
	@CGO_ENABLED=0 go test -cover ./... -coverprofile=coverage.txt -covermode=atomic
