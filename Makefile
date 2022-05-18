# Extern dependencies commit hashes
filecoin-ffi-branch = 3f9ecec25017a871

all: extern/filecoin-ffi
.PHONY: all

extern/filecoin-ffi:
	git clone https://github.com/filecoin-project/filecoin-ffi -b $(filecoin-ffi-branch) extern/filecoin-ffi
	make -C extern/filecoin-ffi

clean:
	rm -rf extern
.PHONY: clean