package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"example.com/social-commerce/internal/checkout"
	"example.com/social-commerce/internal/infrai"
)

func main() {
	client := infrai.NewClient(os.Getenv("INFRAI_API_KEY"))
	workflow := checkout.Workflow{Captcha: client, Now: time.Now}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /checkout", func(w http.ResponseWriter, r *http.Request) {
		var input checkout.Checkout
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			http.Error(w, "invalid checkout JSON", http.StatusBadRequest)
			return
		}
		result, err := workflow.Place(r.Context(), input)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	address := ":8080"
	log.Printf("storefront listening on %s", address)
	log.Fatal(http.ListenAndServe(address, mux))
}
