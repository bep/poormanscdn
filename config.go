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
)

const (
	awsAccessKeyEnv       = "AWS_ACCESS_KEY"
	awsSecretAccessKeyEnv = "AWS_SECRET_ACCESS_KEY"
	pcdnSecretEnv         = "PCDN_SECRET"
)

type Configuration struct {
	Listen                    string
	Hosts                     map[string]Host
	DefaultAccessKey          string
	DefaultSecretKey          string
	TmpDir                    string
	CacheDir                  string
	CacheSize                 uint64
	DatabaseDir               string
	TLSCertificateDir         string
	FreeSpaceBatchSizeInBytes uint64
	Secret                    string
	SigRequired               bool
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
		return false, fmt.Errorf("dir %d not valid as certificate dir", c.TLSCertificateDir)
	}

	return true, nil

}

type Host struct {
	Bucket string
	Path   string

	// Note that if not set, these will get their values from the default or from env.
	AccessKey string
	SecretKey string
}

func readConfiguration(r io.Reader) (conf Configuration, err error) {

	decoder := json.NewDecoder(r)
	conf = Configuration{}
	err = decoder.Decode(&conf)
	if err != nil {
		return
	}

	if conf.DefaultAccessKey == "" {
		// check for AWS keys in environment
		s3AccessKey := os.Getenv(awsAccessKeyEnv)
		if s3AccessKey != "" {
			conf.DefaultAccessKey = s3AccessKey
			log.Printf("Using %s from environment var\n", awsAccessKeyEnv)
		}
	}

	if conf.DefaultSecretKey == "" {
		s3SecretKey := os.Getenv(awsSecretAccessKeyEnv)
		if s3SecretKey != "" {
			conf.DefaultSecretKey = s3SecretKey
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

	for name, host := range conf.Hosts {
		if host.AccessKey == "" {
			host.AccessKey = conf.DefaultAccessKey
		}
		if host.SecretKey == "" {
			host.SecretKey = conf.DefaultSecretKey
		}
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
