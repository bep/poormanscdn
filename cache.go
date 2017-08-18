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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	"os"
	pathLib "path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alexandres/poormanscdn/client"
	"github.com/syndtr/goleveldb/leveldb"
)

type StorageProvider interface {
	Read(path string, w *CacheWriter) *StorageProviderError
	//Stat(path string) (Stat, error)
}

type StorageProviderError struct {
	status int
	error
}

type CacheError struct {
	status int
	error
}

type Stat struct {
	Path           string
	SizeInBytes    uint64
	LastModifiedAt time.Time
	ETag           string
}

type Cache struct {
	db                        *leveldb.DB
	storageProvider           StorageProvider
	cacheDir                  string
	cacheSize                 uint64
	tmpDir                    string
	bytesInUse                uint64
	bytesUsedChan             chan int64
	freeSpaceBatchSizeInBytes uint64
	bytesOut                  uint64
	bytesIn                   uint64
	startedAt                 time.Time
}

type CacheStats struct {
	BytesInUse uint64
	BytesOut   uint64
	BytesIn    uint64
	Uptime     int64
}

func (c *Cache) getStats() string {
	cacheStats := CacheStats{
		c.bytesInUse,
		c.bytesOut,
		c.bytesIn,
		time.Now().Unix() - c.startedAt.Unix(),
	}
	stats, _ := json.Marshal(cacheStats)
	return string(stats)
}

type CacheWriter struct {
	client CacheClient
	io.Writer
	bytesWritten int64
}

type CacheClient struct {
	http.ResponseWriter
	req *http.Request
}

func (c *CacheWriter) WriteSize(sizeInBytes int64) {
	c.client.Header().Set("Content-Length", strconv.FormatInt(sizeInBytes, 10))
	c.bytesWritten = sizeInBytes
}

func (c *Cache) Read(path string, lastModifiedAt time.Time, cacheClient CacheClient) *CacheError {
	pathParts := strings.Split(path, "/")
	for _, elem := range pathParts {
		if elem == "." || elem == ".." {
			err := errors.New("naughty path")
			return &CacheError{http.StatusBadRequest, err}
		}
	}
	path = client.TrimPath(path)
	if len(path) == 0 {
		err := errors.New("Empty path")
		return &CacheError{http.StatusBadRequest, err}
	}

	if path == "cacheStats" {
		_, err := fmt.Fprint(cacheClient, c.getStats())
		if err != nil {
			return &CacheError{http.StatusInternalServerError, err}
		}
		return nil
	}

	fullPath := c.buildCachePath(path)

	stat, err := os.Stat(fullPath)
	if err == nil && !stat.ModTime().Before(lastModifiedAt) {
		file, err := os.Open(fullPath)
		if err != nil {
			return &CacheError{http.StatusInternalServerError, err}
		}
		defer file.Close()
		err = PutFile(c.db, path)
		if err != nil {
			return &CacheError{http.StatusInternalServerError, err}
		}
		c.bytesOut += uint64(stat.Size())
		http.ServeContent(cacheClient, cacheClient.req, fullPath, stat.ModTime(), file)
		return nil
	}

	tmp, err := c.getTmpFile()
	if err != nil {
		return &CacheError{http.StatusInternalServerError, err}
	}
	tmpName := tmp.Name()
	tmpClosed := false
	tmpRemoved := false
	defer func() {
		if !tmpClosed {
			tmp.Close()
		}
		if !tmpRemoved {
			os.Remove(tmpName)
		}
	}()

	multiWriter := io.MultiWriter(tmp, cacheClient)
	cacheWriter := CacheWriter{cacheClient, multiWriter, 0}

	cacheClient.Header().Set("Content-Type", mime.TypeByExtension(pathLib.Ext(fullPath)))
	cacheClient.Header().Set("Accept-Ranges", "none")

	storageProviderError := c.storageProvider.Read(path, &cacheWriter)
	if storageProviderError != nil {
		return &CacheError{storageProviderError.status, storageProviderError}
	}
	dirPath := pathLib.Dir(fullPath)
	err = os.MkdirAll(dirPath, 0755)
	if err != nil {
		return &CacheError{http.StatusInternalServerError, err}
	}
	tmpClosed = true
	err = tmp.Close()
	if err != nil {
		return &CacheError{http.StatusInternalServerError, err}
	}

	err = os.Chmod(tmpName, 0644)
	if err != nil {
		return &CacheError{http.StatusInternalServerError, err}
	}

	err = os.Rename(tmpName, fullPath)
	if err != nil {
		return &CacheError{http.StatusInternalServerError, err}
	}
	tmpRemoved = true

	err = PutFile(c.db, path)
	if err != nil {
		return &CacheError{http.StatusInternalServerError, err}
	}
	sizeInBytes := cacheWriter.bytesWritten
	c.bytesOut += uint64(sizeInBytes)
	c.bytesIn += uint64(sizeInBytes)
	c.bytesUsedChan <- sizeInBytes
	return nil
}

func (c *Cache) buildCachePath(path string) string {
	return c.cacheDir + "/" + path
}

func (c *Cache) FreeSpaceWatchdog() {
	for size := range c.bytesUsedChan {
		c.bytesInUse += uint64(size)
		if c.bytesInUse > c.cacheSize {
			c.freeSpace()
		}
	}
}

func (c *Cache) freeSpace() {
	paths, err := ListPathsByModificationTime(c.db)
	if err != nil {
		log.Fatal(err)
	}
	bytesLeftToRemove := (c.bytesInUse - c.cacheSize) + c.freeSpaceBatchSizeInBytes
	for _, path := range paths {
		if bytesLeftToRemove <= 0 {
			break
		}
		freedBytes, err := c.Delete(path)
		if err != nil {
			log.Println("failed to delete " + path)
			log.Println(err)
			continue
		}
		c.bytesInUse -= freedBytes
		bytesLeftToRemove -= freedBytes
	}
}

func (c *Cache) DeleteAll() (freedBytes uint64, err error) {
	paths, err := ListPathsByModificationTime(c.db)
	if err != nil {
		log.Fatal(err)
	}
	for _, path := range paths {
		freedBytesForPath, err := c.Delete(path)
		if err != nil {
			log.Println("failed to delete " + path)
			log.Println(err)
			continue
		}
		freedBytes += freedBytesForPath
	}
	return
}

func (c *Cache) Delete(path string) (freedBytes uint64, err error) {
	fullPath := c.buildCachePath(path)
	stat, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Println(path + "no longer exists, deleting from db")
			DeleteFile(c.db, path)
			return 0, nil
		}
		return
	}
	size := uint64(stat.Size())
	err = os.Remove(fullPath)
	if err != nil {
		return
	}
	freedBytes = size
	DeleteFile(c.db, path)
	return
}

func (c *Cache) getTmpFile() (file *os.File, err error) {
	file, err = ioutil.TempFile(c.tmpDir, "poormanscdn")
	return
}

func GetCache(config Configuration, db *leveldb.DB, storageProvider StorageProvider) (cache *Cache, err error) {
	stat, err := os.Stat(config.CacheDir)
	if err != nil {
		return
	}
	if !stat.IsDir() {
		err = errors.New("invalid cache dir")
	}
	stat, err = os.Stat(config.TmpDir)
	if err != nil {
		return
	}
	if !stat.IsDir() {
		err = errors.New("invalid tmp dir")
	}

	bytesInUse := uint64(0)

	err = filepath.Walk(config.CacheDir, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// strip cache prefix
		path = strings.TrimPrefix(path, config.CacheDir+"/")
		if !f.IsDir() {
			has, err := HasFile(db, path)
			if err != nil {
				return err
			}
			if !has {
				PutFile(db, path)
			}
			bytesInUse += uint64(f.Size())
		}
		return nil
	})

	log.Printf("%d bytes in use", bytesInUse)

	if err != nil {
		return
	}

	cache = &Cache{
		db:                        db,
		storageProvider:           storageProvider,
		cacheDir:                  config.CacheDir,
		cacheSize:                 config.CacheSize,
		tmpDir:                    config.TmpDir,
		bytesInUse:                bytesInUse,
		bytesUsedChan:             make(chan int64, 1000),
		freeSpaceBatchSizeInBytes: config.FreeSpaceBatchSizeInBytes,
		startedAt:                 time.Now(),
	}
	return
}
