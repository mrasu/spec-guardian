// Package idempotentrequest contains the idempotent-request example application.
package idempotentrequest

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/go-sql-driver/mysql"
)

const databaseDSN = "root@tcp(localhost:13306)/idempotent_request?parseTime=true"

// Run starts the example HTTP server and blocks until it exits.
func Run() error {
	configuration, err := mysql.ParseDSN(databaseDSN)
	if err != nil {
		return err
	}
	connector, err := mysql.NewConnector(configuration)
	if err != nil {
		return err
	}
	database := sql.OpenDB(connector)
	defer database.Close()
	dynamo := NewDynamoDBClient()
	server := NewHTTPServer(NewOrderServer(database, dynamo), NewDeliveryServer(database, dynamo))
	log.Printf("listening at %s", ":18081")
	return http.ListenAndServe(":18081", server.Handler())
}
