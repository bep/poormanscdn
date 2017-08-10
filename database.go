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
	"github.com/syndtr/goleveldb/leveldb"
	"sort"
	"time"
)

func GetDatabase(path string) (db *leveldb.DB, err error) {
	return leveldb.OpenFile(path, nil)
}

func PutFile(db *leveldb.DB, path string) (err error) {
	timeBinary, err := time.Now().MarshalBinary()
	if err != nil {
		return
	}
	err = db.Put([]byte(path), timeBinary, nil)
	return
}

func HasFile(db *leveldb.DB, path string) (bool, error) {
	return db.Has([]byte(path), nil)
}

func DeleteFile(db *leveldb.DB, path string) (err error) {
	err = db.Delete([]byte(path), nil)
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
		v := iter.Value()
		path := string(k[:])
		lastModifiedAt := time.Now()
		err := lastModifiedAt.UnmarshalBinary(v)
		if err != nil {
			return nil, err
		}
		pathModified = append(pathModified, PathModified{path, lastModifiedAt})
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
