IMG ?= watcher:latest

.PHONY: build
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o bin/manager cmd/main.go

.PHONY: run
run:
	LOG_LEVEL=info METRICS_ADDR=:9091 go run cmd/main.go -zap-log-level=info

.PHONY: docker-build
docker-build:
	docker build -t ${IMG} .

.PHONY: docker-push
docker-push:
	docker push ${IMG}

.PHONY: deploy
deploy:
	kubectl apply -f config/crd/
	kubectl apply -f config/rbac/
	kubectl apply -f config/validation/
	kubectl apply -f config/manager/

.PHONY: undeploy
undeploy:
	kubectl delete -f config/manager/ --ignore-not-found=true
	kubectl delete -f config/rbac/ --ignore-not-found=true
	kubectl delete -f config/crd/ --ignore-not-found=true

.PHONY: install-crd
install-crd:
	kubectl apply -f config/crd/

.PHONY: uninstall-crd
uninstall-crd:
	kubectl delete -f config/crd/ --ignore-not-found=true

.PHONY: test
test:
	go test ./... -coverprofile cover.out

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...