# poormanscdn

poormanscdn is a caching proxy to Amazon S3 built using Go. It is highly performant, very easy to configure (just copy the binary) and can save you a lot of money on S3 bandwidth through caching. If you run poormanscdn on a couple of cheap dedicated servers and round-robin DNS with health-checks, you can have a highly-available CDN for very low cost.

## Features

- Trivial to setup: single binary and simple config file
- Caching: Least-Recently-Used files are evicted when the cache is full
- URL signing: protect your downloads through URL signing and link expiration
- Streaming: if a file is not in the cache, the file is streamed from S3 to the client while being cached so that large files can be download immediately
- Realtime stats: call http://poormanscdnhost/cacheStats with signed request to get realtime stats on transfer and cache size
- Referer control: only allow signed downloads for users coming from your site
- Host control: only allow signed downloads from a specific IP address

## Installation

```bash
go get github.com/alexandres/poormanscdn
cd $GOPATH/src/github.com/alexandres/
make
```

This builds the `poormanscdn` standalone executable. It expects to find `config.json` in the current working directory.


## Configuration

All configuration is done by editing the `config.json` file. Options:

- Listen: interface and port to listen on - examples: 127.0.0.1:8080 to listen on localhost on port 8080 or :80 to listen on all interfaces on port 80
- S3Bucket: S3 bucket name
- S3AccessKey: S3 Access Key
- S3SecretKey: S3 Secret Key
- TmpDir: where to store temporary files, need not persist between executions
- CacheDir: where to store cached files, should persist between executions to avoid emptying the cache
- CacheSize: the maximum size in bytes of the cache - example: 40000000000 to use at most 40GB
- DatabaseDir: where to store database files, should persist between executions to maintain last-downloaded times for cached files
- FreeSpaceBatchSizeInBytes: when the cache is full, free this many bytes, should be at least as large as the largest file you'll store in your cache - example: 1000000000 to free 1GB
- Secret: the secret key used to sign download URLs and purge requests - example: use `$ hexdump -n 16 -e '4/4 "%08X" 1 "\n"' /dev/urandom` to generate 128 bit key.
- SigRequired: if true, only allows downloads using signed URLs

If ENV variables `AWS_ACCESS_KEY`, `AWS_SECRET_ACCESS_KEY`, or `PCDN_SECRET` are present, 
they will override `config.json` values.

## Usage

An S3 URL http://yourbucket.s3.amazonaws.com/some/path.ext can be served by poormanscdn by calling http://hostwithpoormanscdn/some/path.ext

Program errors and request errors are written to stderr. All request information is logged to stdout using the combined log format.

poormanscdn must have write access to CacheDir, DatabaseDir, and TmpDir, which must be created before running the program.

### Cache Invalidation

You can invalidate cached files in 2 ways:

1. Send a signed DELETE request to `http://poormanscdnhost/somepath.ext` to purge 
`somepath.ext` from the cache. Alternatively, send the request `http://poormanscdnhost/` (base path) to invalidate the entire cache.

   **Pro tip:** you can get [Netlify](http://netlify.com)-like functionality by creating a git commit hook that
   calls the signed `DELETE /` URL after your files have been pushed to S3. See URL Signing below.

2. poormanscdn invalidates a cached file if the **modified** query parameter is newer than the last modified time as given by the local filesystem. For example, passing **modified=0** means a file will never be invalidated. This only works with signed URLs.

### URL Signing

If SigRequired is set to true in your configuration, poormanscdn will only allow downloads with signed URLs. See `client/sign.go` (Go) and `client/python/poormanscdn/__init__.py` (Python) for sample implementations. There is a Go tool in `client/go/pcdn` that allows you to sign URLs from the command line.

**Dowload URL signing using Python**

```python
import poormanscdn
import datetime

last_modified_at = datetime.datetime.now() - datetime.timedelta(days=7) # file changes weekly
expires_at =  datetime.datetime.now() + datetime.timedelta(hours=1) # signed URL expires in 1 hour
domain = "mysite.com" # only allow download if Referer header is from mysite.com, set to "" to allow from any Referer
host = "192.168.1.100" # only allow download from this IP address, set to "" to allow from any IP
poormanscdn.get_signed_url("mysecretkey", "http://mycdnhost.com", "GET", "/some/file.ext", last_modified_at, expires_at, restrict_domain=domain, restrict_host=host)
```

**DELETE URL signing using Python**

```python
import poormanscdn

last_modified_at = None
expires_at = None # never expire
poormanscdn.get_signed_url("mysecretkey", "http://mycdnhost.com", "DELETE", "/", last_modified_at, expires_at)
```

As this URL never expires, you can add this to your commit hook to clear the cache after your files
have been deployed to S3.

**Viewing Cache Stats**

Construct a signed url as follows: 

```python
import poormanscdn

last_modified_at = None
expires_at = None # never expire
poormanscdn.get_signed_url("mysecretkey", "http://mycdnhost.com", "GET", "/cacheStats", last_modified_at, expires_at)
```

## Run as a service

To run poormanscdn as a service (persist across reboots), you can use the
[immortal](https://immortal.run) supervisor (or any supervisor you like). Here is an example immortal
`run.yml` file

```yaml
cmd: /path/to/home/poormanscdn
cwd: /path/to/home
env:
  AWS_ACCESS_KEY: aws-access-key
  AWS_SECRET_ACCESS_KEY: aws-secret-key
  PCDN_SECRET: pcdn_secret
log:
  file: /var/log/poormanscdn.log
  age: 86400 # seconds
  num: 7     # int
  size: 1    # MegaBytes
```

    immortal -l /var/log/poormanscdn.log ./poormanscdn


## TODO

I would love to receive pull requests for the following features:

- [ ] Script for building binary releases
- [ ] Add TLS support using Let's Encrypt
- [x] Cache Invalidation through Purging
- [ ] Add support for more backends in addition to S3
- [ ] PIP-ify Python signing lib
- [ ] JavaScript signing lib
- [ ] PHP signing lib
- [ ] Ruby signing lib
- [x] Secrets Configuration via ENV

# License

Copyright (c) 2017 Salle, Alexandre <atsalle@inf.ufrgs.br>. All work in this package is distributed under the MIT License.
