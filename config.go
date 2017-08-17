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
	"os"
)

type Configuration struct {
	Listen                    string
	S3Bucket                  string
	S3AccessKey               string
	S3SecretKey               string
	TmpDir                    string
	CacheDir                  string
	CacheSize                 uint64
	DatabaseDir               string
	FreeSpaceBatchSizeInBytes uint64
	Secret                    string
	SigRequired               bool
}

func GetConfiguration(configPath string) (conf Configuration, err error) {
	file, err := os.Open(configPath)
	if err != nil {
		return
	}
	decoder := json.NewDecoder(file)
	conf = Configuration{}
	err = decoder.Decode(&conf)
	if err != nil {
		return
	}

	// check for AWS keys in environment
	s3AccessKey := os.Getenv("S3AccessKey")
	if s3AccessKey != "" {
		conf.S3AccessKey = s3AccessKey
	}

	s3SecretKey := os.Getenv("S3SecretKey")
	if s3SecretKey != "" {
		conf.S3AccessKey = s3SecretKey
	}

	secret := os.Getenv("SECRET")
	if secret != "" {
		conf.Secret = secret
	}

	if conf.SigRequired && conf.Secret == "" {
		err = errors.New("sig is required but no secret provided")
		return
	}
	return
}
