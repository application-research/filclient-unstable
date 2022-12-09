# Extern dependency tags or branches
filecoin_ffi_version = 280c4f8b94fd46dc824a5c827dece73ec7fe3efd

all: filecoin_ffi filctl.go
.PHONY: all

filctl.go:
	go build ./cmd/filctl

extern/filecoin-ffi:
	git clone https://github.com/filecoin-project/filecoin-ffi extern/filecoin-ffi

filecoin_ffi: extern/filecoin-ffi
	cd extern/filecoin-ffi \
		&& git fetch origin $(filecoin_ffi_version) \
		&& git checkout $(filecoin_ffi_version) \
		&& make
.PHONY: filecoin_ffi

clean:
	rm -rf extern
	rm filctl
.PHONY: clean