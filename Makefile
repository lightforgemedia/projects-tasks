.PHONY: build test install clean

BINARY := pt
BUILD_DIR := ./cmd/pt
INSTALL_DIR := $(HOME)/go/bin

build:
	go build -o $(BINARY) $(BUILD_DIR)

test:
	go test ./...

install: build
	mkdir -p $(INSTALL_DIR)
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)

clean:
	rm -f $(BINARY)
