package main

import (
	"log"

	idempotentrequest "spec-guardian/examples/idempotent-request"
)

func main() { log.Fatal(idempotentrequest.Run()) }
