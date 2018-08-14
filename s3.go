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
	"fmt"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/kr/s3"
)

type S3Client struct {
	bucket          string
	path            string
	preserveHeaders []string
	accessKey       string
	secretKey       string
}

func (c S3Client) buildS3Url(p string) string {
	url, _ := url.Parse("http://" + c.bucket + ".s3.amazonaws.com/")
	if c.path != "" {
		p = path.Join(c.path, p)
	}
	url.Path = p

	return url.String()
}

func (c S3Client) Read(path string, w *CacheWriter) (bytesRead int64, storageProviderErr *StorageProviderError) {
	url := c.buildS3Url(path)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, &StorageProviderError{http.StatusInternalServerError, err}
	}
	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))

	s3.Sign(req, s3.Keys{
		AccessKey: c.accessKey,
		SecretKey: c.secretKey,
	})
	client := http.DefaultClient
	res, err := client.Do(req)
	if err != nil {
		return 0, &StorageProviderError{http.StatusServiceUnavailable, err}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		err = errors.New(fmt.Sprintf("status code: %d", res.StatusCode))
		return 0, &StorageProviderError{res.StatusCode, err}
	}

	// as per https://golang.org/pkg/net/http/#Response, Content-Length has been
	// deleted from res.Header if file was gzipped on S3 and res.Body below
	// is decompressed and Content-Length  will not be preserved
	// nor passed through to the client. Future requests served from the cache will
	// support both clients that do or do not support compression and will handle
	// Content-Length automatically.

	w.PreserveAndWriteHeaders(res.Header)
	bytesRead, err = w.Write(res.Body)
	if err != nil {
		return 0, &StorageProviderError{http.StatusRequestTimeout, err}
	}
	return
}

func (c S3Client) PreserveHeaders() []string {
	return c.preserveHeaders
}
