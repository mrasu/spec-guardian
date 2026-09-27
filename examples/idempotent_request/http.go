package idempotentrequest

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// HTTPServer exposes the two modeled Actions over HTTP.
type HTTPServer struct {
	orders   *OrderServer
	delivery *DeliveryServer
}

// NewHTTPServer returns an HTTPServer with validated Action servers.
func NewHTTPServer(orders *OrderServer, delivery *DeliveryServer) *HTTPServer {
	if orders == nil || delivery == nil {
		panic("idempotentrequest: HTTPServer dependencies must not be nil")
	}
	return &HTTPServer{orders: orders, delivery: delivery}
}

// Handler returns the example's HTTP routes.
func (s *HTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders", s.handleCreateOrder)
	mux.HandleFunc("POST /orders/{order_id}/deliver", s.handleDeliver)
	return mux
}

func (s *HTTPServer) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RequestID string `json:"request_id"`
		Payload   string `json:"payload"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request")
		return
	}
	order, err := s.orders.CreateOrder(r.Context(), request.RequestID, request.Payload)
	if err != nil {
		writeActionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (s *HTTPServer) handleDeliver(w http.ResponseWriter, r *http.Request) {
	orderID, err := strconv.ParseUint(r.PathValue("order_id"), 10, 64)
	if err != nil || orderID == 0 {
		writeError(w, http.StatusBadRequest, "invalid order ID")
		return
	}
	var request struct {
		Data string `json:"data"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request")
		return
	}
	delivery, err := s.delivery.Deliver(r.Context(), orderID, request.Data)
	if err != nil {
		writeActionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, delivery)
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func writeActionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrConflict), errors.Is(err, ErrAlreadyDelivered):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrOrderNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case strings.Contains(err.Error(), "must not be empty"):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("write response: %v", err)
	}
}
