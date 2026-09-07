.PHONY: test fmt vet lint check

test:
	go test -race -count=1 ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

lint:
	staticcheck ./...

# What CI runs.
check: vet test
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; echo "$$unformatted"; exit 1; \
	fi
