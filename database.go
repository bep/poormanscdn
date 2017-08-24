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
	"bytes"
	"encoding/gob"
	pathLib "path"
	"sort"
	"strings"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
)

const (
	accessTimeNamespace = "accessed"
	headersNamepace     = "headers"
)

func namespacedPath(namespace, path string) string {
	return pathLib.Join(namespace, path)
}

func GetDatabase(path string) (db *leveldb.DB, err error) {
	return leveldb.OpenFile(path, nil)
}

func TouchFileAccessTime(db *leveldb.DB, path string) (err error) {
	timeBinary, err := time.Now().MarshalBinary()
	if err != nil {
		return
	}
	err = db.Put([]byte(namespacedPath(accessTimeNamespace, path)), timeBinary, nil)
	return
}

func PutHeaders(db *leveldb.DB, path string, headers map[string]string) (err error) {
	var headerBuf bytes.Buffer
	enc := gob.NewEncoder(&headerBuf)
	err = enc.Encode(headers)
	if err != nil {
		return
	}
	err = db.Put([]byte(namespacedPath(headersNamepace, path)), headerBuf.Bytes(), nil)
	return
}

func GetHeaders(db *leveldb.DB, path string) (headers map[string]string, err error) {
	headerBytes, err := db.Get([]byte(namespacedPath(headersNamepace, path)), nil)
	headerBuf := bytes.NewBuffer(headerBytes)
	if err != nil {
		return
	}
	dec := gob.NewDecoder(headerBuf)
	err = dec.Decode(&headers)
	return
}

func HasFile(db *leveldb.DB, path string) (has bool, err error) {
	hasAccessTime, err := db.Has([]byte(namespacedPath(accessTimeNamespace, path)), nil)
	if err != nil {
		return
	}
	hasHeaders, err := db.Has([]byte(namespacedPath(headersNamepace, path)), nil)
	if err != nil {
		return
	}
	has = hasAccessTime && hasHeaders
	return
}

func DeleteFile(db *leveldb.DB, path string) (err error) {
	err = db.Delete([]byte(namespacedPath(accessTimeNamespace, path)), nil)
	if err != nil {
		return
	}
	err = db.Delete([]byte(namespacedPath(headersNamepace, path)), nil)
	return
}

type PathModified struct {
	path           string
	lastModifiedAt time.Time
}

type ByLastModifiedAt []PathModified

func (a ByLastModifiedAt) Len() int      { return len(a) }
func (a ByLastModifiedAt) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByLastModifiedAt) Less(i, j int) bool {
	return a[i].lastModifiedAt.Before(a[j].lastModifiedAt)
}

func ListPathsByModificationTime(db *leveldb.DB) (paths []string, err error) {
	var pathModified []PathModified
	iter := db.NewIterator(nil, nil)
	defer iter.Release()
	for iter.Next() {
		k := iter.Key()
		path := string(k[:])
		if !strings.HasPrefix(path, accessTimeNamespace) {
			continue
		}
		v := iter.Value()
		lastModifiedAt := time.Now()
		err := lastModifiedAt.UnmarshalBinary(v)
		if err != nil {
			return nil, err
		}
		pathStrippedOfNamespace := strings.TrimPrefix(path, accessTimeNamespace+"/")
		pathModified = append(pathModified, PathModified{pathStrippedOfNamespace, lastModifiedAt})
	}
	if err != nil {
		return
	}
	sort.Sort(ByLastModifiedAt(pathModified))
	for _, aPathModified := range pathModified {
		paths = append(paths, aPathModified.path)
	}
	return
}
