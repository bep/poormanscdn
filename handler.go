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
	"errors"
	"net"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/alexandres/poormanscdn/client"
)

func CacheHandler(config Configuration, cache *Cache, w http.ResponseWriter, r *http.Request) (status int, err error) {
	urlPath := client.TrimPath(r.URL.Path)
	q := r.URL.Query()

	lastModifiedAtInt := int64(0) // no cached file will have timestamp <= 0

	// check if request wants to conditionally purge cache with modified invalidation
	lastModifiedAt := q.Get("modified")
	if lastModifiedAt != "" {
		lastModifiedAtInt, err = strconv.ParseInt(lastModifiedAt, 10, 64)
		if err != nil {
			return http.StatusBadRequest, errors.New("bad modified")
		}
	}

	lastModifiedAtTime := time.Unix(lastModifiedAtInt, 0)

	// require valid signature if
	//     - modified threatens to purge cache or
	//     - SigRequired is true
	//     - DELETE request
	if lastModifiedAtInt > 0 || config.SigRequired || r.Method == http.MethodDelete || urlPath == "cacheStats" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			return http.StatusInternalServerError, errors.New("bad remoteaddr")
		}

		err = client.VerifySig(q.Get("sig"), config.Secret, r.Method, urlPath, lastModifiedAt, q.Get("expires"), q.Get("host"),
			q.Get("domain"), host, r.Referer())
		if err != nil {
			return http.StatusForbidden, errors.New("bad sig")
		}
	}

	if r.Method == http.MethodDelete {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host // r.Host not host:port, likely only host
		}
		hostConfig, found := config.Hosts[host]
		if !found {
			return http.StatusBadRequest, errors.New("config not found for host")
		}

		if urlPath == "" {
			_, err = cache.DeleteWithPrefix(hostConfig.Path)
		} else {
			if hostConfig.Path != "" {
				urlPath = path.Join(hostConfig.Path, urlPath)
			}
			_, err = cache.Delete(urlPath)
		}
		if err != nil {
			return http.StatusInternalServerError, errors.New("failed to delete")
		}
	} else {
		cacheClient := CacheClient{w, r}
		if r.URL.Path[len(r.URL.Path)-1] == '/' {
			urlPath += "/index.html" // support static websites
		}
		cacheError := cache.Read(urlPath, lastModifiedAtTime, cacheClient)
		if cacheError != nil {
			return cacheError.status, cacheError
		}
	}
	return http.StatusOK, nil
}
