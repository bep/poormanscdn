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
	"net/url"
	"time"

	"github.com/alexandres/poormanscdn/client"
)

func CacheHandler(config Configuration, cache *Cache, w http.ResponseWriter, r *http.Request) (status int, err error) {
	urlPath := client.TrimPath(r.URL.Path)
	q := r.URL.Query()

	hostname, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		hostname = r.Host // c.req.Host not host:port, likely only host
	}
	host, found := config.Hosts[hostname]
	if !found {
		return http.StatusBadRequest, errors.New("invalid host")
	}
	storage := host.storageProvider

	// if ?modified=... not in query this will default to time.Unix(0, 0)
	modifiedAt, err := client.UnixTimeStrToTime(q.Get(client.ModifiedParam))
	if err != nil {
		return http.StatusBadRequest, errors.New("bad modified date")
	}

	// require valid signature if
	//     - modified query parameter threatens to purge cache (modified > epoch) or
	//     - SigRequired is true
	//     - DELETE request
	//     - view cacheStats
	if !modifiedAt.Equal(client.ZeroTime()) || host.SigRequired || r.Method == http.MethodDelete || urlPath == CacheStatsPath {
		urlString, err := buildUrlString(r)
		if err != nil {
			return http.StatusBadRequest, err
		}
		sigParams, err := client.ParseAndAuthenticateSignedUrl(config.Secret, r.Method, urlString)
		if err != nil {
			return http.StatusForbidden, err
		}

		// if expiration is not 0 and earlier than now fail
		if !sigParams.Expires.Equal(client.ZeroTime()) && sigParams.Expires.Before(time.Now()) {
			return http.StatusForbidden, errors.New("url expired")
		}

		if sigParams.UserHost != "" && sigParams.UserHost != r.RemoteAddr {
			return http.StatusForbidden, errors.New("bad userhost")
		}

		if sigParams.RefererHost != "" {
			parsedRefererUrl, err := url.Parse(r.Referer())
			if err != nil || parsedRefererUrl.Host != sigParams.RefererHost {
				return http.StatusForbidden, errors.New("bad referer")
			}
		}
	}

	if r.Method == http.MethodDelete {
		if urlPath == "" {
			_, err = cache.DeleteAll(hostname)
		} else {
			_, err = cache.Delete(hostname, urlPath)
		}
		if err != nil {
			return http.StatusInternalServerError, errors.New("failed to delete")
		}
	} else {
		cacheClient := CacheClient{w, r}
		if r.URL.Path[len(r.URL.Path)-1] == '/' {
			urlPath += "/index.html" // support static websites
		}
		cacheError := cache.Read(hostname, storage, urlPath, modifiedAt, cacheClient)
		if cacheError != nil {
			return cacheError.status, cacheError
		}
	}
	return http.StatusOK, nil
}

func buildUrlString(r *http.Request) (string, error) {
	if r.URL.IsAbs() {
		return r.URL.String(), nil
	}
	parsedUrl, err := url.Parse("https://" + r.Host + r.URL.String()) // TODO: get scheme somehow. doesn't matter here because signing doesn't use scheme
	if err != nil {
		return "", err
	}
	return parsedUrl.String(), err
}
