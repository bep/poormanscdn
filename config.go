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
	"log"
	"os"
	"sort"
	"strings"
)

const (
	awsAccessKeyEnv       = "AWS_ACCESS_KEY"
	awsSecretAccessKeyEnv = "AWS_SECRET_ACCESS_KEY"
	pcdnSecretEnv         = "PCDN_SECRET"
)

type Configuration struct {
	Listen                    string
	GzipContentTypes          []string
	Hosts                     map[string]Host
	AccessKey                 string
	SecretKey                 string
	PreserveHeaders           []string
	TmpDir                    string
	CacheDir                  string
	CacheSize                 uint64
	DatabaseDir               string
	TLSCertificateDir         string
	FreeSpaceBatchSizeInBytes uint64
	Secret                    string
}

func (c Configuration) hostNames() []string {
	var names []string
	for k, _ := range c.Hosts {
		names = append(names, k)
	}

	sort.Strings(names)

	return names
}

func (c Configuration) isTLSConfigured() (bool, error) {
	if c.TLSCertificateDir == "" {
		return false, nil
	}

	fi, err := os.Stat(c.TLSCertificateDir)
	if err != nil || !fi.IsDir() {
		return false, fmt.Errorf("dir %s not valid as certificate dir", c.TLSCertificateDir)
	}

	return true, nil

}

type Host struct {
	Bucket string
	Path   string

	// Note that if not set, these will get their values from the default or from env.
	PreserveHeaders []string
	AccessKey       string
	SecretKey       string

	SigRequired bool

	storageProvider StorageProvider
}

func readConfiguration(r io.Reader) (conf Configuration, err error) {

	decoder := json.NewDecoder(r)
	conf = Configuration{}
	err = decoder.Decode(&conf)
	if err != nil {
		return
	}

	if conf.AccessKey == "" {
		// check for AWS keys in environment
		s3AccessKey := os.Getenv(awsAccessKeyEnv)
		if s3AccessKey != "" {
			conf.AccessKey = s3AccessKey
			log.Printf("Using %s from environment var\n", awsAccessKeyEnv)
		}
	}

	if conf.SecretKey == "" {
		s3SecretKey := os.Getenv(awsSecretAccessKeyEnv)
		if s3SecretKey != "" {
			conf.SecretKey = s3SecretKey
			log.Printf("Using %s from environment var\n", awsSecretAccessKeyEnv)
		}
	}

	if conf.Secret == "" {
		secret := os.Getenv(pcdnSecretEnv)
		if secret != "" {
			conf.Secret = secret
			log.Printf("Using %s from environment var\n", pcdnSecretEnv)
		}
	}

	if conf.Secret == "" {
		err = errors.New("secret is required")
		return
	}

	mandatoryPreserveHeaders := [...]string{"Content-Type"}
	illegalPreserveHeaders := [...]string{"Content-Encoding", "Accept-Ranges", "Content-Length"}

	for name, host := range conf.Hosts {
		if host.PreserveHeaders == nil {
			host.PreserveHeaders = conf.PreserveHeaders
		}
		preserveHeaders := make([]string, len(host.PreserveHeaders))
		copy(preserveHeaders, host.PreserveHeaders)

		for _, illegalHeader := range illegalPreserveHeaders {
			for _, v := range host.PreserveHeaders {
				if strings.ToLower(v) == strings.ToLower(illegalHeader) {
					err = errors.New(fmt.Sprintf("header %s cannot be preserved", illegalHeader))
				}
			}
		}

		for _, mandatoryHeader := range mandatoryPreserveHeaders {
			found := false
			for _, v := range host.PreserveHeaders {
				if v == mandatoryHeader {
					found = true
					break
				}
			}
			if !found {
				preserveHeaders = append(preserveHeaders, mandatoryHeader)
				log.Printf("header %s added to PreserveHeaders as it is mandatory\n", mandatoryHeader)
			}
		}
		host.PreserveHeaders = preserveHeaders

		if host.AccessKey == "" {
			host.AccessKey = conf.AccessKey
			if host.AccessKey == "" {
				err = errors.New(fmt.Sprintf("no AccessKey found for host %s", name))
				return
			}
		}
		if host.SecretKey == "" {
			host.SecretKey = conf.SecretKey
			if host.SecretKey == "" {
				err = errors.New(fmt.Sprintf("no SecretKey found for host %s", name))
				return
			}
		}

		storageProvider := S3Client{
			bucket:          host.Bucket,
			path:            host.Path,
			preserveHeaders: host.PreserveHeaders,
			accessKey:       host.AccessKey,
			secretKey:       host.SecretKey,
		}
		host.storageProvider = storageProvider

		conf.Hosts[name] = host
	}
	return
}

func getConfiguration(configPath string) (conf Configuration, err error) {
	file, err := os.Open(configPath)
	if err != nil {
		return
	}
	defer file.Close()

	return readConfiguration(file)

}
