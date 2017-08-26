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

const (
	UserHostParam    = "host"
	RefererHostParam = "domain"
	ModifiedParam    = "modified"
	ExpiresParam     = "expires"
	SigParam         = "sig"
)

func hashString(s string) string {
	hash := sha1.Sum([]byte(s))
	return fmt.Sprintf("%x", hash)
}

func TrimPath(path string) string {
	return strings.Trim(path, " /")
}

type SigParams struct {
	Host        string
	Method      string
	Path        string
	Modified    time.Time
	Expires     time.Time
	RefererHost string
	UserHost    string
}

func Sign(secret string, p SigParams) string {
	toSign := strings.Join([]string{p.Host, p.Method, p.Path, timeToStr(p.Modified), timeToStr(p.Expires), p.UserHost, p.RefererHost}, "&")
	return hashString(secret + hashString(toSign))
}

func timeToStr(t time.Time) string {
	return strconv.FormatInt(t.Unix(), 10)
}

func GetSignedUrl(secret string, cdnUrl string, p SigParams) (signedUrl string, err error) {
	parsedBaseUrl, err := url.Parse(cdnUrl)
	if err != nil {
		return
	}
	p.Host = parsedBaseUrl.Host
	q := parsedBaseUrl.Query()
	q.Set(UserHostParam, p.UserHost)
	q.Set(RefererHostParam, p.RefererHost)
	q.Set(ModifiedParam, timeToStr(p.Modified))
	q.Set(ExpiresParam, timeToStr(p.Expires))
	q.Set(SigParam, Sign(secret, p))
	newUrl := url.URL{
		Scheme:   parsedBaseUrl.Scheme,
		User:     parsedBaseUrl.User,
		Host:     parsedBaseUrl.Host,
		Path:     TrimPath(p.Path),
		RawQuery: q.Encode(),
		Fragment: parsedBaseUrl.Fragment,
	}
	signedUrl = newUrl.String()
	return
}

func UnixTimeStrToTime(s string) (t time.Time, err error) {
	t = ZeroTime()
	if s != "" {
		tInt, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return t, err
		}
		t = time.Unix(tInt, 0)
	}
	return
}

func ZeroTime() time.Time {
	return time.Unix(0, 0)
}

func ParseAndAuthenticateSignedUrl(secret string, method string, signedUrl string) (sigParams SigParams, err error) {
	parsedUrl, err := url.Parse(signedUrl)
	if err != nil {
		return
	}
	q := parsedUrl.Query()

	modifiedAt, err := UnixTimeStrToTime(q.Get(ModifiedParam))
	if err != nil {
		return
	}

	expiresAt, err := UnixTimeStrToTime(q.Get(ExpiresParam))
	if err != nil {
		return
	}

	sigParams = SigParams{
		Host:        parsedUrl.Host,
		Method:      method,
		Path:        TrimPath(parsedUrl.Path),
		Modified:    modifiedAt,
		Expires:     expiresAt,
		UserHost:    q.Get(UserHostParam),
		RefererHost: q.Get(RefererHostParam),
	}
	sig := Sign(secret, sigParams)
	if sig != q.Get(SigParam) {
		err = errors.New("bad sig")
	}
	return
}
