package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"unicode"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "text/html; charset=utf-8")
	htmlRes := `
  <html>
    <body>
      <h1>Welcome, Chirpy Admin</h1>
      <p>Chirpy has been visited %d times!</p>
    </body>
  </html>
  `
	fmt.Fprintf(w, htmlRes, cfg.fileserverHits.Load())
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {
	cfg.fileserverHits.Store(0)
}

type reqParameters struct {
	Body string `json:"body"`
}
type validResParameters struct {
	Cleaned_Body string `json:"cleaned_body"`
}

//	func writeJSON(w http.ResponseWriter, status int, v any) error {
//		data, err := json.Marshal(v)
//		if err != nil {
//			return err
//		}
//		w.Header().Set("Content-Type", "application/json")
//		w.WriteHeader(status)
//		_, err = w.Write(data)
//		return err
//	}
func respondWithError(w http.ResponseWriter, status int, msg string, err error) {
	if err != nil {
		log.Println(err)
	}
	if status >= 500 {
		log.Printf("Responding with 5XX error: %s", msg)
	}
	type errorResponse struct {
		Error string `json:"error"`
	}
	respondWithJSON(w, status, errorResponse{
		Error: msg,
	})
}

func respondWithJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	log.Println(payload)
	data, err := json.Marshal(payload)
	log.Printf("\nmarshal: %v\n", data)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		w.Write(data)
		return
	}
	w.WriteHeader(status)
	w.Write(data)
}

func cleanWord(word string) string {
	var result []rune
	for _, char := range word {
		if unicode.IsLetter(char) {
			result = append(result, char)
		}
	}
	return string(result)
}

func validateHandler(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	reqbody := reqParameters{}
	err := decoder.Decode(&reqbody)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters", err)
		return
	}
	if len(reqbody.Body) > 140 {
		respondWithError(w, http.StatusBadRequest, "Chirpy is too long", nil)
		return
	}
	// splitted := strings.Split(strings.ToLower(reqbody.Body), " ")
	splitted := strings.Split(reqbody.Body, " ")
	for i, s := range splitted {
		cleanword := cleanWord(strings.ToLower(s))
		if cleanword == "kerfuffle" || cleanword == "sharbert" || cleanword == "fornax" {
			splitted[i] = "****"
		}
	}

	resBody := validResParameters{
		Cleaned_Body: strings.Join(splitted, " "),
	}
	log.Println(resBody.Cleaned_Body)
	respondWithJSON(w, http.StatusOK, resBody)
}

func main() {
	const port = "8080"
	mux := http.NewServeMux()
	apiCfg := apiConfig{}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app/", http.FileServer(http.Dir(".")))))
	mux.HandleFunc("GET /admin/metrics", apiCfg.metricsHandler)
	mux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)
	mux.HandleFunc("POST /api/validate_chirp", validateHandler)
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Printf("Serving on port: %s\n", port)
	log.Fatal(srv.ListenAndServe())
}
