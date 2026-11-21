//go:build windows

package main

// Windows has no O_NOFOLLOW. writeFresh still Lstats the path and refuses a
// non-regular file before creating it, and O_EXCL refuses an existing name;
// together those cover the symlink case without an extra flag here.
const noFollowFlag = 0
