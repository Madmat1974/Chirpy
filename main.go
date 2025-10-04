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
	"github.com/Madmat1974/Chirpy.git/internal/auth"

    "github.com/joho/godotenv"
    _ "github.com/lib/pq"

    "github.com/google/uuid"
    "github.com/Madmat1974/Chirpy.git/internal/database"
	"errors"
	"sort"
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
	secret: os.Getenv("JWT_SECRET"),
	pkey: os.Getenv("POLKA_KEY"),
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
	mux.HandleFunc("POST /api/login", cfg.loginUser)
	mux.HandleFunc("POST /api/refresh", cfg.refresh)
	mux.HandleFunc("POST /api/revoke", cfg.revoke)
	mux.HandleFunc("PUT /api/users", cfg.putUser)
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", cfg.deleteChirp)
	mux.HandleFunc("POST /api/polka/webhooks", cfg.webhooks)
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
	db 			*database.Queries
	platform 	string
	secret 		string
	pkey		string
}

type userCreateParams struct {
	Password			string 	`json:"password"`
	Email 				string 	`json:"email"`
	
}

type User struct {
	ID 			uuid.UUID `json:"id"`
	CreatedAt	time.Time `json:"created_at"`
	UpdatedAt	time.Time `json:"updated_at"`
	Email		string	  `json:"email"`
	IsChirpyRed bool	  `json:"is_chirpy_red"`
}

type Data struct {
	UserID uuid.UUID `json:"user_id"`
}

type Webhooker struct {
	Event		string	`json:"event"`
	Data		Data	`json:"data"`
}

func (cfg *apiConfig) webhooks(w http.ResponseWriter, r *http.Request) {
	getkey, err := auth.GetAPIKey(r.Header)
	if err != nil || getkey == "" {
		respondWithError(w, http.StatusUnauthorized, "invalid or missing apikey")
		return
	}
	if cfg.pkey != getkey {
		respondWithError(w, 401, "apikey mismatch")
	}
	
	var payload Webhooker

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if payload.Event != "user.upgraded" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	_, err = cfg.db.UpgradeRedByID(r.Context(), payload.Data.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "db error")
		return
	}
	w.WriteHeader(http.StatusNoContent)

}

func (cfg *apiConfig) deleteChirp(w http.ResponseWriter, r *http.Request) {
    chirpPath := r.PathValue("chirpID")
    idPath, err := uuid.Parse(chirpPath)
    if err != nil {
        respondWithError(w, http.StatusBadRequest, "invalid chirp id")
        return
    }

    token, err := auth.GetBearerToken(r.Header)
    if err != nil {
        respondWithError(w, http.StatusUnauthorized, "missing or invalid auth header")
        return
    }

    subStr, err := auth.ValidateJWT(token, cfg.secret)
    if err != nil {
        respondWithError(w, http.StatusUnauthorized, "invalid token")
        return
    }
  
    dbChirp, err := cfg.db.GetChirp(r.Context(), idPath)
    if err != nil {
        respondWithError(w, http.StatusNotFound, "chirp not found")
        return
    }

    if dbChirp.UserID != subStr {
        respondWithError(w, http.StatusForbidden, "forbidden")
        return
    }

    if err := cfg.db.DeleteChirp(r.Context(), idPath); err != nil {
        respondWithError(w, http.StatusInternalServerError, "could not delete")
        return
    }
    w.WriteHeader(http.StatusNoContent)
}


func (cfg *apiConfig) loginUser(w http.ResponseWriter, r *http.Request) {
	var params userCreateParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if strings.TrimSpace(params.Password) == "" {
		respondWithError(w, http.StatusBadRequest, "password required")
		return
	}
	finduser, err := cfg.db.GetUserByEmail(r.Context(), params.Email)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "user not found")
		return
	}

	if !finduser.HashedPassword.Valid {
    // Handle the case where the password is NULL
    // This might happen for users created before the password column was added
    respondWithError(w, 401, "Incorrect email or password")
    return
}
	pcheck, err := auth.CheckPasswordHash(params.Password, finduser.HashedPassword.String)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "problem validating password")
			return
		}
		if pcheck == false {
			respondWithError(w, 401, "invalid email or password")
			return
		}

	
	token, err := auth.MakeJWT(finduser.ID, cfg.secret, time.Hour)
	if err != nil {
    	respondWithError(w, http.StatusInternalServerError, "could not create token")
    	return
	}

	rt, err := auth.MakeRefreshToken()
	if err != nil {
    	respondWithError(w, http.StatusInternalServerError, "could not create refresh token")
    	return
	}
	if _, err := cfg.db.RefreshToken(r.Context(), database.RefreshTokenParams{
    	Token:  rt,
    	UserID: finduser.ID,
	}); err != nil {
    	respondWithError(w, http.StatusInternalServerError, "could not persist refresh token")
    	return
	}
	
	respondWithJSON(w, 200, struct {
    	ID        		uuid.UUID 	`json:"id"`
    	CreatedAt 		time.Time 	`json:"created_at"`
    	UpdatedAt 		time.Time 	`json:"updated_at"`
    	Email     		string    	`json:"email"`
    	Token     		string    	`json:"token"`
		RefreshToken	string		`json:"refresh_token"`
		IsChirpyRed		bool		`json:"is_chirpy_red"`
	}{
    	ID: finduser.ID,
    	CreatedAt: finduser.CreatedAt,
    	UpdatedAt: finduser.UpdatedAt,
    	Email: finduser.Email,
    	Token: token,
    	RefreshToken: rt,
		IsChirpyRed: finduser.IsChirpyRed,
})
}


func (cfg *apiConfig) userCreate(w http.ResponseWriter, r *http.Request) {
	var params userCreateParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if strings.TrimSpace(params.Password) == "" {
		respondWithError(w, http.StatusBadRequest, "password required")
		return
	}

	phash, err := auth.HashPassword(params.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not use password")
		return
	}

	if strings.TrimSpace(params.Email) == "" {
		respondWithError(w, http.StatusBadRequest, "email required")
		return
	}
	dbUser, err := cfg.db.CreateUser(r.Context(), database.CreateUserParams{
    Email:          params.Email,
    HashedPassword: sql.NullString{
		String: phash,
		Valid: true,
	},
	})

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "could not create user")
		return
	}

	resp := User {
		ID:				dbUser.ID,
		CreatedAt:		dbUser.CreatedAt,
		UpdatedAt:		dbUser.UpdatedAt,
		Email:			dbUser.Email,
		IsChirpyRed:	dbUser.IsChirpyRed,
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
	aidStr := r.URL.Query().Get("author_id")
	sortSTR := r.URL.Query().Get("sort")

	var dbChirps []database.Chirp
	var err error

	if aidStr == "" {
    	dbChirps, err = cfg.db.GetChirps(r.Context())
	} else {
    	uid, err := uuid.Parse(aidStr)
    	if err != nil {
        	respondWithError(w, http.StatusBadRequest, "invalid author_id")
        	return
    	}
    	dbChirps, err = cfg.db.GetChirpsByUserID(r.Context(), uid)
	}
	if err != nil {
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

	//sorting time
	if sortSTR == "" || sortSTR == "asc" {
		sort.Slice(rChirps, func(i, j int) bool {
			return rChirps[i].Createdat.Before(rChirps[j].Createdat)
		})
	}
	if sortSTR == "desc" {
		sort.Slice(rChirps, func(i, j int) bool {
			return rChirps[i].Createdat.After(rChirps[j].Createdat)
	})
	}

	respondWithJSON(w, 200, rChirps)
}

func (cfg *apiConfig) handlerChirps(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Authorization header missing or invalid")
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	var paramsinsert ChirpCreateParams	
	if err := json.NewDecoder(r.Body).Decode(&paramsinsert); err != nil {
		respondWithError(w, 400, "Something went wrong")
		return
	}
	if len(paramsinsert.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	}

	cleaned := sanitizeChirp(paramsinsert.Body)
	arg := database.CreateChirpParams{
		ID:			uuid.New(),
		Body:		cleaned,
		UserID:		userID,
	}
	chirp, err := cfg.db.CreateChirp(r.Context(), arg)
	if err != nil {
		respondWithError(w, 500, "Couldn't create chirp")
		return
	}
	
	respondWithJSON(w, 201, ChirpResponse{
		Id:			chirp.ID,
		Createdat:	chirp.CreatedAt,
		Updatedat:	chirp.UpdatedAt,
		Body:		chirp.Body,
		Userid:		chirp.UserID,
	})
	
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

func (cfg *apiConfig) refresh(w http.ResponseWriter, r *http.Request) {
	tok, err := auth.GetBearerToken(r.Header)
	if err != nil || tok == "" {
		respondWithError(w, http.StatusUnauthorized, "invalid or missing refresh token")
		return
	}

	u, err := cfg.db.GetUserFromRefreshToken(r.Context(), tok)
	if err != nil {
    respondWithError(w, http.StatusUnauthorized, "invalid or expired refresh token")
    return
	}

	jwt, err := auth.MakeJWT(u.ID, cfg.secret, time.Hour)
	if err != nil {
    	respondWithError(w, http.StatusInternalServerError, "could not create access token")
    	return
	}

	respondWithJSON(w, http.StatusOK, map[string]string{"token":jwt})
}

func (cfg *apiConfig) revoke(w http.ResponseWriter, r *http.Request) {
	tok, err := auth.GetBearerToken(r.Header)
	if err != nil || tok == "" {
		respondWithError(w, http.StatusUnauthorized, "invalid or missing refresh token")
		return
	}

	rows, err := cfg.db.RevokeRefreshToken(r.Context(), tok) 
	if rows == 0 {
    respondWithError(w, http.StatusUnauthorized, "invalid or revoked token")
    return
	}
	w.WriteHeader(http.StatusNoContent)
}

// go
func (cfg *apiConfig) putUser(w http.ResponseWriter, r *http.Request) {
    tok, err := auth.GetBearerToken(r.Header)
    if err != nil || tok == "" {
        respondWithError(w, 401, "invalid or missing access token")
        return
    }

    userID, err := auth.ValidateJWT(tok, cfg.secret)
    if err != nil {
        respondWithError(w, 401, "invalid or missing access token")
        return
    }

    type updateParams struct {
        Email    string `json:"email"`
        Password string `json:"password"`
    }
    type userResponse struct {
        ID        string    `json:"id"`
        Email     string    `json:"email"`
        CreatedAt time.Time `json:"created_at"`
        UpdatedAt time.Time `json:"updated_at"`
    }

    var params updateParams
    if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
        respondWithError(w, http.StatusBadRequest, "invalid JSON")
        return
    }
    if strings.TrimSpace(params.Email) == "" || strings.TrimSpace(params.Password) == "" {
        respondWithError(w, http.StatusBadRequest, "email and password required")
        return
    }

    p, err := auth.HashPassword(params.Password)
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "could not use password")
        return
    }

    u, err := cfg.db.UpdateUserByID(r.Context(), database.UpdateUserByIDParams{
        ID:    userID,
        Email: params.Email,
        HashedPassword: sql.NullString{
            String: p,
            Valid:  true,
        },
    })
    if err != nil {
        respondWithError(w, http.StatusInternalServerError, "could not update user")
        return
    }

    resp := userResponse{
        ID:        u.ID.String(),
        Email:     u.Email,
        CreatedAt: u.CreatedAt,
        UpdatedAt: u.UpdatedAt,
    }
    respondWithJSON(w, http.StatusOK, resp)
}