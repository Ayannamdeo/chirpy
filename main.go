package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/Ayannamdeo/chirpy/internal/auth"
	"github.com/Ayannamdeo/chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

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
	data, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(500)
		w.Write(data)
		return
	}
	w.WriteHeader(status)
	w.Write(data)
}

type apiConfig struct {
	db             *database.Queries
	platform       string
	jwtSecret      string
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
  if cfg.platform != "dev" {
    respondWithError(w, http.StatusForbidden, "Only dev allowed to reset", nil)
    return
  }
  err := cfg.db.DeleteAllUsers(r.Context())
  if err != nil {
    respondWithError(w, http.StatusInternalServerError, "Error deleting users", err)
    return
  }
  respondWithJSON(w, http.StatusOK, nil)
  cfg.fileserverHits.Store(0)
}

type chirpsParam struct {
	Body string `json:"body"`
  UserId string `json:"user_id"`
  Token string `json:"token"`
}

type Chirp struct {
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
}

func (cfg *apiConfig) chirpsHandler(w http.ResponseWriter, r *http.Request) {
  if r.Method != http.MethodPost {
    respondWithError(w, http.StatusMethodNotAllowed, "method not supported", nil)
    return
  }
  reqbody := chirpsParam{}
	err := json.NewDecoder(r.Body).Decode(&reqbody)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters", err)
		return
	}
  token, err := auth.GetBearerToken(r.Header)
  if err !=  nil {
    //what the status type/code for missing parameters or headers?
    respondWithError(w, 400, "Error getting token", err)
    return
  }
  userUUID, err := auth.ValidateJWT(token, cfg.jwtSecret)
  if err != nil {
    respondWithError(w, http.StatusUnauthorized, "Invalid JWT token", err)
    return
  }
	if len(reqbody.Body) > 140 {
		respondWithError(w, http.StatusBadRequest, "Chirpy is too long", nil)
		return
	}
  user, err := cfg.db.CreateChirp(r.Context(), database.CreateChirpParams{ Body: reqbody.Body,
    UserID: userUUID,
  })
  if err != nil {
    respondWithError(w, http.StatusInternalServerError, "Couldn't create chirp", err)
    return
  }
  resUser := Chirp{
    ID: user.ID,
    CreatedAt: user.CreatedAt,
    UpdatedAt: user.UpdatedAt,
    Body: user.Body,
    UserID: user.UserID,
  }
	respondWithJSON(w, http.StatusCreated, resUser)
}

type User struct {
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	Token        string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
	ID           uuid.UUID `json:"id"`
}

func (cfg *apiConfig) usersHandler(w http.ResponseWriter, r *http.Request){
  if r.Method != http.MethodPost {
    respondWithError(w, http.StatusMethodNotAllowed, "method not supported", nil)
    return
  }
	reqBody := struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}{}
  decoder := json.NewDecoder(r.Body)
  err := decoder.Decode(&reqBody)
  if err != nil {
    respondWithError(w, 500, "Error while decoding", err)
    return
  }
  hashedPass, err := auth.HashPassword(reqBody.Password)
  user, err := cfg.db.CreateUser(r.Context(), database.CreateUserParams{
    Email: reqBody.Email,
    HashedPassword: hashedPass,
  })
  if err != nil {
    respondWithError(w, 500, "Error while creating user", err)
    return
  }
  apiUser := User{
    ID: user.ID,
    CreatedAt: user.CreatedAt,
    UpdatedAt: user.UpdatedAt,
    Email: user.Email,
  }
  respondWithJSON(w, 201, apiUser)
}


func (cfg *apiConfig) updateUsersHandler(w http.ResponseWriter, r *http.Request) {
	reqBody := struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}{}
	accessToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't get accessTokne /updateUsersHandler", err)
		return
	}
	userId, err := auth.ValidateJWT(accessToken, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate jwt", err)
		return
	}

	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error while decoding /updateUsersHandler", err)
		return
	}

	hashedPass, err := auth.HashPassword(reqBody.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't hash password", err)
		return
	}

	user, err := cfg.db.UpdateUserById(r.Context(), database.UpdateUserByIdParams{
		Email:          reqBody.Email,
		HashedPassword: hashedPass,
		ID:             userId,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "couldn't update the user by id", err)
		return
	}
  apiUser := User{
    CreatedAt: user.CreatedAt,
    UpdatedAt: user.UpdatedAt,
    Email: user.Email,
    ID: user.ID,
  }

  respondWithJSON(w, http.StatusOK, apiUser)
}

func (cfg *apiConfig) loginHandler(w http.ResponseWriter, r *http.Request){
	reqBody := struct {
	Email            string `json:"email"`
	Password         string `json:"password"`
}{}
  if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
    respondWithError(w, http.StatusInternalServerError, "Error decoding r.body", err)
    return
  }
  user, err := cfg.db.GetUserByEmail(r.Context(), reqBody.Email)
  if err != nil {
    log.Printf("Error fetching user: %v", err)
    respondWithError(w, http.StatusUnauthorized, "incorrect email or password", err)
    return
  }

  if err := auth.CheckPasswordHash(reqBody.Password, user.HashedPassword); err != nil {
    log.Printf("Password mismatch for user: %s", reqBody.Email)
    respondWithError(w, http.StatusUnauthorized, "incorrect email or password", err)
    return
  }

  accessToken, err := auth.MakeJWT(user.ID, cfg.jwtSecret, time.Hour)
  if err != nil {
    respondWithError(w, http.StatusInternalServerError, "Error generating accessToken", err)
    return
  }

  refreshToken, err := auth.MakeRefreshToken()
  if err != nil {
    respondWithError(w, http.StatusInternalServerError, "Error getting refreshToken", err)
    return 
  }

  _, err = cfg.db.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
    Token: refreshToken,
    UserID: user.ID,
    ExpiresAt: time.Now().UTC().Add(time.Hour * 24 * 60),
  })

  if err != nil {
    respondWithError(w, http.StatusInternalServerError, "Couldn't save refreshToken", err)
    return
  }

  apiUser := User{
    ID: user.ID,
    CreatedAt: user.CreatedAt,
    UpdatedAt: user.UpdatedAt,
    Email: user.Email,
    Token: accessToken,
    RefreshToken: refreshToken,
  }
  respondWithJSON(w, http.StatusOK, apiUser)
}

func (cfg *apiConfig) getAllChirpsHandler(w http.ResponseWriter, r *http.Request){
  chirpSlice, err := cfg.db.GetAllChirps(r.Context())
  if err != nil {
    respondWithError(w, 500, "Error getting all the chirps", err)
    return
  }
  res := []Chirp{}
  for _, v := range chirpSlice {
    resUser := Chirp{
      ID: v.ID,
      CreatedAt: v.CreatedAt,
      UpdatedAt: v.UpdatedAt,
      Body: v.Body,
      UserID: v.UserID,
    }
    res = append(res, resUser)
  }
  respondWithJSON(w,http.StatusOK, res)
}

func (cfg *apiConfig) refreshHandler (w http.ResponseWriter, r *http.Request){
  refreshToken, err := auth.GetBearerToken(r.Header)
  if err != nil {
    respondWithError(w, http.StatusBadRequest, "Couldn't find refreshToken", err)
    return
  }
  user, err := cfg.db.GetUserFromRefreshToken(r.Context(), refreshToken)
  if err != nil {
    respondWithError(w, http.StatusUnauthorized, "Couldn't get user for refreshToken", err)
    return
  }
  accessToken, err := auth.MakeJWT(user.ID, 
  cfg.jwtSecret, time.Hour)
  if err != nil {
    respondWithError(w, http.StatusUnauthorized, "Couldn't make jwt in /refresh", err)
    return
  }

type response struct {
		Token string `json:"token"`
	}
  respondWithJSON(w, http.StatusOK, response{
    Token: accessToken,
  })
}

func (cfg *apiConfig) revokeHandler(w http.ResponseWriter, r *http.Request){
  refreshToken, err:= auth.GetBearerToken(r.Header)
  if err != nil {
    respondWithError(w, http.StatusBadRequest, "Couldn't get refreshToken /revoke", err)
    return
  }
  _, err = cfg.db.RevokeRefreshToken(r.Context(), refreshToken)
  if err != nil {
    respondWithError(w, http.StatusInternalServerError, "couldn't revoke refreshToken", err)
    return
  }
  w.WriteHeader(http.StatusNoContent)
}

func (cfg *apiConfig) getChirpsByIdHandler(w http.ResponseWriter, r *http.Request){
  chirpIdStr := r.PathValue("chirpID")
  chirpId, err := uuid.Parse(chirpIdStr) 
      if err != nil {
          http.Error(w, "Invalid ID format", http.StatusBadRequest)
          return
      }
  chirp, err := cfg.db.GetChirpsById(r.Context(), chirpId)
  if err != nil {
    respondWithError(w, http.StatusNotFound, "Not fount Chirp", err)
    return
  }
  resChirp := Chirp{
    ID: chirp.ID,
    CreatedAt: chirp.CreatedAt,
    UpdatedAt: chirp.UpdatedAt,
    Body: chirp.Body,
    UserID: chirp.UserID,
  }
  respondWithJSON(w, http.StatusOK, resChirp)
}

func (cfg *apiConfig) deleteChirpsByIdHandler(w http.ResponseWriter, r *http.Request){
  accessToken, err := auth.GetBearerToken(r.Header)
  if err != nil {
    respondWithError(w, http.StatusUnauthorized, "Couldn't get accessToken", err)
    return
  }
  userId, err := auth.ValidateJWT(accessToken, cfg.jwtSecret)
  if err != nil {
    respondWithError(w, http.StatusUnauthorized, "Couldn't validate jwt", err)
    return
  }

  chirpIdStr := r.PathValue("chirpID")
  chirpId, err := uuid.Parse(chirpIdStr) 
      if err != nil {
          http.Error(w, "Invalid ID format", http.StatusBadRequest)
          return
      }
  chirp, err := cfg.db.GetChirpsById(r.Context(), chirpId)
  if err != nil {
    respondWithError(w, http.StatusNotFound, "Not fount Chirp", err)
    return
  }

  if chirp.UserID != userId {
    respondWithError(w, http.StatusForbidden, "user is not authorised to perform this action", err)
    return
  }

  if err := cfg.db.DeleteChirpsById(r.Context(), chirpId); err != nil {
    respondWithError(w, http.StatusInternalServerError, "error deleting chirp by id", err)
    return
  }
  w.WriteHeader(http.StatusNoContent)
}

func main() {
  godotenv.Load()
  dbURL := os.Getenv("DB_URL")
  platf := os.Getenv("PLATFORM")
  jwtS := os.Getenv("JWTSECRET")
  if dbURL == "" {
    log.Fatal("DB_URL must be set")
  }
  dbConn, err := sql.Open("postgres", dbURL)
  if err != nil {
		log.Fatalf("Error opening database: %s", err)
  }
  dbQueries := database.New(dbConn)
  apiCfg := apiConfig{
		fileserverHits: atomic.Int32{},
		db:             dbQueries,
    platform:       platf,
    jwtSecret: jwtS,
	}

	const port = "8080"
	mux := http.NewServeMux()

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app/", http.FileServer(http.Dir(".")))))

	mux.HandleFunc("GET /admin/metrics", apiCfg.metricsHandler)
	mux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)

  mux.HandleFunc("POST /api/login", apiCfg.loginHandler)
  mux.HandleFunc("POST /api/refresh", apiCfg.refreshHandler)
  mux.HandleFunc("POST /api/revoke", apiCfg.revokeHandler)

  mux.HandleFunc("POST /api/users", apiCfg.usersHandler)
  mux.HandleFunc("PUT /api/users", apiCfg.updateUsersHandler)

  mux.HandleFunc("POST /api/chirps", apiCfg.chirpsHandler)
  mux.HandleFunc("GET /api/chirps", apiCfg.getAllChirpsHandler)
  mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.getChirpsByIdHandler)
  mux.HandleFunc("DELETE /api/chirps/{chirpID}", apiCfg.deleteChirpsByIdHandler)

	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Printf("Serving on port: %s\n", port)
	log.Fatal(srv.ListenAndServe())
}
