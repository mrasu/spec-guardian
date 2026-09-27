OUTPUT := docs/spec/fizzbee_output

run:
	go run ./cmd/cache-aside

test:
	go test ./...

test-conformance:
	go test -tags=specguardian ./conformance/...

generate: generate-fizz-output generate-conformance-spec

generate-fizz-output:
	rm -rf $(OUTPUT)
	fizz --output-dir $(OUTPUT) docs/spec/CacheAside.fizz
	rm -f docs/spec/CacheAside.json $(OUTPUT)/communication.dot $(OUTPUT)/graph.dot

generate-conformance-spec:
	go run github.com/mrasu/spec-guardian generate --project-dir . --fizz-output-dir $(OUTPUT) --dev
