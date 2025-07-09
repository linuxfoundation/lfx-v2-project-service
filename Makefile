.PHONY: apigen run

apigen:
	goa gen github.com/linuxfoundation/lfx-v2-project-service/design

run:
	go run .

debug:
	go run . -d
