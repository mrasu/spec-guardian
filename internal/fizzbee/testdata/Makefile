FIZZ ?= fizz
FIXTURE_DIRS := read_db read_write_db
FIZZ_FILES := $(foreach dir,$(FIXTURE_DIRS),$(wildcard $(dir)/*.fizz))

.PHONY: generate
generate:
	@set -eu; \
	for fizz_file in $(FIZZ_FILES); do \
		dir=$${fizz_file%/*}; \
		rm -rf "$$dir/fizzbee_output"; \
		$(FIZZ) --output-dir "$$dir/fizzbee_output" "$$fizz_file"; \
		rm -f "$${fizz_file%.fizz}.json"; \
		rm -f "$$dir/fizzbee_output/communication.dot" "$$dir/fizzbee_output/graph.dot"; \
	done
