# poormanscdn

poormanscdn is a caching proxy to Amazon S3 built using Go. It is highly performant, very easy to configure (just copy the binary) and can save you a lot of money on S3 bandwidth through caching. If you run poormanscdn on a couple of cheap dedicated servers and round-robin DNS with health-checks, you can have a highly-available CDN for very low cost.

## Features

- Trivial to setup: single binary and simple config file
- Caching: Least-Recently-Used files are evicted when the cache is full
- Automatic HTTPS: using [Let's Encrypt](https://letsencrypt.org/)
- URL signing: protect your downloads through URL signing and link expiration
- Virtual Hosts: host multiple domains on the same server
- Streaming: if a file is not in the cache, the file is streamed from S3 to the client while being cached so that large files can be download immediately
- Realtime stats: call http://poormanscdnhost/cacheStats with signed request to get realtime stats on transfer and cache size
- Referer control: only allow signed downloads for users coming from your site
- Host control: only allow signed downloads from a specific IP address

## Installation

Add poormanscdn and its package dependencies to your go Go `src` directory:

    go get -v github.com/alexandres/poormanscdn

Once the `get` completes, you should find your new `poormanscdn` (or `poormanscdn.exe`) executable sitting inside `$GOPATH/bin/`.

To update poormanscdn's dependencies, use `go get` with the `-u` option.

    go get -u -v github.com/alexandres/poormanscdn

Run poormanscdn:

    poormanscdn --config config.json

See `$GOPATH/src/github.com/alexandres/poormanscdn/config.json.example` for an example configuration.

## Configuration

All configuration is done by editing the `config.json` file. Options:

- `Listen`: interface and port to listen on - examples: `127.0.0.1:8080` to listen on localhost on port 8080 or `:https` to listen on all interfaces on port 443
- `DefaultAccessKey`: S3 Access Key (alternatively leave this blank and use `AWS_ACCESS_KEY` ENV variable). Override at the `Hosts` level if needed.
- `DefaultSecretKey`: S3 Secret Key (alternatively leave this blank and use `AWS_SECRET_ACCESS_KEY` ENV variable). Override at the `Hosts` level if needed.
- `Secret`: the secret key used to sign download URLs and purge requests - example: use `$ hexdump -n 16 -e '4/4 "%08X" 1 "\n"' /dev/urandom` to generate 128 bit key. (alternatively leave this blank and use `PCDN_SECRET` ENV variable)
- `TmpDir`: where to store temporary files, need not persist between executions
- `CacheDir`: where to store cached files, should persist between executions to avoid emptying the cache
- `CacheSize`: the maximum size in bytes of the cache - example: 40000000000 to use at most 40GB
- `DatabaseDir`: where to store database files, should persist between executions to maintain last-downloaded times for cached files
- `TLSCertificateDir`: where to store Let's Encrypt certificates (if blank, will disable https:// on all hosts). See *Using HTTPS* below.
- `FreeSpaceBatchSizeInBytes`: when the cache is full, free this many bytes, should be at least as large as the largest file you'll store in your cache - example: 1000000000 to free 1GB
- `SigRequired`: if true, only allows downloads using signed URLs
- `Hosts`: dictionary of virtual hosts (see *Virtual Hosts* below), in the format:
	`"domainoripaddress": { "Bucket": "s3bucket", "Path": "base path within bucket", "AccessKey": "leaveblanktouseDefaultAccessKey", "SecretKey": "leaveblanktouseDefaultSecretKey" }`

   Note: Usage of HTTPS requires a valid domain name.


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

### Virtual Hosts

A single poormanscdn server can serve multiple domains through virtual hosts. For example, if you have three domains `myblog.com`, `familyblog.com`, and `carrentals.com`, with:

- `myblog.com` and `familyblog.com` using `DefaultAccessKey` and `DefaultSecretKey` and sharing S3 bucket `blogs` under different paths
- `carrentals.com` in a separate AWS account and bucket using the root path

Your `Hosts` configuration would be:

```
"Hosts": {
	"myblog.com": { "Bucket": "blogs", "Path": "my" },
	"familyblog.com": { "Bucket": "blogs", "Path": "family" },
	"carrentals.com": { "Bucket": "carrentals", "AccessKey": "someotherawsaccesskey", "SecretKey": "someotherawssecretkey" }
}
```

### Automatic HTTPS

Suppose you own domain `myblog.com`. Create an `A record` pointing to the IP address of your poormanscdn server. Then in your `config.json` file:

1. Create an entry `myblog.com` in the `Hosts` section, as shown above.
2. Make sure `TLSCertificateDir` to a writable directory.
3. Set `Listen` to `:https`.

That's it! Start poormanscdn and HTTPS should just work. If you have more domains simply add them to `Hosts` and they will also be HTTPS enabled.

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

## TODO

I would love to receive pull requests for the following features:

- [ ] Script for building binary releases
- [x] Add TLS support using Let's Encrypt - implemented by [bep](https://github.com/bep)
- [x] Cache Invalidation through Purging
- [x] Virtual Hosts - implemented by [bep](https://github.com/bep)
- [ ] Add support for more backends in addition to S3
- [ ] PIP-ify Python signing lib
- [ ] JavaScript signing lib
- [ ] PHP signing lib
- [ ] Ruby signing lib
- [x] Secrets Configuration via ENV

# License

Copyright (c) 2017 Salle, Alexandre <atsalle@inf.ufrgs.br>. All work in this package is distributed under the MIT License.
