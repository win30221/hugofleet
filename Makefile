SHELL := /bin/bash
.DEFAULT_GOAL := help

.PHONY: test-oidc
test-oidc:
	@bash stages/m4/scripts/oidc.sh

.PHONY: help tools doctor fmt lint build test-unit test-race benchmark kind-up kind-down demo-image-build demo-image-load demo-deploy demo-forward demo test-e2e test-e2e-m1 test-e2e-m0 operator-tools generate manifests verify-generated envtest test-integration operator-image-build operator-install cache-native hub-bootstrap hub-up
help:
	@bash scripts/hf.sh help

.PHONY: m0-up m0-forward m0-down m1-up m1-forward m1-down m2-up m2-forward m2-down m3-up m3-down m3-hub-up m3-hub-stop m3-hub-recreate agent-install certs-dev proto-tools proto proto-check test-e2e-m3
m3-hub-recreate:
	@bash scripts/hf.sh $@

tools:
	@bash scripts/bootstrap-tools.sh

.PHONY: docs-check
doctor fmt lint build docs-check test-unit test-race benchmark kind-up kind-down demo-image-build demo-image-load demo-deploy demo-forward demo test-e2e test-e2e-m1 test-e2e-m0 operator-tools generate manifests verify-generated envtest test-integration operator-image-build operator-install cache-native hub-bootstrap hub-up m0-up m0-forward m0-down m1-up m1-forward m1-down m2-up m2-forward m2-down m3-up m3-down m3-hub-up m3-hub-stop agent-install certs-dev proto-tools proto proto-check test-e2e-m3:
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
