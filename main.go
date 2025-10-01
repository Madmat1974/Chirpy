package main

import ( 
    "database/sql"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "strings"
	"sync/atomic"
    "time"
	

    "github.com/joho/godotenv"
    _ "github.com/lib/pq"

    "github.com/google/uuid"
    "github.com/Madmat1974/Chirpy.git/internal/database"
)


func main() {
	
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	fmt.Println("DB_URL:", dbURL)
	db, err := sql.Open("postgres", dbURL)
	if err != nil { panic(err) }
	if err := db.Ping(); err != nil { panic(err) }
	defer db.Close()
	queries := database.New(db)

	cfg := apiConfig{
    db:       queries,
    platform: os.Getenv("PLATFORM"),
}
	const filepathRoot = "."
	mux := http.NewServeMux()

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	fs := http.FileServer(http.Dir(filepathRoot))
	handler := http.StripPrefix("/app", fs)
	mux.HandleFunc("GET /api/healthz", healthHandler)
	mux.Handle("/app/", cfg.middlewareMetricsInc(handler))
	mux.HandleFunc("/app", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/app/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /admin/metrics", cfg.handlerMetrics)
	mux.HandleFunc("POST /admin/reset", cfg.handlerReset)
	mux.HandleFunc("POST /api/chirps", cfg.handlerChirps)
	mux.HandleFunc("POST /api/users", cfg.userCreate)
	mux.HandleFunc("GET /api/chirps", cfg.retrieveChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", cfg.retrieveChirp)
	fmt.Printf("Starting server on %s", server.Addr)
	err = server.ListenAndServe()
	if err != nil {
		fmt.Printf("Server failed to start: %v", err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

type apiConfig struct {
	fileserverHits 	atomic.Int32
	db 		*database.Queries
	platform string
}

type userCreateParams struct {
	Email string `json:"email"`
}

type User struct {
	ID 			uuid.UUID `json:"id"`
	CreatedAt	time.Time `json:"created_at"`
	UpdatedAt	time.Time `json:"updated_at"`
	Email		string	  `json:"email"`
}

func (cfg *apiConfig) userCreate(w http.ResponseWriter, r *http.Request) {
	var params userCreateParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(params.Email) == "" {
		respondWithError(w, http.StatusBadRequest, "email required")
		return
	}

	dbUser, err := cfg.db.CreateUser(r.Context(), params.Email)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not create user")
		return
	}

	resp := User {
		ID:			dbUser.ID,
		CreatedAt:	dbUser.CreatedAt,
		UpdatedAt:	dbUser.UpdatedAt,
		Email:		dbUser.Email,
	}
	respondWithJSON(w, http.StatusCreated, resp)
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	tmpl := `<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>`
	fmt.Fprintf(w, tmpl, cfg.fileserverHits.Load())
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) { // go
    if cfg.platform != "dev" {
        respondWithError(w, http.StatusForbidden, "forbidden")
        return
    }
    // delete users in DB via your sqlc reset query
    if err := cfg.db.DeleteUsers(r.Context()); err != nil { // name may differ
        respondWithError(w, http.StatusInternalServerError, "reset failed")
        return
    }
    cfg.fileserverHits.Store(0)
    w.WriteHeader(http.StatusOK)
}

type ChirpResponse struct {
    Id        uuid.UUID    `json:"id"`
    Createdat time.Time `json:"created_at"`
    Updatedat time.Time `json:"updated_at"`
    Body      string    `json:"body"`
    Userid    uuid.UUID    `json:"user_id"`
}

type ChirpCreateParams struct {
	Body 	string `json:"body"`
	UserID	string `json:"user_id"`
}

func(cfg *apiConfig) retrieveChirp(w http.ResponseWriter, r *http.Request) {
	chir := ChirpResponse{}
	chirpPath := r.PathValue("chirpID")
	idPath, err := uuid.Parse(chirpPath)
	if err != nil {
		respondWithError(w, 400, "Invalid uuid entered")
		return
	}

	dbChirp, err := cfg.db.GetChirp(r.Context(), idPath)
	if err != nil{
		respondWithError(w, 404, "Chirp not found")
		return
	}
	chir.Id = dbChirp.ID
	chir.Createdat = dbChirp.CreatedAt
	chir.Updatedat = dbChirp.UpdatedAt
	chir.Body = dbChirp.Body
	chir.Userid = dbChirp.UserID
	respondWithJSON(w, 200, chir)
}

func (cfg *apiConfig) retrieveChirps(w http.ResponseWriter, r *http.Request) {
	rChirps := []ChirpResponse{}
	dbChirps, err := cfg.db.GetChirps(r.Context())
	if err !=  nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't retrieve chirps")
		return
	}
	for _, c := range dbChirps {
		rChirps = append(rChirps, ChirpResponse{
			Id:			c.ID,
			Createdat:	c.CreatedAt,
			Updatedat:	c.UpdatedAt,
			Body:		c.Body,
			Userid:		c.UserID,
		})
	}
	respondWithJSON(w, 200, rChirps)
}

func (cfg *apiConfig) handlerChirps(w http.ResponseWriter, r *http.Request) {
	id := uuid.New()
	var paramsinsert ChirpCreateParams

	
	if err := json.NewDecoder(r.Body).Decode(&paramsinsert); err != nil {
		respondWithError(w, 400, "Something went wrong")
		return
	}
	if len(paramsinsert.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	}

	uid, err := uuid.Parse(paramsinsert.UserID)
	if err != nil {
		respondWithError(w, 400, "user_id must be a valid UUID")
		return
	}

	cleaned := sanitizeChirp(paramsinsert.Body)

	

	arg := database.CreateChirpParams{
		ID:			id,
		Body:		cleaned,
		UserID:		uid,
	}
	chirp, err := cfg.db.CreateChirp(r.Context(), arg)
	if err != nil {
		respondWithError(w, 500, "Couldn't create chirp")
		return
	}
	
	var chirpy ChirpResponse
	chirpy.Id = chirp.ID
	//chirpy.created_at = chirp.CreatedAt
	//chirpy.updated_at = chirp.UpdatedAt
	chirpy.Body = chirp.Body
	chirpy.Userid = chirp.UserID

	respondWithJSON(w, 201, chirpy)
	
}

func sanitizeChirp (body string) string {
	banned := map[string]struct{}{
    "kerfuffle": {},
    "sharbert":  {},
    "fornax":    {},
}

tokens := strings.Split(body, " ")
result := make([]string, 0, len(tokens))
	
    for _, tok := range tokens {
        if tok == "" {
            continue 
        }
        if _, found := banned[strings.ToLower(tok)]; found {
            result = append(result, "****")
        } else {
            result = append(result, tok)
        }
    }

    return strings.Join(result, " ")
}
