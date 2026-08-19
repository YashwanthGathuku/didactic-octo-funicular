# SentinelFlow Platform Makefile

.PHONY: help demo test test-race lint build nacha up down clean

help:
	@echo "SentinelFlow Engineering Targets:"
	@echo "  make demo        - Execute full end-to-end demo"
	@echo "  make test        - Run complete Go multi-package test suite"
	@echo "  make test-race   - Run Go test suite with -race detector"
	@echo "  make lint        - Run Go format verification & frontend typecheck"
	@echo "  make build       - Build Gateway binary & Frontend bundle"
	@echo "  make nacha       - Generate synthetic NACHA PPD test file"
	@echo "  make up          - Start Docker Compose stack"
	@echo "  make down        - Stop Docker Compose stack"

demo:
	bash scripts/demo.sh

test:
	cd gateway && go test -count=1 ./...

test-race:
	cd gateway && go test -race -count=1 ./...

lint:
	cd gateway && gofmt -s -l .
	npx tsc --noEmit

build:
	cd gateway && go build -o bin/gateway .
	npm run build

nacha:
	python scripts/generate_nacha.py --entries 20 --amount-cents 2500000 --output sample_payroll.ach

up:
	docker compose up -d --build

down:
	docker compose down

clean:
	rm -f sample_payroll.ach demo_payroll_batch.ach gateway/bin/gateway
