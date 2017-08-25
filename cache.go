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
	"net/http"
	"os"
	pathLib "path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alexandres/poormanscdn/client"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	CacheStatsPath = "cacheStats"
)

type StorageProvider interface {
	Read(path string, w *CacheWriter) (bytesRead int64, err *StorageProviderError)
	PreserveHeaders() []string
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
	cacheDir                  string
	cacheSize                 uint64
	tmpDir                    string
	bytesInUse                uint64
	bytesUsedChan             chan int64
	freeSpaceBatchSizeInBytes uint64
	bytesOut                  uint64
	bytesIn                   uint64
	startedAt                 time.Time
	deleteTimestamp           uint64
	cacheLock                 *sync.RWMutex
}

type CacheStats struct {
	BytesInUse uint64
	BytesOut   uint64
	BytesIn    uint64
	Uptime     int64
}

func (c *Cache) getStats() string {
	cacheStats := CacheStats{
		atomic.LoadUint64(&c.bytesInUse),
		atomic.LoadUint64(&c.bytesOut),
		atomic.LoadUint64(&c.bytesIn),
		time.Now().Unix() - c.startedAt.Unix(),
	}
	stats, _ := json.Marshal(cacheStats)
	return string(stats)
}

type CacheClient struct {
	http.ResponseWriter
	req *http.Request
}

type CacheWriter struct {
	client           CacheClient
	preservedHeaders map[string]string
	cacheFileWriter  io.Writer
	bytesWritten     int64
	storageProvider  StorageProvider
}

func (c *CacheWriter) Write(reader io.Reader) (bytesWritten int64, err error) {
	multiWriter := io.MultiWriter(c.cacheFileWriter, c.client)
	bytesWritten, err = io.Copy(multiWriter, reader)
	return
}

func (c *CacheWriter) PreserveAndWriteHeaders(storageProviderHeaders http.Header) {
	c.WriteHeader("Content-Length", storageProviderHeaders.Get("Content-Length")) // this is why we only accepted identity encoding when fetching file from S3

	for headerName, headerVal := range storageProviderHeaders {
		headerNameLower := strings.ToLower(headerName)
		firstVal := headerVal[0]
		c.preservedHeaders[headerNameLower] = firstVal
		mapHeaderIfPreserved(headerNameLower, c.storageProvider.PreserveHeaders(), func(preservedName string) {
			c.WriteHeader(preservedName, firstVal)
		})
	}
}

func mapHeaderIfPreserved(headerName string, preservedHeaders []string, callback func(preservedName string)) {
	for _, preserveHeader := range preservedHeaders {
		if headerName == strings.ToLower(preserveHeader) { // headerName already lowered
			callback(preserveHeader)
			return
		}
	}
}

func (c *CacheWriter) WriteHeader(name, value string) {
	c.client.Header().Set(name, value)
}

func (c *Cache) Read(namespace string, storage StorageProvider, path string, lastModifiedAt time.Time, cacheClient CacheClient) *CacheError {
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

	if path == CacheStatsPath {
		_, err := fmt.Fprint(cacheClient, c.getStats())
		if err != nil {
			return &CacheError{http.StatusInternalServerError, err}
		}
		return nil
	}

	namespacedPath := pathLib.Join(namespace, path)
	fullPath := c.buildCachePath(namespacedPath)

	deleteTimestampBefore := atomic.LoadUint64(&c.deleteTimestamp)

	var file *os.File
	var preservedHeaders map[string]string
	// need to lock to ensure delete doesn't remove file
	// between Stat and Open. Once we have file handle can release lock.
	c.cacheLock.RLock()
	stat, err := os.Stat(fullPath)
	if err == nil && !stat.ModTime().Before(lastModifiedAt) {
		file, err = os.Open(fullPath)
		if err != nil {
			c.cacheLock.RUnlock()
			return &CacheError{http.StatusInternalServerError, err}
		}
		defer file.Close()
		preservedHeaders, err = GetHeaders(c.db, namespacedPath)
		if err != nil {
			c.cacheLock.RUnlock() // these multiple Unlocks are ugly. should refactor to only have one
			return &CacheError{http.StatusInternalServerError, err}
		}
	}
	c.cacheLock.RUnlock()

	if file != nil { // file is in cache
		err = TouchFileAccessTime(c.db, namespacedPath)
		if err != nil {
			return &CacheError{http.StatusInternalServerError, err}
		}
		for headerName, headerVal := range preservedHeaders {
			mapHeaderIfPreserved(headerName, storage.PreserveHeaders(), func(preservedName string) {
				cacheClient.Header().Set(headerName, headerVal)
			})
		}
		atomic.AddUint64(&c.bytesOut, uint64(stat.Size()))
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

	cacheWriter := CacheWriter{
		client:           cacheClient,
		preservedHeaders: make(map[string]string),
		cacheFileWriter:  tmp,
		storageProvider:  storage,
	}

	// can't accept range requests since we want to cache the full file,
	// future requests for cache file do support range requests
	cacheClient.ResponseWriter.Header().Set("Accept-Ranges", "none")

	sizeInBytes, storageProviderError := storage.Read(path, &cacheWriter)
	if storageProviderError != nil {
		return &CacheError{storageProviderError.status, storageProviderError}
	}
	atomic.AddUint64(&c.bytesOut, uint64(sizeInBytes))
	atomic.AddUint64(&c.bytesIn, uint64(sizeInBytes))

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

	// optimistic locking - we started the read operation to serve the client immediately
	// but before committing the read file to cache we must check whether a delete call
	// came in. the file we just read could be stale.
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()
	if deleteTimestampBefore == c.deleteTimestamp {
		err = TouchFileAccessTime(c.db, namespacedPath)
		if err != nil {
			return &CacheError{http.StatusInternalServerError, err}
		}
		err = PutHeaders(c.db, namespacedPath, cacheWriter.preservedHeaders)
		if err != nil {
			return &CacheError{http.StatusInternalServerError, err}
		}
		err = os.Rename(tmpName, fullPath)
		if err != nil {
			return &CacheError{http.StatusInternalServerError, err}
		}
		tmpRemoved = true
		c.bytesUsedChan <- sizeInBytes
	}
	return nil
}

func (c *Cache) buildCachePath(path string) string {
	return c.cacheDir + "/" + path
}

func (c *Cache) FreeSpaceWatchdog() {
	for size := range c.bytesUsedChan {
		atomic.AddUint64(&c.bytesInUse, uint64(size))
		if atomic.LoadUint64(&c.bytesInUse) > c.cacheSize {
			c.freeSpace()
		}
	}
}

func (c *Cache) freeSpace() {
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()
	paths, err := ListPathsByModificationTime(c.db)
	if err != nil {
		log.Fatal(err)
	}
	bytesLeftToRemove := (atomic.LoadUint64(&c.bytesInUse) - c.cacheSize) + c.freeSpaceBatchSizeInBytes
	for _, path := range paths {
		if bytesLeftToRemove <= 0 {
			break
		}
		_, err := c.delete(path, false)
		if err != nil {
			log.Println("failed to delete " + path)
			log.Println(err)
			continue
		}
	}
}

func (c *Cache) DeleteAll(namespace string) (freedBytes uint64, err error) {
	paths, err := ListPathsByModificationTime(c.db)
	if err != nil {
		log.Fatal(err)
	}
	for _, namespacedPath := range paths {
		if !strings.HasPrefix(namespacedPath, namespace) {
			continue
		}
		path := strings.TrimPrefix(namespacedPath, namespace+"/") // a little redundant but for clarity to avoid calling Delete with empty namespace
		freedBytesForPath, err := c.Delete(namespace, path)
		if err != nil {
			log.Println("failed to delete " + path)
			log.Println(err)
			continue
		}
		freedBytes += freedBytesForPath
	}
	return
}

func (c *Cache) Delete(namespace, path string) (uint64, error) {
	return c.delete(pathLib.Join(namespace, path), true)
}

func (c *Cache) delete(path string, acquireLock bool) (freedBytes uint64, err error) {
	if acquireLock {
		c.cacheLock.Lock()
		defer c.cacheLock.Unlock()
	}
	c.deleteTimestamp += 1
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
	atomic.AddUint64(&c.bytesInUse, -freedBytes)
	return
}

func (c *Cache) getTmpFile() (file *os.File, err error) {
	file, err = ioutil.TempFile(c.tmpDir, "poormanscdn")
	return
}

func GetCache(config Configuration, db *leveldb.DB) (cache *Cache, err error) {
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
				log.Printf("file %s in cache but headers missing, deleting\n", path)
				err = os.Remove(path)
				if err != nil {
					return err
				}
			} else {
				bytesInUse += uint64(f.Size())
			}
		}
		return nil
	})

	log.Printf("%d bytes in use", bytesInUse)

	if err != nil {
		return
	}

	cache = &Cache{
		db:                        db,
		cacheDir:                  config.CacheDir,
		cacheSize:                 config.CacheSize,
		tmpDir:                    config.TmpDir,
		bytesInUse:                bytesInUse,
		bytesUsedChan:             make(chan int64, 1000),
		freeSpaceBatchSizeInBytes: config.FreeSpaceBatchSizeInBytes,
		startedAt:                 time.Now(),
		deleteTimestamp:           0,
		cacheLock:                 &sync.RWMutex{},
	}
	return
}
