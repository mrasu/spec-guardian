FIZZ ?= fizz

.PHONY: generate
generate:
	rm -rf cache_aside_fizzbee_output
	$(FIZZ) --output-dir cache_aside_fizzbee_output Generate.fizz
	rm -f Generate.json
	rm -f cache_aside_fizzbee_output/communication.dot cache_aside_fizzbee_output/graph.dot
	rm -rf idempotent_request_fizzbee_output
	$(FIZZ) --output-dir idempotent_request_fizzbee_output IdempotentRequest.fizz
	rm -f IdempotentRequest.json
	rm -f idempotent_request_fizzbee_output/communication.dot idempotent_request_fizzbee_output/graph.dot
