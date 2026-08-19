BINARY := uplift
CMD    := ./cmd/uplift

.PHONY: build clean test

build:
	go build -o $(BINARY) $(CMD)
ifeq ($(shell uname -s),Darwin)
	@xattr -d com.apple.quarantine $(BINARY) 2>/dev/null || true
	@echo "Built $(BINARY) (quarantine attribute cleared)"
else
	@echo "Built $(BINARY)"
endif

test:
	go test ./...

clean:
	rm -f $(BINARY)