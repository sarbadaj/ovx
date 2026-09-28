SHELL := /bin/bash
.PHONY: setup start dev stop restart status logs check test smoke demo package extension test-go
setup start stop restart status logs:
	./bin/ovx $@
dev:
	./bin/ovx start
check:
	./bin/ovx doctor
test:
	./bin/ovx test
smoke:
	./bin/ovx smoke
demo:
	./bin/ovx demo
package:
	./bin/ovx package-extension
extension:
	./bin/ovx install-extension
test-go:
	cd services/runtime && ../../.ovx/tools/go/bin/go test ./...
