package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

var (
	cache      = make(map[string]CacheItem) // Map to store cache items
	cacheMutex = sync.RWMutex{}             // Mutex to handle access to the cache
)

// CacheItem represents a cached response
type CacheItem struct {
	Response []byte      // The actual response body
	Headers  http.Header // The headers of the response
	CachedAt time.Time   // Time when the response was cached
}

func main() {
	port := flag.Int("port", 6000, "Port on which the proxy server will run")
	origin := flag.String("origin", "", "The origin server to forward requests to")
	flag.Parse()

	if *origin == "" {
		log.Fatal("Origin not specified")
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		HandleRequest(w, r, *origin)
	})

	// Start the proxy server
	log.Printf("Proxy started on port %d, forwarding to %s\n", *port, *origin)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}

func HandleRequest(w http.ResponseWriter, r *http.Request, origin string) {
	originURL := fmt.Sprintf("%s%s", origin, r.URL.Path)

	// Check if the URL is already in cache
	cacheMutex.RLock()
	cacheItem, found := cache[originURL]
	cacheMutex.RUnlock()

	if found {
		// If found in cache, return the cached response
		for k, v := range cacheItem.Headers {
			w.Header()[k] = v // Copy cached headers to the response
		}
		w.Header().Set("X-Cache", "HIT")
		w.Write(cacheItem.Response) // Write cached response body
		return
	}

	// If not found in cache, fetch from origin
	res, err := http.Get(originURL)
	if err != nil {
		http.Error(w, "Error fetching from origin", http.StatusBadGateway)
		return
	}
	defer res.Body.Close()

	var responseData map[string]interface{}
	err = json.NewDecoder(res.Body).Decode(&responseData)
	if err != nil {
		http.Error(w, "Error decoding response from origin", http.StatusInternalServerError)
		return
	}

	w.Header().Set("X-Cache", "MISS")
	if err := json.NewEncoder(w).Encode(responseData); err != nil {
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
		return
	}

	// Cache the response for future requests
	cacheMutex.Lock()
	body, err := json.Marshal(responseData)
	if err != nil {
		http.Error(w, "Error encoding response for cache", http.StatusInternalServerError)
		cacheMutex.Unlock()
		return
	}

	// Copy headers to avoid modifying the original headers
	copiedHeaders := make(http.Header)
	for k, v := range res.Header {
		copiedHeaders[k] = v
	}

	cache[originURL] = CacheItem{
		Response: body,          // Store the body in the cache
		Headers:  copiedHeaders, // Store the headers
		CachedAt: time.Now(),    // Record when cached
	}
	cacheMutex.Unlock()
}
