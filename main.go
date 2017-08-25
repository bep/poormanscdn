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
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/NYTimes/gziphandler"
	"golang.org/x/crypto/acme/autocert"
)

func httpError(w http.ResponseWriter, err error, code int) {
	w.Header().Set("Content-Type", "text/plain")
	http.Error(w, err.Error(), code)
}

func main() {
	checkFdlimit()
	configFile := flag.String("config", "config.json", "path to config file")
	flag.Parse()
	config, err := getConfiguration(*configFile)
	if err != nil {
		log.Fatal(err)
	}
	db, err := GetDatabase(config.DatabaseDir)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	cache, err := GetCache(config, db)
	if err != nil {
		log.Fatal(err)
	}

	go cache.FreeSpaceWatchdog()
	cache.bytesUsedChan <- 0 // just to free space if needed on startup

	h := http.NewServeMux()

	cacheHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, err := CacheHandler(config, cache, w, r)
		contentLength := getContentLength(w)
		if status != http.StatusOK {
			if contentLength == 0 { // response not yet sent, ok to write error to response
				http.Error(w, fmt.Sprintf("%d: something went wrong", status), status)
			}
			WriteError(os.Stderr, r, time.Now(), status, err) // log all errors to stderr
		}
		WriteCombinedLog(os.Stdout, r, *r.URL, time.Now(), status, contentLength)
	})

	gzipHandler, err := gziphandler.GzipHandlerWithOpts(gziphandler.ContentTypes(config.GzipContentTypes))
	if err != nil {
		log.Fatal(err)
	}

	h.Handle("/", gzipHandler(cacheHandler))

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

func getContentLength(w http.ResponseWriter) int64 {
	contentLength, err := strconv.ParseInt(w.Header().Get("Content-Length"), 10, 64)
	if err != nil {
		contentLength = 0
	}
	return contentLength
}
