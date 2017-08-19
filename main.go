/*
 * Copyright (c) 2017 Salle, Alexandre <atsalle@inf.ufrgs.br>
 * Author: Salle, Alexandre <atsalle@inf.ufrgs.br>
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy of
 * this software and associated documentation files (the "Software"), to deal in
 * the Software without restriction, including without limitation the rights to
 * use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
 * the Software, and to permit persons to whom the Software is furnished to do so,
 * subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
 * FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
 * COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
 * IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
 * CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

func httpError(w http.ResponseWriter, err error, code int) {
	w.Header().Set("Content-Type", "text/plain")
	http.Error(w, err.Error(), code)
}

func main() {
	config, err := getConfiguration("config.json")
	if err != nil {
		log.Fatal(err)
	}
	db, err := GetDatabase(config.DatabaseDir)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	storageClients := GetS3Clients(config)
	if err != nil {
		log.Fatal(err)
	}
	cache, err := GetCache(config, db, storageClients)
	if err != nil {
		log.Fatal(err)
	}

	go cache.FreeSpaceWatchdog()
	cache.bytesUsedChan <- 0 // just to free space if needed on startup

	h := http.NewServeMux()

	h.HandleFunc("/robots.txt", makeHandler(
		config,
		cache,
		func(config Configuration, cache *Cache, w http.ResponseWriter, r *http.Request) (int, error) {
			fmt.Fprint(w, "User-agent: *\nDisallow: /")
			return http.StatusOK, nil
		}))
	h.HandleFunc("/favicon.ico", makeHandler(
		config,
		cache,
		func(config Configuration, cache *Cache, w http.ResponseWriter, r *http.Request) (int, error) {
			return http.StatusNotFound, errors.New("not found")
		}))

	h.HandleFunc("/", makeHandler(config, cache, CacheHandler))

	needsTLS, err := config.isTLSConfigured()
	if err != nil {
		log.Fatal(err)
	}

	if needsTLS {
		m := autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(config.hostNames()...),
			Cache:      autocert.DirCache(config.TLSCertificateDir),
		}
		s := &http.Server{
			Addr:      config.Listen,
			TLSConfig: &tls.Config{GetCertificate: m.GetCertificate},
			Handler:   h,
		}
		log.Fatal(s.ListenAndServeTLS("", ""))
	} else {
		s := &http.Server{
			Addr:    config.Listen,
			Handler: h,
		}
		log.Fatal(s.ListenAndServe())
	}
}

func makeHandler(config Configuration, cache *Cache, handler func(Configuration, *Cache, http.ResponseWriter, *http.Request) (int, error)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := handler(config, cache, w, r)
		if status != http.StatusOK {
			if w.Header().Get("Content-Length") == "" { // response not yet sent, ok to write to repsonse
				WriteResponseError(os.Stderr, w, r, status, err)
			} else if status == http.StatusInternalServerError {
				WriteError(os.Stderr, r, time.Now(), status, err) // don't write to response
			}
		}
		WriteCombinedLog(os.Stdout, r, *r.URL, time.Now(), status, getContentLength(w))
	}
}

func getContentLength(w http.ResponseWriter) int64 {
	contentLength, err := strconv.ParseInt(w.Header().Get("Content-Length"), 10, 64)
	if err != nil {
		contentLength = 0
	}
	return contentLength
}
