//go:build !unix && !windows

package main

// Other targets have no O_NOFOLLOW constant; writeFresh's Lstat check still
// applies. File staging is only exercised on Linux and Windows in this repo.
const noFollowFlag = 0
