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

package client

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func hashString(s string) string {
	hash := sha1.Sum([]byte(s))
	return fmt.Sprintf("%x", hash)
}

func TrimPath(path string) string {
	return strings.Trim(path, " /")
}

func VerifySig(sig, secret, method, path, modified, expires, host, domain, userHost, referer string) (err error) {
	if method == "" {
		err = errors.New("missing method")
		return
	}
	if modified == "" {
		err = errors.New("missing modified")
		return
	}
	_, err = strconv.ParseInt(modified, 10, 64)
	if err != nil {
		err = errors.New("bad modified")
		return
	}
	if sig == "" {
		err = errors.New("missing sig")
		return
	}
	if expires != "" {
		expiresInt, err := strconv.ParseInt(expires, 10, 64)
		if err != nil {
			err = errors.New("bad expiration")
			return err
		}

		if expiresInt < time.Now().Unix() {
			err = errors.New("link expired")
			return err
		}
	}
	if host != "" {
		if host != userHost {
			err = errors.New(fmt.Sprintf("only downloads from %s allowed, you are %s", host, userHost))
			return
		}
	}
	if domain != "" {
		refererUrl, err := url.Parse(referer)
		refererHost := strings.Split(refererUrl.Host, ":")[0]
		if err == nil && !strings.Contains(refererUrl.Host, domain) {
			err = errors.New(fmt.Sprintf("bad ref %s, should be %s", refererHost, domain))
			return err
		}
	}
	correctSig := Sign(secret, method, path, modified, expires, host, domain)
	if correctSig != sig {
		err = errors.New("auth failed")
		return err
	}
	return
}

func Sign(secret, method, path, modified, expires, host, domain string) string {
	toSign := strings.Join([]string{method, path, modified, expires, host, domain}, "&")
	return hashString(secret + hashString(toSign))
}

func GetSignedUrl(secret string, baseUrl string, method string, path string, host string,
	domain string, modified *time.Time, expires *time.Time) (signedUrl string, err error) {
	path = TrimPath(path)
	parsedBaseUrl, err := url.Parse(baseUrl)
	if err != nil {
		return
	}
	q := parsedBaseUrl.Query()
	q.Set("host", host)
	q.Set("domain", domain)
	modifiedStr := "0"
	if modified != nil {
		modifiedStr = strconv.FormatInt(modified.Unix(), 10)
	}
	q.Set("modified", modifiedStr)
	expiresStr := ""
	if expires != nil {
		expiresStr = strconv.FormatInt(expires.Unix(), 10)
	}
	q.Set("expires", expiresStr)
	q.Set("sig", Sign(secret, method, path, modifiedStr, expiresStr, host, domain))
	newUrl := url.URL{
		Scheme:   parsedBaseUrl.Scheme,
		User:     parsedBaseUrl.User,
		Host:     parsedBaseUrl.Host,
		Path:     path,
		RawQuery: q.Encode(),
		Fragment: parsedBaseUrl.Fragment,
	}
	signedUrl = newUrl.String()
	return
}
