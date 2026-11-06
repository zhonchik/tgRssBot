package settings

import (
	"crypto/hmac"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"tgRssBot/aggregatorservice"
	"tgRssBot/types"
	"time"
)

//go:embed index.html
var page []byte

func mac(key []byte, msg string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(msg))
	return h.Sum(nil)
}

func identity(data, token string) (int64, bool) {
	q, err := url.ParseQuery(data)
	if err != nil {
		return 0, false
	}
	hash, err := hex.DecodeString(q.Get("hash"))
	if err != nil {
		return 0, false
	}
	q.Del("hash")
	var fields []string
	for k, v := range q {
		if len(v) != 1 {
			return 0, false
		}
		fields = append(fields, k+"="+v[0])
	}
	sort.Strings(fields)
	if !hmac.Equal(hash, mac(mac([]byte("WebAppData"), token), strings.Join(fields, "\n"))) {
		return 0, false
	}
	ts, err := strconv.ParseInt(q.Get("auth_date"), 10, 64)
	age := time.Now().Unix() - ts
	if err != nil || age < 0 || age > 3600 {
		return 0, false
	}
	var user struct{ ID int64 }
	err = json.Unmarshal([]byte(q.Get("user")), &user)
	return user.ID, err == nil && user.ID > 0
}

func Handler(as *aggregatorservice.AggregatorService, token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(page)
	})
	mux.HandleFunc("/api/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		id, ok := identity(r.Header.Get("X-Telegram-Init-Data"), token)
		if !ok {
			http.Error(w, "Reopen Settings from Telegram.", 401)
			return
		}
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(as.Subscriptions(id))
			return
		}
		var input struct {
			URL      string
			Settings types.SubscriptionSettings
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input); err != nil {
			http.Error(w, "Invalid request", 400)
			return
		}
		var err error
		switch r.Method {
		case "POST":
			err = as.TrySubscribe(id, input.URL)
		case "PUT":
			err = as.Update(id, input.URL, input.Settings)
		case "DELETE":
			err = as.TryUnsubscribe(id, input.URL)
		default:
			err = errors.New("unsupported method")
			http.Error(w, err.Error(), 405)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}
