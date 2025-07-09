# Copyright The Linux Foundation and each contributor to LFX.
# SPDX-License-Identifier: MIT
.PHONY: apigen run debug

apigen:
	goa gen github.com/linuxfoundation/lfx-v2-project-service/design

run:
	go run .

debug:
	go run . -d
