SHELL := /bin/bash
.DEFAULT_GOAL := help

.PHONY: m4-forward-a m5-forward-a
m4-forward-a m5-forward-a:
	@bash scripts/hf.sh $@

.PHONY: m2-status m2-hub-up m3-status-a m3-status-b m3-forward-a m3-forward-b m3-agent-stop-a m3-agent-start-a m3-drift-a m3-observe-a m4-workload-status m5-workload-status m5-drift m5-observe m5-agent-stop-b m5-agent-start-b
m2-status m2-hub-up m3-status-a m3-status-b m3-forward-a m3-forward-b m3-agent-stop-a m3-agent-start-a m3-drift-a m3-observe-a m4-workload-status m5-workload-status m5-drift m5-observe m5-agent-stop-b m5-agent-start-b:
	@bash scripts/hf.sh $@

.PHONY: m0-update-good m0-scale-two m0-break m0-restore m0-status m0-observe m1-update-good m1-drift m1-break m1-restore m1-status m1-observe m1-delete
m0-update-good m0-scale-two m0-break m0-restore m0-status m0-observe m1-update-good m1-drift m1-break m1-restore m1-status m1-observe m1-delete:
	@bash scripts/hf.sh $@

.PHONY: test-oidc
test-oidc:
	@bash stages/m4/scripts/oidc.sh

.PHONY: help tools doctor fmt lint build test-unit test-race benchmark kind-up kind-down demo-image-build demo-image-load test-e2e test-e2e-m1 test-e2e-m0 operator-tools generate manifests verify-generated envtest test-integration operator-image-build operator-install cache-native
help:
	@bash scripts/hf.sh help

.PHONY: m0-up m0-forward m0-down m1-up m1-forward m1-down m2-up m2-forward m2-down m3-up m3-down m3-hub-up m3-hub-stop m3-hub-recreate agent-install certs-dev proto-tools proto proto-check test-e2e-m3
m3-hub-recreate:
	@bash scripts/hf.sh $@

tools:
	@bash scripts/bootstrap-tools.sh

.PHONY: docs-check
doctor fmt lint build docs-check test-unit test-race benchmark kind-up kind-down demo-image-build demo-image-load test-e2e test-e2e-m1 test-e2e-m0 operator-tools generate manifests verify-generated envtest test-integration operator-image-build operator-install cache-native m0-up m0-forward m0-down m1-up m1-forward m1-down m2-up m2-forward m2-down m3-up m3-down m3-hub-up m3-hub-stop agent-install certs-dev proto-tools proto proto-check test-e2e-m3:
	@bash scripts/hf.sh $@

.PHONY: m0-build m0-test m1-build m1-test m2-build m2-test m3-build m3-test
m0-build m0-test m1-build m1-test m2-build m2-test m3-build m3-test:
	@bash scripts/hf.sh $@

.PHONY: m4-up m4-down m4-hub-up m4-hub-stop m4-hub-recreate m4-status m4-forward m4-build m4-test test-e2e-m4
m4-up m4-down m4-hub-up m4-hub-stop m4-hub-recreate m4-status m4-forward m4-build m4-test test-e2e-m4:
	@bash scripts/hf.sh $@

.PHONY: m5-up m5-down m5-hub-up m5-hub-stop m5-hub-recreate m5-status m5-forward m5-build m5-test helm-tools m5-chart-test test-e2e-m5
m5-up m5-down m5-hub-up m5-hub-stop m5-hub-recreate m5-status m5-forward m5-build m5-test helm-tools m5-chart-test test-e2e-m5:
	@bash scripts/hf.sh $@
