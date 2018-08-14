// +build windows plan9 nacl

package main

// borrowed from https://github.com/mholt/caddy

// checkFdlimit issues a warning if the OS limit for
// max file descriptors is below a recommended minimum.
func checkFdlimit() {
}
