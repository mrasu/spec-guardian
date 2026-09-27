test:
	go test ./...

TESTDATA_MAKEFILES := $(shell find internal guardian -type f -path '*/testdata/Makefile' -print | sort)
TESTDATA_DIRS := $(patsubst %/Makefile,%,$(TESTDATA_MAKEFILES))

generate-testdata:
	@set -eu; \
	for dir in $(TESTDATA_DIRS); do \
		$(MAKE) -C "$$dir" generate; \
	done

generate-proto:
	rm -f internal/fizzbee/pb/* && \
	mkdir -p internal/fizzbee/pb && \
	protoc \
	  -I internal/fizzbee/proto \
	  --go_out=internal/fizzbee/pb \
	  --go_opt=paths=source_relative \
	  internal/fizzbee/proto/*.proto
